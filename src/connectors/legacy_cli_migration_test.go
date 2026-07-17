package connectors

import (
	"os"
	"path/filepath"
	"testing"

	"awaken/src/mcpclient"
	"awaken/src/tool"
)

func TestLegacyCLIMigrationRemovesOnlyProvenBuiltinRecords(t *testing.T) {
	home := t.TempDir()
	t.Setenv("AWAKEN_HOME", home)
	manual := mcpclient.McpServerConfig{
		Stdio:       &mcpclient.StdioTransportConfig{Command: "lark-cli", Args: []string{"mcp"}},
		DisplayName: "My Lark",
	}
	legacy := mcpclient.McpServerConfig{
		Stdio:       &mcpclient.StdioTransportConfig{Command: "dingtalk-workspace-cli", Args: []string{"mcp"}, Env: map[string]string{}},
		DisplayName: "钉钉CLI",
		Provider: &mcpclient.ServerProviderManifest{
			ProviderID: "connector:dingtalk-workspace",
			Source:     tool.SourceConnector,
		},
	}
	if err := mcpclient.SaveUserConfig(mcpclient.ConfigFile{Servers: map[string]mcpclient.McpServerConfig{
		"dingtalk-workspace": legacy,
		"lark":               manual,
	}}); err != nil {
		t.Fatal(err)
	}
	report, err := MigrateLegacyBuiltinCLIConfigs()
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Removed) != 1 || report.Removed[0] != "dingtalk-workspace" {
		t.Fatalf("unexpected removed records: %+v", report)
	}
	if len(report.ReviewRequired) != 1 || report.ReviewRequired[0] != "lark" {
		t.Fatalf("manual record was not preserved for review: %+v", report)
	}
	if _, err := os.Stat(report.BackupPath); err != nil {
		t.Fatalf("migration backup missing: %v", err)
	}
	cfg, err := mcpclient.LoadUserConfig()
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := cfg.Servers["dingtalk-workspace"]; exists {
		t.Fatal("proven legacy record was not removed")
	}
	if _, exists := cfg.Servers["lark"]; !exists {
		t.Fatal("manual same-name record was removed")
	}
	if filepath.Dir(report.BackupPath) != filepath.Join(home, "backups", "migrations", "cli-connectors-v1") {
		t.Fatalf("unexpected backup directory: %s", report.BackupPath)
	}
}

func TestLegacyCLIMigrationIsIdempotent(t *testing.T) {
	t.Setenv("AWAKEN_HOME", t.TempDir())
	first, err := MigrateLegacyBuiltinCLIConfigs()
	if err != nil {
		t.Fatal(err)
	}
	second, err := MigrateLegacyBuiltinCLIConfigs()
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Removed)+len(second.Removed) != 0 {
		t.Fatalf("empty config migration changed state: first=%+v second=%+v", first, second)
	}
}
