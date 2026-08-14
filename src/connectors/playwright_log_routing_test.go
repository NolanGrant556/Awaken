package connectors

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"awaken/src/filestore"
	"awaken/src/mcpclient"
	"awaken/src/tool"
)

func TestPreparePlaywrightLogRoutingUpdatesOnlyOwnedConfig(t *testing.T) {
	awakenHome := t.TempDir()
	t.Setenv("AWAKEN_HOME", awakenHome)
	configured := mcpclient.McpServerConfig{
		Stdio: &mcpclient.StdioTransportConfig{
			Command: "npx",
			Args:    []string{"-y", "@executeautomation/playwright-mcp-server"},
			Env: map[string]string{
				"EXISTING":     "preserved",
				"NODE_OPTIONS": "--trace-warnings",
			},
		},
		Provider: &mcpclient.ServerProviderManifest{
			ProviderID: "connector:playwright",
			Source:     tool.SourceConnector,
		},
	}
	manual := mcpclient.McpServerConfig{
		Stdio: &mcpclient.StdioTransportConfig{
			Command: "npx",
			Args:    []string{"-y", "@executeautomation/playwright-mcp-server"},
		},
	}
	if err := mcpclient.SaveUserConfig(mcpclient.ConfigFile{Servers: map[string]mcpclient.McpServerConfig{
		"playwright":        configured,
		"playwright-manual": manual,
	}}); err != nil {
		t.Fatalf("save user config: %v", err)
	}

	changed, err := PreparePlaywrightLogRouting()
	if err != nil {
		t.Fatalf("prepare log routing: %v", err)
	}
	if !changed {
		t.Fatal("expected existing connector config to be updated")
	}
	loaded, err := mcpclient.LoadUserConfig()
	if err != nil {
		t.Fatalf("load user config: %v", err)
	}
	env := loaded.Servers["playwright"].Stdio.Env
	wantLog := filepath.Join(awakenHome, "logs", "mcp", "playwright-mcp-server.log")
	if env[playwrightLogPathEnv] != wantLog {
		t.Fatalf("log path = %q, want %q", env[playwrightLogPathEnv], wantLog)
	}
	if env["EXISTING"] != "preserved" {
		t.Fatalf("existing env was lost: %+v", env)
	}
	if !strings.Contains(env[playwrightNodeOptions], "--trace-warnings") || !strings.Contains(env[playwrightNodeOptions], playwrightLoaderName) {
		t.Fatalf("NODE_OPTIONS did not preserve existing options and add loader: %q", env[playwrightNodeOptions])
	}
	if loaded.Servers["playwright-manual"].Stdio.Env != nil {
		t.Fatalf("same-name manual config was modified: %+v", loaded.Servers["playwright-manual"])
	}
	loaderPath := filepath.Join(awakenHome, "runtime", "mcp", "playwright", playwrightLoaderName)
	info, err := os.Stat(loaderPath)
	if err != nil {
		t.Fatalf("stat loader: %v", err)
	}
	if info.Mode().Perm() != filestore.FilePerm {
		t.Fatalf("loader permissions = %v, want %v", info.Mode().Perm(), filestore.FilePerm)
	}

	changed, err = PreparePlaywrightLogRouting()
	if err != nil {
		t.Fatalf("repeat preparation: %v", err)
	}
	if changed {
		t.Fatal("repeat preparation should be idempotent")
	}
}

func TestPlaywrightConfigFactoryRoutesLogsWithoutChangingHome(t *testing.T) {
	awakenHome := t.TempDir()
	t.Setenv("AWAKEN_HOME", awakenHome)
	configs, err := playwrightConfigFactory(playwrightConnector(), nil)
	if err != nil {
		t.Fatalf("factory: %v", err)
	}
	env := configs["playwright"].Config.Stdio.Env
	if _, ok := env["HOME"]; ok {
		t.Fatalf("factory must not override HOME: %+v", env)
	}
	if env[playwrightLogPathEnv] != filepath.Join(awakenHome, "logs", "mcp", "playwright-mcp-server.log") {
		t.Fatalf("unexpected log path: %+v", env)
	}
}

func TestPlaywrightLogLoaderRedirectsUpstreamHardcodedPath(t *testing.T) {
	nodePath, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is unavailable")
	}
	awakenHome := t.TempDir()
	t.Setenv("AWAKEN_HOME", awakenHome)
	env, err := preparePlaywrightLogEnvironment()
	if err != nil {
		t.Fatalf("prepare environment: %v", err)
	}
	fixtureRoot := t.TempDir()
	packageRoot := filepath.Join(fixtureRoot, "node_modules", "@executeautomation", "playwright-mcp-server")
	distDir := filepath.Join(packageRoot, "dist")
	if err := os.MkdirAll(distDir, filestore.DirPerm); err != nil {
		t.Fatalf("create fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(packageRoot, "package.json"), []byte("{\"type\":\"module\"}\n"), filestore.FilePerm); err != nil {
		t.Fatalf("write fixture package: %v", err)
	}
	fixture := "import fs from 'node:fs';\nconst config = { filePath: `${process.env.HOME || '/tmp'}/playwright-mcp-server.log`, };\nfs.writeFileSync(config.filePath, 'redirected');\n"
	entryPath := filepath.Join(distDir, "index.js")
	if err := os.WriteFile(entryPath, []byte(fixture), filestore.FilePerm); err != nil {
		t.Fatalf("write fixture entry: %v", err)
	}
	fakeHome := t.TempDir()
	cmd := exec.Command(nodePath, entryPath)
	cmd.Env = append(os.Environ(),
		"HOME="+fakeHome,
		playwrightLogPathEnv+"="+env[playwrightLogPathEnv],
		playwrightNodeOptions+"="+env[playwrightNodeOptions],
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("run loader fixture: %v\n%s", err, output)
	}
	data, err := os.ReadFile(env[playwrightLogPathEnv])
	if err != nil {
		t.Fatalf("read redirected log: %v", err)
	}
	if string(data) != "redirected" {
		t.Fatalf("redirected log = %q", data)
	}
	if _, err := os.Stat(filepath.Join(fakeHome, "playwright-mcp-server.log")); !os.IsNotExist(err) {
		t.Fatalf("upstream home log was still written: %v", err)
	}
}
