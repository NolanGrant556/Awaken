// Package profile defines ExecutionIntent, the unified type that every tool
// call is converted into before execution. This is the foundational type for
// Phase 3 (unified execution profile) of the "零审批自动执行策略" spec:
// every ToolCall — whether builtin (Read, Edit, Bash, ...),
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
	ToolKindFileOperation ToolKind = "file_operation"
	ToolKindShellCommand  ToolKind = "shell_command"
	ToolKindMcpApp        ToolKind = "mcp_app"
	ToolKindReviewAgent   ToolKind = "review_agent"
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
//   - Read, Edit, Write, Glob, Grep and trash_file -> file_operation
//   - Bash → shell_command
//   - MCP/connector-sourced tools (name starts with "mcp__" or
//     annotations.ServerName != "") → mcp_app
//   - review/guardian tools → review_agent
//   - otherwise: read-only tools default to file_operation, everything else falls
//     back to shell_command as the conservative default.
//
// Note: the annotations struct does not carry ToolDefinition.Source directly;
// the ServerName field and the "mcp__" name prefix are used as reliable
// proxies for MCP/connector origin.
func InferToolKind(toolName string, annotations tool.ToolAnnotations) ToolKind {
	name := strings.TrimSpace(toolName)
	switch strings.ToLower(name) {
	case "read", "edit", "write", "glob", "grep", "trash_file":
		return ToolKindFileOperation
	case "bash":
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

	// Conservative default: read-only tools look like typed file reads;
	// everything else is treated as a shell-style command.
	if annotations.ReadOnly {
		return ToolKindFileOperation
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
	case ToolKindFileOperation, ToolKindReviewAgent:
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
	return BuildExecutionIntentWithDefinition(call, selectedTool, selectedTool.Definition(), permCtx, subagentOrigin)
}

// BuildExecutionIntentWithDefinition uses the immutable registry definition
// snapshot supplied by ToolRuntime. The legacy wrapper above remains for
// callers that do not yet have a registered contract.
func BuildExecutionIntentWithDefinition(
	call convcontext.ToolCall,
	selectedTool tool.Tool,
	def tool.ToolDefinition,
	permCtx tool.PermissionContext,
	subagentOrigin *SubagentOrigin,
) ExecutionIntent {
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

// inferRequestedAction derives the primary action a tool will perform.
func inferRequestedAction(toolName string, kind ToolKind, annotations tool.ToolAnnotations, arguments string) RequestedAction {
	switch kind {
	case ToolKindShellCommand:
		return ActionExecute
	case ToolKindFileOperation:
		if annotations.Destructive {
			return ActionWrite
		}
		if annotations.ReadOnly {
			switch strings.ToLower(toolName) {
			case "glob", "grep":
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

// extractPaths performs a best-effort extraction of the file paths a tool
// invocation will touch. For any PatchPreviewingTool it
// uses the PatchPreviewingTool interface so the same path-resolution logic
// the decision metadata path uses is reused here. For Read/Edit/Write/Glob/
// Grep it parses the JSON arguments and pulls the path parameter. For Bash it
// scans for redirect targets. If paths
// cannot be extracted, an empty slice is returned.
func extractPaths(selectedTool tool.Tool, arguments string, permCtx tool.PermissionContext) []string {
	// A generic PatchPreviewingTool can enumerate affected paths with its own
	// workspace resolution. Normalize the input before requesting a preview.
	if previewingTool, ok := selectedTool.(tool.PatchPreviewingTool); ok {
		normalised := extractToolInput(arguments, selectedTool.Parameters())
		preview, ok := previewingTool.PatchPreview(normalised, permCtx)
		if ok && len(preview.AffectedPaths) > 0 {
			return dedupe(preview.AffectedPaths)
		}
	}

	name := strings.ToLower(selectedTool.Name())
	normalised := extractToolInput(arguments, selectedTool.Parameters())
	switch name {
	case "read", "edit", "write":
		if path := jsonStringField(normalised, "file_path"); path != "" {
			return []string{path}
		}
	case "glob", "grep":
		if root := jsonStringField(normalised, "path"); root != "" {
			return []string{root}
		}
	case "bash":
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
