package ui

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/ardao/llm-replay/internal/artifact"
)

type Server struct {
	comparison *artifact.Comparison
	page       []byte
}

func NewServer(runDirs []string) (*Server, error) {
	comparison, err := artifact.LoadComparison(runDirs)
	if err != nil {
		return nil, err
	}
	page, err := RenderPage(nil, false)
	if err != nil {
		return nil, err
	}
	return &Server{comparison: comparison, page: page}, nil
}

func (s *Server) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	setSecurityHeaders(writer.Header())
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		writer.Header().Set("Allow", "GET, HEAD")
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !validRequestHost(request.Host) {
		http.Error(writer, "invalid host", http.StatusBadRequest)
		return
	}

	switch {
	case request.URL.Path == "/":
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		writer.Header().Set("Cache-Control", "no-store")
		_, _ = writer.Write(s.page)
	case request.URL.Path == "/assets/app.css":
		s.serveAsset(writer, "app.css", "text/css; charset=utf-8")
	case request.URL.Path == "/assets/app.js":
		s.serveAsset(writer, "app.js", "text/javascript; charset=utf-8")
	case request.URL.Path == "/api/v1/health":
		writeJSON(writer, http.StatusOK, map[string]string{"status": "ok"})
	case request.URL.Path == "/api/v1/report":
		data := s.comparison.ReportData(s.comparison.Records, len(s.comparison.Records))
		data.Records = nil
		data.Truncated = false
		writeJSON(writer, http.StatusOK, data)
	case request.URL.Path == "/api/v1/results":
		s.serveResults(writer, request)
	case strings.HasPrefix(request.URL.Path, "/api/v1/results/"):
		s.serveResult(writer, request)
	default:
		http.NotFound(writer, request)
	}
}

func (s *Server) serveAsset(writer http.ResponseWriter, name, contentType string) {
	asset, err := Asset(name)
	if err != nil {
		http.Error(writer, "asset unavailable", http.StatusInternalServerError)
		return
	}
	writer.Header().Set("Content-Type", contentType)
	writer.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = writer.Write(asset)
}

func (s *Server) serveResults(writer http.ResponseWriter, request *http.Request) {
	query, limit, cursor, err := parseQuery(request.URL.Query())
	if err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}
	if !s.comparison.HasModel(query.Model) {
		http.Error(writer, fmt.Sprintf("unknown model %q", query.Model), http.StatusBadRequest)
		return
	}
	page, err := s.comparison.Page(query, cursor, limit)
	if err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(writer, http.StatusOK, page)
}

func (s *Server) serveResult(writer http.ResponseWriter, request *http.Request) {
	rawID := strings.TrimPrefix(request.URL.Path, "/api/v1/results/")
	if rawID == "" || strings.Contains(rawID, "/") {
		http.Error(writer, "invalid record id", http.StatusBadRequest)
		return
	}
	id, err := url.PathUnescape(rawID)
	if err != nil || id == "" || strings.ContainsAny(id, `/\\`) {
		http.Error(writer, "invalid record id", http.StatusBadRequest)
		return
	}
	record, ok := s.comparison.Record(id)
	if !ok {
		http.NotFound(writer, request)
		return
	}
	writeJSON(writer, http.StatusOK, record)
}

func parseQuery(values url.Values) (artifact.Query, int, string, error) {
	query := artifact.Query{Status: values.Get("status"), Evaluator: values.Get("evaluator"), Model: values.Get("model"), Search: values.Get("q")}
	if !artifact.ValidStatus(query.Status) {
		return artifact.Query{}, 0, "", fmt.Errorf("invalid status %q", query.Status)
	}
	if !artifact.ValidEvaluator(query.Evaluator) {
		return artifact.Query{}, 0, "", fmt.Errorf("invalid evaluator %q", query.Evaluator)
	}
	if raw := values.Get("passed"); raw != "" {
		passed, err := strconv.ParseBool(raw)
		if err != nil {
			return artifact.Query{}, 0, "", errors.New("passed must be true or false")
		}
		query.Passed = &passed
	}
	if raw := values.Get("min_latency_ms"); raw != "" {
		latency, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || latency < 0 {
			return artifact.Query{}, 0, "", errors.New("min_latency_ms must be a non-negative integer")
		}
		query.MinLatencyMS = latency
	}
	limit := 50
	if raw := values.Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			return artifact.Query{}, 0, "", errors.New("limit must be an integer")
		}
		limit = parsed
	}
	return query, limit, values.Get("cursor"), nil
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-store")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func setSecurityHeaders(headers http.Header) {
	headers.Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; base-uri 'none'; frame-ancestors 'none'")
	headers.Set("X-Content-Type-Options", "nosniff")
	headers.Set("Referrer-Policy", "no-referrer")
	headers.Set("X-Frame-Options", "DENY")
}

func validRequestHost(value string) bool {
	host := value
	if parsedHost, _, err := net.SplitHostPort(value); err == nil {
		host = parsedHost
	}
	host = strings.Trim(host, "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func ValidateHost(host string) error {
	if host == "" || !validRequestHost(host) {
		return fmt.Errorf("UI host %q is not loopback; use 127.0.0.1 or localhost", host)
	}
	return nil
}
