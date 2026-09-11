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

func TestArenaCommandUsesWebModelSelection(t *testing.T) {
	var output bytes.Buffer
	cmd := NewRootCommand(config.Default(), "test", &output, &output)
	arenaCommand, _, err := cmd.Find([]string{"arena"})
	if err != nil {
		t.Fatal(err)
	}
	if arenaCommand.Flags().Lookup("model") != nil {
		t.Fatal("arena still exposes CLI model selection")
	}
}

func TestArenaCatalogContainsDistinctSupportedModels(t *testing.T) {
	seen := make(map[string]struct{}, len(defaultArenaModels))
	providers := make(map[string]bool)
	for _, model := range defaultArenaModels {
		id := model.provider + "/" + model.model
		if _, duplicate := seen[id]; duplicate {
			t.Fatalf("duplicate arena catalog model: %s", id)
		}
		seen[id] = struct{}{}
		providers[model.provider] = true
	}
	if len(defaultArenaModels) < 2 || !providers["openai"] || !providers["anthropic"] {
		t.Fatalf("incomplete arena catalog: %#v", defaultArenaModels)
	}
}
