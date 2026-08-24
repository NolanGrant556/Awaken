package plugins

import (
	"path/filepath"
	"testing"
)

func TestParseMCPJSONStdioAndRemote(t *testing.T) {
	root := t.TempDir()
	data := filepath.Join(t.TempDir(), "data")
	raw := []byte(`{
		"$schema":"` + SchemaMCPV1 + `",
		"mcpServers":{
			"local-validator":{
				"type":"stdio",
				"command":"./bin/validator",
				"args":["--data","${PLUGIN_DATA}/validator"],
				"env":{"CONFIG":"${PLUGIN_ROOT}/config.json"},
				"cwd":"${PLUGIN_ROOT}"
			},
			"deployment-api":{
				"type":"streamable-http",
				"url":"https://deploy.example.com/mcp",
				"headers":{"X-Tenant":"public-tenant"}
			},
			"legacy-events":{
				"type":"sse",
				"url":"https://legacy.example.com/sse"
			}
		}
	}`)
	result := parseMCPJSON(raw, root, data, "acme.tools", nil, SchemaPluginV1)
	if result.Invalid != nil {
		t.Fatalf("invalid: %+v", result.Invalid)
	}
	if len(result.Servers) != 3 {
		t.Fatalf("servers=%d", len(result.Servers))
	}
	byName := map[string]LoadedServer{}
	for _, server := range result.Servers {
		byName[server.ShortName] = server
	}
	local := byName["local-validator"]
	if local.Identity != "plugin__acme.tools__local-validator" {
		t.Fatalf("identity=%q", local.Identity)
	}
	if local.Config.Stdio == nil {
		t.Fatal("missing stdio")
	}
	if local.Config.Stdio.Command != filepath.Join(root, "bin", "validator") {
		t.Fatalf("command=%q", local.Config.Stdio.Command)
	}
	if local.Config.Stdio.Args[1] != filepath.Join(data, "validator") {
		t.Fatalf("args=%v", local.Config.Stdio.Args)
	}
	if local.Config.Stdio.Env["CONFIG"] != filepath.Join(root, "config.json") {
		t.Fatalf("env=%v", local.Config.Stdio.Env)
	}
	if local.Config.Stdio.Cwd != root || local.Config.Stdio.PluginRoot != root || local.Config.Stdio.PluginData != data {
		t.Fatalf("cwd/root/data=%q %q %q", local.Config.Stdio.Cwd, local.Config.Stdio.PluginRoot, local.Config.Stdio.PluginData)
	}
	if byName["deployment-api"].Config.HTTP == nil || byName["legacy-events"].Config.SSE == nil {
		t.Fatalf("remote transports missing: %+v", byName)
	}
}

func TestParseMCPJSONIsolatesInvalidEntries(t *testing.T) {
	root := t.TempDir()
	raw := []byte(`{
		"$schema":"` + SchemaMCPV1 + `",
		"mcpServers":{
			"ok":{"type":"stdio","command":"npx"},
			"bad-env":{"type":"stdio","command":"npx","env":{"PLUGIN_ROOT":"/tmp"}},
			"bad-cwd":{"type":"stdio","command":"npx","cwd":"../escape"},
			"http-plain":{"type":"streamable-http","url":"http://example.com/mcp"},
			"dup-header":{"type":"sse","url":"https://example.com/sse","headers":{"X-A":"1","x-a":"2"}},
			"unknown":{"type":"websocket","url":"wss://example.com"}
		}
	}`)
	result := parseMCPJSON(raw, root, root, "sample", nil, SchemaPluginV1)
	if result.Invalid != nil {
		t.Fatalf("top-level invalid: %+v", result.Invalid)
	}
	okCount := 0
	for _, server := range result.Servers {
		if server.ShortName == "ok" && server.Invalid == nil {
			okCount++
		}
		if server.ShortName != "ok" && server.Invalid == nil {
			t.Fatalf("expected invalid %s", server.ShortName)
		}
	}
	if okCount != 1 {
		t.Fatalf("okCount=%d servers=%d", okCount, len(result.Servers))
	}
}

func TestParseMCPJSONTopLevelFailures(t *testing.T) {
	cases := []string{
		`{"$schema":"` + SchemaMCPV1 + `"}`,
		`{"$schema":"` + SchemaMCPV1 + `","mcpServers":{},"extra":true}`,
		`{"$schema":"https://agent-plugins.org/schemas/2.0.0/mcp.schema.json","mcpServers":{}}`,
		`not-json`,
	}
	for _, raw := range cases {
		result := parseMCPJSON([]byte(raw), t.TempDir(), t.TempDir(), "sample", nil, SchemaPluginV1)
		if result.Invalid == nil {
			t.Fatalf("expected top-level invalid for %s", raw)
		}
	}
}

func TestPlaceholderExpansionIsSinglePass(t *testing.T) {
	got := expandPluginPlaceholders("${PLUGIN_ROOT}/${PLUGIN_DATA}", "/root", "${PLUGIN_ROOT}/nested")
	if got != "/root/${PLUGIN_ROOT}/nested" {
		t.Fatalf("got %q", got)
	}
}

func TestCommandAndURLAreNotExpanded(t *testing.T) {
	root := t.TempDir()
	result := parseMCPJSON([]byte(`{
		"$schema":"`+SchemaMCPV1+`",
		"mcpServers":{
			"literal":{"type":"stdio","command":"npx","args":["${PLUGIN_ROOT}"]}
		}
	}`), root, root, "sample", nil, SchemaPluginV1)
	if result.Invalid != nil || result.Servers[0].Config.Stdio.Command != "npx" {
		t.Fatalf("%+v", result)
	}
	if result.Servers[0].Config.Stdio.Args[0] != root {
		t.Fatalf("args=%v", result.Servers[0].Config.Stdio.Args)
	}
}
