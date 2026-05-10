package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

func TestFindCopilotOAuthTokenFromHostsJSON(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "github-copilot")
	if err := os.MkdirAll(cfg, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg, "hosts.json"), []byte(`{"github.com":{"oauth_token":"host-token"}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	token, err := FindCopilotOAuthToken(dir)
	if err != nil {
		t.Fatal(err)
	}
	if token != "host-token" {
		t.Fatalf("token = %q", token)
	}
}

func TestExchangeCopilotToken(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		if r.Header.Get("Authorization") != "Bearer oauth-token" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"token":"copilot-token","expires_at":4102444800,"endpoints":{"api":"https://copilot.test"}}`))
	}))
	t.Cleanup(server.Close)

	resolver := NewCopilotResolver(ResolverOptions{TokenURL: server.URL, HTTPClient: server.Client()})
	tok, err := resolver.Exchange(t.Context(), "oauth-token")
	if err != nil {
		t.Fatal(err)
	}
	if tok.Token != "copilot-token" || tok.ChatEndpoint != "https://copilot.test/chat/completions" {
		t.Fatalf("bad token: %+v", tok)
	}

	_, err = resolver.Exchange(t.Context(), "oauth-token")
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("expected cached token, calls = %d", calls)
	}
}

func TestListCopilotModelsFiltersAndCachesCheapestChatModel(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		if r.URL.Path != "/models" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.Header.Get("Copilot-Integration-Id") != "vscode-chat" {
			t.Fatalf("missing copilot header")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{
			{"id": "expensive", "model_picker_enabled": true, "capabilities": map[string]any{"type": "chat"}, "billing": map[string]any{"multiplier": 2}},
			{"id": "cheap", "model_picker_enabled": true, "capabilities": map[string]any{"type": "chat"}, "billing": map[string]any{"multiplier": 1}},
			{"id": "disabled", "model_picker_enabled": false, "capabilities": map[string]any{"type": "chat"}},
			{"id": "embed", "model_picker_enabled": true, "capabilities": map[string]any{"type": "embedding"}},
		}})
	}))
	t.Cleanup(server.Close)

	resolver := NewCopilotResolver(ResolverOptions{HTTPClient: server.Client()})
	models, err := resolver.ListModels(t.Context(), CopilotToken{Token: "token", ChatEndpoint: server.URL + "/chat/completions"})
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 || models[0].ID != "cheap" || models[1].ID != "expensive" {
		t.Fatalf("models = %+v", models)
	}
	model, err := resolver.SelectModel(t.Context(), CopilotToken{Token: "token", ChatEndpoint: server.URL + "/chat/completions"})
	if err != nil {
		t.Fatal(err)
	}
	if model != "cheap" {
		t.Fatalf("model = %q", model)
	}
	_, _ = resolver.ListModels(t.Context(), CopilotToken{Token: "token", ChatEndpoint: server.URL + "/chat/completions"})
	if calls != 1 {
		t.Fatalf("expected cached models, calls = %d", calls)
	}
}
