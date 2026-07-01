package runtime

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"awaken/src/agentcore"
	convcontext "awaken/src/context"
)

const (
	transcriptRecordTypeEvent   = "event"
	transcriptRecordTypeMessage = "message"
)

type TranscriptRecord struct {
	RecordType        string                   `json:"record_type"`
	SessionID         string                   `json:"session_id,omitempty"`
	ThreadID          string                   `json:"thread_id"`
	RunID             string                   `json:"run_id,omitempty"`
	TurnID            string                   `json:"turn_id,omitempty"`
	MessageID         string                   `json:"message_id,omitempty"`
	ParentMessageID   string                   `json:"parent_message_id,omitempty"`
	Role              string                   `json:"role,omitempty"`
	Content           string                   `json:"content,omitempty"`
	ToolCalls         []convcontext.ToolCall   `json:"tool_calls,omitempty"`
	ToolCallID        string                   `json:"tool_call_id,omitempty"`
	ToolName          string                   `json:"tool_name,omitempty"`
	IsError           bool                     `json:"is_error,omitempty"`
	Event             *agentcore.AwakenEvent   `json:"event,omitempty"`
	TerminalReason    agentcore.TerminalReason `json:"terminal_reason,omitempty"`
	Timestamp         time.Time                `json:"timestamp"`
	CompactBoundaryID string                   `json:"compact_boundary_id,omitempty"`
}

type restoredTranscript struct {
	Messages               []convcontext.Message
	RunIDs                 []string
	Label                  string
	CreatedAt              time.Time
	UpdatedAt              time.Time
	LastContextWindowState *agentcore.ContextWindowState
	TerminalReason         agentcore.TerminalReason
	TerminalEvent          *agentcore.AwakenEvent
}

func appendUserTranscriptRecord(workspace WorkspaceConfig, run Run, sessionID string, input string) error {
	if strings.TrimSpace(input) == "" {
		return nil
	}
	path := transcriptPath(workspace.WorkspaceRoot, run.ThreadID)
	records, err := readTranscriptRecords(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for _, record := range records {
		if record.RecordType == transcriptRecordTypeMessage &&
			record.RunID == run.RunID &&
			record.Role == string(convcontext.RoleUser) &&
			record.Content == input {
			return nil
		}
	}
	index := transcriptMessageCount(records)
	record := transcriptRecordForMessage(run.ThreadID, run.RunID, sessionID, index, convcontext.Message{
		Role:    convcontext.RoleUser,
		Content: input,
	}, time.Now().UTC())
	return appendTranscriptRecords(path, []TranscriptRecord{record})
}

func appendConversationTranscript(workspace WorkspaceConfig, run Run, messages []convcontext.Message) error {
	path := transcriptPath(workspace.WorkspaceRoot, run.ThreadID)
	records, err := readTranscriptRecords(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	seen := map[string]bool{}
	for _, record := range records {
		if record.RecordType == transcriptRecordTypeMessage && record.MessageID != "" {
			seen[record.MessageID] = true
		}
	}

	now := time.Now().UTC()
	pending := make([]TranscriptRecord, 0, len(messages))
	for i, message := range messages {
		id := transcriptMessageID(run.ThreadID, i)
		if seen[id] {
			continue
		}
		pending = append(pending, transcriptRecordForMessage(run.ThreadID, run.RunID, "", i, message, now))
	}
	if len(pending) == 0 {
		return nil
	}
	return appendTranscriptRecords(path, pending)
}

func appendRunEventTranscript(workspace WorkspaceConfig, run Run, events []Event) error {
	path := transcriptPath(workspace.WorkspaceRoot, run.ThreadID)
	records, err := readTranscriptRecords(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	seen := map[string]bool{}
	for _, record := range records {
		if record.RecordType == transcriptRecordTypeEvent && record.MessageID != "" {
			seen[record.MessageID] = true
		}
	}

	pending := make([]TranscriptRecord, 0, len(events))
	parentMessageID := ""
	if messageCount := transcriptMessageCount(records); messageCount > 0 {
		parentMessageID = transcriptMessageID(run.ThreadID, messageCount-1)
	}
	for _, event := range events {
		if event.Kind != EventKindAgent || !isRecoverableTranscriptEvent(event.AgentEvent.Type) {
			continue
		}
		id := transcriptEventID(run.ThreadID, run.RunID, event.Seq)
		if seen[id] {
			continue
		}
		agentEvent := event.AgentEvent
		timestamp := event.Timestamp
		if timestamp.IsZero() {
			timestamp = agentEvent.Timestamp
		}
		if timestamp.IsZero() {
			timestamp = time.Now().UTC()
		}
		reason := agentEvent.TerminalReason
		if reason == "" {
			reason = event.TerminalReason
		}
		pending = append(pending, TranscriptRecord{
			RecordType:        transcriptRecordTypeEvent,
			ThreadID:          run.ThreadID,
			RunID:             run.RunID,
			TurnID:            agentEvent.TurnID,
			MessageID:         id,
			ParentMessageID:   parentMessageID,
			Event:             &agentEvent,
			TerminalReason:    reason,
			Timestamp:         timestamp.UTC(),
			CompactBoundaryID: compactBoundaryIDFromEvent(agentEvent),
		})
	}
	if len(pending) == 0 {
		return nil
	}
	return appendTranscriptRecords(path, pending)
}

func loadRestoredTranscript(workspace WorkspaceConfig, threadID string) (restoredTranscript, error) {
	path := transcriptPath(workspace.WorkspaceRoot, threadID)
	records, err := readTranscriptRecords(path)
	if err != nil {
		return restoredTranscript{}, err
	}

	seenMessages := map[string]bool{}
	seenRuns := map[string]bool{}
	restored := restoredTranscript{}
	messageRecords, err := orderedTranscriptMessageRecords(records)
	if err != nil {
		return restoredTranscript{}, err
	}
	for _, record := range records {
		if record.Timestamp.IsZero() {
			record.Timestamp = time.Now().UTC()
		}
		if restored.CreatedAt.IsZero() || record.Timestamp.Before(restored.CreatedAt) {
			restored.CreatedAt = record.Timestamp
		}
		if record.Timestamp.After(restored.UpdatedAt) {
			restored.UpdatedAt = record.Timestamp
		}
		if record.RunID != "" && !seenRuns[record.RunID] {
			seenRuns[record.RunID] = true
			restored.RunIDs = append(restored.RunIDs, record.RunID)
		}
		if record.RecordType == transcriptRecordTypeEvent && record.Event != nil {
			updateRestoredTranscriptState(&restored, record)
		}
	}
	for _, record := range messageRecords {
		seenMessages[record.MessageID] = true
		message := convcontext.Message{
			Role:              convcontext.Role(record.Role),
			Content:           record.Content,
			ToolCalls:         append([]convcontext.ToolCall(nil), record.ToolCalls...),
			ToolCallID:        record.ToolCallID,
			Name:              record.ToolName,
			IsError:           record.IsError,
			CompactBoundaryID: record.CompactBoundaryID,
		}
		restored.Messages = append(restored.Messages, message)
		if restored.Label == "" && message.Role == convcontext.RoleUser {
			restored.Label = trimThreadLabel(message.Content)
		}
	}
	if restored.Label == "" {
		restored.Label = threadID
	}
	if restored.CreatedAt.IsZero() {
		restored.CreatedAt = time.Now().UTC()
	}
	if restored.UpdatedAt.IsZero() {
		restored.UpdatedAt = restored.CreatedAt
	}
	if err := convcontext.ValidateToolResultPairing(restored.Messages); err != nil {
		return restoredTranscript{}, fmt.Errorf("invalid transcript tool result pairing: %w", err)
	}
	return restored, nil
}

func loadDurableTranscriptRuns(workspace WorkspaceConfig, threadID string) (map[string]ThreadTranscriptRun, error) {
	path := transcriptPath(workspace.WorkspaceRoot, threadID)
	records, err := readTranscriptRecords(path)
	if err != nil {
		return nil, err
	}

	type durableRun struct {
		run    Run
		events []Event
	}
	runs := map[string]*durableRun{}
	order := []string{}
	for _, record := range records {
		if record.RunID == "" {
			continue
		}
		current, ok := runs[record.RunID]
		if !ok {
			current = &durableRun{run: Run{
				RunID:     record.RunID,
				ThreadID:  threadID,
				Status:    RunStatusRunning,
				CreatedAt: record.Timestamp,
				UpdatedAt: record.Timestamp,
			}}
			runs[record.RunID] = current
			order = append(order, record.RunID)
		}
		if current.run.CreatedAt.IsZero() || record.Timestamp.Before(current.run.CreatedAt) {
			current.run.CreatedAt = record.Timestamp
		}
		if record.Timestamp.After(current.run.UpdatedAt) {
			current.run.UpdatedAt = record.Timestamp
		}
		if record.RecordType == transcriptRecordTypeMessage &&
			record.Role == string(convcontext.RoleUser) &&
			current.run.Input == "" {
			current.run.Input = record.Content
		}
		if record.RecordType == transcriptRecordTypeEvent && record.Event != nil {
			event := Event{
				Kind:           EventKindAgent,
				Timestamp:      record.Timestamp,
				ThreadID:       threadID,
				RunID:          record.RunID,
				AgentEvent:     *record.Event,
				TerminalReason: firstTranscriptTerminalReason(record),
			}
			current.events = append(current.events, event)
			applyTerminalTranscriptEvent(&current.run, record)
		}
	}

	result := make(map[string]ThreadTranscriptRun, len(runs))
	for _, runID := range order {
		current := runs[runID]
		for i := range current.events {
			current.events[i].Seq = uint64(i + 1)
		}
		result[runID] = ThreadTranscriptRun{
			Run:    current.run,
			Events: current.events,
		}
	}
	return result, nil
}

func readTranscriptRecords(path string) ([]TranscriptRecord, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var records []TranscriptRecord
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var record TranscriptRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

func appendTranscriptRecords(path string, records []TranscriptRecord) error {
	if len(records) == 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	for _, record := range records {
		if record.Timestamp.IsZero() {
			record.Timestamp = time.Now().UTC()
		}
		if err := encoder.Encode(record); err != nil {
			return err
		}
	}
	return nil
}

func transcriptRecordForMessage(threadID, runID, sessionID string, index int, message convcontext.Message, timestamp time.Time) TranscriptRecord {
	record := TranscriptRecord{
		RecordType:        transcriptRecordTypeMessage,
		SessionID:         strings.TrimSpace(sessionID),
		ThreadID:          threadID,
		RunID:             runID,
		MessageID:         transcriptMessageID(threadID, index),
		Role:              string(message.Role),
		Content:           message.Content,
		ToolCalls:         append([]convcontext.ToolCall(nil), message.ToolCalls...),
		ToolCallID:        message.ToolCallID,
		ToolName:          message.Name,
		IsError:           message.IsError,
		Timestamp:         timestamp,
		CompactBoundaryID: message.CompactBoundaryID,
	}
	if index > 0 {
		record.ParentMessageID = transcriptMessageID(threadID, index-1)
	}
	if len(record.ToolCalls) > 0 && record.ToolName == "" {
		record.ToolName = record.ToolCalls[0].Function.Name
	}
	return record
}

func transcriptMessageCount(records []TranscriptRecord) int {
	count := 0
	for _, record := range records {
		if record.RecordType == transcriptRecordTypeMessage && record.MessageID != "" {
			count++
		}
	}
	return count
}

func transcriptPath(workspaceRoot string, threadID string) string {
	return filepath.Join(filepath.Clean(workspaceRoot), ".awaken", "transcripts", safeTranscriptFileName(threadID)+".jsonl")
}

func transcriptMessageID(threadID string, index int) string {
	return fmt.Sprintf("%s_msg_%06d", safeTranscriptFileName(threadID), index)
}

func transcriptEventID(threadID, runID string, seq uint64) string {
	return fmt.Sprintf("%s_%s_event_%06d", safeTranscriptFileName(threadID), safeTranscriptFileName(runID), seq)
}

func safeTranscriptFileName(value string) string {
	value = strings.TrimSpace(value)
	replacer := strings.NewReplacer("/", "_", "\\", "_", ":", "_", string(filepath.Separator), "_")
	value = replacer.Replace(value)
	value = strings.ReplaceAll(value, "..", "_")
	if value == "" {
		return "thread"
	}
	return value
}

func trimThreadLabel(input string) string {
	label := strings.TrimSpace(input)
	if len(label) > 50 {
		return label[:50]
	}
	return label
}

func orderedTranscriptMessageRecords(records []TranscriptRecord) ([]TranscriptRecord, error) {
	byID := map[string]TranscriptRecord{}
	children := map[string][]TranscriptRecord{}
	roots := []TranscriptRecord{}
	for _, record := range records {
		if record.RecordType != transcriptRecordTypeMessage || record.MessageID == "" {
			continue
		}
		if _, exists := byID[record.MessageID]; exists {
			continue
		}
		byID[record.MessageID] = record
		if record.ParentMessageID == "" {
			roots = append(roots, record)
			continue
		}
		children[record.ParentMessageID] = append(children[record.ParentMessageID], record)
	}
	if len(byID) == 0 {
		return nil, nil
	}
	if len(roots) != 1 {
		return nil, fmt.Errorf("invalid transcript message chain: expected one root, got %d", len(roots))
	}
	for id, record := range byID {
		if record.ParentMessageID != "" {
			if _, ok := byID[record.ParentMessageID]; !ok {
				return nil, fmt.Errorf("invalid transcript message chain: message %s references missing parent %s", id, record.ParentMessageID)
			}
		}
		if len(children[id]) > 1 {
			return nil, fmt.Errorf("invalid transcript message chain: message %s has %d children", id, len(children[id]))
		}
	}

	ordered := make([]TranscriptRecord, 0, len(byID))
	current := roots[0]
	for {
		ordered = append(ordered, current)
		next := children[current.MessageID]
		if len(next) == 0 {
			break
		}
		current = next[0]
	}
	if len(ordered) != len(byID) {
		return nil, fmt.Errorf("invalid transcript message chain: disconnected or cyclic messages")
	}
	return ordered, nil
}

func updateRestoredTranscriptState(restored *restoredTranscript, record TranscriptRecord) {
	if restored == nil || record.Event == nil {
		return
	}
	if isContextTranscriptEvent(record.Event.Type) {
		state := contextWindowStateFromTranscriptEvent(*record.Event)
		restored.LastContextWindowState = &state
	}
	reason := firstTranscriptTerminalReason(record)
	if reason == "" {
		return
	}
	restored.TerminalReason = reason
	event := *record.Event
	restored.TerminalEvent = &event
}

func applyTerminalTranscriptEvent(run *Run, record TranscriptRecord) {
	if run == nil || record.Event == nil {
		return
	}
	event := record.Event
	switch event.Type {
	case agentcore.RunFinishedEventType:
		run.Status = RunStatusFinished
		run.Result = event.Content
		finishedAt := record.Timestamp
		run.FinishedAt = &finishedAt
	case agentcore.RunErrorEventType:
		run.Status = RunStatusError
		run.Error = firstNonEmpty(strings.TrimSpace(event.Error), "run failed")
		finishedAt := record.Timestamp
		run.FinishedAt = &finishedAt
	}
}

func firstTranscriptTerminalReason(record TranscriptRecord) agentcore.TerminalReason {
	if record.TerminalReason != "" {
		return record.TerminalReason
	}
	if record.Event != nil {
		return record.Event.TerminalReason
	}
	return ""
}

func isRecoverableTranscriptEvent(eventType agentcore.AwakenEventType) bool {
	return isContextTranscriptEvent(eventType) ||
		eventType == agentcore.TurnFinishedEventType ||
		eventType == agentcore.RunFinishedEventType ||
		eventType == agentcore.RunErrorEventType
}

func isContextTranscriptEvent(eventType agentcore.AwakenEventType) bool {
	switch eventType {
	case agentcore.ContextEstimatedEventType,
		agentcore.ContextWarningEventType,
		agentcore.ContextBlockedEventType,
		agentcore.CompactStartedEventType,
		agentcore.CompactFinishedEventType,
		agentcore.CompactFailedEventType:
		return true
	default:
		return false
	}
}

func contextWindowStateFromTranscriptEvent(event agentcore.AwakenEvent) agentcore.ContextWindowState {
	return agentcore.ContextWindowState{
		ModelWindow:                 event.ContextModelWindow,
		OutputReserve:               event.ContextOutputReserve,
		EffectiveWindow:             event.ContextEffectiveWindow,
		EstimatedTokens:             event.ContextEstimatedTokens,
		WarningThreshold:            event.ContextWarningThreshold,
		AutoCompactThreshold:        event.ContextAutoCompactThreshold,
		CompactTarget:               event.ContextCompactTarget,
		EstimatedDeltaTokens:        event.ContextEstimatedDeltaTokens,
		LastCompactBoundaryID:       compactBoundaryIDFromEvent(event),
		HasAttemptedReactiveCompact: strings.Contains(event.Content, "reactive"),
	}
}

func compactBoundaryIDFromEvent(event agentcore.AwakenEvent) string {
	const marker = "boundary="
	start := strings.Index(event.Content, marker)
	if start < 0 {
		return ""
	}
	value := event.Content[start+len(marker):]
	if end := strings.IndexAny(value, " \t\r\n"); end >= 0 {
		value = value[:end]
	}
	return strings.TrimSpace(value)
}
