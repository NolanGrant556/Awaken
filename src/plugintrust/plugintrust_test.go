package plugintrust

import "testing"

func TestValidateManifest(t *testing.T) {
	tests := []struct {
		name          string
		raw           []byte
		wantOK        bool
		wantErrorPath string
		wantWarnPath  string
	}{
		{
			name:   "valid manifest",
			raw:    []byte(`{"name":"my-plugin","version":"1.0.0","homepage":"https://example.com","description":"test","commands":["commands/run.md"],"agents":["agents/review.md"],"skills":["skills/go/SKILL.md"],"mcpServers":["mcp/server.json"]}`),
			wantOK: true,
		},
		{
			name:          "missing name",
			raw:           []byte(`{"version":"1.0.0"}`),
			wantErrorPath: "name",
		},
		{
			name:          "command path traversal",
			raw:           []byte(`{"name":"my-plugin","commands":["../escape.md"]}`),
			wantErrorPath: "commands[0]",
		},
		{
			name:          "agent path traversal",
			raw:           []byte(`{"name":"my-plugin","agents":["agents/../escape.md"]}`),
			wantErrorPath: "agents[0]",
		},
		{
			name:          "skill path traversal",
			raw:           []byte(`{"name":"my-plugin","skills":["skills/../../escape.md"]}`),
			wantErrorPath: "skills[0]",
		},
		{
			name:          "mcp server path traversal",
			raw:           []byte(`{"name":"my-plugin","mcpServers":["../server.json"]}`),
			wantErrorPath: "mcpServers[0]",
		},
		{
			name:         "marketplace-only warning",
			raw:          []byte(`{"name":"my-plugin","source":"./plugins/my-plugin","tags":["dev"]}`),
			wantOK:       true,
			wantWarnPath: "source",
		},
		{
			name:         "kebab-case warning",
			raw:          []byte(`{"name":"MyPlugin"}`),
			wantOK:       true,
			wantWarnPath: "name",
		},
		{
			name:         "unknown fields warning",
			raw:          []byte(`{"name":"my-plugin","typo":true}`),
			wantOK:       true,
			wantWarnPath: "typo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := ValidateManifest(tt.raw)
			if gotOK := report.OK(); gotOK != tt.wantOK {
				t.Fatalf("OK() = %v, want %v; report = %#v", gotOK, tt.wantOK, report)
			}
			if tt.wantErrorPath != "" && !hasFinding(report.Errors, tt.wantErrorPath) {
				t.Fatalf("expected error at %q, got %#v", tt.wantErrorPath, report.Errors)
			}
			if tt.wantWarnPath != "" && !hasFinding(report.Warnings, tt.wantWarnPath) {
				t.Fatalf("expected warning at %q, got %#v", tt.wantWarnPath, report.Warnings)
			}
		})
	}
}

func TestAssessTrust(t *testing.T) {
	tests := []struct {
		name      string
		source    Source
		wantTrust TrustStatus
		wantWarns bool
		wantMsg   bool
	}{
		{
			name:      "trusted official source",
			source:    Source{Name: "awaken-core", Origin: "official"},
			wantTrust: TrustTrusted,
			wantMsg:   true,
		},
		{
			name:      "allowlisted source",
			source:    Source{Name: "team-plugin", Origin: "remote", Allowlisted: true},
			wantTrust: TrustTrusted,
			wantMsg:   true,
		},
		{
			name:      "untrusted remote source requires confirmation",
			source:    Source{Name: "remote-plugin", Origin: "remote", URL: "https://example.com/plugin.git"},
			wantTrust: TrustRequiresConfirmation,
			wantWarns: true,
			wantMsg:   true,
		},
		{
			name:      "explicitly untrusted source",
			source:    Source{Name: "blocked-plugin", Origin: "untrusted"},
			wantTrust: TrustUntrusted,
			wantMsg:   true,
		},
		{
			name:      "unknown source",
			source:    Source{Name: "local-plugin"},
			wantTrust: TrustUnknown,
			wantMsg:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := AssessTrust(tt.source)
			if report.Trust != tt.wantTrust {
				t.Fatalf("Trust = %q, want %q; report = %#v", report.Trust, tt.wantTrust, report)
			}
			if tt.wantWarns && len(report.Warnings) == 0 {
				t.Fatalf("expected warnings, got none")
			}
			if tt.wantMsg && report.Message == "" {
				t.Fatalf("expected message, got empty")
			}
		})
	}
}

func TestValidateManifestMapCopiesSlices(t *testing.T) {
	commands := []string{"commands/run.md"}
	values := map[string]any{
		"name":     "my-plugin",
		"commands": commands,
	}

	report := ValidateManifestMap(values)
	if !report.OK() {
		t.Fatalf("expected valid report, got %#v", report)
	}

	commands[0] = "../mutated.md"
	if report.Manifest.Commands[0] != "commands/run.md" {
		t.Fatalf("manifest command changed after caller mutation: %#v", report.Manifest.Commands)
	}
}

func hasFinding(findings []Finding, path string) bool {
	for _, finding := range findings {
		if finding.Path == path {
			return true
		}
	}
	return false
}
