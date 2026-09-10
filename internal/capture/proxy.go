package capture

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/ardao/llm-replay/internal/dataset"
	"github.com/ardao/llm-replay/internal/domain"
)

const maxCaptureBodySize = 16 * 1024 * 1024

type Config struct {
	ListenAddr     string
	UpstreamURL    string
	OutputPath     string
	Transport      http.RoundTripper
	OnCaptureError func(error)
}

type Proxy struct {
	listenAddr     string
	upstream       *url.URL
	reverse        *httputil.ReverseProxy
	writer         *dataset.Writer
	onCaptureError func(error)
	now            func() time.Time
}

type requestState struct {
	startedAt time.Time
	request   domain.Request
	model     string
	id        string
	once      sync.Once
}

type stateKey struct{}

type multiReadCloser struct {
	io.Reader
	io.Closer
}

type openAIRequest struct {
	Model       string           `json:"model"`
	Messages    []domain.Message `json:"messages"`
	Temperature *float64         `json:"temperature,omitempty"`
	TopP        *float64         `json:"top_p,omitempty"`
	MaxTokens   *int             `json:"max_tokens,omitempty"`
	Stream      bool             `json:"stream,omitempty"`
}

type openAIResponse struct {
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
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

func New(config Config) (*Proxy, error) {
	if config.ListenAddr == "" {
		config.ListenAddr = "127.0.0.1:8787"
	}
	if err := ValidateListenAddress(config.ListenAddr); err != nil {
		return nil, err
	}
	if config.UpstreamURL == "" {
		config.UpstreamURL = "https://api.openai.com"
	}
	upstream, err := url.Parse(config.UpstreamURL)
	if err != nil || upstream.Host == "" || (upstream.Scheme != "http" && upstream.Scheme != "https") {
		return nil, fmt.Errorf("invalid capture upstream URL %q", config.UpstreamURL)
	}
	if config.OutputPath == "" {
		return nil, errors.New("capture output path is required")
	}
	writer, err := dataset.NewWriter(config.OutputPath)
	if err != nil {
		return nil, err
	}

	proxy := &Proxy{
		listenAddr:     config.ListenAddr,
		upstream:       upstream,
		writer:         writer,
		onCaptureError: config.OnCaptureError,
		now:            time.Now,
	}
	reverse := httputil.NewSingleHostReverseProxy(upstream)
	originalDirector := reverse.Director
	reverse.Director = func(request *http.Request) {
		originalDirector(request)
		request.Host = upstream.Host
	}
	if config.Transport != nil {
		reverse.Transport = config.Transport
	}
	reverse.ModifyResponse = proxy.modifyResponse
	reverse.ErrorHandler = proxy.handleProxyError
	proxy.reverse = reverse
	return proxy, nil
}

func (p *Proxy) ListenAddr() string {
	return p.listenAddr
}

func (p *Proxy) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if request.URL.Path != "/v1/chat/completions" {
		p.reverse.ServeHTTP(writer, request)
		return
	}
	if request.Method != http.MethodPost {
		http.Error(writer, "capture endpoint requires POST", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(io.LimitReader(request.Body, maxCaptureBodySize+1))
	if err != nil {
		http.Error(writer, "unable to read request body", http.StatusBadRequest)
		return
	}
	_ = request.Body.Close()
	if len(body) > maxCaptureBodySize {
		http.Error(writer, "request body exceeds 16 MiB capture limit", http.StatusRequestEntityTooLarge)
		return
	}
	request.Body = io.NopCloser(bytes.NewReader(body))
	request.ContentLength = int64(len(body))

	var captured openAIRequest
	if err := json.Unmarshal(body, &captured); err != nil {
		http.Error(writer, "request body is not valid OpenAI JSON", http.StatusBadRequest)
		return
	}
	if captured.Stream {
		http.Error(writer, "streaming capture is not supported in this release", http.StatusBadRequest)
		return
	}
	normalized := domain.Request{
		Messages:    redactMessages(captured.Messages),
		Temperature: captured.Temperature,
		TopP:        captured.TopP,
		MaxTokens:   captured.MaxTokens,
	}
	if captured.Model == "" {
		http.Error(writer, "OpenAI request model is required", http.StatusBadRequest)
		return
	}
	if err := normalized.Validate(); err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}

	startedAt := p.now().UTC()
	state := &requestState{
		startedAt: startedAt,
		request:   normalized,
		model:     captured.Model,
		id:        newRecordID(startedAt),
	}
	request = request.WithContext(context.WithValue(request.Context(), stateKey{}, state))
	p.reverse.ServeHTTP(writer, request)
}

func (p *Proxy) modifyResponse(response *http.Response) error {
	state := stateFromContext(response.Request.Context())
	if state == nil {
		return nil
	}

	originalBody := response.Body
	body, err := io.ReadAll(io.LimitReader(originalBody, maxCaptureBodySize+1))
	if err != nil {
		response.Body = &multiReadCloser{Reader: io.MultiReader(bytes.NewReader(body), originalBody), Closer: originalBody}
		p.reportError(fmt.Errorf("read upstream response for capture: %w", err))
		return nil
	}
	if len(body) > maxCaptureBodySize {
		response.Body = &multiReadCloser{Reader: io.MultiReader(bytes.NewReader(body), originalBody), Closer: originalBody}
		p.reportError(errors.New("upstream response exceeds 16 MiB capture limit"))
		return nil
	}
	_ = originalBody.Close()
	response.Body = io.NopCloser(bytes.NewReader(body))
	response.ContentLength = int64(len(body))

	status := domain.StatusSuccess
	capturedResponse := domain.Response{}
	metrics := domain.RecordMetrics{}
	var decoded openAIResponse
	if err := json.Unmarshal(body, &decoded); err != nil || len(decoded.Choices) == 0 {
		status = domain.StatusError
	} else {
		capturedResponse.Content = RedactString(decoded.Choices[0].Message.Content)
		capturedResponse.FinishReason = decoded.Choices[0].FinishReason
		metrics.InputTokens = decoded.Usage.PromptTokens
		metrics.OutputTokens = decoded.Usage.CompletionTokens
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		status = domain.StatusError
		capturedResponse.Content = RedactString(decoded.Error.Message)
	}
	p.capture(state, status, capturedResponse, metrics)
	return nil
}

func (p *Proxy) handleProxyError(writer http.ResponseWriter, request *http.Request, err error) {
	if state := stateFromContext(request.Context()); state != nil {
		p.capture(state, domain.StatusError, domain.Response{Content: "upstream request failed"}, domain.RecordMetrics{})
	}
	p.reportError(fmt.Errorf("capture upstream request: %w", err))
	http.Error(writer, "upstream request failed", http.StatusBadGateway)
}

func (p *Proxy) capture(state *requestState, status domain.Status, response domain.Response, metrics domain.RecordMetrics) {
	state.once.Do(func() {
		metrics.LatencyMs = time.Since(state.startedAt).Milliseconds()
		record := domain.Record{
			SchemaVersion: domain.SchemaVersion,
			ID:            state.id,
			Timestamp:     state.startedAt,
			Provider:      "openai",
			Model:         state.model,
			Request:       state.request,
			Response:      response,
			Metrics:       metrics,
			Status:        status,
		}
		if err := p.writer.Append(record); err != nil {
			p.reportError(err)
		}
	})
}

func (p *Proxy) reportError(err error) {
	if p.onCaptureError != nil {
		p.onCaptureError(err)
	}
}

func (p *Proxy) Close() error {
	return p.writer.Close()
}

func ValidateListenAddress(address string) error {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("capture listen address must be host:port: %w", err)
	}
	if host == "" {
		return errors.New("capture listen host cannot be empty; use 127.0.0.1 or localhost")
	}
	if host != "localhost" {
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			return fmt.Errorf("capture listen host %q is not loopback", host)
		}
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 0 || portNumber > 65535 {
		return fmt.Errorf("capture listen port %q is invalid", port)
	}
	return nil
}

func redactMessages(messages []domain.Message) []domain.Message {
	result := make([]domain.Message, len(messages))
	for index, message := range messages {
		result[index] = message
		result[index].Content = RedactString(message.Content)
	}
	return result
}

func stateFromContext(ctx context.Context) *requestState {
	state, _ := ctx.Value(stateKey{}).(*requestState)
	return state
}

func newRecordID(timestamp time.Time) string {
	random := make([]byte, 6)
	if _, err := rand.Read(random); err != nil {
		return fmt.Sprintf("capture_%d", timestamp.UnixNano())
	}
	return fmt.Sprintf("capture_%d_%s", timestamp.UnixNano(), hex.EncodeToString(random))
}
