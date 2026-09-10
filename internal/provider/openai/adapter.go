package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ardao/llm-replay/internal/domain"
	"github.com/ardao/llm-replay/internal/provider"
)

const (
	defaultBaseURL      = "https://api.openai.com"
	defaultTimeout      = 30 * time.Second
	maxResponseBodySize = 16 * 1024 * 1024
)

type Config struct {
	APIKey     string
	Model      string
	BaseURL    string
	Timeout    time.Duration
	HTTPClient *http.Client
}

type Adapter struct {
	apiKey  string
	model   string
	baseURL string
	client  *http.Client
}

func New(config Config) (*Adapter, error) {
	if strings.TrimSpace(config.APIKey) == "" {
		return nil, errors.New("OPENAI_API_KEY is required")
	}
	if strings.TrimSpace(config.Model) == "" {
		return nil, errors.New("OpenAI model is required")
	}
	if config.BaseURL == "" {
		config.BaseURL = defaultBaseURL
	}
	parsedURL, err := url.Parse(config.BaseURL)
	if err != nil || parsedURL.Host == "" || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		return nil, fmt.Errorf("invalid OpenAI base URL %q", config.BaseURL)
	}
	if config.HTTPClient == nil {
		if config.Timeout <= 0 {
			config.Timeout = defaultTimeout
		}
		config.HTTPClient = &http.Client{Timeout: config.Timeout}
	}

	return &Adapter{
		apiKey:  config.APIKey,
		model:   config.Model,
		baseURL: strings.TrimRight(config.BaseURL, "/"),
		client:  config.HTTPClient,
	}, nil
}

func (a *Adapter) Name() string {
	return "openai"
}

type chatRequest struct {
	Model       string           `json:"model"`
	Messages    []domain.Message `json:"messages"`
	Temperature *float64         `json:"temperature,omitempty"`
	TopP        *float64         `json:"top_p,omitempty"`
	MaxTokens   *int             `json:"max_tokens,omitempty"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

type errorResponse struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    any    `json:"code"`
	} `json:"error"`
}

func (a *Adapter) Generate(ctx context.Context, request domain.Request) (*domain.Response, *domain.RecordMetrics, error) {
	if err := request.Validate(); err != nil {
		return nil, nil, &provider.Error{Type: provider.ErrInvalidRequest, Message: err.Error(), Underlying: err}
	}
	payload, err := json.Marshal(chatRequest{
		Model:       a.model,
		Messages:    request.Messages,
		Temperature: request.Temperature,
		TopP:        request.TopP,
		MaxTokens:   request.MaxTokens,
	})
	if err != nil {
		return nil, nil, &provider.Error{Type: provider.ErrInvalidRequest, Message: "encode OpenAI request", Underlying: err}
	}

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/v1/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return nil, nil, &provider.Error{Type: provider.ErrInvalidRequest, Message: "create OpenAI request", Underlying: err}
	}
	httpRequest.Header.Set("Authorization", "Bearer "+a.apiKey)
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", "application/json")

	startedAt := time.Now()
	httpResponse, err := a.client.Do(httpRequest)
	latency := time.Since(startedAt).Milliseconds()
	metrics := &domain.RecordMetrics{LatencyMs: latency}
	if err != nil {
		return nil, metrics, normalizeTransportError(ctx, err)
	}
	defer httpResponse.Body.Close()

	body, err := io.ReadAll(io.LimitReader(httpResponse.Body, maxResponseBodySize+1))
	if err != nil {
		return nil, metrics, &provider.Error{Type: provider.ErrNetwork, Message: "read OpenAI response", Underlying: err}
	}
	if len(body) > maxResponseBodySize {
		return nil, metrics, &provider.Error{Type: provider.ErrParse, StatusCode: httpResponse.StatusCode, Message: "OpenAI response exceeds 16 MiB"}
	}
	if httpResponse.StatusCode < http.StatusOK || httpResponse.StatusCode >= http.StatusMultipleChoices {
		return nil, metrics, normalizeHTTPError(httpResponse, body)
	}

	var decoded chatResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, metrics, &provider.Error{Type: provider.ErrParse, StatusCode: httpResponse.StatusCode, Message: "decode OpenAI response", Underlying: err}
	}
	if len(decoded.Choices) == 0 {
		return nil, metrics, &provider.Error{Type: provider.ErrParse, StatusCode: httpResponse.StatusCode, Message: "OpenAI response contains no choices"}
	}

	metrics.InputTokens = decoded.Usage.PromptTokens
	metrics.OutputTokens = decoded.Usage.CompletionTokens
	response := &domain.Response{
		Content:      decoded.Choices[0].Message.Content,
		FinishReason: decoded.Choices[0].FinishReason,
		Raw:          string(body),
	}
	return response, metrics, nil
}

func normalizeTransportError(ctx context.Context, err error) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return &provider.Error{Type: provider.ErrTimeout, Message: "OpenAI request timed out", Underlying: err}
	}
	var networkErr net.Error
	if errors.As(err, &networkErr) {
		if networkErr.Timeout() {
			return &provider.Error{Type: provider.ErrTimeout, Message: "OpenAI request timed out", Underlying: err}
		}
		return &provider.Error{Type: provider.ErrNetwork, Message: "OpenAI network request failed", Underlying: err}
	}
	return &provider.Error{Type: provider.ErrNetwork, Message: "OpenAI request failed", Underlying: err}
}

func normalizeHTTPError(response *http.Response, body []byte) error {
	errorType := provider.ErrUnknown
	switch response.StatusCode {
	case http.StatusBadRequest:
		errorType = provider.ErrInvalidRequest
	case http.StatusUnauthorized, http.StatusForbidden:
		errorType = provider.ErrAuthentication
	case http.StatusTooManyRequests:
		errorType = provider.ErrRateLimit
	default:
		if response.StatusCode >= http.StatusInternalServerError {
			errorType = provider.ErrProviderDown
		}
	}

	message := http.StatusText(response.StatusCode)
	var decoded errorResponse
	if json.Unmarshal(body, &decoded) == nil && decoded.Error.Message != "" {
		message = decoded.Error.Message
	}
	return &provider.Error{
		Type:       errorType,
		StatusCode: response.StatusCode,
		Message:    message,
		RetryAfter: parseRetryAfter(response.Header.Get("Retry-After"), time.Now()),
	}
}

func parseRetryAfter(value string, now time.Time) time.Duration {
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	when, err := http.ParseTime(value)
	if err != nil || !when.After(now) {
		return 0
	}
	return when.Sub(now)
}
