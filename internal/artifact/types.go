package artifact

import (
	"github.com/ardao/llm-replay/internal/domain"
	"github.com/ardao/llm-replay/internal/replay"
)

type Run struct {
	Dir     string
	Name    string
	Config  replay.RunConfig
	Summary replay.ModelSummary
	Results map[string]replay.Result
}

type Comparison struct {
	DatasetSHA256 string
	Runs          []Run
	Records       []Record
}

type Record struct {
	ID       string
	Request  domain.Request
	Baseline domain.Response
	Models   []ModelResult
}

type ModelResult struct {
	Name   string
	Result replay.Result
}

type ReportData struct {
	DatasetSHA256 string       `json:"dataset_sha256"`
	TotalRecords  int          `json:"total_records"`
	Included      int          `json:"included_records"`
	Truncated     bool         `json:"truncated"`
	Runs          []RunView    `json:"runs"`
	Latency       LatencyChart `json:"latency"`
	Records       []RecordView `json:"records,omitempty"`
}

type LatencyChart struct {
	MaxMS  int64           `json:"max_ms"`
	Bins   int             `json:"bins"`
	Series []LatencySeries `json:"series"`
}

type LatencySeries struct {
	Name         string `json:"name"`
	Counts       []int  `json:"counts"`
	P50MS        int64  `json:"p50_ms"`
	P90MS        int64  `json:"p90_ms"`
	P95MS        int64  `json:"p95_ms"`
	P99MS        int64  `json:"p99_ms"`
	TimeoutCount int    `json:"timeout_count"`
}

type RunView struct {
	Name    string              `json:"name"`
	Config  replay.RunConfig    `json:"config"`
	Summary replay.ModelSummary `json:"summary"`
}

type RecordView struct {
	ID       string            `json:"id"`
	Request  domain.Request    `json:"request"`
	Baseline domain.Response   `json:"baseline"`
	Models   []ModelResultView `json:"models"`
}

type ModelResultView struct {
	Name        string               `json:"name"`
	Status      domain.Status        `json:"status"`
	Candidate   *domain.Response     `json:"candidate,omitempty"`
	Metrics     domain.RecordMetrics `json:"metrics"`
	Evaluations []EvaluationView     `json:"evaluations"`
	Error       *replay.ResultError  `json:"error,omitempty"`
}

type EvaluationView struct {
	Name    string         `json:"name"`
	Passed  bool           `json:"passed"`
	Skipped bool           `json:"skipped,omitempty"`
	Score   float64        `json:"score,omitempty"`
	Details map[string]any `json:"details,omitempty"`
	Error   string         `json:"error,omitempty"`
}

type Query struct {
	Status       string
	Evaluator    string
	Passed       *bool
	MinLatencyMS int64
	Model        string
	Search       string
}

type Page struct {
	Records    []RecordView `json:"records"`
	Total      int          `json:"total"`
	NextCursor string       `json:"next_cursor,omitempty"`
}
