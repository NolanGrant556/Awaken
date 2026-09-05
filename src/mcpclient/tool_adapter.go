package mcpclient

import (
	"context"
	"encoding/base32"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"awaken/src/internal/outputlimit"
	"awaken/src/tool"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	pluginIdentityPrefix = "plugin__"
	mcpEncodedSentinel   = "x0_"
)

var mcpNameEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

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
	errorReadOnly   bool // Original, internally consistent provider declaration.
	destructive     bool
	openWorld       bool
	requiresAuth    bool
	capabilityClass tool.CapabilityClass
	trustedOrigin   tool.TrustedOrigin
	fileBridgeScope tool.FileBridgeScope
	networkScope    tool.NetworkScope
	// inputSchemaJSON preserves the raw JSON Schema for callers that need
	// the original structure rather than the flattened ToolParameter list.
	inputSchemaJSON json.RawMessage
	// outputSchemaJSON preserves the server-authored MCP output schema. The
	// adapted form is details-only and may use outputWrapperKey when an external
	// property would otherwise collide with ExecutionOutcome's envelope.
	outputSchemaJSON      json.RawMessage
	outputSchema          tool.ToolParam
	outputWrapperKey      string
	outputSchemaWarning   []string
	providerMetadata      MCPToolProviderMetadata
	artifactOutputEnabled bool
	providerManifest      tool.ProviderManifest
}

// McpToolAdapterOption configures an adapter at construction.
type McpToolAdapterOption func(*McpToolAdapter)

// WithInputSchemaJSON attaches the raw JSON Schema from the MCP server.
func WithInputSchemaJSON(raw json.RawMessage) McpToolAdapterOption {
	return func(a *McpToolAdapter) {
		a.inputSchemaJSON = append(json.RawMessage(nil), raw...)
	}
}

// NewMcpToolAdapter builds an adapter from an MCP tool definition. The
// annotations are derived from the SDK's *mcp.ToolAnnotations hints when
// present; conservative defaults are applied otherwise.
func NewMcpToolAdapter(manager *Manager, serverName string, mcpTool *mcp.Tool, opts ...McpToolAdapterOption) *McpToolAdapter {
	adapter := &McpToolAdapter{
		manager:          manager,
		serverName:       serverName,
		toolName:         mcpTool.Name,
		prefixedName:     McpToolName(serverName, mcpTool.Name),
		description:      mcpTool.Description,
		parameters:       convertMcpInputSchema(mcpTool.InputSchema),
		inputSchemaJSON:  marshalMcpSchema(mcpTool.InputSchema),
		outputSchemaJSON: marshalMcpSchema(mcpTool.OutputSchema),
		fileBridgeScope:  tool.FileBridgeNone,
		networkScope:     tool.NetworkScopeRestricted,
	}
	if mcpTool.OutputSchema == nil {
		// MCP servers may return StructuredContent without declaring an
		// outputSchema. Absence means "untyped", not "no details allowed".
		// Keep the result compatibility-graded while accepting the server's
		// object-shaped structured payload without a false mismatch warning.
		adapter.outputSchema = tool.ToolParam{
			Type:                 "object",
			AdditionalProperties: tool.BoolPtr(true),
		}
	} else {
		adapter.loadOutputSchema()
	}
	if mcpTool.Annotations != nil {
		adapter.errorReadOnly = mcpTool.Annotations.ReadOnlyHint && (mcpTool.Annotations.DestructiveHint == nil || !*mcpTool.Annotations.DestructiveHint)
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
	adapter.resolveProviderManifest(mcpTool)
	return adapter
}

// McpToolName formats the namespaced identifier for an MCP tool.
func McpToolName(serverName, toolName string) string {
	return McpToolPrefix(serverName) + encodeMCPNameComponent(toolName)
}

// McpToolPrefix returns the registry-safe prefix shared by every tool from a
// raw MCP server identity. Agent Plugin server identities retain the
// plugin__<plugin>__<server> structure, while each model-facing component is
// made safe for provider function-name contracts.
func McpToolPrefix(serverName string) string {
	return "mcp__" + modelFacingMCPServerName(serverName) + "__"
}

func modelFacingMCPServerName(serverName string) string {
	rest, isPlugin := strings.CutPrefix(serverName, pluginIdentityPrefix)
	if !isPlugin {
		return encodeMCPNameComponent(serverName)
	}
	pluginName, serverShortName, ok := strings.Cut(rest, "__")
	if !ok || pluginName == "" || serverShortName == "" {
		return encodeMCPNameComponent(serverName)
	}

	return pluginIdentityPrefix + modelFacingPluginName(pluginName) + "__" + encodeMCPNameComponent(serverShortName)
}

// modelFacingPluginName uses the Agent Plugins manifest grammar to keep the
// common dotted name readable and length-stable. Underscore is not legal in a
// manifest name, so replacing dots with underscores cannot collide with a
// different valid plugin name. Inputs outside that grammar use the generic,
// reversible component encoding instead.
func modelFacingPluginName(pluginName string) string {
	if isAgentPluginNameComponent(pluginName) {
		return strings.ReplaceAll(pluginName, ".", "_")
	}
	return forceEncodeMCPNameComponent(pluginName)
}

func isAgentPluginNameComponent(value string) bool {
	if value == "" {
		return false
	}
	for i := 0; i < len(value); i++ {
		b := value[i]
		if (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9') || b == '.' || b == '-' {
			continue
		}
		return false
	}
	return true
}

func encodeMCPNameComponent(component string) string {
	if isSafeMCPNameComponent(component) && !strings.HasPrefix(component, mcpEncodedSentinel) {
		return component
	}
	return forceEncodeMCPNameComponent(component)
}

func forceEncodeMCPNameComponent(component string) string {
	return mcpEncodedSentinel + mcpNameEncoding.EncodeToString([]byte(component))
}

func decodeMCPNameComponent(component string) (string, bool) {
	if !strings.HasPrefix(component, mcpEncodedSentinel) {
		return component, component != ""
	}
	decoded, err := mcpNameEncoding.DecodeString(strings.TrimPrefix(component, mcpEncodedSentinel))
	if err != nil {
		return "", false
	}
	raw := string(decoded)
	if encodeMCPNameComponent(raw) != component {
		return "", false
	}
	return raw, raw != ""
}

func isSafeMCPNameComponent(component string) bool {
	if component == "" {
		return false
	}
	for i := 0; i < len(component); i++ {
		b := component[i]
		if (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '-' || b == '_' {
			continue
		}
		return false
	}
	return true
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

// InputSchema implements tool.InputSchemaProvider. MCP servers own their
// recursive JSON Schema, so the adapter prefers the original schema over the
// lossy flat Parameters representation. Unsupported JSON Schema keywords are
// ignored by Awaken's current schema type; when the supported subset cannot be
// decoded safely, validation falls back to the legacy flat parameters.
func (a *McpToolAdapter) InputSchema() tool.ToolParametersSchema {
	if len(a.inputSchemaJSON) > 0 {
		normalized, _, normalizeErr := normalizeMCPJSONSchema(a.inputSchemaJSON)
		var schema tool.ToolParametersSchema
		if normalizeErr == nil {
			if err := json.Unmarshal(normalized, &schema); err != nil {
				return tool.SchemaFromParameters(a.parameters)
			}
			if strings.TrimSpace(schema.Type) == "" && len(schema.Properties) > 0 {
				schema.Type = "object"
			}
			if strings.TrimSpace(schema.Type) != "" {
				return schema
			}
		}
	}
	return tool.SchemaFromParameters(a.parameters)
}

// IsReadOnly implements tool.ReadOnlyTool so the existing permission system
// auto-allows read-only MCP tools.
func (a *McpToolAdapter) IsReadOnly() bool { return a.readOnly }

// Definition implements tool.Tool.
func (a *McpToolAdapter) Definition() tool.ToolDefinition {
	return tool.ToolDefinition{
		Name:        a.prefixedName,
		DisplayName: a.prefixedName,
		// Every definition produced by this adapter is an MCP tool schema sent
		// to the model. Connector ownership remains available through the
		// ProviderManifest; it must not reclassify model-visible MCP schemas as
		// Awaken built-ins in context accounting.
		Source:       tool.SourceMCP,
		Description:  a.Description(),
		OutputSchema: a.outputSchema,
		ProviderID:   a.providerManifest.ProviderID,
		Capabilities: append([]tool.Capability(nil), a.providerManifest.Capabilities...),
		Dependencies: append([]tool.DependencyRequirement(nil), a.providerManifest.Dependencies...),
		SideEffects:  append([]tool.ToolSideEffect(nil), a.providerManifest.SideEffects...),
		Annotations: tool.ToolAnnotations{
			ReadOnly:            a.readOnly,
			Destructive:         a.destructive,
			OpenWorld:           a.openWorld,
			RequiresAuth:        a.requiresAuth,
			ServerName:          a.serverName,
			CanonicalServerName: modelFacingMCPServerName(a.serverName),
			ToolName:            a.toolName,
			CapabilityClass:     a.capabilityClass,
			TrustedOrigin:       a.trustedOrigin,
			FileBridgeScope:     a.fileBridgeScope,
			NetworkScope:        a.networkScope,
		},
	}
}

// CheckPermission implements tool.PermissionCheckingTool. A connected and
// authenticated MCP server is callable without Awaken-specific per-tool
// metadata or side-effect gates. The server's schema still validates input.
func (a *McpToolAdapter) CheckPermission(ctx context.Context, input string, permCtx tool.PermissionContext) tool.PermissionDecision {
	_ = ctx
	_ = permCtx
	if a.manager == nil {
		if _, err := parseArguments(input, a.parameters); err != nil {
			return tool.DenyPermission("invalid MCP tool arguments: " + err.Error())
		}
		return tool.AllowPermission()
	}
	snap, err := a.manager.SnapshotServer(a.serverName)
	if err != nil {
		return tool.DenyPermission("mcp server " + a.serverName + " not found")
	}
	if snap.State == StateNeedsAuth {
		return tool.NeedsAuthPermission("mcp server " + a.serverName + " requires authorization")
	}
	if snap.State != StateConnected {
		return tool.NeedsRuntimePermission("mcp server " + a.serverName + " is " + string(snap.State))
	}
	if _, err := parseArguments(input, a.parameters); err != nil {
		return tool.DenyPermission("invalid MCP tool arguments: " + err.Error())
	}
	return tool.AllowPermission()
}

// Execute implements the legacy tool.Tool execution contract. New runtime
// callers use ExecuteOutcome so MCP's IsError bit remains a machine-readable
// semantic signal. The wrapper maps a completed MCP business failure to a
// ToolError for older callers that only understand (content, error).
func (a *McpToolAdapter) Execute(ctx context.Context, input string) (string, error) {
	outcome, err := a.ExecuteOutcome(ctx, input, nil)
	if err != nil {
		return outcome.Content, err
	}
	if outcome.IsError() {
		return outcome.Content, tool.ToolError{
			Code:            outcome.ErrorCode,
			Category:        outcome.ErrorCategory,
			Message:         firstNonEmpty(outcome.ErrorMessage, outcome.Content),
			Retryable:       outcome.Retryable,
			RecoveryOptions: append([]string(nil), outcome.RecoveryOptions...),
			Diagnostic:      outcome.Diagnostic,
		}
	}
	return outcome.Content, nil
}

// ExecuteOutcome implements tool.OutcomeTool. MCP protocol/transport errors
// remain Go errors, while a server-returned CallToolResult with IsError=true is
// a completed transport carrying a failed semantic outcome.
func (a *McpToolAdapter) ExecuteOutcome(ctx context.Context, input string, _ tool.ToolObserver) (tool.ExecutionOutcome, error) {
	started := time.Now().UTC()
	arguments, err := parseArguments(input, a.parameters)
	if err != nil {
		content := fmt.Sprintf("invalid MCP tool arguments: %v", err)
		outcome := tool.FailedOutcome(content, tool.ErrorCodeValidationError, tool.ErrorCategoryValidation, false)
		outcome.TransportStatus = tool.TransportNotStarted
		outcome.RecoveryOptions = []string{"correct_tool_arguments"}
		outcome.Diagnostic = a.mcpDiagnostic(nil)
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}
	if a.manager == nil {
		err := errors.New("MCP manager is unavailable")
		outcome := a.callFailureOutcome(err)
		return outcome.WithTiming(started, time.Now().UTC()), err
	}
	result, err := a.manager.callServerTool(ctx, a.serverName, a.toolName, a.prefixedName, arguments)
	if err != nil {
		outcome := a.callFailureOutcome(err)
		return outcome.WithTiming(started, time.Now().UTC()), err
	}
	if a.artifactOutputEnabled {
		outcome := a.publishArtifactOutput(ctx, result)
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}
	outcome := a.callResultOutcome(result)
	return outcome.WithTiming(started, time.Now().UTC()), nil
}

const (
	errorCodeMCPReportedError  = "mcp_tool_reported_error"
	errorCodeMCPEmptyResult    = "mcp_empty_result"
	errorCodeMCPManagerMissing = "mcp_manager_unavailable"
	errorCodeMCPToolNotFound   = "mcp_tool_not_found"
	errorCodeMCPCallFailed     = "mcp_call_failed"
)

func (a *McpToolAdapter) callResultOutcome(result *mcp.CallToolResult) tool.ExecutionOutcome {
	if result == nil {
		outcome := tool.FailedOutcome(
			"MCP tool returned no result",
			errorCodeMCPEmptyResult,
			tool.ErrorCategoryRuntime,
			true,
		)
		outcome.RecoveryOptions = []string{"check_mcp_server_state", "retry_after_reconnect"}
		outcome.Diagnostic = a.mcpDiagnostic(nil)
		return outcome
	}

	content := formatCallToolResult(result)
	if result.IsError {
		if strings.TrimSpace(content) == "" {
			content = "MCP tool reported an error with no content"
		}
		outcome := tool.FailedOutcome(content, errorCodeMCPReportedError, tool.ErrorCategoryDomain, false)
		outcome.RecoveryOptions = []string{"inspect_tool_error", "adjust_arguments_or_strategy"}
		outcome.Diagnostic = a.mcpDiagnostic(result)
		a.applyErrorMetadata(&outcome, result)
		return outcome
	}

	outcome := tool.SucceededOutcome(content)
	outcome.Diagnostic = a.mcpDiagnostic(result)
	if details, ok := a.structuredDetails(result.StructuredContent); ok {
		outcome.Details = details
		delete(outcome.Diagnostic, "structured_content")
	}
	return outcome
}

func (a *McpToolAdapter) callFailureOutcome(err error) tool.ExecutionOutcome {
	content := firstNonEmpty(strings.TrimSpace(err.Error()), "MCP tool call failed")
	diagnostic := a.mcpDiagnostic(nil)
	diagnostic["cause"] = content

	var needsAuth *NeedsAuthError
	if errors.As(err, &needsAuth) {
		now := time.Now().UTC()
		return tool.ExecutionOutcome{
			Content:         content,
			Status:          tool.OutcomeNeedsAuth,
			TransportStatus: tool.TransportNotStarted,
			SemanticStatus:  tool.SemanticUnknown,
			ProcessStatus:   tool.ProcessNotApplicable,
			ErrorCode:       tool.ErrorCodeNeedsAuth,
			ErrorCategory:   tool.ErrorCategoryAuthorization,
			ErrorMessage:    content,
			Retryable:       false,
			RecoveryOptions: []string{"authenticate_mcp_server"},
			Diagnostic:      diagnostic,
			StartedAt:       now,
			FinishedAt:      now,
		}
	}

	var notFound *ToolNotFoundError
	if errors.As(err, &notFound) {
		now := time.Now().UTC()
		return tool.ExecutionOutcome{
			Content:         content,
			Status:          tool.OutcomeUnsupported,
			TransportStatus: tool.TransportNotStarted,
			SemanticStatus:  tool.SemanticFailure,
			ProcessStatus:   tool.ProcessNotApplicable,
			ErrorCode:       errorCodeMCPToolNotFound,
			ErrorCategory:   tool.ErrorCategoryUnsupported,
			ErrorMessage:    content,
			Retryable:       false,
			RecoveryOptions: []string{"refresh_mcp_tool_registry"},
			Diagnostic:      diagnostic,
			StartedAt:       now,
			FinishedAt:      now,
		}
	}

	code := errorCodeMCPCallFailed
	if a.manager == nil {
		code = errorCodeMCPManagerMissing
	}
	outcome := tool.FailedOutcome(content, code, tool.ErrorCategoryRuntime, true)
	outcome.TransportStatus = tool.TransportFailed
	outcome.RecoveryOptions = []string{"check_mcp_server_state", "retry_after_reconnect"}
	outcome.Diagnostic = diagnostic
	return outcome
}

func (a *McpToolAdapter) mcpDiagnostic(result *mcp.CallToolResult) map[string]any {
	diagnostic := map[string]any{
		"protocol":    "mcp",
		"server_name": a.serverName,
		"tool_name":   a.toolName,
	}
	if result == nil {
		return diagnostic
	}
	diagnostic["mcp_is_error"] = result.IsError
	diagnostic["content_items"] = len(result.Content)
	if structured, ok := jsonCompatibleValue(result.StructuredContent); ok {
		diagnostic["structured_content"] = structured
	}
	return diagnostic
}

func (a *McpToolAdapter) loadOutputSchema() {
	if len(a.outputSchemaJSON) == 0 {
		return
	}
	normalized, nullableUnion, err := normalizeMCPJSONSchema(a.outputSchemaJSON)
	if err != nil {
		a.outputSchemaWarning = []string{"MCP output schema uses unsupported JSON Schema forms and was not exposed as typed details"}
		return
	}
	var schema tool.ToolParam
	if err := json.Unmarshal(normalized, &schema); err != nil {
		a.outputSchemaWarning = []string{"MCP output schema uses unsupported JSON Schema forms and was not exposed as typed details"}
		return
	}
	adapted := tool.AdaptExternalDetailsOutputSchema(schema)
	a.outputSchema = adapted.Schema
	a.outputWrapperKey = adapted.WrapperKey
	a.outputSchemaWarning = append([]string(nil), adapted.Warnings...)
	if nullableUnion {
		a.outputSchemaWarning = append(a.outputSchemaWarning, "MCP output schema nullable type arrays were preserved as oneOf")
	}
}

// normalizeMCPJSONSchema translates JSON Schema's compact union form
// (`"type": ["null", "array"]`) into the equivalent oneOf form supported by
// Awaken's recursive ToolParam contract. The Go MCP SDK emits compact nullable
// unions for slices and pointers in both inputs and outputs. Rejecting an entire
// schema for one nested nullable property either hides the model's parameters
// or makes a successful result look like it returned undeclared fields.
func normalizeMCPJSONSchema(raw json.RawMessage) (json.RawMessage, bool, error) {
	var schema any
	if err := json.Unmarshal(raw, &schema); err != nil {
		return nil, false, err
	}
	changed, err := normalizeMCPTypeArrays(schema)
	if err != nil {
		return nil, false, err
	}
	normalized, err := json.Marshal(schema)
	if err != nil {
		return nil, false, err
	}
	return normalized, changed, nil
}

func normalizeMCPTypeArrays(value any) (bool, error) {
	changed := false
	switch current := value.(type) {
	case []any:
		for _, item := range current {
			itemChanged, err := normalizeMCPTypeArrays(item)
			if err != nil {
				return false, err
			}
			changed = changed || itemChanged
		}
	case map[string]any:
		for key, item := range current {
			if key == "type" {
				continue
			}
			itemChanged, err := normalizeMCPTypeArrays(item)
			if err != nil {
				return false, err
			}
			changed = changed || itemChanged
		}
		types, ok := current["type"].([]any)
		if !ok {
			return changed, nil
		}
		if len(types) == 0 || current["oneOf"] != nil {
			return false, fmt.Errorf("unsupported MCP output schema type union")
		}
		options := make([]any, 0, len(types))
		for _, rawType := range types {
			typeName, ok := rawType.(string)
			if !ok || (!isJSONSchemaType(typeName) && typeName != "null") {
				return false, fmt.Errorf("unsupported MCP output schema type union")
			}
			option := make(map[string]any, len(current))
			for key, item := range current {
				if key == "type" || key == "description" {
					continue
				}
				option[key] = item
			}
			option["type"] = typeName
			options = append(options, option)
		}
		description, hasDescription := current["description"]
		for key := range current {
			delete(current, key)
		}
		if hasDescription {
			current["description"] = description
		}
		current["oneOf"] = options
		changed = true
	}
	return changed, nil
}

func (a *McpToolAdapter) structuredDetails(value any) (map[string]any, bool) {
	structured, ok := jsonCompatibleValue(value)
	if !ok {
		return nil, false
	}
	if a.outputWrapperKey != "" {
		return map[string]any{a.outputWrapperKey: structured}, true
	}
	details, ok := structured.(map[string]any)
	if !ok {
		// MCP output schemas and StructuredContent are object-shaped by
		// protocol. Preserve a non-object protocol violation in diagnostics
		// unless the schema adapter explicitly declared an mcp_value wrapper.
		return nil, false
	}
	return details, true
}

// ServerName returns the owning MCP server name.
func (a *McpToolAdapter) ServerName() string { return a.serverName }

// ToolName returns the original, unprefixed tool name from the server.
func (a *McpToolAdapter) ToolName() string { return a.toolName }

// OutputSchemaWarnings reports compatibility adaptations applied to an MCP
// server's schema. The registry remains permissive for external tools; callers
// may surface these warnings in diagnostics or audits.
func (a *McpToolAdapter) OutputSchemaWarnings() []string {
	return append([]string(nil), a.outputSchemaWarning...)
}

func (a *McpToolAdapter) resolveProviderManifest(mcpTool *mcp.Tool) {
	metadata, warnings, _ := decodeMCPToolProviderMetadata(mcpTool)
	a.providerMetadata = metadata
	providerContext := a.manager.adapterProviderContext(a.serverName)

	source := providerContext.serverManifest.Source
	if source == "" {
		if providerContext.connectorID != "" {
			source = tool.SourceConnector
		} else {
			source = tool.SourceMCP
		}
	}
	providerID := metadata.ProviderID
	if providerID == "" {
		providerID = providerContext.serverManifest.ProviderID
		if providerID != "" {
			providerID = tool.ProviderID(strings.TrimSuffix(string(providerID), ":") + ":" + a.toolName)
		}
	}
	if providerID == "" {
		providerID = tool.ProviderID(string(source) + ":" + a.serverName + ":" + a.toolName)
	}
	readiness := providerReadinessFromServerState(providerContext.state)
	if a.manager == nil {
		readiness = tool.ProviderUnavailable
	}
	dependencies := mergeDependencies(
		providerContext.serverManifest.Dependencies,
		metadata.Dependencies,
		[]tool.DependencyRequirement{{
			Kind:    tool.DependencyMCPConnector,
			Name:    a.serverName,
			ProbeID: "mcp.server." + a.serverName,
			Source:  string(source),
		}},
	)
	capabilities := mergeCapabilities(
		metadata.Capabilities,
		inferMCPProviderCapabilities(a.toolName, a.capabilityClass),
	)
	var sideEffects []tool.ToolSideEffect
	if metadata.ArtifactOutput != nil {
		if reason := a.validateArtifactOutputContract(capabilities); reason != "" {
			warnings = append(warnings, "artifact_output ignored: "+reason)
		} else {
			a.artifactOutputEnabled = true
		}
	}
	if a.artifactOutputEnabled {
		a.readOnly = false
		a.destructive = false
		a.capabilityClass = tool.CapabilityArtifactWrite
		a.fileBridgeScope = tool.FileBridgeTaskScopeWrite
		sideEffects = []tool.ToolSideEffect{
			{Kind: tool.SideEffectCreate, Resources: []string{"TaskScope output artifact"}, Description: "May create a host-validated artifact in writable output roots.", Reversibility: tool.ReversibilityConditional},
			{Kind: tool.SideEffectUpdate, Resources: []string{"TaskScope output artifact"}, Description: "May replace an output artifact after its preimage is moved to Trash.", Reversibility: tool.ReversibilityReversible},
		}
	}
	displayName := strings.TrimSpace(providerContext.serverManifest.DisplayName)
	if displayName == "" {
		displayName = a.serverName + " / " + a.toolName
	}
	a.providerManifest = tool.ProviderManifest{
		ProviderID:   providerID,
		ToolName:     a.prefixedName,
		DisplayName:  displayName,
		Source:       source,
		Capabilities: capabilities,
		Readiness:    readiness,
		Dependencies: dependencies,
		SideEffects:  sideEffects,
		Warnings:     append([]string(nil), warnings...),
	}
}

// ProviderManifest exposes stable implementation identity, capabilities and
// dependencies to the unified tool catalog.
func (a *McpToolAdapter) ProviderManifest() tool.ProviderManifest {
	manifest := a.providerManifest
	manifest.Capabilities = append([]tool.Capability(nil), manifest.Capabilities...)
	manifest.Dependencies = append([]tool.DependencyRequirement(nil), manifest.Dependencies...)
	manifest.SideEffects = append([]tool.ToolSideEffect(nil), manifest.SideEffects...)
	manifest.Warnings = append([]string(nil), manifest.Warnings...)
	return manifest
}

// ProviderReadiness projects the live MCP connection/auth state when a run
// availability snapshot is built. It performs no external call or mutation.
func (a *McpToolAdapter) ProviderReadiness(_ tool.ExecutionEnvironmentSnapshot) tool.ProviderReadinessReport {
	if a.manager == nil {
		return tool.ProviderReadinessReport{Readiness: tool.ProviderUnavailable, Reason: "MCP manager is unavailable"}
	}
	snap, err := a.manager.SnapshotServer(a.serverName)
	if err != nil {
		return tool.ProviderReadinessReport{Readiness: tool.ProviderUnavailable, Reason: err.Error()}
	}
	readiness := providerReadinessFromServerState(snap.State)
	report := tool.ProviderReadinessReport{Readiness: readiness}
	switch readiness {
	case tool.ProviderNeedsAuth:
		report.Reason = firstNonEmpty(strings.TrimSpace(snap.ErrorReason), "MCP server requires authentication")
		report.RetryableAfterRemediation = true
	case tool.ProviderUnavailable:
		report.Reason = firstNonEmpty(strings.TrimSpace(snap.ErrorReason), "MCP server is "+string(snap.State))
		report.RetryableAfterRemediation = snap.State == StatePending || snap.State == StateFailed || snap.State == StateDisabled
	}
	return report
}

func providerReadinessFromServerState(state ServerState) tool.ProviderReadiness {
	switch state {
	case StateConnected:
		return tool.ProviderReady
	case StateNeedsAuth:
		return tool.ProviderNeedsAuth
	case StatePending, StateFailed, StateDisabled:
		return tool.ProviderUnavailable
	default:
		return tool.ProviderUnavailable
	}
}

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
		NetworkScope:    a.networkScope,
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
	NetworkScope    tool.NetworkScope    `json:"networkScope,omitempty"`
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
	// A user-controlled server name is not provenance. Only a host-injected
	// connector identity may elevate an MCP server to an official origin.
	return tool.TrustedOriginThirdPartyMCP
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

// formatCallToolResult collapses the SDK's content slice into a single text
// string the agent layer can stream. IsError deliberately is not encoded in
// this text: ExecuteOutcome carries it as canonical semantic status and the
// legacy Execute wrapper exposes it as ToolError.
func formatCallToolResult(result *mcp.CallToolResult) string {
	if result == nil {
		return ""
	}
	var sb strings.Builder
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
		return ""
	}
	return strings.TrimRight(sb.String(), "\n")
}

func marshalMcpSchema(schema any) json.RawMessage {
	if schema == nil {
		return nil
	}
	raw, err := json.Marshal(schema)
	if err != nil || !json.Valid(raw) {
		return nil
	}
	return append(json.RawMessage(nil), raw...)
}

func jsonCompatibleValue(value any) (any, bool) {
	if value == nil {
		return nil, false
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, false
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, false
	}
	return decoded, true
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
	raw, _, err = normalizeMCPJSONSchema(raw)
	if err != nil {
		return nil
	}
	var parsed struct {
		Type       string `json:"type"`
		Properties map[string]struct {
			Type        string `json:"type"`
			Description string `json:"description"`
			OneOf       []struct {
				Type string `json:"type"`
			} `json:"oneOf"`
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
	names := make([]string, 0, len(parsed.Properties))
	for name := range parsed.Properties {
		names = append(names, name)
	}
	sort.Strings(names)
	params := make([]tool.ToolParameter, 0, len(parsed.Properties))
	for _, name := range names {
		prop := parsed.Properties[name]
		paramType := prop.Type
		desc := prop.Description
		if paramType == "" {
			for _, option := range prop.OneOf {
				if option.Type == "null" {
					continue
				}
				if paramType != "" || !isJSONSchemaType(option.Type) {
					paramType = ""
					break
				}
				paramType = option.Type
			}
		}
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

func excerpt(value string, max int) string {
	if max <= 0 {
		return value
	}
	if normalized, truncated := outputlimit.Prefix(value, max); !truncated {
		return normalized
	}
	prefix, _ := outputlimit.Prefix(value, max-1)
	return prefix + "…"
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
