package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadUsesConfigFileEnvironmentAndOverrides(t *testing.T) {
	t.Setenv("TTOOLS_LLM_MODEL", "env-model")
	t.Setenv("TTOOLS_LLM_API_KEY", "env-key")

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(configPath, []byte(`
[llm]
provider = "openai"
model = "file-model"
api_key = "file-key"
base_url = "https://example.test/v1"
`), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(Options{ConfigFile: configPath, Model: "flag-model"})
	if err != nil {
		t.Fatal(err)
	}

	if cfg.LLM.Provider != "openai" {
		t.Fatalf("provider = %q", cfg.LLM.Provider)
	}
	if cfg.LLM.Model != "flag-model" {
		t.Fatalf("model = %q", cfg.LLM.Model)
	}
	if cfg.LLM.APIKey != "env-key" {
		t.Fatalf("api key = %q", cfg.LLM.APIKey)
	}
	if cfg.LLM.BaseURL != "https://example.test/v1" {
		t.Fatalf("base url = %q", cfg.LLM.BaseURL)
	}
}

func TestLoadDefaultsToCopilot(t *testing.T) {
	cfg, err := Load(Options{})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LLM.Provider != "copilot" {
		t.Fatalf("provider = %q", cfg.LLM.Provider)
	}
}
