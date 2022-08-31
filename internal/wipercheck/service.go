package wipercheck

import (
	"encoding/json"
	"go.uber.org/zap"
	"io"
	"net/http"
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
	Logger *zap.SugaredLogger
}

func New() *Service {
	s := &Service{}

	baseLogger, _ := zap.NewProduction()
	defer baseLogger.Sync()
	s.Logger = baseLogger.Sugar()

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

}

func (s *Service) WriteError(w http.ResponseWriter, statusCode int, errorMsg string) {
	bodyBytes, _ := json.Marshal(JourneyResponse{Error: errorMsg})
	w.WriteHeader(statusCode)
	io.WriteString(w, string(bodyBytes[:]))
}
