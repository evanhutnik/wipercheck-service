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

type CodeError struct {
	code int
	msg  string
}

func (c CodeError) Error() string {
	return c.msg
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
	mux.HandleFunc("/journey", s.JourneyHandler)

	_ = http.ListenAndServe(":80", mux)
}

func (s *Service) JourneyHandler(w http.ResponseWriter, r *http.Request) {
	err := s.Journey(r)
	if err != nil {
		s.writeError(w, err)
	}
}

func (s *Service) Journey(r *http.Request) error {
	req := JourneyRequest{
		from: r.URL.Query().Get("from"),
		to:   r.URL.Query().Get("to"),
	}
	if req.from == "" {
		return CodeError{code: 400, msg: "Missing 'from' query parameter in request"}
	} else if req.to == "" {
		return CodeError{code: 400, msg: "Missing 'to' query parameter in request"}
	}

	var fromCoord, toCoord *ps.Coordinate
	g := new(errgroup.Group)

	g.Go(func() error {
		var err error
		fromCoord, err = s.geoCode(req.from)
		return err
	})
	g.Go(func() error {
		var err error
		toCoord, err = s.geoCode(req.to)
		return err
	})

	if err := g.Wait(); err != nil {
		return err
	}

	return nil
}

func (s *Service) geoCode(address string) (*ps.Coordinate, error) {
	toGeo, err := s.psc.GeoCode(address)
	if err != nil {
		s.Logger.Errorw(err.Error(),
			"address", address, "action", "GeoCode")
		return nil, CodeError{code: 500, msg: fmt.Sprintf("Internal error geocoding address '%v'.", address)}
	} else if len(toGeo.Data) == 0 {
		return nil, CodeError{code: 400, msg: fmt.Sprintf("Unrecognized address '%v'. Check spelling or be more specific.", address)}
	}
	return toGeo.Data[0], err
}

func (s *Service) writeError(w http.ResponseWriter, err error) {
	codeErr, ok := err.(CodeError)
	if ok {
		bodyBytes, _ := json.Marshal(JourneyResponse{Error: codeErr.Error()})
		w.WriteHeader(codeErr.code)
		io.WriteString(w, string(bodyBytes[:]))
	} else {
		w.WriteHeader(500)
		io.WriteString(w, "Internal server error")
	}
}
