package osrm

import (
	"encoding/json"
	"errors"
	"fmt"
	t "github.com/evanhutnik/wipercheck-service/internal/types"
	"io"
	"net/http"
	"net/url"
)

type ClientOption func(*Client)

func BaseUrlOption(baseUrl string) ClientOption {
	return func(c *Client) {
		c.baseUrl = baseUrl
	}
}

type Client struct {
	baseUrl string
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

func (c *Client) Route(trip *t.Trip) (*t.RouteResponse, error) {
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

	resp, err := http.Get(req.String())
	if err != nil {
		err = errors.New(fmt.Sprintf("error on osrm api request: %s", err.Error()))
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		err = errors.New(fmt.Sprintf("error code %d returned from osrm", resp.StatusCode))
		return nil, err
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		err = errors.New(fmt.Sprintf("error reading osrm response body: %s", err.Error()))
		return nil, err
	}
	//unmarshal into object
	var respObj t.RouteResponse
	err = json.Unmarshal(body, &respObj)
	if err != nil {
		err = errors.New(fmt.Sprintf("error unmarshalling response from osrm: %s", err.Error()))
		return nil, err
	}
	return &respObj, nil
}
