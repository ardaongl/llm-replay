package anthropic

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
	defaultBaseURL      = "https://api.anthropic.com"
	defaultAPIVersion   = "2023-06-01"
	defaultMaxTokens    = 1024
	defaultTimeout      = 30 * time.Second
	maxResponseBodySize = 16 * 1024 * 1024
)

type Config struct {
	APIKey     string
	Model      string
	BaseURL    string
	APIVersion string
	Timeout    time.Duration
	HTTPClient *http.Client
}

type Adapter struct {
	apiKey     string
	model      string
	baseURL    string
	apiVersion string
	client     *http.Client
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type messagesRequest struct {
	Model       string    `json:"model"`
	System      string    `json:"system,omitempty"`
	Messages    []message `json:"messages"`
	MaxTokens   int       `json:"max_tokens"`
	Temperature *float64  `json:"temperature,omitempty"`
	TopP        *float64  `json:"top_p,omitempty"`
}

type messagesResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

type errorResponse struct {
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

func New(config Config) (*Adapter, error) {
	if strings.TrimSpace(config.APIKey) == "" {
		return nil, errors.New("ANTHROPIC_API_KEY is required")
	}
	if strings.TrimSpace(config.Model) == "" {
		return nil, errors.New("Anthropic model is required")
	}
	if config.BaseURL == "" {
		config.BaseURL = defaultBaseURL
	}
	parsedURL, err := url.Parse(config.BaseURL)
	if err != nil || parsedURL.Host == "" || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		return nil, fmt.Errorf("invalid Anthropic base URL %q", config.BaseURL)
	}
	if config.APIVersion == "" {
		config.APIVersion = defaultAPIVersion
	}
	if config.HTTPClient == nil {
		if config.Timeout <= 0 {
			config.Timeout = defaultTimeout
		}
		config.HTTPClient = &http.Client{Timeout: config.Timeout}
	}

	return &Adapter{
		apiKey:     config.APIKey,
		model:      config.Model,
		baseURL:    strings.TrimRight(config.BaseURL, "/"),
		apiVersion: config.APIVersion,
		client:     config.HTTPClient,
	}, nil
}

func (a *Adapter) Name() string {
	return "anthropic"
}

func (a *Adapter) Generate(ctx context.Context, request domain.Request) (*domain.Response, *domain.RecordMetrics, error) {
	payloadRequest, err := translateRequest(a.model, request)
	if err != nil {
		return nil, nil, err
	}
	payload, err := json.Marshal(payloadRequest)
	if err != nil {
		return nil, nil, &provider.Error{Type: provider.ErrInvalidRequest, Message: "encode Anthropic request", Underlying: err}
	}

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/v1/messages", bytes.NewReader(payload))
	if err != nil {
		return nil, nil, &provider.Error{Type: provider.ErrInvalidRequest, Message: "create Anthropic request", Underlying: err}
	}
	httpRequest.Header.Set("x-api-key", a.apiKey)
	httpRequest.Header.Set("anthropic-version", a.apiVersion)
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", "application/json")

	startedAt := time.Now()
	httpResponse, err := a.client.Do(httpRequest)
	metrics := &domain.RecordMetrics{LatencyMs: time.Since(startedAt).Milliseconds()}
	if err != nil {
		return nil, metrics, normalizeTransportError(ctx, err)
	}
	defer httpResponse.Body.Close()

	body, err := io.ReadAll(io.LimitReader(httpResponse.Body, maxResponseBodySize+1))
	if err != nil {
		return nil, metrics, &provider.Error{Type: provider.ErrNetwork, Message: "read Anthropic response", Underlying: err}
	}
	if len(body) > maxResponseBodySize {
		return nil, metrics, &provider.Error{Type: provider.ErrParse, StatusCode: httpResponse.StatusCode, Message: "Anthropic response exceeds 16 MiB"}
	}
	if httpResponse.StatusCode < http.StatusOK || httpResponse.StatusCode >= http.StatusMultipleChoices {
		return nil, metrics, normalizeHTTPError(httpResponse, body)
	}

	var decoded messagesResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, metrics, &provider.Error{Type: provider.ErrParse, StatusCode: httpResponse.StatusCode, Message: "decode Anthropic response", Underlying: err}
	}
	var textBlocks []string
	for _, block := range decoded.Content {
		if block.Type == "text" {
			textBlocks = append(textBlocks, block.Text)
		}
	}
	if len(textBlocks) == 0 {
		return nil, metrics, &provider.Error{Type: provider.ErrParse, StatusCode: httpResponse.StatusCode, Message: "Anthropic response contains no text content"}
	}

	metrics.InputTokens = decoded.Usage.InputTokens
	metrics.OutputTokens = decoded.Usage.OutputTokens
	return &domain.Response{
		Content:      strings.Join(textBlocks, ""),
		FinishReason: decoded.StopReason,
		Raw:          string(body),
	}, metrics, nil
}

func translateRequest(model string, request domain.Request) (messagesRequest, error) {
	if err := request.Validate(); err != nil {
		return messagesRequest{}, &provider.Error{Type: provider.ErrInvalidRequest, Message: err.Error(), Underlying: err}
	}

	var systemParts []string
	messages := make([]message, 0, len(request.Messages))
	for _, item := range request.Messages {
		if item.Role == "system" {
			systemParts = append(systemParts, item.Content)
			continue
		}
		messages = append(messages, message{Role: item.Role, Content: item.Content})
	}
	if len(messages) == 0 {
		return messagesRequest{}, &provider.Error{Type: provider.ErrInvalidRequest, Message: "Anthropic request requires at least one user or assistant message"}
	}

	maxTokens := defaultMaxTokens
	if request.MaxTokens != nil {
		maxTokens = *request.MaxTokens
	}
	return messagesRequest{
		Model:       model,
		System:      strings.Join(systemParts, "\n\n"),
		Messages:    messages,
		MaxTokens:   maxTokens,
		Temperature: request.Temperature,
		TopP:        request.TopP,
	}, nil
}

func normalizeTransportError(ctx context.Context, err error) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return &provider.Error{Type: provider.ErrTimeout, Message: "Anthropic request timed out", Underlying: err}
	}
	var networkErr net.Error
	if errors.As(err, &networkErr) {
		if networkErr.Timeout() {
			return &provider.Error{Type: provider.ErrTimeout, Message: "Anthropic request timed out", Underlying: err}
		}
		return &provider.Error{Type: provider.ErrNetwork, Message: "Anthropic network request failed", Underlying: err}
	}
	return &provider.Error{Type: provider.ErrNetwork, Message: "Anthropic request failed", Underlying: err}
}

func normalizeHTTPError(response *http.Response, body []byte) error {
	message := http.StatusText(response.StatusCode)
	apiType := ""
	var decoded errorResponse
	if json.Unmarshal(body, &decoded) == nil {
		apiType = decoded.Error.Type
		if decoded.Error.Message != "" {
			message = decoded.Error.Message
		}
	}

	errorType := provider.ErrUnknown
	switch apiType {
	case "authentication_error", "permission_error":
		errorType = provider.ErrAuthentication
	case "rate_limit_error":
		errorType = provider.ErrRateLimit
	case "invalid_request_error", "request_too_large":
		errorType = provider.ErrInvalidRequest
	case "overloaded_error", "api_error", "timeout_error":
		errorType = provider.ErrProviderDown
	}
	if errorType == provider.ErrUnknown {
		switch response.StatusCode {
		case http.StatusBadRequest, http.StatusRequestEntityTooLarge:
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
