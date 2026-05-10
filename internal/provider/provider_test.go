package provider

import (
	"testing"

	"ttools/internal/config"
)

func TestNewFactorySelectsSupportedProviders(t *testing.T) {
	factory := NewFactory(nil)

	for _, name := range []string{"openai", "copilot"} {
		client, err := factory.New(config.LLMConfig{Provider: name, Model: "gpt-test", APIKey: "key", BaseURL: "https://example.test/v1"})
		if err != nil {
			t.Fatalf("provider %s returned error: %v", name, err)
		}
		if client == nil {
			t.Fatalf("provider %s returned nil client", name)
		}
	}
}

func TestNewFactoryRejectsUnknownProvider(t *testing.T) {
	_, err := NewFactory(nil).New(config.LLMConfig{Provider: "unknown"})
	if err == nil {
		t.Fatal("expected error")
	}
}
