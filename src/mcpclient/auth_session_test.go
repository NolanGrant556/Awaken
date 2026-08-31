package mcpclient

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	sdkauth "github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestCoreOwnedMCPOAuthSessionConnectsWithoutInstalledFetcher(t *testing.T) {
	mcpHandler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		server := mcp.NewServer(&mcp.Implementation{Name: "oauth-mcp", Version: "v1"}, nil)
		mcp.AddTool(server, &mcp.Tool{Name: "ping", Description: "ping", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}}, func(context.Context, *mcp.CallToolRequest, map[string]string) (*mcp.CallToolResult, map[string]string, error) {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "pong"}}}, nil, nil
		})
		return server
	}, nil)

	var origin string
	mux := http.NewServeMux()
	mux.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer access-token" {
			w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+origin+`/.well-known/oauth-protected-resource/mcp"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		mcpHandler.ServeHTTP(w, r)
	})
	mux.HandleFunc("/.well-known/oauth-protected-resource/mcp", func(w http.ResponseWriter, _ *http.Request) {
		writeTestJSON(w, map[string]any{"resource": origin + "/mcp", "authorization_servers": []string{origin}, "scopes_supported": []string{"mcp.read"}})
	})
	mux.HandleFunc("/.well-known/oauth-authorization-server", func(w http.ResponseWriter, _ *http.Request) {
		writeTestJSON(w, map[string]any{
			"issuer": origin, "authorization_endpoint": origin + "/authorize", "token_endpoint": origin + "/token",
			"registration_endpoint": origin + "/register", "response_types_supported": []string{"code"},
			"grant_types_supported": []string{"authorization_code"}, "token_endpoint_auth_methods_supported": []string{"none"},
			"code_challenge_methods_supported": []string{"S256"},
		})
	})
	mux.HandleFunc("/register", func(w http.ResponseWriter, r *http.Request) {
		var metadata map[string]any
		if err := json.NewDecoder(r.Body).Decode(&metadata); err != nil || metadata["redirect_uris"] == nil {
			http.Error(w, "bad registration", http.StatusBadRequest)
			return
		}
		grantTypes, _ := metadata["grant_types"].([]any)
		if metadata["token_endpoint_auth_method"] != "none" || metadata["scope"] != "mcp.read" || len(grantTypes) != 2 {
			http.Error(w, "desktop client must register as public native PKCE client", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusCreated)
		writeTestJSON(w, map[string]any{"client_id": "awaken-test", "redirect_uris": metadata["redirect_uris"], "token_endpoint_auth_method": "none"})
	})
	mux.HandleFunc("/authorize", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil || r.Form.Get("code") != "approved" || r.Form.Get("code_verifier") == "" {
			http.Error(w, "bad token request", http.StatusBadRequest)
			return
		}
		writeTestJSON(w, map[string]any{"access_token": "access-token", "token_type": "Bearer", "expires_in": 3600})
	})
	httpServer := httptest.NewServer(mux)
	origin = httpServer.URL
	defer httpServer.Close()

	manager := NewManager(WithLogger(noopLogger{}))
	defer manager.Shutdown(context.Background())
	manager.SetCredentialStore(NewInMemoryCredentialStore())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	manager.Start(ctx, []ScopedMcpServerConfig{{Name: "oauth", Scope: ScopePlugin, Config: McpServerConfig{HTTP: &HTTPTransportConfig{URL: origin + "/mcp"}, OAuthResource: origin + "/mcp"}}})
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := manager.SnapshotServer("oauth"); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	started, err := manager.StartAuth(ctx, "oauth", AuthRequest{})
	if err != nil {
		t.Fatal(err)
	}
	authorizationURL, err := url.Parse(started.AuthorizationURL)
	if err != nil {
		t.Fatal(err)
	}
	callback := authorizationURL.Query().Get("redirect_uri")
	state := authorizationURL.Query().Get("state")
	if callback == "" || state == "" {
		t.Fatalf("authorization URL incomplete: %s", started.AuthorizationURL)
	}
	response, err := http.Get(callback + "?code=approved&state=" + url.QueryEscape(state))
	if err != nil {
		t.Fatal(err)
	}
	callbackBody, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("callback status=%d body=%s auth=%s", response.StatusCode, callbackBody, started.AuthorizationURL)
	}
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		view, viewErr := manager.AuthSession(started.SessionID)
		if viewErr == nil && view.Status == "ready" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := waitForState(manager, "oauth", StateConnected, 5*time.Second); err != nil {
		view, _ := manager.AuthSession(started.SessionID)
		snapshot, _ := manager.SnapshotServer("oauth")
		t.Fatalf("authorized server did not connect: %v view=%+v snapshot=%+v", err, view, snapshot)
	}
	result, err := manager.CallTool(ctx, "mcp__oauth__ping", map[string]any{})
	if err != nil || len(result.Content) == 0 || !strings.Contains(result.Content[0].(*mcp.TextContent).Text, "pong") {
		t.Fatalf("authenticated call failed: result=%+v err=%v", result, err)
	}
}

func TestOAuthHandlerUsesAdvertisedCIMDWithoutDynamicRegistration(t *testing.T) {
	var origin string
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/oauth-protected-resource/mcp", func(w http.ResponseWriter, _ *http.Request) {
		writeTestJSON(w, map[string]any{"resource": origin + "/mcp", "authorization_servers": []string{origin}, "scopes_supported": []string{"mcp.read"}})
	})
	mux.HandleFunc("/.well-known/oauth-authorization-server", func(w http.ResponseWriter, _ *http.Request) {
		writeTestJSON(w, map[string]any{
			"issuer": origin, "authorization_endpoint": origin + "/authorize", "token_endpoint": origin + "/token",
			"client_id_metadata_document_supported": true, "token_endpoint_auth_methods_supported": []string{"none"},
			"response_types_supported": []string{"code"}, "grant_types_supported": []string{"authorization_code"}, "code_challenge_methods_supported": []string{"S256"},
		})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil || r.Form.Get("client_id") != "https://client.example/awaken.json" || r.Form.Get("code_verifier") == "" {
			http.Error(w, "bad CIMD token request", http.StatusBadRequest)
			return
		}
		writeTestJSON(w, map[string]any{"access_token": "cimd-token", "token_type": "Bearer", "expires_in": 3600})
	})
	server := httptest.NewServer(mux)
	origin = server.URL
	defer server.Close()

	handler, err := buildOAuthHandler(AuthFlowConfig{
		ServerURL: origin + "/mcp", RedirectURL: "http://127.0.0.1:43123/callback",
		ClientMetadataURL: "https://client.example/awaken.json",
		AuthorizationCodeFetcher: func(_ context.Context, args *sdkauth.AuthorizationArgs) (*sdkauth.AuthorizationResult, error) {
			parsed, parseErr := url.Parse(args.URL)
			if parseErr != nil || parsed.Query().Get("client_id") != "https://client.example/awaken.json" {
				t.Fatalf("CIMD client id missing from authorization URL: %s", args.URL)
			}
			return &sdkauth.AuthorizationResult{Code: "approved", State: parsed.Query().Get("state")}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodPost, origin+"/mcp", nil)
	resp := &http.Response{StatusCode: http.StatusUnauthorized, Header: make(http.Header), Body: http.NoBody, Request: req}
	resp.Header.Set("WWW-Authenticate", `Bearer resource_metadata="`+origin+`/.well-known/oauth-protected-resource/mcp"`)
	if err := handler.Authorize(context.Background(), req, resp); err != nil {
		t.Fatal(err)
	}
}

func writeTestJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

type runtimeSecretResolverFixture map[string]string

func (f runtimeSecretResolverFixture) ResolveMCPSecret(_ context.Context, ref string) (string, error) {
	return f[ref], nil
}

func TestResolveRuntimeSecretsKeepsOriginalConfigOpaque(t *testing.T) {
	original := McpServerConfig{
		Stdio:          &StdioTransportConfig{Command: "mail-mcp", Env: map[string]string{"SAFE": "visible"}},
		SecretBindings: []RuntimeSecretBinding{{Target: "process_env", Name: "MAIL_TOKEN", Ref: "opaque-ref"}},
	}
	resolved, err := resolveRuntimeSecrets(context.Background(), "mail", original, runtimeSecretResolverFixture{"opaque-ref": "secret-value"})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Stdio.Env["MAIL_TOKEN"] != "secret-value" || original.Stdio.Env["MAIL_TOKEN"] != "" {
		t.Fatalf("runtime secret resolution mutated the stored config: original=%+v resolved=%+v", original.Stdio.Env, resolved.Stdio.Env)
	}
	encoded, _ := json.Marshal(original)
	if strings.Contains(string(encoded), "opaque-ref") || strings.Contains(string(encoded), "secret-value") {
		t.Fatalf("opaque runtime binding leaked through config JSON: %s", encoded)
	}
}
