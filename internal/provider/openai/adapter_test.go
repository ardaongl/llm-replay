package openai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ardao/llm-replay/internal/domain"
	"github.com/ardao/llm-replay/internal/provider"
)

var _ provider.Provider = (*Adapter)(nil)

func TestGenerateSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/v1/chat/completions" {
			t.Errorf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
		if got := request.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q", got)
		}
		if got := request.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q", got)
		}

		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if payload["model"] != "gpt-test" {
			t.Errorf("model = %v", payload["model"])
		}
		if payload["temperature"] != 0.2 || payload["top_p"] != 0.9 || payload["max_tokens"] != float64(128) {
			t.Errorf("optional parameters were not forwarded: %#v", payload)
		}
		if _, exists := payload["metadata"]; exists {
			t.Error("internal metadata leaked into provider request")
		}

		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{
            "choices":[{"message":{"content":"Hello!"},"finish_reason":"stop"}],
            "usage":{"prompt_tokens":12,"completion_tokens":3}
        }`))
	}))
	defer server.Close()

	adapter := mustAdapter(t, Config{APIKey: "test-key", Model: "gpt-test", BaseURL: server.URL})
	response, metrics, err := adapter.Generate(context.Background(), validRequest())
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if adapter.Name() != "openai" {
		t.Fatalf("Name() = %q", adapter.Name())
	}
	if response.Content != "Hello!" || response.FinishReason != "stop" || response.Raw == "" {
		t.Fatalf("unexpected response: %#v", response)
	}
	if metrics.InputTokens != 12 || metrics.OutputTokens != 3 || metrics.LatencyMs < 0 {
		t.Fatalf("unexpected metrics: %#v", metrics)
	}
}

func TestNewValidatesConfiguration(t *testing.T) {
	tests := []struct {
		name   string
		config Config
		want   string
	}{
		{name: "missing key", config: Config{Model: "gpt-test"}, want: "OPENAI_API_KEY"},
		{name: "missing model", config: Config{APIKey: "test-key"}, want: "model"},
		{name: "invalid URL", config: Config{APIKey: "test-key", Model: "gpt-test", BaseURL: "://bad"}, want: "base URL"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(tt.config)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("New() error = %v, want error containing %q", err, tt.want)
			}
		})
	}
}

func TestGenerateNormalizesHTTPError(t *testing.T) {
	tests := []struct {
		status    int
		wantType  provider.ErrorType
		retryable bool
	}{
		{http.StatusBadRequest, provider.ErrInvalidRequest, false},
		{http.StatusUnauthorized, provider.ErrAuthentication, false},
		{http.StatusForbidden, provider.ErrAuthentication, false},
		{http.StatusTooManyRequests, provider.ErrRateLimit, true},
		{http.StatusInternalServerError, provider.ErrProviderDown, true},
	}

	for _, tt := range tests {
		t.Run(http.StatusText(tt.status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Retry-After", "2")
				writer.WriteHeader(tt.status)
				_, _ = writer.Write([]byte(`{"error":{"message":"provider test error","type":"test"}}`))
			}))
			defer server.Close()

			adapter := mustAdapter(t, Config{APIKey: "test-key", Model: "gpt-test", BaseURL: server.URL})
			response, metrics, err := adapter.Generate(context.Background(), validRequest())
			if response != nil || metrics == nil || err == nil {
				t.Fatalf("Generate() = (%#v, %#v, %v), want nil response, metrics, error", response, metrics, err)
			}
			if got := provider.TypeOf(err); got != tt.wantType {
				t.Fatalf("error type = %q, want %q", got, tt.wantType)
			}
			if got := provider.IsRetryable(err); got != tt.retryable {
				t.Fatalf("retryable = %v, want %v", got, tt.retryable)
			}
			var providerErr *provider.Error
			if !errors.As(err, &providerErr) {
				t.Fatalf("error is not provider.Error: %T", err)
			}
			if providerErr.StatusCode != tt.status || providerErr.Message != "provider test error" {
				t.Fatalf("unexpected provider error: %#v", providerErr)
			}
			if tt.status == http.StatusTooManyRequests && providerErr.RetryAfter != 2*time.Second {
				t.Fatalf("RetryAfter = %s, want 2s", providerErr.RetryAfter)
			}
		})
	}
}

func TestGenerateTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	adapter := mustAdapter(t, Config{APIKey: "test-key", Model: "gpt-test", BaseURL: server.URL})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	response, metrics, err := adapter.Generate(ctx, validRequest())
	if response != nil || metrics == nil {
		t.Fatalf("Generate() = (%#v, %#v, %v)", response, metrics, err)
	}
	if got := provider.TypeOf(err); got != provider.ErrTimeout {
		t.Fatalf("error type = %q, want %q", got, provider.ErrTimeout)
	}
}

func TestGenerateNetworkError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	baseURL := server.URL
	server.Close()

	adapter := mustAdapter(t, Config{APIKey: "test-key", Model: "gpt-test", BaseURL: baseURL})
	_, metrics, err := adapter.Generate(context.Background(), validRequest())
	if metrics == nil || provider.TypeOf(err) != provider.ErrNetwork {
		t.Fatalf("Generate() metrics = %#v, error = %v", metrics, err)
	}
}

func TestGenerateRejectsInvalidRequestBeforeHTTP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("HTTP server should not be called for an invalid request")
	}))
	defer server.Close()

	adapter := mustAdapter(t, Config{APIKey: "test-key", Model: "gpt-test", BaseURL: server.URL})
	_, _, err := adapter.Generate(context.Background(), domain.Request{})
	if got := provider.TypeOf(err); got != provider.ErrInvalidRequest {
		t.Fatalf("error type = %q, want %q", got, provider.ErrInvalidRequest)
	}
}

func TestGenerateRejectsMalformedSuccessResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write([]byte(`{"choices":[]}`))
	}))
	defer server.Close()

	adapter := mustAdapter(t, Config{APIKey: "test-key", Model: "gpt-test", BaseURL: server.URL})
	_, _, err := adapter.Generate(context.Background(), validRequest())
	if got := provider.TypeOf(err); got != provider.ErrParse {
		t.Fatalf("error type = %q, want %q", got, provider.ErrParse)
	}
}

func TestParseRetryAfterHTTPDate(t *testing.T) {
	now := time.Date(2026, time.September, 10, 12, 0, 0, 0, time.UTC)
	value := now.Add(5 * time.Second).Format(http.TimeFormat)
	if got := parseRetryAfter(value, now); got != 5*time.Second {
		t.Fatalf("parseRetryAfter() = %s, want 5s", got)
	}
}

func validRequest() domain.Request {
	temperature := 0.2
	topP := 0.9
	maxTokens := 128
	return domain.Request{
		Messages: []domain.Message{
			{Role: "system", Content: "Answer briefly."},
			{Role: "user", Content: "Hello"},
		},
		Temperature: &temperature,
		TopP:        &topP,
		MaxTokens:   &maxTokens,
		Metadata:    map[string]any{"internal": "must-not-leak"},
	}
}

func mustAdapter(t *testing.T, config Config) *Adapter {
	t.Helper()
	adapter, err := New(config)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	return adapter
}
