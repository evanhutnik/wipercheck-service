package positionstack

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

type ForwardResponse struct {
	Data []*Coordinate
}

type Coordinate struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Label     string  `json:"label"`
}

type ReverseResponse struct {
	Data []*Location
}

type Location struct {
	Number   string
	Street   string
	Locality string
	Region   string
	Country  string
}

type Client struct {
	apiKey  string
	baseUrl string
}

type ClientOption func(*Client)

func ApiKeyOption(apiKey string) ClientOption {
	return func(c *Client) {
		c.apiKey = apiKey
	}
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

	if c.apiKey == "" {
		panic("Missing apikey in positionStack client")
	}
	if c.baseUrl == "" {
		panic("Missing baseUrl in positionStack client")
	}
	return c
}

func (c *Client) GeoCode(ctx context.Context, location string) (*t.Coordinates, error) {
	req, err := url.Parse(fmt.Sprintf("%v/forward", c.baseUrl))
	if err != nil {
		err = errors.New(fmt.Sprintf("failed to parse positionstack baseUrl %s: %s", c.baseUrl, err.Error()))
		return nil, err
	}

	q := req.Query()
	q.Add("access_key", c.apiKey)
	q.Add("query", location)
	q.Add("limit", "1")
	req.RawQuery = q.Encode()

	ctxReq, _ := http.NewRequestWithContext(ctx, "GET", req.String(), nil)
	resp, err := common.GetWithRetry(ctxReq, "positionstack")
	if err != nil {
		return nil, err
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		err = errors.New(fmt.Sprintf("error reading positionstack response body: %s", err.Error()))
		return nil, err
	}

	var respObj ForwardResponse
	err = json.Unmarshal(body, &respObj)
	if err != nil {
		err = errors.New(fmt.Sprintf("error unmarshalling response from positionstack: %s", err.Error()))
		return nil, err
	} else if len(respObj.Data) == 0 {
		return nil, nil
	}
	return &t.Coordinates{
		Latitude:  respObj.Data[0].Latitude,
		Longitude: respObj.Data[0].Longitude,
	}, nil
}

func (c *Client) ReverseGeoCode(ctx context.Context, coords t.Coordinates) (*t.Location, error) {
	req, err := url.Parse(fmt.Sprintf("%v/reverse", c.baseUrl))
	if err != nil {
		err = errors.New(fmt.Sprintf("failed to parse positionstack baseUrl %s: %s", c.baseUrl, err.Error()))
		return nil, err
	}

	q := req.Query()
	q.Add("access_key", c.apiKey)
	q.Add("query", fmt.Sprintf("%v,%v", coords.Latitude, coords.Longitude))
	q.Add("limit", "1")
	req.RawQuery = q.Encode()

	ctxReq, _ := http.NewRequestWithContext(ctx, "GET", req.String(), nil)
	resp, err := http.DefaultClient.Do(ctxReq)
	if err != nil {
		err = errors.New(fmt.Sprintf("error on positionstack reverse geocode request: %s", err.Error()))
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		err = errors.New(fmt.Sprintf("error code %d returned from positionstack", resp.StatusCode))
		return nil, err
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		err = errors.New(fmt.Sprintf("error reading positionstack response body: %s", err.Error()))
		return nil, err
	}

	var respObj ReverseResponse
	err = json.Unmarshal(body, &respObj)
	if err != nil {
		err = errors.New(fmt.Sprintf("error unmarshalling response from positionstack: %s", err.Error()))
		return nil, err
	} else if len(respObj.Data) == 0 {
		return nil, nil
	}

	return &t.Location{
		Number:   respObj.Data[0].Number,
		Street:   respObj.Data[0].Street,
		Locality: respObj.Data[0].Locality,
		Region:   respObj.Data[0].Region,
		Country:  respObj.Data[0].Country,
	}, nil
}
