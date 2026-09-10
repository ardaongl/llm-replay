package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ardao/llm-replay/internal/config"
	"github.com/ardao/llm-replay/internal/dataset"
	"github.com/ardao/llm-replay/internal/domain"
)

func TestInspectCommand(t *testing.T) {
	path := writeCLIDataset(t)
	var output bytes.Buffer
	root := NewRootCommand(config.Default(), "test", &output, &output)
	root.SetArgs([]string{"inspect", path})

	if err := root.Execute(); err != nil {
		t.Fatalf("inspect command: %v", err)
	}
	if !strings.Contains(output.String(), "Total Requests") || !strings.Contains(output.String(), "1") {
		t.Fatalf("unexpected inspect output: %q", output.String())
	}
}

func TestReplayAndCompareCommandsWithMockOpenAI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/chat/completions" {
			t.Errorf("unexpected path: %s", request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("authorization header missing")
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{
            "choices":[{"message":{"content":"{\"category\":\"billing\"}"},"finish_reason":"stop"}],
            "usage":{"prompt_tokens":10,"completion_tokens":4}
        }`))
	}))
	defer server.Close()

	datasetPath := writeCLIDataset(t)
	outputDir := filepath.Join(t.TempDir(), "runs")
	cfg := config.Default()
	cfg.OpenAIAPIKey = "test-key"
	var output bytes.Buffer
	root := NewRootCommand(cfg, "test", &output, &output)
	root.SetArgs([]string{
		"replay", datasetPath,
		"--model", "openai/model-a",
		"--model", "openai/model-b",
		"--openai-base-url", server.URL,
		"--output", outputDir,
		"--concurrency", "1",
		"--timeout", "2s",
		"--retries", "0",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("replay command: %v", err)
	}
	if !strings.Contains(output.String(), "RESULTS COMPARISON") ||
		!strings.Contains(output.String(), "openai/model-a") ||
		!strings.Contains(output.String(), "openai/model-b") ||
		!strings.Contains(output.String(), "1/1") {
		t.Fatalf("unexpected replay output: %q", output.String())
	}

	runDirs, err := filepath.Glob(filepath.Join(outputDir, "run_*"))
	if err != nil {
		t.Fatalf("glob run directories: %v", err)
	}
	if len(runDirs) != 2 {
		t.Fatalf("run directory count = %d, want 2", len(runDirs))
	}
	for _, runDir := range runDirs {
		if _, err := os.Stat(filepath.Join(runDir, "config.json")); err != nil {
			t.Fatalf("config artifact missing: %v", err)
		}
		if _, err := os.Stat(filepath.Join(runDir, "results.jsonl")); err != nil {
			t.Fatalf("results artifact missing: %v", err)
		}
		if _, err := os.Stat(filepath.Join(runDir, "summary.json")); err != nil {
			t.Fatalf("summary artifact missing: %v", err)
		}
	}

	output.Reset()
	compareRoot := NewRootCommand(config.Default(), "test", &output, &output)
	compareRoot.SetArgs(append([]string{"compare"}, runDirs...))
	if err := compareRoot.Execute(); err != nil {
		t.Fatalf("compare command: %v", err)
	}
	if !strings.Contains(output.String(), "RESULTS COMPARISON") || !strings.Contains(output.String(), "JSON Validity") {
		t.Fatalf("unexpected compare output: %q", output.String())
	}
}

func TestReplayRunsOpenAIAndAnthropicInParallel(t *testing.T) {
	bothStarted := make(chan struct{})
	var started atomic.Int32
	waitForPeer := func(t *testing.T) {
		t.Helper()
		if started.Add(1) == 2 {
			close(bothStarted)
		}
		select {
		case <-bothStarted:
		case <-time.After(2 * time.Second):
			t.Error("provider runs did not overlap")
		}
	}

	openAIServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/chat/completions" || request.Header.Get("Authorization") != "Bearer openai-key" {
			t.Errorf("unexpected OpenAI request: %s headers=%v", request.URL.Path, request.Header)
		}
		waitForPeer(t)
		_, _ = writer.Write([]byte(`{
            "choices":[{"message":{"content":"{\"category\":\"billing\"}"},"finish_reason":"stop"}],
            "usage":{"prompt_tokens":10,"completion_tokens":4}
        }`))
	}))
	defer openAIServer.Close()

	anthropicServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/messages" || request.Header.Get("x-api-key") != "anthropic-key" {
			t.Errorf("unexpected Anthropic request: %s headers=%v", request.URL.Path, request.Header)
		}
		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Errorf("decode Anthropic request: %v", err)
		}
		if payload["model"] != "claude-test" || payload["max_tokens"] == nil {
			t.Errorf("unexpected Anthropic payload: %#v", payload)
		}
		waitForPeer(t)
		_, _ = writer.Write([]byte(`{
            "content":[{"type":"text","text":"{\"category\":\"billing\"}"}],
            "stop_reason":"end_turn",
            "usage":{"input_tokens":11,"output_tokens":5}
        }`))
	}))
	defer anthropicServer.Close()

	cfg := config.Default()
	cfg.OpenAIAPIKey = "openai-key"
	cfg.AnthropicAPIKey = "anthropic-key"
	var output bytes.Buffer
	root := NewRootCommand(cfg, "test", &output, &output)
	root.SetArgs([]string{
		"replay", writeCLIDataset(t),
		"--model", "openai/gpt-test",
		"--model", "anthropic/claude-test",
		"--openai-base-url", openAIServer.URL,
		"--anthropic-base-url", anthropicServer.URL,
		"--output", filepath.Join(t.TempDir(), "runs"),
		"--concurrency", "1",
		"--timeout", "3s",
		"--retries", "0",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("cross-provider replay: %v", err)
	}
	for _, expected := range []string{"RESULTS COMPARISON", "openai/gpt-test", "anthropic/claude-test", "1/1"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("output does not contain %q: %s", expected, output.String())
		}
	}
}

func TestReplayCommandRequiresAPIKey(t *testing.T) {
	path := writeCLIDataset(t)
	root := NewRootCommand(config.Default(), "test", &bytes.Buffer{}, &bytes.Buffer{})
	root.SetArgs([]string{"replay", path, "--model", "openai/model"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "OPENAI_API_KEY") {
		t.Fatalf("replay error = %v, want API key guidance", err)
	}
}

func TestCaptureCommandRejectsNonLoopbackListenAddress(t *testing.T) {
	root := NewRootCommand(config.Default(), "test", &bytes.Buffer{}, &bytes.Buffer{})
	root.SetArgs([]string{"capture", "--listen", "0.0.0.0:8787", "--output", filepath.Join(t.TempDir(), "capture.jsonl")})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "not loopback") {
		t.Fatalf("capture error = %v, want loopback validation", err)
	}
}

func TestUICommandRejectsNonLoopbackHost(t *testing.T) {
	root := NewRootCommand(config.Default(), "test", &bytes.Buffer{}, &bytes.Buffer{})
	root.SetArgs([]string{"ui", "unused-run", "--host", "0.0.0.0", "--no-browser"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "not loopback") {
		t.Fatalf("UI error = %v, want loopback validation", err)
	}
}

func TestSelectEvaluators(t *testing.T) {
	selected, err := selectEvaluators([]string{"json", "schema", "match", "json"})
	if err != nil || len(selected) != 3 {
		t.Fatalf("selectEvaluators() = %d, %v", len(selected), err)
	}
	selected, err = selectEvaluators([]string{"none"})
	if err != nil || selected == nil || len(selected) != 0 {
		t.Fatalf("select none = %#v, %v", selected, err)
	}
	if _, err := selectEvaluators([]string{"unknown"}); err == nil {
		t.Fatal("unknown evaluator was accepted")
	}
}

func writeCLIDataset(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "dataset.jsonl")
	writer, err := dataset.NewWriter(path)
	if err != nil {
		t.Fatalf("create dataset: %v", err)
	}
	record := domain.Record{
		SchemaVersion: domain.SchemaVersion,
		ID:            "cli-test-1",
		Timestamp:     time.Date(2026, time.September, 10, 12, 0, 0, 0, time.UTC),
		Provider:      "openai",
		Model:         "baseline",
		Request: domain.Request{Messages: []domain.Message{
			{Role: "user", Content: "classify billing issue"},
		}},
		Response: domain.Response{Content: `{"category":"billing"}`, FinishReason: "stop"},
		Metrics:  domain.RecordMetrics{LatencyMs: 100, InputTokens: 8, OutputTokens: 4},
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
	if err := writer.Append(record); err != nil {
		t.Fatalf("append dataset: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close dataset: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil || !json.Valid(bytes.TrimSpace(data)) {
		t.Fatalf("invalid dataset fixture: %v", err)
	}
	return path
}
