package mcpclient

import (
	"errors"
	"strings"
	"testing"
)

func TestBuildToolNameNormalizesAndCapsProviderSafeName(t *testing.T) {
	name := BuildToolName("github.prod/中国 with spaces", strings.Repeat("search issues with spaces/", 8))
	if !IsProviderSafeName(name) {
		t.Fatalf("expected provider-safe name, got %q len=%d", name, len(name))
	}
	if !strings.HasPrefix(name, "mcp__") {
		t.Fatalf("expected MCP prefix, got %q", name)
	}
	if strings.ContainsAny(name, " ./") {
		t.Fatalf("expected unsafe separators to be normalized, got %q", name)
	}
}

func TestToolDefinitionCapsDescriptionAndConvertsSchema(t *testing.T) {
	longDescription := strings.Repeat("a", MaxDescriptionLength+100)
	definition := ToolDefinitionForRemote("docs.server", RemoteTool{
		Name:        "lookup document",
		Description: longDescription,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query text": map[string]any{
					"type":        "string",
					"description": "Search query",
				},
				"limit": map[string]any{
					"type": "integer",
				},
			},
			"required": []any{"query text"},
		},
	}, ServerConfig{Name: "docs.server", Transport: TransportHTTP})

	if len([]rune(definition.Function.Description)) != MaxDescriptionLength {
		t.Fatalf("expected capped description length %d, got %d", MaxDescriptionLength, len([]rune(definition.Function.Description)))
	}
	if !strings.HasSuffix(definition.Function.Description, truncatedSuffix) {
		t.Fatalf("expected truncated suffix, got %q", definition.Function.Description[len(definition.Function.Description)-20:])
	}
	if !IsProviderSafeName(definition.Function.Name) {
		t.Fatalf("expected provider-safe function name, got %q", definition.Function.Name)
	}
	if _, ok := definition.Function.Parameters.Properties["query_text"]; !ok {
		t.Fatalf("expected normalized parameter name, got %+v", definition.Function.Parameters.Properties)
	}
	if len(definition.Function.Parameters.Required) != 1 || definition.Function.Parameters.Required[0] != "query_text" {
		t.Fatalf("expected normalized required parameter, got %+v", definition.Function.Parameters.Required)
	}
}

func TestToolDefinitionKeepsCollidingNormalizedParametersDistinct(t *testing.T) {
	definition := ToolDefinitionForRemote("docs", RemoteTool{
		Name: "lookup",
		InputSchema: map[string]any{
			"properties": map[string]any{
				"query text": map[string]any{"type": "string"},
				"query_text": map[string]any{"type": "integer"},
			},
			"required": []any{"query text", "query_text"},
		},
	}, ServerConfig{Name: "docs", Transport: TransportHTTP})

	properties := definition.Function.Parameters.Properties
	if len(properties) != 2 {
		t.Fatalf("expected two distinct parameters after normalization, got %+v", properties)
	}
	if _, ok := properties["query_text"]; !ok {
		t.Fatalf("expected base safe parameter name, got %+v", properties)
	}

	required := definition.Function.Parameters.Required
	if len(required) != 2 {
		t.Fatalf("expected both colliding parameters to remain required, got %+v", required)
	}
	if required[0] == required[1] {
		t.Fatalf("expected distinct required names, got %+v", required)
	}

	nameMap := ParameterNameMap(map[string]any{
		"properties": map[string]any{
			"query text": map[string]any{"type": "string"},
			"query_text": map[string]any{"type": "integer"},
		},
	})
	if len(nameMap) != 2 {
		t.Fatalf("expected both remapped colliding parameters, got %+v", nameMap)
	}
}

func TestStateTransitionsAndErrorStatusMapping(t *testing.T) {
	if got := NextStatus(StatusPending, EventConnectSucceeded); got != StatusConnected {
		t.Fatalf("expected connected, got %s", got)
	}
	if got := NextStatus(StatusConnected, EventSessionExpired); got != StatusPending {
		t.Fatalf("expected pending after session expiry, got %s", got)
	}
	if got := NextStatus(StatusDisabled, EventConnectSucceeded); got != StatusDisabled {
		t.Fatalf("expected disabled to stay disabled, got %s", got)
	}

	config := ServerConfig{Name: "github", Transport: TransportHTTP}
	authState := StateFromError("github", config, errors.New("401 unauthorized: token expired"))
	if authState.Status != StatusNeedsAuth || authState.ErrorClass != ErrorAuth {
		t.Fatalf("expected needs_auth auth state, got %+v", authState)
	}
	sessionState := StateFromError("github", config, errors.New(`404 {"error":{"code":-32001,"message":"Session not found"}}`))
	if sessionState.Status != StatusPending || sessionState.ErrorClass != ErrorSessionExpired {
		t.Fatalf("expected pending session-expired state, got %+v", sessionState)
	}
	configState := StateFromError("github", config, errors.New("invalid config: missing command"))
	if configState.Status != StatusFailed || configState.ErrorClass != ErrorConfig {
		t.Fatalf("expected failed config state, got %+v", configState)
	}
}

func TestClassifyAuthAndSessionErrors(t *testing.T) {
	if got := ClassifyError(errors.Join(ErrAuthenticationRequired, errors.New("provider rejected request"))); got != ErrorAuth {
		t.Fatalf("expected auth classification, got %s", got)
	}
	if got := ClassifyError(errors.New(`{"error":{"code": -32001, "message":"Session not found"}}`)); got != ErrorSessionExpired {
		t.Fatalf("expected session_expired classification, got %s", got)
	}
	if got := ClassifyError(errors.New("dial tcp: connection refused")); got != ErrorTransient {
		t.Fatalf("expected transient classification, got %s", got)
	}
}

func TestSecretRedactionDoesNotLeakHeaderOrEnvValues(t *testing.T) {
	config := ServerConfig{
		Name:      "private",
		Transport: TransportHTTP,
		Stdio: &StdioConfig{Env: map[string]string{
			"API_KEY": "env-secret-value",
		}},
		HTTP: &RemoteConfig{
			URL: "https://example.test/mcp?token=url-secret",
			Headers: map[string]string{
				"Authorization": "Bearer header-secret-value",
				"X-API-Key":     "x-api-secret",
			},
			OAuth: &OAuthConfig{ClientSecret: "oauth-secret"},
		},
	}
	message := "request failed Authorization: Bearer header-secret-value API_KEY=env-secret-value token=url-secret x-api-secret oauth-secret"
	state := StateFromError("private", config, errors.New(message))

	for _, secret := range []string{"header-secret-value", "env-secret-value", "url-secret", "x-api-secret", "oauth-secret"} {
		if strings.Contains(state.Message, secret) {
			t.Fatalf("state message leaked secret %q: %q", secret, state.Message)
		}
	}
	if strings.Contains(state.Message, "Bearer header") {
		t.Fatalf("expected bearer token redaction, got %q", state.Message)
	}
}

func TestRegistryRegistersStaticDescriptorAndListsConnectedTools(t *testing.T) {
	registry := NewRegistry()
	err := registry.RegisterServer(ServerDescriptor{
		Config: ServerConfig{
			Name:      "github",
			Transport: TransportHTTP,
			HTTP:      &RemoteConfig{URL: "https://mcp.example.test?token=secret"},
		},
		Tools: []RemoteTool{{
			Name:        "search issues",
			Description: "Search issues",
			InputSchema: map[string]any{
				"properties": map[string]any{
					"query": map[string]any{"type": "string"},
				},
			},
		}},
	})
	if err != nil {
		t.Fatalf("register server: %v", err)
	}

	servers := registry.ListServers()
	if len(servers) != 1 || servers[0].Status != StatusConnected {
		t.Fatalf("expected connected static server, got %+v", servers)
	}
	definitions := registry.ToolDefinitions()
	if len(definitions) != 1 {
		t.Fatalf("expected one tool definition, got %+v", definitions)
	}
	if definitions[0].Function.Name != "mcp__github__search_issues" {
		t.Fatalf("unexpected tool name %q", definitions[0].Function.Name)
	}
}
