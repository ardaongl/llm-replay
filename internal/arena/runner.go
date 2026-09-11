package arena

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ardao/llm-replay/internal/domain"
	"github.com/ardao/llm-replay/internal/evaluation"
	"github.com/ardao/llm-replay/internal/pricing"
	"github.com/ardao/llm-replay/internal/provider"
)

var ErrModelUnavailable = errors.New("provider credentials are unavailable")

type Runner struct {
	models  []ModelSpec
	byID    map[string]ModelSpec
	pricing pricing.Registry
	timeout time.Duration
}

func NewRunner(models []ModelSpec, registry pricing.Registry, timeout time.Duration) (*Runner, error) {
	if len(models) < 2 {
		return nil, errors.New("arena catalog requires at least two models")
	}
	if timeout <= 0 || timeout > time.Minute {
		return nil, errors.New("arena timeout must be between 1ns and 60s")
	}
	byID := make(map[string]ModelSpec, len(models))
	for _, model := range models {
		if strings.TrimSpace(model.ID) == "" || strings.TrimSpace(model.Provider) == "" {
			return nil, errors.New("arena model id and provider are required")
		}
		if _, duplicate := byID[model.ID]; duplicate {
			return nil, fmt.Errorf("arena model %q appears more than once", model.ID)
		}
		if model.Available && model.Adapter == nil {
			return nil, fmt.Errorf("arena model %q is available but has no provider adapter", model.ID)
		}
		byID[model.ID] = model
	}
	return &Runner{models: append([]ModelSpec(nil), models...), byID: byID, pricing: registry, timeout: timeout}, nil
}

func (r *Runner) Models() []ModelSpec {
	return append([]ModelSpec(nil), r.models...)
}

func (r *Runner) AvailableCount() int {
	count := 0
	for _, model := range r.models {
		if model.Available {
			count++
		}
	}
	return count
}

func (r *Runner) selectedModels(ids []string) ([]ModelSpec, error) {
	if len(ids) != 2 || ids[0] == ids[1] {
		return nil, errors.New("select exactly two different models")
	}
	selected := make([]ModelSpec, 0, 2)
	for _, id := range ids {
		model, allowed := r.byID[id]
		if !allowed {
			return nil, errors.New("selected model is not in the arena allowlist")
		}
		if !model.Available {
			return nil, fmt.Errorf("%w for model %q", ErrModelUnavailable, id)
		}
		selected = append(selected, model)
	}
	return selected, nil
}

func (r *Runner) Compare(ctx context.Context, input CompareRequest) (Comparison, error) {
	request, err := validateCompareRequest(input)
	if err != nil {
		return Comparison{}, err
	}
	models, err := r.selectedModels(input.Models)
	if err != nil {
		return Comparison{}, err
	}

	type indexedResult struct {
		index  int
		result ModelResult
	}
	results := make(chan indexedResult, len(models))
	for index, model := range models {
		go func() {
			callCtx, cancel := context.WithTimeout(ctx, r.timeout)
			defer cancel()
			results <- indexedResult{index: index, result: r.runModel(callCtx, model, request, input.Schema)}
		}()
	}
	ordered := make([]ModelResult, len(models))
	for range models {
		result := <-results
		ordered[result.index] = result.result
	}
	id, err := randomID("ar_")
	if err != nil {
		return Comparison{}, err
	}
	return Comparison{ID: id, CreatedAt: time.Now().UTC(), Results: ordered, Request: request, Schema: input.Schema}, nil
}

func (r *Runner) runModel(ctx context.Context, model ModelSpec, request domain.Request, schema map[string]any) ModelResult {
	response, metrics, err := model.Adapter.Generate(ctx, request)
	result := ModelResult{Model: model.ID, Provider: model.Provider, Status: domain.StatusSuccess, Evaluations: []evaluation.Result{}}
	if metrics != nil {
		result.Metrics = *metrics
	}
	if err != nil {
		result.Status = domain.StatusError
		if provider.TypeOf(err) == provider.ErrTimeout {
			result.Status = domain.StatusTimeout
		}
		result.Error = safeProviderError(err)
		return result
	}
	result.Response = &SafeResponse{Content: response.Content, FinishReason: response.FinishReason}
	if cost, costErr := r.pricing.Cost(model.ID, int64(result.Metrics.InputTokens), int64(result.Metrics.OutputTokens)); costErr == nil {
		result.EstimatedCostUSD = &cost
	}
	record := &domain.Record{Request: request}
	if len(schema) > 0 {
		record.Evaluation = &domain.ExpectedEvaluation{Schema: schema}
	}
	safeResponse := &domain.Response{Content: response.Content, FinishReason: response.FinishReason}
	evaluators := []evaluation.Evaluator{evaluation.JSONValidity{}}
	if len(schema) > 0 {
		evaluators = append(evaluators, evaluation.SchemaAdherence{})
	}
	for _, evaluator := range evaluators {
		result.Evaluations = append(result.Evaluations, evaluator.Evaluate(ctx, record, safeResponse))
	}
	return result
}

func validateCompareRequest(input CompareRequest) (domain.Request, error) {
	if len(input.Messages) == 0 || len(input.Messages) > MaxMessages {
		return domain.Request{}, fmt.Errorf("messages must contain between 1 and %d items", MaxMessages)
	}
	total := 0
	for index, message := range input.Messages {
		if !utf8.ValidString(message.Content) {
			return domain.Request{}, fmt.Errorf("messages[%d].content must be valid UTF-8", index)
		}
		if len(message.Content) > MaxMessageBytes {
			return domain.Request{}, fmt.Errorf("messages[%d].content exceeds 64 KiB", index)
		}
		total += len(message.Content)
	}
	if total > MaxPromptBytes {
		return domain.Request{}, errors.New("combined message content exceeds 128 KiB")
	}
	if input.Temperature != nil && (*input.Temperature < 0 || *input.Temperature > 2) {
		return domain.Request{}, errors.New("temperature must be between 0 and 2")
	}
	if input.MaxTokens != nil && (*input.MaxTokens < 1 || *input.MaxTokens > MaxOutputTokens) {
		return domain.Request{}, fmt.Errorf("max_tokens must be between 1 and %d", MaxOutputTokens)
	}
	if len(input.Schema) > 0 {
		encoded, err := json.Marshal(input.Schema)
		if err != nil || len(encoded) > MaxSchemaBytes {
			return domain.Request{}, errors.New("schema must be valid JSON no larger than 64 KiB")
		}
		if _, err := evaluation.CompileSchema(input.Schema); err != nil {
			return domain.Request{}, fmt.Errorf("invalid schema: %w", err)
		}
	}
	request := domain.Request{Messages: input.Messages, Temperature: input.Temperature, MaxTokens: input.MaxTokens}
	if err := request.Validate(); err != nil {
		return domain.Request{}, err
	}
	return request, nil
}

func safeProviderError(err error) *SafeError {
	result := &SafeError{Type: provider.TypeOf(err), Message: "provider request failed"}
	var providerErr *provider.Error
	if errors.As(err, &providerErr) {
		result.StatusCode = providerErr.StatusCode
		message := strings.TrimSpace(providerErr.Message)
		if len(message) > 300 {
			message = message[:300]
		}
		if message != "" {
			result.Message = message
		}
	}
	return result
}
