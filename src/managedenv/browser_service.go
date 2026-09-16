package managedenv

import (
	"encoding/json"
	"strings"
)

func commandHasOption(args []string, names ...string) bool {
	for _, arg := range args {
		for _, name := range names {
			if arg == name || strings.HasPrefix(arg, name+"=") {
				return true
			}
		}
	}
	return false
}
func serviceEnvValue(env []string, name string) string {
	for _, item := range env {
		key, value, ok := strings.Cut(item, "=")
		if ok && environmentKey(key) == environmentKey(name) {
			return value
		}
	}
	return ""
}

// Only known default browser launches receive an executable. Remote endpoints,
// extensions, custom config files, browser channels and explicit paths retain
// the user's original launch semantics.
func needsManagedBrowser(p PackageCommand, env []string) bool {
	if p.Ecosystem != "node" {
		return false
	}
	switch p.Package {
	case "@playwright/mcp":
		if commandHasOption(p.Args, "--executable-path", "--cdp-endpoint", "--endpoint", "--extension", "--browser", "--config") {
			return false
		}
		for _, key := range []string{"PLAYWRIGHT_MCP_EXECUTABLE_PATH", "PLAYWRIGHT_MCP_CDP_ENDPOINT", "PLAYWRIGHT_MCP_ENDPOINT", "PLAYWRIGHT_MCP_EXTENSION", "PLAYWRIGHT_MCP_BROWSER", "PLAYWRIGHT_MCP_CONFIG"} {
			if serviceEnvValue(env, key) != "" {
				return false
			}
		}
		return true
	case "@modelcontextprotocol/server-puppeteer":
		if serviceEnvValue(env, "PUPPETEER_EXECUTABLE_PATH") != "" {
			return false
		}
		if raw := serviceEnvValue(env, "PUPPETEER_LAUNCH_OPTIONS"); raw != "" {
			var options map[string]any
			if json.Unmarshal([]byte(raw), &options) != nil {
				return false
			}
			if options["executablePath"] != nil || options["channel"] != nil {
				return false
			}
		}
		return true
	case "chrome-devtools-mcp":
		return !commandHasOption(p.Args, "--executablePath", "--executable-path", "--browserUrl", "--browser-url", "--wsEndpoint", "--ws-endpoint", "--autoConnect", "--auto-connect", "--channel")
	}
	return false
}

func configureManagedBrowser(p *PackageCommand, b Binding) {
	chrome := b.Executables["chrome"].Executable
	if chrome == "" {
		return
	}
	switch p.Package {
	case "@playwright/mcp":
		p.Args = append(p.Args, "--executable-path", chrome)
	case "chrome-devtools-mcp":
		p.Args = append(p.Args, "--executablePath", chrome)
	case "@modelcontextprotocol/server-puppeteer":
		b.Environment["PUPPETEER_EXECUTABLE_PATH"] = chrome
	}
}
