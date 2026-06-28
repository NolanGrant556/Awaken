package runtime

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"awaken/src/agentcore"
	convcontext "awaken/src/context"
)

const persistenceVersion = 1
const timeLayout = time.RFC3339Nano

type fileStore struct {
	root string
	mu   sync.Mutex
}

type persistedThread struct {
	Version               int                   `json:"version"`
	Thread                ThreadSnapshot        `json:"thread"`
	RunIDs                []string              `json:"run_ids"`
	ConversationMessages  []convcontext.Message `json:"conversation_messages"`
	ConversationCreatedAt string                `json:"conversation_created_at,omitempty"`
	ConversationUpdatedAt string                `json:"conversation_updated_at,omitempty"`
}

type persistedRun struct {
	Version int `json:"version"`
	Run     Run `json:"run"`
}

type persistedEvent struct {
	Kind       EventKind              `json:"kind"`
	Seq        uint64                 `json:"seq"`
	Timestamp  string                 `json:"timestamp"`
	ThreadID   string                 `json:"thread_id,omitempty"`
	RunID      string                 `json:"run_id,omitempty"`
	Status     string                 `json:"status,omitempty"`
	AgentEvent *agentcoreEventPayload `json:"agent_event,omitempty"`
	Message    string                 `json:"message,omitempty"`
	Code       string                 `json:"code,omitempty"`
}

type agentcoreEventPayload struct {
	Type                  string   `json:"type,omitempty"`
	Timestamp             string   `json:"timestamp,omitempty"`
	ThreadID              string   `json:"thread_id,omitempty"`
	RunID                 string   `json:"run_id,omitempty"`
	MessageID             string   `json:"message_id,omitempty"`
	ParentMessageID       string   `json:"parent_message_id,omitempty"`
	Role                  string   `json:"role,omitempty"`
	Delta                 string   `json:"delta,omitempty"`
	Content               string   `json:"content,omitempty"`
	Error                 string   `json:"error,omitempty"`
	Code                  string   `json:"code,omitempty"`
	ToolCallID            string   `json:"tool_call_id,omitempty"`
	ToolName              string   `json:"tool_name,omitempty"`
	Arguments             string   `json:"arguments,omitempty"`
	Progress              string   `json:"progress,omitempty"`
	CommandStream         string   `json:"command_stream,omitempty"`
	Patch                 string   `json:"patch,omitempty"`
	PatchSummary          string   `json:"patch_summary,omitempty"`
	ApprovalID            string   `json:"approval_id,omitempty"`
	ApprovalTitle         string   `json:"approval_title,omitempty"`
	ApprovalDescription   string   `json:"approval_description,omitempty"`
	ApprovalRiskTone      string   `json:"approval_risk_tone,omitempty"`
	ApprovalDecision      string   `json:"approval_decision,omitempty"`
	ArtifactID            string   `json:"artifact_id,omitempty"`
	ArtifactTitle         string   `json:"artifact_title,omitempty"`
	ArtifactDescription   string   `json:"artifact_description,omitempty"`
	ArtifactKind          string   `json:"artifact_kind,omitempty"`
	ArtifactPath          string   `json:"artifact_path,omitempty"`
	ArtifactStatus        string   `json:"artifact_status,omitempty"`
	ArtifactContent       string   `json:"artifact_content,omitempty"`
	ApprovalAffectedPaths []string `json:"approval_affected_paths,omitempty"`
	ApprovalPatchSummary  string   `json:"approval_patch_summary,omitempty"`
	Diff                  string   `json:"diff,omitempty"`
	Truncated             bool     `json:"truncated,omitempty"`
}

func newFileStore(root string) *fileStore {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil
	}
	return &fileStore{root: filepath.Clean(root)}
}

func (s *fileStore) loadThreads() ([]persistedThread, error) {
	if s == nil {
		return nil, nil
	}

	paths, err := filepath.Glob(filepath.Join(s.root, "threads", "*.json"))
	if err != nil {
		return nil, err
	}

	sort.Strings(paths)
	threads := make([]persistedThread, 0, len(paths))
	for _, path := range paths {
		var thread persistedThread
		if err := readJSONFile(path, &thread); err != nil {
			return nil, err
		}
		threads = append(threads, thread)
	}
	return threads, nil
}

func (s *fileStore) loadRun(runID string) (persistedRun, error) {
	var run persistedRun
	if s == nil {
		return run, fmt.Errorf("persistence disabled")
	}
	if err := readJSONFile(s.runPath(runID), &run); err != nil {
		return run, err
	}
	return run, nil
}

func (s *fileStore) loadEvents(runID string) ([]Event, error) {
	if s == nil {
		return nil, fmt.Errorf("persistence disabled")
	}

	file, err := os.Open(s.eventsPath(runID))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	events := make([]Event, 0)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, err
		}
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			if errors.Is(err, io.EOF) {
				break
			}
			continue
		}
		var raw persistedEvent
		if unmarshalErr := json.Unmarshal(line, &raw); unmarshalErr != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, unmarshalErr
		}
		events = append(events, raw.toRuntimeEvent())
		if errors.Is(err, io.EOF) {
			break
		}
	}
	return events, nil
}

func (s *fileStore) saveThread(thread persistedThread) error {
	if s == nil {
		return nil
	}
	return writeJSONFile(s.threadPath(thread.Thread.ThreadID), thread)
}

func (s *fileStore) saveRun(run Run) error {
	if s == nil {
		return nil
	}
	return writeJSONFile(s.runPath(run.RunID), persistedRun{
		Version: persistenceVersion,
		Run:     run,
	})
}

func (s *fileStore) appendEvent(event Event) error {
	if s == nil {
		return nil
	}

	if err := s.ensureDirs(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := os.OpenFile(s.eventsPath(event.RunID), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	data, err := json.Marshal(newPersistedEvent(event))
	if err != nil {
		return err
	}
	if _, err := file.Write(append(data, '\n')); err != nil {
		return err
	}
	return nil
}

func (s *fileStore) threadPath(threadID string) string {
	return filepath.Join(s.root, "threads", threadID+".json")
}

func (s *fileStore) runPath(runID string) string {
	return filepath.Join(s.root, "runs", runID+".json")
}

func (s *fileStore) eventsPath(runID string) string {
	return filepath.Join(s.root, "events", runID+".jsonl")
}

func (s *fileStore) ensureDirs() error {
	if s == nil {
		return nil
	}

	for _, dir := range []string{
		filepath.Join(s.root, "threads"),
		filepath.Join(s.root, "runs"),
		filepath.Join(s.root, "events"),
	} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	return nil
}

func writeJSONFile(path string, value interface{}) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, append(data, '\n'), 0644); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func readJSONFile(path string, value interface{}) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, value)
}

func newPersistedThread(thread *Thread) persistedThread {
	record := persistedThread{
		Version: persistenceVersion,
		Thread:  thread.snapshot(),
		RunIDs:  append([]string(nil), thread.runIDs...),
	}
	if thread.conversation != nil {
		record.ConversationMessages = thread.conversation.Context.GetMessages()
		record.ConversationCreatedAt = thread.conversation.CreatedAt.UTC().Format(timeLayout)
		record.ConversationUpdatedAt = thread.conversation.UpdatedAt.UTC().Format(timeLayout)
	}
	return record
}

func restoreConversation(thread persistedThread) *convcontext.Conversation {
	conv := convcontext.NewConversation(nil)
	conv.Context.SetMessages(append([]convcontext.Message(nil), thread.ConversationMessages...))
	if createdAt, ok := parsePersistedTime(thread.ConversationCreatedAt); ok {
		conv.CreatedAt = createdAt
	}
	if updatedAt, ok := parsePersistedTime(thread.ConversationUpdatedAt); ok {
		conv.UpdatedAt = updatedAt
	}
	return conv
}

func newPersistedEvent(event Event) persistedEvent {
	var agentEvent *agentcoreEventPayload
	if event.Kind == EventKindAgent {
		payload := newAgentcoreEventPayload(event.AgentEvent)
		agentEvent = &payload
	}
	return persistedEvent{
		Kind:       event.Kind,
		Seq:        event.Seq,
		Timestamp:  event.Timestamp.UTC().Format(timeLayout),
		ThreadID:   event.ThreadID,
		RunID:      event.RunID,
		Status:     event.Status,
		AgentEvent: agentEvent,
		Message:    event.Message,
		Code:       event.Code,
	}
}

func (e persistedEvent) toRuntimeEvent() Event {
	var agentEvent agentcore.AwakenEvent
	if e.AgentEvent != nil {
		agentEvent = e.AgentEvent.toAwakenEvent()
	}
	return Event{
		Kind:       e.Kind,
		Seq:        e.Seq,
		Timestamp:  mustParsePersistedTime(e.Timestamp),
		ThreadID:   e.ThreadID,
		RunID:      e.RunID,
		Status:     e.Status,
		AgentEvent: agentEvent,
		Message:    e.Message,
		Code:       e.Code,
	}
}

func newAgentcoreEventPayload(event agentcore.AwakenEvent) agentcoreEventPayload {
	return agentcoreEventPayload{
		Type:                  string(event.Type),
		Timestamp:             event.Timestamp.UTC().Format(timeLayout),
		ThreadID:              event.ThreadID,
		RunID:                 event.RunID,
		MessageID:             event.MessageID,
		ParentMessageID:       event.ParentMessageID,
		Role:                  event.Role,
		Delta:                 event.Delta,
		Content:               event.Content,
		Error:                 event.Error,
		Code:                  event.Code,
		ToolCallID:            event.ToolCallID,
		ToolName:              event.ToolName,
		Arguments:             event.Arguments,
		Progress:              event.Progress,
		CommandStream:         event.CommandStream,
		Patch:                 event.Patch,
		PatchSummary:          event.PatchSummary,
		ApprovalID:            event.ApprovalID,
		ApprovalTitle:         event.ApprovalTitle,
		ApprovalDescription:   event.ApprovalDescription,
		ApprovalRiskTone:      string(event.ApprovalRiskTone),
		ApprovalDecision:      string(event.ApprovalDecision),
		ArtifactID:            event.ArtifactID,
		ArtifactTitle:         event.ArtifactTitle,
		ArtifactDescription:   event.ArtifactDescription,
		ArtifactKind:          event.ArtifactKind,
		ArtifactPath:          event.ArtifactPath,
		ArtifactStatus:        string(event.ArtifactStatus),
		ArtifactContent:       event.ArtifactContent,
		ApprovalAffectedPaths: append([]string(nil), event.ApprovalAffectedPaths...),
		ApprovalPatchSummary:  event.ApprovalPatchSummary,
		Diff:                  event.Diff,
		Truncated:             event.Truncated,
	}
}

func (p agentcoreEventPayload) toAwakenEvent() agentcore.AwakenEvent {
	return agentcore.AwakenEvent{
		Type:                  agentcore.AwakenEventType(p.Type),
		Timestamp:             mustParsePersistedTime(p.Timestamp),
		ThreadID:              p.ThreadID,
		RunID:                 p.RunID,
		MessageID:             p.MessageID,
		ParentMessageID:       p.ParentMessageID,
		Role:                  p.Role,
		Delta:                 p.Delta,
		Content:               p.Content,
		Error:                 p.Error,
		Code:                  p.Code,
		ToolCallID:            p.ToolCallID,
		ToolName:              p.ToolName,
		Arguments:             p.Arguments,
		Progress:              p.Progress,
		CommandStream:         p.CommandStream,
		Patch:                 p.Patch,
		PatchSummary:          p.PatchSummary,
		ApprovalID:            p.ApprovalID,
		ApprovalTitle:         p.ApprovalTitle,
		ApprovalDescription:   p.ApprovalDescription,
		ApprovalRiskTone:      agentcore.ApprovalRiskTone(p.ApprovalRiskTone),
		ApprovalDecision:      agentcore.ApprovalDecision(p.ApprovalDecision),
		ArtifactID:            p.ArtifactID,
		ArtifactTitle:         p.ArtifactTitle,
		ArtifactDescription:   p.ArtifactDescription,
		ArtifactKind:          p.ArtifactKind,
		ArtifactPath:          p.ArtifactPath,
		ArtifactStatus:        agentcore.ArtifactStatus(p.ArtifactStatus),
		ArtifactContent:       p.ArtifactContent,
		ApprovalAffectedPaths: append([]string(nil), p.ApprovalAffectedPaths...),
		ApprovalPatchSummary:  p.ApprovalPatchSummary,
		Diff:                  p.Diff,
		Truncated:             p.Truncated,
	}
}

type persistenceObserver struct {
	store *fileStore
}

func (o persistenceObserver) OnRunUpdated(run Run) {
	if o.store == nil {
		return
	}
	if err := o.store.saveRun(run); err != nil {
		log.Printf("runtime persistence: failed to save run %s: %v", run.RunID, err)
	}
}

func (o persistenceObserver) OnEvent(event Event) {
	if o.store == nil {
		return
	}
	if err := o.store.appendEvent(event); err != nil {
		log.Printf("runtime persistence: failed to append event for run %s: %v", event.RunID, err)
	}
}

func parsePersistedTime(value string) (time.Time, bool) {
	if strings.TrimSpace(value) == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(timeLayout, value)
	if err != nil {
		return time.Time{}, false
	}
	return parsed.UTC(), true
}

func mustParsePersistedTime(value string) time.Time {
	if parsed, ok := parsePersistedTime(value); ok {
		return parsed
	}
	return time.Time{}
}
