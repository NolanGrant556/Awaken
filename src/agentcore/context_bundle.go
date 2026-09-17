package agentcore

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	convcontext "awaken/src/context"
)

const (
	PostCompactContextBundleSchemaVersion = 2
	EmptySectionDigest                    = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

	MaterializationNotMaterialized = "not_materialized"
	MaterializationMaterialized    = "materialized"
	MaterializationUnavailable     = "unavailable"

	ReferenceAvailable   = "available"
	ReferenceUnavailable = "unavailable"

	CompactProjectionV1Messages   = "v1_messages"
	CompactProjectionCheckpointV2 = "checkpoint_v2"
)

// MaterializedSection is the stable section state machine of the V2 compact
// envelope. A nil value is never overloaded to mean both "not implemented"
// and "unavailable"; State and Reason make that distinction explicit.
type MaterializedSection[T any] struct {
	State  string `json:"state"`
	Value  *T     `json:"value"`
	Digest string `json:"digest"`
	Reason string `json:"reason"`
}

type CompactConversationProjectionV2 struct {
	Format   string                     `json:"format"`
	Messages []SemanticCompactedMessage `json:"messages"`
}

// SnapshotRef is a secret-free reference to one frozen host authority.
// Digest binds the referenced projection, not credentials or executable
// interfaces. Unavailable references carry a stable lower-snake-case reason.
type SnapshotRef struct {
	State    string `json:"state"`
	ID       string `json:"id"`
	Digest   string `json:"digest"`
	Revision uint64 `json:"revision"`
	Reason   string `json:"reason"`
}

type VersionedRef = SnapshotRef

type MemoryRecallItemRef struct {
	MemoryID      string `json:"memory_id"`
	Scope         string `json:"scope"`
	Type          string `json:"type"`
	Revision      int    `json:"revision"`
	Bytes         int    `json:"bytes"`
	ContentDigest string `json:"content_digest"`
}

// MemoryRecallSnapshotRef deliberately stores identities and content digests,
// not Memory bodies. The Memory store remains the body authority.
type MemoryRecallSnapshotRef struct {
	State         string                `json:"state"`
	WorkspaceRoot string                `json:"workspace_root"`
	Items         []MemoryRecallItemRef `json:"items"`
	Digest        string                `json:"digest"`
	Reason        string                `json:"reason"`
}

// SkillContextRef includes the exact bounded provider-neutral projection used
// after compaction. Skill activation records already persist this host-loaded
// body; carrying the bounded projection here prevents live/restart divergence.
type SkillContextRef struct {
	ID               string    `json:"id"`
	SkillID          string    `json:"skill_id"`
	SourcePath       string    `json:"source_path"`
	AgentID          string    `json:"agent_id"`
	InvokedAt        time.Time `json:"invoked_at"`
	Compacted        bool      `json:"compacted"`
	ProjectedContent string    `json:"projected_content"`
	Digest           string    `json:"digest"`
}

func (ref *SkillContextRef) Seal() error {
	if ref == nil {
		return fmt.Errorf("skill context ref is nil")
	}
	ref.Digest = ""
	digest, err := digestCanonicalValue(ref)
	if err != nil {
		return err
	}
	ref.Digest = digest
	return nil
}

type GateRef struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	State  string `json:"state"`
	Digest string `json:"digest"`
}

type SubagentHandoffRef struct {
	TaskID          string `json:"task_id"`
	AgentID         string `json:"agent_id"`
	ParentAgentID   string `json:"parent_agent_id"`
	Status          string `json:"status"`
	Description     string `json:"description"`
	ResultPreview   string `json:"result_preview"`
	ErrorCode       string `json:"error_code"`
	SideEffectState string `json:"side_effect_state"`
	Digest          string `json:"digest"`
}

type ArtifactRef struct {
	ID     string `json:"id"`
	RunID  string `json:"run_id"`
	Kind   string `json:"kind"`
	Path   string `json:"path"`
	Role   string `json:"role"`
	Status string `json:"status"`
	Digest string `json:"digest"`
}

// The following types freeze the V2 envelope shape. Phases 2-5 materialize
// them without changing PostCompactContextBundleV2 or its schema version.
type FactRef struct {
	FactID string `json:"fact_id"`
	Digest string `json:"digest"`
}

type CheckpointFact struct {
	FactID          string   `json:"fact_id"`
	Kind            string   `json:"kind"`
	NormalizedValue string   `json:"normalized_value"`
	SourceRecordIDs []string `json:"source_record_ids"`
	SequenceStart   int64    `json:"sequence_start"`
	SequenceEnd     int64    `json:"sequence_end"`
	Priority        string   `json:"priority"`
	Active          bool     `json:"active"`
	Digest          string   `json:"digest"`
}

type TaskCheckpointV2 struct {
	ActiveTask       string           `json:"active_task"`
	Goal             string           `json:"goal"`
	PendingUserAsks  []CheckpointFact `json:"pending_user_asks"`
	Constraints      []CheckpointFact `json:"constraints"`
	UserCorrections  []CheckpointFact `json:"user_corrections"`
	Decisions        []CheckpointFact `json:"decisions"`
	CompletedActions []CheckpointFact `json:"completed_actions"`
	InProgress       []CheckpointFact `json:"in_progress"`
	Blockers         []CheckpointFact `json:"blockers"`
	RemainingWork    []CheckpointFact `json:"remaining_work"`
	RelevantFiles    []CheckpointFact `json:"relevant_files"`
	Commands         []CheckpointFact `json:"commands"`
	Errors           []CheckpointFact `json:"errors"`
	ToolFindings     []CheckpointFact `json:"tool_findings"`
	SubagentStates   []CheckpointFact `json:"subagent_states"`
	MemoryRefs       []FactRef        `json:"memory_refs"`
	SkillRefs        []FactRef        `json:"skill_refs"`
	ArtifactRefs     []FactRef        `json:"artifact_refs"`
}

type CompactTurnV2 struct {
	TurnID   string                     `json:"turn_id"`
	Closed   bool                       `json:"closed"`
	Messages []SemanticCompactedMessage `json:"messages"`
}

type TranscriptLocator struct {
	ThreadID     string `json:"thread_id"`
	SequenceFrom int64  `json:"sequence_from"`
	SequenceTo   int64  `json:"sequence_to"`
	BoundaryID   string `json:"boundary_id"`
}

type ProviderOpaqueCompactionRef struct {
	ProviderID         string            `json:"provider_id"`
	ProfileID          string            `json:"profile_id"`
	CredentialRevision uint64            `json:"credential_revision"`
	Protocol           string            `json:"protocol"`
	Model              string            `json:"model"`
	EndpointContractID string            `json:"endpoint_contract_id"`
	Output             []json.RawMessage `json:"output"`
	StateDigest        string            `json:"state_digest"`
}

type CompactBudgetReceipt struct {
	EffectiveInputTokens  int            `json:"effective_input_tokens"`
	CompactTargetTokens   int            `json:"compact_target_tokens"`
	LocalProjectedTokens  int            `json:"local_projected_tokens"`
	ConservativeProjected int            `json:"conservative_projected_tokens"`
	MandatoryFloorTokens  int            `json:"mandatory_floor_tokens"`
	CalibrationRatio      float64        `json:"calibration_ratio"`
	ContributionTokens    map[string]int `json:"contribution_tokens"`
}

type CompactCoverageReceipt struct {
	CriticalFactTotal    int      `json:"critical_fact_total"`
	CriticalFactCovered  int      `json:"critical_fact_covered"`
	ImportantFactTotal   int      `json:"important_fact_total"`
	ImportantFactCovered int      `json:"important_fact_covered"`
	CriticalFactIDs      []string `json:"critical_fact_ids"`
	ImportantFactIDs     []string `json:"important_fact_ids"`
	MissingFactIDs       []string `json:"missing_fact_ids"`
	RepairAttempts       int      `json:"repair_attempts"`
}

type EnvironmentBindingRef struct {
	Reference  string `json:"environment_ref"`
	InstanceID string `json:"instance_id"`
}

type PostCompactContextBundleV2 struct {
	EnvironmentBindings []EnvironmentBindingRef `json:"environment_bindings,omitempty"`
	SchemaVersion       int                     `json:"schema_version"`
	BoundaryID          string                  `json:"boundary_id"`
	ParentBoundaryID    string                  `json:"parent_boundary_id"`
	Generation          int                     `json:"generation"`
	Mode                string                  `json:"mode"`
	SourceSequenceStart int64                   `json:"source_sequence_start"`
	SourceSequenceEnd   int64                   `json:"source_sequence_end"`

	ConversationProjection CompactConversationProjectionV2       `json:"conversation_projection"`
	TaskCheckpoint         MaterializedSection[TaskCheckpointV2] `json:"task_checkpoint"`
	SemanticSummary        MaterializedSection[string]           `json:"semantic_summary"`
	RecentTurns            MaterializedSection[[]CompactTurnV2]  `json:"recent_turns"`

	InstructionSnapshotRef SnapshotRef             `json:"instruction_snapshot_ref"`
	MemorySnapshot         MemoryRecallSnapshotRef `json:"memory_snapshot"`
	RunPlanRef             VersionedRef            `json:"run_plan_ref"`
	SkillRefs              []SkillContextRef       `json:"skill_refs"`
	ToolSnapshotRef        SnapshotRef             `json:"tool_snapshot_ref"`
	HookSnapshotRef        SnapshotRef             `json:"hook_snapshot_ref"`
	ActiveGateRefs         []GateRef               `json:"active_gate_refs"`
	PendingSubagents       []SubagentHandoffRef    `json:"pending_subagents"`
	ArtifactRefs           []ArtifactRef           `json:"artifact_refs"`

	TranscriptLocator   MaterializedSection[TranscriptLocator]           `json:"transcript_locator"`
	ProviderOpaqueState MaterializedSection[ProviderOpaqueCompactionRef] `json:"provider_opaque_state"`
	BudgetReceipt       MaterializedSection[CompactBudgetReceipt]        `json:"budget_receipt"`
	CoverageReceipt     MaterializedSection[CompactCoverageReceipt]      `json:"coverage_receipt"`
	BundleDigest        string                                           `json:"bundle_digest"`
}

func NotMaterializedSection[T any]() MaterializedSection[T] {
	return MaterializedSection[T]{State: MaterializationNotMaterialized, Digest: EmptySectionDigest}
}

func MaterializeSection[T any](value T) (MaterializedSection[T], error) {
	digest, err := digestCanonicalValue(value)
	if err != nil {
		return MaterializedSection[T]{}, err
	}
	return MaterializedSection[T]{State: MaterializationMaterialized, Value: &value, Digest: digest}, nil
}

func UnavailableSnapshotRef(reason string) SnapshotRef {
	return SnapshotRef{State: ReferenceUnavailable, Digest: EmptySectionDigest, Reason: reason}
}

func UnavailableMemorySnapshotRef(reason string) MemoryRecallSnapshotRef {
	return MemoryRecallSnapshotRef{State: ReferenceUnavailable, Items: []MemoryRecallItemRef{}, Digest: EmptySectionDigest, Reason: reason}
}

func digestCanonicalValue(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func digestText(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func (bundle *PostCompactContextBundleV2) Seal() error {
	if bundle == nil {
		return fmt.Errorf("context bundle is nil")
	}
	bundle.normalizeCollections()
	bundle.BundleDigest = ""
	digest, err := digestCanonicalValue(bundle)
	if err != nil {
		return fmt.Errorf("digest context bundle: %w", err)
	}
	bundle.BundleDigest = digest
	return bundle.Validate()
}

func (bundle PostCompactContextBundleV2) Validate() error {
	if bundle.SchemaVersion != PostCompactContextBundleSchemaVersion {
		return fmt.Errorf("unsupported context bundle schema_version=%d", bundle.SchemaVersion)
	}
	if strings.TrimSpace(bundle.BoundaryID) == "" {
		return fmt.Errorf("context bundle boundary_id is required")
	}
	if bundle.Generation < 1 {
		return fmt.Errorf("context bundle generation must be positive")
	}
	switch CompactMode(bundle.Mode) {
	case CompactModeAuto, CompactModeReactive, CompactModeManual:
	default:
		return fmt.Errorf("context bundle mode %q is invalid", bundle.Mode)
	}
	if bundle.SourceSequenceStart < 0 || bundle.SourceSequenceEnd < 0 ||
		(bundle.SourceSequenceEnd > 0 && bundle.SourceSequenceStart > bundle.SourceSequenceEnd) {
		return fmt.Errorf("context bundle source sequence range is invalid")
	}
	if err := validateConversationProjection(bundle.BoundaryID, bundle.ConversationProjection); err != nil {
		return err
	}
	if err := validateMaterializedSection("task_checkpoint", bundle.TaskCheckpoint); err != nil {
		return err
	}
	if err := validateMaterializedSection("semantic_summary", bundle.SemanticSummary); err != nil {
		return err
	}
	if err := validateMaterializedSection("recent_turns", bundle.RecentTurns); err != nil {
		return err
	}
	if err := validateMaterializedSection("transcript_locator", bundle.TranscriptLocator); err != nil {
		return err
	}
	if err := validateMaterializedSection("provider_opaque_state", bundle.ProviderOpaqueState); err != nil {
		return err
	}
	if bundle.ProviderOpaqueState.State == MaterializationMaterialized && bundle.ProviderOpaqueState.Value != nil {
		if err := validateProviderOpaqueCompactionRef(*bundle.ProviderOpaqueState.Value); err != nil {
			return err
		}
	}
	if err := validateMaterializedSection("budget_receipt", bundle.BudgetReceipt); err != nil {
		return err
	}
	if err := validateMaterializedSection("coverage_receipt", bundle.CoverageReceipt); err != nil {
		return err
	}
	if bundle.TaskCheckpoint.State == MaterializationMaterialized {
		if bundle.SemanticSummary.State != MaterializationMaterialized || bundle.CoverageReceipt.State != MaterializationMaterialized {
			return fmt.Errorf("materialized task checkpoint requires semantic_summary and coverage_receipt")
		}
		if err := validateTaskCheckpointCoverage(*bundle.TaskCheckpoint.Value, *bundle.CoverageReceipt.Value); err != nil {
			return err
		}
	}
	if bundle.RecentTurns.State == MaterializationMaterialized && bundle.RecentTurns.Value != nil {
		if err := validateCompactTurns(*bundle.RecentTurns.Value); err != nil {
			return err
		}
	}
	if bundle.TranscriptLocator.State == MaterializationMaterialized && bundle.TranscriptLocator.Value != nil {
		locator := *bundle.TranscriptLocator.Value
		if strings.TrimSpace(locator.ThreadID) == "" || locator.BoundaryID != bundle.BoundaryID || locator.SequenceFrom < 1 || locator.SequenceTo < locator.SequenceFrom || locator.SequenceTo != bundle.SourceSequenceEnd {
			return fmt.Errorf("transcript_locator authority/range is invalid")
		}
	}
	if bundle.BudgetReceipt.State == MaterializationMaterialized && bundle.BudgetReceipt.Value != nil {
		receipt := *bundle.BudgetReceipt.Value
		if receipt.EffectiveInputTokens <= 0 || receipt.CompactTargetTokens != int(float64(receipt.EffectiveInputTokens)*0.10) ||
			receipt.CalibrationRatio < 1 || receipt.LocalProjectedTokens < 0 || receipt.MandatoryFloorTokens < 0 {
			return fmt.Errorf("budget_receipt hard target gate failed")
		}
		if receipt.MandatoryFloorTokens <= receipt.CompactTargetTokens && receipt.ConservativeProjected > receipt.CompactTargetTokens {
			return fmt.Errorf("budget_receipt hard target gate failed")
		}
	}
	for name, ref := range map[string]SnapshotRef{
		"instruction_snapshot_ref": bundle.InstructionSnapshotRef,
		"run_plan_ref":             bundle.RunPlanRef,
		"tool_snapshot_ref":        bundle.ToolSnapshotRef,
		"hook_snapshot_ref":        bundle.HookSnapshotRef,
	} {
		if err := validateSnapshotRef(name, ref); err != nil {
			return err
		}
	}
	if err := validateMemorySnapshotRef(bundle.MemorySnapshot); err != nil {
		return err
	}
	for index, ref := range bundle.SkillRefs {
		if strings.TrimSpace(ref.ID) == "" || strings.TrimSpace(ref.SkillID) == "" {
			return fmt.Errorf("skill_refs[%d] identity is required", index)
		}
		digest := ref.Digest
		ref.Digest = ""
		expected, err := digestCanonicalValue(ref)
		if err != nil || digest != expected {
			return fmt.Errorf("skill_refs[%d] digest mismatch", index)
		}
	}
	if err := validateDirectRefDigests(bundle); err != nil {
		return err
	}
	if err := validateDigest("bundle_digest", bundle.BundleDigest); err != nil {
		return err
	}
	copyBundle := bundle
	copyBundle.normalizeCollections()
	copyBundle.BundleDigest = ""
	expected, err := digestCanonicalValue(copyBundle)
	if err != nil || bundle.BundleDigest != expected {
		return fmt.Errorf("context bundle digest mismatch")
	}
	return nil
}

func DecodePostCompactContextBundleV2(raw any) (*PostCompactContextBundleV2, error) {
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("encode context bundle payload: %w", err)
	}
	var bundle PostCompactContextBundleV2
	if err := json.Unmarshal(encoded, &bundle); err != nil {
		return nil, fmt.Errorf("decode context bundle payload: %w", err)
	}
	if err := bundle.Validate(); err != nil {
		return nil, err
	}
	return &bundle, nil
}

func (bundle *PostCompactContextBundleV2) normalizeCollections() {
	if bundle.ConversationProjection.Messages == nil {
		bundle.ConversationProjection.Messages = []SemanticCompactedMessage{}
	}
	if bundle.SkillRefs == nil {
		bundle.SkillRefs = []SkillContextRef{}
	}
	if bundle.MemorySnapshot.Items == nil {
		bundle.MemorySnapshot.Items = []MemoryRecallItemRef{}
	}
	if bundle.ActiveGateRefs == nil {
		bundle.ActiveGateRefs = []GateRef{}
	}
	if bundle.PendingSubagents == nil {
		bundle.PendingSubagents = []SubagentHandoffRef{}
	}
	if bundle.ArtifactRefs == nil {
		bundle.ArtifactRefs = []ArtifactRef{}
	}
}

func validateConversationProjection(boundaryID string, projection CompactConversationProjectionV2) error {
	if projection.Format != CompactProjectionV1Messages && projection.Format != CompactProjectionCheckpointV2 {
		return fmt.Errorf("context bundle projection format %q is invalid", projection.Format)
	}
	if projection.Format == CompactProjectionV1Messages && len(projection.Messages) == 0 {
		return fmt.Errorf("v1_messages projection requires messages")
	}
	messages := make([]convcontext.Message, 0, len(projection.Messages))
	for index, message := range projection.Messages {
		role := convcontext.Role(strings.TrimSpace(message.Role))
		switch role {
		case convcontext.RoleUser, convcontext.RoleAssistant, convcontext.RoleTool:
		default:
			return fmt.Errorf("conversation_projection.messages[%d] role is invalid", index)
		}
		compactBoundaryID := strings.TrimSpace(message.CompactBoundaryID)
		if index == 0 && compactBoundaryID != strings.TrimSpace(boundaryID) {
			return fmt.Errorf("conversation projection boundary mismatch")
		}
		messages = append(messages, convcontext.Message{
			Role: role, Content: message.Content, ToolCalls: append([]convcontext.ToolCall(nil), message.ToolCalls...),
			ToolCallID: message.ToolCallID, Name: message.Name, IsError: message.IsError,
			CompactBoundaryID: compactBoundaryID,
		})
	}
	if err := convcontext.ValidateToolResultPairing(messages); err != nil {
		return fmt.Errorf("context bundle tool pairing: %w", err)
	}
	return nil
}

func validateMaterializedSection[T any](name string, section MaterializedSection[T]) error {
	switch section.State {
	case MaterializationNotMaterialized:
		if section.Value != nil || section.Digest != EmptySectionDigest || section.Reason != "" {
			return fmt.Errorf("%s not_materialized state is inconsistent", name)
		}
	case MaterializationUnavailable:
		if section.Value != nil || section.Digest != EmptySectionDigest || !validStableReason(section.Reason) {
			return fmt.Errorf("%s unavailable state is inconsistent", name)
		}
	case MaterializationMaterialized:
		if section.Value == nil || section.Reason != "" {
			return fmt.Errorf("%s materialized state is inconsistent", name)
		}
		expected, err := digestCanonicalValue(*section.Value)
		if err != nil || section.Digest != expected {
			return fmt.Errorf("%s materialized digest mismatch: stored=%s expected=%s", name, section.Digest, expected)
		}
	default:
		return fmt.Errorf("%s state %q is invalid", name, section.State)
	}
	return nil
}

func validateSnapshotRef(name string, ref SnapshotRef) error {
	switch ref.State {
	case ReferenceAvailable:
		if strings.TrimSpace(ref.ID) == "" || ref.Reason != "" {
			return fmt.Errorf("%s available reference is inconsistent", name)
		}
		return validateDigest(name+".digest", ref.Digest)
	case ReferenceUnavailable:
		if ref.ID != "" || ref.Revision != 0 || ref.Digest != EmptySectionDigest || !validStableReason(ref.Reason) {
			return fmt.Errorf("%s unavailable reference is inconsistent", name)
		}
		return nil
	default:
		return fmt.Errorf("%s state %q is invalid", name, ref.State)
	}
}

func validateMemorySnapshotRef(ref MemoryRecallSnapshotRef) error {
	switch ref.State {
	case ReferenceAvailable:
		if strings.TrimSpace(ref.WorkspaceRoot) == "" || len(ref.Items) == 0 || ref.Reason != "" {
			return fmt.Errorf("memory_snapshot available reference is inconsistent")
		}
		for index, item := range ref.Items {
			if strings.TrimSpace(item.MemoryID) == "" || item.Revision < 1 {
				return fmt.Errorf("memory_snapshot.items[%d] identity is invalid", index)
			}
			if err := validateDigest(fmt.Sprintf("memory_snapshot.items[%d].content_digest", index), item.ContentDigest); err != nil {
				return err
			}
		}
		copyRef := ref
		copyRef.Digest = ""
		expected, err := digestCanonicalValue(copyRef)
		if err != nil || ref.Digest != expected {
			return fmt.Errorf("memory_snapshot digest mismatch")
		}
		return nil
	case ReferenceUnavailable:
		if ref.WorkspaceRoot != "" || len(ref.Items) != 0 || ref.Digest != EmptySectionDigest || !validStableReason(ref.Reason) {
			return fmt.Errorf("memory_snapshot unavailable reference is inconsistent")
		}
		return nil
	default:
		return fmt.Errorf("memory_snapshot state %q is invalid", ref.State)
	}
}

func validateDigest(name, value string) error {
	if len(value) != sha256.Size*2 {
		return fmt.Errorf("%s must be a sha256 hex digest", name)
	}
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != sha256.Size || strings.ToLower(value) != value {
		return fmt.Errorf("%s must be a lowercase sha256 hex digest", name)
	}
	return nil
}

func validStableReason(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || value[0] == '_' || value[len(value)-1] == '_' {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || character == '_' {
			continue
		}
		return false
	}
	return true
}
