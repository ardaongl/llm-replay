package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ardao/llm-replay/internal/config"
)

func TestRootCommandHelp(t *testing.T) {
	var output bytes.Buffer
	cmd := NewRootCommand(config.Default(), "test", &output, &output)
	cmd.SetArgs([]string{"--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute help: %v", err)
	}

	if !strings.Contains(output.String(), "LLM Replay captures and replays real LLM workloads") {
		t.Fatalf("help output is missing the command description: %q", output.String())
	}
	if !strings.Contains(output.String(), "--verbose") {
		t.Fatalf("help output is missing the verbose flag: %q", output.String())
	}
}

func TestRootCommandVersion(t *testing.T) {
	var output bytes.Buffer
	cmd := NewRootCommand(config.Default(), "1.2.3-test", &output, &output)
	cmd.SetArgs([]string{"--version"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute version: %v", err)
	}
	if !strings.Contains(output.String(), "1.2.3-test") {
		t.Fatalf("unexpected version output: %q", output.String())
	}
}
