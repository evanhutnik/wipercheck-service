package openweather

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/evanhutnik/wipercheck-service/internal/common"
	"github.com/evanhutnik/wipercheck-service/internal/types"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

type Response struct {
	Lat    float64
	Lon    float64
	Hourly []HourlyWeather
}

type HourlyWeather struct {
	Time       int64 `json:"dt"`
	Pop        float64
	Conditions []Conditions `json:"weather"`
}

type Conditions struct {
	Id          int
	Main        string
	Description string
}

type ClientOption func(*Client)

type Client struct {
	apiKey  string
	baseUrl string
}

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
		panic("Missing apikey in openweather client")
	}
	if c.baseUrl == "" {
		panic("Missing baseUrl in openweather client")
	}
	return c
}

func (c Client) GetWeather(ctx context.Context, lat float64, long float64) ([]types.Weather, error) {
	req, err := url.Parse(c.baseUrl)
	if err != nil {
		err = errors.New(fmt.Sprintf("failed to parse baseUrl %s: %s", c.baseUrl, err.Error()))
		return nil, err
	}

	q := req.Query()
	q.Add("appid", c.apiKey)
	q.Add("lat", strconv.FormatFloat(lat, 'f', -1, 64))
	q.Add("lon", strconv.FormatFloat(long, 'f', -1, 64))
	q.Add("units", "metric")
	q.Add("exclude", "current,minutely,daily,alerts")
	req.RawQuery = q.Encode()

	ctxReq, _ := http.NewRequestWithContext(ctx, "GET", req.String(), nil)
	resp, err := common.GetWithRetry(ctxReq, "openweather")
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		err = errors.New(fmt.Sprintf("error reading body of response: %s", err.Error()))
		return nil, err
	}

	var respObj Response
	err = json.Unmarshal(body, &respObj)
	if err != nil {
		err = errors.New(fmt.Sprintf("error unmarshalling response from openweather: %s", err.Error()))
		return nil, err
	}

	return c.hourlyWeatherFromOW(respObj.Hourly), nil
}

func (c Client) GetHourlyWeather(ctx context.Context, coords types.Coordinates, time int64) (*types.Weather, error) {
	weatherData, err := c.GetWeather(ctx, coords.Latitude, coords.Longitude)
	if err != nil {
		return nil, err
	}
	for _, hourly := range weatherData {
		if hourly.Time == time {
			return &hourly, nil
		}
	}
	return nil, errors.New("no hourly weather found for time")
}

func (c Client) hourlyWeatherFromOW(owHourly []HourlyWeather) []types.Weather {
	var hourly []types.Weather
	for _, owHour := range owHourly {
		var conditions types.Conditions
		if len(owHour.Conditions) > 0 {
			conditions = types.Conditions{
				Id:          owHour.Conditions[0].Id,
				Main:        owHour.Conditions[0].Main,
				Description: owHour.Conditions[0].Description,
			}
		}
		hourly = append(hourly, types.Weather{
			Time:       owHour.Time,
			Conditions: conditions,
			Pop:        owHour.Pop,
		})
	}
	return hourly
}
