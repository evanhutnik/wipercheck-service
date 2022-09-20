package openweather

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/evanhutnik/wipercheck-service/internal/types"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

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

type Client struct {
	apiKey  string
	baseUrl string
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

func (c Client) GetWeather(ctx context.Context, lat float64, long float64) (*types.WeatherData, error) {
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
	resp, err := http.DefaultClient.Do(ctxReq)
	if err != nil {
		err = errors.New(fmt.Sprintf("error on openweather api request: %s", err.Error()))
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		err = errors.New(fmt.Sprintf("error code %d returned from openweather", resp.StatusCode))
		return nil, err
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		err = errors.New(fmt.Sprintf("error reading body of response: %s", err.Error()))
		return nil, err
	}

	var respObj types.OWResponse
	err = json.Unmarshal(body, &respObj)
	if err != nil {
		err = errors.New(fmt.Sprintf("error unmarshalling response from openweather: %s", err.Error()))
		return nil, err
	}

	wr := &types.WeatherData{
		Lat:    respObj.Lat,
		Lon:    respObj.Lon,
		Hourly: c.hourlyWeatherFromOW(respObj.Hourly),
	}

	return wr, nil
}

func (c Client) GetHourlyWeather(ctx context.Context, coords types.Coordinates, time int64) (*types.HourlyWeather, error) {
	weatherData, err := c.GetWeather(ctx, coords.Latitude, coords.Longitude)
	if err != nil {
		return nil, err
	}
	for _, hourly := range weatherData.Hourly {
		if hourly.Time == time {
			return &hourly, nil
		}
	}
	return nil, errors.New("no hourly weather found for time")
}

func (c Client) hourlyWeatherFromOW(owHourly []types.OWHourlyWeather) []types.HourlyWeather {
	var hourly []types.HourlyWeather
	for _, owHour := range owHourly {
		var conditions types.Conditions
		if len(owHour.Conditions) > 0 {
			conditions = types.Conditions{
				Id:          owHour.Conditions[0].Id,
				Main:        owHour.Conditions[0].Main,
				Description: owHour.Conditions[0].Description,
			}
		}
		hourly = append(hourly, types.HourlyWeather{
			Time:       owHour.Time,
			Conditions: conditions,
			Pop:        owHour.Pop,
		})
	}
	return hourly
}
