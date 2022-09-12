package types

type GeoCodeResponse struct {
	Data []*Coordinate
}

type Coordinate struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Label     string  `json:"label"`
}

type Trip struct {
	From *Coordinate
	To   *Coordinate
}

type Route struct {
	Steps    []Step
	Duration float64
}

type Step struct {
	Name          string
	StepDuration  float64
	TotalDuration float64
	Latitude      float64
	Longitude     float64
	HourlyWeather HourlyWeather
}

type WeatherData struct {
	Lat    float64
	Lon    float64
	Hourly []HourlyWeather
}

type HourlyWeather struct {
	Time       int64 `json:"dt"`
	Conditions Conditions
	Pop        float64
}

type Conditions struct {
	Id          int
	Main        string
	Description string
	Icon        string
}

type RedisHourlyWeather struct {
	Rand   float64
	Hourly HourlyWeather
}

// External Objects

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
	Icon        string
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
