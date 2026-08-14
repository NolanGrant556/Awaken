package connectors

import (
	_ "embed"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"awaken/src/desktopdata"
	"awaken/src/filestore"
	"awaken/src/mcpclient"
	"awaken/src/tool"
)

const (
	playwrightLogPathEnv  = "AWAKEN_PLAYWRIGHT_MCP_LOG_PATH"
	playwrightNodeOptions = "NODE_OPTIONS"
	playwrightLoaderName  = "playwright-log-loader.mjs"
)

//go:embed playwright_log_loader.mjs
var playwrightLogLoader []byte

// PreparePlaywrightLogRouting upgrades an existing Awaken-owned Playwright
// connector config before the MCP manager reads it. Same-name manual configs
// are intentionally left untouched.
func PreparePlaywrightLogRouting() (bool, error) {
	configured := false
	matched := false
	env, err := playwrightLogEnvironment()
	if err != nil {
		return false, err
	}
	err = mcpclient.MutateUserConfig(func(cfg *mcpclient.ConfigFile) (bool, error) {
		current, ok := cfg.Servers["playwright"]
		if !ok || !isAwakenPlaywrightConfig(current) {
			return false, nil
		}
		matched = true
		if current.Stdio.Env == nil {
			current.Stdio.Env = map[string]string{}
		}
		changed := false
		for key, value := range env {
			if key == playwrightNodeOptions {
				value = mergePlaywrightNodeOptions(current.Stdio.Env[key], value)
			}
			if current.Stdio.Env[key] == value {
				continue
			}
			current.Stdio.Env[key] = value
			changed = true
		}
		if changed {
			cfg.Servers["playwright"] = current
			configured = true
		}
		return changed, nil
	})
	if err != nil || !matched {
		return configured, err
	}
	if err := ensurePlaywrightLogRuntime(); err != nil {
		return configured, err
	}
	return configured, nil
}

func isAwakenPlaywrightConfig(cfg mcpclient.McpServerConfig) bool {
	if cfg.Stdio == nil || cfg.Provider == nil {
		return false
	}
	if cfg.Provider.ProviderID != tool.ProviderID("connector:playwright") || cfg.Provider.Source != tool.SourceConnector {
		return false
	}
	for _, arg := range cfg.Stdio.Args {
		if arg == "@executeautomation/playwright-mcp-server" {
			return true
		}
	}
	return false
}

func preparePlaywrightLogEnvironment() (map[string]string, error) {
	env, err := playwrightLogEnvironment()
	if err != nil {
		return nil, err
	}
	if err := ensurePlaywrightLogRuntime(); err != nil {
		return nil, err
	}
	return env, nil
}

func playwrightLogEnvironment() (map[string]string, error) {
	awakenHome := desktopdata.AwakenHomeRoot()
	if awakenHome == "" {
		return nil, fmt.Errorf("AWAKEN_HOME is required")
	}
	loaderPath := filepath.Join(awakenHome, "runtime", "mcp", "playwright", playwrightLoaderName)
	loaderURL, err := fileURL(loaderPath)
	if err != nil {
		return nil, fmt.Errorf("resolve Playwright log loader URL: %w", err)
	}
	return map[string]string{
		playwrightLogPathEnv:  filepath.Join(awakenHome, "logs", "mcp", "playwright-mcp-server.log"),
		playwrightNodeOptions: "--experimental-loader=" + loaderURL,
	}, nil
}

func ensurePlaywrightLogRuntime() error {
	awakenHome := desktopdata.AwakenHomeRoot()
	if awakenHome == "" {
		return fmt.Errorf("AWAKEN_HOME is required")
	}
	runtimeDir := filepath.Join(awakenHome, "runtime", "mcp", "playwright")
	if err := ensurePrivateDirectory(runtimeDir); err != nil {
		return err
	}
	loaderPath := filepath.Join(runtimeDir, playwrightLoaderName)
	if err := writeRuntimeFile(loaderPath, playwrightLogLoader); err != nil {
		return err
	}
	logDir := filepath.Join(awakenHome, "logs", "mcp")
	if err := ensurePrivateDirectory(logDir); err != nil {
		return err
	}
	return nil
}

func ensurePrivateDirectory(path string) error {
	if err := os.MkdirAll(path, filestore.DirPerm); err != nil {
		return fmt.Errorf("create Playwright MCP runtime directory: %w", err)
	}
	if err := os.Chmod(path, filestore.DirPerm); err != nil {
		return fmt.Errorf("protect Playwright MCP runtime directory: %w", err)
	}
	return nil
}

func writeRuntimeFile(path string, data []byte) error {
	current, err := os.ReadFile(path)
	if err == nil && string(current) == string(data) {
		return os.Chmod(path, filestore.FilePerm)
	}
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read Playwright MCP log loader: %w", err)
	}
	if err := filestore.WriteFileAtomic(path, data); err != nil {
		return fmt.Errorf("write Playwright MCP log loader: %w", err)
	}
	return nil
}

func fileURL(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	slashPath := filepath.ToSlash(absolute)
	if runtime.GOOS == "windows" && !strings.HasPrefix(slashPath, "/") {
		slashPath = "/" + slashPath
	}
	return (&url.URL{Scheme: "file", Path: slashPath}).String(), nil
}

func mergePlaywrightNodeOptions(existing, required string) string {
	existing = strings.TrimSpace(existing)
	if existing == "" || existing == required {
		return required
	}
	if strings.Contains(existing, required) {
		return existing
	}
	return existing + " " + required
}
