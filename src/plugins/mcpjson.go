package plugins

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"awaken/src/mcpclient"
)

type mcpLoadResult struct {
	Invalid     *Diagnostic
	Servers     []LoadedServer
	Diagnostics []Diagnostic
}

func loadMCPFile(pluginRoot, dataDir, pluginName string, toggles map[string]MCPServerToggle, pluginSchema string) mcpLoadResult {
	path := filepath.Join(pluginRoot, "mcp.json")
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return mcpLoadResult{}
		}
		return mcpLoadResult{Invalid: diagPtr(errorDiag("plugin_mcp_invalid", err.Error()))}
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return mcpLoadResult{Invalid: diagPtr(errorDiag("plugin_mcp_invalid", "mcp.json must be a regular file"))}
	}
	if err := resolvedStayWithin(pluginRoot, path); err != nil {
		return mcpLoadResult{Invalid: diagPtr(errorDiag("plugin_mcp_invalid", "mcp.json escapes the plugin root"))}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return mcpLoadResult{Invalid: diagPtr(errorDiag("plugin_mcp_invalid", err.Error()))}
	}
	return parseMCPJSON(raw, pluginRoot, dataDir, pluginName, toggles, pluginSchema)
}

func parseMCPJSON(raw []byte, pluginRoot, dataDir, pluginName string, toggles map[string]MCPServerToggle, pluginSchema string) mcpLoadResult {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return mcpLoadResult{Invalid: diagPtr(errorDiag("plugin_mcp_invalid", "mcp.json is not valid JSON"))}
	}
	object, ok := value.(map[string]any)
	if !ok {
		return mcpLoadResult{Invalid: diagPtr(errorDiag("plugin_mcp_invalid", "mcp.json must contain a JSON object"))}
	}
	if len(object) != 2 {
		return mcpLoadResult{Invalid: diagPtr(errorDiag("plugin_mcp_invalid", "mcp.json may only contain $schema and mcpServers"))}
	}
	schema, err := requiredString(object, "$schema")
	if err != nil {
		return mcpLoadResult{Invalid: diagPtr(errorDiag("plugin_mcp_invalid", err.Error()))}
	}
	if schema != SchemaMCPV1 {
		if strings.HasPrefix(schema, SchemaPrefix) {
			return mcpLoadResult{Invalid: diagPtr(errorDiag("plugin_mcp_invalid", "unsupported MCP schema "+schema))}
		}
		return mcpLoadResult{Invalid: diagPtr(errorDiag("plugin_mcp_invalid", "mcp.json is not an Agent Plugins MCP document"))}
	}
	if schemaVersion(schema) != schemaVersion(pluginSchema) {
		return mcpLoadResult{Invalid: diagPtr(errorDiag("plugin_mcp_invalid", "mcp.json $schema version must match plugin.json"))}
	}
	rawServers, ok := object["mcpServers"]
	if !ok {
		return mcpLoadResult{Invalid: diagPtr(errorDiag("plugin_mcp_invalid", "mcpServers is required"))}
	}
	serversObject, ok := rawServers.(map[string]any)
	if !ok {
		return mcpLoadResult{Invalid: diagPtr(errorDiag("plugin_mcp_invalid", "mcpServers must be an object"))}
	}

	result := mcpLoadResult{}
	for shortName, rawServer := range serversObject {
		server, invalid := parseServerEntry(shortName, rawServer, pluginRoot, dataDir, pluginName)
		if invalid != nil {
			result.Diagnostics = append(result.Diagnostics, *invalid)
			result.Servers = append(result.Servers, LoadedServer{
				ShortName: shortName,
				Identity:  ServerIdentity(pluginName, shortName),
				Invalid:   invalid,
			})
			continue
		}
		enabled := true
		if toggle, ok := toggles[shortName]; ok {
			enabled = toggle.Enabled
		}
		server.Enabled = enabled
		enabledFlag := enabled
		server.Config.Enabled = &enabledFlag
		result.Servers = append(result.Servers, server)
	}
	return result
}

func parseServerEntry(shortName string, raw any, pluginRoot, dataDir, pluginName string) (LoadedServer, *Diagnostic) {
	object, ok := raw.(map[string]any)
	if !ok {
		return LoadedServer{}, diagPtr(errorDiag("plugin_mcp_server_invalid", shortName+": server entry must be an object"))
	}
	typeValue, _ := object["type"].(string)
	switch typeValue {
	case "stdio":
		return parseStdioServer(shortName, object, pluginRoot, dataDir, pluginName)
	case "streamable-http":
		return parseRemoteServer(shortName, object, pluginName, false)
	case "sse":
		return parseRemoteServer(shortName, object, pluginName, true)
	case "":
		return LoadedServer{}, diagPtr(errorDiag("plugin_mcp_server_invalid", shortName+": type is required"))
	default:
		return LoadedServer{}, diagPtr(errorDiag("plugin_mcp_server_unsupported", shortName+": unsupported transport "+typeValue))
	}
}

func parseStdioServer(shortName string, object map[string]any, pluginRoot, dataDir, pluginName string) (LoadedServer, *Diagnostic) {
	for field := range object {
		switch field {
		case "type", "command", "args", "env", "cwd":
		default:
			return LoadedServer{}, diagPtr(errorDiag("plugin_mcp_server_invalid", shortName+": unknown field "+field))
		}
	}
	command, ok := object["command"].(string)
	if !ok || strings.TrimSpace(command) == "" {
		return LoadedServer{}, diagPtr(errorDiag("plugin_mcp_server_invalid", shortName+": command is required"))
	}
	command = strings.TrimSpace(command)
	if strings.ContainsAny(command, " \t") {
		return LoadedServer{}, diagPtr(errorDiag("plugin_mcp_server_invalid", shortName+": command must be a single executable token"))
	}
	resolvedCommand := command
	if strings.HasPrefix(command, "./") {
		abs, err := resolvePluginRelativePath(pluginRoot, command, "command")
		if err != nil {
			return LoadedServer{}, diagPtr(errorDiag("plugin_mcp_server_invalid", shortName+": "+err.Error()))
		}
		if err := resolvedStayWithin(pluginRoot, abs); err != nil {
			return LoadedServer{}, diagPtr(errorDiag("plugin_mcp_server_invalid", shortName+": command escapes the plugin root"))
		}
		resolvedCommand = abs
	} else if strings.Contains(command, "/") || strings.Contains(command, "\\") {
		return LoadedServer{}, diagPtr(errorDiag("plugin_mcp_server_invalid", shortName+": command must be a bare name or a ./ plugin-relative path"))
	}

	args, err := optionalStringArray(object, "args")
	if err != nil {
		return LoadedServer{}, diagPtr(errorDiag("plugin_mcp_server_invalid", shortName+": "+err.Error()))
	}
	env, err := optionalStringMap(object, "env")
	if err != nil {
		return LoadedServer{}, diagPtr(errorDiag("plugin_mcp_server_invalid", shortName+": "+err.Error()))
	}
	for key := range env {
		if strings.EqualFold(key, "PLUGIN_ROOT") || strings.EqualFold(key, "PLUGIN_DATA") {
			return LoadedServer{}, diagPtr(errorDiag("plugin_mcp_server_invalid", shortName+": env must not contain PLUGIN_ROOT or PLUGIN_DATA"))
		}
	}
	cwdRaw, err := optionalString(object, "cwd")
	if err != nil {
		return LoadedServer{}, diagPtr(errorDiag("plugin_mcp_server_invalid", shortName+": "+err.Error()))
	}

	expandedArgs := make([]string, len(args))
	for i, arg := range args {
		expandedArgs[i] = expandPluginPlaceholders(arg, pluginRoot, dataDir)
	}
	expandedEnv := make(map[string]string, len(env))
	for key, value := range env {
		expandedEnv[key] = expandPluginPlaceholders(value, pluginRoot, dataDir)
	}
	cwd, err := resolveStdioCwd(cwdRaw, pluginRoot, dataDir)
	if err != nil {
		return LoadedServer{}, diagPtr(errorDiag("plugin_mcp_server_invalid", shortName+": "+err.Error()))
	}

	return LoadedServer{
		ShortName: shortName,
		Identity:  ServerIdentity(pluginName, shortName),
		Config: mcpclient.McpServerConfig{
			Stdio: &mcpclient.StdioTransportConfig{
				Command:    resolvedCommand,
				Args:       expandedArgs,
				Env:        expandedEnv,
				Cwd:        cwd,
				PluginRoot: pluginRoot,
				PluginData: dataDir,
			},
		},
	}, nil
}

func parseRemoteServer(shortName string, object map[string]any, pluginName string, sse bool) (LoadedServer, *Diagnostic) {
	for field := range object {
		switch field {
		case "type", "url", "headers":
		default:
			return LoadedServer{}, diagPtr(errorDiag("plugin_mcp_server_invalid", shortName+": unknown field "+field))
		}
	}
	rawURL, ok := object["url"].(string)
	if !ok || strings.TrimSpace(rawURL) == "" {
		return LoadedServer{}, diagPtr(errorDiag("plugin_mcp_server_invalid", shortName+": url is required"))
	}
	if err := validateRemoteMCPURL(rawURL); err != nil {
		return LoadedServer{}, diagPtr(errorDiag("plugin_mcp_server_invalid", shortName+": "+err.Error()))
	}
	headers, err := optionalStringMap(object, "headers")
	if err != nil {
		return LoadedServer{}, diagPtr(errorDiag("plugin_mcp_server_invalid", shortName+": "+err.Error()))
	}
	if err := validateHeaders(headers); err != nil {
		return LoadedServer{}, diagPtr(errorDiag("plugin_mcp_server_invalid", shortName+": "+err.Error()))
	}
	httpCfg := &mcpclient.HTTPTransportConfig{URL: strings.TrimSpace(rawURL), Headers: headers}
	cfg := mcpclient.McpServerConfig{}
	if sse {
		cfg.SSE = httpCfg
	} else {
		cfg.HTTP = httpCfg
	}
	return LoadedServer{
		ShortName: shortName,
		Identity:  ServerIdentity(pluginName, shortName),
		Config:    cfg,
	}, nil
}

func resolveStdioCwd(raw, pluginRoot, dataDir string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return pluginRoot, nil
	}
	expanded := expandPluginPlaceholders(raw, pluginRoot, dataDir)
	switch {
	case strings.HasPrefix(raw, "./"):
		abs, err := resolvePluginRelativePath(pluginRoot, raw, "cwd")
		if err != nil {
			return "", err
		}
		if err := resolvedStayWithin(pluginRoot, abs); err != nil {
			return "", fmt.Errorf("cwd escapes the plugin root")
		}
		return abs, nil
	case raw == "${PLUGIN_ROOT}" || strings.HasPrefix(raw, "${PLUGIN_ROOT}/"):
		if err := resolvedStayWithin(pluginRoot, expanded); err != nil {
			return "", fmt.Errorf("cwd escapes the plugin root")
		}
		return expanded, nil
	case raw == "${PLUGIN_DATA}" || strings.HasPrefix(raw, "${PLUGIN_DATA}/"):
		if dataDir == "" {
			return "", fmt.Errorf("cwd references PLUGIN_DATA but the data directory is missing")
		}
		if err := stayWithin(dataDir, expanded); err != nil {
			return "", fmt.Errorf("cwd escapes the plugin data directory")
		}
		return expanded, nil
	default:
		return "", fmt.Errorf("cwd must be plugin-relative, ${PLUGIN_ROOT}, or ${PLUGIN_DATA}")
	}
}

func expandPluginPlaceholders(value, pluginRoot, dataDir string) string {
	replaced := strings.ReplaceAll(value, "${PLUGIN_ROOT}", pluginRoot)
	return strings.ReplaceAll(replaced, "${PLUGIN_DATA}", dataDir)
}

func optionalStringMap(object map[string]any, field string) (map[string]string, error) {
	raw, ok := object[field]
	if !ok || raw == nil {
		return nil, nil
	}
	items, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an object of strings", field)
	}
	out := make(map[string]string, len(items))
	for key, value := range items {
		text, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("%s.%s must be a string", field, key)
		}
		out[key] = text
	}
	return out, nil
}

func validateRemoteMCPURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("url is not a valid URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("url must be an absolute HTTP or HTTPS URL")
	}
	if parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return fmt.Errorf("url must not contain user information or a fragment")
	}
	if parsed.Scheme == "http" && !isLoopbackHost(parsed.Hostname()) {
		return fmt.Errorf("non-loopback endpoints must use HTTPS")
	}
	return nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func validateHeaders(headers map[string]string) error {
	seen := map[string]string{}
	for name := range headers {
		if strings.TrimSpace(name) == "" || strings.ContainsAny(name, "\r\n") {
			return fmt.Errorf("header name %q is invalid", name)
		}
		folded := strings.ToLower(name)
		if previous, ok := seen[folded]; ok && previous != name {
			return fmt.Errorf("header %q is duplicated under different casing", name)
		}
		seen[folded] = name
	}
	return nil
}

func ServerIdentity(pluginName, serverName string) string {
	return PluginIdentityPrefix + pluginName + "__" + serverName
}

func IsPluginServerIdentity(name string) bool {
	return strings.HasPrefix(name, PluginIdentityPrefix)
}

func diagPtr(d Diagnostic) *Diagnostic {
	return &d
}
