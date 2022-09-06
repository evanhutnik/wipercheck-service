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

type RouteResponse struct {
	Code   string  `json:"code"`
	Routes []Route `json:"routes"`
}

type Route struct {
	Legs       []Leg   `json:"legs"`
	WeightName string  `json:"weight_name"`
	Weight     float64 `json:"weight"`
	Duration   float64 `json:"duration"`
	Distance   float64 `json:"distance"`
}

type Leg struct {
	Steps    []Step  `json:"steps"`
	Summary  string  `json:"summary"`
	Weight   float64 `json:"weight"`
	Duration float64 `json:"duration"`
	Distance float64 `json:"distance"`
}

type Step struct {
	Geometry     string   `json:"geometry"`
	Maneuver     Maneuver `json:"maneuver"`
	Mode         string   `json:"mode"`
	DrivingSide  string   `json:"driving_side"`
	Name         string   `json:"name"`
	Weight       float64  `json:"weight"`
	Duration     float64  `json:"duration"`
	Distance     float64  `json:"distance"`
	Destinations string   `json:"destinations,omitempty"`
	Ref          string   `json:"ref,omitempty"`
}

type Maneuver struct {
	BearingAfter  int       `json:"bearing_after"`
	BearingBefore int       `json:"bearing_before"`
	Location      []float64 `json:"location"`
	Type          string    `json:"type"`
	Modifier      string    `json:"modifier,omitempty"`
	Exit          int       `json:"exit,omitempty"`
}
