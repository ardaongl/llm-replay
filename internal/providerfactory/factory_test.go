package providerfactory

import (
	"testing"
	"time"
)

func TestAvailabilityRequiresNonWhitespaceCredential(t *testing.T) {
	config := Config{OpenAIAPIKey: "   ", AnthropicAPIKey: "key"}
	if Available("openai", config) {
		t.Fatal("whitespace OpenAI credential was accepted")
	}
	if !Available("anthropic", config) {
		t.Fatal("Anthropic credential was not detected")
	}
	if Available("unknown", config) {
		t.Fatal("unsupported provider was marked available")
	}
}

func TestFactoryBuildsSupportedProvidersAndRejectsUnknown(t *testing.T) {
	config := Config{OpenAIAPIKey: "key", AnthropicAPIKey: "key", Timeout: time.Second}
	for _, test := range []struct{ provider, model string }{{"openai", "one"}, {"anthropic", "two"}} {
		adapter, err := New(test.provider, test.model, config)
		if err != nil || adapter.Name() != test.provider {
			t.Fatalf("New(%s) = %#v, %v", test.provider, adapter, err)
		}
	}
	if _, err := New("unknown", "model", config); err == nil {
		t.Fatal("unsupported provider was accepted")
	}
}
