package plugins

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"awaken/src/mcpclient"
	"awaken/src/plugins/ir"
	pluginvariables "awaken/src/plugins/variables"
)

func (s *Store) loadVendorMCPServers(plugin *InstalledPlugin, record Record) ([]LoadedServer, []Diagnostic) {
	if plugin == nil || plugin.Dialect == ir.DialectAgentPlugins {
		return nil, nil
	}
	values, diagnostics := s.pluginVariableValues(plugin, record)
	context := pluginvariables.InterpolationContext{PluginRoot: plugin.Root, PluginData: plugin.DataDir, Values: values}
	var servers []LoadedServer
	for _, component := range plugin.Components {
		if component.Kind != ir.KindMCPServer || component.MCP == nil || component.MCP.HostConnectorID != "" || component.Validation == "invalid" || component.Mapping != ir.MappingAdapted && component.Mapping != ir.MappingNative {
			continue
		}
		loaded, invalid := translateVendorMCP(component, plugin, context)
		if invalid != nil {
			loaded = LoadedServer{ShortName: component.MCP.ShortName, Identity: ServerIdentity(plugin.Name, component.MCP.ShortName), Invalid: invalid}
			diagnostics = append(diagnostics, *invalid)
		}
		enabled := record.Enabled
		if toggle, ok := record.MCPServers[component.MCP.ShortName]; ok {
			enabled = enabled && toggle.Enabled
		}
		if override, ok := record.Components[component.LogicalID]; ok && override.Desired == "disabled" {
			enabled = false
		}
		loaded.Enabled = enabled
		loaded.Config.Enabled = boolPointer(enabled)
		if override, ok := record.Components[component.LogicalID]; ok {
			for toolName, toolEnabled := range override.ToolOverrides {
				if !toolEnabled {
					loaded.Config.DisabledTools = append(loaded.Config.DisabledTools, toolName)
				}
			}
			sort.Strings(loaded.Config.DisabledTools)
		}
		servers = append(servers, loaded)
	}
	sort.Slice(servers, func(i, j int) bool { return servers[i].ShortName < servers[j].ShortName })
	return servers, diagnostics
}

func (s *Store) pluginVariableValues(plugin *InstalledPlugin, record Record) (map[string]string, []Diagnostic) {
	values := map[string]string{}
	var diagnostics []Diagnostic
	for _, component := range plugin.Components {
		if component.Variable == nil {
			continue
		}
		spec := component.Variable
		stored, configured := record.Variables[spec.Key]
		if configured && stored.SecretRef != "" {
			if s.secrets == nil {
				diagnostics = append(diagnostics, errorDiag("plugin_variable_secret_unavailable", spec.Key+" secret store is unavailable"))
				continue
			}
			secret, err := s.secrets.Load(stored.SecretRef)
			if err != nil {
				diagnostics = append(diagnostics, errorDiag("plugin_variable_secret_unavailable", spec.Key+": "+err.Error()))
				continue
			}
			values[spec.Key] = secret
			continue
		}
		if configured && stored.Value != nil {
			values[spec.Key] = variableString(stored.Value)
			continue
		}
		if spec.Default != nil {
			values[spec.Key] = variableString(spec.Default)
		}
	}
	return values, diagnostics
}

func variableString(value any) string {
	switch typed := value.(type) {
	case []string:
		return strings.Join(typed, string(os.PathListSeparator))
	case []any:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			items = append(items, fmt.Sprint(item))
		}
		return strings.Join(items, string(os.PathListSeparator))
	default:
		return fmt.Sprint(value)
	}
}

func translateVendorMCP(component ir.Component, plugin *InstalledPlugin, context pluginvariables.InterpolationContext) (LoadedServer, *Diagnostic) {
	spec := component.MCP
	invalid := func(err error) (LoadedServer, *Diagnostic) {
		return LoadedServer{}, diagPtr(errorDiag("plugin_mcp_server_invalid", spec.ShortName+": "+err.Error()))
	}
	loaded := LoadedServer{ShortName: spec.ShortName, Identity: ServerIdentity(plugin.Name, spec.ShortName)}
	switch spec.Transport {
	case "stdio":
		command, err := pluginvariables.Expand(spec.Command, plugin.Dialect, "mcp.command", context)
		if err != nil {
			return invalid(err)
		}
		if strings.HasPrefix(command, "./") {
			command, err = resolvePluginRelativePath(plugin.Root, command, "command")
			if err != nil {
				return invalid(err)
			}
			info, statErr := os.Lstat(command)
			if statErr != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
				return invalid(fmt.Errorf("bundled command must be a regular file"))
			}
			if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
				return invalid(fmt.Errorf("bundled command is not executable"))
			}
		} else if strings.Contains(command, "/") || strings.Contains(command, "\\") {
			return invalid(fmt.Errorf("command must be a bare executable name or ./ plugin-relative path"))
		}
		args := make([]string, 0, len(spec.Args))
		for _, raw := range spec.Args {
			expanded, expandErr := pluginvariables.Expand(raw, plugin.Dialect, "mcp.args", context)
			if expandErr != nil {
				return invalid(expandErr)
			}
			args = append(args, expanded)
		}
		env := make(map[string]string, len(spec.Env))
		for key, raw := range spec.Env {
			if environmentNameEqual(key, "PLUGIN_ROOT") || environmentNameEqual(key, "PLUGIN_DATA") {
				return invalid(fmt.Errorf("env must not override PLUGIN_ROOT or PLUGIN_DATA"))
			}
			expanded, expandErr := pluginvariables.Expand(raw, plugin.Dialect, "mcp.env", context)
			if expandErr != nil {
				return invalid(expandErr)
			}
			env[key] = expanded
		}
		cwd := plugin.Root
		if spec.Cwd != "" {
			cwd, err = pluginvariables.Expand(spec.Cwd, plugin.Dialect, "mcp.cwd", context)
			if err != nil {
				return invalid(err)
			}
			if !filepath.IsAbs(cwd) {
				cwd = filepath.Join(plugin.Root, filepath.FromSlash(cwd))
			}
			if resolvedStayWithin(plugin.Root, cwd) != nil && resolvedStayWithin(plugin.DataDir, cwd) != nil {
				return invalid(fmt.Errorf("cwd must stay within PLUGIN_ROOT or PLUGIN_DATA"))
			}
		}
		loaded.Config.Stdio = &mcpclient.StdioTransportConfig{Command: command, Args: args, Env: env, Cwd: cwd, PluginRoot: plugin.Root, PluginData: plugin.DataDir}
	case "streamable-http", "sse":
		rawURL, err := pluginvariables.Expand(spec.URL, plugin.Dialect, "mcp.url", context)
		if err != nil {
			return invalid(err)
		}
		if _, parseErr := url.ParseRequestURI(rawURL); parseErr != nil {
			return invalid(fmt.Errorf("invalid remote URL: %w", parseErr))
		}
		if err := validateRemoteMCPURL(rawURL); err != nil {
			return invalid(err)
		}
		headers := make(map[string]string, len(spec.Headers))
		for key, raw := range spec.Headers {
			expanded, expandErr := pluginvariables.Expand(raw, plugin.Dialect, "mcp.headers", context)
			if expandErr != nil {
				return invalid(expandErr)
			}
			headers[key] = expanded
		}
		if err := validateHeaders(headers); err != nil {
			return invalid(err)
		}
		remote := &mcpclient.HTTPTransportConfig{URL: rawURL, Headers: headers}
		if resource := strings.TrimSpace(spec.OAuthResource); resource != "" {
			expandedResource, expandErr := pluginvariables.Expand(resource, plugin.Dialect, "mcp.url", context)
			if expandErr != nil {
				return invalid(expandErr)
			}
			if _, parseErr := url.ParseRequestURI(expandedResource); parseErr != nil {
				return invalid(fmt.Errorf("invalid OAuth resource URI: %w", parseErr))
			}
			loaded.Config.OAuthResource = expandedResource
		}
		loaded.Config.OAuthClientID = strings.TrimSpace(spec.OAuthClientID)
		if metadataURL := strings.TrimSpace(spec.OAuthClientMetadataURL); metadataURL != "" {
			parsedMetadataURL, parseErr := url.Parse(metadataURL)
			if parseErr != nil || parsedMetadataURL.Scheme != "https" || parsedMetadataURL.Host == "" || parsedMetadataURL.Path == "" || parsedMetadataURL.Path == "/" || parsedMetadataURL.User != nil || parsedMetadataURL.Fragment != "" {
				return invalid(fmt.Errorf("invalid OAuth client metadata URL: must be a non-root public HTTPS URL"))
			}
			if err := validateRemoteMCPURL(metadataURL); err != nil {
				return invalid(fmt.Errorf("invalid OAuth client metadata URL: %w", err))
			}
			loaded.Config.OAuthClientMetadataURL = metadataURL
		}
		loaded.Config.OAuthScopes = append([]string(nil), spec.OAuthScopes...)
		if spec.Transport == "sse" {
			loaded.Config.SSE = remote
		} else {
			loaded.Config.HTTP = remote
		}
	default:
		return invalid(fmt.Errorf("unsupported transport %q", spec.Transport))
	}
	loaded.Config.DisplayName = firstNonEmptyPlugin(component.DisplayName, spec.ShortName)
	return loaded, nil
}

func boolPointer(value bool) *bool { return &value }
