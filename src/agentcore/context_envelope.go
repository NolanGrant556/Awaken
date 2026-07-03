package agentcore

import (
	"fmt"
	"strings"

	convcontext "awaken/src/context"
	"awaken/src/prompt"
	"awaken/src/tool"
)

type ContextEnvelope struct {
	SystemBlocks       []ContextBlock
	UserContextItems   []ContextItem
	SystemContextItems []ContextItem
	Attachments        []ContextAttachment
	ToolSchemas        []tool.ToolSchema
	RuntimeSnapshot    RuntimeSnapshot
	ContextWindowState ContextWindowState
	CacheKeys          []ContextCacheKey
	Deltas             []ContextDelta
	Messages           []convcontext.Message
	CompactModelConfig ModelConfig
	Model              string
	Temperature        float64
	MaxTokens          int
}

type ContextBlock struct {
	ID      string
	Content string
}

type ContextItem struct {
	ID      string
	Type    string
	Content string
}

type ContextAttachment struct {
	ID      string
	Kind    string
	Path    string
	Content string
}

type RuntimeSnapshot struct {
	CurrentDate      string
	WorkingDirectory string
	ModelName        string
	EntryPoint       string
	ToolSummary      string
	WorkspaceRoot    string
	WritableRoots    []string
}

type ContextCacheKey struct {
	Name  string
	Value string
}

type ContextDeltaType string

const (
	ContextDeltaRuntimeSnapshot ContextDeltaType = "runtime_snapshot_delta"
	ContextDeltaToolPool        ContextDeltaType = "tool_pool_delta"
	ContextDeltaMemoryHit       ContextDeltaType = "memory_hit"
	ContextDeltaMCPInstruction  ContextDeltaType = "mcp_instruction_delta"
)

type ContextDelta struct {
	Type    ContextDeltaType
	Summary string
}

func (snapshot RuntimeSnapshot) ToRuntimeInfo() prompt.RuntimeInfo {
	return prompt.RuntimeInfo{
		WorkingDir:  snapshot.WorkingDirectory,
		CurrentDate: snapshot.CurrentDate,
		ModelName:   snapshot.ModelName,
		EntryPoint:  snapshot.EntryPoint,
		ToolSummary: snapshot.ToolSummary,
	}
}

func runtimeSnapshotSystemItem(snapshot RuntimeSnapshot) ContextItem {
	content := fmt.Sprintf(
		"current_date=%s\nworking_directory=%s\nmodel_name=%s\nentry_point=%s\ntool_summary=%s\nworkspace_root=%s\nwritable_roots=%s",
		snapshot.CurrentDate,
		snapshot.WorkingDirectory,
		snapshot.ModelName,
		snapshot.EntryPoint,
		snapshot.ToolSummary,
		snapshot.WorkspaceRoot,
		strings.Join(snapshot.WritableRoots, ","),
	)
	return ContextItem{
		ID:      "runtime_snapshot",
		Type:    "runtime_snapshot",
		Content: content,
	}
}
