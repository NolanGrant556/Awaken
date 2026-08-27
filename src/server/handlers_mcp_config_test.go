package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"awaken/src/mcpclient"
)

func TestParseMCPServersConfigAcceptsStreamableHTTPAlias(t *testing.T) {
	configs, err := parseMCPServersConfig(`{
		"mcpServers": {
			"remote-example": {
				"type": "streamable-http",
				"url": "https://mcp.example.test/mcp",
				"headers": {"X-Test": "placeholder"}
			}
		}
	}`)
	if err != nil {
		t.Fatalf("parse Streamable HTTP alias: %v", err)
	}

	cfg, ok := configs["remote_example"]
	if !ok {
		t.Fatalf("normalized config missing: %+v", configs)
	}
	if cfg.TransportKind() != mcpclient.TransportHTTP || cfg.HTTP == nil {
		t.Fatalf("expected HTTP transport, got kind=%q config=%+v", cfg.TransportKind(), cfg)
	}
	if cfg.HTTP.URL != "https://mcp.example.test/mcp" {
		t.Fatalf("unexpected endpoint: %q", cfg.HTTP.URL)
	}
	if cfg.HTTP.Headers["X-Test"] != "placeholder" {
		t.Fatalf("headers were not preserved: %+v", cfg.HTTP.Headers)
	}
}

func TestParseMCPServersConfigAcceptsUnicodeOnlyServerName(t *testing.T) {
	configs, err := parseMCPServersConfig(`{
		"mcpServers": {
			"钉钉文档": {
				"type": "streamable-http",
				"url": "https://mcp.example.test/mcp"
			}
		}
	}`)
	if err != nil {
		t.Fatalf("parse Unicode server name: %v", err)
	}
	if len(configs) != 1 {
		t.Fatalf("expected one config, got %+v", configs)
	}
	for name, cfg := range configs {
		if !strings.HasPrefix(name, "server_") {
			t.Fatalf("expected stable safe fallback name, got %q", name)
		}
		if cfg.DisplayName != "钉钉文档" {
			t.Fatalf("expected original display name, got %q", cfg.DisplayName)
		}
		if cfg.TransportKind() != mcpclient.TransportHTTP || cfg.HTTP == nil {
			t.Fatalf("expected HTTP transport, got %+v", cfg)
		}
	}
}

func TestParseMCPServersConfigStillRejectsPunctuationOnlyServerName(t *testing.T) {
	_, err := parseMCPServersConfig(`{"mcpServers":{"---":{"url":"https://mcp.example.test/mcp"}}}`)
	if err == nil || !strings.Contains(err.Error(), "server name") {
		t.Fatalf("expected invalid server name error, got %v", err)
	}
}

func TestParseMCPServersConfigAcceptsBareStreamableHTTPURL(t *testing.T) {
	const endpoint = "https://mcp-gw.example.test/server/sample?key=placeholder"
	configs, err := parseMCPServersConfig(endpoint)
	if err != nil {
		t.Fatalf("parse bare Streamable HTTP URL: %v", err)
	}
	if len(configs) != 1 {
		t.Fatalf("expected one config, got %+v", configs)
	}
	for name, cfg := range configs {
		if !strings.HasPrefix(name, "remote_") {
			t.Fatalf("expected stable remote name, got %q", name)
		}
		if cfg.DisplayName != "mcp-gw.example.test" {
			t.Fatalf("expected host display name, got %q", cfg.DisplayName)
		}
		if cfg.HTTP == nil || cfg.HTTP.URL != endpoint {
			t.Fatalf("expected endpoint to be preserved, got %+v", cfg.HTTP)
		}
	}
}

func TestParseMCPServersConfigBareURLNameIgnoresQueryCredentials(t *testing.T) {
	first, err := parseMCPServersConfig("https://mcp.example.test/server/sample?key=first")
	if err != nil {
		t.Fatalf("parse first URL: %v", err)
	}
	second, err := parseMCPServersConfig("https://mcp.example.test/server/sample?key=rotated")
	if err != nil {
		t.Fatalf("parse rotated URL: %v", err)
	}
	var firstName, secondName string
	for firstName = range first {
	}
	for secondName = range second {
	}
	if firstName != secondName {
		t.Fatalf("query credential rotation changed identity: %q != %q", firstName, secondName)
	}
}

func TestParseMCPServersConfigKeepsHTTPAsCanonicalStreamableTransport(t *testing.T) {
	for _, configText := range []string{
		`{"mcpServers":{"remote":{"type":"http","url":"https://mcp.example.test/mcp"}}}`,
		`{"mcpServers":{"remote":{"url":"https://mcp.example.test/mcp"}}}`,
	} {
		configs, err := parseMCPServersConfig(configText)
		if err != nil {
			t.Fatalf("parse HTTP shorthand %s: %v", configText, err)
		}
		if got := configs["remote"].TransportKind(); got != mcpclient.TransportHTTP {
			t.Fatalf("expected HTTP transport, got %q", got)
		}
	}
}

func TestParseMCPServersConfigListsSupportedRemoteTransports(t *testing.T) {
	_, err := parseMCPServersConfig(
		`{"mcpServers":{"remote":{"type":"socket","url":"https://mcp.example.test/mcp"}}}`,
	)
	if err == nil {
		t.Fatal("expected unsupported transport error")
	}
	for _, expected := range []string{"streamable-http", "http", "sse"} {
		if !strings.Contains(err.Error(), expected) {
			t.Fatalf("error %q does not list %q", err, expected)
		}
	}
}

func TestConfigureMCPServersPersistsStreamableHTTPAliasAsCanonicalHTTP(t *testing.T) {
	awakenHome := t.TempDir()
	t.Setenv("AWAKEN_HOME", awakenHome)
	mgr := mcpclient.NewManager(mcpclient.WithLogger(testLogger{}))
	router := NewRouterWithWorkspace(nil, nil, mgr, nil, nil, "")
	configText := `{"mcpServers":{"remote-example":{"type":"streamable-http","url":"http://127.0.0.1:1/mcp"}}}`
	body, err := json.Marshal(configureMCPServersRequest{ConfigText: configText})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/mcp/servers", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	storedConfig, err := mcpclient.LoadUserConfig()
	if err != nil {
		t.Fatalf("load persisted user config: %v", err)
	}
	stored := storedConfig.Servers["remote_example"]
	if stored.HTTP == nil || stored.HTTP.URL != "http://127.0.0.1:1/mcp" {
		t.Fatalf("expected canonical HTTP config, got %+v", stored)
	}
	encoded, err := json.Marshal(storedConfig)
	if err != nil {
		t.Fatalf("marshal persisted user config: %v", err)
	}
	if strings.Contains(string(encoded), "streamable-http") {
		t.Fatalf("persisted config retained input alias: %s", encoded)
	}
}

func TestConfigureMCPServersPersistsBareURLAsCanonicalHTTP(t *testing.T) {
	awakenHome := t.TempDir()
	t.Setenv("AWAKEN_HOME", awakenHome)
	mgr := mcpclient.NewManager(mcpclient.WithLogger(testLogger{}))
	router := NewRouterWithWorkspace(nil, nil, mgr, nil, nil, "")
	const endpoint = "http://127.0.0.1:1/mcp?key=placeholder"
	body, err := json.Marshal(configureMCPServersRequest{ConfigText: endpoint})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/mcp/servers", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	storedConfig, err := mcpclient.LoadUserConfig()
	if err != nil {
		t.Fatalf("load persisted user config: %v", err)
	}
	if len(storedConfig.Servers) != 1 {
		t.Fatalf("expected one persisted server, got %+v", storedConfig.Servers)
	}
	for name, stored := range storedConfig.Servers {
		if !strings.HasPrefix(name, "remote_") {
			t.Fatalf("expected generated remote identity, got %q", name)
		}
		if stored.DisplayName != "127.0.0.1" {
			t.Fatalf("expected endpoint host display name, got %q", stored.DisplayName)
		}
		if stored.HTTP == nil || stored.HTTP.URL != endpoint {
			t.Fatalf("expected canonical HTTP config, got %+v", stored)
		}
	}
}
