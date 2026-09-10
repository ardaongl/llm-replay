package config

import (
	"testing"
	"time"
)

func TestDefault(t *testing.T) {
	cfg := Default()
	if cfg.Concurrency != 5 {
		t.Fatalf("default concurrency = %d, want 5", cfg.Concurrency)
	}
	if cfg.Timeout != 30*time.Second {
		t.Fatalf("default timeout = %s, want 30s", cfg.Timeout)
	}
}

func TestLoadFromEnvironment(t *testing.T) {
	t.Setenv("LLM_REPLAY_VERBOSE", "true")
	t.Setenv("LLM_REPLAY_CONCURRENCY", "9")
	t.Setenv("LLM_REPLAY_TIMEOUT", "45s")
	t.Setenv("OPENAI_API_KEY", "test-openai-secret")
	t.Setenv("ANTHROPIC_API_KEY", "test-anthropic-secret")

	cfg := Load()
	if !cfg.Verbose || cfg.Concurrency != 9 || cfg.Timeout != 45*time.Second {
		t.Fatalf("environment settings were not loaded: %+v", cfg)
	}
	if cfg.OpenAIAPIKey == "" || cfg.AnthropicAPIKey == "" {
		t.Fatal("provider keys were not loaded")
	}
}

func TestLoadIgnoresInvalidEnvironmentValues(t *testing.T) {
	t.Setenv("LLM_REPLAY_CONCURRENCY", "0")
	t.Setenv("LLM_REPLAY_TIMEOUT", "invalid")

	cfg := Load()
	if cfg.Concurrency != 5 || cfg.Timeout != 30*time.Second {
		t.Fatalf("invalid values changed defaults: %+v", cfg)
	}
}
