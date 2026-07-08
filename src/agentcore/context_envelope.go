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
	ScopeID          string
	WorkspaceRoot    string
	ReadableRoots    []string
	WritableRoots    []string
	OutputRoots      []string
	AttachedFiles    []tool.ScopedFile
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
	attachedFiles := make([]prompt.RuntimeAttachedFile, 0, len(snapshot.AttachedFiles))
	for _, file := range snapshot.AttachedFiles {
		attachedFiles = append(attachedFiles, prompt.RuntimeAttachedFile{
			Path:        file.Path,
			DisplayName: file.DisplayName,
		})
	}
	return prompt.RuntimeInfo{
		WorkingDir:    snapshot.WorkingDirectory,
		CurrentDate:   snapshot.CurrentDate,
		ModelName:     snapshot.ModelName,
		EntryPoint:    snapshot.EntryPoint,
		ToolSummary:   snapshot.ToolSummary,
		AttachedFiles: attachedFiles,
	}
}

func runtimeSnapshotSystemItem(snapshot RuntimeSnapshot) ContextItem {
	content := fmt.Sprintf(
		"current_date=%s\nworking_directory=%s\nmodel_name=%s\nentry_point=%s\ntool_summary=%s\nscope_id=%s\nworkspace_root=%s\nreadable_roots=%s\nwritable_roots=%s\noutput_roots=%s\nattached_files=%s",
		snapshot.CurrentDate,
		snapshot.WorkingDirectory,
		snapshot.ModelName,
		snapshot.EntryPoint,
		snapshot.ToolSummary,
		snapshot.ScopeID,
		snapshot.WorkspaceRoot,
		strings.Join(snapshot.ReadableRoots, ","),
		strings.Join(snapshot.WritableRoots, ","),
		strings.Join(snapshot.OutputRoots, ","),
		formatRuntimeSnapshotAttachedFiles(snapshot.AttachedFiles),
	)
	return ContextItem{
		ID:      "runtime_snapshot",
		Type:    "runtime_snapshot",
		Content: content,
	}
}

func formatRuntimeSnapshotAttachedFiles(files []tool.ScopedFile) string {
	if len(files) == 0 {
		return "none"
	}
	parts := make([]string, 0, len(files))
	for _, file := range files {
		path := strings.TrimSpace(file.Path)
		if path == "" {
			continue
		}
		displayName := strings.TrimSpace(file.DisplayName)
		if displayName == "" {
			parts = append(parts, path)
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%s", displayName, path))
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, ",")
}
