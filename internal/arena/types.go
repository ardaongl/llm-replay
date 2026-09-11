package arena

import (
	"time"

	"github.com/ardao/llm-replay/internal/domain"
	"github.com/ardao/llm-replay/internal/evaluation"
	"github.com/ardao/llm-replay/internal/provider"
)

const (
	MaxRequestBytes = 256 * 1024
	MaxMessages     = 32
	MaxMessageBytes = 64 * 1024
	MaxPromptBytes  = 128 * 1024
	MaxSchemaBytes  = 64 * 1024
	MaxOutputTokens = 8192
)

type ModelSpec struct {
	ID        string
	Provider  string
	Available bool
	Adapter   provider.Provider
}

type CompareRequest struct {
	Models      []string         `json:"models"`
	Messages    []domain.Message `json:"messages"`
	Temperature *float64         `json:"temperature,omitempty"`
	MaxTokens   *int             `json:"max_tokens,omitempty"`
	Schema      map[string]any   `json:"schema,omitempty"`
}

type SafeResponse struct {
	Content      string `json:"content"`
	FinishReason string `json:"finish_reason,omitempty"`
}

type SafeError struct {
	Type       provider.ErrorType `json:"type"`
	Message    string             `json:"message"`
	StatusCode int                `json:"status_code,omitempty"`
}

type ModelResult struct {
	Model            string               `json:"model"`
	Provider         string               `json:"provider"`
	Status           domain.Status        `json:"status"`
	Response         *SafeResponse        `json:"response,omitempty"`
	Metrics          domain.RecordMetrics `json:"metrics"`
	EstimatedCostUSD *float64             `json:"estimated_cost_usd,omitempty"`
	Evaluations      []evaluation.Result  `json:"evaluations"`
	Error            *SafeError           `json:"error,omitempty"`
}

type Comparison struct {
	ID        string         `json:"arena_result_id"`
	CreatedAt time.Time      `json:"created_at"`
	Results   []ModelResult  `json:"results"`
	Request   domain.Request `json:"-"`
	Schema    map[string]any `json:"-"`
}

type SaveRequest struct {
	ArenaResultID         string `json:"arena_result_id"`
	BaselineModel         string `json:"baseline_model"`
	UseResponseAsExpected bool   `json:"use_response_as_expected"`
	IncludeSchema         bool   `json:"include_schema"`
}

type SaveResponse struct {
	Status             string `json:"status"`
	RecordID           string `json:"record_id"`
	DatasetRecordCount int    `json:"dataset_record_count"`
}
