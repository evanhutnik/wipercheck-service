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
	from       string
	to         string
	reverseGeo bool
	minPop     float64
	delay      int64
}

type JourneyResponse struct {
	Error string   `json:"error,omitempty"`
	Steps []t.Step `json:"steps,omitempty"`
}

type CodeError struct {
	code int
	msg  string
}

func (c CodeError) Error() string {
	return c.msg
}

type Service struct {
	osrm         *osrm.Client
	ow           *ow.Client
	psc          *ps.Client
	rc           *redis.Client
	disableRedis bool

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

	disableRedis, err := strconv.ParseBool(os.Getenv("disable_redis"))
	if err == nil {
		s.disableRedis = disableRedis
	}

	return s
}

func (s *Service) Start() {
	mux := http.NewServeMux()
	mux.HandleFunc("/journey", s.JourneyHandler)

	_ = http.ListenAndServe(":80", mux)
}

func (s *Service) JourneyHandler(w http.ResponseWriter, r *http.Request) {
	resp, err := s.Journey(r.Context(), r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeResponse(w, resp)
}

func (s *Service) Journey(ctx context.Context, r *http.Request) (*JourneyResponse, error) {
	req, err := s.parseRequest(r)
	if err != nil {
		return nil, err
	}

	trip, err := s.tripCoordinates(ctx, req)
	if err != nil {
		return nil, err
	}

	route, err := s.tripRoute(ctx, trip)
	if err != nil {
		return nil, err
	}

	steps := s.steps(route)

	s.weather(ctx, steps, req.delay)

	resp, err := s.response(ctx, steps, req)

	return resp, nil
}

func (s *Service) parseRequest(r *http.Request) (*JourneyRequest, error) {
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	if from == "" {
		return nil, CodeError{code: 400, msg: "Missing 'from' query parameter in request"}
	} else if to == "" {
		return nil, CodeError{code: 400, msg: "Missing 'to' query parameter in request"}
	}
	req := &JourneyRequest{
		from: from,
		to:   to,
	}
	reverseGeo, err := strconv.ParseBool(r.URL.Query().Get("reverseGeo"))
	if err == nil {
		req.reverseGeo = reverseGeo
	}
	minPop, err := strconv.ParseFloat(r.URL.Query().Get("minPop"), 64)
	if err == nil {
		req.minPop = minPop
	}

	if r.URL.Query().Get("delay") != "" {
		delay, err := strconv.ParseInt(r.URL.Query().Get("delay"), 10, 64)
		if err != nil || delay > 43200 {
			return nil, CodeError{code: 400, msg: "'delay' parameter must be an integer less than 43200 (12 hours)"}
		}
		req.delay = delay
	}

	return req, nil
}

func (s *Service) tripCoordinates(ctx context.Context, req *JourneyRequest) (*t.Trip, error) {
	var fromCoord, toCoord *t.Coordinates
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

func (s *Service) geoCode(ctx context.Context, address string) (*t.Coordinates, error) {
	toGeo, err := s.psc.GeoCode(ctx, address)
	if err != nil {
		s.Logger.Errorw(err.Error(),
			"address", address, "action", "GeoCode")
		return nil, CodeError{code: 500, msg: fmt.Sprintf("Internal error geocoding address '%v'.", address)}
	} else if toGeo == nil {
		return nil, CodeError{code: 400, msg: fmt.Sprintf("Unrecognized address '%v'. Check spelling or be more specific.", address)}
	}
	return toGeo, err
}

func (s *Service) tripRoute(ctx context.Context, trip *t.Trip) (*t.Route, error) {
	route, err := s.osrm.Route(ctx, trip)
	if err != nil {
		s.Logger.Errorf("Error routing trip (%v,%v) to (%v,%v): %v",
			trip.From.Latitude, trip.From.Longitude, trip.To.Latitude, trip.From.Longitude, err.Error())
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
		if currentDuration >= goalDuration {
			weatherStep := routeSteps[i]
			weatherStep.TotalDuration = currentDuration
			if weatherStep.Name == "" {
				weatherStep.Name = s.lastNamedStep(routeSteps, i)
			}
			weatherSteps = append(weatherSteps, weatherStep)
			goalDuration = currentDuration + durationStep
		}
		currentDuration += step.StepDuration
	}
	return weatherSteps
}

func (s *Service) weather(ctx context.Context, steps []t.Step, delay int64) {
	wg := new(sync.WaitGroup)
	wg.Add(len(steps))

	for i, step := range steps {
		i, step := i, step
		go func() {
			defer wg.Done()
			unixTime := time.Now().Unix() + int64(step.TotalDuration) + delay
			stepHour := time.Unix(unixTime, 0).UTC().Truncate(time.Hour).UTC().Unix()
			if !s.disableRedis {
				geoResponse := s.rc.GeoRadius(ctx, strconv.FormatInt(stepHour, 10), step.Coordinates.Longitude, step.Coordinates.Latitude,
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
						step.Coordinates.Latitude, step.Coordinates.Longitude, err.Error())
				}
				if len(locations) > 0 {
					var redisWeather t.RedisHourlyWeather
					err := json.Unmarshal([]byte(locations[0].Name), &redisWeather)
					if err != nil {
						s.Logger.Errorf("Error unmarshalling redis weather for (%v, %v): %v",
							step.Coordinates.Latitude, step.Coordinates.Longitude, err.Error())
					} else {
						redisWeather.Hourly.Time = stepHour
						step.HourlyWeather = redisWeather.Hourly
						steps[i] = step
						return
					}
				}
			}
			hourly, err := s.ow.GetHourlyWeather(ctx, step.Coordinates, stepHour)
			if err != nil {
				s.Logger.Warnf("Error getting hourly weather data: %v", err.Error())
				return
			}
			hourly.Time = stepHour
			step.HourlyWeather = *hourly
			steps[i] = step
		}()
	}
	wg.Wait()
}

func (s *Service) response(ctx context.Context, steps []t.Step, req *JourneyRequest) (*JourneyResponse, error) {
	resp := &JourneyResponse{}
	for _, step := range steps {
		if step.HourlyWeather.Pop >= req.minPop {
			resp.Steps = append(resp.Steps, step)
		}
	}
	if req.reverseGeo {
		wg := new(sync.WaitGroup)
		wg.Add(len(resp.Steps))
		for i, step := range resp.Steps {
			i, step := i, step
			go func() {
				defer wg.Done()
				location, err := s.psc.ReverseGeoCode(ctx, step.Coordinates)
				if err != nil {
					s.Logger.Warnf("Error reverse geocoding (%v,%v): %v",
						step.Coordinates.Latitude, step.Coordinates.Longitude, err.Error())
					return
				}
				step.Location = location
				resp.Steps[i] = step
			}()
		}
		wg.Wait()
	}
	return resp, nil
}

func (s *Service) lastNamedStep(routeSteps []t.Step, i int) string {
	if i == 0 {
		return ""
	} else if routeSteps[i-1].Name != "" {
		return routeSteps[i-1].Name
	} else {
		return s.lastNamedStep(routeSteps, i-1)
	}
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

func (s *Service) writeResponse(w http.ResponseWriter, resp *JourneyResponse) {
	bodyBytes, _ := json.Marshal(resp)
	w.WriteHeader(200)
	io.WriteString(w, string(bodyBytes[:]))
}
