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
	"awaken/src/desktopdata"
)

const (
	transcriptRecordTypeThread  = "thread"
	transcriptRecordTypeEvent   = "event"
	transcriptRecordTypeMessage = "message"
)

type TranscriptRecord struct {
	RecordType        string                        `json:"record_type"`
	SessionID         string                        `json:"session_id,omitempty"`
	ThreadID          string                        `json:"thread_id"`
	RunID             string                        `json:"run_id,omitempty"`
	TurnID            string                        `json:"turn_id,omitempty"`
	MessageID         string                        `json:"message_id,omitempty"`
	ParentMessageID   string                        `json:"parent_message_id,omitempty"`
	Role              string                        `json:"role,omitempty"`
	Content           string                        `json:"content,omitempty"`
	Attachments       []transcriptMessageAttachment `json:"attachments,omitempty"`
	ToolCalls         []convcontext.ToolCall        `json:"tool_calls,omitempty"`
	ToolCallID        string                        `json:"tool_call_id,omitempty"`
	ToolName          string                        `json:"tool_name,omitempty"`
	IsError           bool                          `json:"is_error,omitempty"`
	Event             *agentcore.AwakenEvent        `json:"event,omitempty"`
	TerminalReason    agentcore.TerminalReason      `json:"terminal_reason,omitempty"`
	Timestamp         time.Time                     `json:"timestamp"`
	CompactBoundaryID string                        `json:"compact_boundary_id,omitempty"`
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

func appendUserTranscriptRecordAtPath(path string, run Run, sessionID string, input string, attachments []RunInputAttachment) error {
	if strings.TrimSpace(input) == "" && len(attachments) == 0 {
		return nil
	}
	records, err := readTranscriptRecords(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for _, record := range records {
		if record.RecordType == transcriptRecordTypeMessage &&
			record.RunID == run.RunID &&
			record.Role == string(convcontext.RoleUser) &&
			record.Content == input &&
			transcriptAttachmentsEqual(record.Attachments, contextAttachmentsToTranscriptAttachments(inputAttachmentsToContextAttachments(attachments))) {
			return nil
		}
	}
	index := transcriptMessageCount(records)
	record := transcriptRecordForMessage(run.ThreadID, run.RunID, sessionID, index, convcontext.Message{
		Role:        convcontext.RoleUser,
		Content:     input,
		Attachments: inputAttachmentsToContextAttachments(attachments),
	}, time.Now().UTC())
	return appendTranscriptRecords(path, []TranscriptRecord{record})
}

func appendConversationTranscriptAtPath(path string, run Run, messages []convcontext.Message) error {
	records, err := readTranscriptRecords(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	messageRecords, err := orderedTranscriptMessageRecords(records)
	if err != nil {
		return err
	}
	fullView := transcriptMessagesFromRecords(messageRecords)
	if transcriptMessagesEqual(fullView, messages) {
		return nil
	}
	if transcriptMessagesHavePrefix(messages, fullView) {
		return appendTranscriptMessageSuffix(path, run, records, messages, len(fullView))
	}

	activeView := transcriptActiveViewMessages(messageRecords)
	if transcriptMessagesEqual(activeView, messages) {
		return nil
	}
	if transcriptMessagesHavePrefix(messages, activeView) {
		return appendTranscriptMessageSuffix(path, run, records, messages, len(activeView))
	}
	return appendTranscriptMessageSuffix(path, run, records, messages, 0)
}

func appendRunEventTranscriptAtPath(path string, run Run, events []Event) error {
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

func appendThreadLabelTranscriptRecordAtPath(path, threadID, label string, timestamp time.Time) error {
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}
	return appendTranscriptRecords(path, []TranscriptRecord{{
		RecordType: transcriptRecordTypeThread,
		ThreadID:   threadID,
		Content:    trimThreadLabel(strings.TrimSpace(label)),
		Timestamp:  timestamp.UTC(),
	}})
}

func loadRestoredTranscriptAtPath(path string) (restoredTranscript, error) {
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
		if record.RecordType == transcriptRecordTypeThread && strings.TrimSpace(record.Content) != "" {
			restored.Label = trimThreadLabel(record.Content)
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
			Attachments:       transcriptAttachmentsToContextAttachments(record.Attachments),
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
		restored.Label = strings.TrimPrefix(strings.TrimSuffix(filepath.Base(path), ".jsonl"), "thread_")
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

func loadDurableTranscriptRunsAtPath(path string, threadID string) (map[string]ThreadTranscriptRun, error) {
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
			current.run.InputAttachments = contextAttachmentsToRunInputAttachments(
				transcriptAttachmentsToContextAttachments(record.Attachments),
			)
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

func deleteThreadTranscriptAtPath(path string) error {
	if err := os.Remove(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrThreadNotFound
		}
		return err
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
		Attachments:       contextAttachmentsToTranscriptAttachments(message.Attachments),
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

func transcriptPath(threadID string) string {
	return desktopdata.Detect().ThreadTranscriptPath(threadID)
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
	if hasContextWindowPayload(*record.Event) {
		state := mergeContextWindowState(restored.LastContextWindowState, *record.Event)
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

func hasContextWindowPayload(event agentcore.AwakenEvent) bool {
	if isContextTranscriptEvent(event.Type) {
		return true
	}
	return event.ContextModelWindow > 0 ||
		event.ContextOutputReserve > 0 ||
		event.ContextEffectiveWindow > 0 ||
		event.ContextEstimatedTokens > 0 ||
		event.ContextActualTokens > 0 ||
		event.ContextWarningThreshold > 0 ||
		event.ContextAutoCompactThreshold > 0 ||
		event.ContextCompactTarget > 0 ||
		event.ContextEstimatedDeltaTokens != 0 ||
		event.CompactPreEstimate != nil ||
		event.CompactPostEstimate != nil ||
		strings.TrimSpace(event.CompactBoundaryID) != ""
}

func contextWindowStateFromTranscriptEvent(event agentcore.AwakenEvent) agentcore.ContextWindowState {
	return agentcore.ContextWindowState{
		ModelWindow:                 event.ContextModelWindow,
		OutputReserve:               event.ContextOutputReserve,
		EffectiveWindow:             event.ContextEffectiveWindow,
		EstimatedTokens:             compactEstimatedTokensFromEvent(event),
		LastAPIUsageTokens:          event.ContextActualTokens,
		WarningThreshold:            event.ContextWarningThreshold,
		AutoCompactThreshold:        event.ContextAutoCompactThreshold,
		CompactTarget:               event.ContextCompactTarget,
		EstimatedDeltaTokens:        event.ContextEstimatedDeltaTokens,
		LastCompactBoundaryID:       compactBoundaryIDFromEvent(event),
		HasAttemptedReactiveCompact: compactModeFromEvent(event) == agentcore.CompactModeReactive,
	}
}

func mergeContextWindowState(previous *agentcore.ContextWindowState, event agentcore.AwakenEvent) agentcore.ContextWindowState {
	next := contextWindowStateFromTranscriptEvent(event)
	if previous == nil {
		return next
	}
	merged := *previous
	if next.ModelWindow > 0 {
		merged.ModelWindow = next.ModelWindow
	}
	if next.OutputReserve > 0 {
		merged.OutputReserve = next.OutputReserve
	}
	if next.EffectiveWindow > 0 {
		merged.EffectiveWindow = next.EffectiveWindow
	}
	if next.EstimatedTokens > 0 {
		merged.EstimatedTokens = next.EstimatedTokens
	}
	if next.LastAPIUsageTokens > 0 {
		merged.LastAPIUsageTokens = next.LastAPIUsageTokens
	}
	if next.WarningThreshold > 0 {
		merged.WarningThreshold = next.WarningThreshold
	}
	if next.AutoCompactThreshold > 0 {
		merged.AutoCompactThreshold = next.AutoCompactThreshold
	}
	if next.CompactTarget > 0 {
		merged.CompactTarget = next.CompactTarget
	}
	if event.ContextEstimatedDeltaTokens != 0 {
		merged.EstimatedDeltaTokens = next.EstimatedDeltaTokens
	}
	if next.LastCompactBoundaryID != "" {
		merged.LastCompactBoundaryID = next.LastCompactBoundaryID
	}
	if next.HasAttemptedReactiveCompact {
		merged.HasAttemptedReactiveCompact = true
	}
	return merged
}

func compactBoundaryIDFromEvent(event agentcore.AwakenEvent) string {
	if value := strings.TrimSpace(event.CompactBoundaryID); value != "" {
		return value
	}
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

func compactModeFromEvent(event agentcore.AwakenEvent) agentcore.CompactMode {
	if event.CompactMode != "" {
		return event.CompactMode
	}
	if strings.Contains(strings.ToLower(event.Content), "reactive") {
		return agentcore.CompactModeReactive
	}
	return ""
}

func compactEstimatedTokensFromEvent(event agentcore.AwakenEvent) int {
	if event.Type == agentcore.CompactFinishedEventType {
		if event.CompactPostEstimate != nil {
			return *event.CompactPostEstimate
		}
		if event.ContextEstimatedTokens > 0 {
			return event.ContextEstimatedTokens
		}
		if event.CompactPreEstimate != nil {
			return *event.CompactPreEstimate
		}
		return 0
	}
	if event.CompactPreEstimate != nil {
		return *event.CompactPreEstimate
	}
	if event.ContextEstimatedTokens > 0 {
		return event.ContextEstimatedTokens
	}
	if event.CompactPostEstimate != nil {
		return *event.CompactPostEstimate
	}
	return 0
}

func transcriptActiveViewMessages(records []TranscriptRecord) []convcontext.Message {
	if len(records) == 0 {
		return nil
	}
	start := 0
	for i, record := range records {
		if strings.TrimSpace(record.CompactBoundaryID) != "" {
			start = i
		}
	}
	return transcriptMessagesFromRecords(records[start:])
}

func transcriptMessagesFromRecords(records []TranscriptRecord) []convcontext.Message {
	messages := make([]convcontext.Message, 0, len(records))
	for _, record := range records {
		messages = append(messages, convcontext.Message{
			Role:              convcontext.Role(record.Role),
			Content:           record.Content,
			Attachments:       transcriptAttachmentsToContextAttachments(record.Attachments),
			ToolCalls:         append([]convcontext.ToolCall(nil), record.ToolCalls...),
			ToolCallID:        record.ToolCallID,
			Name:              record.ToolName,
			IsError:           record.IsError,
			CompactBoundaryID: record.CompactBoundaryID,
		})
	}
	return messages
}

func transcriptMessagesHavePrefix(messages []convcontext.Message, prefix []convcontext.Message) bool {
	if len(prefix) > len(messages) {
		return false
	}
	for i := range prefix {
		if !transcriptMessageEqual(messages[i], prefix[i]) {
			return false
		}
	}
	return true
}

func transcriptMessagesEqual(left []convcontext.Message, right []convcontext.Message) bool {
	if len(left) != len(right) {
		return false
	}
	return transcriptMessagesHavePrefix(left, right)
}

func transcriptMessageEqual(left convcontext.Message, right convcontext.Message) bool {
	if left.Role != right.Role ||
		left.Content != right.Content ||
		left.ToolCallID != right.ToolCallID ||
		transcriptMessageToolName(left) != transcriptMessageToolName(right) ||
		left.IsError != right.IsError ||
		left.CompactBoundaryID != right.CompactBoundaryID ||
		len(left.Attachments) != len(right.Attachments) ||
		len(left.ToolCalls) != len(right.ToolCalls) {
		return false
	}
	for i := range left.Attachments {
		if left.Attachments[i] != right.Attachments[i] {
			return false
		}
	}
	for i := range left.ToolCalls {
		if left.ToolCalls[i] != right.ToolCalls[i] {
			return false
		}
	}
	return true
}

func transcriptAttachmentsEqual(left []transcriptMessageAttachment, right []transcriptMessageAttachment) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func transcriptMessageToolName(message convcontext.Message) string {
	if strings.TrimSpace(message.Name) != "" {
		return message.Name
	}
	if len(message.ToolCalls) > 0 {
		return message.ToolCalls[0].Function.Name
	}
	return ""
}

func appendTranscriptMessageSuffix(path string, run Run, records []TranscriptRecord, messages []convcontext.Message, start int) error {
	if start >= len(messages) {
		return nil
	}
	baseIndex := transcriptMessageCount(records)
	now := time.Now().UTC()
	pending := make([]TranscriptRecord, 0, len(messages)-start)
	for i := start; i < len(messages); i++ {
		pending = append(pending, transcriptRecordForMessage(run.ThreadID, run.RunID, "", baseIndex+len(pending), messages[i], now))
	}
	return appendTranscriptRecords(path, pending)
}
