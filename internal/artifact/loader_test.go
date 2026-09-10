package artifact

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ardao/llm-replay/internal/domain"
	"github.com/ardao/llm-replay/internal/evaluation"
	"github.com/ardao/llm-replay/internal/metrics"
	"github.com/ardao/llm-replay/internal/replay"
)

func TestLoadComparisonJoinsRunsByRecordID(t *testing.T) {
	root := t.TempDir()
	first := writeRun(t, root, "run-a", "openai/model-a", "same-hash", []replay.Result{
		result("record-2", "openai/model-a", domain.StatusError, 2200, false),
		result("record-1", "openai/model-a", domain.StatusSuccess, 120, true),
	})
	second := writeRun(t, root, "run-b", "anthropic/model-b", "same-hash", []replay.Result{
		result("record-1", "anthropic/model-b", domain.StatusSuccess, 90, true),
		result("record-2", "anthropic/model-b", domain.StatusSuccess, 180, true),
	})

	comparison, err := LoadComparison([]string{first, second})
	if err != nil {
		t.Fatalf("LoadComparison() error: %v", err)
	}
	if len(comparison.Runs) != 2 || len(comparison.Records) != 2 {
		t.Fatalf("unexpected comparison dimensions: %#v", comparison)
	}
	if comparison.Records[0].ID != "record-1" || len(comparison.Records[0].Models) != 2 {
		t.Fatalf("records were not joined and sorted: %#v", comparison.Records)
	}

	failed := false
	page, err := comparison.Page(Query{Evaluator: "schema", Passed: &failed, MinLatencyMS: 2000}, "", 1)
	if err != nil || page.Total != 1 || page.Records[0].ID != "record-2" {
		t.Fatalf("filtered page = %#v, %v", page, err)
	}
	if _, err := comparison.Page(Query{}, "bad cursor", 50); err == nil {
		t.Fatal("invalid cursor was accepted")
	}
	firstPage, err := comparison.Page(Query{}, "", 1)
	if err != nil || firstPage.NextCursor == "" || firstPage.Records[0].ID != "record-1" {
		t.Fatalf("first cursor page = %#v, %v", firstPage, err)
	}
	secondPage, err := comparison.Page(Query{}, firstPage.NextCursor, 1)
	if err != nil || secondPage.NextCursor != "" || secondPage.Records[0].ID != "record-2" {
		t.Fatalf("second cursor page = %#v, %v", secondPage, err)
	}
	if _, err := comparison.Page(Query{}, "", 201); err == nil {
		t.Fatal("page limit above 200 was accepted")
	}
}

func TestLoadComparisonRejectsIncompatibleRuns(t *testing.T) {
	root := t.TempDir()
	first := writeRun(t, root, "run-a", "openai/model-a", "hash-a", []replay.Result{result("record-1", "openai/model-a", domain.StatusSuccess, 10, true)})
	second := writeRun(t, root, "run-b", "anthropic/model-b", "hash-b", []replay.Result{result("record-1", "anthropic/model-b", domain.StatusSuccess, 10, true)})
	if _, err := LoadComparison([]string{first, second}); err == nil || !strings.Contains(err.Error(), "expected hash-a") {
		t.Fatalf("dataset mismatch error = %v", err)
	}

	third := writeRun(t, root, "run-c", "anthropic/model-c", "hash-a", []replay.Result{result("record-2", "anthropic/model-c", domain.StatusSuccess, 10, true)})
	if _, err := LoadComparison([]string{first, third}); err == nil || !strings.Contains(err.Error(), "record set differs") {
		t.Fatalf("record mismatch error = %v", err)
	}

	fourthResult := result("record-1", "anthropic/model-d", domain.StatusSuccess, 10, true)
	fourthResult.Request.Messages[0].Content = "different request"
	fourth := writeRun(t, root, "run-d", "anthropic/model-d", "hash-a", []replay.Result{fourthResult})
	if _, err := LoadComparison([]string{first, fourth}); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("record content mismatch error = %v", err)
	}
}

func TestLoadComparisonRejectsDuplicateRecordID(t *testing.T) {
	root := t.TempDir()
	run := writeRun(t, root, "run-a", "openai/model-a", "hash", []replay.Result{
		result("record-1", "openai/model-a", domain.StatusSuccess, 10, true),
		result("record-1", "openai/model-a", domain.StatusSuccess, 11, true),
	})
	if _, err := LoadComparison([]string{run}); err == nil || !strings.Contains(err.Error(), "duplicate record_id") {
		t.Fatalf("duplicate ID error = %v", err)
	}
}

func TestLoadComparisonRejectsDuplicateModelAndMalformedRun(t *testing.T) {
	root := t.TempDir()
	run := writeRun(t, root, "run-a", "openai/model-a", "hash", []replay.Result{result("record-1", "openai/model-a", domain.StatusSuccess, 10, true)})
	if _, err := LoadComparison([]string{run, run}); err == nil || !strings.Contains(err.Error(), "duplicate model") {
		t.Fatalf("duplicate model error = %v", err)
	}

	badJSON := writeRun(t, root, "run-b", "openai/model-b", "hash", []replay.Result{result("record-1", "openai/model-b", domain.StatusSuccess, 10, true)})
	if err := os.WriteFile(filepath.Join(badJSON, "results.jsonl"), []byte("not-json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadComparison([]string{badJSON}); err == nil || !strings.Contains(err.Error(), "decode") {
		t.Fatalf("malformed JSONL error = %v", err)
	}

	badSchema := writeRun(t, root, "run-c", "openai/model-c", "hash", []replay.Result{result("record-1", "openai/model-c", domain.StatusSuccess, 10, true)})
	var config replay.RunConfig
	configData, err := os.ReadFile(filepath.Join(badSchema, "config.json"))
	if err != nil || json.Unmarshal(configData, &config) != nil {
		t.Fatal("read config fixture")
	}
	config.SchemaVersion = "unsupported"
	writeTestJSON(t, filepath.Join(badSchema, "config.json"), config)
	if _, err := LoadComparison([]string{badSchema}); err == nil || !strings.Contains(err.Error(), "unsupported schema") {
		t.Fatalf("schema version error = %v", err)
	}

	missing := writeRun(t, root, "run-d", "openai/model-d", "hash", []replay.Result{result("record-1", "openai/model-d", domain.StatusSuccess, 10, true)})
	if err := os.Remove(filepath.Join(missing, "summary.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadComparison([]string{missing}); err == nil || !strings.Contains(err.Error(), "summary.json") {
		t.Fatalf("missing artifact error = %v", err)
	}
}

func TestLoadComparisonRejectsOversizedArtifacts(t *testing.T) {
	root := t.TempDir()
	run := writeRun(t, root, "run-a", "openai/model-a", "hash", []replay.Result{result("record-1", "openai/model-a", domain.StatusSuccess, 10, true)})
	if err := os.Truncate(filepath.Join(run, "results.jsonl"), maxComparisonArtifactSize+1); err != nil {
		t.Skipf("cannot create sparse artifact: %v", err)
	}
	if _, err := LoadComparison([]string{run}); err == nil || !strings.Contains(err.Error(), "512 MiB") {
		t.Fatalf("oversized artifact error = %v", err)
	}
}

func writeRun(t *testing.T, root, directory, model, hash string, results []replay.Result) string {
	t.Helper()
	path := filepath.Join(root, directory)
	if err := os.Mkdir(path, 0o750); err != nil {
		t.Fatal(err)
	}
	config := replay.RunConfig{ID: directory, CreatedAt: time.Now().UTC(), DatasetPath: "dataset.jsonl", DatasetSHA256: hash, Provider: strings.Split(model, "/")[0], Model: model, Concurrency: 1, TimeoutMS: 1000, SchemaVersion: domain.SchemaVersion}
	summary := replay.RunSummary{SchemaVersion: domain.SchemaVersion, RunID: directory, CreatedAt: config.CreatedAt, DatasetPath: config.DatasetPath, DatasetSHA256: hash, TotalRequests: len(results), Models: map[string]replay.ModelSummary{model: {Summary: metrics.Summary{TotalRequests: len(results)}}}}
	writeTestJSON(t, filepath.Join(path, "config.json"), config)
	writeTestJSON(t, filepath.Join(path, "summary.json"), summary)
	file, err := os.Create(filepath.Join(path, "results.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	encoder := json.NewEncoder(file)
	for _, item := range results {
		if err := encoder.Encode(item); err != nil {
			t.Fatal(err)
		}
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeTestJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func result(id, model string, status domain.Status, latency int64, schemaPassed bool) replay.Result {
	providerName := strings.Split(model, "/")[0]
	return replay.Result{
		SchemaVersion: domain.SchemaVersion, RecordID: id, Provider: providerName, Model: model,
		Request:          domain.Request{Messages: []domain.Message{{Role: "user", Content: "request " + id}}},
		BaselineResponse: domain.Response{Content: `{"ok":true}`}, CandidateResponse: &domain.Response{Content: `{"ok":true}`},
		Metrics: domain.RecordMetrics{LatencyMs: latency}, Status: status,
		Evaluations: []evaluation.Result{{Name: evaluation.SchemaAdherenceName, Passed: schemaPassed}},
	}
}
