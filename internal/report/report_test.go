package report

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ardao/llm-replay/internal/dataset"
	"github.com/ardao/llm-replay/internal/domain"
	"github.com/ardao/llm-replay/internal/metrics"
	"github.com/ardao/llm-replay/internal/replay"
)

func TestInspectDatasetAndPrint(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dataset.jsonl")
	writer, err := dataset.NewWriter(path)
	if err != nil {
		t.Fatalf("create dataset: %v", err)
	}
	records := []domain.Record{
		reportRecord("one", domain.StatusSuccess, 10, 2, `{"ok":true}`, true),
		reportRecord("two", domain.StatusSuccess, 30, 8, "plain text", false),
		reportRecord("three", domain.StatusError, 999, 999, "", false),
		reportRecord("four", domain.StatusTimeout, 888, 888, "", false),
	}
	for _, record := range records {
		if err := writer.Append(record); err != nil {
			t.Fatalf("append dataset: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close dataset: %v", err)
	}

	summary, err := InspectDataset(path)
	if err != nil {
		t.Fatalf("InspectDataset() error: %v", err)
	}
	if summary.TotalRecords != 4 || summary.SuccessfulCaptures != 2 || summary.FailedCaptures != 1 || summary.TimeoutCaptures != 1 {
		t.Fatalf("unexpected counts: %#v", summary)
	}
	if summary.InputTokensP50 != 10 || summary.InputTokensP95 != 30 || summary.InputTokensMax != 30 {
		t.Fatalf("unexpected input distribution: %#v", summary)
	}
	if summary.ValidJSONOutputs != 1 || summary.EvaluationRecords != 1 {
		t.Fatalf("unexpected content summary: %#v", summary)
	}

	var output bytes.Buffer
	PrintDatasetSummary(&output, summary)
	if !strings.Contains(output.String(), "Total Requests") || !strings.Contains(output.String(), "Valid JSON Outputs") {
		t.Fatalf("unexpected inspect output: %q", output.String())
	}
}

func TestLoadCandidatesAndPrintComparison(t *testing.T) {
	runDir := t.TempDir()
	jsonRate := 0.9
	summary := replay.RunSummary{
		RunID: "run-test",
		Models: map[string]replay.ModelSummary{
			"openai/model-b": {
				Summary:          metrics.Summary{SuccessRate: 0.8, LatencyP95MS: 200, InputTokens: 120},
				PricingAvailable: true,
				EstimatedCostUSD: 2,
			},
			"openai/model-a": {
				Summary:          metrics.Summary{SuccessRate: 1, LatencyP95MS: 100, InputTokens: 100},
				PricingAvailable: true,
				EstimatedCostUSD: 1,
				Evaluations:      replay.EvaluationMetrics{JSONValidRate: &jsonRate},
			},
		},
	}
	data, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("marshal summary: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "summary.json"), data, 0o600); err != nil {
		t.Fatalf("write summary: %v", err)
	}

	candidates, err := LoadCandidates(runDir)
	if err != nil {
		t.Fatalf("LoadCandidates() error: %v", err)
	}
	if len(candidates) != 2 || candidates[0].Name != "openai/model-a" {
		t.Fatalf("candidates are not stable and sorted: %#v", candidates)
	}
	var output bytes.Buffer
	PrintComparison(&output, candidates)
	if !strings.Contains(output.String(), "RESULTS COMPARISON") || !strings.Contains(output.String(), "(+100.0%)") {
		t.Fatalf("unexpected comparison output: %q", output.String())
	}
}

func TestProgress(t *testing.T) {
	var output bytes.Buffer
	progress := NewProgress(&output, "Running", 2)
	progress.Update(1)
	progress.Update(2)
	if !strings.Contains(output.String(), "1/2") || !strings.Contains(output.String(), "2/2\n") {
		t.Fatalf("unexpected progress output: %q", output.String())
	}
}

func reportRecord(id string, status domain.Status, inputTokens, outputTokens int, content string, withEvaluation bool) domain.Record {
	record := domain.Record{
		SchemaVersion: domain.SchemaVersion,
		ID:            id,
		Timestamp:     time.Date(2026, time.September, 10, 12, 0, 0, 0, time.UTC),
		Provider:      "openai",
		Model:         "baseline",
		Request: domain.Request{Messages: []domain.Message{
			{Role: "user", Content: "hello"},
		}},
		Response: domain.Response{Content: content},
		Metrics:  domain.RecordMetrics{InputTokens: inputTokens, OutputTokens: outputTokens},
		Status:   status,
	}
	if withEvaluation {
		record.Evaluation = &domain.ExpectedEvaluation{Expected: content}
	}
	return record
}
