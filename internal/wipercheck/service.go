package wipercheck

import (
	"encoding/json"
	"fmt"
	ps "github.com/evanhutnik/wipercheck-service/internal/positionstack"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"io"
	"net/http"
	"os"
)

type JourneyRequest struct {
	from string
	to   string
	//TODO: when field
}

type JourneyResponse struct {
	Error string `json:"error"`
}

type Service struct {
	psc    *ps.Client
	Logger *zap.SugaredLogger
}

func New() *Service {
	s := &Service{}

	baseLogger, _ := zap.NewProduction()
	defer baseLogger.Sync()
	s.Logger = baseLogger.Sugar()

	s.psc = ps.New(
		ps.ApiKeyOption(os.Getenv("positionstack_apikey")),
		ps.BaseUrlOption(os.Getenv("positionstack_baseurl")),
	)

	return s
}

func (s *Service) Start() {
	mux := http.NewServeMux()
	mux.HandleFunc("/journey", s.Journey)

	_ = http.ListenAndServe(":80", mux)
}

func (s *Service) Journey(w http.ResponseWriter, r *http.Request) {
	req := JourneyRequest{
		from: r.URL.Query().Get("from"),
		to:   r.URL.Query().Get("to"),
	}
	if req.from == "" {
		s.WriteError(w, 400, "Missing 'from' query parameter in request")
		return
	} else if req.to == "" {
		s.WriteError(w, 400, "Missing 'to' query parameter in request.")
		return
	}

	g := new(errgroup.Group)

	g.Go(func() error {
		return s.geoCode(w, req.from)
	})
	g.Go(func() error {
		return s.geoCode(w, req.to)
	})

	if err := g.Wait(); err != nil {
		return
	}

}

func (s *Service) geoCode(w http.ResponseWriter, address string) error {
	toGeo, err := s.psc.GeoCode(address)
	if err != nil {
		s.Logger.Errorw(err.Error(),
			"address", address)
		s.WriteError(w, 500, fmt.Sprintf("Internal error geocoding address '%v'.", address))
	} else if len(toGeo.Data) == 0 {
		s.WriteError(w, 400, fmt.Sprintf("Unrecognized address '%v'. Check spelling or be more specific.", address))
	}
	return err
}

func (s *Service) WriteError(w http.ResponseWriter, statusCode int, errorMsg string) {
	bodyBytes, _ := json.Marshal(JourneyResponse{Error: errorMsg})
	w.WriteHeader(statusCode)
	io.WriteString(w, string(bodyBytes[:]))
}
