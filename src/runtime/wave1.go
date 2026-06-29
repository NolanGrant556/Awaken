package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"awaken/src/agentcore"
	convcontext "awaken/src/context"
	"awaken/src/contextfiles"
	"awaken/src/contextviz"
	"awaken/src/modeldisplay"
	"awaken/src/sessionbranch"
	"awaken/src/sessioninsights"
	"awaken/src/sessionlite"
	"awaken/src/sessionstate"
	"awaken/src/sessiontags"
	"awaken/src/titlegen"
)

type ThreadMetadata struct {
	SessionID            string                `json:"session_id,omitempty"`
	CustomTitle          string                `json:"custom_title,omitempty"`
	AITitle              string                `json:"ai_title,omitempty"`
	DisplayTitle         string                `json:"display_title,omitempty"`
	Tag                  string                `json:"tag,omitempty"`
	LastPrompt           string                `json:"last_prompt,omitempty"`
	TaskSummary          string                `json:"task_summary,omitempty"`
	TaskSummaryTimestamp string                `json:"task_summary_timestamp,omitempty"`
	Mode                 sessiontags.Mode      `json:"mode,omitempty"`
	Branch               *ThreadBranchMetadata `json:"branch,omitempty"`
}

type ThreadBranchMetadata struct {
	OriginalThreadID      string `json:"original_thread_id,omitempty"`
	OriginalSessionID     string `json:"original_session_id,omitempty"`
	ForkedFromMessageUUID string `json:"forked_from_message_uuid,omitempty"`
	Title                 string `json:"title,omitempty"`
}

type ThreadLiteSnapshot struct {
	ThreadID string             `json:"thread_id"`
	Label    string             `json:"label"`
	State    sessionstate.State `json:"state"`
	Head     string             `json:"head"`
	Tail     string             `json:"tail"`
	Size     int                `json:"size"`
}

type ThreadContextSnapshot struct {
	ThreadID string             `json:"thread_id"`
	Files    ThreadContextFiles `json:"files"`
	Usage    ThreadContextUsage `json:"usage"`
}

type ThreadContextFiles struct {
	Markdown string              `json:"markdown"`
	Files    []ThreadContextFile `json:"files"`
}

type ThreadContextFile struct {
	Path         string    `json:"path"`
	Source       string    `json:"source,omitempty"`
	Tokens       int       `json:"tokens,omitempty"`
	LastReadTime time.Time `json:"last_read_time,omitempty"`
}

type ThreadContextUsage struct {
	Markdown         string `json:"markdown"`
	Model            string `json:"model"`
	ModelDisplayName string `json:"model_display_name,omitempty"`
	TotalTokens      int    `json:"total_tokens"`
	MaxTokens        int    `json:"max_tokens"`
	Percentage       int    `json:"percentage"`
	MessageCount     int    `json:"message_count"`
}

type BranchThreadRequest struct {
	ForkSessionID string `json:"fork_session_id,omitempty"`
	CustomTitle   string `json:"custom_title,omitempty"`
}

type BranchThreadResult struct {
	Thread ThreadSnapshot     `json:"thread"`
	Fork   sessionbranch.Fork `json:"fork"`
}

type promptTitleGenerator struct{}

func normalizeTitleGenerator(generator titlegen.Generator) titlegen.Generator {
	if generator != nil {
		return generator
	}
	return promptTitleGenerator{}
}

func (promptTitleGenerator) GenerateTitle(ctx context.Context, prompt titlegen.Prompt) (string, error) {
	user := prompt.UserPrompt
	const prefix = "User:"
	if idx := strings.Index(user, prefix); idx >= 0 {
		user = user[idx+len(prefix):]
	}
	if idx := strings.Index(user, "\n\nAssistant:"); idx >= 0 {
		user = user[:idx]
	}
	title := titlegen.CleanTitle(user)
	if title == "" {
		return "", errors.New("empty title prompt")
	}
	return title, nil
}

func (s *RunService) snapshotThreadLocked(thread *Thread) ThreadSnapshot {
	latest := s.latestRunBriefLocked(thread)
	pending := s.pendingActionLocked(thread)
	snapshot := thread.snapshot(latest)
	snapshot.PendingAction = clonePendingAction(pending)
	if pending != nil {
		snapshot.State = sessionstate.StateRequiresAction
	}
	return snapshot
}

func (t *Thread) metadataSnapshot() ThreadMetadata {
	metadata := t.metadata
	if metadata.SessionID == "" {
		metadata.SessionID = t.threadID
	}
	displayTitle := metadata.DisplayTitle()
	if displayTitle == "" {
		displayTitle = t.label
	}
	return ThreadMetadata{
		SessionID:            metadata.SessionID,
		CustomTitle:          metadata.CustomTitle,
		AITitle:              metadata.AITitle,
		DisplayTitle:         displayTitle,
		Tag:                  metadata.Tag,
		LastPrompt:           metadata.LastPrompt,
		TaskSummary:          metadata.TaskSummary,
		TaskSummaryTimestamp: metadata.TaskSummaryTimestamp,
		Mode:                 metadata.Mode,
		Branch:               cloneThreadBranchMetadata(t.branch),
	}
}

func metadataFromSnapshot(snapshot ThreadSnapshot) sessiontags.Metadata {
	metadata := sessiontags.Metadata{
		SessionID:            firstNonEmpty(snapshot.Metadata.SessionID, snapshot.ThreadID),
		CustomTitle:          snapshot.Metadata.CustomTitle,
		AITitle:              snapshot.Metadata.AITitle,
		Tag:                  snapshot.Metadata.Tag,
		LastPrompt:           snapshot.Metadata.LastPrompt,
		TaskSummary:          snapshot.Metadata.TaskSummary,
		TaskSummaryTimestamp: snapshot.Metadata.TaskSummaryTimestamp,
		Mode:                 snapshot.Metadata.Mode,
	}
	if metadata.Mode == "" {
		metadata.Mode = sessiontags.ModeNormal
	}
	return metadata
}

func applyThreadPromptMetadata(metadata sessiontags.Metadata, threadID, prompt string, timestamp time.Time) sessiontags.Metadata {
	if metadata.SessionID == "" {
		metadata.SessionID = threadID
	}
	if metadata.Mode == "" {
		metadata.Mode = sessiontags.ModeNormal
	}
	return sessiontags.Apply(metadata, sessiontags.LastPromptEntry{
		SessionID:  threadID,
		LastPrompt: strings.TrimSpace(prompt),
	})
}

// titleGenInputs is the lock-free snapshot needed to run title generation
// outside the RunService mutex. The LLM call inside titlegen.MaybeGenerate can
// take seconds to tens of seconds; holding s.mu during that call blocks
// thread listing, context reads, branching, and the next StartRun.
type titleGenInputs struct {
	threadID      string
	sessionID     string
	messages      []convcontext.Message
	existingTitle string // non-empty only when MaybeGenerate should respect skip-existing-title
}

// generateThreadTitleAsync runs the (potentially slow) LLM title generator
// without holding s.mu, then takes a short lock to write the result back.
// It is safe to run as a goroutine: it re-validates state under the lock and
// only writes when the title is still missing or still weak.
func (s *RunService) generateThreadTitleAsync(inputs titleGenInputs) {
	result := titlegen.MaybeGenerate(context.Background(), titlegen.MaybeGenerateRequest{
		SessionID:     inputs.sessionID,
		ExistingTitle: inputs.existingTitle,
		Messages:      inputs.messages,
		Generator:     s.titleGenerator,
	})
	if !result.Generated {
		return
	}

	s.mu.Lock()
	thread, ok := s.threads[inputs.threadID]
	if !ok {
		s.mu.Unlock()
		return
	}
	// Don't clobber a user-set custom title, or an AI title that has already
	// been upgraded to something substantive. Allow overwrite only when the
	// current AI title is still weak (or empty) and no custom title exists.
	if thread.metadata.CustomTitle != "" {
		s.mu.Unlock()
		return
	}
	current := thread.metadata.AITitle
	if current != "" && !isWeakAITitle(current) {
		s.mu.Unlock()
		return
	}
	thread.metadata = sessiontags.Apply(thread.metadata, sessiontags.AITitleEntry{
		SessionID: inputs.sessionID,
		AITitle:   result.Title,
	})
	thread.updatedAt = time.Now().UTC()
	s.mu.Unlock()

	s.persistThread(inputs.threadID)
}

// isWeakAITitle reports whether an existing AI-generated title is too thin to
// represent the thread's real topic (e.g. it came from a greeting first
// exchange). Weak titles may be overwritten by a later, more substantive run.
func isWeakAITitle(title string) bool {
	title = strings.TrimSpace(title)
	if title == "" {
		return true
	}
	if utf8.RuneCountInString(title) < 4 {
		return true
	}
	lower := strings.ToLower(title)
	for _, greeting := range []string{"hello", "hi", "hey", "greeting", "你好", "嗨", "测试", "test", "untitled", "新会话", "new chat", "new task"} {
		if lower == greeting {
			return true
		}
	}
	return false
}

func (s *RunService) updateThreadAfterRunLocked(thread *Thread, run Run) *titleGenInputs {
	if thread == nil || strings.TrimSpace(run.RunID) == "" {
		return nil
	}
	metadata := thread.metadata
	if metadata.SessionID == "" {
		metadata.SessionID = thread.threadID
	}
	if metadata.Mode == "" {
		metadata.Mode = sessiontags.ModeNormal
	}

	summary := run.Result
	if run.Status == RunStatusError || run.Status == RunStatusCancelled {
		summary = firstNonEmpty(run.Error, string(run.Status))
	}
	summary = truncateRunes(strings.Join(strings.Fields(summary), " "), 240)
	if summary != "" {
		metadata = sessiontags.Apply(metadata, sessiontags.TaskSummaryEntry{
			SessionID: metadata.SessionID,
			Summary:   summary,
			Timestamp: run.UpdatedAt.UTC().Format(time.RFC3339Nano),
		})
	}

	thread.metadata = metadata

	// Capture titlegen inputs while we hold the lock (cheap: slice reads +
	// a skip-rule check). The actual LLM call runs async after unlock.
	if metadata.CustomTitle != "" {
		return nil
	}
	// Allow (re)titling when there is no AI title yet, or the existing one is
	// weak. For a fresh title prefer the conversation's first exchange; when
	// upgrading a weak title use the latest run's exchange so the title tracks
	// the current topic.
	inputs := titleGenInputs{
		threadID:      thread.threadID,
		sessionID:     metadata.SessionID,
		existingTitle: metadata.AITitle,
	}
	if metadata.AITitle == "" {
		messages := threadMessages(thread)
		if !titlegen.ShouldAutoTitle(titlegen.AutoTitleRequest{
			SessionID:     metadata.SessionID,
			ExistingTitle: metadata.AITitle,
			Messages:      messages,
		}) {
			messages = syntheticTitleMessages(run)
		}
		inputs.messages = append([]convcontext.Message(nil), messages...)
	} else if isWeakAITitle(metadata.AITitle) {
		// Upgrade path: re-title from the latest run exchange.
		// Leave inputs.existingTitle empty so MaybeGenerate does not skip before
		// the later re-lock validation decides whether it is still safe to write.
		inputs.existingTitle = ""
		messages := syntheticTitleMessages(run)
		if len(messages) == 0 {
			return nil
		}
		inputs.messages = append([]convcontext.Message(nil), messages...)
	} else {
		return nil
	}
	if len(inputs.messages) == 0 {
		return nil
	}
	return &inputs
}

func threadMessages(thread *Thread) []convcontext.Message {
	if thread == nil || thread.conversation == nil || thread.conversation.Context == nil {
		return nil
	}
	return thread.conversation.Context.GetMessages()
}

func syntheticTitleMessages(run Run) []convcontext.Message {
	input := strings.TrimSpace(run.Input)
	response := strings.TrimSpace(firstNonEmpty(run.Result, run.Error))
	if input == "" || response == "" {
		return nil
	}
	return []convcontext.Message{
		{Role: convcontext.RoleUser, Content: input},
		{Role: convcontext.RoleAssistant, Content: response},
	}
}

func (s *RunService) pendingActionLocked(thread *Thread) *sessionstate.RequiresActionDetails {
	if thread == nil || thread.latestRunID == "" {
		return nil
	}
	session := s.sessions[thread.latestRunID]
	if session == nil {
		return nil
	}
	return session.PendingActionDetails()
}

func (s *Session) PendingActionDetails() *sessionstate.RequiresActionDetails {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.run.Status != RunStatusAwaitingApproval {
		return nil
	}
	for _, pending := range s.approvals {
		request := pending.request
		return &sessionstate.RequiresActionDetails{
			ToolName:          request.ToolName,
			ActionDescription: request.Description,
			ToolUseID:         request.ToolCallID,
			RequestID:         request.ID,
			Input: map[string]any{
				"title":          request.Title,
				"risk_tone":      string(request.RiskTone),
				"affected_paths": append([]string(nil), request.AffectedPaths...),
				"patch_summary":  request.PatchSummary,
			},
		}
	}
	return nil
}

func threadState(latest *ThreadRunBrief) sessionstate.State {
	reporter := sessionstate.NewReporter()
	state := sessionstate.StateIdle
	if latest != nil {
		switch latest.Status {
		case RunStatusAwaitingApproval:
			state = sessionstate.StateRequiresAction
		case RunStatusQueued, RunStatusRunning, RunStatusCancelling:
			state = sessionstate.StateRunning
		default:
			state = sessionstate.StateIdle
		}
	}
	reporter.Transition(state, nil)
	return reporter.State()
}

func clonePendingAction(details *sessionstate.RequiresActionDetails) *sessionstate.RequiresActionDetails {
	if details == nil {
		return nil
	}
	clone := *details
	if details.Input != nil {
		clone.Input = make(map[string]any, len(details.Input))
		for key, value := range details.Input {
			clone.Input[key] = value
		}
	}
	return &clone
}

func (s *RunService) GetThreadLite(threadID string) (ThreadLiteSnapshot, error) {
	s.mu.RLock()
	thread, ok := s.threads[threadID]
	if !ok {
		s.mu.RUnlock()
		return ThreadLiteSnapshot{}, ErrThreadNotFound
	}
	snapshot := s.snapshotThreadLocked(thread)
	s.mu.RUnlock()

	data, err := json.Marshal(snapshot)
	if err != nil {
		return ThreadLiteSnapshot{}, err
	}
	headTail := sessionlite.ReadHeadTail(data, 4096)
	return ThreadLiteSnapshot{
		ThreadID: snapshot.ThreadID,
		Label:    snapshot.Label,
		State:    snapshot.State,
		Head:     headTail.Head,
		Tail:     headTail.Tail,
		Size:     headTail.Size,
	}, nil
}

func (s *RunService) GetThreadContext(threadID string) (ThreadContextSnapshot, error) {
	s.mu.RLock()
	thread, ok := s.threads[threadID]
	if !ok {
		s.mu.RUnlock()
		return ThreadContextSnapshot{}, ErrThreadNotFound
	}
	workspace := thread.workspace
	model := "unknown"
	maxTokens := 0
	if thread.agent != nil {
		config := thread.agent.GetConfig()
		if config.ModelConfig.Model != "" {
			model = config.ModelConfig.Model
		}
		maxTokens = config.ModelConfig.ContextWindow
	}
	messages := append([]convcontext.Message(nil), threadMessages(thread)...)
	runIDs := append([]string(nil), thread.runIDs...)
	sessions := make([]*Session, 0, len(runIDs))
	for _, runID := range runIDs {
		if session := s.sessions[runID]; session != nil {
			sessions = append(sessions, session)
		}
	}
	s.mu.RUnlock()

	files := contextFilesFromSessions(sessions)
	filesMarkdown := contextfiles.Format(files, workspace.WorkspaceRoot, contextfiles.Options{Verbose: true})
	totalTokens := estimateConversationTokens(messages)
	if maxTokens <= 0 {
		maxTokens = totalTokens
		if maxTokens <= 0 {
			maxTokens = 1
		}
	}
	usageData := contextviz.Data{
		Model:        model,
		TotalTokens:  totalTokens,
		RawMaxTokens: maxTokens,
		Categories: []contextviz.Category{
			{Name: "Conversation messages", Tokens: totalTokens},
		},
		MessageBreakdown: messageBreakdown(messages),
	}
	return ThreadContextSnapshot{
		ThreadID: threadID,
		Files: ThreadContextFiles{
			Markdown: filesMarkdown,
			Files:    threadContextFiles(files),
		},
		Usage: ThreadContextUsage{
			Markdown:         contextviz.FormatMarkdownTable(usageData, contextviz.Options{AntOnly: true}),
			Model:            model,
			ModelDisplayName: modeldisplay.RenderModelName(model),
			TotalTokens:      totalTokens,
			MaxTokens:        maxTokens,
			Percentage:       usageData.Percentage(),
			MessageCount:     len(messages),
		},
	}, nil
}

func (s *RunService) SessionInsights(opts sessioninsights.Options) sessioninsights.Report {
	s.mu.RLock()
	sessions := make([]sessioninsights.Session, 0, len(s.sessions))
	for _, thread := range s.threads {
		source := "awaken"
		platform := "desktop"
		model := "unknown"
		if thread.agent != nil {
			if configured := strings.TrimSpace(thread.agent.GetConfig().ModelConfig.Model); configured != "" {
				model = configured
			}
		}
		for _, runID := range thread.runIDs {
			session := s.sessions[runID]
			if session == nil {
				continue
			}
			run := session.Snapshot()
			events := session.History(0)
			sessions = append(sessions, sessionInsightFromRun(run, events, source, platform, model))
		}
	}
	s.mu.RUnlock()

	return sessioninsights.Generate(sessions, opts)
}

func (s *RunService) BranchThread(threadID string, req BranchThreadRequest) (BranchThreadResult, error) {
	s.mu.Lock()
	thread, ok := s.threads[threadID]
	if !ok {
		s.mu.Unlock()
		return BranchThreadResult{}, ErrThreadNotFound
	}
	messages := append([]convcontext.Message(nil), threadMessages(thread)...)
	if len(messages) == 0 {
		messages = messagesFromThreadRunsLocked(s, thread)
	}
	entries := branchEntriesFromMessages(threadID, messages)
	forkID := strings.TrimSpace(req.ForkSessionID)
	if forkID == "" {
		forkID = newRuntimeID("thread")
	}
	if _, exists := s.threads[forkID]; exists {
		s.mu.Unlock()
		return BranchThreadResult{}, ErrThreadAlreadyExists
	}
	fork, err := sessionbranch.BuildFork(entries, sessionbranch.Options{
		ForkSessionID:     forkID,
		OriginalSessionID: threadID,
		CustomTitle:       req.CustomTitle,
		ExistingTitles:    s.threadTitlesLocked(),
	})
	if err != nil {
		s.mu.Unlock()
		return BranchThreadResult{}, err
	}

	workspace := thread.workspace
	agent := s.newAgentForThreadLocked(workspace, fork.SessionID)
	conversation := conversationForAgent(agent)
	if conversation == nil {
		conversation = convcontext.NewConversation(nil)
	}
	conversation.Context.SetMessages(append([]convcontext.Message(nil), messages...))

	branch := &ThreadBranchMetadata{
		OriginalThreadID:      threadID,
		OriginalSessionID:     fork.OriginalSessionID,
		ForkedFromMessageUUID: lastForkedFromMessage(fork),
		Title:                 fork.Title,
	}
	metadata := sessiontags.Apply(sessiontags.Metadata{
		SessionID: fork.SessionID,
		Tag:       "branch",
		Mode:      sessiontags.ModeNormal,
	}, sessiontags.CustomTitleEntry{
		SessionID:   fork.SessionID,
		CustomTitle: fork.Title,
	})
	metadata = sessiontags.Apply(metadata, sessiontags.LastPromptEntry{
		SessionID:  fork.SessionID,
		LastPrompt: sessionbranch.DeriveFirstPrompt(fork.Messages),
	})
	newThread := &Thread{
		threadID:     fork.SessionID,
		agent:        agent,
		conversation: conversation,
		workspace:    workspace,
		label:        fork.Title,
		metadata:     metadata,
		branch:       branch,
		createdAt:    time.Now().UTC(),
		updatedAt:    time.Now().UTC(),
	}
	s.threads[fork.SessionID] = newThread
	s.pruneInactiveThreadsLocked()
	snapshot := s.snapshotThreadLocked(newThread)
	s.mu.Unlock()

	s.persistThread(fork.SessionID)
	return BranchThreadResult{Thread: snapshot, Fork: fork}, nil
}

func (s *RunService) newAgentForThreadLocked(workspace WorkspaceConfig, threadID string) *agentcore.Agent {
	if s.newAgent == nil {
		return nil
	}
	return s.newAgent(workspace, threadID)
}

func messagesFromThreadRunsLocked(s *RunService, thread *Thread) []convcontext.Message {
	var messages []convcontext.Message
	for _, runID := range thread.runIDs {
		session := s.sessions[runID]
		if session == nil {
			continue
		}
		run := session.Snapshot()
		if strings.TrimSpace(run.Input) != "" {
			messages = append(messages, convcontext.Message{Role: convcontext.RoleUser, Content: run.Input})
		}
		if strings.TrimSpace(run.Result) != "" {
			messages = append(messages, convcontext.Message{Role: convcontext.RoleAssistant, Content: run.Result})
		}
	}
	return messages
}

func branchEntriesFromMessages(threadID string, messages []convcontext.Message) []sessionbranch.Entry {
	entries := make([]sessionbranch.Entry, 0, len(messages))
	var parent *string
	for index, message := range messages {
		entryType := sessionbranch.TypeAssistant
		switch message.Role {
		case convcontext.RoleUser:
			entryType = sessionbranch.TypeUser
		case convcontext.RoleAssistant:
			entryType = sessionbranch.TypeAssistant
		default:
			continue
		}
		uuid := fmt.Sprintf("%s-msg-%d", threadID, index+1)
		entry := sessionbranch.Entry{
			SerializedMessage: sessionbranch.SerializedMessage{
				UUID:       uuid,
				Type:       entryType,
				SessionID:  threadID,
				ParentUUID: cloneString(parent),
				Content: []sessionbranch.ContentBlock{{
					Type: "text",
					Text: message.Content,
				}},
			},
		}
		entries = append(entries, entry)
		parent = &uuid
	}
	return entries
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func (s *RunService) threadTitlesLocked() []string {
	titles := make([]string, 0, len(s.threads))
	for _, thread := range s.threads {
		title := thread.metadataSnapshot().DisplayTitle
		if title == "" {
			title = thread.label
		}
		if title != "" {
			titles = append(titles, title)
		}
	}
	return titles
}

func lastForkedFromMessage(fork sessionbranch.Fork) string {
	for i := len(fork.Messages) - 1; i >= 0; i-- {
		if fork.Messages[i].ForkedFrom != nil {
			return fork.Messages[i].ForkedFrom.MessageUUID
		}
	}
	return ""
}

func contextFilesFromSessions(sessions []*Session) []contextfiles.FileEntry {
	entries := make([]contextfiles.FileEntry, 0)
	seen := map[string]struct{}{}
	for _, session := range sessions {
		if session == nil {
			continue
		}
		for _, event := range session.History(0) {
			if event.Kind != EventKindAgent {
				continue
			}
			for _, path := range pathsFromEvent(event.AgentEvent) {
				if _, ok := seen[path]; ok {
					continue
				}
				seen[path] = struct{}{}
				entries = append(entries, contextfiles.FileEntry{
					Path:         path,
					Source:       string(event.AgentEvent.Type),
					LastReadTime: event.Timestamp,
				})
			}
		}
	}
	return contextfiles.SortAndDedupe(entries)
}

func threadContextFiles(files []contextfiles.FileEntry) []ThreadContextFile {
	result := make([]ThreadContextFile, 0, len(files))
	for _, file := range files {
		result = append(result, ThreadContextFile{
			Path:         file.Path,
			Source:       file.Source,
			Tokens:       file.Tokens,
			LastReadTime: file.LastReadTime,
		})
	}
	return result
}

func pathsFromEvent(event agentcore.AwakenEvent) []string {
	var paths []string
	if strings.TrimSpace(event.ArtifactPath) != "" {
		paths = append(paths, event.ArtifactPath)
	}
	paths = append(paths, event.ApprovalAffectedPaths...)
	if event.Arguments != "" {
		paths = append(paths, pathsFromArguments(event.Arguments)...)
	}
	return cleanPathList(paths)
}

func pathsFromArguments(raw string) []string {
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil
	}
	var paths []string
	for _, key := range []string{"path", "file", "filename"} {
		if value, ok := payload[key].(string); ok {
			paths = append(paths, value)
		}
	}
	for _, key := range []string{"paths", "files"} {
		if values, ok := payload[key].([]any); ok {
			for _, value := range values {
				if text, ok := value.(string); ok {
					paths = append(paths, text)
				}
			}
		}
	}
	return paths
}

func cleanPathList(paths []string) []string {
	cleaned := make([]string, 0, len(paths))
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		cleaned = append(cleaned, filepath.Clean(p))
	}
	return cleaned
}

func estimateConversationTokens(messages []convcontext.Message) int {
	total := 0
	for _, message := range messages {
		total += convcontext.EstimateTokens(message.Content)
		for _, call := range message.ToolCalls {
			total += convcontext.EstimateTokens(call.Function.Name)
			total += convcontext.EstimateTokens(call.Function.Arguments)
		}
	}
	return total
}

func messageBreakdown(messages []convcontext.Message) *contextviz.MessageBreakdown {
	breakdown := &contextviz.MessageBreakdown{}
	toolCalls := map[string]*contextviz.ToolCallBreakdown{}
	for _, message := range messages {
		tokens := convcontext.EstimateTokens(message.Content)
		switch message.Role {
		case convcontext.RoleUser:
			breakdown.UserMessageTokens += tokens
		case convcontext.RoleAssistant:
			breakdown.AssistantMessageTokens += tokens
			for _, call := range message.ToolCalls {
				name := call.Function.Name
				if name == "" {
					name = "tool"
				}
				item := toolCalls[name]
				if item == nil {
					item = &contextviz.ToolCallBreakdown{Name: name}
					toolCalls[name] = item
				}
				callTokens := convcontext.EstimateTokens(call.Function.Arguments)
				item.CallTokens += callTokens
				breakdown.ToolCallTokens += callTokens
			}
		case convcontext.RoleTool:
			breakdown.ToolResultTokens += tokens
			name := firstNonEmpty(message.Name, message.ToolCallID, "tool")
			item := toolCalls[name]
			if item == nil {
				item = &contextviz.ToolCallBreakdown{Name: name}
				toolCalls[name] = item
			}
			item.ResultTokens += tokens
		}
	}
	names := make([]string, 0, len(toolCalls))
	for name := range toolCalls {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		breakdown.ToolCallsByType = append(breakdown.ToolCallsByType, *toolCalls[name])
	}
	return breakdown
}

func sessionInsightFromRun(run Run, events []Event, source, platform, model string) sessioninsights.Session {
	toolCounts := map[string]int{}
	messageCount := 0
	for _, event := range events {
		if event.Kind != EventKindAgent {
			continue
		}
		switch event.AgentEvent.Type {
		case agentcore.ToolCallStartedEventType:
			name := firstNonEmpty(event.AgentEvent.ToolName, "tool")
			toolCounts[name]++
		case agentcore.TextMessageFinishedEventType:
			messageCount++
		}
	}
	if run.Input != "" {
		messageCount++
	}
	tools := make([]sessioninsights.ToolCall, 0, len(toolCounts))
	for name, count := range toolCounts {
		tools = append(tools, sessioninsights.ToolCall{Name: name, Count: count})
	}
	sort.Slice(tools, func(i, j int) bool {
		return tools[i].Name < tools[j].Name
	})
	startedAt := run.CreatedAt
	endedAt := run.UpdatedAt
	if run.FinishedAt != nil {
		endedAt = *run.FinishedAt
	}
	inputTokens := int64(convcontext.EstimateTokens(run.Input))
	outputTokens := int64(convcontext.EstimateTokens(firstNonEmpty(run.Result, run.Error)))
	return sessioninsights.Session{
		ID:           run.RunID,
		Source:       source,
		Platform:     platform,
		Model:        model,
		StartedAt:    startedAt,
		EndedAt:      endedAt,
		MessageCount: messageCount,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		ToolCalls:    tools,
	}
}

func cloneThreadBranchMetadata(branch *ThreadBranchMetadata) *ThreadBranchMetadata {
	if branch == nil {
		return nil
	}
	clone := *branch
	return &clone
}

func truncateRunes(value string, limit int) string {
	if limit <= 0 {
		return value
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}
