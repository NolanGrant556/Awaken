package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"awaken/src/agentcore"
	"awaken/src/subagentruntime"
	"crypto/sha256"
)

func restoreCollaboration(records []agentcore.SemanticRecord) ([]subagentruntime.RestoredTask, error) {
	byID := map[string]*subagentruntime.RestoredTask{}
	var order []string
	delivered := map[string]bool{}
	incorporated := map[string]bool{}
	researchRuns := map[string]bool{}
	for _, record := range activeSemanticRecords(records) {
		if strings.HasPrefix(string(record.Kind), "research_") {
			researchRuns[record.RunID] = true
		}
		if record.Kind == agentcore.SemanticKindRuntimeNotice && stringPayload(record.Payload, "event_type") == "agent_context_message" {
			if id := strings.TrimPrefix(stringPayload(record.Payload, "message_id"), "subagent_terminal_"); id != stringPayload(record.Payload, "message_id") {
				delivered[id] = true
			}
		}
		if record.Kind == agentcore.SemanticKindTurnBoundary {
			var ids []string
			decodeSemanticUserMessageField(record.Payload, "incorporated_message_ids", &ids)
			for _, id := range ids {
				incorporated[id] = true
			}
		}
		if record.Kind == agentcore.SemanticKindToolResult {
			var ids []string
			decodeSemanticUserMessageField(record.Payload, "terminal_subagent_task_ids", &ids)
			for _, id := range ids {
				delivered[id] = true
			}
		}
		if !strings.HasPrefix(string(record.Kind), "subagent_") {
			continue
		}
		id := stringPayload(record.Payload, "task_id")
		if id == "" {
			continue
		}
		entry := byID[id]
		if entry == nil {
			entry = &subagentruntime.RestoredTask{}
			byID[id] = entry
			order = append(order, id)
		}
		if record.Kind != agentcore.SemanticKindSubagentMessageQueued {
			encoded, err := json.Marshal(record.Payload)
			if err != nil {
				return nil, err
			}
			if err := json.Unmarshal(encoded, &entry.Snapshot); err != nil {
				return nil, err
			}
			if full := stringPayload(record.Payload, "result_content"); full != "" {
				if want := stringPayload(record.Payload, "result_digest"); want != "" && fmt.Sprintf("%x", sha256.Sum256([]byte(full))) != want {
					return nil, fmt.Errorf("child result digest mismatch")
				}
				entry.Snapshot.ResultContent = full
			}
		}
		if record.Kind == agentcore.SemanticKindSubagentSpawned {
			entry.Prompt = stringPayload(record.Payload, "assignment_prompt")
			if record.Payload["context_seed"] != nil {
				entry.ContextSeed, _ = json.Marshal(record.Payload["context_seed"])
			}
		}
		if record.Kind == agentcore.SemanticKindSubagentMessageQueued {
			entry.PendingMessages = append(entry.PendingMessages, subagentruntime.QueuedMessage{ID: stringPayload(record.Payload, "message_id"), Content: stringPayload(record.Payload, "message_content"), QueuedAt: record.Timestamp})
		}
	}
	result := make([]subagentruntime.RestoredTask, 0, len(order))
	for _, id := range order {
		entry := byID[id]
		if researchRuns[entry.Snapshot.RunID] {
			entry.Snapshot.Workflow = "research"
		}
		entry.ResultDelivered = delivered[id]
		pending := entry.PendingMessages[:0]
		for _, message := range entry.PendingMessages {
			if !incorporated[message.ID] && message.Content != "" {
				pending = append(pending, message)
			}
		}
		entry.PendingMessages = pending
		result = append(result, *entry)
	}
	return result, nil
}

func (s *RunService) childContextLoader(threadID string) func(context.Context, string) (*agentcore.ChildRestoreState, error) {
	return func(ctx context.Context, agentID string) (*agentcore.ChildRestoreState, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if agentID == "" || agentID == "root" || s.journals == nil {
			return nil, fmt.Errorf("invalid child context scope")
		}
		journal, err := s.journals.Open(threadID)
		if err != nil {
			return nil, err
		}
		records, err := journal.SemanticRecordsForAgent(agentID, 1, journal.LastAssignedSequence())
		if err != nil {
			return nil, err
		}
		state := &agentcore.ChildRestoreState{}
		modelTurns := make(map[string]bool)
		projected := make([]agentcore.SemanticRecord, 0, len(records))
		for _, record := range records {
			if state.RoleDigest == "" {
				state.RoleDigest = stringPayload(record.Payload, "role_digest")
			}
			if record.Payload["worktree"] != nil {
				var receipt subagentruntime.WorktreeReceipt
				if decodeSemanticUserMessageField(record.Payload, "worktree", &receipt) {
					state.Worktree = &receipt
				}
			}
			if record.Kind == agentcore.SemanticKindSubagentSpawned && len(state.ContextSeed) == 0 && record.Payload["context_seed"] != nil {
				state.ContextSeed, err = json.Marshal(record.Payload["context_seed"])
				if err != nil {
					return nil, err
				}
			}
			if record.Kind == agentcore.SemanticKindTurnBoundary {
				state.ContextRevision = max(state.ContextRevision, uint64(intPayload(record.Payload, "context_revision")))
			}
			if record.Kind == agentcore.SemanticKindToolCall {
				state.ToolExecutions++
			}
			if record.Kind == agentcore.SemanticKindModelRequest && stringPayload(record.Payload, "purpose") == "subagent" {
				key := stringPayload(record.Payload, "logical_call_id")
				if key == "" {
					key = record.RunID + ":" + record.TurnID
				}
				if record.TurnID == "" && key == record.RunID+":" {
					key = record.RecordID
				}
				if !modelTurns[key] {
					state.ModelTurns++
					modelTurns[key] = true
				}
			}
			if record.Kind == agentcore.SemanticKindProviderOpaqueState {
				continue
			}
			copy := record
			copy.Payload = make(map[string]any, len(record.Payload))
			for key, value := range record.Payload {
				if key != "agent_id" {
					copy.Payload[key] = value
				}
			}
			projected = append(projected, copy)
		}
		if len(projected) == 0 {
			return nil, fmt.Errorf("saved Agent context was not found")
		}
		state.Messages = messagesFromSemanticRecords(projected)
		state.Bundle = latestContextBundleV2FromSemanticRecords(projected)
		state.SkillBlocks = skillContextBlocksFromSemanticRecords(projected)
		state.ActivatedTools = activatedDeferredToolsFromSemanticRecords(projected)
		state.UsageAnchor = contextWindowUsageAnchorForAgent(records, strings.TrimSpace(agentID))
		plans := runPlanSnapshotsFromSemanticRecords(projected)
		for _, record := range projected {
			if record.Kind == agentcore.SemanticKindPlanUpdated {
				if snapshot, ok := plans[record.RunID]; ok {
					copy := snapshot
					state.RunPlan = &copy
				}
			}
		}
		return state, nil
	}
}
