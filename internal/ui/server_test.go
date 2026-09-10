package ui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ardao/llm-replay/internal/artifact"
	"github.com/ardao/llm-replay/internal/domain"
	"github.com/ardao/llm-replay/internal/evaluation"
	"github.com/ardao/llm-replay/internal/metrics"
	"github.com/ardao/llm-replay/internal/replay"
)

func TestServerEndpointsAndFilters(t *testing.T) {
	server := testServer(t)

	health := serve(t, server, "/api/v1/health", "127.0.0.1:8080")
	if health.Code != http.StatusOK || health.Header().Get("Content-Security-Policy") == "" {
		t.Fatalf("health response = %d headers=%v", health.Code, health.Header())
	}

	response := serve(t, server, "/api/v1/results?status=error&evaluator=schema&passed=false&min_latency_ms=2000&limit=1", "localhost:8080")
	if response.Code != http.StatusOK {
		t.Fatalf("results response = %d: %s", response.Code, response.Body.String())
	}
	var page artifact.Page
	if err := json.NewDecoder(response.Body).Decode(&page); err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(page.Records) != 1 || page.Records[0].ID != "record-2" {
		t.Fatalf("unexpected filtered page: %#v", page)
	}
	reportResponse := serve(t, server, "/api/v1/report", "localhost:8080")
	var reportData artifact.ReportData
	if err := json.NewDecoder(reportResponse.Body).Decode(&reportData); err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, value := range reportData.Latency.Series[0].Counts {
		count += value
	}
	if count != 1 || reportData.Latency.MaxMS != 100 {
		t.Fatalf("unexpected full latency histogram: %#v", reportData.Latency)
	}

	detail := serve(t, server, "/api/v1/results/record-1", "[::1]:8080")
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), "record-1") {
		t.Fatalf("detail response = %d: %s", detail.Code, detail.Body.String())
	}
}

func TestServerRejectsUnsafeRequests(t *testing.T) {
	server := testServer(t)
	if response := serve(t, server, "/api/v1/report", "attacker.example"); response.Code != http.StatusBadRequest {
		t.Fatalf("bad host status = %d", response.Code)
	}
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/v1/report", nil)
	request.Host = "127.0.0.1"
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status = %d", response.Code)
	}
	if response := serve(t, server, "/api/v1/results/..%2Fsecret", "127.0.0.1"); response.Code != http.StatusBadRequest {
		t.Fatalf("path traversal status = %d", response.Code)
	}
	if err := ValidateHost("0.0.0.0"); err == nil {
		t.Fatal("non-loopback host accepted")
	}
	for _, path := range []string{"/api/v1/results?evaluator=unknown", "/api/v1/results?model=unknown"} {
		if response := serve(t, server, path, "127.0.0.1"); response.Code != http.StatusBadRequest {
			t.Fatalf("invalid query %q status = %d", path, response.Code)
		}
	}
}

func TestBrowserCommandsDoNotUseShell(t *testing.T) {
	tests := map[string]string{"windows": "rundll32", "darwin": "open", "linux": "xdg-open"}
	for goos, expected := range tests {
		name, arguments, err := browserCommand(goos, "http://127.0.0.1:8080/")
		if err != nil || name != expected || arguments[len(arguments)-1] != "http://127.0.0.1:8080/" {
			t.Fatalf("browserCommand(%s) = %q, %v, %v", goos, name, arguments, err)
		}
	}
}

func testServer(t *testing.T) *Server {
	t.Helper()
	comparison := &artifact.Comparison{
		DatasetSHA256: "hash",
		Runs:          []artifact.Run{{Name: "openai/model", Config: replay.RunConfig{Model: "openai/model"}, Summary: replay.ModelSummary{Summary: metrics.Summary{TotalRequests: 2}}}},
		Records: []artifact.Record{
			testRecord("record-1", domain.StatusSuccess, 100, true),
			testRecord("record-2", domain.StatusError, 2500, false),
		},
	}
	page, err := RenderPage(nil, false)
	if err != nil {
		t.Fatal(err)
	}
	return &Server{comparison: comparison, page: page}
}

func testRecord(id string, status domain.Status, latency int64, passed bool) artifact.Record {
	result := replay.Result{SchemaVersion: domain.SchemaVersion, RecordID: id, Provider: "openai", Model: "openai/model", Request: domain.Request{Messages: []domain.Message{{Role: "user", Content: id}}}, BaselineResponse: domain.Response{Content: "baseline"}, CandidateResponse: &domain.Response{Content: "candidate"}, Status: status, Metrics: domain.RecordMetrics{LatencyMs: latency}, Evaluations: []evaluation.Result{{Name: evaluation.SchemaAdherenceName, Passed: passed}}}
	return artifact.Record{ID: id, Request: result.Request, Baseline: result.BaselineResponse, Models: []artifact.ModelResult{{Name: "openai/model", Result: result}}}
}

func serve(t *testing.T, server *Server, path, host string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1"+path, nil)
	request.Host = host
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	return response
}
