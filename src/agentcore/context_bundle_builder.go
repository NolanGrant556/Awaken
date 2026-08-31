package agentcore

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"awaken/src/subagentruntime"
	"awaken/src/tool"
)

func (r *Runner) buildPostCompactContextBundleV2(result CompactResult, skillBlocks []ContextBlock) (PostCompactContextBundleV2, error) {
	parentBoundaryID := lastCompactBoundaryID(r.conversation.Context.GetMessages())
	generation := 1
	prior := r.config.RestoredContextBundleV2
	if prior != nil && prior.BoundaryID == parentBoundaryID && prior.Generation > 0 {
		generation = prior.Generation + 1
	}
	sourceStart, sourceEnd := compactSourceSequenceRange(r.recorder, prior)
	agentID := ""
	if r.config.SubagentOrigin != nil {
		agentID = strings.TrimSpace(r.config.SubagentOrigin.AgentID)
	}
	sourceFacts, err := checkpointSourceFacts(r.recorder, sourceStart, sourceEnd, agentID)
	if err != nil {
		return PostCompactContextBundleV2{}, fmt.Errorf("read checkpoint source facts: %w", err)
	}
	instructionRef := instructionSnapshotRef(r.config)
	memoryRef := mergeMemorySnapshotRefs(prior, memorySnapshotRef(r.memoryRecallSnapshot))
	planRef := runPlanSnapshotRef(r.runPlanService)
	skillRefs := skillContextRefs(skillBlocks)
	toolRef := toolSnapshotRef(r.config)
	hookRef := hookSnapshotRef(r.config)
	subagentRefs := subagentHandoffRefs(r.subagentController)
	artifacts := artifactRefs(r.config)
	checkpoint, expectedFacts := buildTaskCheckpointV2(prior, r.conversation.Context.GetMessages(), sourceFacts, r.runPlanService, memoryRef, skillRefs, subagentRefs, artifacts)
	checkpointSection, err := MaterializeSection(checkpoint)
	if err != nil {
		return PostCompactContextBundleV2{}, fmt.Errorf("materialize task checkpoint: %w", err)
	}
	semanticSummary := strings.TrimSpace(result.SummaryMessage.Content)
	if semanticSummary == "" && len(result.Messages) > 0 {
		semanticSummary = strings.TrimSpace(result.Messages[0].Content)
	}
	summarySection, err := MaterializeSection(semanticSummary)
	if err != nil {
		return PostCompactContextBundleV2{}, fmt.Errorf("materialize semantic summary: %w", err)
	}
	coverageSection, err := MaterializeSection(checkpointCoverageReceipt(expectedFacts, checkpoint))
	if err != nil {
		return PostCompactContextBundleV2{}, fmt.Errorf("materialize coverage receipt: %w", err)
	}
	recentTurnsSection, err := MaterializeSection(compactTurnsFromMessages(r.conversation.Context.GetMessages()))
	if err != nil {
		return PostCompactContextBundleV2{}, fmt.Errorf("materialize recent turns: %w", err)
	}
	transcriptSection := NotMaterializedSection[TranscriptLocator]()
	if sourceEnd > 0 {
		transcriptSection, err = MaterializeSection(TranscriptLocator{
			ThreadID: r.threadID, SequenceFrom: sourceStart, SequenceTo: sourceEnd, BoundaryID: strings.TrimSpace(result.BoundaryID),
		})
		if err != nil {
			return PostCompactContextBundleV2{}, fmt.Errorf("materialize transcript locator: %w", err)
		}
	}

	providerOpaqueSection := NotMaterializedSection[ProviderOpaqueCompactionRef]()
	if result.NativeCompactionState != nil {
		providerOpaqueSection, err = MaterializeSection(providerOpaqueCompactionRef(result.NativeCompactionState))
		if err != nil {
			return PostCompactContextBundleV2{}, fmt.Errorf("materialize provider opaque compaction: %w", err)
		}
	}
	bundle := PostCompactContextBundleV2{
		SchemaVersion:       PostCompactContextBundleSchemaVersion,
		BoundaryID:          strings.TrimSpace(result.BoundaryID),
		ParentBoundaryID:    parentBoundaryID,
		Generation:          generation,
		Mode:                string(result.Mode),
		SourceSequenceStart: sourceStart,
		SourceSequenceEnd:   sourceEnd,
		ConversationProjection: CompactConversationProjectionV2{
			Format: CompactProjectionV1Messages, Messages: semanticCompactedMessages(result.Messages),
		},
		TaskCheckpoint:         checkpointSection,
		SemanticSummary:        summarySection,
		RecentTurns:            recentTurnsSection,
		InstructionSnapshotRef: instructionRef,
		MemorySnapshot:         memoryRef,
		RunPlanRef:             planRef,
		SkillRefs:              skillRefs,
		ToolSnapshotRef:        toolRef,
		HookSnapshotRef:        hookRef,
		ActiveGateRefs:         []GateRef{},
		PendingSubagents:       subagentRefs,
		ArtifactRefs:           artifacts,
		TranscriptLocator:      transcriptSection,
		ProviderOpaqueState:    providerOpaqueSection,
		BudgetReceipt:          NotMaterializedSection[CompactBudgetReceipt](),
		CoverageReceipt:        coverageSection,
	}
	if r.config.ModelConfig.ContextWindow > 0 || r.config.ModelConfig.MaxInputTokens > 0 {
		budgetReceipt, budgetErr := r.evaluatePostCompactBudgetWithRecentTurnAllocation(&bundle, result, skillBlocks)
		if budgetErr != nil {
			return PostCompactContextBundleV2{}, budgetErr
		}
		bundle.BudgetReceipt, err = MaterializeSection(budgetReceipt)
		if err != nil {
			return PostCompactContextBundleV2{}, fmt.Errorf("materialize budget receipt: %w", err)
		}
	}
	// Prepared-body projection may normalize raw provider items while validating
	// their replay shape. Seal the materialized section from its final value so
	// the section digest and the enclosing bundle digest cover identical bytes.
	if bundle.ProviderOpaqueState.State == MaterializationMaterialized && bundle.ProviderOpaqueState.Value != nil {
		bundle.ProviderOpaqueState, err = MaterializeSection(*bundle.ProviderOpaqueState.Value)
		if err != nil {
			return PostCompactContextBundleV2{}, fmt.Errorf("seal provider opaque compaction: %w", err)
		}
	}
	if err := bundle.Seal(); err != nil {
		return PostCompactContextBundleV2{}, err
	}
	return bundle, nil
}

func (r *Runner) evaluatePostCompactBudgetWithRecentTurnAllocation(
	bundle *PostCompactContextBundleV2,
	result CompactResult,
	skillBlocks []ContextBlock,
) (CompactBudgetReceipt, error) {
	receipt, err := r.evaluatePostCompactBudget(bundle, result, skillBlocks)
	for err != nil && errors.Is(err, ErrCompactTargetUnachievable) &&
		bundle != nil && bundle.RecentTurns.State == MaterializationMaterialized &&
		bundle.RecentTurns.Value != nil && len(*bundle.RecentTurns.Value) > 0 {
		turns := append([]CompactTurnV2(nil), (*bundle.RecentTurns.Value)...)
		turns = turns[1:]
		section, sectionErr := MaterializeSection(turns)
		if sectionErr != nil {
			return CompactBudgetReceipt{}, fmt.Errorf("allocate recent turns: %w", sectionErr)
		}
		bundle.RecentTurns = section
		receipt, err = r.evaluatePostCompactBudget(bundle, result, skillBlocks)
	}
	if err != nil && errors.Is(err, ErrCompactTargetUnachievable) && len(skillBlocks) > 0 {
		if deferErr := deferPostCompactSkillProjection(bundle); deferErr != nil {
			return CompactBudgetReceipt{}, deferErr
		}
		receipt, err = r.evaluatePostCompactBudget(bundle, result, nil)
	}
	return receipt, err
}

func deferPostCompactSkillProjection(bundle *PostCompactContextBundleV2) error {
	if bundle == nil || len(bundle.SkillRefs) == 0 {
		return nil
	}
	for index := range bundle.SkillRefs {
		bundle.SkillRefs[index].ProjectedContent = ""
		if err := bundle.SkillRefs[index].Seal(); err != nil {
			return fmt.Errorf("defer compacted skill projection: %w", err)
		}
	}
	if bundle.TaskCheckpoint.State == MaterializationMaterialized && bundle.TaskCheckpoint.Value != nil {
		checkpoint := cloneTaskCheckpoint(*bundle.TaskCheckpoint.Value)
		checkpoint.SkillRefs = checkpoint.SkillRefs[:0]
		for _, ref := range bundle.SkillRefs {
			checkpoint.SkillRefs = append(checkpoint.SkillRefs, FactRef{FactID: ref.SkillID, Digest: ref.Digest})
		}
		section, err := MaterializeSection(checkpoint)
		if err != nil {
			return fmt.Errorf("materialize deferred skill checkpoint: %w", err)
		}
		bundle.TaskCheckpoint = section
	}
	return nil
}

func compactSourceSequenceRange(recorder SemanticRecorder, prior *PostCompactContextBundleV2) (int64, int64) {
	reader, ok := recorder.(SemanticSequenceReader)
	if !ok || reader == nil {
		return 0, 0
	}
	end := int64(reader.LastAssignedSequence())
	if end <= 0 {
		return 0, 0
	}
	start := int64(1)
	if prior != nil && prior.SourceSequenceEnd > 0 && prior.SourceSequenceEnd < end {
		start = prior.SourceSequenceEnd + 1
	}
	return start, end
}

func instructionSnapshotRef(config AgentConfig) SnapshotRef {
	if config.InstructionSnapshot == nil || strings.TrimSpace(config.InstructionSnapshot.Summary.SnapshotID) == "" || strings.TrimSpace(config.InstructionSnapshot.Digest) == "" {
		return UnavailableSnapshotRef("instruction_snapshot_unavailable")
	}
	return SnapshotRef{
		State: ReferenceAvailable, ID: config.InstructionSnapshot.Summary.SnapshotID,
		Digest: strings.ToLower(config.InstructionSnapshot.Digest),
	}
}

func memorySnapshotRef(snapshot *MemoryRecallSnapshot) MemoryRecallSnapshotRef {
	if snapshot == nil || strings.TrimSpace(snapshot.WorkspaceRoot) == "" || len(snapshot.Items) == 0 {
		return UnavailableMemorySnapshotRef("memory_snapshot_unavailable")
	}
	items := make([]MemoryRecallItemRef, 0, len(snapshot.Items))
	for _, item := range snapshot.Items {
		if strings.TrimSpace(item.MemoryID) == "" || item.Revision < 1 {
			continue
		}
		items = append(items, MemoryRecallItemRef{
			MemoryID: item.MemoryID, Scope: item.Scope, Type: item.Type,
			Revision: item.Revision, Bytes: item.Bytes, ContentDigest: digestText(item.Content),
		})
	}
	if len(items) == 0 {
		return UnavailableMemorySnapshotRef("memory_snapshot_unavailable")
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].MemoryID < items[j].MemoryID })
	ref := MemoryRecallSnapshotRef{State: ReferenceAvailable, WorkspaceRoot: snapshot.WorkspaceRoot, Items: items}
	digest, _ := digestCanonicalValue(ref)
	ref.Digest = digest
	return ref
}

func mergeMemorySnapshotRefs(prior *PostCompactContextBundleV2, current MemoryRecallSnapshotRef) MemoryRecallSnapshotRef {
	if prior == nil || prior.MemorySnapshot.State != ReferenceAvailable {
		return current
	}
	workspaceRoot := strings.TrimSpace(current.WorkspaceRoot)
	if workspaceRoot == "" {
		workspaceRoot = prior.MemorySnapshot.WorkspaceRoot
	}
	items := make(map[string]MemoryRecallItemRef, len(prior.MemorySnapshot.Items)+len(current.Items))
	for _, item := range prior.MemorySnapshot.Items {
		items[item.MemoryID] = item
	}
	if current.State == ReferenceAvailable {
		for _, item := range current.Items {
			items[item.MemoryID] = item
		}
	}
	merged := MemoryRecallSnapshotRef{State: ReferenceAvailable, WorkspaceRoot: workspaceRoot, Items: make([]MemoryRecallItemRef, 0, len(items))}
	for _, item := range items {
		merged.Items = append(merged.Items, item)
	}
	sort.SliceStable(merged.Items, func(i, j int) bool { return merged.Items[i].MemoryID < merged.Items[j].MemoryID })
	merged.Digest = ""
	digest, err := digestCanonicalValue(merged)
	if err != nil {
		return UnavailableMemorySnapshotRef("memory_snapshot_digest_unavailable")
	}
	merged.Digest = digest
	return merged
}

func runPlanSnapshotRef(service tool.RunPlanService) SnapshotRef {
	if service == nil {
		return UnavailableSnapshotRef("run_plan_unavailable")
	}
	snapshot, ok := service.Snapshot()
	if !ok || strings.TrimSpace(snapshot.PlanID) == "" || snapshot.Version == 0 {
		return UnavailableSnapshotRef("run_plan_unavailable")
	}
	digest, err := digestCanonicalValue(snapshot)
	if err != nil {
		return UnavailableSnapshotRef("run_plan_digest_unavailable")
	}
	return SnapshotRef{State: ReferenceAvailable, ID: snapshot.PlanID, Revision: snapshot.Version, Digest: digest}
}

func skillContextRefs(blocks []ContextBlock) []SkillContextRef {
	refs := make([]SkillContextRef, 0, len(blocks))
	for _, block := range blocks {
		id := strings.TrimSpace(block.ID)
		skillID := strings.TrimSpace(block.SkillID)
		if id == "" || skillID == "" || strings.TrimSpace(block.Content) == "" {
			continue
		}
		ref := SkillContextRef{
			ID: id, SkillID: skillID, SourcePath: block.SourcePath, AgentID: block.AgentID,
			InvokedAt: block.InvokedAt.UTC(), Compacted: block.Compacted, ProjectedContent: block.Content,
		}
		_ = ref.Seal()
		refs = append(refs, ref)
	}
	return refs
}

func SkillContextBlocksFromBundleV2(bundle *PostCompactContextBundleV2) []ContextBlock {
	if bundle == nil {
		return nil
	}
	blocks := make([]ContextBlock, 0, len(bundle.SkillRefs))
	for _, ref := range bundle.SkillRefs {
		blocks = append(blocks, ContextBlock{
			ID: ref.ID, SkillID: ref.SkillID, SourcePath: ref.SourcePath, AgentID: ref.AgentID,
			InvokedAt: ref.InvokedAt, Compacted: ref.Compacted, Content: ref.ProjectedContent,
		})
	}
	return blocks
}

func toolSnapshotRef(config AgentConfig) SnapshotRef {
	snapshot := config.ToolAvailabilitySnapshot
	if snapshot == nil || strings.TrimSpace(snapshot.SnapshotID) == "" {
		return UnavailableSnapshotRef("tool_snapshot_unavailable")
	}
	identity := struct {
		SnapshotID            string `json:"snapshot_id"`
		CatalogVersion        uint64 `json:"catalog_version"`
		EnvironmentSnapshotID string `json:"environment_snapshot_id"`
	}{snapshot.SnapshotID, snapshot.CatalogVersion, snapshot.EnvironmentSnapshotID}
	digest, _ := digestCanonicalValue(identity)
	return SnapshotRef{State: ReferenceAvailable, ID: snapshot.SnapshotID, Revision: snapshot.CatalogVersion, Digest: digest}
}

func hookSnapshotRef(config AgentConfig) SnapshotRef {
	snapshot := config.HookAvailabilitySnapshot
	if snapshot == nil || strings.TrimSpace(snapshot.SnapshotID) == "" {
		return UnavailableSnapshotRef("hook_snapshot_unavailable")
	}
	digest, err := digestCanonicalValue(snapshot)
	if err != nil {
		return UnavailableSnapshotRef("hook_snapshot_digest_unavailable")
	}
	return SnapshotRef{State: ReferenceAvailable, ID: snapshot.SnapshotID, Revision: uint64(snapshot.SchemaVersion), Digest: digest}
}

func subagentHandoffRefs(controller subagentruntime.Controller) []SubagentHandoffRef {
	if controller == nil {
		return []SubagentHandoffRef{}
	}
	snapshots := controller.List()
	sort.SliceStable(snapshots, func(i, j int) bool { return snapshots[i].TaskID < snapshots[j].TaskID })
	refs := make([]SubagentHandoffRef, 0, len(snapshots))
	for _, snapshot := range snapshots {
		if strings.TrimSpace(snapshot.TaskID) == "" {
			continue
		}
		ref := SubagentHandoffRef{
			TaskID: snapshot.TaskID, AgentID: snapshot.AgentID, ParentAgentID: snapshot.ParentAgentID,
			Status: string(snapshot.Status), Description: snapshot.Description, ResultPreview: snapshot.ResultPreview,
			ErrorCode: snapshot.ErrorCode, SideEffectState: snapshot.SideEffectState,
		}
		digest, _ := digestCanonicalValue(ref)
		ref.Digest = digest
		refs = append(refs, ref)
	}
	return refs
}

func artifactRefs(config AgentConfig) []ArtifactRef {
	artifacts := append([]tool.ScopedMediaArtifact(nil), config.MediaArtifactReferences...)
	sort.SliceStable(artifacts, func(i, j int) bool { return artifacts[i].ID < artifacts[j].ID })
	refs := make([]ArtifactRef, 0, len(artifacts))
	for _, artifact := range artifacts {
		if strings.TrimSpace(artifact.ID) == "" {
			continue
		}
		ref := ArtifactRef{
			ID: artifact.ID, RunID: artifact.RunID, Kind: artifact.Kind, Path: artifact.Path,
			Role: string(artifact.Role), Status: artifact.Status,
		}
		digest, _ := digestCanonicalValue(ref)
		ref.Digest = digest
		refs = append(refs, ref)
	}
	return refs
}

func validateDirectRefDigests(bundle PostCompactContextBundleV2) error {
	for index, ref := range bundle.PendingSubagents {
		digest := ref.Digest
		ref.Digest = ""
		expected, err := digestCanonicalValue(ref)
		if err != nil || digest != expected {
			return fmt.Errorf("pending_subagents[%d] digest mismatch", index)
		}
	}
	for index, ref := range bundle.ArtifactRefs {
		digest := ref.Digest
		ref.Digest = ""
		expected, err := digestCanonicalValue(ref)
		if err != nil || digest != expected {
			return fmt.Errorf("artifact_refs[%d] digest mismatch", index)
		}
	}
	for index, ref := range bundle.ActiveGateRefs {
		if strings.TrimSpace(ref.ID) == "" || strings.TrimSpace(ref.Kind) == "" || strings.TrimSpace(ref.State) == "" {
			return fmt.Errorf("active_gate_refs[%d] identity is required", index)
		}
		digest := ref.Digest
		ref.Digest = ""
		expected, err := digestCanonicalValue(ref)
		if err != nil || digest != expected {
			return fmt.Errorf("active_gate_refs[%d] digest mismatch", index)
		}
	}
	return nil
}
