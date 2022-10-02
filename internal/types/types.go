package types

type SummaryStep struct {
	Location   string  `json:"location,omitempty"`
	Conditions string  `json:"conditions,omitempty"`
	Pop        float64 `json:"precipChance"`
}

type Route struct {
	Steps    []Step
	Duration float64
}

type Step struct {
	Name          string      `json:"name,omitempty"`
	StepDuration  float64     `json:"stepDuration,omitempty"`
	TotalDuration float64     `json:"totalDuration,omitempty"`
	Coordinates   Coordinates `json:"coordinates,omitempty"`
	Weather       *Weather    `json:"weather,omitempty"`
	Location      *Location   `json:"location,omitempty"`
}

type Weather struct {
	Time       int64      `json:"-"`
	Pop        float64    `json:"precipChance"`
	Conditions Conditions `json:"conditions,omitempty"`
}

type Conditions struct {
	Id          int    `json:"id,omitempty"`
	Main        string `json:"main,omitempty"`
	Description string `json:"description,omitempty"`
	IconURL     string `json:"iconURL,omitempty"`
}

type Location struct {
	Number   string `json:"number,omitempty"`
	Street   string `json:"street,omitempty"`
	Locality string `json:"locality,omitempty"`
	Region   string `json:"region,omitempty"`
	Country  string `json:"country,omitempty"`
}

type Trip struct {
	From *Coordinates
	To   *Coordinates
}

type Coordinates struct {
	Latitude  float64 `json:"latitude,omitempty"`
	Longitude float64 `json:"longitude,omitempty"`
}

type RedisHourlyWeather struct {
	Rand   float64
	Hourly *Weather
}
