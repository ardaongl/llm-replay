package pricing

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestPricingCalculation(t *testing.T) {
	registry := Registry{
		Version: "test",
		Models: map[string]Rate{
			"provider/model": {InputPerMillion: 2, OutputPerMillion: 8},
		},
	}
	cost, err := registry.Cost("provider/model", 1_500_000, 250_000)
	if err != nil {
		t.Fatalf("Cost() error: %v", err)
	}
	if math.Abs(cost-5.0) > 1e-12 {
		t.Fatalf("Cost() = %.12f, want 5.0", cost)
	}
}

func TestLoadWithDefaultsOverridesModels(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pricing.yaml")
	data := []byte(`
version: enterprise-2026-09
models:
  openai/gpt-4o-mini:
    input_per_million: 0.10
    output_per_million: 0.40
  custom/local-model:
    input_per_million: 0
    output_per_million: 0
`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write pricing fixture: %v", err)
	}
	registry, err := LoadWithDefaults(path)
	if err != nil {
		t.Fatalf("LoadWithDefaults() error: %v", err)
	}
	if registry.Version != "enterprise-2026-09" {
		t.Fatalf("version = %q", registry.Version)
	}
	if registry.Models["openai/gpt-4o-mini"].InputPerMillion != 0.10 {
		t.Fatal("default model price was not overridden")
	}
	if _, ok := registry.Models["openai/gpt-4o"]; !ok {
		t.Fatal("unmodified default model was removed")
	}
	if _, ok := registry.Models["custom/local-model"]; !ok {
		t.Fatal("custom model was not added")
	}
}

func TestParseRejectsUnknownOrInvalidFields(t *testing.T) {
	tests := [][]byte{
		[]byte("version: test\nunknown: true\nmodels:\n  a/b: {input_per_million: 1, output_per_million: 1}\n"),
		[]byte("version: test\nmodels:\n  a/b: {input_per_million: -1, output_per_million: 1}\n"),
	}
	for _, data := range tests {
		if _, err := Parse(data); err == nil {
			t.Fatalf("Parse() accepted invalid registry: %s", data)
		}
	}
}

func TestDefaultRegistry(t *testing.T) {
	registry := Default()
	if _, ok := registry.Models["openai/gpt-4o-mini"]; !ok {
		t.Fatal("default registry does not contain gpt-4o-mini")
	}
}
