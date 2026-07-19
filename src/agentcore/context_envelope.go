package agentcore

import (
	"fmt"
	"strings"
	"time"

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
	AttributedTools    []AttributedToolSchema
	RuntimeSnapshot    RuntimeSnapshot
	ContextWindowState ContextWindowState
	CacheKeys          []ContextCacheKey
	Deltas             []ContextDelta
	Messages           []convcontext.Message
	CompactModelConfig ModelConfig
	Model              string
	Temperature        float64
	MaxTokens          int
	EpochSources       []ContextEpochSourceCandidate
}

// PromptSection is a host-owned contribution wrapper. Content is sent only as
// part of the normal system prompt; Kind/ID never cross the provider boundary.
type PromptSection struct {
	ID      string
	Kind    ContextContributionKind
	Content string
}

// ToolSchemaProvenance preserves the pinned catalog identity that was lost
// when tool definitions were flattened to provider JSON.
type ToolSchemaProvenance struct {
	Origin         tool.ToolSource
	ProviderID     tool.ProviderID
	RegistrationID string
	// SkillCatalog is the exact model-visible catalog listing embedded in the
	// unified Skill tool description. It remains host-owned provenance: the
	// provider sees only the ordinary lowered tool definition.
	SkillCatalog string `json:"skill_catalog,omitempty"`
}

type AttributedToolSchema struct {
	Schema     tool.ToolSchema
	Provenance ToolSchemaProvenance
}

type ContextBlock struct {
	ID         string
	Kind       ContextContributionKind
	Content    string
	SkillID    string
	SourcePath string
	AgentID    string
	InvokedAt  time.Time
	Compacted  bool
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
	CurrentDate       string
	WorkingDirectory  string
	ModelName         string
	EntryPoint        string
	ToolSummary       string
	SkillSummary      string
	ScopeID           string
	WorkspaceRoot     string
	ReadableRoots     []string
	WritableRoots     []string
	OutputRoots       []string
	AttachedFiles     []tool.ScopedFile
	ToolSnapshotID    string
	CatalogVersion    uint64
	EnvironmentID     string
	OperatingSystem   string
	Architecture      string
	PATHSource        string
	SkillRoots        []tool.SkillRootObservation
	TemporaryWorkRoot string
	RefreshReason     string
	Executables       []string
	PackageManagers   []string
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
		SkillSummary:  snapshot.SkillSummary,
		AttachedFiles: attachedFiles,
	}
}

func runtimeSnapshotSystemItem(snapshot RuntimeSnapshot) ContextItem {
	content := fmt.Sprintf(
		"current_date=%s\nworking_directory=%s\nmodel_name=%s\nentry_point=%s\ntool_summary=%s\nskill_summary=%s\nscope_id=%s\nworkspace_root=%s\nreadable_roots=%s\nwritable_roots=%s\noutput_roots=%s\nattached_files=%s\ntool_snapshot_id=%s\ntool_catalog_version=%d\nenvironment_snapshot_id=%s\nos=%s\narchitecture=%s\npath_source=%s\nskill_roots=%s\ntemporary_work_root=%s\nenvironment_refresh_reason=%s\nobserved_executables=%s\npackage_managers=%s",
		snapshot.CurrentDate,
		snapshot.WorkingDirectory,
		snapshot.ModelName,
		snapshot.EntryPoint,
		snapshot.ToolSummary,
		snapshot.SkillSummary,
		snapshot.ScopeID,
		snapshot.WorkspaceRoot,
		strings.Join(snapshot.ReadableRoots, ","),
		strings.Join(snapshot.WritableRoots, ","),
		strings.Join(snapshot.OutputRoots, ","),
		formatRuntimeSnapshotAttachedFiles(snapshot.AttachedFiles),
		snapshot.ToolSnapshotID,
		snapshot.CatalogVersion,
		snapshot.EnvironmentID,
		snapshot.OperatingSystem,
		snapshot.Architecture,
		snapshot.PATHSource,
		formatRuntimeSnapshotSkillRoots(snapshot.SkillRoots),
		snapshot.TemporaryWorkRoot,
		snapshot.RefreshReason,
		strings.Join(snapshot.Executables, ","),
		strings.Join(snapshot.PackageManagers, ","),
	)
	return ContextItem{
		ID:      "runtime_snapshot",
		Type:    "runtime_snapshot",
		Content: content,
	}
}

func formatRuntimeSnapshotSkillRoots(roots []tool.SkillRootObservation) string {
	if len(roots) == 0 {
		return "none"
	}
	parts := make([]string, 0, len(roots))
	for _, root := range roots {
		if strings.TrimSpace(root.Path) == "" {
			continue
		}
		parts = append(parts, strings.TrimSpace(root.Source)+"="+root.Path)
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, ",")
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
