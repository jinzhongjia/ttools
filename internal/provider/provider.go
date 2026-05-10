package provider

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"os"
	"strings"

	"charm.land/fantasy"
	"charm.land/fantasy/providers/openaicompat"

	"ttools/internal/ai"
	"ttools/internal/auth"
	"ttools/internal/config"
	gitx "ttools/internal/git"
	"ttools/internal/prompt"
)

type CopilotAuth interface {
	Exchange(ctx context.Context, oauthToken string) (auth.CopilotToken, error)
}

type CopilotModelSelector interface {
	SelectModel(ctx context.Context, token auth.CopilotToken) (string, error)
}

type Factory struct {
	copilotAuth CopilotAuth
}

func NewFactory(copilotAuth CopilotAuth) Factory {
	return Factory{copilotAuth: copilotAuth}
}

func (f Factory) New(cfg config.LLMConfig) (ai.Client, error) {
	switch strings.ToLower(cfg.Provider) {
	case "", "openai":
		return newOpenAIClient(cfg), nil
	case "copilot":
		return newCopilotClient(cfg, f.copilotAuth), nil
	default:
		return nil, fmt.Errorf("unsupported llm provider %q", cfg.Provider)
	}
}

type fantasyClient struct {
	provider    string
	model       string
	apiKey      string
	baseURL     string
	headers     map[string]string
	copilotAuth CopilotAuth
	oauthToken  string
}

func newOpenAIClient(cfg config.LLMConfig) ai.Client {
	return &fantasyClient{provider: "openai", model: cfg.Model, apiKey: cfg.APIKey, baseURL: cfg.BaseURL}
}

func newCopilotClient(cfg config.LLMConfig, resolver CopilotAuth) ai.Client {
	return &fantasyClient{provider: "copilot", model: cfg.Model, copilotAuth: resolver, oauthToken: cfg.APIKey}
}

func (c *fantasyClient) SummarizeFileDiff(ctx context.Context, fd gitx.FileDiff) (string, error) {
	return c.generate(ctx, prompt.BuildFileSummaryPrompt(fd))
}

func (c *fantasyClient) GenerateCommitMessage(ctx context.Context, input ai.FinalInput) (string, error) {
	converted := prompt.CommitPromptInput{TotalFiles: input.TotalFiles, Additions: input.Additions, Deletions: input.Deletions}
	for _, fs := range input.Summaries {
		converted.FileSummaries = append(converted.FileSummaries, prompt.FileSummary{Path: fs.Path, Status: fs.Status, Summary: fs.Summary})
	}
	return c.generate(ctx, prompt.BuildCommitMessagePrompt(converted))
}

func (c *fantasyClient) generate(ctx context.Context, userPrompt string) (string, error) {
	baseURL := c.baseURL
	apiKey := c.apiKey
	headers := map[string]string{}
	maps.Copy(headers, c.headers)

	if c.provider == "copilot" {
		if c.copilotAuth == nil {
			return "", errors.New("copilot auth resolver is required")
		}
		if c.oauthToken == "" {
			configDir, err := os.UserConfigDir()
			if err != nil {
				return "", err
			}
			token, err := auth.FindCopilotOAuthToken(configDir)
			if err != nil {
				return "", err
			}
			c.oauthToken = token
		}
		tok, err := c.copilotAuth.Exchange(ctx, c.oauthToken)
		if err != nil {
			return "", err
		}
		apiKey = tok.Token
		baseURL = strings.TrimSuffix(tok.ChatEndpoint, "/chat/completions")
		if c.model == "" {
			if selector, ok := c.copilotAuth.(CopilotModelSelector); ok {
				if selected, err := selector.SelectModel(ctx, tok); err == nil && selected != "" {
					c.model = selected
				} else {
					c.model = "gpt-4.1"
				}
			} else {
				c.model = "gpt-4.1"
			}
		}
		headers["Copilot-Integration-Id"] = "vscode-chat"
		headers["X-Github-Api-Version"] = "2025-10-01"
	}

	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	if c.model == "" {
		c.model = "gpt-4.1-mini"
	}

	p, err := openaicompat.New(
		openaicompat.WithBaseURL(baseURL),
		openaicompat.WithAPIKey(apiKey),
		openaicompat.WithHeaders(headers),
		openaicompat.WithHTTPClient(http.DefaultClient),
	)
	if err != nil {
		return "", err
	}
	model, err := p.LanguageModel(ctx, c.model)
	if err != nil {
		return "", err
	}
	resp, err := model.Generate(ctx, fantasy.Call{Prompt: fantasy.Prompt{fantasy.NewUserMessage(userPrompt)}})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(resp.Content.Text()), nil
}
