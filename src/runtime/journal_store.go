package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"awaken/src/agentcore"
	convcontext "awaken/src/context"
	"awaken/src/instructions"
	"awaken/src/subagentruntime"
	"awaken/src/tool"
)

// journalManager owns one ExecutionJournal writer per thread.
type journalManager struct {
	mu       sync.Mutex
	journals map[string]*ExecutionJournal
	pathFn   func(threadID string) string
}

func newJournalManager(pathFn func(threadID string) string) *journalManager {
	return &journalManager{
		journals: map[string]*ExecutionJournal{},
		pathFn:   pathFn,
	}
}

func (m *journalManager) Open(threadID string) (*ExecutionJournal, error) {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return nil, fmt.Errorf("thread id is required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.journals[threadID]; ok {
		return existing, nil
	}
	path := m.pathFn(threadID)
	schema, err := DetectJournalSchema(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if schema == 1 {
		return nil, fmt.Errorf("thread %s uses legacy v1 transcript", threadID)
	}
	journal, err := NewExecutionJournal(path, threadID)
	if err != nil {
		return nil, err
	}
	m.journals[threadID] = journal
	return journal, nil
}

func (m *journalManager) OpenRecorder(threadID string) (agentcore.SemanticRecorder, error) {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return nil, fmt.Errorf("thread id is required")
	}
	path := m.pathFn(threadID)
	schema, err := DetectJournalSchema(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if schema == 1 {
		return newV1SemanticRecorder(path, threadID), nil
	}
	return m.Open(threadID)
}

func (m *journalManager) Get(threadID string) (*ExecutionJournal, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	j, ok := m.journals[threadID]
	return j, ok
}

func (m *journalManager) Flush(ctx context.Context, threadID string) error {
	journal, ok := m.Get(threadID)
	if !ok {
		return nil
	}
	return journal.Flush(ctx)
}

func (m *journalManager) FlushAll(ctx context.Context) error {
	m.mu.Lock()
	journals := make([]*ExecutionJournal, 0, len(m.journals))
	for _, journal := range m.journals {
		journals = append(journals, journal)
	}
	m.mu.Unlock()
	var firstErr error
	for _, journal := range journals {
		if err := journal.Flush(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (m *journalManager) CloseThread(threadID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	journal, ok := m.journals[threadID]
	if !ok {
		return nil
	}
	delete(m.journals, threadID)
	return journal.Close()
}

func ensureThreadJournalRecord(ctx context.Context, journal *ExecutionJournal, threadID, label string) error {
	records, err := ReadExecutionJournal(journal.Path())
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, record := range records {
		if record.Kind == agentcore.SemanticKindThread {
			return nil
		}
	}
	return journal.Append(ctx, agentcore.NewSemanticRecord(
		agentcore.SemanticKindThread,
		threadID,
		"",
		"",
		"",
		"",
		map[string]any{"label": trimThreadLabel(label)},
	))
}

func loadSemanticRecordsForThread(path string) ([]agentcore.SemanticRecord, int, error) {
	schema, err := DetectJournalSchema(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, 0, err
		}
		// Empty file returns schema 2.
		if !os.IsNotExist(err) && schema == 0 {
			return nil, 0, err
		}
	}
	if schema == 1 {
		records, err := (LegacyTranscriptAdapter{}).ReadFile(path)
		return records, 1, err
	}
	records, err := ReadExecutionJournal(path)
	return records, agentcore.SemanticSchemaVersion, err
}

func messagesFromSemanticRecords(records []agentcore.SemanticRecord) []convcontext.Message {
	messages := make([]convcontext.Message, 0, len(records))
	pendingAssistant := map[string]int{}
	pendingProviderState := map[string]*convcontext.ProviderOpaqueMessageState{}

	for _, record := range records {
		if strings.TrimSpace(stringPayload(record.Payload, "agent_id")) != "" {
			// Child-Agent conversations stay out of the reusable parent model
			// transcript. Their records remain available to replay and audit.
			continue
		}
		switch record.Kind {
		case agentcore.SemanticKindUserMessage:
			messages = append(messages, convcontext.Message{
				Role:    convcontext.RoleUser,
				Content: stringPayload(record.Payload, "content"),
				Attachments: transcriptAttachmentsToContextAttachments(
					transcriptAttachmentsFromSemanticUserMessage(record.Payload),
				),
			})
		case agentcore.SemanticKindAssistantMessage:
			id := stringPayload(record.Payload, "message_id")
			if index, exists := pendingAssistant[id]; id != "" && exists && index >= 0 && index < len(messages) {
				messages[index].Content = stringPayload(record.Payload, "content")
				continue
			}
			msg := convcontext.Message{Role: convcontext.RoleAssistant, Content: stringPayload(record.Payload, "content"), MessageID: id}
			if state := pendingProviderState[id]; state != nil {
				msg.ProviderOpaqueState = state
			}
			messages = append(messages, msg)
			if id != "" {
				pendingAssistant[id] = len(messages) - 1
			}
		case agentcore.SemanticKindProviderOpaqueState:
			id := stringPayload(record.Payload, "message_id")
			state, ok := providerOpaqueStateFromPayload(record.Payload)
			if !ok || id == "" {
				continue
			}
			pendingProviderState[id] = state
			if index, exists := pendingAssistant[id]; exists && index >= 0 && index < len(messages) {
				messages[index].ProviderOpaqueState = state
				continue
			}
			messages = append(messages, convcontext.Message{Role: convcontext.RoleAssistant, MessageID: id, ProviderOpaqueState: state})
			pendingAssistant[id] = len(messages) - 1
		case agentcore.SemanticKindToolCall:
			call := convcontext.ToolCall{
				ID:   stringPayload(record.Payload, "tool_call_id"),
				Type: "function",
				Function: convcontext.FunctionCall{
					Name:      stringPayload(record.Payload, "tool_name"),
					Arguments: stringPayload(record.Payload, "arguments"),
				},
			}
			if len(messages) > 0 && messages[len(messages)-1].Role == convcontext.RoleAssistant {
				messages[len(messages)-1].ToolCalls = append(messages[len(messages)-1].ToolCalls, call)
			} else {
				messages = append(messages, convcontext.Message{
					Role:      convcontext.RoleAssistant,
					ToolCalls: []convcontext.ToolCall{call},
				})
			}
		case agentcore.SemanticKindToolResult:
			content := stringPayload(record.Payload, "content")
			isError := boolPayload(record.Payload, "is_error")
			var modelContent []tool.ModelContentPart
			// SemanticJournal stores human-readable display content separately
			// from the canonical outcome. Model history must receive the same
			// compact structured payload used during the live run; feeding display
			// text here loses error_code/retryable/process facts after restart.
			if outcome, ok := executionOutcomePayload(record.Payload); ok {
				// Replay restores storage_key + metadata only. Hydration of
				// near-tail media happens later in the current-window request
				// projection, so restart and live prepared requests match.
				content = outcome.ModelPayload()
				isError = outcome.IsError()
				modelContent = tool.CloneModelContent(outcome.ModelContent)
			}
			msg := convcontext.Message{
				Role:         convcontext.RoleTool,
				Content:      content,
				ToolCallID:   stringPayload(record.Payload, "tool_call_id"),
				Name:         stringPayload(record.Payload, "tool_name"),
				IsError:      isError,
				ModelContent: modelContent,
			}
			messages = append(messages, msg)
		case agentcore.SemanticKindContextCompacted:
			compacted, ok := compactedMessagesFromSemanticRecord(record.ThreadID, record.Payload)
			if !ok {
				// A malformed compaction record must not erase the last known-good
				// conversation. The record remains auditable while replay keeps the
				// pre-compaction messages as its fail-closed fallback.
				continue
			}
			messages = compacted
			pendingAssistant = map[string]int{}
			pendingProviderState = map[string]*convcontext.ProviderOpaqueMessageState{}
		}
	}
	_ = pendingAssistant
	return messages
}

func compactedMessagesFromSemanticRecord(_ string, payload map[string]any) ([]convcontext.Message, bool) {
	var durable []agentcore.SemanticCompactedMessage
	if !decodeSemanticUserMessageField(payload, "messages", &durable) || len(durable) == 0 {
		return nil, false
	}
	boundaryID := strings.TrimSpace(stringPayload(payload, "compact_boundary_id"))
	messages := make([]convcontext.Message, 0, len(durable))
	for _, message := range durable {
		role := convcontext.Role(strings.TrimSpace(message.Role))
		switch role {
		case convcontext.RoleUser, convcontext.RoleAssistant, convcontext.RoleTool:
		default:
			return nil, false
		}
		attachments := make([]transcriptMessageAttachment, 0, len(message.Attachments))
		for _, attachment := range message.Attachments {
			attachments = append(attachments, transcriptMessageAttachment{
				ID:          attachment.ID,
				Kind:        RunInputAttachmentKind(attachment.Kind),
				MimeType:    attachment.MimeType,
				Path:        attachment.Path,
				DisplayName: attachment.DisplayName,
				Width:       attachment.Width,
				Height:      attachment.Height,
				ByteSize:    attachment.ByteSize,
				StorageKey:  attachment.StorageKey,
			})
		}
		compactBoundaryID := strings.TrimSpace(message.CompactBoundaryID)
		content := message.Content
		modelContent := tool.CloneModelContent(message.ModelContent)
		if len(messages) == 0 {
			if compactBoundaryID == "" {
				compactBoundaryID = boundaryID
			}
			if boundaryID == "" {
				boundaryID = compactBoundaryID
			}
			if compactBoundaryID == "" || compactBoundaryID != boundaryID {
				return nil, false
			}
		}
		messages = append(messages, convcontext.Message{
			Role:                role,
			Content:             content,
			Attachments:         transcriptAttachmentsToContextAttachments(attachments),
			ToolCalls:           append([]convcontext.ToolCall(nil), message.ToolCalls...),
			ToolCallID:          message.ToolCallID,
			Name:                message.Name,
			IsError:             message.IsError,
			CompactBoundaryID:   compactBoundaryID,
			ModelContent:        modelContent,
			Origin:              message.Origin,
			MessageID:           message.MessageID,
			TurnID:              message.TurnID,
			ProviderOpaqueState: message.ProviderOpaqueState,
		})
	}
	if err := convcontext.ValidateToolResultPairing(messages); err != nil {
		return nil, false
	}
	return messages, true
}

func providerOpaqueStateFromPayload(payload map[string]any) (*convcontext.ProviderOpaqueMessageState, bool) {
	raw, ok := payload["state"]
	if !ok || raw == nil {
		return nil, false
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil, false
	}
	var state convcontext.ProviderOpaqueMessageState
	if err := json.Unmarshal(encoded, &state); err != nil || strings.TrimSpace(state.Content) == "" {
		return nil, false
	}
	return &state, true
}

func executionOutcomePayload(payload map[string]any) (tool.ExecutionOutcome, bool) {
	if payload == nil {
		return tool.ExecutionOutcome{}, false
	}
	raw, ok := payload["outcome"]
	if !ok || raw == nil {
		return tool.ExecutionOutcome{}, false
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return tool.ExecutionOutcome{}, false
	}
	var outcome tool.ExecutionOutcome
	if err := json.Unmarshal(encoded, &outcome); err != nil || outcome.Status == "" {
		return tool.ExecutionOutcome{}, false
	}
	return outcome, true
}

func recoverOrphanRuns(ctx context.Context, journal *ExecutionJournal, records []agentcore.SemanticRecord) ([]agentcore.SemanticRecord, error) {
	type runState struct {
		terminal          bool
		openCalls         map[string]agentcore.SemanticRecord
		openModelRequests map[string]agentcore.SemanticRecord
		openHooks         map[string]agentcore.SemanticRecord
		openSubagents     map[string]agentcore.SemanticRecord
	}
	runs := map[string]*runState{}
	for _, record := range records {
		runID := record.RunID
		if runID == "" {
			continue
		}
		state, ok := runs[runID]
		if !ok {
			state = &runState{
				openCalls: map[string]agentcore.SemanticRecord{}, openModelRequests: map[string]agentcore.SemanticRecord{},
				openHooks: map[string]agentcore.SemanticRecord{}, openSubagents: map[string]agentcore.SemanticRecord{},
			}
			runs[runID] = state
		}
		switch record.Kind {
		case agentcore.SemanticKindRunTerminal:
			state.terminal = true
		case agentcore.SemanticKindSubagentSpawned, agentcore.SemanticKindSubagentStatusChanged, agentcore.SemanticKindSubagentProgress:
			if taskID := stringPayload(record.Payload, "task_id"); taskID != "" {
				state.openSubagents[taskID] = record
			}
		case agentcore.SemanticKindSubagentTerminal:
			delete(state.openSubagents, stringPayload(record.Payload, "task_id"))
		case agentcore.SemanticKindToolCall:
			callID := stringPayload(record.Payload, "tool_call_id")
			if callID != "" {
				state.openCalls[callID] = record
			}
		case agentcore.SemanticKindToolResult:
			callID := stringPayload(record.Payload, "tool_call_id")
			delete(state.openCalls, callID)
		case agentcore.SemanticKindModelRequest:
			requestID := stringPayload(record.Payload, "request_id")
			if requestID != "" {
				state.openModelRequests[requestID] = record
			}
		case agentcore.SemanticKindModelUsage:
			requestID := stringPayload(record.Payload, "request_id")
			delete(state.openModelRequests, requestID)
		case agentcore.SemanticKindHookInvocationStarted:
			invocationID := stringPayload(record.Payload, "invocation_id")
			if invocationID != "" {
				state.openHooks[invocationID] = record
			}
		case agentcore.SemanticKindHookInvocationCompleted:
			delete(state.openHooks, stringPayload(record.Payload, "invocation_id"))
		}
	}

	var appended []agentcore.SemanticRecord
	for runID, state := range runs {
		if state.terminal && len(state.openCalls) == 0 && len(state.openModelRequests) == 0 && len(state.openHooks) == 0 && len(state.openSubagents) == 0 {
			continue
		}
		for taskID, latest := range state.openSubagents {
			payload := map[string]any{}
			for key, value := range latest.Payload {
				payload[key] = value
			}
			payload["task_id"] = taskID
			payload["status"] = string(subagentruntime.StatusInterrupted)
			payload["error_code"] = subagentruntime.ErrorInterrupted
			payload["error_message"] = "subagent was interrupted by process restart"
			payload["current_activity"] = ""
			now := time.Now().UTC().Format(time.RFC3339Nano)
			payload["finished_at"] = now
			terminal := agentcore.NewSemanticRecord(
				agentcore.SemanticKindSubagentTerminal,
				journal.ThreadID(), runID, latest.TurnID, latest.ParentID, latest.SessionID, payload,
			)
			assigned, err := journal.AppendAssigned(ctx, terminal)
			if err != nil {
				return appended, err
			}
			appended = append(appended, assigned)
		}
		for invocationID, started := range state.openHooks {
			completed := agentcore.NewSemanticRecord(
				agentcore.SemanticKindHookInvocationCompleted,
				journal.ThreadID(), runID, started.TurnID, "", started.SessionID,
				map[string]any{
					"invocation_id": invocationID,
					"hook_id":       stringPayload(started.Payload, "hook_id"),
					"event":         stringPayload(started.Payload, "event"),
					"source_kind":   stringPayload(started.Payload, "source_kind"),
					"status":        "interrupted",
					"spawn_state":   "unknown",
					"process_state": "unknown",
					"action":        "continue",
					"reason_code":   "process_restarted",
				},
			)
			assigned, err := journal.AppendAssigned(ctx, completed)
			if err != nil {
				return appended, err
			}
			appended = append(appended, assigned)
		}
		for requestID, requestRecord := range state.openModelRequests {
			usage := agentcore.ModelUsageRecord{
				SchemaVersion: 1, RequestID: requestID,
				Purpose:        stringPayload(requestRecord.Payload, "purpose"),
				ModelID:        stringPayload(requestRecord.Payload, "model_id"),
				ProtocolFamily: stringPayload(requestRecord.Payload, "protocol_family"),
				Status:         agentcore.ModelUsageInterrupted, ErrorCode: "usage_unknown",
				CompletedAt: time.Now().UTC(),
			}
			if estimate := intPayload(requestRecord.Payload, "estimated_input_tokens"); estimate > 0 {
				usage.SentEstimatedInputTokens = &estimate
			}
			usageRecord := agentcore.ModelUsageSemanticRecord(
				journal.ThreadID(), runID, requestRecord.TurnID, requestRecord.SessionID, usage,
			)
			assigned, err := journal.AppendAssigned(ctx, usageRecord)
			if err != nil {
				return appended, err
			}
			appended = append(appended, assigned)
		}
		for callID, callRecord := range state.openCalls {
			outcome := SyntheticInterruptedToolResult()
			payload := map[string]any{
				"tool_call_id": callID,
				"tool_name":    stringPayload(callRecord.Payload, "tool_name"),
				"content":      outcome.Content,
				"is_error":     true,
				"outcome":      outcomeToAnyMap(outcome),
			}
			resultRec := agentcore.NewSemanticRecord(
				agentcore.SemanticKindToolResult,
				journal.ThreadID(),
				runID,
				callRecord.TurnID,
				callID,
				callRecord.SessionID,
				payload,
			)
			// Preserve original seq numbering by letting journal assign next seq.
			assigned, err := journal.AppendAssigned(ctx, resultRec)
			if err != nil {
				return records, err
			}
			appended = append(appended, assigned)
		}
		if !state.terminal {
			terminal := agentcore.NewSemanticRecord(
				agentcore.SemanticKindRunTerminal,
				journal.ThreadID(),
				runID,
				"",
				"",
				"",
				map[string]any{
					"status": "interrupted",
					"reason": "process_restarted",
				},
			)
			assignedTerminal, err := journal.AppendAssigned(ctx, terminal)
			if err != nil {
				return records, err
			}
			appended = append(appended, assignedTerminal)
		}
		if err := journal.Flush(ctx); err != nil {
			return records, err
		}
	}
	if len(appended) == 0 {
		return records, nil
	}
	return append(append([]agentcore.SemanticRecord{}, records...), appended...), nil
}

func outcomeToAnyMap(outcome tool.ExecutionOutcome) map[string]any {
	raw, err := json.Marshal(outcome)
	if err != nil {
		return map[string]any{
			"status":           string(outcome.Status),
			"error_code":       outcome.ErrorCode,
			"error_category":   string(outcome.ErrorCategory),
			"retryable":        outcome.Retryable,
			"transport_status": string(outcome.TransportStatus),
			"semantic_status":  string(outcome.SemanticStatus),
			"process_status":   string(outcome.ProcessStatus),
			"diagnostic":       outcome.Diagnostic,
		}
	}
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return out
}

func stringPayload(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	value, ok := payload[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return fmt.Sprint(typed)
	}
}

func boolPayload(payload map[string]any, key string) bool {
	if payload == nil {
		return false
	}
	value, ok := payload[key]
	if !ok || value == nil {
		return false
	}
	switch typed := value.(type) {
	case bool:
		return typed
	default:
		return false
	}
}

func intPayload(payload map[string]any, key string) int {
	if payload == nil {
		return 0
	}
	value, ok := payload[key]
	if !ok || value == nil {
		return 0
	}
	switch typed := value.(type) {
	case int:
		return typed
	case int8:
		return int(typed)
	case int16:
		return int(typed)
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case uint:
		return int(typed)
	case uint8:
		return int(typed)
	case uint16:
		return int(typed)
	case uint32:
		return int(typed)
	case uint64:
		return int(typed)
	case float32:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		value, err := typed.Int64()
		if err == nil {
			return int(value)
		}
	}
	return 0
}

func runIDsFromSemanticRecords(records []agentcore.SemanticRecord) []string {
	seen := map[string]bool{}
	var out []string
	for _, record := range records {
		if record.RunID == "" || seen[record.RunID] {
			continue
		}
		seen[record.RunID] = true
		out = append(out, record.RunID)
	}
	return out
}

func restoredTranscriptFromSemantic(records []agentcore.SemanticRecord) restoredTranscript {
	restored := restoredTranscript{
		Messages:           messagesFromSemanticRecords(records),
		RunIDs:             runIDsFromSemanticRecords(records),
		SkillContextBlocks: skillContextBlocksFromSemanticRecords(records),
		SkillInvokedAt:     skillInvocationTimesFromSemanticRecords(records),
		RunPlanSnapshots:   runPlanSnapshotsFromSemanticRecords(records),
	}
	for _, record := range records {
		if restored.CreatedAt.IsZero() || record.Timestamp.Before(restored.CreatedAt) {
			restored.CreatedAt = record.Timestamp
		}
		if record.Timestamp.After(restored.UpdatedAt) {
			restored.UpdatedAt = record.Timestamp
		}
		if record.Kind == agentcore.SemanticKindThread {
			restored.Label = trimThreadLabel(stringPayload(record.Payload, "label"))
		}
	}
	if restored.CreatedAt.IsZero() {
		restored.CreatedAt = time.Now().UTC()
	}
	if restored.UpdatedAt.IsZero() {
		restored.UpdatedAt = restored.CreatedAt
	}
	return restored
}

func skillContextBlocksFromSemanticRecords(records []agentcore.SemanticRecord) []agentcore.ContextBlock {
	latest := make(map[string]agentcore.ContextBlock)
	order := make([]string, 0)
	for _, record := range records {
		if record.Kind != agentcore.SemanticKindSkillActivated {
			continue
		}
		// Fork bodies and child-local invocations never become parent system
		// context on restart. The parent retains only the child result already
		// recorded in its Skill tool exchange.
		if strings.EqualFold(strings.TrimSpace(stringPayload(record.Payload, "context")), "fork") ||
			strings.TrimSpace(stringPayload(record.Payload, "agent_id")) != "" {
			continue
		}
		skillID := strings.TrimSpace(stringPayload(record.Payload, "skill_id"))
		content := strings.TrimSpace(stringPayload(record.Payload, "content"))
		if skillID == "" || content == "" {
			continue
		}
		if _, exists := latest[skillID]; !exists {
			order = append(order, skillID)
		}
		invokedAt := record.Timestamp
		if raw := strings.TrimSpace(stringPayload(record.Payload, "invoked_at")); raw != "" {
			if parsed, err := time.Parse(time.RFC3339Nano, raw); err == nil {
				invokedAt = parsed
			}
		}
		latest[skillID] = agentcore.ContextBlock{
			ID:         "skill_activation_" + skillID,
			SkillID:    skillID,
			SourcePath: stringPayload(record.Payload, "skill_path"),
			AgentID:    stringPayload(record.Payload, "agent_id"),
			InvokedAt:  invokedAt,
			Content:    content,
		}
	}
	blocks := make([]agentcore.ContextBlock, 0, len(order))
	for _, skillID := range order {
		blocks = append(blocks, latest[skillID])
	}
	return blocks
}

// runPlanSnapshotsFromSemanticRecords rebuilds the latest root run plan
// snapshot per run from durable plan_updated records (运行计划契约 §7.3).
// Child-local records (payload agent_id set) never contribute, and a lower
// version never replaces a higher one — replay is idempotent by plan version.
func runPlanSnapshotsFromSemanticRecords(records []agentcore.SemanticRecord) map[string]tool.RunPlanSnapshot {
	latest := map[string]tool.RunPlanSnapshot{}
	for _, record := range records {
		if record.Kind != agentcore.SemanticKindPlanUpdated || strings.TrimSpace(record.RunID) == "" {
			continue
		}
		if strings.TrimSpace(stringPayload(record.Payload, "agent_id")) != "" {
			continue
		}
		planID := strings.TrimSpace(stringPayload(record.Payload, "plan_id"))
		version := uint64(intPayload(record.Payload, "version"))
		if planID == "" || version == 0 {
			continue
		}
		if existing, ok := latest[record.RunID]; ok && existing.Version >= version {
			continue
		}
		snapshot := tool.RunPlanSnapshot{
			PlanID:      planID,
			Version:     version,
			Explanation: stringPayload(record.Payload, "explanation"),
			UpdatedAt:   record.Timestamp.UTC(),
		}
		// Payloads arrive both as JSON round-tripped []any (journal replay)
		// and as in-memory []map[string]any (direct recorder captures);
		// normalize through JSON instead of asserting either concrete type.
		if rawItems, err := json.Marshal(record.Payload["items"]); err == nil {
			var items []tool.RunPlanItem
			if json.Unmarshal(rawItems, &items) == nil {
				snapshot.Items = items
			}
		}
		latest[record.RunID] = snapshot
	}
	return latest
}

func skillInvocationTimesFromSemanticRecords(records []agentcore.SemanticRecord) map[string]time.Time {
	result := map[string]time.Time{}
	for _, record := range records {
		if record.Kind != agentcore.SemanticKindSkillActivated {
			continue
		}
		skillID := strings.TrimSpace(stringPayload(record.Payload, "skill_id"))
		if skillID == "" {
			continue
		}
		invokedAt := record.Timestamp.UTC()
		if raw := strings.TrimSpace(stringPayload(record.Payload, "invoked_at")); raw != "" {
			if parsed, err := time.Parse(time.RFC3339Nano, raw); err == nil {
				invokedAt = parsed.UTC()
			}
		}
		if previous, exists := result[skillID]; !exists || invokedAt.After(previous) {
			result[skillID] = invokedAt
		}
	}
	return result
}

func durableRunsFromSemanticRecords(threadID string, records []agentcore.SemanticRecord) map[string]ThreadTranscriptRun {
	byRun := map[string][]agentcore.SemanticRecord{}
	runMeta := map[string]*Run{}
	seenUserInput := map[string]bool{}
	for _, record := range records {
		if record.RunID == "" {
			continue
		}
		byRun[record.RunID] = append(byRun[record.RunID], record)
		run, ok := runMeta[record.RunID]
		if !ok {
			created := record.Timestamp
			run = &Run{
				RunID:     record.RunID,
				ThreadID:  firstNonEmpty(record.ThreadID, threadID),
				Status:    RunStatusRunning,
				CreatedAt: created,
				UpdatedAt: created,
			}
			runMeta[record.RunID] = run
		}
		if record.Timestamp.Before(run.CreatedAt) || run.CreatedAt.IsZero() {
			run.CreatedAt = record.Timestamp
		}
		if record.Timestamp.After(run.UpdatedAt) {
			run.UpdatedAt = record.Timestamp
		}
		switch record.Kind {
		case agentcore.SemanticKindUserMessage:
			if !seenUserInput[record.RunID] {
				run.Input = stringPayload(record.Payload, "content")
				run.InputAttachments = transcriptAttachmentsToRunInputAttachments(
					transcriptAttachmentsFromSemanticUserMessage(record.Payload),
				)
				run.ComposerTokens = composerTokensFromSemanticUserMessage(record.Payload)
				seenUserInput[record.RunID] = true
			}
		case agentcore.SemanticKindRunStarted:
			run.Status = RunStatusRunning
		case agentcore.SemanticKindInstructionSnapshot:
			encoded, err := json.Marshal(record.Payload)
			if err == nil {
				var summary instructions.Summary
				if json.Unmarshal(encoded, &summary) == nil && strings.TrimSpace(summary.SnapshotID) != "" {
					run.InstructionSnapshot = &summary
				}
			}
		case agentcore.SemanticKindRunTerminal:
			status := stringPayload(record.Payload, "status")
			finished := record.Timestamp
			run.FinishedAt = &finished
			switch status {
			case "finished":
				run.Status = RunStatusFinished
				run.Result = stringPayload(record.Payload, "result")
			case "cancelled":
				run.Status = RunStatusCancelled
				run.Error = stringPayload(record.Payload, "error")
				run.Result = stringPayload(record.Payload, "result")
			case "interrupted":
				run.Status = RunStatusError
				run.Error = firstNonEmpty(stringPayload(record.Payload, "error"), "process restarted")
			default:
				run.Status = RunStatusError
				run.Error = firstNonEmpty(stringPayload(record.Payload, "error"), "run failed")
			}
		}
	}

	out := make(map[string]ThreadTranscriptRun, len(byRun))
	for runID, runRecords := range byRun {
		runRecords = semanticRecordsWithTerminalLast(runRecords)
		run := *runMeta[runID]
		events := make([]Event, 0, len(runRecords))
		for _, record := range runRecords {
			for _, agentEvent := range semanticRecordToAgentEvents(record) {
				events = append(events, Event{
					Kind:           EventKindAgent,
					Seq:            record.Seq,
					Timestamp:      record.Timestamp,
					ThreadID:       run.ThreadID,
					RunID:          runID,
					AgentEvent:     agentEvent,
					TerminalReason: agentEvent.TerminalReason,
				})
			}
		}
		out[runID] = ThreadTranscriptRun{
			Run:             run,
			Events:          events,
			SemanticRecords: append([]agentcore.SemanticRecord(nil), runRecords...),
		}
	}
	return out
}

func semanticRecordsWithTerminalLast(records []agentcore.SemanticRecord) []agentcore.SemanticRecord {
	if len(records) < 2 {
		return append([]agentcore.SemanticRecord(nil), records...)
	}
	ordered := make([]agentcore.SemanticRecord, 0, len(records))
	var finalTerminal *agentcore.SemanticRecord
	for _, record := range records {
		if record.Kind == agentcore.SemanticKindRunTerminal {
			candidate := record
			finalTerminal = &candidate
			continue
		}
		ordered = append(ordered, record)
	}
	if finalTerminal != nil {
		ordered = append(ordered, *finalTerminal)
	}
	return ordered
}

func acpReplayEvent(record agentcore.SemanticRecord, eventType agentcore.AwakenEventType) agentcore.AwakenEvent {
	diagnostic := make(map[string]any, len(record.Payload))
	for k, v := range record.Payload {
		diagnostic[k] = v
	}
	return agentcore.AwakenEvent{
		Type:          eventType,
		Timestamp:     record.Timestamp,
		ThreadID:      record.ThreadID,
		RunID:         record.RunID,
		TurnID:        record.TurnID,
		ToolCallID:    stringPayload(record.Payload, "tool_call_id"),
		ToolName:      stringPayload(record.Payload, "tool_name"),
		Content:       stringPayload(record.Payload, "content"),
		InteractionID: stringPayload(record.Payload, "interaction_id"),
		Diagnostic:    diagnostic,
	}
}

func semanticRecordToAgentEvents(record agentcore.SemanticRecord) []agentcore.AwakenEvent {
	// Local projection keeps runtime independent of importing projector cycles in tests.
	switch record.Kind {
	case agentcore.SemanticKindRunStarted:
		return []agentcore.AwakenEvent{{
			Type:      agentcore.RunStartedEventType,
			Timestamp: record.Timestamp,
			ThreadID:  record.ThreadID,
			RunID:     record.RunID,
			TurnID:    record.TurnID,
		}}
	case agentcore.SemanticKindReasoningSummary:
		content := stringPayload(record.Payload, "content")
		if strings.TrimSpace(content) == "" {
			return nil
		}
		messageID := firstNonEmpty(stringPayload(record.Payload, "message_id"), record.RecordID)
		return []agentcore.AwakenEvent{
			{Type: agentcore.ReasoningStartedEventType, Timestamp: record.Timestamp, ThreadID: record.ThreadID, RunID: record.RunID, TurnID: record.TurnID, MessageID: messageID},
			{Type: agentcore.ReasoningDeltaEventType, Timestamp: record.Timestamp, ThreadID: record.ThreadID, RunID: record.RunID, TurnID: record.TurnID, MessageID: messageID, Delta: content},
			{Type: agentcore.ReasoningFinishedEventType, Timestamp: record.Timestamp, ThreadID: record.ThreadID, RunID: record.RunID, TurnID: record.TurnID, MessageID: messageID},
		}
	case agentcore.SemanticKindAssistantMessage:
		content := stringPayload(record.Payload, "content")
		if strings.TrimSpace(content) == "" {
			return nil
		}
		messageID := firstNonEmpty(stringPayload(record.Payload, "message_id"), record.RecordID)
		return []agentcore.AwakenEvent{
			{Type: agentcore.TextMessageStartedEventType, Timestamp: record.Timestamp, ThreadID: record.ThreadID, RunID: record.RunID, TurnID: record.TurnID, MessageID: messageID, Role: "assistant"},
			{Type: agentcore.TextDeltaEventType, Timestamp: record.Timestamp, ThreadID: record.ThreadID, RunID: record.RunID, TurnID: record.TurnID, MessageID: messageID, Role: "assistant", Delta: content},
			{Type: agentcore.TextMessageFinishedEventType, Timestamp: record.Timestamp, ThreadID: record.ThreadID, RunID: record.RunID, TurnID: record.TurnID, MessageID: messageID, Role: "assistant"},
		}
	case agentcore.SemanticKindToolCall:
		toolCallID := stringPayload(record.Payload, "tool_call_id")
		toolName := stringPayload(record.Payload, "tool_name")
		args := stringPayload(record.Payload, "arguments")
		events := []agentcore.AwakenEvent{{
			Type:            agentcore.ToolCallStartedEventType,
			Timestamp:       record.Timestamp,
			ThreadID:        record.ThreadID,
			RunID:           record.RunID,
			TurnID:          record.TurnID,
			ToolCallID:      toolCallID,
			ToolName:        toolName,
			ParentMessageID: record.ParentID,
		}}
		if args != "" {
			events = append(events, agentcore.AwakenEvent{
				Type:       agentcore.ToolCallArgsEventType,
				Timestamp:  record.Timestamp,
				ThreadID:   record.ThreadID,
				RunID:      record.RunID,
				TurnID:     record.TurnID,
				ToolCallID: toolCallID,
				ToolName:   toolName,
				Arguments:  args,
			})
		}
		events = append(events, agentcore.AwakenEvent{
			Type:       agentcore.ToolCallFinishedEventType,
			Timestamp:  record.Timestamp,
			ThreadID:   record.ThreadID,
			RunID:      record.RunID,
			TurnID:     record.TurnID,
			ToolCallID: toolCallID,
			ToolName:   toolName,
		})
		return events
	case agentcore.SemanticKindToolResult:
		event := agentcore.AwakenEvent{
			Type:       agentcore.ToolResultEventType,
			Timestamp:  record.Timestamp,
			ThreadID:   record.ThreadID,
			RunID:      record.RunID,
			TurnID:     record.TurnID,
			ToolCallID: stringPayload(record.Payload, "tool_call_id"),
			ToolName:   stringPayload(record.Payload, "tool_name"),
			Content:    stringPayload(record.Payload, "content"),
			Role:       "tool",
		}
		if outcome, ok := record.Payload["outcome"].(map[string]any); ok {
			event.ToolStatus = stringPayload(outcome, "status")
			event.TransportStatus = stringPayload(outcome, "transport_status")
			event.SemanticStatus = stringPayload(outcome, "semantic_status")
			event.ProcessStatus = stringPayload(outcome, "process_status")
			event.ErrorCode = stringPayload(outcome, "error_code")
			event.ErrorCategory = stringPayload(outcome, "error_category")
			event.Retryable = boolPayload(outcome, "retryable")
		} else if boolPayload(record.Payload, "is_error") {
			event.ToolStatus = "failed"
		}
		return []agentcore.AwakenEvent{event}
	case agentcore.SemanticKindArtifact:
		eventType := agentcore.ArtifactUpdatedEventType
		if boolPayload(record.Payload, "created") {
			eventType = agentcore.ArtifactCreatedEventType
		}
		producer := firstNonEmpty(stringPayload(record.Payload, "producer"), stringPayload(record.Payload, "tool_name"))
		artifactKind := stringPayload(record.Payload, "kind")
		artifactPath := stringPayload(record.Payload, "path")
		return []agentcore.AwakenEvent{{
			Type:                    eventType,
			Timestamp:               record.Timestamp,
			ThreadID:                record.ThreadID,
			RunID:                   record.RunID,
			TurnID:                  record.TurnID,
			ToolCallID:              stringPayload(record.Payload, "tool_call_id"),
			ToolName:                stringPayload(record.Payload, "tool_name"),
			ArtifactContractVersion: tool.ArtifactContractVersionOrLegacy(intPayload(record.Payload, "contract_version")),
			ArtifactRole:            tool.ResolveArtifactRole(tool.ArtifactRole(stringPayload(record.Payload, "role")), producer, artifactKind, artifactPath),
			ArtifactProducer:        producer,
			ArtifactID:              stringPayload(record.Payload, "artifact_id"),
			ArtifactGroupID:         stringPayload(record.Payload, "group_id"),
			ArtifactGroupIndex:      intPayload(record.Payload, "group_index"),
			ArtifactTitle:           stringPayload(record.Payload, "title"),
			ArtifactDescription:     stringPayload(record.Payload, "description"),
			ArtifactKind:            artifactKind,
			ArtifactPath:            artifactPath,
			ArtifactContent:         stringPayload(record.Payload, "content"),
			ArtifactStatus:          agentcore.ArtifactStatus(stringPayload(record.Payload, "status")),
		}}
	case agentcore.SemanticKindTurnDiff:
		return []agentcore.AwakenEvent{{
			Type:      agentcore.TurnDiffEventType,
			Timestamp: record.Timestamp,
			ThreadID:  record.ThreadID,
			RunID:     record.RunID,
			TurnID:    record.TurnID,
			Diff:      stringPayload(record.Payload, "diff"),
			Truncated: boolPayload(record.Payload, "truncated"),
		}}
	case agentcore.SemanticKindTurnBoundary:
		return []agentcore.AwakenEvent{{
			Type:             agentcore.TurnFinishedEventType,
			Timestamp:        record.Timestamp,
			ThreadID:         record.ThreadID,
			RunID:            record.RunID,
			TurnID:           record.TurnID,
			TerminalReason:   agentcore.TerminalReason(stringPayload(record.Payload, "terminal_reason")),
			TransitionReason: agentcore.TransitionReason(stringPayload(record.Payload, "transition_reason")),
		}}
	case agentcore.SemanticKindACPSessionStarted:
		return []agentcore.AwakenEvent{acpReplayEvent(record, agentcore.ACPSessionStartedEventType)}
	case agentcore.SemanticKindACPSessionClosed:
		return []agentcore.AwakenEvent{acpReplayEvent(record, agentcore.ACPSessionClosedEventType)}
	case agentcore.SemanticKindACPSessionInterrupted:
		return []agentcore.AwakenEvent{acpReplayEvent(record, agentcore.ACPSessionInterruptedEventType)}
	case agentcore.SemanticKindACPTurnStarted:
		return []agentcore.AwakenEvent{acpReplayEvent(record, agentcore.ACPTurnStartedEventType)}
	case agentcore.SemanticKindACPTurnFinished:
		return []agentcore.AwakenEvent{acpReplayEvent(record, agentcore.ACPTurnFinishedEventType)}
	case agentcore.SemanticKindACPToolSnapshot:
		return []agentcore.AwakenEvent{acpReplayEvent(record, agentcore.ACPToolSnapshotEventType)}
	case agentcore.SemanticKindACPPermissionRequested:
		return []agentcore.AwakenEvent{acpReplayEvent(record, agentcore.ACPPermissionRequestedEventType)}
	case agentcore.SemanticKindACPPermissionResolved:
		return []agentcore.AwakenEvent{acpReplayEvent(record, agentcore.ACPPermissionResolvedEventType)}
	case agentcore.SemanticKindRunTerminal:
		status := stringPayload(record.Payload, "status")
		if status == "finished" {
			return []agentcore.AwakenEvent{{
				Type:           agentcore.RunFinishedEventType,
				Timestamp:      record.Timestamp,
				ThreadID:       record.ThreadID,
				RunID:          record.RunID,
				TurnID:         record.TurnID,
				Content:        stringPayload(record.Payload, "result"),
				TerminalReason: agentcore.TerminalReason(stringPayload(record.Payload, "terminal_reason")),
			}}
		}
		code := stringPayload(record.Payload, "code")
		if status == "cancelled" && code == "" {
			code = "RUN_CANCELLED"
		}
		if status == "interrupted" && code == "" {
			code = "PROCESS_RESTARTED"
		}
		return []agentcore.AwakenEvent{{
			Type:           agentcore.RunErrorEventType,
			Timestamp:      record.Timestamp,
			ThreadID:       record.ThreadID,
			RunID:          record.RunID,
			TurnID:         record.TurnID,
			Error:          firstNonEmpty(stringPayload(record.Payload, "error"), status),
			Code:           code,
			Content:        stringPayload(record.Payload, "result"),
			TerminalReason: agentcore.TerminalReason(firstNonEmpty(stringPayload(record.Payload, "terminal_reason"), stringPayload(record.Payload, "reason"))),
		}}
	default:
		return nil
	}
}
