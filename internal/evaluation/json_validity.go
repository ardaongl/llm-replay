package evaluation

import (
	"context"
	"encoding/json"

	"github.com/ardao/llm-replay/internal/domain"
)

type JSONValidity struct{}

func (JSONValidity) Name() string {
	return JSONValidName
}

func (e JSONValidity) Evaluate(ctx context.Context, _ *domain.Record, candidate *domain.Response) Result {
	if err := ctx.Err(); err != nil {
		return Result{Name: e.Name(), Error: err.Error()}
	}
	if candidate == nil {
		return skipped(e.Name(), "candidate response is unavailable")
	}
	if json.Valid([]byte(candidate.Content)) {
		return Result{Name: e.Name(), Passed: true, Score: 1}
	}
	return Result{
		Name:    e.Name(),
		Passed:  false,
		Details: map[string]any{"reason": "candidate content is not valid JSON"},
	}
}
