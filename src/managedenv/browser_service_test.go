package managedenv

import "testing"

func TestBrowserServicePreservesExplicitConfiguration(t *testing.T) {
	for _, name := range []string{"@playwright/mcp", "chrome-devtools-mcp", "@modelcontextprotocol/server-puppeteer"} {
		if !needsManagedBrowser(PackageCommand{Ecosystem: "node", Package: name}, nil) {
			t.Fatalf("default %s should prepare its browser", name)
		}
	}
	for _, tc := range []struct {
		name      string
		args, env []string
	}{
		{"@playwright/mcp", []string{"--cdp-endpoint=http://localhost:9222"}, nil},
		{"@playwright/mcp", []string{"--config", "custom.json"}, nil},
		{"@playwright/mcp", nil, []string{"PLAYWRIGHT_MCP_BROWSER=firefox"}},
		{"chrome-devtools-mcp", []string{"--auto-connect"}, nil},
		{"chrome-devtools-mcp", []string{"--browser-url=http://localhost:9222"}, nil},
		{"chrome-devtools-mcp", []string{"--executablePath", "custom/chrome"}, nil},
		{"@modelcontextprotocol/server-puppeteer", nil, []string{`PUPPETEER_LAUNCH_OPTIONS={"channel":"chrome"}`}},
		{"@modelcontextprotocol/server-puppeteer", nil, []string{"PUPPETEER_LAUNCH_OPTIONS=invalid"}},
		{"@modelcontextprotocol/server-puppeteer", nil, []string{"PUPPETEER_EXECUTABLE_PATH=custom/chrome"}},
	} {
		if needsManagedBrowser(PackageCommand{Ecosystem: "node", Package: tc.name, Args: tc.args}, tc.env) {
			t.Fatalf("must preserve external config: %+v", tc)
		}
	}
}

func TestBrowserServiceUsesExactManagedPath(t *testing.T) {
	path := "/用户 环境/Chrome.app/chrome"
	b := Binding{Executables: map[string]Launch{"chrome": {Executable: path}}, Environment: map[string]string{}}
	p := PackageCommand{Package: "@playwright/mcp", Args: []string{"--headless"}}
	configureManagedBrowser(&p, b)
	if len(p.Args) != 3 || p.Args[1] != "--executable-path" || p.Args[2] != path {
		t.Fatal(p.Args)
	}
	p = PackageCommand{Package: "@modelcontextprotocol/server-puppeteer"}
	configureManagedBrowser(&p, b)
	if b.Environment["PUPPETEER_EXECUTABLE_PATH"] != path {
		t.Fatal(b.Environment)
	}
}
