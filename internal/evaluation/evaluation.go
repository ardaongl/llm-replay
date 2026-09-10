package evaluation

import (
	"context"

	"github.com/ardao/llm-replay/internal/domain"
)

const (
	JSONValidName       = "json_valid"
	SchemaAdherenceName = "schema_adherence"
	ExactMatchName      = "exact_match"
)

type Result struct {
	Name    string         `json:"name"`
	Passed  bool           `json:"passed"`
	Skipped bool           `json:"skipped,omitempty"`
	Score   float64        `json:"score,omitempty"`
	Details map[string]any `json:"details,omitempty"`
	Error   string         `json:"error,omitempty"`
}

type Evaluator interface {
	Name() string
	Evaluate(ctx context.Context, record *domain.Record, candidateResponse *domain.Response) Result
}

func Defaults() []Evaluator {
	return []Evaluator{
		JSONValidity{},
		SchemaAdherence{},
		ExactMatch{TrimSpace: true},
	}
}

type Aggregator struct {
	results map[string]aggregate
}

type aggregate struct {
	evaluated int
	passed    int
}

func NewAggregator() *Aggregator {
	return &Aggregator{results: make(map[string]aggregate)}
}

func (a *Aggregator) Add(results []Result) {
	for _, result := range results {
		if result.Skipped {
			continue
		}
		value := a.results[result.Name]
		value.evaluated++
		if result.Passed {
			value.passed++
		}
		a.results[result.Name] = value
	}
}

func (a *Aggregator) Rate(name string) (float64, bool) {
	value, ok := a.results[name]
	if !ok || value.evaluated == 0 {
		return 0, false
	}
	return float64(value.passed) / float64(value.evaluated), true
}

func skipped(name, reason string) Result {
	return Result{Name: name, Skipped: true, Details: map[string]any{"reason": reason}}
}
