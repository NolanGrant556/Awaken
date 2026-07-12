package connectors

import (
	"context"
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

func (r *providerTestRuntime) RemoveDynamic(string, string)                             {}
func (r *providerTestRuntime) Start(context.Context, []mcpclient.ScopedMcpServerConfig) {}
func (r *providerTestRuntime) Remove(string) error                                      { return nil }
func (r *providerTestRuntime) Snapshot() []mcpclient.ServerSnapshot {
	if r.injected.Name == "" {
		return nil
	}
	return []mcpclient.ServerSnapshot{{Name: r.injected.Name, State: r.state}}
}
func (r *providerTestRuntime) SnapshotServer(name string) (mcpclient.ServerSnapshot, error) {
	return mcpclient.ServerSnapshot{Name: name, State: r.state}, nil
}

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
			if !tool.HasCapability(definition.Capabilities, tool.CapabilityMessageSend) {
				t.Errorf("%s missing message capability", definition.ID)
			}
		}
	}
}
