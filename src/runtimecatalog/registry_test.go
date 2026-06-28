package runtimecatalog

import (
	"errors"
	"testing"
)

func TestRegistryRegisterListFilter(t *testing.T) {
	registry, err := NewRegistry(
		RuntimeEntry{
			ID:            "zeta",
			Kind:          RuntimeKindRemote,
			Name:          "Remote Agent",
			Enabled:       false,
			InstallStatus: InstallStatusInstalled,
			Health:        HealthSnapshot{Status: HealthStatusHealthy},
		},
		RuntimeEntry{
			ID:            "alpha",
			Kind:          RuntimeKindACP,
			Name:          "Claude ACP",
			Enabled:       true,
			InstallStatus: InstallStatusInstalled,
			Health:        HealthSnapshot{Status: HealthStatusHealthy},
		},
	)
	if err != nil {
		t.Fatalf("NewRegistry failed: %v", err)
	}

	if err := registry.Register(RuntimeEntry{
		ID:            "alpha",
		Kind:          RuntimeKindACP,
		Name:          "Duplicate",
		InstallStatus: InstallStatusInstalled,
		Health:        HealthSnapshot{Status: HealthStatusHealthy},
	}); !errors.Is(err, ErrDuplicateRuntime) {
		t.Fatalf("expected duplicate runtime error, got %v", err)
	}

	entries := registry.List()
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].ID != "alpha" || entries[1].ID != "zeta" {
		t.Fatalf("expected entries sorted by id, got %q then %q", entries[0].ID, entries[1].ID)
	}
	entries[0].Name = "mutated"
	got, ok := registry.Get("alpha")
	if !ok {
		t.Fatalf("expected alpha entry")
	}
	if got.Name != "Claude ACP" {
		t.Fatalf("registry leaked mutable list entry, got name %q", got.Name)
	}

	acp := registry.FindByKind("ACP")
	if len(acp) != 1 || acp[0].ID != "alpha" {
		t.Fatalf("expected ACP filter to return alpha, got %+v", acp)
	}

	enabled := registry.Enabled()
	if len(enabled) != 1 || enabled[0].ID != "alpha" {
		t.Fatalf("expected enabled filter to return alpha, got %+v", enabled)
	}
}

func TestRegistryUpdateHealth(t *testing.T) {
	registry, err := NewRegistry(RuntimeEntry{
		ID:            "claude",
		Kind:          RuntimeKindLocalCLI,
		Name:          "Claude Code",
		Enabled:       true,
		InstallStatus: InstallStatusInstalled,
		Health:        HealthSnapshot{Status: HealthStatusUnknown},
	})
	if err != nil {
		t.Fatalf("NewRegistry failed: %v", err)
	}

	if err := registry.UpdateHealth(" claude ", HealthSnapshot{
		Status:    "HEALTHY",
		Message:   " ready ",
		LatencyMS: 42,
	}); err != nil {
		t.Fatalf("UpdateHealth failed: %v", err)
	}
	got, _ := registry.Get("claude")
	if got.Health.Status != HealthStatusHealthy {
		t.Fatalf("expected healthy status, got %q", got.Health.Status)
	}
	if got.Health.Message != "ready" {
		t.Fatalf("expected trimmed message, got %q", got.Health.Message)
	}
	if got.Health.LatencyMS != 42 {
		t.Fatalf("expected latency 42, got %d", got.Health.LatencyMS)
	}

	if err := registry.UpdateHealth("missing", HealthSnapshot{Status: HealthStatusHealthy}); !errors.Is(err, ErrRuntimeNotFound) {
		t.Fatalf("expected not found error, got %v", err)
	}
	if err := registry.UpdateHealth("claude", HealthSnapshot{Status: "bad"}); !errors.Is(err, ErrInvalidRuntime) {
		t.Fatalf("expected invalid runtime error, got %v", err)
	}
}

func TestNormalizeRedactsEnvAndDedupes(t *testing.T) {
	entry := Normalize(RuntimeEntry{
		ID:            " claude ",
		Kind:          "LOCAL_CLI",
		Name:          " Claude Code ",
		InstallStatus: "",
		Health:        HealthSnapshot{},
		Env: []EnvVar{
			{Name: "OPENAI_API_KEY", Value: "secret"},
			{Name: "PATH", Value: "/usr/bin"},
			{Name: "CUSTOM_HEADER", Value: "Bearer secret"},
			{Name: "  "},
		},
		Capabilities: []Capability{
			{ID: "tool.shell", Name: "Shell", MCPTransports: []MCPTransport{"STDIO", "stdio"}},
			{ID: "tool.shell", Name: "Duplicate"},
			{ID: "mcp.files", MCPTransports: []MCPTransport{MCPTransportHTTP}},
		},
		Models: []ModelOption{
			{ID: "sonnet", Label: "Sonnet"},
			{ID: "sonnet", Label: "Duplicate Sonnet"},
			{ID: "opus", Label: "Opus"},
		},
		Modes: []ModeOption{
			{ID: "plan", Label: "Plan"},
			{ID: "plan", Label: "Duplicate Plan"},
			{ID: "act", Label: "Act"},
		},
		MCPTransports: []MCPTransport{"STDIO", "http", "stdio"},
	})

	if entry.ID != "claude" || entry.Kind != RuntimeKindLocalCLI || entry.Name != "Claude Code" {
		t.Fatalf("entry was not normalized: %+v", entry)
	}
	if entry.InstallStatus != InstallStatusUnknown || entry.Health.Status != HealthStatusUnknown {
		t.Fatalf("expected unknown default statuses, got install=%q health=%q", entry.InstallStatus, entry.Health.Status)
	}
	if len(entry.Env) != 3 {
		t.Fatalf("expected blank env to be removed, got %+v", entry.Env)
	}
	if entry.Env[0].Value != RedactedValue || !entry.Env[0].Redacted {
		t.Fatalf("expected API key env to be redacted, got %+v", entry.Env[0])
	}
	if entry.Env[1].Value != "/usr/bin" || entry.Env[1].Redacted {
		t.Fatalf("expected PATH env to remain visible, got %+v", entry.Env[1])
	}
	if entry.Env[2].Value != RedactedValue || !entry.Env[2].Redacted {
		t.Fatalf("expected bearer value to be redacted, got %+v", entry.Env[2])
	}
	if len(entry.Capabilities) != 2 {
		t.Fatalf("expected duplicate capability removed, got %+v", entry.Capabilities)
	}
	if len(entry.Capabilities[0].MCPTransports) != 1 || entry.Capabilities[0].MCPTransports[0] != MCPTransportStdio {
		t.Fatalf("expected capability transports normalized and deduped, got %+v", entry.Capabilities[0].MCPTransports)
	}
	if len(entry.Models) != 2 || entry.Models[0].Label != "Sonnet" {
		t.Fatalf("expected duplicate model removed with first value kept, got %+v", entry.Models)
	}
	if len(entry.Modes) != 2 || entry.Modes[0].Label != "Plan" {
		t.Fatalf("expected duplicate mode removed with first value kept, got %+v", entry.Modes)
	}
	if len(entry.MCPTransports) != 2 || entry.MCPTransports[0] != MCPTransportStdio || entry.MCPTransports[1] != MCPTransportHTTP {
		t.Fatalf("expected runtime MCP transports normalized and deduped, got %+v", entry.MCPTransports)
	}
}

func TestDiagnosticSummaryCountsAndOrdering(t *testing.T) {
	registry, err := NewRegistry(
		runtimeForDiagnostic("healthy", "Healthy", true, InstallStatusInstalled, HealthStatusHealthy),
		runtimeForDiagnostic("missing", "Missing", true, InstallStatusNeedsInstall, HealthStatusUnknown),
		runtimeForDiagnostic("offline", "Offline", true, InstallStatusInstalled, HealthStatusUnreachable),
		runtimeForDiagnostic("disabled", "Disabled", false, InstallStatusInstalled, HealthStatusHealthy),
		RuntimeEntry{
			ID:            "slow",
			Kind:          RuntimeKindLocalCLI,
			Name:          "Slow",
			Enabled:       true,
			InstallStatus: InstallStatusInstalled,
			Health: HealthSnapshot{
				Status:  HealthStatusDegraded,
				Message: "health check exceeded latency budget",
			},
		},
	)
	if err != nil {
		t.Fatalf("NewRegistry failed: %v", err)
	}

	summary := registry.DiagnosticSummary()
	if summary.Total != 5 {
		t.Fatalf("expected total 5, got %d", summary.Total)
	}
	if summary.Available != 1 || summary.NeedsInstall != 1 || summary.Unreachable != 1 || summary.Disabled != 1 {
		t.Fatalf("unexpected counts: %+v", summary)
	}
	if len(summary.Issues) != 4 {
		t.Fatalf("expected 4 issues, got %+v", summary.Issues)
	}
	want := []IssueSeverity{IssueSeverityCritical, IssueSeverityError, IssueSeverityWarning, IssueSeverityInfo}
	for i, severity := range want {
		if summary.Issues[i].Severity != severity {
			t.Fatalf("issue %d: expected severity %q, got %q in %+v", i, severity, summary.Issues[i].Severity, summary.Issues)
		}
	}
	if summary.Issues[0].Code != "unreachable" || summary.Issues[0].Message != "Offline is unreachable" {
		t.Fatalf("expected unreachable issue first, got %+v", summary.Issues[0])
	}
}

func TestValidateRejectsMissingAndInvalidFields(t *testing.T) {
	valid := RuntimeEntry{
		ID:            "claude",
		Kind:          RuntimeKindACP,
		Name:          "Claude ACP",
		Enabled:       true,
		InstallStatus: InstallStatusInstalled,
		Health:        HealthSnapshot{Status: HealthStatusHealthy},
		Capabilities:  []Capability{{ID: "tool.read"}},
		Models:        []ModelOption{{ID: "sonnet"}},
		Modes:         []ModeOption{{ID: "plan"}},
		MCPTransports: []MCPTransport{MCPTransportStdio},
	}
	if err := Validate(valid); err != nil {
		t.Fatalf("expected valid entry, got %v", err)
	}

	cases := []struct {
		name  string
		entry RuntimeEntry
	}{
		{
			name:  "missing id",
			entry: RuntimeEntry{Kind: RuntimeKindACP, Name: "Claude", InstallStatus: InstallStatusInstalled, Health: HealthSnapshot{Status: HealthStatusHealthy}},
		},
		{
			name:  "invalid kind",
			entry: withKind(valid, "desktop"),
		},
		{
			name:  "missing name",
			entry: RuntimeEntry{ID: "claude", Kind: RuntimeKindACP, InstallStatus: InstallStatusInstalled, Health: HealthSnapshot{Status: HealthStatusHealthy}},
		},
		{
			name:  "invalid install status",
			entry: withInstallStatus(valid, "installed-ish"),
		},
		{
			name:  "invalid health status",
			entry: withHealthStatus(valid, "sleepy"),
		},
		{
			name:  "invalid mcp transport",
			entry: withMCPTransports(valid, []MCPTransport{"websocket"}),
		},
		{
			name:  "missing capability id",
			entry: withCapabilities(valid, []Capability{{Name: "Shell"}}),
		},
		{
			name:  "missing model id",
			entry: withModels(valid, []ModelOption{{Label: "Sonnet"}}),
		},
		{
			name:  "missing mode id",
			entry: withModes(valid, []ModeOption{{Label: "Plan"}}),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := Validate(tc.entry); !errors.Is(err, ErrInvalidRuntime) {
				t.Fatalf("expected invalid runtime error, got %v", err)
			}
		})
	}
}

func runtimeForDiagnostic(id, name string, enabled bool, install InstallStatus, health HealthStatus) RuntimeEntry {
	return RuntimeEntry{
		ID:            id,
		Kind:          RuntimeKindLocalCLI,
		Name:          name,
		Enabled:       enabled,
		InstallStatus: install,
		Health:        HealthSnapshot{Status: health},
	}
}

func withKind(entry RuntimeEntry, kind RuntimeKind) RuntimeEntry {
	entry.Kind = kind
	return entry
}

func withInstallStatus(entry RuntimeEntry, status InstallStatus) RuntimeEntry {
	entry.InstallStatus = status
	return entry
}

func withHealthStatus(entry RuntimeEntry, status HealthStatus) RuntimeEntry {
	entry.Health.Status = status
	return entry
}

func withMCPTransports(entry RuntimeEntry, transports []MCPTransport) RuntimeEntry {
	entry.MCPTransports = transports
	return entry
}

func withCapabilities(entry RuntimeEntry, capabilities []Capability) RuntimeEntry {
	entry.Capabilities = capabilities
	return entry
}

func withModels(entry RuntimeEntry, models []ModelOption) RuntimeEntry {
	entry.Models = models
	return entry
}

func withModes(entry RuntimeEntry, modes []ModeOption) RuntimeEntry {
	entry.Modes = modes
	return entry
}
