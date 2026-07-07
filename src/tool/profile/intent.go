// Package profile defines ExecutionIntent, the unified type that every tool
// call is converted into before execution. This is the foundational type for
// Phase 3 (unified execution profile) of the "零审批自动执行策略" spec:
// every ToolCall — whether builtin (apply_patch, shell, file_read, ...),
// MCP-sourced, connector-sourced, or issued by a sub-agent — is normalised
// into an ExecutionIntent so that the decision layer can reason about a
// single shape.
package profile

import (
	"encoding/json"
	"fmt"
	"strings"

	convcontext "awaken/src/context"
	"awaken/src/tool"
)

// ToolKind is the legacy execution-kind classifier kept for backward
// compatibility with older tests and call sites.
type ToolKind string

const (
	ToolKindFileChain    ToolKind = "file_chain"
	ToolKindShellCommand ToolKind = "shell_command"
	ToolKindMcpApp       ToolKind = "mcp_app"
	ToolKindReviewAgent  ToolKind = "review_agent"
)

// ExecutionLane is the current top-level routing key used by the executor
// layer. Product-facing capabilities route through lanes rather than exposing
// file/shell/mcp as first-class user concepts.
type ExecutionLane string

const (
	LaneArtifact     ExecutionLane = "artifact_lane"
	LaneConnector    ExecutionLane = "connector_lane"
	LaneLocalCompute ExecutionLane = "local_compute_lane"
	LaneSystem       ExecutionLane = "system_lane"
)

// NetworkScope grades the network reach of a tool invocation.
type NetworkScope string

const (
	NetworkScopeNone       NetworkScope = "none"
	NetworkScopeRestricted NetworkScope = "restricted"
	NetworkScopeOpen       NetworkScope = "open"
)

// RequestedAction describes the primary action the tool will perform.
type RequestedAction string

const (
	ActionRead    RequestedAction = "read"
	ActionWrite   RequestedAction = "write"
	ActionDelete  RequestedAction = "delete"
	ActionMove    RequestedAction = "move"
	ActionExecute RequestedAction = "execute"
	ActionInspect RequestedAction = "inspect"
)

// SubagentOrigin records where this execution originated when the tool call
// is issued by a sub-agent (guardian or otherwise). It is nil for the root
// agent.
type SubagentOrigin struct {
	AgentID       string
	ParentAgentID string
	IsGuardian    bool
	Depth         int
}

// ExecutionIntent is the unified type that every tool call is converted into
// before execution. It is a superset of agentcore.ToolDecisionRequest: in
// addition to the permission-related fields (WorkspaceRoot, WritableRoots,
// read-only/destructive/open-world signals), it carries the tool kind, the
// requested action, the file paths the tool will touch, the network scope,
// and the sub-agent origin.
type ExecutionIntent struct {
	ToolName        string
	Lane            ExecutionLane
	ToolKind        ToolKind
	RequestedAction RequestedAction
	Paths           []string
	NetworkScope    NetworkScope
	WorkspaceRoot   string
	WritableRoots   []string
	SubagentOrigin  *SubagentOrigin
}

// String returns a compact one-line representation suitable for logging.
func (i ExecutionIntent) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "intent(tool=%s kind=%s action=%s net=%s",
		i.ToolName, firstNonEmptyLane(i.Lane, ExecutionLane(i.ToolKind)), i.RequestedAction, i.NetworkScope)
	if len(i.Paths) > 0 {
		fmt.Fprintf(&b, " paths=[%s]", strings.Join(i.Paths, ","))
	}
	if i.SubagentOrigin != nil {
		fmt.Fprintf(&b, " origin=agent=%s parent=%s depth=%d",
			i.SubagentOrigin.AgentID, i.SubagentOrigin.ParentAgentID, i.SubagentOrigin.Depth)
		if i.SubagentOrigin.IsGuardian {
			b.WriteString(" guardian")
		}
	}
	b.WriteString(")")
	return b.String()
}

// InferToolKind maps a tool name plus its declared annotations to a ToolKind.
//
// Mapping rules (from the spec):
//   - apply_patch, file_read/read_file, list_files, grep_search → file_chain
//   - shell → shell_command
//   - MCP/connector-sourced tools (name starts with "mcp__" or
//     annotations.ServerName != "") → mcp_app
//   - review/guardian tools → review_agent
//   - otherwise: read-only tools default to file_chain, everything else falls
//     back to shell_command as the conservative default.
//
// Note: the annotations struct does not carry ToolDefinition.Source directly;
// the ServerName field and the "mcp__" name prefix are used as reliable
// proxies for MCP/connector origin.
func InferToolKind(toolName string, annotations tool.ToolAnnotations) ToolKind {
	name := strings.TrimSpace(toolName)
	switch name {
	case "apply_patch", "file_read", "read_file", "list_files", "grep_search":
		return ToolKindFileChain
	case "shell":
		return ToolKindShellCommand
	}

	// Review/guardian tools are identified by name. The spec does not
	// enumerate them; we match common prefixes so future review-agent
	// tools (e.g. "review_code", "guardian_check") classify correctly.
	lower := strings.ToLower(name)
	if strings.HasPrefix(lower, "review") || strings.HasPrefix(lower, "guardian") {
		return ToolKindReviewAgent
	}

	// MCP/connector-sourced tools. The builtin registry names MCP tools as
	// "mcp__{server}__{tool}"; connector tools carry a ServerName annotation.
	if strings.HasPrefix(name, "mcp__") || strings.TrimSpace(annotations.ServerName) != "" {
		return ToolKindMcpApp
	}

	// Conservative default: read-only tools look like file-chain reads;
	// everything else is treated as a shell-style command.
	if annotations.ReadOnly {
		return ToolKindFileChain
	}
	return ToolKindShellCommand
}

func InferExecutionLane(toolName string, annotations tool.ToolAnnotations) ExecutionLane {
	switch annotations.CapabilityClass {
	case tool.CapabilityArtifactRead, tool.CapabilityArtifactWrite:
		return LaneArtifact
	case tool.CapabilityConnectorRead, tool.CapabilityConnectorWrite, tool.CapabilityBrowserRead, tool.CapabilityBrowserInteract:
		return LaneConnector
	case tool.CapabilityLocalTransform:
		return LaneLocalCompute
	case tool.CapabilityExternalPublish, tool.CapabilitySystemOperation:
		return LaneSystem
	}

	switch InferToolKind(toolName, annotations) {
	case ToolKindFileChain, ToolKindReviewAgent:
		return LaneArtifact
	case ToolKindMcpApp:
		return LaneConnector
	case ToolKindShellCommand:
		return LaneSystem
	default:
		return LaneSystem
	}
}

// InferNetworkScope maps a tool's annotations to a NetworkScope.
//
//   - OpenWorld: true → NetworkScopeOpen (the tool can reach arbitrary
//     network endpoints).
//   - tools that talk to a named server or require auth → NetworkScopeRestricted
//     (network reach is bounded to a specific service).
//   - default → NetworkScopeNone (no network reach expected).
func InferNetworkScope(annotations tool.ToolAnnotations) NetworkScope {
	if annotations.OpenWorld {
		return NetworkScopeOpen
	}
	if strings.TrimSpace(annotations.ServerName) != "" || annotations.RequiresAuth {
		return NetworkScopeRestricted
	}
	return NetworkScopeNone
}

// BuildExecutionIntent converts a ToolCall + resolved Tool + PermissionContext
// into an ExecutionIntent. It is called at the start of ToolRuntime.Execute so
// the decision layer (Task 9) can reason about a unified shape. Path
// extraction is best-effort: if paths cannot be extracted, the Paths field is
// left empty.
func BuildExecutionIntent(
	call convcontext.ToolCall,
	selectedTool tool.Tool,
	permCtx tool.PermissionContext,
	subagentOrigin *SubagentOrigin,
) ExecutionIntent {
	def := selectedTool.Definition()
	toolName := strings.TrimSpace(call.Function.Name)
	if toolName == "" {
		toolName = def.Name
	}
	annotations := def.Annotations

	intent := ExecutionIntent{
		ToolName:       toolName,
		Lane:           InferExecutionLane(toolName, annotations),
		ToolKind:       InferToolKind(toolName, annotations),
		NetworkScope:   InferNetworkScope(annotations),
		WorkspaceRoot:  permCtx.WorkspaceRoot,
		WritableRoots:  append([]string(nil), permCtx.WritableRoots...),
		SubagentOrigin: subagentOrigin,
	}

	intent.RequestedAction = inferRequestedAction(toolName, intent.ToolKind, annotations, call.Function.Arguments)
	intent.Paths = extractPaths(selectedTool, call.Function.Arguments, permCtx)

	return intent
}

func firstNonEmptyLane(values ...ExecutionLane) ExecutionLane {
	for _, value := range values {
		if strings.TrimSpace(string(value)) != "" {
			return value
		}
	}
	return ""
}

// inferRequestedAction derives the primary action a tool will perform. For
// apply_patch the action is refined by inspecting the patch body so that
// delete-only and move patches classify as ActionDelete / ActionMove rather
// than the generic ActionWrite.
func inferRequestedAction(toolName string, kind ToolKind, annotations tool.ToolAnnotations, arguments string) RequestedAction {
	switch kind {
	case ToolKindShellCommand:
		return ActionExecute
	case ToolKindFileChain:
		if toolName == "apply_patch" {
			return refinePatchAction(arguments)
		}
		if annotations.Destructive {
			return ActionWrite
		}
		if annotations.ReadOnly {
			switch toolName {
			case "list_files", "grep_search":
				return ActionInspect
			default:
				return ActionRead
			}
		}
		return ActionWrite
	case ToolKindMcpApp:
		if annotations.ReadOnly {
			return ActionRead
		}
		if annotations.Destructive {
			return ActionWrite
		}
		return ActionExecute
	case ToolKindReviewAgent:
		return ActionInspect
	}
	return ActionRead
}

// refinePatchAction inspects a Codex-style patch body and returns the
// highest-risk action present: delete > move > write. If the patch cannot be
// parsed, it falls back to ActionWrite.
func refinePatchAction(arguments string) RequestedAction {
	patch := extractPatchBody(arguments)
	if patch == "" {
		return ActionWrite
	}
	hasDelete := false
	hasMove := false
	hasAddOrUpdate := false
	for _, line := range strings.Split(patch, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "*** Delete File:"):
			hasDelete = true
		case strings.HasPrefix(trimmed, "*** Move to:"):
			hasMove = true
		case strings.HasPrefix(trimmed, "*** Add File:"), strings.HasPrefix(trimmed, "*** Update File:"):
			hasAddOrUpdate = true
		}
	}
	switch {
	case hasDelete:
		return ActionDelete
	case hasMove:
		return ActionMove
	case hasAddOrUpdate:
		return ActionWrite
	}
	return ActionWrite
}

// extractPaths performs a best-effort extraction of the file paths a tool
// invocation will touch. For apply_patch (and any PatchPreviewingTool) it
// uses the PatchPreviewingTool interface so the same path-resolution logic
// the decision metadata path uses is reused here. For file_read/read_file,
// list_files, and grep_search it parses the JSON arguments and pulls the
// path/root parameter. For shell it scans for redirect targets. If paths
// cannot be extracted, an empty slice is returned.
func extractPaths(selectedTool tool.Tool, arguments string, permCtx tool.PermissionContext) []string {
	// apply_patch (and any other PatchPreviewingTool) already knows how to
	// enumerate affected paths with full workspace resolution. The tool
	// expects the normalised input (the patch body for apply_patch, not the
	// raw JSON wrapper), so we extract it first.
	if previewingTool, ok := selectedTool.(tool.PatchPreviewingTool); ok {
		normalised := extractToolInput(arguments, selectedTool.Parameters())
		preview, ok := previewingTool.PatchPreview(normalised, permCtx)
		if ok && len(preview.AffectedPaths) > 0 {
			return dedupe(preview.AffectedPaths)
		}
	}

	name := selectedTool.Name()
	normalised := extractToolInput(arguments, selectedTool.Parameters())
	switch name {
	case "file_read", "read_file":
		if path := jsonStringField(normalised, "path"); path != "" {
			return []string{path}
		}
	case "list_files", "grep_search":
		if root := jsonStringField(normalised, "root"); root != "" {
			return []string{root}
		}
	case "shell":
		if cmd := jsonStringField(normalised, "command"); cmd != "" {
			return extractShellRedirectTargets(cmd)
		}
	}
	return nil
}

// extractShellRedirectTargets returns the file paths targeted by > or >>
// redirects in a shell command. This is deliberately conservative: it does
// not try to identify command arguments that happen to be file paths, because
// doing so reliably requires full shell parsing. File descriptor redirects
// (>&1, >&2) are ignored.
func extractShellRedirectTargets(command string) []string {
	var targets []string
	inSingle := false
	inDouble := false
	i := 0
	for i < len(command) {
		c := command[i]
		switch {
		case c == '\'' && !inDouble:
			inSingle = !inSingle
			i++
		case c == '"' && !inSingle:
			inDouble = !inDouble
			i++
		case inSingle || inDouble:
			i++
		case c == '>':
			rest := command[i+1:]
			if strings.HasPrefix(rest, ">") {
				rest = rest[1:]
			}
			rest = strings.TrimLeft(rest, " \t")
			if rest == "" {
				i++
				continue
			}
			target := readShellToken(rest)
			if target == "" || strings.HasPrefix(target, "&") {
				i++
				continue
			}
			targets = append(targets, target)
			i++
		default:
			i++
		}
	}
	return dedupe(targets)
}

func readShellToken(s string) string {
	end := strings.IndexAny(s, " \t")
	if end == -1 {
		return s
	}
	return s[:end]
}

// extractToolInput extracts the effective input string for a tool from its
// raw JSON arguments. For single-parameter tools, this is the parameter
// value; for multi-parameter tools, it's the re-marshalled JSON object. This
// mirrors agentcore.normalizeToolInput but without validation, so it can be
// called from the profile package without creating an import cycle.
func extractToolInput(arguments string, params []tool.ToolParameter) string {
	trimmed := strings.TrimSpace(arguments)
	if len(params) == 0 {
		return trimmed
	}
	if trimmed == "" {
		return ""
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal([]byte(trimmed), &decoded); err != nil {
		return trimmed
	}
	if decoded == nil {
		return ""
	}
	if len(params) == 1 {
		value, ok := decoded[params[0].Name]
		if !ok {
			return ""
		}
		if str, ok := value.(string); ok {
			return str
		}
		data, err := json.Marshal(value)
		if err != nil {
			return ""
		}
		return string(data)
	}
	data, err := json.Marshal(decoded)
	if err != nil {
		return trimmed
	}
	return string(data)
}

// extractPatchBody extracts the "patch" parameter value from a JSON arguments
// string. Returns "" if the patch field is absent or the input is not valid
// JSON.
func extractPatchBody(arguments string) string {
	return jsonStringField(arguments, "patch")
}

// jsonStringField decodes a single string field from a JSON object. Returns
// "" if the input is not a JSON object, the field is absent, or the field is
// not a string.
func jsonStringField(arguments string, field string) string {
	trimmed := strings.TrimSpace(arguments)
	if trimmed == "" {
		return ""
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal([]byte(trimmed), &decoded); err != nil {
		return ""
	}
	value, ok := decoded[field]
	if !ok {
		return ""
	}
	str, ok := value.(string)
	if !ok {
		return ""
	}
	return str
}

func dedupe(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	deduped := make([]string, 0, len(values))
	for _, v := range values {
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		deduped = append(deduped, v)
	}
	return deduped
}
