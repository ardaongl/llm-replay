package replay

import (
	"time"

	"github.com/ardao/llm-replay/internal/domain"
	"github.com/ardao/llm-replay/internal/evaluation"
	"github.com/ardao/llm-replay/internal/metrics"
	"github.com/ardao/llm-replay/internal/pricing"
	"github.com/ardao/llm-replay/internal/provider"
)

const (
	defaultConcurrency = 5
	defaultTimeout     = 30 * time.Second
	defaultMaxRetries  = 3
	defaultBackoff     = time.Second
)

type Config struct {
	Concurrency    int
	RequestTimeout time.Duration
	MaxRetries     int
	DisableRetries bool
	BaseBackoff    time.Duration
	OutputDir      string
	DisableJitter  bool
	Pricing        *pricing.Registry
	Evaluators     []evaluation.Evaluator
	OnResult       func(Result)
}

type RunConfig struct {
	ID             string    `json:"id"`
	CreatedAt      time.Time `json:"created_at"`
	DatasetPath    string    `json:"dataset_path"`
	DatasetSHA256  string    `json:"dataset_sha256"`
	Provider       string    `json:"provider"`
	Model          string    `json:"model"`
	Concurrency    int       `json:"concurrency"`
	TimeoutMS      int64     `json:"timeout_ms"`
	MaxRetries     int       `json:"max_retries"`
	BaseBackoffMS  int64     `json:"base_backoff_ms"`
	PricingVersion string    `json:"pricing_version"`
	SchemaVersion  string    `json:"schema_version"`
}

type ResultError struct {
	Type       provider.ErrorType `json:"type"`
	Message    string             `json:"message"`
	StatusCode int                `json:"status_code,omitempty"`
}

type Result struct {
	SchemaVersion      string                     `json:"schema_version"`
	RecordID           string                     `json:"record_id"`
	Provider           string                     `json:"provider"`
	Model              string                     `json:"model"`
	Request            domain.Request             `json:"request"`
	BaselineResponse   domain.Response            `json:"baseline_response"`
	CandidateResponse  *domain.Response           `json:"candidate_response,omitempty"`
	ExpectedEvaluation *domain.ExpectedEvaluation `json:"expected_evaluation,omitempty"`
	Metrics            domain.RecordMetrics       `json:"metrics"`
	Status             domain.Status              `json:"status"`
	Attempts           int                        `json:"attempts"`
	EstimatedCostUSD   *float64                   `json:"estimated_cost_usd,omitempty"`
	Evaluations        []evaluation.Result        `json:"evaluations,omitempty"`
	Error              *ResultError               `json:"error,omitempty"`
}

type EvaluationMetrics struct {
	JSONValidRate       *float64 `json:"json_valid_rate,omitempty"`
	SchemaAdherenceRate *float64 `json:"schema_adherence_rate,omitempty"`
	ExactMatchRate      *float64 `json:"exact_match_rate,omitempty"`
}

type ModelSummary struct {
	metrics.Summary
	PricingAvailable                   bool              `json:"pricing_available"`
	EstimatedCostUSD                   float64           `json:"estimated_cost_usd"`
	AverageCostPerSuccessfulRequestUSD float64           `json:"average_cost_per_successful_request_usd"`
	Evaluations                        EvaluationMetrics `json:"evaluations"`
}

type RunSummary struct {
	SchemaVersion  string                  `json:"schema_version"`
	RunID          string                  `json:"run_id"`
	CreatedAt      time.Time               `json:"created_at"`
	DatasetPath    string                  `json:"dataset_path"`
	DatasetSHA256  string                  `json:"dataset_sha256"`
	TotalRequests  int                     `json:"total_requests"`
	PricingVersion string                  `json:"pricing_version"`
	Models         map[string]ModelSummary `json:"models"`
}

type Outcome struct {
	RunID        string
	RunDir       string
	TotalRecords int
	Succeeded    int
	Failed       int
	TimedOut     int
}
