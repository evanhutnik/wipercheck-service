package types

type Coordinates struct {
	Latitude  float64 `json:"latitude,omitempty"`
	Longitude float64 `json:"longitude,omitempty"`
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

type Route struct {
	Steps    []Step
	Duration float64
}

type Step struct {
	Name          string        `json:"name,omitempty"`
	StepDuration  float64       `json:"stepDuration,omitempty"`
	TotalDuration float64       `json:"totalDuration,omitempty"`
	Coordinates   Coordinates   `json:"coordinates,omitempty"`
	HourlyWeather HourlyWeather `json:"hourlyWeather,omitempty"`
	Location      *Location     `json:"location,omitempty"`
}

type WeatherData struct {
	Lat    float64
	Lon    float64
	Hourly []HourlyWeather
}

type HourlyWeather struct {
	Time       int64      `json:"unixTime,omitempty"`
	Conditions Conditions `json:"conditions,omitempty"`
	Pop        float64    `json:"pop,omitempty"`
}

type Conditions struct {
	Id          int    `json:"id,omitempty"`
	Main        string `json:"main,omitempty"`
	Description string `json:"description,omitempty"`
}

type RedisHourlyWeather struct {
	Rand   float64
	Hourly HourlyWeather
}

// External Objects

type PSForwardResponse struct {
	Data []*PSCoordinate
}

type PSCoordinate struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Label     string  `json:"label"`
}

type PSReverseResponse struct {
	Data []*PSLocation
}

type PSLocation struct {
	Number   string
	Street   string
	Locality string
	Region   string
	Country  string
}

type OSRMResponse struct {
	Code   string      `json:"code"`
	Routes []OSRMRoute `json:"routes"`
}

type OSRMRoute struct {
	Legs       []OSRMLeg `json:"legs"`
	WeightName string    `json:"weight_name"`
	Weight     float64   `json:"weight"`
	Duration   float64   `json:"duration"`
	Distance   float64   `json:"distance"`
}

type OSRMLeg struct {
	Steps    []OSRMStep `json:"steps"`
	Summary  string     `json:"summary"`
	Weight   float64    `json:"weight"`
	Duration float64    `json:"duration"`
	Distance float64    `json:"distance"`
}

type OSRMStep struct {
	Geometry     string       `json:"geometry"`
	Maneuver     OSRMManeuver `json:"maneuver"`
	Mode         string       `json:"mode"`
	DrivingSide  string       `json:"driving_side"`
	Name         string       `json:"name"`
	Weight       float64      `json:"weight"`
	Duration     float64      `json:"duration"`
	Distance     float64      `json:"distance"`
	Destinations string       `json:"destinations,omitempty"`
	Ref          string       `json:"ref,omitempty"`
}

type OSRMManeuver struct {
	BearingAfter  int       `json:"bearing_after"`
	BearingBefore int       `json:"bearing_before"`
	Location      []float64 `json:"location"`
	Type          string    `json:"type"`
	Modifier      string    `json:"modifier,omitempty"`
	Exit          int       `json:"exit,omitempty"`
}

type OWConditions struct {
	Id          int
	Main        string
	Description string
}

type OWHourlyWeather struct {
	Time       int64          `json:"dt"`
	Conditions []OWConditions `json:"weather"`
	Pop        float64
}

type OWResponse struct {
	Lat    float64
	Lon    float64
	Hourly []OWHourlyWeather
}
