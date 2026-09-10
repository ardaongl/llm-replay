package main

import (
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	keep := flag.Bool("keep", false, "keep generated run and report artifacts for manual inspection")
	flag.Parse()
	workspace, err := os.Getwd()
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(workspace, "go.mod")); err != nil {
		return fmt.Errorf("run smoke test from the repository root: %w", err)
	}
	temporary, err := os.MkdirTemp("", "llm-replay-smoke-")
	if err != nil {
		return err
	}
	if !*keep {
		defer os.RemoveAll(temporary)
	}

	binary := filepath.Join(temporary, "llm-replay")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	build := exec.Command("go", "build", "-trimpath", "-o", binary, "./cmd/llm-replay")
	build.Dir = workspace
	if output, err := build.CombinedOutput(); err != nil {
		return fmt.Errorf("build CLI: %w\n%s", err, output)
	}

	openAI := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"choices":[{"message":{"content":"{\"category\":\"billing\",\"urgency\":\"high\"}"},"finish_reason":"stop"}],"usage":{"prompt_tokens":40,"completion_tokens":14}}`))
	}))
	defer openAI.Close()
	anthropic := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"content":[{"type":"text","text":"{\"category\":\"billing\",\"urgency\":\"high\"}"}],"stop_reason":"end_turn","usage":{"input_tokens":42,"output_tokens":15}}`))
	}))
	defer anthropic.Close()

	command := exec.Command(binary,
		"replay", filepath.Join("examples", "datasets", "support.jsonl"),
		"--model", "openai/smoke-model",
		"--model", "anthropic/smoke-model",
		"--openai-base-url", openAI.URL,
		"--anthropic-base-url", anthropic.URL,
		"--output", filepath.Join(temporary, "runs"),
		"--concurrency", "8",
		"--timeout", "10s",
		"--retries", "0",
	)
	command.Dir = workspace
	command.Env = append(os.Environ(), "OPENAI_API_KEY=smoke-key", "ANTHROPIC_API_KEY=smoke-key")
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("run benchmark: %w\n%s", err, output)
	}
	text := string(output)
	for _, expected := range []string{"RESULTS COMPARISON", "openai/smoke-model", "anthropic/smoke-model", "120/120"} {
		if !strings.Contains(text, expected) {
			return fmt.Errorf("benchmark output is missing %q\n%s", expected, output)
		}
	}
	runs, err := filepath.Glob(filepath.Join(temporary, "runs", "run_*"))
	if err != nil || len(runs) != 2 {
		return fmt.Errorf("expected two run artifact directories, got %d: %w", len(runs), err)
	}
	reportPath := filepath.Join(temporary, "comparison.html")
	reportArgs := append([]string{"report"}, runs...)
	reportArgs = append(reportArgs, "--output", reportPath)
	reportCommand := exec.Command(binary, reportArgs...)
	reportCommand.Dir = workspace
	if output, err := reportCommand.CombinedOutput(); err != nil {
		return fmt.Errorf("generate HTML report: %w\n%s", err, output)
	}
	reportData, err := os.ReadFile(reportPath)
	if err != nil {
		return fmt.Errorf("read HTML report: %w", err)
	}
	for _, expected := range []string{"replay-data", "openai/smoke-model", "anthropic/smoke-model"} {
		if !strings.Contains(string(reportData), expected) {
			return fmt.Errorf("HTML report is missing %q", expected)
		}
	}
	fmt.Println("Smoke test passed: replayed 120 records across two provider mocks and generated an offline HTML report.")
	if *keep {
		fmt.Printf("Smoke artifacts: %s\n", temporary)
	}
	return nil
}
