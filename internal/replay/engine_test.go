package replay

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ardao/llm-replay/internal/dataset"
	"github.com/ardao/llm-replay/internal/domain"
	"github.com/ardao/llm-replay/internal/pricing"
	"github.com/ardao/llm-replay/internal/provider"
)

type mockProvider struct {
	generate func(context.Context, domain.Request) (*domain.Response, *domain.RecordMetrics, error)
}

func (m *mockProvider) Name() string {
	return "mock"
}

func (m *mockProvider) Generate(ctx context.Context, request domain.Request) (*domain.Response, *domain.RecordMetrics, error) {
	return m.generate(ctx, request)
}

func TestEngineRunStreamsWithBoundedConcurrencyAndWritesArtifacts(t *testing.T) {
	datasetPath := writeDataset(t, 100)
	var active atomic.Int32
	var maximum atomic.Int32
	adapter := &mockProvider{generate: func(context.Context, domain.Request) (*domain.Response, *domain.RecordMetrics, error) {
		current := active.Add(1)
		for {
			seen := maximum.Load()
			if current <= seen || maximum.CompareAndSwap(seen, current) {
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
		active.Add(-1)
		return &domain.Response{Content: `{"category":"billing"}`, FinishReason: "stop"}, &domain.RecordMetrics{InputTokens: 10, OutputTokens: 2}, nil
	}}
	outputDir := filepath.Join(t.TempDir(), "runs")
	registry := pricing.Registry{
		Version: "test-pricing",
		Models: map[string]pricing.Rate{
			"mock/candidate-model": {InputPerMillion: 1, OutputPerMillion: 2},
		},
	}
	engine := mustEngine(t, adapter, Config{
		Concurrency:    5,
		RequestTimeout: time.Second,
		MaxRetries:     1,
		BaseBackoff:    time.Millisecond,
		OutputDir:      outputDir,
		DisableJitter:  true,
		Pricing:        &registry,
	})
	fixedTime := time.Date(2026, time.September, 10, 15, 30, 0, 123, time.UTC)
	engine.now = func() time.Time { return fixedTime }

	outcome, err := engine.Run(context.Background(), datasetPath)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if outcome.TotalRecords != 100 || outcome.Succeeded != 100 || outcome.Failed != 0 || outcome.TimedOut != 0 {
		t.Fatalf("unexpected outcome: %#v", outcome)
	}
	if got := maximum.Load(); got < 2 || got > 5 {
		t.Fatalf("maximum concurrency = %d, want 2..5", got)
	}
	if outcome.RunID != "run_20260910_153000_000000123" {
		t.Fatalf("run ID = %q", outcome.RunID)
	}

	configData, err := os.ReadFile(filepath.Join(outcome.RunDir, "config.json"))
	if err != nil {
		t.Fatalf("read config artifact: %v", err)
	}
	var runConfig RunConfig
	if err := json.Unmarshal(configData, &runConfig); err != nil {
		t.Fatalf("decode config artifact: %v", err)
	}
	wantHash, err := dataset.HashFile(datasetPath)
	if err != nil {
		t.Fatalf("hash fixture: %v", err)
	}
	if runConfig.DatasetSHA256 != wantHash || runConfig.Concurrency != 5 || runConfig.Model != "candidate-model" {
		t.Fatalf("unexpected run config: %#v", runConfig)
	}
	if runConfig.PricingVersion != "test-pricing" {
		t.Fatalf("pricing version = %q", runConfig.PricingVersion)
	}

	summaryData, err := os.ReadFile(filepath.Join(outcome.RunDir, "summary.json"))
	if err != nil {
		t.Fatalf("read summary artifact: %v", err)
	}
	var runSummary RunSummary
	if err := json.Unmarshal(summaryData, &runSummary); err != nil {
		t.Fatalf("decode summary artifact: %v", err)
	}
	modelSummary, ok := runSummary.Models["mock/candidate-model"]
	if !ok {
		t.Fatalf("model summary missing: %#v", runSummary.Models)
	}
	if modelSummary.TotalRequests != 100 || modelSummary.InputTokens != 1000 || modelSummary.OutputTokens != 200 {
		t.Fatalf("unexpected model summary: %#v", modelSummary)
	}
	if !modelSummary.PricingAvailable || math.Abs(modelSummary.EstimatedCostUSD-0.0014) > 1e-12 {
		t.Fatalf("unexpected cost summary: %#v", modelSummary)
	}
	if modelSummary.Evaluations.JSONValidRate == nil || *modelSummary.Evaluations.JSONValidRate != 1 ||
		modelSummary.Evaluations.SchemaAdherenceRate == nil || *modelSummary.Evaluations.SchemaAdherenceRate != 1 ||
		modelSummary.Evaluations.ExactMatchRate == nil || *modelSummary.Evaluations.ExactMatchRate != 1 {
		t.Fatalf("unexpected evaluation summary: %#v", modelSummary.Evaluations)
	}

	results := readResults(t, filepath.Join(outcome.RunDir, "results.jsonl"))
	if len(results) != 100 {
		t.Fatalf("result count = %d, want 100", len(results))
	}
	for _, result := range results {
		if result.Status != domain.StatusSuccess || result.CandidateResponse == nil || result.Attempts != 1 || result.EstimatedCostUSD == nil || len(result.Evaluations) != 3 {
			t.Fatalf("unexpected result: %#v", result)
		}
	}
}

func TestExecuteRecordRetriesTransientErrors(t *testing.T) {
	var calls atomic.Int32
	adapter := &mockProvider{generate: func(context.Context, domain.Request) (*domain.Response, *domain.RecordMetrics, error) {
		if calls.Add(1) <= 2 {
			return nil, &domain.RecordMetrics{}, &provider.Error{Type: provider.ErrRateLimit, StatusCode: 429, Message: "slow down"}
		}
		return &domain.Response{Content: "ok"}, &domain.RecordMetrics{InputTokens: 4, OutputTokens: 1}, nil
	}}
	engine := mustEngine(t, adapter, Config{Concurrency: 1, RequestTimeout: time.Second, MaxRetries: 2, BaseBackoff: time.Millisecond, DisableJitter: true})

	result := engine.executeRecord(context.Background(), replayRecord("rec-retry"))
	if result.Status != domain.StatusSuccess || result.Attempts != 3 || calls.Load() != 3 {
		t.Fatalf("unexpected retry result: %#v, calls=%d", result, calls.Load())
	}
}

func TestExecuteRecordDoesNotRetryPermanentError(t *testing.T) {
	var calls atomic.Int32
	adapter := &mockProvider{generate: func(context.Context, domain.Request) (*domain.Response, *domain.RecordMetrics, error) {
		calls.Add(1)
		return nil, &domain.RecordMetrics{}, &provider.Error{Type: provider.ErrInvalidRequest, StatusCode: 400, Message: "bad input"}
	}}
	engine := mustEngine(t, adapter, Config{Concurrency: 1, RequestTimeout: time.Second, MaxRetries: 3, BaseBackoff: time.Millisecond})

	result := engine.executeRecord(context.Background(), replayRecord("rec-invalid"))
	if result.Status != domain.StatusError || result.Attempts != 1 || calls.Load() != 1 {
		t.Fatalf("unexpected permanent error result: %#v, calls=%d", result, calls.Load())
	}
}

func TestExecuteRecordHonorsRetryAfter(t *testing.T) {
	var calls atomic.Int32
	adapter := &mockProvider{generate: func(context.Context, domain.Request) (*domain.Response, *domain.RecordMetrics, error) {
		if calls.Add(1) == 1 {
			return nil, nil, &provider.Error{Type: provider.ErrRateLimit, Message: "slow down", RetryAfter: 7 * time.Second}
		}
		return &domain.Response{Content: "ok"}, &domain.RecordMetrics{}, nil
	}}
	engine := mustEngine(t, adapter, Config{Concurrency: 1, RequestTimeout: time.Minute, MaxRetries: 1, BaseBackoff: time.Millisecond})
	var slept time.Duration
	engine.sleep = func(_ context.Context, duration time.Duration) error {
		slept = duration
		return nil
	}

	result := engine.executeRecord(context.Background(), replayRecord("rec-retry-after"))
	if result.Status != domain.StatusSuccess || slept != 7*time.Second {
		t.Fatalf("result = %#v, slept = %s", result, slept)
	}
}

func TestExecuteRecordTimeoutDoesNotBlock(t *testing.T) {
	adapter := &mockProvider{generate: func(ctx context.Context, _ domain.Request) (*domain.Response, *domain.RecordMetrics, error) {
		<-ctx.Done()
		return nil, &domain.RecordMetrics{}, &provider.Error{Type: provider.ErrTimeout, Message: "timed out", Underlying: ctx.Err()}
	}}
	engine := mustEngine(t, adapter, Config{Concurrency: 1, RequestTimeout: 20 * time.Millisecond, MaxRetries: 2, BaseBackoff: time.Millisecond})

	startedAt := time.Now()
	result := engine.executeRecord(context.Background(), replayRecord("rec-timeout"))
	if result.Status != domain.StatusTimeout || time.Since(startedAt) > 500*time.Millisecond {
		t.Fatalf("unexpected timeout result: %#v", result)
	}
}

func TestEngineContinuesAfterRequestTimeouts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mixed.jsonl")
	writer, err := dataset.NewWriter(path)
	if err != nil {
		t.Fatalf("create fixture: %v", err)
	}
	for i := 0; i < 10; i++ {
		record := replayRecord(time.Unix(int64(i), 0).UTC().Format("150405"))
		if i%2 == 0 {
			record.Request.Messages[0].Content = "slow"
		} else {
			record.Request.Messages[0].Content = "fast"
		}
		if err := writer.Append(record); err != nil {
			t.Fatalf("append fixture: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close fixture: %v", err)
	}

	adapter := &mockProvider{generate: func(ctx context.Context, request domain.Request) (*domain.Response, *domain.RecordMetrics, error) {
		if request.Messages[0].Content == "slow" {
			<-ctx.Done()
			return nil, &domain.RecordMetrics{}, &provider.Error{Type: provider.ErrTimeout, Message: "timed out", Underlying: ctx.Err()}
		}
		return &domain.Response{Content: "ok"}, &domain.RecordMetrics{}, nil
	}}
	engine := mustEngine(t, adapter, Config{
		Concurrency:    3,
		RequestTimeout: 20 * time.Millisecond,
		MaxRetries:     1,
		BaseBackoff:    time.Millisecond,
		OutputDir:      filepath.Join(t.TempDir(), "runs"),
		DisableJitter:  true,
	})

	outcome, err := engine.Run(context.Background(), path)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if outcome.TotalRecords != 10 || outcome.Succeeded != 5 || outcome.TimedOut != 5 {
		t.Fatalf("unexpected mixed outcome: %#v", outcome)
	}
}

func TestRunRejectsCorruptedDatasetBeforeCreatingArtifacts(t *testing.T) {
	tempDir := t.TempDir()
	datasetPath := filepath.Join(tempDir, "broken.jsonl")
	if err := os.WriteFile(datasetPath, []byte("{broken}\n"), 0o600); err != nil {
		t.Fatalf("write broken dataset: %v", err)
	}
	outputDir := filepath.Join(tempDir, "runs")
	adapter := &mockProvider{generate: func(context.Context, domain.Request) (*domain.Response, *domain.RecordMetrics, error) {
		t.Fatal("provider should not be called")
		return nil, nil, nil
	}}
	engine := mustEngine(t, adapter, Config{Concurrency: 1, RequestTimeout: time.Second, OutputDir: outputDir})

	_, err := engine.Run(context.Background(), datasetPath)
	if err == nil {
		t.Fatal("Run() succeeded for corrupted dataset")
	}
	if _, statErr := os.Stat(outputDir); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("output directory exists after validation failure: %v", statErr)
	}
}

func writeDataset(t *testing.T, count int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "dataset.jsonl")
	writer, err := dataset.NewWriter(path)
	if err != nil {
		t.Fatalf("create dataset: %v", err)
	}
	for i := 0; i < count; i++ {
		record := replayRecord("rec-" + time.Unix(int64(i), 0).UTC().Format("150405"))
		if err := writer.Append(record); err != nil {
			t.Fatalf("append fixture: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close fixture: %v", err)
	}
	return path
}

func replayRecord(id string) domain.Record {
	return domain.Record{
		SchemaVersion: domain.SchemaVersion,
		ID:            id,
		Timestamp:     time.Date(2026, time.September, 10, 12, 0, 0, 0, time.UTC),
		Provider:      "openai",
		Model:         "baseline-model",
		Request: domain.Request{Messages: []domain.Message{
			{Role: "user", Content: "hello"},
		}},
		Response: domain.Response{Content: "baseline", FinishReason: "stop"},
		Metrics:  domain.RecordMetrics{LatencyMs: 10, InputTokens: 3, OutputTokens: 1},
		Evaluation: &domain.ExpectedEvaluation{
			Expected: `{"category":"billing"}`,
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"category": map[string]any{"type": "string"},
				},
				"required": []any{"category"},
			},
		},
		Status: domain.StatusSuccess,
	}
}

func readResults(t *testing.T, path string) []Result {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open results: %v", err)
	}
	defer file.Close()

	var results []Result
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var result Result
		if err := json.Unmarshal(scanner.Bytes(), &result); err != nil {
			t.Fatalf("decode result: %v", err)
		}
		results = append(results, result)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan results: %v", err)
	}
	return results
}

func mustEngine(t *testing.T, adapter provider.Provider, config Config) *Engine {
	t.Helper()
	engine, err := New(adapter, "candidate-model", config)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	return engine
}
