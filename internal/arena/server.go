package arena

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type ServerConfig struct {
	Origin            string
	Runner            *Runner
	Store             *Store
	Sink              *DatasetSink
	RequestsPerMinute int
	MaxConcurrent     int
}

type Server struct {
	origin string
	host   string
	runner *Runner
	store  *Store
	sink   *DatasetSink
	csrf   string
	page   []byte
	limit  int
	sem    chan struct{}
	mu     sync.Mutex
	recent []time.Time
	now    func() time.Time
}

func NewServer(config ServerConfig) (*Server, error) {
	if config.Runner == nil {
		return nil, errors.New("arena runner is required")
	}
	parsedOrigin, err := url.Parse(strings.TrimSuffix(config.Origin, "/"))
	if err != nil || parsedOrigin.Scheme != "http" || parsedOrigin.Host == "" || parsedOrigin.Path != "" || parsedOrigin.RawQuery != "" || parsedOrigin.Fragment != "" || parsedOrigin.User != nil {
		return nil, errors.New("arena origin must be an http origin without a path")
	}
	hostname := parsedOrigin.Hostname()
	address := net.ParseIP(hostname)
	if hostname != "localhost" && (address == nil || !address.IsLoopback()) {
		return nil, errors.New("arena origin must use a loopback host")
	}
	csrf, err := randomID("")
	if err != nil {
		return nil, err
	}
	page, err := renderPage(csrf)
	if err != nil {
		return nil, err
	}
	if config.Store == nil {
		config.Store = NewStore()
	}
	if config.RequestsPerMinute <= 0 {
		config.RequestsPerMinute = 10
	}
	if config.MaxConcurrent <= 0 {
		config.MaxConcurrent = 2
	}
	return &Server{
		origin: parsedOrigin.String(), host: parsedOrigin.Host, runner: config.Runner, store: config.Store,
		sink: config.Sink, csrf: csrf, page: page, limit: config.RequestsPerMinute,
		sem: make(chan struct{}, config.MaxConcurrent), now: time.Now,
	}, nil
}

func (s *Server) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	setArenaHeaders(writer.Header())
	if !strings.EqualFold(request.Host, s.host) {
		writeAPIError(writer, http.StatusBadRequest, "invalid host")
		return
	}
	switch {
	case (request.Method == http.MethodGet || request.Method == http.MethodHead) && request.URL.Path == "/":
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = writer.Write(s.page)
	case (request.Method == http.MethodGet || request.Method == http.MethodHead) && request.URL.Path == "/assets/app.css":
		s.serveAsset(writer, "app.css", "text/css; charset=utf-8")
	case (request.Method == http.MethodGet || request.Method == http.MethodHead) && request.URL.Path == "/assets/app.js":
		s.serveAsset(writer, "app.js", "text/javascript; charset=utf-8")
	case (request.Method == http.MethodGet || request.Method == http.MethodHead) && request.URL.Path == "/api/v1/arena/config":
		s.serveConfig(writer)
	case request.Method == http.MethodPost && request.URL.Path == "/api/v1/arena/compare":
		s.serveCompare(writer, request)
	case request.Method == http.MethodPost && request.URL.Path == "/api/v1/arena/save":
		s.serveSave(writer, request)
	case request.URL.Path == "/api/v1/arena/compare" || request.URL.Path == "/api/v1/arena/save":
		writer.Header().Set("Allow", "POST")
		writeAPIError(writer, http.StatusMethodNotAllowed, "method not allowed")
	default:
		http.NotFound(writer, request)
	}
}

func (s *Server) serveConfig(writer http.ResponseWriter) {
	type modelView struct {
		ID        string `json:"id"`
		Provider  string `json:"provider"`
		Available bool   `json:"available"`
	}
	models := s.runner.Models()
	views := make([]modelView, 0, len(models))
	for _, model := range models {
		views = append(views, modelView{ID: model.ID, Provider: model.Provider, Available: model.Available})
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"models": views, "save_enabled": s.sink != nil,
		"limits": map[string]any{"max_tokens": MaxOutputTokens, "timeout_ms": s.runner.timeout.Milliseconds()},
	})
}

func (s *Server) serveCompare(writer http.ResponseWriter, request *http.Request) {
	if err := s.validatePOST(request); err != nil {
		writeAPIError(writer, http.StatusForbidden, err.Error())
		return
	}
	var input CompareRequest
	if err := decodeBody(writer, request, &input); err != nil {
		writeAPIError(writer, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := validateCompareRequest(input); err != nil {
		writeAPIError(writer, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := s.runner.selectedModels(input.Models); err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, ErrModelUnavailable) {
			status = http.StatusConflict
		}
		writeAPIError(writer, status, err.Error())
		return
	}
	if ok, message := s.acquire(); !ok {
		writeAPIError(writer, http.StatusTooManyRequests, message)
		return
	}
	defer func() { <-s.sem }()
	comparison, err := s.runner.Compare(request.Context(), input)
	if err != nil {
		writeAPIError(writer, http.StatusBadRequest, err.Error())
		return
	}
	s.store.Put(comparison)
	writeJSON(writer, http.StatusOK, comparison)
}

func (s *Server) serveSave(writer http.ResponseWriter, request *http.Request) {
	if s.sink == nil {
		http.NotFound(writer, request)
		return
	}
	if err := s.validatePOST(request); err != nil {
		writeAPIError(writer, http.StatusForbidden, err.Error())
		return
	}
	var input SaveRequest
	if err := decodeBody(writer, request, &input); err != nil {
		writeAPIError(writer, http.StatusBadRequest, err.Error())
		return
	}
	if input.ArenaResultID == "" || input.BaselineModel == "" {
		writeAPIError(writer, http.StatusBadRequest, "arena_result_id and baseline_model are required")
		return
	}
	comparison, ok := s.store.Get(input.ArenaResultID)
	if !ok {
		writeAPIError(writer, http.StatusGone, "arena result expired")
		return
	}
	response, err := s.sink.Save(comparison, input)
	if err != nil {
		writeAPIError(writer, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(writer, http.StatusOK, response)
}

func (s *Server) validatePOST(request *http.Request) error {
	if request.Header.Get("Origin") != s.origin {
		return errors.New("invalid origin")
	}
	provided := request.Header.Get("X-LLM-Replay-CSRF")
	if len(provided) != len(s.csrf) || subtle.ConstantTimeCompare([]byte(provided), []byte(s.csrf)) != 1 {
		return errors.New("invalid CSRF token")
	}
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return errors.New("content type must be application/json")
	}
	return nil
}

func (s *Server) acquire() (bool, string) {
	s.mu.Lock()
	now := s.now()
	cutoff := now.Add(-time.Minute)
	kept := s.recent[:0]
	for _, timestamp := range s.recent {
		if timestamp.After(cutoff) {
			kept = append(kept, timestamp)
		}
	}
	s.recent = kept
	if len(s.recent) >= s.limit {
		s.mu.Unlock()
		return false, "arena rate limit exceeded"
	}
	select {
	case s.sem <- struct{}{}:
		s.recent = append(s.recent, now)
		s.mu.Unlock()
		return true, ""
	default:
		s.mu.Unlock()
		return false, "too many active arena comparisons"
	}
}

func (s *Server) serveAsset(writer http.ResponseWriter, name, contentType string) {
	data, err := asset(name)
	if err != nil {
		writeAPIError(writer, http.StatusInternalServerError, "asset unavailable")
		return
	}
	writer.Header().Set("Content-Type", contentType)
	_, _ = writer.Write(data)
}

func decodeBody(writer http.ResponseWriter, request *http.Request, destination any) error {
	request.Body = http.MaxBytesReader(writer, request.Body, MaxRequestBytes)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("invalid JSON body: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain one JSON value")
	}
	return nil
}

func setArenaHeaders(headers http.Header) {
	headers.Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; img-src data:; connect-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
	headers.Set("X-Content-Type-Options", "nosniff")
	headers.Set("Referrer-Policy", "no-referrer")
	headers.Set("X-Frame-Options", "DENY")
	headers.Set("Cache-Control", "no-store")
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func writeAPIError(writer http.ResponseWriter, status int, message string) {
	writeJSON(writer, status, map[string]string{"error": message})
}
