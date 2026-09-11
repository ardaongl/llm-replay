package arena

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ardao/llm-replay/internal/domain"
	"github.com/ardao/llm-replay/internal/pricing"
	"github.com/ardao/llm-replay/internal/provider"
)

type fakeProvider struct {
	name     string
	started  chan<- string
	release  <-chan struct{}
	response string
	err      error
}

func (f fakeProvider) Name() string { return f.name }

func (f fakeProvider) Generate(ctx context.Context, _ domain.Request) (*domain.Response, *domain.RecordMetrics, error) {
	if f.started != nil {
		f.started <- f.name
	}
	if f.release != nil {
		select {
		case <-f.release:
		case <-ctx.Done():
			return nil, nil, &provider.Error{Type: provider.ErrTimeout, Message: "timed out", Underlying: ctx.Err()}
		}
	}
	metrics := &domain.RecordMetrics{LatencyMs: 25, InputTokens: 10, OutputTokens: 5}
	if f.err != nil {
		return nil, metrics, f.err
	}
	return &domain.Response{Content: f.response, FinishReason: "stop", Raw: "secret raw payload"}, metrics, nil
}

func TestRunnerStartsBothProvidersInParallelAndKeepsOrder(t *testing.T) {
	started := make(chan string, 2)
	release := make(chan struct{})
	runner := testRunner(t, []ModelSpec{
		{ID: "openai/left", Provider: "openai", Available: true, Adapter: fakeProvider{name: "left", started: started, release: release, response: `{"answer":"left"}`}},
		{ID: "anthropic/right", Provider: "anthropic", Available: true, Adapter: fakeProvider{name: "right", started: started, release: release, response: `{"answer":"right"}`}},
	})

	done := make(chan Comparison, 1)
	go func() {
		comparison, _ := runner.Compare(context.Background(), validCompareRequest())
		done <- comparison
	}()
	for range 2 {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("both providers were not started before either was released")
		}
	}
	close(release)
	comparison := <-done
	if len(comparison.Results) != 2 || comparison.Results[0].Model != "openai/left" || comparison.Results[1].Model != "anthropic/right" {
		t.Fatalf("results were not kept in CLI order: %#v", comparison.Results)
	}
	if comparison.Results[0].Response.Content != `{"answer":"left"}` || comparison.Results[0].EstimatedCostUSD == nil {
		t.Fatalf("unexpected successful result: %#v", comparison.Results[0])
	}
}

func TestRunnerSelectsTwoModelsFromLargerCatalog(t *testing.T) {
	runner := testRunner(t, []ModelSpec{
		{ID: "openai/left", Provider: "openai", Available: true, Adapter: fakeProvider{name: "left", response: "left"}},
		{ID: "anthropic/right", Provider: "anthropic", Available: true, Adapter: fakeProvider{name: "right", response: "right"}},
		{ID: "openai/third", Provider: "openai", Available: true, Adapter: fakeProvider{name: "third", response: "third"}},
	})
	input := validCompareRequest()
	input.Models = []string{"openai/third", "openai/left"}
	comparison, err := runner.Compare(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if comparison.Results[0].Model != "openai/third" || comparison.Results[0].Response.Content != "third" || comparison.Results[1].Model != "openai/left" {
		t.Fatalf("selected results are out of order: %#v", comparison.Results)
	}
}

func TestRunnerRejectsUnsafeModelSelections(t *testing.T) {
	runner := testRunner(t, []ModelSpec{
		{ID: "openai/left", Provider: "openai", Available: true, Adapter: fakeProvider{name: "left"}},
		{ID: "anthropic/right", Provider: "anthropic", Available: false},
	})
	for _, models := range [][]string{{"openai/left", "openai/left"}, {"openai/left", "unknown/model"}, {"openai/left", "anthropic/right"}} {
		input := validCompareRequest()
		input.Models = models
		if _, err := runner.Compare(context.Background(), input); err == nil {
			t.Fatalf("unsafe model selection was accepted: %v", models)
		}
	}
}

func TestRunnerReturnsPartialFailureWithoutRawProviderData(t *testing.T) {
	providerErr := &provider.Error{Type: provider.ErrRateLimit, StatusCode: 429, Message: "limit reached", Underlying: errors.New("private body")}
	runner := testRunner(t, []ModelSpec{
		{ID: "openai/left", Provider: "openai", Available: true, Adapter: fakeProvider{name: "left", response: "plain text"}},
		{ID: "anthropic/right", Provider: "anthropic", Available: true, Adapter: fakeProvider{name: "right", err: providerErr}},
	})
	comparison, err := runner.Compare(context.Background(), validCompareRequest())
	if err != nil {
		t.Fatal(err)
	}
	if comparison.Results[0].Status != domain.StatusSuccess || comparison.Results[0].Response.Content != "plain text" {
		t.Fatalf("unexpected first result: %#v", comparison.Results[0])
	}
	failed := comparison.Results[1]
	if failed.Status != domain.StatusError || failed.Error == nil || failed.Error.Type != provider.ErrRateLimit || failed.Error.Message != "limit reached" {
		t.Fatalf("unexpected safe error: %#v", failed)
	}
}

func TestRunnerOnlyRunsSchemaEvaluatorWhenSchemaIsProvided(t *testing.T) {
	runner := testRunner(t, []ModelSpec{
		{ID: "openai/left", Provider: "openai", Available: true, Adapter: fakeProvider{name: "left", response: `{"ok":true}`}},
		{ID: "anthropic/right", Provider: "anthropic", Available: true, Adapter: fakeProvider{name: "right", response: `{"ok":true}`}},
	})
	input := validCompareRequest()
	input.Schema = nil
	comparison, err := runner.Compare(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	for _, result := range comparison.Results {
		if len(result.Evaluations) != 1 || result.Evaluations[0].Name != "json_valid" || !result.Evaluations[0].Passed {
			t.Fatalf("unexpected evaluator set: %#v", result.Evaluations)
		}
	}
}

func TestRunnerTimeoutCancelsBothProviders(t *testing.T) {
	release := make(chan struct{})
	models := []ModelSpec{
		{ID: "openai/left", Provider: "openai", Available: true, Adapter: fakeProvider{name: "left", release: release}},
		{ID: "anthropic/right", Provider: "anthropic", Available: true, Adapter: fakeProvider{name: "right", release: release}},
	}
	runner, err := NewRunner(models, pricing.Default(), 10*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	comparison, err := runner.Compare(context.Background(), validCompareRequest())
	if err != nil {
		t.Fatal(err)
	}
	for _, result := range comparison.Results {
		if result.Status != domain.StatusTimeout || result.Error == nil || result.Error.Type != provider.ErrTimeout {
			t.Fatalf("provider did not return a normalized timeout: %#v", result)
		}
	}
}

func TestCompareRequestValidation(t *testing.T) {
	temperature := 3.0
	tooManyTokens := MaxOutputTokens + 1
	tests := []CompareRequest{
		{},
		{Messages: []domain.Message{{Role: "tool", Content: "invalid"}}},
		{Messages: []domain.Message{{Role: "user", Content: "hi"}}, Temperature: &temperature},
		{Messages: []domain.Message{{Role: "user", Content: "hi"}}, MaxTokens: &tooManyTokens},
		{Messages: []domain.Message{{Role: "user", Content: "hi"}}, Schema: map[string]any{"$ref": "https://example.com/schema"}},
	}
	for index, input := range tests {
		if _, err := validateCompareRequest(input); err == nil {
			t.Fatalf("case %d was accepted", index)
		}
	}
}

func TestRunnerRejectsInvalidCatalog(t *testing.T) {
	model := ModelSpec{ID: "openai/left", Provider: "openai", Available: true, Adapter: fakeProvider{name: "left"}}
	if _, err := NewRunner([]ModelSpec{model}, pricing.Default(), time.Second); err == nil {
		t.Fatal("single-model catalog was accepted")
	}
	if _, err := NewRunner([]ModelSpec{model, model}, pricing.Default(), time.Second); err == nil {
		t.Fatal("duplicate catalog model was accepted")
	}
}

func testRunner(t *testing.T, models []ModelSpec) *Runner {
	t.Helper()
	registry := pricing.Registry{Version: "test", Models: map[string]pricing.Rate{"openai/left": {InputPerMillion: 1, OutputPerMillion: 2}}}
	runner, err := NewRunner(models, registry, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return runner
}

func validCompareRequest() CompareRequest {
	return CompareRequest{Models: []string{"openai/left", "anthropic/right"}, Messages: []domain.Message{{Role: "user", Content: "hello"}}, Schema: map[string]any{"type": "object"}}
}
