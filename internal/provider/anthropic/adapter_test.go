package anthropic

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

func TestGenerateTranslatesRequestAndParsesResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/v1/messages" {
			t.Errorf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("x-api-key") != "test-key" {
			t.Errorf("x-api-key header missing")
		}
		if request.Header.Get("anthropic-version") != defaultAPIVersion {
			t.Errorf("anthropic-version = %q", request.Header.Get("anthropic-version"))
		}
		if request.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type = %q", request.Header.Get("Content-Type"))
		}

		var payload messagesRequest
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if payload.Model != "claude-test" || payload.System != "First rule.\n\nSecond rule." {
			t.Errorf("unexpected model/system: %#v", payload)
		}
		if len(payload.Messages) != 2 || payload.Messages[0].Role != "user" || payload.Messages[1].Role != "assistant" {
			t.Errorf("system messages leaked into messages: %#v", payload.Messages)
		}
		if payload.MaxTokens != 128 || payload.Temperature == nil || *payload.Temperature != 0.2 {
			t.Errorf("request options not translated: %#v", payload)
		}

		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{
            "content":[{"type":"text","text":"Hello"},{"type":"text","text":"!"}],
            "stop_reason":"end_turn",
            "usage":{"input_tokens":12,"output_tokens":3}
        }`))
	}))
	defer server.Close()

	adapter := mustAdapter(t, Config{APIKey: "test-key", Model: "claude-test", BaseURL: server.URL})
	response, metrics, err := adapter.Generate(context.Background(), validRequest())
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if adapter.Name() != "anthropic" || response.Content != "Hello!" || response.FinishReason != "end_turn" || response.Raw == "" {
		t.Fatalf("unexpected response: %#v", response)
	}
	if metrics.InputTokens != 12 || metrics.OutputTokens != 3 || metrics.LatencyMs < 0 {
		t.Fatalf("unexpected metrics: %#v", metrics)
	}
}

func TestTranslateRequestUsesDefaultMaxTokens(t *testing.T) {
	payload, err := translateRequest("claude-test", domain.Request{Messages: []domain.Message{{Role: "user", Content: "hello"}}})
	if err != nil {
		t.Fatalf("translateRequest() error: %v", err)
	}
	if payload.MaxTokens != defaultMaxTokens {
		t.Fatalf("max_tokens = %d, want %d", payload.MaxTokens, defaultMaxTokens)
	}
}

func TestTranslateRequestRejectsSystemOnlyConversation(t *testing.T) {
	_, err := translateRequest("claude-test", domain.Request{Messages: []domain.Message{{Role: "system", Content: "rules"}}})
	if provider.TypeOf(err) != provider.ErrInvalidRequest {
		t.Fatalf("error = %v, want invalid request", err)
	}
}

func TestNewValidatesConfiguration(t *testing.T) {
	tests := []struct {
		name   string
		config Config
		want   string
	}{
		{name: "missing key", config: Config{Model: "claude-test"}, want: "ANTHROPIC_API_KEY"},
		{name: "missing model", config: Config{APIKey: "test-key"}, want: "model"},
		{name: "invalid URL", config: Config{APIKey: "test-key", Model: "claude-test", BaseURL: "://bad"}, want: "base URL"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(tt.config)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("New() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestGenerateNormalizesAnthropicErrors(t *testing.T) {
	tests := []struct {
		apiType   string
		status    int
		wantType  provider.ErrorType
		retryable bool
	}{
		{apiType: "authentication_error", status: http.StatusUnauthorized, wantType: provider.ErrAuthentication},
		{apiType: "rate_limit_error", status: http.StatusTooManyRequests, wantType: provider.ErrRateLimit, retryable: true},
		{apiType: "invalid_request_error", status: http.StatusBadRequest, wantType: provider.ErrInvalidRequest},
		{apiType: "overloaded_error", status: 529, wantType: provider.ErrProviderDown, retryable: true},
	}
	for _, tt := range tests {
		t.Run(tt.apiType, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Retry-After", "2")
				writer.WriteHeader(tt.status)
				_, _ = writer.Write([]byte(`{"type":"error","error":{"type":"` + tt.apiType + `","message":"provider test error"}}`))
			}))
			defer server.Close()

			adapter := mustAdapter(t, Config{APIKey: "test-key", Model: "claude-test", BaseURL: server.URL})
			response, metrics, err := adapter.Generate(context.Background(), validRequest())
			if response != nil || metrics == nil || provider.TypeOf(err) != tt.wantType {
				t.Fatalf("Generate() = (%#v, %#v, %v), want %q", response, metrics, err, tt.wantType)
			}
			if provider.IsRetryable(err) != tt.retryable {
				t.Fatalf("retryable = %v, want %v", provider.IsRetryable(err), tt.retryable)
			}
			var providerErr *provider.Error
			if !errors.As(err, &providerErr) || providerErr.Message != "provider test error" || providerErr.StatusCode != tt.status {
				t.Fatalf("unexpected provider error: %#v", providerErr)
			}
			if tt.apiType == "rate_limit_error" && providerErr.RetryAfter != 2*time.Second {
				t.Fatalf("RetryAfter = %s, want 2s", providerErr.RetryAfter)
			}
		})
	}
}

func TestGenerateTimeoutAndMalformedResponse(t *testing.T) {
	t.Run("timeout", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			time.Sleep(100 * time.Millisecond)
			writer.WriteHeader(http.StatusOK)
		}))
		defer server.Close()
		adapter := mustAdapter(t, Config{APIKey: "test-key", Model: "claude-test", BaseURL: server.URL})
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		_, metrics, err := adapter.Generate(ctx, validRequest())
		if metrics == nil || provider.TypeOf(err) != provider.ErrTimeout {
			t.Fatalf("metrics = %#v, error = %v", metrics, err)
		}
	})

	t.Run("no text", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			_, _ = writer.Write([]byte(`{"content":[{"type":"tool_use"}],"stop_reason":"tool_use"}`))
		}))
		defer server.Close()
		adapter := mustAdapter(t, Config{APIKey: "test-key", Model: "claude-test", BaseURL: server.URL})
		_, _, err := adapter.Generate(context.Background(), validRequest())
		if provider.TypeOf(err) != provider.ErrParse {
			t.Fatalf("error = %v, want parse error", err)
		}
	})
}

func validRequest() domain.Request {
	temperature := 0.2
	topP := 0.9
	maxTokens := 128
	return domain.Request{
		Messages: []domain.Message{
			{Role: "system", Content: "First rule."},
			{Role: "user", Content: "Hello"},
			{Role: "system", Content: "Second rule."},
			{Role: "assistant", Content: "Hi"},
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
