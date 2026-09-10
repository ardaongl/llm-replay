package replay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ardao/llm-replay/internal/dataset"
	"github.com/ardao/llm-replay/internal/domain"
	"github.com/ardao/llm-replay/internal/evaluation"
	"github.com/ardao/llm-replay/internal/metrics"
	"github.com/ardao/llm-replay/internal/pricing"
	"github.com/ardao/llm-replay/internal/provider"
)

type Engine struct {
	provider provider.Provider
	model    string
	config   Config
	now      func() time.Time
	sleep    func(context.Context, time.Duration) error
}

func New(providerAdapter provider.Provider, model string, config Config) (*Engine, error) {
	if providerAdapter == nil {
		return nil, errors.New("replay provider is required")
	}
	if model == "" {
		return nil, errors.New("replay model is required")
	}
	if config.Concurrency == 0 {
		config.Concurrency = defaultConcurrency
	}
	if config.Concurrency < 0 {
		return nil, errors.New("replay concurrency must be greater than zero")
	}
	if config.RequestTimeout == 0 {
		config.RequestTimeout = defaultTimeout
	}
	if config.RequestTimeout < 0 {
		return nil, errors.New("replay timeout must be greater than zero")
	}
	if config.MaxRetries == 0 && !config.DisableRetries {
		config.MaxRetries = defaultMaxRetries
	}
	if config.MaxRetries < 0 {
		return nil, errors.New("replay max retries cannot be negative")
	}
	if config.BaseBackoff == 0 {
		config.BaseBackoff = defaultBackoff
	}
	if config.BaseBackoff < 0 {
		return nil, errors.New("replay backoff cannot be negative")
	}
	if config.OutputDir == "" {
		config.OutputDir = "runs"
	}
	if config.Pricing == nil {
		defaultPricing := pricing.Default()
		config.Pricing = &defaultPricing
	} else if err := config.Pricing.Validate(); err != nil {
		return nil, fmt.Errorf("invalid pricing registry: %w", err)
	}
	if config.Evaluators == nil {
		config.Evaluators = evaluation.Defaults()
	}

	return &Engine{
		provider: providerAdapter,
		model:    model,
		config:   config,
		now:      time.Now,
		sleep:    sleepContext,
	}, nil
}

func (e *Engine) Run(ctx context.Context, datasetPath string) (Outcome, error) {
	if datasetPath == "" {
		return Outcome{}, errors.New("dataset path is required")
	}
	absDatasetPath, err := filepath.Abs(datasetPath)
	if err != nil {
		return Outcome{}, fmt.Errorf("resolve dataset path: %w", err)
	}
	if err := dataset.ReadFile(absDatasetPath, func(domain.Record) error { return nil }); err != nil {
		return Outcome{}, fmt.Errorf("validate dataset: %w", err)
	}
	datasetHash, err := dataset.HashFile(absDatasetPath)
	if err != nil {
		return Outcome{}, err
	}

	createdAt := e.now().UTC()
	baseRunID := "run_" + createdAt.Format("20060102_150405") + fmt.Sprintf("_%09d", createdAt.Nanosecond())
	if err := os.MkdirAll(e.config.OutputDir, 0o750); err != nil {
		return Outcome{}, fmt.Errorf("create replay output directory: %w", err)
	}
	var runID, runDir string
	for suffix := 0; ; suffix++ {
		runID = baseRunID
		if suffix > 0 {
			runID = fmt.Sprintf("%s_%d", baseRunID, suffix)
		}
		runDir = filepath.Join(e.config.OutputDir, runID)
		err := os.Mkdir(runDir, 0o750)
		if err == nil {
			break
		}
		if errors.Is(err, os.ErrExist) {
			continue
		}
		return Outcome{}, fmt.Errorf("create replay run directory: %w", err)
	}

	runConfig := RunConfig{
		ID:             runID,
		CreatedAt:      createdAt,
		DatasetPath:    absDatasetPath,
		DatasetSHA256:  datasetHash,
		Provider:       e.provider.Name(),
		Model:          e.model,
		Concurrency:    e.config.Concurrency,
		TimeoutMS:      e.config.RequestTimeout.Milliseconds(),
		MaxRetries:     e.config.MaxRetries,
		BaseBackoffMS:  e.config.BaseBackoff.Milliseconds(),
		PricingVersion: e.config.Pricing.Version,
		SchemaVersion:  domain.SchemaVersion,
	}
	if err := writeJSONFile(filepath.Join(runDir, "config.json"), runConfig); err != nil {
		return Outcome{}, err
	}

	writer, err := newResultWriter(filepath.Join(runDir, "results.jsonl"))
	if err != nil {
		return Outcome{}, err
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	jobs := make(chan domain.Record, e.config.Concurrency)
	results := make(chan Result, e.config.Concurrency)
	readErr := make(chan error, 1)

	go e.streamDataset(runCtx, absDatasetPath, jobs, readErr)

	var workers sync.WaitGroup
	workers.Add(e.config.Concurrency)
	for workerID := 0; workerID < e.config.Concurrency; workerID++ {
		go func() {
			defer workers.Done()
			e.worker(runCtx, jobs, results)
		}()
	}
	go func() {
		workers.Wait()
		close(results)
	}()

	outcome := Outcome{RunID: runID, RunDir: runDir}
	aggregator := &metrics.Aggregator{}
	evaluationAggregator := evaluation.NewAggregator()
	var writeErr error
	for result := range results {
		if writeErr == nil {
			if err := writer.Append(result); err != nil {
				writeErr = err
				cancel()
			}
		}
		aggregator.Add(result.Status, result.Metrics)
		evaluationAggregator.Add(result.Evaluations)
		if e.config.OnResult != nil {
			e.config.OnResult(result)
		}
		outcome.TotalRecords++
		switch result.Status {
		case domain.StatusSuccess:
			outcome.Succeeded++
		case domain.StatusTimeout:
			outcome.TimedOut++
		default:
			outcome.Failed++
		}
	}

	datasetErr := <-readErr
	closeErr := writer.Close()
	if writeErr != nil {
		return outcome, writeErr
	}
	if closeErr != nil {
		return outcome, closeErr
	}
	if ctx.Err() != nil {
		return outcome, ctx.Err()
	}
	if datasetErr != nil {
		return outcome, fmt.Errorf("stream dataset: %w", datasetErr)
	}

	metricSummary := aggregator.Summary()
	modelSummary := ModelSummary{
		Summary:     metricSummary,
		Evaluations: evaluationMetrics(evaluationAggregator),
	}
	if cost, costErr := e.config.Pricing.Cost(e.modelKey(), metricSummary.InputTokens, metricSummary.OutputTokens); costErr == nil {
		modelSummary.PricingAvailable = true
		modelSummary.EstimatedCostUSD = cost
		if metricSummary.SuccessCount > 0 {
			modelSummary.AverageCostPerSuccessfulRequestUSD = cost / float64(metricSummary.SuccessCount)
		}
	}
	runSummary := RunSummary{
		SchemaVersion:  domain.SchemaVersion,
		RunID:          runID,
		CreatedAt:      createdAt,
		DatasetPath:    absDatasetPath,
		DatasetSHA256:  datasetHash,
		TotalRequests:  metricSummary.TotalRequests,
		PricingVersion: e.config.Pricing.Version,
		Models:         map[string]ModelSummary{e.modelKey(): modelSummary},
	}
	if err := writeJSONFile(filepath.Join(runDir, "summary.json"), runSummary); err != nil {
		return outcome, err
	}
	return outcome, nil
}

func (e *Engine) streamDataset(ctx context.Context, path string, jobs chan<- domain.Record, result chan<- error) {
	defer close(jobs)
	err := dataset.ReadFile(path, func(record domain.Record) error {
		select {
		case jobs <- record:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	result <- err
}

func (e *Engine) worker(ctx context.Context, jobs <-chan domain.Record, results chan<- Result) {
	for {
		select {
		case <-ctx.Done():
			return
		case record, ok := <-jobs:
			if !ok {
				return
			}
			result := e.executeRecord(ctx, record)
			select {
			case results <- result:
			case <-ctx.Done():
				return
			}
		}
	}
}

func (e *Engine) executeRecord(ctx context.Context, record domain.Record) Result {
	result := Result{
		SchemaVersion:      domain.SchemaVersion,
		RecordID:           record.ID,
		Provider:           e.provider.Name(),
		Model:              e.model,
		Request:            record.Request,
		BaselineResponse:   record.Response,
		ExpectedEvaluation: record.Evaluation,
		Status:             domain.StatusError,
	}

	requestCtx, cancel := context.WithTimeout(ctx, e.config.RequestTimeout)
	defer cancel()
	startedAt := time.Now()

	for attempt := 0; attempt <= e.config.MaxRetries; attempt++ {
		result.Attempts = attempt + 1
		response, metrics, err := e.provider.Generate(requestCtx, record.Request)
		if metrics != nil {
			result.Metrics = *metrics
		}
		if err == nil && response == nil {
			err = &provider.Error{Type: provider.ErrParse, Message: "provider returned an empty response"}
		}
		if err == nil {
			result.CandidateResponse = response
			result.Status = domain.StatusSuccess
			result.Metrics.LatencyMs = time.Since(startedAt).Milliseconds()
			if cost, costErr := e.config.Pricing.Cost(e.modelKey(), int64(result.Metrics.InputTokens), int64(result.Metrics.OutputTokens)); costErr == nil {
				result.EstimatedCostUSD = &cost
			}
			e.evaluate(ctx, &record, &result)
			return result
		}

		result.Error = normalizedResultError(err)
		if provider.TypeOf(err) == provider.ErrTimeout {
			result.Status = domain.StatusTimeout
		}
		if !provider.IsRetryable(err) || attempt == e.config.MaxRetries {
			break
		}
		if err := e.sleep(requestCtx, e.retryDelay(attempt, err)); err != nil {
			result.Error = normalizedResultError(&provider.Error{
				Type:       provider.ErrTimeout,
				Message:    "request deadline exceeded during retry backoff",
				Underlying: err,
			})
			result.Status = domain.StatusTimeout
			break
		}
	}

	result.Metrics.LatencyMs = time.Since(startedAt).Milliseconds()
	e.evaluate(ctx, &record, &result)
	return result
}

func (e *Engine) evaluate(ctx context.Context, record *domain.Record, result *Result) {
	for _, evaluator := range e.config.Evaluators {
		result.Evaluations = append(result.Evaluations, evaluator.Evaluate(ctx, record, result.CandidateResponse))
	}
}

func evaluationMetrics(aggregator *evaluation.Aggregator) EvaluationMetrics {
	result := EvaluationMetrics{}
	if rate, ok := aggregator.Rate(evaluation.JSONValidName); ok {
		result.JSONValidRate = &rate
	}
	if rate, ok := aggregator.Rate(evaluation.SchemaAdherenceName); ok {
		result.SchemaAdherenceRate = &rate
	}
	if rate, ok := aggregator.Rate(evaluation.ExactMatchName); ok {
		result.ExactMatchRate = &rate
	}
	return result
}

func (e *Engine) modelKey() string {
	if strings.Contains(e.model, "/") {
		return e.model
	}
	return e.provider.Name() + "/" + e.model
}

func (e *Engine) retryDelay(attempt int, err error) time.Duration {
	var providerErr *provider.Error
	if errors.As(err, &providerErr) && providerErr.RetryAfter > 0 {
		return providerErr.RetryAfter
	}
	delay := e.config.BaseBackoff * time.Duration(1<<attempt)
	if e.config.DisableJitter || delay <= 0 {
		return delay
	}
	return delay + time.Duration(rand.Int63n(int64(delay/2)+1))
}

func normalizedResultError(err error) *ResultError {
	result := &ResultError{Type: provider.TypeOf(err), Message: err.Error()}
	var providerErr *provider.Error
	if errors.As(err, &providerErr) {
		result.Message = providerErr.Message
		result.StatusCode = providerErr.StatusCode
	}
	return result
}

func writeJSONFile(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode replay config: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write replay config: %w", err)
	}
	return nil
}

func sleepContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
