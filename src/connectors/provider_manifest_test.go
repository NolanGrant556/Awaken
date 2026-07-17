package connectors

import (
	"context"
	"errors"
	"testing"

	"awaken/src/mcpclient"
	"awaken/src/tool"
)

type providerTestSource struct {
	definition ConnectorDefinition
}

func (s providerTestSource) List(context.Context) ([]ConnectorDefinition, error) {
	return []ConnectorDefinition{s.definition}, nil
}

func (providerTestSource) Origin() SourceOrigin { return SourceLocal }
func (providerTestSource) Priority() int        { return 3 }

type providerTestRuntime struct {
	injected mcpclient.ScopedMcpServerConfig
	state    mcpclient.ServerState
}

func (r *providerTestRuntime) InjectDynamic(connectorID string, config mcpclient.ScopedMcpServerConfig) {
	r.injected = config
	r.state = mcpclient.StateConnected
}

func (r *providerTestRuntime) RemoveDynamic(string, string) {}
func (r *providerTestRuntime) Start(_ context.Context, configs []mcpclient.ScopedMcpServerConfig) {
	if len(configs) == 0 {
		return
	}
	r.injected = configs[0]
	r.state = mcpclient.StatePending
}
func (r *providerTestRuntime) Remove(string) error { return nil }
func (r *providerTestRuntime) Snapshot() []mcpclient.ServerSnapshot {
	if r.injected.Name == "" {
		return nil
	}
	return []mcpclient.ServerSnapshot{{Name: r.injected.Name, State: r.state, Scope: r.injected.Scope}}
}
func (r *providerTestRuntime) SnapshotServer(name string) (mcpclient.ServerSnapshot, error) {
	return mcpclient.ServerSnapshot{Name: name, State: r.state, Scope: r.injected.Scope}, nil
}
func (r *providerTestRuntime) ClearCredentialsFor(mcpclient.ScopedMcpServerConfig) error { return nil }

func TestConnectorProviderFactsPropagateToMCPAndReadiness(t *testing.T) {
	runtime := &providerTestRuntime{}
	registry := NewRegistry(runtime, nil)
	definition := ConnectorDefinition{
		ID:           "mail-local",
		DisplayName:  "Local Mail",
		Category:     CategoryChat,
		Provider:     "Mail",
		ProviderID:   "connector:mail-local",
		Capabilities: []tool.Capability{tool.CapabilityMessageSend, tool.CapabilityMessageSend},
		Dependencies: []tool.DependencyRequirement{{
			Kind: tool.DependencyOSExecutable, Name: "mail-cli", ProbeID: "executable.mail-cli",
		}},
		AuthType:     AuthNone,
		TrustLevel:   TrustUserLocal,
		SourceOrigin: SourceLocal,
		MCPRawConfig: &mcpclient.McpServerConfig{
			InProc: &mcpclient.InProcTransportConfig{ServerName: "mail-local"},
		},
	}
	if err := registry.LoadSources(context.Background(), []ConnectorSource{providerTestSource{definition: definition}}); err != nil {
		t.Fatalf("load source: %v", err)
	}
	if err := registry.Connect(context.Background(), definition.ID); err != nil {
		t.Fatalf("connect: %v", err)
	}
	provider := runtime.injected.Config.Provider
	if provider == nil || provider.ProviderID != definition.ProviderID || provider.Source != tool.SourceConnector {
		t.Fatalf("connector identity did not reach MCP server config: %+v", provider)
	}
	if len(provider.Dependencies) != 1 || provider.Dependencies[0].Name != "mail-cli" {
		t.Fatalf("provider dependencies did not reach MCP server config: %+v", provider.Dependencies)
	}

	status, err := registry.Status(definition.ID)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.ProviderID != definition.ProviderID || status.ProviderReadiness != tool.ProviderReady {
		t.Fatalf("provider identity/readiness missing from connector status: %+v", status)
	}
	if len(status.Capabilities) != 1 || status.Capabilities[0] != tool.CapabilityMessageSend {
		t.Fatalf("connector capabilities were not normalized: %+v", status.Capabilities)
	}
	runtime.state = mcpclient.StateNeedsAuth
	status, err = registry.Status(definition.ID)
	if err != nil || status.ProviderReadiness != tool.ProviderNeedsAuth {
		t.Fatalf("needs-auth provider readiness was not projected: status=%+v err=%v", status, err)
	}
}

func TestBuiltinConnectorDefinitionsDeclareProviderFacts(t *testing.T) {
	definitions, err := NewBuiltinSource().List(context.Background())
	if err != nil {
		t.Fatalf("list builtins: %v", err)
	}
	if len(definitions) != 7 {
		t.Fatalf("expected seven builtins, got %d", len(definitions))
	}
	for _, definition := range definitions {
		if definition.ProviderID == "" {
			t.Errorf("%s missing provider identity", definition.ID)
		}
		if len(definition.Dependencies) == 0 {
			t.Errorf("%s missing dependency manifest", definition.ID)
		}
	}
	for _, definition := range definitions {
		if definition.ID == "dingtalk-workspace" || definition.ID == "lark-cli" {
			if definition.Kind != KindCLI || definition.McpConfigFactory != nil {
				t.Errorf("%s must be a CLI-only connector", definition.ID)
			}
			if !tool.HasCapability(definition.Capabilities, tool.CapabilityMessageSend) {
				t.Errorf("%s missing message capability", definition.ID)
			}
		}
	}
}

func TestCLIConnectorRejectsMCPLifecycleAndAcceptsAuthenticatedReadiness(t *testing.T) {
	runtime := &providerTestRuntime{}
	registry := NewRegistry(runtime, nil)
	if err := registry.LoadSources(context.Background(), []ConnectorSource{NewBuiltinSource()}); err != nil {
		t.Fatal(err)
	}
	if err := registry.Connect(context.Background(), "lark-cli"); !errors.Is(err, ErrConnectorActionUnsupported) {
		t.Fatalf("CLI connector entered MCP lifecycle: %v", err)
	}
	registry.SetCLIControlToken("secret")
	snapshot := CLIConnectorReadiness{
		ProviderID: "lark-cli", Activation: CLIActivationEnabled, Install: CLIInstallInstalled,
		Executable: CLIExecutableAvailable, Configuration: CLIConfigurationReady,
		Authentication: CLIAuthenticationAuthorized, Overall: CLIOverallReady, UpdatedAt: "2026-07-17T00:00:00Z",
	}
	if err := registry.UpdateCLIReadiness("wrong", "lark-cli", snapshot); !errors.Is(err, ErrCLIReadinessUnauthorized) {
		t.Fatalf("wrong control token accepted: %v", err)
	}
	if err := registry.UpdateCLIReadiness("secret", "lark-cli", snapshot); err != nil {
		t.Fatal(err)
	}
	status, err := registry.Status("lark-cli")
	if err != nil {
		t.Fatal(err)
	}
	if status.State != StateConnected || status.ProviderReadiness != tool.ProviderReady || status.CLIReadiness == nil {
		t.Fatalf("CLI readiness was not projected: %+v", status)
	}
}

func TestRestoreBuiltinOwnershipUsesPersistedProviderIdentity(t *testing.T) {
	runtime := &providerTestRuntime{
		injected: mcpclient.ScopedMcpServerConfig{Name: "playwright", Scope: mcpclient.ScopeUser},
		state:    mcpclient.StateConnected,
	}
	registry := NewRegistry(runtime, nil)
	if err := registry.LoadSources(context.Background(), []ConnectorSource{NewBuiltinSource()}); err != nil {
		t.Fatalf("load builtins: %v", err)
	}
	registry.RestoreBuiltinOwnership(mcpclient.ConfigFile{Servers: map[string]mcpclient.McpServerConfig{
		"playwright": {
			Stdio: &mcpclient.StdioTransportConfig{Command: "npx"},
			Provider: &mcpclient.ServerProviderManifest{
				ProviderID: "connector:playwright",
				Source:     tool.SourceConnector,
			},
		},
		// A same-name manual entry is not durable connector ownership.
		"puppeteer": {Stdio: &mcpclient.StdioTransportConfig{Command: "npx"}},
	}})

	playwright, err := registry.Status("playwright")
	if err != nil {
		t.Fatalf("playwright status: %v", err)
	}
	if playwright.State != StateConnected || len(playwright.ServerNames) != 1 || playwright.ServerNames[0] != "playwright" {
		t.Fatalf("persisted builtin ownership was not restored: %+v", playwright)
	}
	runtime.injected.Scope = mcpclient.ScopeProject
	playwright, err = registry.Status("playwright")
	if err != nil {
		t.Fatalf("shadowed playwright status: %v", err)
	}
	if playwright.State != StateDisconnected {
		t.Fatalf("project override was incorrectly reported as builtin readiness: %+v", playwright)
	}
	runtime.injected.Scope = mcpclient.ScopeUser
	puppeteer, err := registry.Status("puppeteer")
	if err != nil {
		t.Fatalf("puppeteer status: %v", err)
	}
	if puppeteer.State != StateDisconnected || len(puppeteer.ServerNames) != 0 {
		t.Fatalf("manual same-name config was incorrectly claimed: %+v", puppeteer)
	}
}

func TestConnectRestoredDisabledBuiltinReenablesRuntime(t *testing.T) {
	t.Setenv("AWAKEN_HOME", t.TempDir())
	disabled := false
	persisted := mcpclient.McpServerConfig{
		Stdio:   &mcpclient.StdioTransportConfig{Command: "npx"},
		Enabled: &disabled,
		Provider: &mcpclient.ServerProviderManifest{
			ProviderID: "connector:playwright",
			Source:     tool.SourceConnector,
		},
	}
	if err := mcpclient.SaveUserConfig(mcpclient.ConfigFile{Servers: map[string]mcpclient.McpServerConfig{"playwright": persisted}}); err != nil {
		t.Fatalf("save user config: %v", err)
	}
	runtime := &providerTestRuntime{
		injected: mcpclient.ScopedMcpServerConfig{Name: "playwright", Scope: mcpclient.ScopeUser, Config: persisted},
		state:    mcpclient.StateDisabled,
	}
	registry := NewRegistry(runtime, nil)
	if err := registry.LoadSources(context.Background(), []ConnectorSource{NewBuiltinSource()}); err != nil {
		t.Fatalf("load builtins: %v", err)
	}
	registry.RestoreBuiltinOwnership(mcpclient.ConfigFile{Servers: map[string]mcpclient.McpServerConfig{"playwright": persisted}})
	if err := registry.Connect(context.Background(), "playwright"); err != nil {
		t.Fatalf("connect disabled builtin: %v", err)
	}
	if runtime.injected.Name != "playwright" || runtime.state != mcpclient.StatePending {
		t.Fatalf("disabled builtin runtime was not restarted: injected=%+v state=%s", runtime.injected, runtime.state)
	}
	userCfg, err := mcpclient.LoadUserConfig()
	if err != nil {
		t.Fatalf("load user config: %v", err)
	}
	if enabled := userCfg.Servers["playwright"].Enabled; enabled != nil && !*enabled {
		t.Fatalf("builtin remained disabled after Connect: %+v", userCfg.Servers["playwright"])
	}
}
