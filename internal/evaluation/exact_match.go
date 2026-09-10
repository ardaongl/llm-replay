package evaluation

import (
	"context"
	"strings"

	"github.com/ardao/llm-replay/internal/domain"
)

type ExactMatch struct {
	TrimSpace       bool
	CaseInsensitive bool
}

func (ExactMatch) Name() string {
	return ExactMatchName
}

func (e ExactMatch) Evaluate(ctx context.Context, record *domain.Record, candidate *domain.Response) Result {
	if err := ctx.Err(); err != nil {
		return Result{Name: e.Name(), Error: err.Error()}
	}
	if record == nil || record.Evaluation == nil || record.Evaluation.Expected == "" {
		return skipped(e.Name(), "expected value is not configured")
	}
	if candidate == nil {
		return skipped(e.Name(), "candidate response is unavailable")
	}

	want := record.Evaluation.Expected
	got := candidate.Content
	if e.TrimSpace {
		want = strings.TrimSpace(want)
		got = strings.TrimSpace(got)
	}
	if e.CaseInsensitive {
		want = strings.ToLower(want)
		got = strings.ToLower(got)
	}
	passed := got == want
	result := Result{Name: e.Name(), Passed: passed}
	if passed {
		result.Score = 1
	} else {
		result.Details = map[string]any{"expected": want, "actual": got}
	}
	return result
}
