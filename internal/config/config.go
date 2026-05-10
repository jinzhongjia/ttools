package config

import (
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	LLM LLMConfig
}

type LLMConfig struct {
	Provider string
	Model    string
	APIKey   string
	BaseURL  string
}

type Options struct {
	ConfigFile string
	Provider   string
	Model      string
	APIKey     string
	BaseURL    string
}

func Load(opts Options) (Config, error) {
	v := viper.New()
	v.SetConfigType("toml")
	v.SetEnvPrefix("TTOOLS")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	v.SetDefault("llm.provider", "copilot")
	v.SetDefault("llm.base_url", "https://api.openai.com/v1")

	_ = v.BindEnv("llm.provider")
	_ = v.BindEnv("llm.model")
	_ = v.BindEnv("llm.api_key")
	_ = v.BindEnv("llm.base_url")

	if opts.ConfigFile != "" {
		v.SetConfigFile(opts.ConfigFile)
		if err := v.ReadInConfig(); err != nil {
			return Config{}, err
		}
	} else {
		v.SetConfigName("config")
		v.SetConfigType("toml")
		v.AddConfigPath("$HOME/.config/ttools")
		_ = v.ReadInConfig()
	}

	if opts.Provider != "" {
		v.Set("llm.provider", opts.Provider)
	}
	if opts.Model != "" {
		v.Set("llm.model", opts.Model)
	}
	if opts.APIKey != "" {
		v.Set("llm.api_key", opts.APIKey)
	}
	if opts.BaseURL != "" {
		v.Set("llm.base_url", opts.BaseURL)
	}

	return Config{LLM: LLMConfig{
		Provider: v.GetString("llm.provider"),
		Model:    v.GetString("llm.model"),
		APIKey:   v.GetString("llm.api_key"),
		BaseURL:  v.GetString("llm.base_url"),
	}}, nil
}
