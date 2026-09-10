package config

import (
	"os"
	"strconv"
	"time"
)

const (
	defaultConcurrency = 5
	defaultTimeout     = 30 * time.Second
)

// Config contains process-wide settings shared by CLI commands.
// Provider secrets are read from the environment and must never be logged.
type Config struct {
	Verbose         bool
	Concurrency     int
	Timeout         time.Duration
	OpenAIAPIKey    string
	AnthropicAPIKey string
}

// Default returns safe defaults for a local CLI process.
func Default() *Config {
	return &Config{
		Concurrency: defaultConcurrency,
		Timeout:     defaultTimeout,
	}
}

// Load applies environment variables over the built-in defaults.
func Load() *Config {
	cfg := Default()
	cfg.OpenAIAPIKey = os.Getenv("OPENAI_API_KEY")
	cfg.AnthropicAPIKey = os.Getenv("ANTHROPIC_API_KEY")

	if value, err := strconv.ParseBool(os.Getenv("LLM_REPLAY_VERBOSE")); err == nil {
		cfg.Verbose = value
	}
	if value, err := strconv.Atoi(os.Getenv("LLM_REPLAY_CONCURRENCY")); err == nil && value > 0 {
		cfg.Concurrency = value
	}
	if value, err := time.ParseDuration(os.Getenv("LLM_REPLAY_TIMEOUT")); err == nil && value > 0 {
		cfg.Timeout = value
	}

	return cfg
}
