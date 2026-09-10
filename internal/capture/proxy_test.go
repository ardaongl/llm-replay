package capture

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ardao/llm-replay/internal/dataset"
	"github.com/ardao/llm-replay/internal/domain"
)

func TestProxyForwardsAndCapturesWithoutSecrets(t *testing.T) {
	const apiKey = "sk-ant-abcdefghijklmnopqrstuvwxyz123456"
	const authorization = "Bearer upstream-super-secret-token"
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/chat/completions" {
			t.Errorf("unexpected upstream path: %s", request.URL.Path)
		}
		if request.Header.Get("Authorization") != authorization || request.Header.Get("Cookie") != "session=secret" {
			t.Errorf("upstream headers were changed")
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read upstream request: %v", err)
		}
		if !bytes.Contains(body, []byte(apiKey)) {
			t.Errorf("upstream request body was redacted")
		}
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"choices": []any{map[string]any{
				"message":       map[string]any{"content": "echo " + apiKey},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{"prompt_tokens": 12, "completion_tokens": 4},
		})
	}))
	defer upstream.Close()

	outputPath := filepath.Join(t.TempDir(), "captured.jsonl")
	proxy := mustProxy(t, Config{ListenAddr: "127.0.0.1:0", UpstreamURL: upstream.URL, OutputPath: outputPath})
	server := httptest.NewServer(proxy)

	payload := `{"model":"gpt-test","messages":[{"role":"user","content":"my key is ` + apiKey + `"}],"temperature":0.2}`
	request, err := http.NewRequest(http.MethodPost, server.URL+"/v1/chat/completions", strings.NewReader(payload))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	request.Header.Set("Authorization", authorization)
	request.Header.Set("Cookie", "session=secret")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("proxy request: %v", err)
	}
	responseBody, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("read proxy response: %v", err)
	}
	if response.StatusCode != http.StatusOK || !bytes.Contains(responseBody, []byte(apiKey)) {
		t.Fatalf("upstream response was not preserved: status=%d body=%s", response.StatusCode, responseBody)
	}
	server.Close()
	if err := proxy.Close(); err != nil {
		t.Fatalf("close proxy: %v", err)
	}

	stored, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read captured dataset: %v", err)
	}
	for _, secret := range []string{apiKey, authorization, "session=secret"} {
		if bytes.Contains(stored, []byte(secret)) {
			t.Fatalf("captured dataset contains secret %q: %s", secret, stored)
		}
	}

	var records []domain.Record
	if err := dataset.ReadFile(outputPath, func(record domain.Record) error {
		records = append(records, record)
		return nil
	}); err != nil {
		t.Fatalf("read captured records: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("record count = %d, want 1", len(records))
	}
	record := records[0]
	if record.Model != "gpt-test" || record.Status != domain.StatusSuccess || record.Metrics.InputTokens != 12 || record.Metrics.OutputTokens != 4 {
		t.Fatalf("unexpected captured record: %#v", record)
	}
	if !strings.Contains(record.Request.Messages[0].Content, redacted) || !strings.Contains(record.Response.Content, redacted) {
		t.Fatalf("body secrets were not redacted: %#v", record)
	}
}

func TestProxyCapturesUpstreamHTTPError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusTooManyRequests)
		_, _ = writer.Write([]byte(`{"error":{"message":"rate limited"}}`))
	}))
	defer upstream.Close()

	outputPath := filepath.Join(t.TempDir(), "captured.jsonl")
	proxy := mustProxy(t, Config{ListenAddr: "localhost:0", UpstreamURL: upstream.URL, OutputPath: outputPath})
	server := httptest.NewServer(proxy)
	response, err := http.Post(server.URL+"/v1/chat/completions", "application/json", strings.NewReader(validPayload(false)))
	if err != nil {
		t.Fatalf("proxy request: %v", err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", response.StatusCode)
	}
	server.Close()
	if err := proxy.Close(); err != nil {
		t.Fatalf("close proxy: %v", err)
	}

	var captured domain.Record
	if err := dataset.ReadFile(outputPath, func(record domain.Record) error {
		captured = record
		return nil
	}); err != nil {
		t.Fatalf("read capture: %v", err)
	}
	if captured.Status != domain.StatusError || captured.Response.Content != "rate limited" {
		t.Fatalf("unexpected error capture: %#v", captured)
	}
}

func TestStreamingRequestIsRejectedWithoutForwarding(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		calls.Add(1)
	}))
	defer upstream.Close()

	outputPath := filepath.Join(t.TempDir(), "captured.jsonl")
	proxy := mustProxy(t, Config{ListenAddr: "127.0.0.1:0", UpstreamURL: upstream.URL, OutputPath: outputPath})
	server := httptest.NewServer(proxy)
	response, err := http.Post(server.URL+"/v1/chat/completions", "application/json", strings.NewReader(validPayload(true)))
	if err != nil {
		t.Fatalf("proxy request: %v", err)
	}
	_ = response.Body.Close()
	server.Close()
	if err := proxy.Close(); err != nil {
		t.Fatalf("close proxy: %v", err)
	}
	if response.StatusCode != http.StatusBadRequest || calls.Load() != 0 {
		t.Fatalf("stream request status=%d upstream calls=%d", response.StatusCode, calls.Load())
	}
	info, err := os.Stat(outputPath)
	if err != nil || info.Size() != 0 {
		t.Fatalf("stream request was captured: size=%d error=%v", info.Size(), err)
	}
}

func TestCaptureWriteFailureDoesNotBreakUpstreamResponse(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}],"usage":{}}`))
	}))
	defer upstream.Close()

	errCh := make(chan error, 1)
	proxy := mustProxy(t, Config{
		ListenAddr: "127.0.0.1:0", UpstreamURL: upstream.URL,
		OutputPath:     filepath.Join(t.TempDir(), "captured.jsonl"),
		OnCaptureError: func(err error) { errCh <- err },
	})
	if err := proxy.Close(); err != nil {
		t.Fatalf("close writer before request: %v", err)
	}
	server := httptest.NewServer(proxy)
	defer server.Close()

	response, err := http.Post(server.URL+"/v1/chat/completions", "application/json", strings.NewReader(validPayload(false)))
	if err != nil {
		t.Fatalf("proxy request: %v", err)
	}
	body, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || !bytes.Contains(body, []byte(`"content":"ok"`)) {
		t.Fatalf("capture failure changed response: status=%d body=%s", response.StatusCode, body)
	}
	select {
	case <-errCh:
	default:
		t.Fatal("capture write failure was not reported")
	}
}

func TestValidateListenAddress(t *testing.T) {
	for _, address := range []string{"127.0.0.1:8787", "localhost:8787", "[::1]:8787"} {
		if err := ValidateListenAddress(address); err != nil {
			t.Fatalf("loopback address %q rejected: %v", address, err)
		}
	}
	for _, address := range []string{":8787", "0.0.0.0:8787", "192.168.1.10:8787", "localhost:99999"} {
		if err := ValidateListenAddress(address); err == nil {
			t.Fatalf("unsafe address %q accepted", address)
		}
	}
}

func TestSanitizeHeaders(t *testing.T) {
	headers := http.Header{
		"Authorization": []string{"Bearer secret-token"},
		"Cookie":        []string{"session=secret"},
		"Set-Cookie":    []string{"session=secret"},
		"X-Api-Key":     []string{"secret"},
		"X-Request-Id":  []string{"request-1"},
	}
	sanitized := SanitizeHeaders(headers)
	if sanitized.Get("Authorization") != "" || sanitized.Get("Cookie") != "" || sanitized.Get("X-Api-Key") != "" {
		t.Fatalf("sensitive headers remain: %#v", sanitized)
	}
	if sanitized.Get("X-Request-Id") != "request-1" {
		t.Fatalf("safe header was removed: %#v", sanitized)
	}
}

func validPayload(stream bool) string {
	payload, _ := json.Marshal(map[string]any{
		"model":    "gpt-test",
		"messages": []any{map[string]any{"role": "user", "content": "hello"}},
		"stream":   stream,
	})
	return string(payload)
}

func mustProxy(t *testing.T, config Config) *Proxy {
	t.Helper()
	proxy, err := New(config)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	return proxy
}
