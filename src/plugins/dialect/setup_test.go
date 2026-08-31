package dialect

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"awaken/src/plugins/ir"
)

func setupTestMCPComponent() ir.Component {
	return ir.Component{
		LogicalID: "mcp_server/mail", Kind: ir.KindMCPServer, Mapping: ir.MappingAdapted, Validation: "valid",
		SourceDialect: ir.DialectCursor, MCP: &ir.MCPSpec{ShortName: "mail", Transport: "streamable-http", URL: "https://mail.example.test/mcp"},
	}
}

func TestSetupDocumentNormalizesOAuthBindingAndFingerprint(t *testing.T) {
	component := setupTestMCPComponent()
	raw := []byte(`{
  "version": 1,
  "requirements": [{
    "id": "mail-account",
    "kind": "oauth_pkce",
    "title": "Mail account",
    "required": true,
    "componentIds": ["mcp_server/mail"],
    "oauth": {
      "authorizationEndpoint": "https://login.example.test/authorize",
      "tokenEndpoint": "https://login.example.test/token",
      "clientId": "public-desktop-client",
      "scopes": ["mail.read"]
    },
    "bindings": [{"componentId":"mcp_server/mail","target":"mcp_transport"}]
  }]
}`)
	requirements, _, err := parseSetupDocument(raw, ".awaken/setup.json", ir.DialectCursor, []ir.Component{component})
	if err != nil {
		t.Fatal(err)
	}
	plugin := &ir.NormalizedPlugin{Components: []ir.Component{component}, Setup: requirements}
	if err := finalizeSetupRequirements(plugin); err != nil {
		t.Fatal(err)
	}
	got := plugin.Setup[0]
	if got.Fingerprint == "" || got.MCPServerName != "mail" || len(got.Bindings) != 1 {
		t.Fatalf("setup requirement was not normalized: %+v", got)
	}
}

func TestSetupDeclarationConflictFailsInstallationParsing(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".awaken"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".awaken", "setup.json"), []byte(`{"version":1,"requirements":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	manifest := map[string]any{"extensions": map[string]any{awakenSetupExtension: map[string]any{"version": float64(1), "requirements": []any{}}}}
	err := appendSetupDeclarations(root, manifest, "plugin.json", ir.DialectAgentPlugins, &ir.NormalizedPlugin{})
	if err == nil || !strings.Contains(err.Error(), "declaration conflict") {
		t.Fatalf("expected extension/sidecar conflict, got %v", err)
	}
}

func TestSetupRejectsPackagedClientSecretAndPublicHTTP(t *testing.T) {
	for name, raw := range map[string]string{
		"client secret": `{"version":1,"requirements":[{"id":"oauth","kind":"oauth_pkce","oauth":{"authorizationEndpoint":"https://login.example.test/auth","tokenEndpoint":"https://login.example.test/token","clientId":"public","clientSecret":"must-not-ship"}}]}`,
		"public http":   `{"version":1,"requirements":[{"id":"external","kind":"external_setup","externalUrl":"http://example.test/setup"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := parseSetupDocument([]byte(raw), ".awaken/setup.json", ir.DialectCursor, nil); err == nil {
				t.Fatal("expected unsafe setup declaration to fail")
			}
		})
	}
}

func TestSetupDocumentAcceptsAwakenHostConnectorReference(t *testing.T) {
	component := ir.Component{
		LogicalID: "skill/skills/outlook-email", Kind: ir.KindSkill,
		Mapping: ir.MappingNative, Validation: "valid", Skill: &ir.SkillSpec{Path: "skills/outlook-email"},
	}
	raw := []byte(`{"version":1,"requirements":[{"id":"outlook-account","kind":"host_connector","title":"Connect Outlook","required":true,"connectorId":"awaken.microsoft.outlook","componentIds":["skill/skills/outlook-email"]}]}`)
	requirements, _, err := parseSetupDocument(raw, ".awaken/setup.json", ir.DialectAgentPlugins, []ir.Component{component})
	if err != nil {
		t.Fatal(err)
	}
	plugin := &ir.NormalizedPlugin{Components: []ir.Component{component}, Setup: requirements}
	if err := finalizeSetupRequirements(plugin); err != nil {
		t.Fatal(err)
	}
	if got := plugin.Setup[0]; got.Kind != ir.SetupHostConnector || got.ConnectorID != "awaken.microsoft.outlook" || got.Fingerprint == "" {
		t.Fatalf("host connector setup was not normalized: %+v", got)
	}
	if _, _, err := parseSetupDocument([]byte(`{"version":1,"requirements":[{"id":"missing","kind":"host_connector"}]}`), ".awaken/setup.json", ir.DialectAgentPlugins, nil); err == nil {
		t.Fatal("host_connector without connectorId should fail")
	}
}
