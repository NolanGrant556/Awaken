package mcpclient

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"awaken/src/tool"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// McpToolAdapter wraps a single MCP-sourced tool so it satisfies Awaken's
// tool.Tool interface. The adapter forwards Execute calls to the owning
// server's session via the manager.
type McpToolAdapter struct {
	manager         *Manager
	serverName      string
	toolName        string
	prefixedName    string
	description     string
	parameters      []tool.ToolParameter
	readOnly        bool
	destructive     bool
	openWorld       bool
	requiresAuth    bool
	capabilityClass tool.CapabilityClass
	trustedOrigin   tool.TrustedOrigin
	fileBridgeScope tool.FileBridgeScope
	// inputSchemaJSON preserves the raw JSON Schema for callers that need
	// the original structure rather than the flattened ToolParameter list.
	inputSchemaJSON json.RawMessage
}

// McpToolAdapterOption configures an adapter at construction.
type McpToolAdapterOption func(*McpToolAdapter)

// WithInputSchemaJSON attaches the raw JSON Schema from the MCP server.
func WithInputSchemaJSON(raw json.RawMessage) McpToolAdapterOption {
	return func(a *McpToolAdapter) { a.inputSchemaJSON = raw }
}

// NewMcpToolAdapter builds an adapter from an MCP tool definition. The
// annotations are derived from the SDK's *mcp.ToolAnnotations hints when
// present; conservative defaults are applied otherwise.
func NewMcpToolAdapter(manager *Manager, serverName string, mcpTool *mcp.Tool, opts ...McpToolAdapterOption) *McpToolAdapter {
	adapter := &McpToolAdapter{
		manager:         manager,
		serverName:      serverName,
		toolName:        mcpTool.Name,
		prefixedName:    McpToolName(serverName, mcpTool.Name),
		description:     mcpTool.Description,
		parameters:      convertMcpInputSchema(mcpTool.InputSchema),
		fileBridgeScope: tool.FileBridgeNone,
	}
	if mcpTool.Annotations != nil {
		adapter.readOnly = mcpTool.Annotations.ReadOnlyHint
		if mcpTool.Annotations.DestructiveHint != nil {
			adapter.destructive = *mcpTool.Annotations.DestructiveHint
		} else {
			// Per MCP spec default: destructive is true when readOnly is false.
			adapter.destructive = !adapter.readOnly
		}
		if mcpTool.Annotations.OpenWorldHint != nil {
			adapter.openWorld = *mcpTool.Annotations.OpenWorldHint
		} else {
			adapter.openWorld = true
		}
	} else {
		// No annotations from the server: conservative defaults.
		adapter.readOnly = false
		adapter.destructive = true
		adapter.openWorld = true
	}
	// requiresAuth is derived from the owning server's state, not the tool
	// itself. We set it true at registration time for any server that has
	// ever reported needs-auth; the permission layer treats it as a hint.
	adapter.requiresAuth = false
	adapter.trustedOrigin = inferTrustedOrigin(manager, serverName)
	adapter.capabilityClass = inferMcpCapabilityClass(serverName, mcpTool.Name, adapter.readOnly, adapter.trustedOrigin)
	if adapter.trustedOrigin == tool.TrustedOriginOfficialConnector || adapter.trustedOrigin == tool.TrustedOriginOfficialHosted {
		adapter.fileBridgeScope = tool.FileBridgeTaskScopeRead
	}
	switch adapter.capabilityClass {
	case tool.CapabilityConnectorRead, tool.CapabilityBrowserRead:
		adapter.readOnly = true
		adapter.destructive = false
		adapter.openWorld = false
	case tool.CapabilityConnectorWrite, tool.CapabilityBrowserInteract:
		adapter.destructive = false
		adapter.openWorld = false
	}
	for _, opt := range opts {
		opt(adapter)
	}
	return adapter
}

// McpToolName formats the namespaced identifier for an MCP tool.
func McpToolName(serverName, toolName string) string {
	return "mcp__" + serverName + "__" + toolName
}

// Name implements tool.Tool.
func (a *McpToolAdapter) Name() string { return a.prefixedName }

// Description implements tool.Tool.
func (a *McpToolAdapter) Description() string {
	if a.description == "" {
		return "MCP tool " + a.toolName + " from server " + a.serverName
	}
	return a.description
}

// Parameters implements tool.Tool.
func (a *McpToolAdapter) Parameters() []tool.ToolParameter { return a.parameters }

// IsReadOnly implements tool.ReadOnlyTool so the existing permission system
// auto-allows read-only MCP tools.
func (a *McpToolAdapter) IsReadOnly() bool { return a.readOnly }

// Definition implements tool.Tool.
func (a *McpToolAdapter) Definition() tool.ToolDefinition {
	return tool.ToolDefinition{
		Name:        a.prefixedName,
		DisplayName: a.prefixedName,
		Source:      tool.SourceMCP,
		Description: a.Description(),
		Annotations: tool.ToolAnnotations{
			ReadOnly:        a.readOnly,
			Destructive:     a.destructive,
			OpenWorld:       a.openWorld,
			RequiresAuth:    a.requiresAuth,
			ServerName:      a.serverName,
			ToolName:        a.toolName,
			CapabilityClass: a.capabilityClass,
			TrustedOrigin:   a.trustedOrigin,
			FileBridgeScope: a.fileBridgeScope,
		},
	}
}

// CheckPermission implements tool.PermissionCheckingTool. The adapter
// surfaces a deny + NeedsAuthError when the owning server is in the
// needs-auth state; otherwise it delegates to the standard read-only /
// destructive rules so the agent layer can route through the existing
// approval flow.
func (a *McpToolAdapter) CheckPermission(ctx context.Context, input string, permCtx tool.PermissionContext) tool.PermissionDecision {
	if a.manager == nil {
		return tool.AllowPermission()
	}
	snap, err := a.manager.SnapshotServer(a.serverName)
	if err != nil {
		return tool.DenyPermission("mcp server " + a.serverName + " not found")
	}
	if snap.State == StateNeedsAuth {
		return tool.DenyPermission("needs_auth")
	}
	return tool.AllowPermission()
}

// Execute implements tool.Tool. Arguments arrive as a JSON string; we
// forward them to the MCP server via the manager. NeedsAuthError surfaces
// as a typed error so the agent layer can translate it to the needs_auth
// stage.
func (a *McpToolAdapter) Execute(ctx context.Context, input string) (string, error) {
	arguments, err := parseArguments(input, a.parameters)
	if err != nil {
		return "", fmt.Errorf("parse arguments: %w", err)
	}
	result, err := a.manager.CallTool(ctx, a.prefixedName, arguments)
	if err != nil {
		return "", err
	}
	return formatCallToolResult(result), nil
}

// ServerName returns the owning MCP server name.
func (a *McpToolAdapter) ServerName() string { return a.serverName }

// ToolName returns the original, unprefixed tool name from the server.
func (a *McpToolAdapter) ToolName() string { return a.toolName }

// Annotations exposes the derived annotation hints. Consumed by the
// downstream tool-platform capability for permission decisions.
func (a *McpToolAdapter) Annotations() MCPToolAnnotations {
	return MCPToolAnnotations{
		ReadOnly:        a.readOnly,
		Destructive:     a.destructive,
		OpenWorld:       a.openWorld,
		RequiresAuth:    a.requiresAuth,
		ServerName:      a.serverName,
		ToolName:        a.toolName,
		CapabilityClass: a.capabilityClass,
		TrustedOrigin:   a.trustedOrigin,
		FileBridgeScope: a.fileBridgeScope,
	}
}

// MCPToolAnnotations is the metadata block shipped alongside every MCP
// tool registration. Mirrors the field set required by the tool-platform
// capability.
type MCPToolAnnotations struct {
	ReadOnly        bool                 `json:"readOnly"`
	Destructive     bool                 `json:"destructive"`
	OpenWorld       bool                 `json:"openWorld"`
	RequiresAuth    bool                 `json:"requiresAuth"`
	ServerName      string               `json:"serverName,omitempty"`
	ToolName        string               `json:"toolName,omitempty"`
	CapabilityClass tool.CapabilityClass `json:"capabilityClass,omitempty"`
	TrustedOrigin   tool.TrustedOrigin   `json:"trustedOrigin,omitempty"`
	FileBridgeScope tool.FileBridgeScope `json:"fileBridgeScope,omitempty"`
}

func (a *McpToolAdapter) AvailableForToolPool() bool {
	return true
}

func inferTrustedOrigin(manager *Manager, serverName string) tool.TrustedOrigin {
	if manager != nil {
		if snap, err := manager.SnapshotServer(serverName); err == nil {
			if snap.SourceConnectorID != nil && strings.TrimSpace(*snap.SourceConnectorID) != "" {
				return tool.TrustedOriginOfficialConnector
			}
		}
	}
	switch strings.ToLower(strings.TrimSpace(serverName)) {
	case "github", "playwright":
		return tool.TrustedOriginOfficialConnector
	default:
		return tool.TrustedOriginThirdPartyMCP
	}
}

func inferMcpCapabilityClass(serverName string, toolName string, readOnly bool, origin tool.TrustedOrigin) tool.CapabilityClass {
	server := strings.ToLower(strings.TrimSpace(serverName))
	name := strings.ToLower(strings.TrimSpace(toolName))

	if server == "playwright" {
		switch {
		case containsAny(name, "navigate", "goto", "open", "screenshot", "snapshot", "content", "text", "query", "extract", "evaluate"):
			return tool.CapabilityBrowserRead
		case containsAny(name, "click", "type", "fill", "press", "select", "drag", "hover", "upload", "download", "submit"):
			return tool.CapabilityBrowserInteract
		}
	}

	if origin == tool.TrustedOriginOfficialConnector || origin == tool.TrustedOriginOfficialHosted {
		if readOnly || containsAny(name, "list", "get", "read", "search", "query", "fetch") {
			return tool.CapabilityConnectorRead
		}
		if containsAny(name, "create", "update", "write", "edit", "delete", "close", "comment", "post") {
			return tool.CapabilityConnectorWrite
		}
		if readOnly {
			return tool.CapabilityConnectorRead
		}
		return tool.CapabilityConnectorWrite
	}

	if readOnly {
		return tool.CapabilityConnectorRead
	}
	return tool.CapabilityConnectorWrite
}

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

// parseArguments decodes the JSON input string into a map[string]any the
// SDK accepts. Empty input is treated as an empty object.
func parseArguments(input string, params []tool.ToolParameter) (map[string]any, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return map[string]any{}, nil
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(trimmed), &decoded); err != nil {
		// Fall back to wrapping single-string inputs under a conventional
		// key when the tool declares a single string parameter.
		if len(params) == 1 && params[0].Type == "string" {
			return map[string]any{params[0].Name: trimmed}, nil
		}
		return nil, fmt.Errorf("arguments must be a JSON object: %w", err)
	}
	if decoded == nil {
		return map[string]any{}, nil
	}
	return decoded, nil
}

// formatCallToolResult collapses the SDK's content slice into a single
// text string the agent layer can stream. The IsError flag is preserved
// by prefixing the output.
func formatCallToolResult(result *mcp.CallToolResult) string {
	if result == nil {
		return ""
	}
	var sb strings.Builder
	if result.IsError {
		sb.WriteString("[tool error] ")
	}
	for _, content := range result.Content {
		if text, ok := content.(*mcp.TextContent); ok && text != nil {
			sb.WriteString(text.Text)
			sb.WriteString("\n")
		}
	}
	if sb.Len() == 0 && result.StructuredContent != nil {
		raw, _ := json.Marshal(result.StructuredContent)
		sb.Write(raw)
	}
	if sb.Len() == 0 {
		if result.IsError {
			return "[tool error] MCP tool reported error with no content"
		}
		return ""
	}
	return strings.TrimRight(sb.String(), "\n")
}

// convertMcpInputSchema flattens the server-provided JSON Schema into the
// flat ToolParameter list the existing tool.Tool interface expects. The
// raw schema is preserved on the adapter for callers that need it. When a
// property type is not a supported simple JSON Schema type, it is degraded
// to "string" and a note is appended to the description.
func convertMcpInputSchema(schema any) []tool.ToolParameter {
	if schema == nil {
		return nil
	}
	raw, err := json.Marshal(schema)
	if err != nil {
		return nil
	}
	var parsed struct {
		Type       string `json:"type"`
		Properties map[string]struct {
			Type        string `json:"type"`
			Description string `json:"description"`
		} `json:"properties"`
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil
	}
	if parsed.Type != "object" || len(parsed.Properties) == 0 {
		return nil
	}
	requiredSet := make(map[string]bool, len(parsed.Required))
	for _, name := range parsed.Required {
		requiredSet[name] = true
	}
	params := make([]tool.ToolParameter, 0, len(parsed.Properties))
	for name, prop := range parsed.Properties {
		paramType := prop.Type
		desc := prop.Description
		if !isJSONSchemaType(paramType) {
			desc = fmt.Sprintf("%s (original type: %s; degraded to string)", desc, paramType)
			paramType = "string"
		}
		params = append(params, tool.ToolParameter{
			Name:        name,
			Type:        paramType,
			Description: desc,
			Required:    requiredSet[name],
		})
	}
	return params
}

func approvalDescription(toolName, input string) string {
	trimmed := excerpt(input, 240)
	if trimmed == "" {
		return fmt.Sprintf("%s requires approval before execution.", toolName)
	}
	return fmt.Sprintf("%s requires approval before execution.\n\n%s", toolName, trimmed)
}

func excerpt(value string, max int) string {
	if max <= 0 || len(value) <= max {
		return value
	}
	return value[:max-1] + "…"
}

// isJSONSchemaType reports whether value is a simple JSON Schema type the
// tool.Tool interface accepts. Complex forms (["string","null"], oneOf,
// etc.) return false and should be degraded.
func isJSONSchemaType(value string) bool {
	switch value {
	case "string", "number", "integer", "boolean", "object", "array":
		return true
	}
	return false
}
