package runtime

import (
	"errors"
	"strings"

	"awaken/src/agentcore"
	convcontext "awaken/src/context"
	"awaken/src/convrecovery"
	"awaken/src/sessionclear"
	"awaken/src/sessionexport"
	"awaken/src/sessionshare"
	"awaken/src/transcriptindex"
	"awaken/src/transcriptrepair"
)

type ThreadSearchResult struct {
	Thread       ThreadSnapshot `json:"thread"`
	Score        int            `json:"score"`
	Match        string         `json:"match"`
	MessageIndex *int           `json:"message_index,omitempty"`
	Snippet      string         `json:"snippet,omitempty"`
}

type ThreadExportFormat string

const (
	ThreadExportPlainText ThreadExportFormat = "plain_text"
	ThreadExportJSONL     ThreadExportFormat = "jsonl"
)

type ThreadExportResult struct {
	Thread      ThreadSnapshot     `json:"thread"`
	Format      ThreadExportFormat `json:"format"`
	Filename    string             `json:"filename"`
	ContentType string             `json:"content_type"`
	Content     string             `json:"content"`
}

type ThreadShareBundleRequest struct {
	Visibility         sessionshare.Visibility `json:"visibility,omitempty"`
	Action             sessionshare.Action     `json:"action,omitempty"`
	IncludeToolOutputs *bool                   `json:"include_tool_outputs,omitempty"`
	RedactSecrets      *bool                   `json:"redact_secrets,omitempty"`
	RedactPaths        *bool                   `json:"redact_paths,omitempty"`
	MaxToolOutputChars int                     `json:"max_tool_output_chars,omitempty"`
	SummaryChars       int                     `json:"summary_chars,omitempty"`
}

type ThreadShareBundleResult struct {
	Thread ThreadSnapshot      `json:"thread"`
	Bundle sessionshare.Bundle `json:"bundle"`
}

type ThreadClearPlanResult struct {
	Thread ThreadSnapshot          `json:"thread"`
	Plan   sessionclear.PlanResult `json:"plan"`
}

func (s *RunService) SearchThreads(query string, limit int) []ThreadSearchResult {
	query = strings.TrimSpace(query)
	if query == "" {
		return []ThreadSearchResult{}
	}
	if limit <= 0 {
		limit = 20
	}

	s.mu.RLock()
	items := make([]transcriptindex.SessionItem, 0, len(s.threads))
	threads := make(map[string]ThreadSnapshot, len(s.threads))
	for _, thread := range s.threads {
		snapshot := s.snapshotThreadLocked(thread)
		item := threadSearchItemLocked(s, thread, snapshot)
		items = append(items, item)
		threads[item.SessionID] = snapshot
	}
	s.mu.RUnlock()

	matches := transcriptindex.Search(items, query, limit)
	results := make([]ThreadSearchResult, 0, len(matches))
	for _, match := range matches {
		snapshot, ok := threads[match.Item.SessionID]
		if !ok {
			continue
		}
		result := ThreadSearchResult{
			Thread:  snapshot,
			Score:   match.Score,
			Match:   match.Match,
			Snippet: searchSnippet(match),
		}
		if match.MessageIndex >= 0 {
			result.MessageIndex = &match.MessageIndex
		}
		results = append(results, result)
	}
	return results
}

func (s *RunService) ExportThread(threadID string, format ThreadExportFormat) (ThreadExportResult, error) {
	format = normalizeThreadExportFormat(format)

	s.mu.RLock()
	thread, ok := s.threads[threadID]
	if !ok {
		s.mu.RUnlock()
		return ThreadExportResult{}, ErrThreadNotFound
	}
	snapshot := s.snapshotThreadLocked(thread)
	messages := exportMessagesFromThreadLocked(s, thread)
	s.mu.RUnlock()

	var (
		content     string
		filename    string
		contentType string
		err         error
	)
	switch format {
	case ThreadExportJSONL:
		content, err = sessionexport.RenderJSONL(messages)
		filename = sessionexport.DefaultJSONLFilename(messages, snapshot.UpdatedAt)
		contentType = "application/x-ndjson; charset=utf-8"
	default:
		content = sessionexport.RenderPlainText(messages, sessionexport.PlainTextOptions{IncludeTimestamps: true})
		filename = sessionexport.DefaultTextFilename(messages, snapshot.UpdatedAt)
		contentType = "text/plain; charset=utf-8"
	}
	if err != nil {
		return ThreadExportResult{}, err
	}
	return ThreadExportResult{
		Thread:      snapshot,
		Format:      format,
		Filename:    filename,
		ContentType: contentType,
		Content:     content,
	}, nil
}

func (s *RunService) BuildThreadShareBundle(
	threadID string,
	request ThreadShareBundleRequest,
) (ThreadShareBundleResult, error) {
	s.mu.RLock()
	thread, ok := s.threads[threadID]
	if !ok {
		s.mu.RUnlock()
		return ThreadShareBundleResult{}, ErrThreadNotFound
	}
	snapshot := s.snapshotThreadLocked(thread)
	messages := shareMessagesFromThreadLocked(s, thread)
	workspaceRoot := thread.workspace.WorkspaceRoot
	s.mu.RUnlock()

	opts := threadShareOptions(request, workspaceRoot)
	bundle, err := sessionshare.BuildBundle(sessionshare.BuildInput{
		SessionID: snapshot.ThreadID,
		Title:     firstNonEmpty(snapshot.Metadata.DisplayTitle, snapshot.Label),
		Messages:  messages,
	}, opts)
	if errors.Is(err, sessionshare.ErrUnsafePublicBundle) {
		return ThreadShareBundleResult{}, err
	}
	if err != nil {
		return ThreadShareBundleResult{}, err
	}
	return ThreadShareBundleResult{Thread: snapshot, Bundle: bundle}, nil
}

func (s *RunService) BuildThreadClearPlan(threadID string) (ThreadClearPlanResult, error) {
	s.mu.RLock()
	thread, ok := s.threads[threadID]
	if !ok {
		s.mu.RUnlock()
		return ThreadClearPlanResult{}, ErrThreadNotFound
	}
	snapshot := s.snapshotThreadLocked(thread)
	latestRunID := thread.latestRunID
	workspaceRoot := thread.workspace.WorkspaceRoot
	s.mu.RUnlock()

	plan := sessionclear.PlanClear(sessionclear.Input{
		CurrentSessionID: snapshot.ThreadID,
		NewSessionID:     newRuntimeID("thread"),
		OriginalCWD:      workspaceRoot,
		LastRequestID:    latestRunID,
	})
	return ThreadClearPlanResult{Thread: snapshot, Plan: plan}, nil
}

func repairConversationForRecovery(messages []convcontext.Message, workspaceRoot string) []convcontext.Message {
	repaired, _, err := transcriptrepair.EnsureToolResultPairing(
		repairMessagesFromConversation(messages),
		transcriptrepair.Options{},
	)
	if err != nil {
		repaired = repairMessagesFromConversation(messages)
	}
	recovered := convrecovery.Recover(
		recoveryMessagesFromRepair(repaired),
		convrecovery.Options{CWD: workspaceRoot},
	)
	return conversationMessagesFromRecovery(recovered.Messages)
}

func threadSearchItemLocked(s *RunService, thread *Thread, snapshot ThreadSnapshot) transcriptindex.SessionItem {
	messages := searchMessagesFromThreadLocked(s, thread)
	metadata := snapshot.Metadata
	title := firstNonEmpty(metadata.DisplayTitle, metadata.CustomTitle, metadata.AITitle, snapshot.Label)
	firstPrompt := firstNonEmpty(metadata.LastPrompt, snapshot.Label)
	return transcriptindex.SessionItem{
		Summary: transcriptindex.Summary{
			SessionID:    snapshot.ThreadID,
			Title:        title,
			FirstPrompt:  firstPrompt,
			Text:         metadata.TaskSummary,
			MessageCount: len(messages),
		},
		Messages: messages,
	}
}

func searchMessagesFromThreadLocked(s *RunService, thread *Thread) []transcriptindex.Message {
	conversation := threadMessages(thread)
	if len(conversation) == 0 {
		conversation = messagesFromThreadRunsLocked(s, thread)
	}
	messages := make([]transcriptindex.Message, 0, len(conversation))
	for _, msg := range conversation {
		messages = append(messages, transcriptindex.Message{
			Type: string(msg.Role),
			Role: string(msg.Role),
			Text: msg.Content,
		})
		for _, call := range msg.ToolCalls {
			messages = append(messages, transcriptindex.Message{
				Type: "tool_call",
				Role: "assistant",
				Text: strings.TrimSpace(call.Function.Name + " " + call.Function.Arguments),
			})
		}
	}
	for _, runID := range thread.runIDs {
		session := s.sessions[runID]
		if session == nil {
			continue
		}
		for _, event := range session.History(0) {
			if text := searchableEventText(event.AgentEvent); text != "" {
				messages = append(messages, transcriptindex.Message{
					Type: string(event.AgentEvent.Type),
					Role: event.AgentEvent.Role,
					Text: text,
				})
			}
		}
	}
	return messages
}

func exportMessagesFromThreadLocked(s *RunService, thread *Thread) []sessionexport.Message {
	messages := make([]sessionexport.Message, 0)
	for _, runID := range thread.runIDs {
		session := s.sessions[runID]
		if session == nil {
			continue
		}
		run := session.Snapshot()
		createdAt := run.CreatedAt
		if strings.TrimSpace(run.Input) != "" {
			messages = append(messages, sessionexport.Message{
				ID:        run.RunID + ":user",
				Role:      sessionexport.RoleUser,
				Timestamp: &createdAt,
				Parts:     []sessionexport.Part{sessionexport.TextPart(run.Input)},
			})
		}
		messages = append(messages, exportToolMessages(session.History(0))...)
		finishedAt := run.UpdatedAt
		if run.FinishedAt != nil {
			finishedAt = *run.FinishedAt
		}
		if text := strings.TrimSpace(firstNonEmpty(run.Result, run.Error)); text != "" {
			messages = append(messages, sessionexport.Message{
				ID:        run.RunID + ":assistant",
				Role:      sessionexport.RoleAssistant,
				Timestamp: &finishedAt,
				Parts:     []sessionexport.Part{sessionexport.TextPart(text)},
			})
		}
	}
	return messages
}

func shareMessagesFromThreadLocked(s *RunService, thread *Thread) []sessionshare.Message {
	exported := exportMessagesFromThreadLocked(s, thread)
	messages := make([]sessionshare.Message, 0, len(exported))
	for _, msg := range exported {
		role := sessionshare.Role(msg.Role)
		contentParts := make([]string, 0, len(msg.Parts))
		var toolName string
		var toolCallID string
		for _, part := range msg.Parts {
			if part.ToolEvent != nil {
				toolName = firstNonEmpty(toolName, part.ToolEvent.Name)
				toolCallID = firstNonEmpty(toolCallID, part.ToolEvent.ID)
			}
			rendered := renderExportPartForShare(part)
			if rendered != "" {
				contentParts = append(contentParts, rendered)
			}
		}
		messages = append(messages, sessionshare.Message{
			ID:         msg.ID,
			Role:       role,
			Content:    strings.Join(contentParts, "\n"),
			ToolName:   toolName,
			ToolCallID: toolCallID,
		})
	}
	return messages
}

func exportToolMessages(events []Event) []sessionexport.Message {
	toolNames := map[string]string{}
	toolInputs := map[string]string{}
	messages := make([]sessionexport.Message, 0)
	for _, event := range events {
		agentEvent := event.AgentEvent
		switch agentEvent.Type {
		case agentcore.ToolCallStartedEventType:
			toolNames[agentEvent.ToolCallID] = agentEvent.ToolName
		case agentcore.ToolCallArgsEventType:
			toolNames[agentEvent.ToolCallID] = firstNonEmpty(toolNames[agentEvent.ToolCallID], agentEvent.ToolName)
			toolInputs[agentEvent.ToolCallID] += agentEvent.Arguments
		case agentcore.ToolCallFinishedEventType:
			timestamp := event.Timestamp
			messages = append(messages, sessionexport.Message{
				ID:        agentEvent.ToolCallID + ":call",
				Role:      sessionexport.RoleAssistant,
				Timestamp: &timestamp,
				Parts: []sessionexport.Part{sessionexport.ToolCallPart(sessionexport.ToolEvent{
					ID:    agentEvent.ToolCallID,
					Name:  firstNonEmpty(agentEvent.ToolName, toolNames[agentEvent.ToolCallID]),
					Input: toolInputs[agentEvent.ToolCallID],
				})},
			})
		case agentcore.ToolResultEventType:
			timestamp := event.Timestamp
			messages = append(messages, sessionexport.Message{
				ID:        agentEvent.ToolCallID + ":result",
				Role:      sessionexport.RoleTool,
				Timestamp: &timestamp,
				Parts: []sessionexport.Part{sessionexport.ToolResultPart(sessionexport.ToolEvent{
					ID:     agentEvent.ToolCallID,
					Name:   firstNonEmpty(agentEvent.ToolName, toolNames[agentEvent.ToolCallID]),
					Output: agentEvent.Content,
					Status: "done",
				})},
			})
		}
	}
	return messages
}

func searchableEventText(event agentcore.AwakenEvent) string {
	switch event.Type {
	case agentcore.ToolCallStartedEventType:
		return event.ToolName
	case agentcore.ToolCallArgsEventType:
		return strings.TrimSpace(event.ToolName + " " + event.Arguments)
	case agentcore.ToolResultEventType:
		return strings.TrimSpace(event.ToolName + " " + event.Content)
	case agentcore.ArtifactCreatedEventType, agentcore.ArtifactUpdatedEventType:
		return strings.TrimSpace(event.ArtifactTitle + " " + event.ArtifactPath + " " + event.ArtifactContent)
	case agentcore.TextDeltaEventType, agentcore.ReasoningDeltaEventType:
		return event.Delta
	case agentcore.RunFinishedEventType:
		return event.Content
	default:
		return ""
	}
}

func normalizeThreadExportFormat(format ThreadExportFormat) ThreadExportFormat {
	switch format {
	case ThreadExportJSONL:
		return ThreadExportJSONL
	default:
		return ThreadExportPlainText
	}
}

func threadShareOptions(request ThreadShareBundleRequest, workspaceRoot string) sessionshare.Options {
	opts := sessionshare.DefaultOptions()
	if request.Visibility != "" {
		opts.Visibility = request.Visibility
	}
	if request.Action != "" {
		opts.Action = request.Action
	}
	if request.IncludeToolOutputs != nil {
		opts.IncludeToolOutputs = *request.IncludeToolOutputs
	}
	if request.RedactSecrets != nil {
		opts.RedactSecrets = *request.RedactSecrets
	}
	if request.RedactPaths != nil {
		opts.RedactPaths = *request.RedactPaths
	}
	if request.MaxToolOutputChars > 0 {
		opts.MaxToolOutputChars = request.MaxToolOutputChars
	}
	if request.SummaryChars > 0 {
		opts.SummaryChars = request.SummaryChars
	}
	if strings.TrimSpace(workspaceRoot) != "" {
		opts.PathRoots = []string{workspaceRoot}
	}
	return opts
}

func searchSnippet(result transcriptindex.Result) string {
	if result.MessageIndex >= 0 && result.MessageIndex < len(result.Item.Messages) {
		return result.Item.Messages[result.MessageIndex].Text
	}
	return firstNonEmpty(result.Item.Title, result.Item.FirstPrompt, result.Item.Text)
}

func renderExportPartForShare(part sessionexport.Part) string {
	if part.ToolEvent == nil {
		return strings.TrimSpace(part.Text)
	}
	event := part.ToolEvent
	switch part.Type {
	case sessionexport.PartToolCall:
		return strings.TrimSpace(event.Name + " " + event.Input)
	case sessionexport.PartToolResult:
		return strings.TrimSpace(firstNonEmpty(event.Output, event.Summary, event.Error))
	default:
		return ""
	}
}

func repairMessagesFromConversation(messages []convcontext.Message) []transcriptrepair.Message {
	repaired := make([]transcriptrepair.Message, 0, len(messages))
	for _, msg := range messages {
		switch msg.Role {
		case convcontext.RoleAssistant:
			blocks := []transcriptrepair.ContentBlock{}
			if strings.TrimSpace(msg.Content) != "" {
				blocks = append(blocks, transcriptrepair.ContentBlock{
					Type: transcriptrepair.BlockText,
					Text: msg.Content,
				})
			}
			for _, call := range msg.ToolCalls {
				blocks = append(blocks, transcriptrepair.ContentBlock{
					Type:  transcriptrepair.BlockToolUse,
					ID:    call.ID,
					Name:  call.Function.Name,
					Input: call.Function.Arguments,
				})
			}
			repaired = append(repaired, transcriptrepair.Message{
				Role:    transcriptrepair.RoleAssistant,
				Content: blocks,
			})
		case convcontext.RoleTool:
			repaired = append(repaired, transcriptrepair.Message{
				Role: transcriptrepair.RoleUser,
				Content: []transcriptrepair.ContentBlock{{
					Type:      transcriptrepair.BlockToolResult,
					ToolUseID: msg.ToolCallID,
					Content:   msg.Content,
					IsError:   msg.IsError,
				}},
			})
		default:
			repaired = append(repaired, transcriptrepair.Message{
				Role: transcriptrepair.RoleUser,
				Content: []transcriptrepair.ContentBlock{{
					Type: transcriptrepair.BlockText,
					Text: msg.Content,
				}},
			})
		}
	}
	return repaired
}

func recoveryMessagesFromRepair(messages []transcriptrepair.Message) []convrecovery.Message {
	recovered := make([]convrecovery.Message, 0, len(messages))
	for _, msg := range messages {
		messageType := convrecovery.MessageTypeUser
		if msg.Role == transcriptrepair.RoleAssistant {
			messageType = convrecovery.MessageTypeAssistant
		}
		blocks := make([]convrecovery.Block, 0, len(msg.Content))
		for _, block := range msg.Content {
			blocks = append(blocks, convrecovery.Block{
				Type:      string(block.Type),
				Text:      firstNonEmpty(block.Text, stringFromAny(block.Input), stringFromAny(block.Content)),
				ID:        block.ID,
				Name:      block.Name,
				ToolUseID: block.ToolUseID,
				IsError:   block.IsError,
			})
		}
		recovered = append(recovered, convrecovery.Message{
			Type:          messageType,
			ContentBlocks: blocks,
		})
	}
	return recovered
}

func conversationMessagesFromRecovery(messages []convrecovery.Message) []convcontext.Message {
	result := make([]convcontext.Message, 0, len(messages))
	for _, msg := range messages {
		switch msg.Type {
		case convrecovery.MessageTypeAssistant:
			var text []string
			var calls []convcontext.ToolCall
			for _, block := range msg.ContentBlocks {
				switch block.Type {
				case convrecovery.BlockTypeToolUse:
					calls = append(calls, convcontext.ToolCall{
						ID:   block.ID,
						Type: "function",
						Function: convcontext.FunctionCall{
							Name:      block.Name,
							Arguments: block.Text,
						},
					})
				case convrecovery.BlockTypeText:
					if strings.TrimSpace(block.Text) != "" {
						text = append(text, block.Text)
					}
				}
			}
			result = append(result, convcontext.Message{
				Role:      convcontext.RoleAssistant,
				Content:   strings.Join(text, "\n"),
				ToolCalls: calls,
			})
		case convrecovery.MessageTypeUser:
			for _, block := range msg.ContentBlocks {
				if block.Type == convrecovery.BlockTypeToolResult {
					result = append(result, convcontext.Message{
						Role:       convcontext.RoleTool,
						ToolCallID: block.ToolUseID,
						Content:    block.Text,
						IsError:    block.IsError,
					})
					continue
				}
				if strings.TrimSpace(block.Text) != "" {
					result = append(result, convcontext.Message{
						Role:    convcontext.RoleUser,
						Content: block.Text,
					})
				}
			}
		}
	}
	return result
}

func stringFromAny(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}
