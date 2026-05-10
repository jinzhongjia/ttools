package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	defaultCopilotTokenURL     = "https://api.github.com/copilot_internal/v2/token"
	defaultCopilotChatEndpoint = "https://api.githubcopilot.com/chat/completions"
	defaultCopilotModelsURL    = "https://api.githubcopilot.com/models"
)

type CopilotToken struct {
	Token        string
	ExpiresAt    time.Time
	ChatEndpoint string
}

type CopilotModel struct {
	ID         string
	Multiplier float64
}

type ResolverOptions struct {
	TokenURL   string
	HTTPClient *http.Client
}

type CopilotResolver struct {
	tokenURL    string
	client      *http.Client
	mu          sync.Mutex
	tokenCache  map[string]CopilotToken
	modelsCache map[string]cachedModels
}

type cachedModels struct {
	expires time.Time
	models  []CopilotModel
}

func NewCopilotResolver(opts ResolverOptions) *CopilotResolver {
	if opts.TokenURL == "" {
		opts.TokenURL = defaultCopilotTokenURL
	}
	if opts.HTTPClient == nil {
		opts.HTTPClient = http.DefaultClient
	}
	return &CopilotResolver{
		tokenURL:    opts.TokenURL,
		client:      opts.HTTPClient,
		tokenCache:  map[string]CopilotToken{},
		modelsCache: map[string]cachedModels{},
	}
}

func FindCopilotOAuthToken(configRoot string) (string, error) {
	if tok := os.Getenv("GITHUB_TOKEN"); tok != "" {
		return tok, nil
	}
	for _, name := range []string{"hosts.json", "apps.json"} {
		path := filepath.Join(configRoot, "github-copilot", name)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if token := findOAuthToken(data); token != "" {
			return token, nil
		}
	}
	return "", errors.New("copilot oauth token not found")
}

func findOAuthToken(data []byte) string {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return ""
	}
	return findTokenValue(v, false)
}

func findTokenValue(v any, githubScope bool) string {
	switch x := v.(type) {
	case map[string]any:
		if val, ok := x["oauth_token"].(string); ok && (githubScope || val != "") {
			return val
		}
		for k, child := range x {
			if tok := findTokenValue(child, githubScope || strings.Contains(k, "github.com")); tok != "" {
				return tok
			}
		}
	case []any:
		for _, child := range x {
			if tok := findTokenValue(child, githubScope); tok != "" {
				return tok
			}
		}
	}
	return ""
}

func (r *CopilotResolver) Exchange(ctx context.Context, oauthToken string) (CopilotToken, error) {
	r.mu.Lock()
	if tok, ok := r.tokenCache[oauthToken]; ok && time.Until(tok.ExpiresAt) > time.Minute {
		r.mu.Unlock()
		return tok, nil
	}
	r.mu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.tokenURL, nil)
	if err != nil {
		return CopilotToken{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+oauthToken)
	req.Header.Set("User-Agent", "ttools")

	resp, err := r.client.Do(req)
	if err != nil {
		return CopilotToken{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return CopilotToken{}, errors.New("copilot token exchange failed")
	}

	var body struct {
		Token     string            `json:"token"`
		ExpiresAt int64             `json:"expires_at"`
		Endpoints map[string]string `json:"endpoints"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return CopilotToken{}, err
	}
	endpoint := defaultCopilotChatEndpoint
	if api := body.Endpoints["api"]; api != "" {
		endpoint = strings.TrimRight(api, "/") + "/chat/completions"
	}
	tok := CopilotToken{Token: body.Token, ExpiresAt: time.Unix(body.ExpiresAt, 0), ChatEndpoint: endpoint}
	r.mu.Lock()
	r.tokenCache[oauthToken] = tok
	r.mu.Unlock()
	return tok, nil
}

func (r *CopilotResolver) ListModels(ctx context.Context, token CopilotToken) ([]CopilotModel, error) {
	modelsURL := modelsEndpoint(token.ChatEndpoint)
	r.mu.Lock()
	if cached, ok := r.modelsCache[modelsURL]; ok && time.Now().Before(cached.expires) {
		models := append([]CopilotModel(nil), cached.models...)
		r.mu.Unlock()
		return models, nil
	}
	r.mu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, modelsURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token.Token)
	req.Header.Set("Copilot-Integration-Id", "vscode-chat")
	req.Header.Set("X-Github-Api-Version", "2025-10-01")
	req.Header.Set("User-Agent", "ttools")

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, errors.New("copilot models request failed")
	}

	var body struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	models := filterCopilotModels(body.Data)
	r.mu.Lock()
	r.modelsCache[modelsURL] = cachedModels{expires: time.Now().Add(30 * time.Minute), models: append([]CopilotModel(nil), models...)}
	r.mu.Unlock()
	return models, nil
}

func (r *CopilotResolver) SelectModel(ctx context.Context, token CopilotToken) (string, error) {
	models, err := r.ListModels(ctx, token)
	if err != nil {
		return "", err
	}
	if len(models) == 0 {
		return "", errors.New("no copilot chat models available")
	}
	return models[0].ID, nil
}

func modelsEndpoint(chatEndpoint string) string {
	trimmed := strings.TrimRight(chatEndpoint, "/")
	if before, ok := strings.CutSuffix(trimmed, "/chat/completions"); ok {
		return before + "/models"
	}
	return defaultCopilotModelsURL
}

func filterCopilotModels(raw []map[string]any) []CopilotModel {
	models := make([]CopilotModel, 0, len(raw))
	for _, item := range raw {
		id, _ := item["id"].(string)
		if id == "" || !modelPickerEnabled(item) || !hasChatCapability(item) {
			continue
		}
		models = append(models, CopilotModel{ID: id, Multiplier: billingMultiplier(item)})
	}
	sort.SliceStable(models, func(i, j int) bool {
		return models[i].Multiplier < models[j].Multiplier
	})
	return models
}

func modelPickerEnabled(item map[string]any) bool {
	enabled, ok := item["model_picker_enabled"].(bool)
	return ok && enabled
}

func hasChatCapability(item map[string]any) bool {
	caps, _ := item["capabilities"].(map[string]any)
	typeValue := caps["type"]
	switch v := typeValue.(type) {
	case string:
		return strings.Contains(strings.ToLower(v), "chat")
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok && strings.Contains(strings.ToLower(s), "chat") {
				return true
			}
		}
	}
	return false
}

func billingMultiplier(item map[string]any) float64 {
	billing, _ := item["billing"].(map[string]any)
	switch v := billing["multiplier"].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	default:
		return 1
	}
}
