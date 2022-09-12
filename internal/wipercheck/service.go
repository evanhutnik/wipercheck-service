package wipercheck

import (
	"context"
	"encoding/json"
	"fmt"
	ow "github.com/evanhutnik/wipercheck-service/internal/openweather"
	"github.com/evanhutnik/wipercheck-service/internal/osrm"
	ps "github.com/evanhutnik/wipercheck-service/internal/positionstack"
	t "github.com/evanhutnik/wipercheck-service/internal/types"
	"github.com/go-redis/redis/v8"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"io"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

type JourneyRequest struct {
	from string
	to   string
	//TODO: when field
}

type JourneyResponse struct {
	Error string `json:"error"`
}

type CodeError struct {
	code int
	msg  string
}

func (c CodeError) Error() string {
	return c.msg
}

type Service struct {
	osrm *osrm.Client
	ow   *ow.Client
	psc  *ps.Client
	rc   *redis.Client

	Logger *zap.SugaredLogger
}

func New() *Service {
	s := &Service{}

	baseLogger, _ := zap.NewProduction()
	defer baseLogger.Sync()
	s.Logger = baseLogger.Sugar()

	s.psc = ps.New(
		ps.ApiKeyOption(os.Getenv("positionstack_apikey")),
		ps.BaseUrlOption(os.Getenv("positionstack_baseurl")),
	)

	s.osrm = osrm.New(
		osrm.BaseUrlOption(os.Getenv("osrm_baseurl")),
	)

	s.ow = ow.New(
		ow.ApiKeyOption(os.Getenv("openweather_apikey")),
		ow.BaseUrlOption(os.Getenv("openweather_baseurl")),
	)

	s.rc = redis.NewClient(&redis.Options{
		Addr: os.Getenv("redis_address"),
	})

	return s
}

func (s *Service) Start() {
	mux := http.NewServeMux()
	mux.HandleFunc("/journey", s.JourneyHandler)

	_ = http.ListenAndServe(":80", mux)
}

func (s *Service) JourneyHandler(w http.ResponseWriter, r *http.Request) {
	err := s.Journey(r.Context(), r)
	if err != nil {
		s.writeError(w, err)
	}
}

func (s *Service) Journey(ctx context.Context, r *http.Request) error {
	req := JourneyRequest{
		from: r.URL.Query().Get("from"),
		to:   r.URL.Query().Get("to"),
	}
	if req.from == "" {
		return CodeError{code: 400, msg: "Missing 'from' query parameter in request"}
	} else if req.to == "" {
		return CodeError{code: 400, msg: "Missing 'to' query parameter in request"}
	}

	trip, err := s.getTripCoordinates(ctx, req)
	if err != nil {
		return err
	}

	route, err := s.getTripRoute(ctx, trip)
	if err != nil {
		return err
	}
	s.hourlyWeather(ctx, s.steps(route))

	return nil
}

func (s *Service) getTripCoordinates(ctx context.Context, req JourneyRequest) (*t.Trip, error) {
	var fromCoord, toCoord *t.Coordinate
	g := new(errgroup.Group)

	g.Go(func() error {
		var err error
		fromCoord, err = s.geoCode(ctx, req.from)
		return err
	})
	g.Go(func() error {
		var err error
		toCoord, err = s.geoCode(ctx, req.to)
		return err
	})

	if err := g.Wait(); err != nil {
		return nil, err
	}
	return &t.Trip{
		From: fromCoord,
		To:   toCoord,
	}, nil
}

func (s *Service) geoCode(ctx context.Context, address string) (*t.Coordinate, error) {
	toGeo, err := s.psc.GeoCode(ctx, address)
	if err != nil {
		s.Logger.Errorw(err.Error(),
			"address", address, "action", "GeoCode")
		return nil, CodeError{code: 500, msg: fmt.Sprintf("Internal error geocoding address '%v'.", address)}
	} else if len(toGeo.Data) == 0 {
		return nil, CodeError{code: 400, msg: fmt.Sprintf("Unrecognized address '%v'. Check spelling or be more specific.", address)}
	}
	return toGeo.Data[0], err
}

func (s *Service) getTripRoute(ctx context.Context, trip *t.Trip) (*t.Route, error) {
	route, err := s.osrm.Route(ctx, trip)
	if err != nil {
		s.Logger.Errorw(err.Error(),
			"from", trip.From.Label, "to", trip.To.Label)
		return nil, CodeError{code: 500, msg: "Internal error retrieving trip route."}
	}
	return route, nil
}

func (s *Service) steps(route *t.Route) []t.Step {
	var tripDuration, durationStep float64
	tripDuration = route.Duration
	switch {
	case tripDuration > 18000:
		durationStep = tripDuration / 20
	case tripDuration > 7200:
		durationStep = 15 * 60
	case tripDuration > 3600:
		durationStep = 10 * 60
	case tripDuration > 300:
		durationStep = 5 * 60
	default:
		durationStep = tripDuration / 3
	}

	routeSteps := route.Steps
	var weatherSteps []t.Step
	var currentDuration, goalDuration float64
	goalDuration = durationStep
	for i, step := range routeSteps {
		currentDuration += step.StepDuration
		if currentDuration >= goalDuration {
			weatherStep := routeSteps[i]
			weatherStep.TotalDuration = currentDuration
			weatherSteps = append(weatherSteps, weatherStep)
			goalDuration = currentDuration + durationStep
		}
	}
	return weatherSteps
}

func (s *Service) hourlyWeather(ctx context.Context, steps []t.Step) {
	wg := new(sync.WaitGroup)
	wg.Add(len(steps))

	for i, step := range steps {
		i, step := i, step
		go func() {
			defer wg.Done()
			unixTime := time.Now().Unix() + int64(step.TotalDuration)
			stepHour := time.Unix(unixTime, 0).UTC().Truncate(time.Hour).UTC().Unix()
			geoResponse := s.rc.GeoRadius(ctx, strconv.FormatInt(stepHour, 10), step.Longitude, step.Latitude,
				&redis.GeoRadiusQuery{
					Radius:    10,
					Unit:      "km",
					WithCoord: true,
					WithDist:  true,
					Count:     1,
					Sort:      "ASC",
				})
			locations, err := geoResponse.Result()
			if err != nil {
				s.Logger.Errorf("Redis error when fetching GeoRadius for (%v, %v): %v",
					step.Latitude, step.Longitude, err.Error())
			}
			if len(locations) > 0 {
				var redisWeather t.RedisHourlyWeather
				err := json.Unmarshal([]byte(locations[0].Name), &redisWeather)
				if err != nil {
					s.Logger.Errorf("Error unmarshalling redis weather for (%v, %v): %v",
						step.Latitude, step.Longitude, err.Error())
				} else {
					step.HourlyWeather = redisWeather.Hourly
					steps[i] = step
					return
				}
			}
			hourly, err := s.ow.GetHourlyWeather(ctx, step.Latitude, step.Longitude, stepHour)
			if err != nil {
				s.Logger.Warnf("Error getting hourly weather data: %v", err.Error())
				return
			}
			step.HourlyWeather = *hourly
			steps[i] = step
		}()
	}
	wg.Wait()
}

func (s *Service) writeError(w http.ResponseWriter, err error) {
	codeErr, ok := err.(CodeError)
	if ok {
		bodyBytes, _ := json.Marshal(JourneyResponse{Error: codeErr.Error()})
		w.WriteHeader(codeErr.code)
		io.WriteString(w, string(bodyBytes[:]))
	} else {
		w.WriteHeader(500)
		io.WriteString(w, "Internal server error")
	}
}
