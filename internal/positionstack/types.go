package positionstack

type GeoCodeResponse struct {
	Data []*Coordinate
}

type Coordinate struct {
	Latitude  float64
	Longitude float64
	Label     string
}
