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
	_ "github.com/joho/godotenv/autoload"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"io"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type JourneyRequest struct {
	from   string
	to     string
	minPop float64
	delay  int64
}

type JourneyResponse struct {
	Error   string          `json:"error,omitempty"`
	Summary []t.SummaryStep `json:"summary,omitempty"`
	Steps   []t.Step        `json:"detailedSteps,omitempty"`
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
	mux.HandleFunc("/health", s.HealthCheckHandler)

	_ = http.ListenAndServe(":8080", mux)
}

// HealthCheckHandler is the handler called by the AWS application target group to determine if the service is healthy
func (s *Service) HealthCheckHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(200)
	io.WriteString(w, "OK")
}

// JourneyHandler is the handler for the /journey endpoint of wipercheck-service
func (s *Service) JourneyHandler(w http.ResponseWriter, r *http.Request) {
	resp, err := s.Journey(r.Context(), r)
	if err != nil {
		writeError(w, err)
		return
	}
	writeResponse(w, resp)
}

// Journey contains the core logic for the service, from parsing the request to generating the response
func (s *Service) Journey(ctx context.Context, r *http.Request) (*JourneyResponse, error) {
	req, err := s.validateRequest(r)
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

	steps := s.weather(ctx, route, req.delay)

	resp, err := s.response(ctx, steps, req)

	return resp, nil
}

// validateRequest validates the arguments passed in the request
func (s *Service) validateRequest(r *http.Request) (*JourneyRequest, error) {
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
	minPop, err := strconv.ParseFloat(r.URL.Query().Get("minPop"), 64)
	if err == nil {
		if minPop > 100 {
			return nil, CodeError{code: 400, msg: "'minPop' parameter must be less than 100%"}
		}
		req.minPop = minPop
	}

	if r.URL.Query().Get("delay") != "" {
		delay, err := strconv.ParseInt(r.URL.Query().Get("delay"), 10, 64)
		if err != nil || delay > 720 {
			return nil, CodeError{code: 400, msg: "'delay' parameter must be less than 720 minutes (12 hours)"}
		}
		req.delay = delay
	}

	return req, nil
}

// tripCoordinates converts the 'to' and 'from' fields from unstructured text to coordinates
func (s *Service) tripCoordinates(ctx context.Context, req *JourneyRequest) (*t.Trip, error) {
	var fromCoord, toCoord *t.Coordinates
	// spinning up separate goroutines to geocode the two addresses simultaneously
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

	// both goroutines must complete before proceeding
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return &t.Trip{
		From: fromCoord,
		To:   toCoord,
	}, nil
}

// geoCode is a wrapper function for handling errors returned from the PositionStack client GeoCode method
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

// tripRoute is a wrapper function for handling errors returned from the OSRM client Route method
func (s *Service) tripRoute(ctx context.Context, trip *t.Trip) (*t.Route, error) {
	route, err := s.osrm.Route(ctx, trip)
	if err != nil {
		s.Logger.Errorf("Error routing trip (%v,%v) to (%v,%v): %v",
			trip.From.Latitude, trip.From.Longitude, trip.To.Latitude, trip.From.Longitude, err.Error())
		return nil, CodeError{code: 500, msg: "Internal error retrieving trip route."}
	}
	return route, nil
}

// weather returns the relevant forecasted weather data for the user's trip
func (s *Service) weather(ctx context.Context, route *t.Route, delay int64) []t.Step {
	steps := s.steps(route)

	// spinning up separate goroutines to analyze weather data of all steps simultaneously
	wg := new(sync.WaitGroup)
	wg.Add(len(steps))
	for i, step := range steps {
		// explicitly declaring values as they would change during execution due to async loop otherwise
		i, step := i, step
		go func() {
			defer wg.Done()
			unixTime := time.Now().Unix() + int64(step.TotalDuration) + delay
			stepHour := time.Unix(unixTime, 0).UTC().Truncate(time.Hour).UTC().Unix()
			// querying for forecasted weather data cached by wipercheck-loader
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
						step.Weather = redisWeather.Hourly
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
			step.Weather = hourly
			steps[i] = step
		}()
	}
	wg.Wait()
	var weatherSteps []t.Step
	// only including steps that have non-nil weather in response
	for _, step := range steps {
		if step.Weather != nil {
			weatherSteps = append(weatherSteps, step)
		}
	}
	return weatherSteps
}

// steps returns the steps from the OSRM route that the service will retrieve forecasted weather data for
func (s *Service) steps(route *t.Route) []t.Step {
	var tripDuration, durationStep float64
	tripDuration = route.Duration
	// determining how many steps to analyze forecasted weather data for
	switch {
	case tripDuration > 18000: // if trip is over 5 hours long, analyze 20 steps distributed along trip evenly
		durationStep = tripDuration / 20
	case tripDuration > 7200: // over 2 hours but under 5 hours, analyze every 15 minutes
		durationStep = 15 * 60
	case tripDuration > 3600: // over 1 hour but under 2 hours, analyze every 10 minutes
		durationStep = 10 * 60
	case tripDuration > 300: // over 5 minutes but under 1 hour, analyze every 5 minutes
		durationStep = 5 * 60
	default: // less than 5 minutes, divide the trip into thirds
		durationStep = tripDuration / 3
	}
	routeSteps := route.Steps
	var weatherSteps []t.Step
	var currentDuration, goalDuration float64
	goalDuration = durationStep
	for i, step := range routeSteps {
		// checking if the correct amount of time as specified above has elapsed before we analyze forecasted weather data
		if currentDuration >= goalDuration {
			weatherStep := routeSteps[i]
			weatherStep.TotalDuration = math.Round(currentDuration)
			weatherStep.StepDuration = math.Round(weatherStep.StepDuration)
			// some steps don't include a name if it's the same as the previous one
			if weatherStep.Name == "" {
				weatherStep.Name = lastNamedStep(routeSteps, i)
			}
			weatherSteps = append(weatherSteps, weatherStep)
			goalDuration = currentDuration + durationStep
		}
		currentDuration += step.StepDuration
	}
	return weatherSteps
}

// response builds the response object for the /journey endpoint, including reverse geocoding coordinates and generating the summary
func (s *Service) response(ctx context.Context, steps []t.Step, req *JourneyRequest) (*JourneyResponse, error) {
	resp := &JourneyResponse{}
	for _, step := range steps {
		if step.Weather.Pop >= req.minPop {
			resp.Steps = append(resp.Steps, step)
		}
	}
	wg := new(sync.WaitGroup)
	wg.Add(len(resp.Steps))
	for i, step := range resp.Steps {
		// explicitly declaring values as they would change during execution due to async loop otherwise
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

	var summary []t.SummaryStep
	for i, step := range resp.Steps {
		if len(summary) == 0 || summary[len(summary)-1].Pop != step.Weather.Pop*100 || i == len(resp.Steps)-1 {
			weatherDesc := step.Weather.Conditions.Description
			summaryStep := t.SummaryStep{
				Location:   summaryStepLocation(step.Location),
				Pop:        step.Weather.Pop * 100,
				Conditions: strings.ToUpper(string(weatherDesc[0])) + weatherDesc[1:],
			}
			summary = append(summary, summaryStep)
		}
	}
	resp.Summary = summary

	return resp, nil
}

// summaryStepLocation returns a string representation of a Location struct
func summaryStepLocation(loc *t.Location) string {
	var builder strings.Builder
	if loc.Locality != "" {
		builder.WriteString(loc.Locality + ", ")
	}
	if loc.Region != "" {
		builder.WriteString(loc.Region)
	}
	return builder.String()
}

func lastNamedStep(routeSteps []t.Step, i int) string {
	if i == 0 {
		return ""
	} else if routeSteps[i-1].Name != "" {
		return routeSteps[i-1].Name
	} else {
		return lastNamedStep(routeSteps, i-1)
	}
}

func writeError(w http.ResponseWriter, err error) {
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

func writeResponse(w http.ResponseWriter, resp *JourneyResponse) {
	bodyBytes, _ := json.Marshal(resp)
	w.WriteHeader(200)
	io.WriteString(w, string(bodyBytes[:]))
}
