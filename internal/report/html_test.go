package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ardao/llm-replay/internal/domain"
	"github.com/ardao/llm-replay/internal/metrics"
	"github.com/ardao/llm-replay/internal/replay"
)

func TestGenerateHTMLIsSelfContainedAndEscapesData(t *testing.T) {
	run := writeHTMLRun(t, "openai/model-a", `</script><script>alert("xss")</script>`)
	output := filepath.Join(t.TempDir(), "report.html")
	result, err := GenerateHTML(HTMLOptions{RunDirs: []string{run}, OutputPath: output, MaxResults: 10})
	if err != nil {
		t.Fatalf("GenerateHTML() error: %v", err)
	}
	if result.Included != 1 || result.Total != 1 || result.Truncated {
		t.Fatalf("unexpected result: %#v", result)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	html := string(data)
	for _, expected := range []string{"<!doctype html>", "replay-data", "openai/model-a", "Request explorer"} {
		if !strings.Contains(html, expected) {
			t.Fatalf("HTML does not contain %q", expected)
		}
	}
	if strings.Contains(html, `<script>alert("xss")</script>`) || strings.Contains(html, "src=\"http") || strings.Contains(html, "href=\"http") {
		t.Fatalf("HTML contains executable input or external asset reference")
	}
	if !strings.Contains(html, `\u003c/script\u003e`) {
		t.Fatalf("HTML does not contain safely escaped input")
	}
}

func TestGenerateHTMLTruncatesAndRefusesOverwrite(t *testing.T) {
	run := writeHTMLRunRecords(t, "openai/model-a", []string{"safe one", "safe two"})
	output := filepath.Join(t.TempDir(), "report.html")
	result, err := GenerateHTML(HTMLOptions{RunDirs: []string{run}, OutputPath: output, MaxResults: 1})
	if err != nil || result.Included != 1 || result.Total != 2 || !result.Truncated {
		t.Fatalf("GenerateHTML() = %#v, %v", result, err)
	}
	if _, err := GenerateHTML(HTMLOptions{RunDirs: []string{run}, OutputPath: output}); err == nil {
		t.Fatal("existing report was overwritten")
	}
}

func TestGenerateHTMLFailuresOnly(t *testing.T) {
	run := writeHTMLRunRecords(t, "openai/model-a", []string{"successful", "failed"})
	resultsPath := filepath.Join(run, "results.jsonl")
	data, err := os.ReadFile(resultsPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	var failed replay.Result
	if err := json.Unmarshal([]byte(lines[1]), &failed); err != nil {
		t.Fatal(err)
	}
	failed.Status = domain.StatusError
	failed.CandidateResponse = nil
	encoded, err := json.Marshal(failed)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(resultsPath, []byte(lines[0]+"\n"+string(encoded)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := GenerateHTML(HTMLOptions{RunDirs: []string{run}, OutputPath: filepath.Join(t.TempDir(), "failures.html"), FailuresOnly: true})
	if err != nil || result.Total != 1 || result.Included != 1 {
		t.Fatalf("GenerateHTML(failures only) = %#v, %v", result, err)
	}
}

func writeHTMLRun(t *testing.T, model, prompt string) string {
	t.Helper()
	return writeHTMLRunRecords(t, model, []string{prompt})
}

func writeHTMLRunRecords(t *testing.T, model string, prompts []string) string {
	t.Helper()
	directory := t.TempDir()
	created := time.Date(2026, time.September, 11, 12, 0, 0, 0, time.UTC)
	config := replay.RunConfig{ID: "run-test", CreatedAt: created, DatasetPath: "dataset.jsonl", DatasetSHA256: "dataset-hash", Provider: "openai", Model: model, Concurrency: 1, TimeoutMS: 1000, SchemaVersion: domain.SchemaVersion}
	summary := replay.RunSummary{SchemaVersion: domain.SchemaVersion, RunID: config.ID, CreatedAt: created, DatasetPath: config.DatasetPath, DatasetSHA256: config.DatasetSHA256, TotalRequests: len(prompts), Models: map[string]replay.ModelSummary{model: {Summary: metrics.Summary{TotalRequests: len(prompts), SuccessCount: len(prompts), SuccessRate: 1}}}}
	writeHTMLJSON(t, filepath.Join(directory, "config.json"), config)
	writeHTMLJSON(t, filepath.Join(directory, "summary.json"), summary)
	file, err := os.Create(filepath.Join(directory, "results.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	encoder := json.NewEncoder(file)
	for index, prompt := range prompts {
		result := replay.Result{SchemaVersion: domain.SchemaVersion, RecordID: fmt.Sprintf("record-%d", index+1), Provider: "openai", Model: model, Request: domain.Request{Messages: []domain.Message{{Role: "user", Content: prompt}}}, BaselineResponse: domain.Response{Content: "baseline"}, CandidateResponse: &domain.Response{Content: "candidate"}, Status: domain.StatusSuccess}
		if err := encoder.Encode(result); err != nil {
			t.Fatal(err)
		}
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return directory
}

func writeHTMLJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
