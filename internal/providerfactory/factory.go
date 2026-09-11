package providerfactory

import (
	"fmt"
	"strings"
	"time"

	"github.com/ardao/llm-replay/internal/provider"
	"github.com/ardao/llm-replay/internal/provider/anthropic"
	"github.com/ardao/llm-replay/internal/provider/openai"
)

type Config struct {
	OpenAIAPIKey     string
	AnthropicAPIKey  string
	OpenAIBaseURL    string
	AnthropicBaseURL string
	Timeout          time.Duration
}

func Available(providerName string, config Config) bool {
	switch providerName {
	case "openai":
		return strings.TrimSpace(config.OpenAIAPIKey) != ""
	case "anthropic":
		return strings.TrimSpace(config.AnthropicAPIKey) != ""
	default:
		return false
	}
}

func New(providerName, modelName string, config Config) (provider.Provider, error) {
	switch providerName {
	case "openai":
		return openai.New(openai.Config{
			APIKey: config.OpenAIAPIKey, Model: modelName, BaseURL: config.OpenAIBaseURL, Timeout: config.Timeout,
		})
	case "anthropic":
		return anthropic.New(anthropic.Config{
			APIKey: config.AnthropicAPIKey, Model: modelName, BaseURL: config.AnthropicBaseURL, Timeout: config.Timeout,
		})
	default:
		return nil, fmt.Errorf("provider %q is not supported; use openai or anthropic", providerName)
	}
}
