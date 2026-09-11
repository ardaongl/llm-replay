package arena

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ardao/llm-replay/internal/domain"
)

func TestArenaServerCompareSecurityAndSafeResponse(t *testing.T) {
	server := testArenaServer(t, true, nil)

	page := serveArena(t, server, http.MethodGet, "/", nil, false)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), server.csrf) || page.Header().Get("Content-Security-Policy") == "" {
		t.Fatalf("unexpected page response: %d %s", page.Code, page.Body.String())
	}
	badOrigin := serveArena(t, server, http.MethodPost, "/api/v1/arena/compare", validBody(t), false)
	if badOrigin.Code != http.StatusForbidden {
		t.Fatalf("bad origin status = %d", badOrigin.Code)
	}
	response := serveArena(t, server, http.MethodPost, "/api/v1/arena/compare", validBody(t), true)
	if response.Code != http.StatusOK {
		t.Fatalf("compare status = %d: %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "secret raw payload") {
		t.Fatal("raw provider response leaked through arena API")
	}
	var comparison Comparison
	if err := json.NewDecoder(response.Body).Decode(&comparison); err != nil {
		t.Fatal(err)
	}
	if len(comparison.Results) != 2 || comparison.ID == "" {
		t.Fatalf("unexpected comparison: %#v", comparison)
	}
}

func TestArenaServerRejectsUnavailableAndOversizedOrUnknownBodies(t *testing.T) {
	unavailable := testArenaServer(t, false, nil)
	if response := serveArena(t, unavailable, http.MethodPost, "/api/v1/arena/compare", validBody(t), true); response.Code != http.StatusConflict {
		t.Fatalf("unavailable status = %d", response.Code)
	}

	server := testArenaServer(t, true, nil)
	unknownModel := []byte(`{"models":["openai/left","attacker/arbitrary"],"messages":[{"role":"user","content":"hi"}]}`)
	if response := serveArena(t, server, http.MethodPost, "/api/v1/arena/compare", unknownModel, true); response.Code != http.StatusBadRequest {
		t.Fatalf("unknown model status = %d", response.Code)
	}
	unknown := []byte(`{"messages":[{"role":"user","content":"hi"}],"unexpected":true}`)
	if response := serveArena(t, server, http.MethodPost, "/api/v1/arena/compare", unknown, true); response.Code != http.StatusBadRequest {
		t.Fatalf("unknown field status = %d", response.Code)
	}
	oversized := bytes.Repeat([]byte("x"), MaxRequestBytes+1)
	if response := serveArena(t, server, http.MethodPost, "/api/v1/arena/compare", oversized, true); response.Code != http.StatusBadRequest {
		t.Fatalf("oversized body status = %d", response.Code)
	}
}

func TestArenaServerRejectsHostContentTypeAndTrailingJSON(t *testing.T) {
	server := testArenaServer(t, true, nil)
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8787/api/v1/arena/config", nil)
	request.Host = "attacker.example"
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("bad host status = %d", response.Code)
	}

	request = httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8787/api/v1/arena/compare", bytes.NewReader(validBody(t)))
	request.Host = "127.0.0.1:8787"
	request.Header.Set("Origin", "http://127.0.0.1:8787")
	request.Header.Set("X-LLM-Replay-CSRF", server.csrf)
	request.Header.Set("Content-Type", "text/plain")
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("bad content type status = %d", response.Code)
	}

	trailing := append(validBody(t), []byte(` {}`)...)
	if response := serveArena(t, server, http.MethodPost, "/api/v1/arena/compare", trailing, true); response.Code != http.StatusBadRequest {
		t.Fatalf("trailing JSON status = %d", response.Code)
	}
}

func TestArenaServerRateAndConcurrencyLimits(t *testing.T) {
	rateLimited := testArenaServerWithConfig(t, ServerConfig{RequestsPerMinute: 1}, nil)
	if response := serveArena(t, rateLimited, http.MethodPost, "/api/v1/arena/compare", validBody(t), true); response.Code != http.StatusOK {
		t.Fatalf("first compare status = %d", response.Code)
	}
	if response := serveArena(t, rateLimited, http.MethodPost, "/api/v1/arena/compare", validBody(t), true); response.Code != http.StatusTooManyRequests {
		t.Fatalf("rate-limited status = %d", response.Code)
	}

	started := make(chan string, 2)
	release := make(chan struct{})
	models := []ModelSpec{
		{ID: "openai/left", Provider: "openai", Available: true, Adapter: fakeProvider{name: "left", started: started, release: release, response: "left"}},
		{ID: "anthropic/right", Provider: "anthropic", Available: true, Adapter: fakeProvider{name: "right", started: started, release: release, response: "right"}},
	}
	runner := testRunner(t, models)
	server, err := NewServer(ServerConfig{Origin: "http://127.0.0.1:8787", Runner: runner, MaxConcurrent: 1})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- serveArena(t, server, http.MethodPost, "/api/v1/arena/compare", validBody(t), true) }()
	for range 2 {
		<-started
	}
	if response := serveArena(t, server, http.MethodPost, "/api/v1/arena/compare", validBody(t), true); response.Code != http.StatusTooManyRequests {
		t.Fatalf("concurrency-limited status = %d", response.Code)
	}
	close(release)
	if response := <-done; response.Code != http.StatusOK {
		t.Fatalf("blocked compare status = %d", response.Code)
	}
}

func TestArenaSaveRouteIsHiddenWithoutDataset(t *testing.T) {
	server := testArenaServer(t, true, nil)
	response := serveArena(t, server, http.MethodPost, "/api/v1/arena/save", []byte(`{}`), true)
	if response.Code != http.StatusNotFound {
		t.Fatalf("save status = %d", response.Code)
	}
}

func TestArenaSaveUsesStoredResultAndRejectsClientInjection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "arena.jsonl")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	sink, err := OpenDatasetSink(path)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.Close()
	server := testArenaServer(t, true, sink)
	compare := serveArena(t, server, http.MethodPost, "/api/v1/arena/compare", validBody(t), true)
	var comparison Comparison
	if err := json.NewDecoder(compare.Body).Decode(&comparison); err != nil {
		t.Fatal(err)
	}
	saveBody, _ := json.Marshal(SaveRequest{ArenaResultID: comparison.ID, BaselineModel: "openai/left", UseResponseAsExpected: true})
	saved := serveArena(t, server, http.MethodPost, "/api/v1/arena/save", saveBody, true)
	if saved.Code != http.StatusOK {
		t.Fatalf("save status = %d: %s", saved.Code, saved.Body.String())
	}
	injected := []byte(`{"arena_result_id":"` + comparison.ID + `","baseline_model":"openai/left","path":"other.jsonl","response":"forged"}`)
	if response := serveArena(t, server, http.MethodPost, "/api/v1/arena/save", injected, true); response.Code != http.StatusBadRequest {
		t.Fatalf("injected save status = %d", response.Code)
	}
}

func testArenaServer(t *testing.T, available bool, sink *DatasetSink) *Server {
	t.Helper()
	models := []ModelSpec{
		{ID: "openai/left", Provider: "openai", Available: available, Adapter: fakeProvider{name: "left", response: "left"}},
		{ID: "anthropic/right", Provider: "anthropic", Available: available, Adapter: fakeProvider{name: "right", response: "right"}},
	}
	if !available {
		models[0].Adapter, models[1].Adapter = nil, nil
	}
	return testArenaServerWithConfig(t, ServerConfig{}, modelsWithSink{models: models, sink: sink})
}

type modelsWithSink struct {
	models []ModelSpec
	sink   *DatasetSink
}

func testArenaServerWithConfig(t *testing.T, config ServerConfig, custom any) *Server {
	t.Helper()
	models := []ModelSpec{
		{ID: "openai/left", Provider: "openai", Available: true, Adapter: fakeProvider{name: "left", response: "left"}},
		{ID: "anthropic/right", Provider: "anthropic", Available: true, Adapter: fakeProvider{name: "right", response: "right"}},
	}
	var sink *DatasetSink
	if value, ok := custom.(modelsWithSink); ok {
		models, sink = value.models, value.sink
	}
	runner := testRunner(t, models)
	config.Origin = "http://127.0.0.1:8787"
	config.Runner = runner
	config.Sink = sink
	server, err := NewServer(config)
	if err != nil {
		t.Fatal(err)
	}
	return server
}

func serveArena(t *testing.T, server *Server, method, path string, body []byte, authorized bool) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, "http://127.0.0.1:8787"+path, bytes.NewReader(body))
	request.Host = "127.0.0.1:8787"
	if method == http.MethodPost {
		request.Header.Set("Content-Type", "application/json")
		if authorized {
			request.Header.Set("Origin", "http://127.0.0.1:8787")
			request.Header.Set("X-LLM-Replay-CSRF", server.csrf)
		}
	}
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	return response
}

func validBody(t *testing.T) []byte {
	t.Helper()
	data, err := json.Marshal(CompareRequest{Models: []string{"openai/left", "anthropic/right"}, Messages: []domain.Message{{Role: "user", Content: "hello"}}})
	if err != nil {
		t.Fatal(err)
	}
	return data
}
