package osrm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/evanhutnik/wipercheck-service/internal/common"
	t "github.com/evanhutnik/wipercheck-service/internal/types"
	"io"
	"net/http"
	"net/url"
)

type Response struct {
	Code   string  `json:"code"`
	Routes []Route `json:"routes"`
}

type Route struct {
	WeightName string  `json:"weight_name"`
	Weight     float64 `json:"weight"`
	Duration   float64 `json:"duration"`
	Distance   float64 `json:"distance"`
	Legs       []Leg   `json:"legs"`
}

type Leg struct {
	Summary  string  `json:"summary"`
	Weight   float64 `json:"weight"`
	Duration float64 `json:"duration"`
	Distance float64 `json:"distance"`
	Steps    []Step  `json:"steps"`
}

type Step struct {
	Geometry     string   `json:"geometry"`
	Mode         string   `json:"mode"`
	DrivingSide  string   `json:"driving_side"`
	Name         string   `json:"name"`
	Weight       float64  `json:"weight"`
	Duration     float64  `json:"duration"`
	Distance     float64  `json:"distance"`
	Destinations string   `json:"destinations,omitempty"`
	Ref          string   `json:"ref,omitempty"`
	Maneuver     Maneuver `json:"maneuver"`
}

type Maneuver struct {
	BearingAfter  int       `json:"bearing_after"`
	BearingBefore int       `json:"bearing_before"`
	Location      []float64 `json:"location"`
	Type          string    `json:"type"`
	Modifier      string    `json:"modifier,omitempty"`
	Exit          int       `json:"exit,omitempty"`
}

type ClientOption func(*Client)

type Client struct {
	baseUrl string
}

func BaseUrlOption(baseUrl string) ClientOption {
	return func(c *Client) {
		c.baseUrl = baseUrl
	}
}

func New(opts ...ClientOption) *Client {
	c := &Client{}
	for _, opt := range opts {
		opt(c)
	}

	if c.baseUrl == "" {
		panic("Missing baseUrl in osrm client")
	}
	return c
}

func (c *Client) Route(ctx context.Context, trip *t.Trip) (*t.Route, error) {
	reqUrl := fmt.Sprintf("%v/%f,%f;%f,%f", c.baseUrl, trip.From.Longitude, trip.From.Latitude, trip.To.Longitude, trip.To.Latitude)
	req, err := url.Parse(reqUrl)
	if err != nil {
		err = errors.New(fmt.Sprintf("failed to parse osrm url %s: %s", reqUrl, err.Error()))
		return nil, err
	}

	q := req.Query()
	q.Add("steps", "true")
	q.Add("overview", "false")
	req.RawQuery = q.Encode()

	ctxReq, _ := http.NewRequestWithContext(ctx, "GET", req.String(), nil)
	resp, err := common.GetWithRetry(ctxReq, "osrm")
	if err != nil {
		return nil, err
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		err = errors.New(fmt.Sprintf("error reading osrm response body: %s", err.Error()))
		return nil, err
	}

	var respObj Response
	err = json.Unmarshal(body, &respObj)
	if err != nil {
		err = errors.New(fmt.Sprintf("error unmarshalling response from osrm: %s", err.Error()))
		return nil, err
	}

	route := &t.Route{
		Steps:    c.routeStepsFromOSRM(respObj.Routes[0].Legs[0].Steps),
		Duration: respObj.Routes[0].Duration,
	}
	return route, nil
}

func (c Client) routeStepsFromOSRM(osrm []Step) []t.Step {
	var routeSteps []t.Step
	for _, step := range osrm {
		routeSteps = append(routeSteps, t.Step{
			Name:         step.Name,
			StepDuration: step.Duration,
			Coordinates: t.Coordinates{
				Latitude:  step.Maneuver.Location[1],
				Longitude: step.Maneuver.Location[0],
			},
		})
	}
	return routeSteps
}
