package main

import "github.com/evanhutnik/wipercheck-service/internal/wipercheck"

func main() {
	s := wipercheck.New()
	s.Start()
}
