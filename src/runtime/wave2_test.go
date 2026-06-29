package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"awaken/src/agentcore"
	convcontext "awaken/src/context"
	"awaken/src/sessionshare"
	"awaken/src/transcriptrepair"
)

func TestWave2ThreadCapabilitiesUseAgentToolTranscript(t *testing.T) {
	service := newWave2ToolRunService()
	workspaceRoot := t.TempDir()

	run, err := service.StartRun(StartRunRequest{
		Input:     "Summarize README unique wave2 token with api_key=sk-proj-wave2secret",
		Workspace: WorkspaceConfig{WorkspaceRoot: workspaceRoot},
	})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}
	final := waitForRunTerminal(t, service, run.RunID)
	if final.Status != RunStatusFinished {
		t.Fatalf("expected finished run, got %+v", final)
	}

	matches := service.SearchThreads("README unique wave2", 10)
	if len(matches) != 1 {
		t.Fatalf("expected one search match, got %+v", matches)
	}
	if matches[0].Thread.ThreadID != run.ThreadID || matches[0].Score <= 0 {
		t.Fatalf("unexpected search match: %+v", matches[0])
	}
	if matches[0].MessageIndex != nil {
		t.Fatalf("expected first-prompt match to omit message index, got %+v", *matches[0].MessageIndex)
	}
	encodedSearch, err := json.Marshal(matches[0])
	if err != nil {
		t.Fatalf("failed to marshal search result: %v", err)
	}
	if strings.Contains(string(encodedSearch), "message_index") {
		t.Fatalf("expected first-prompt match JSON to omit message_index, got %s", encodedSearch)
	}

	messageMatches := service.SearchThreads("output from", 10)
	if len(messageMatches) != 1 || messageMatches[0].MessageIndex == nil {
		t.Fatalf("expected message search match with message index, got %+v", messageMatches)
	}

	textExport, err := service.ExportThread(run.ThreadID, ThreadExportPlainText)
	if err != nil {
		t.Fatalf("ExportThread plain text returned error: %v", err)
	}
	if textExport.Format != ThreadExportPlainText || textExport.ContentType != "text/plain; charset=utf-8" {
		t.Fatalf("unexpected plain text export metadata: %+v", textExport)
	}
	for _, want := range []string{"## User", "Summarize README", "read_file", "README unique wave2", "final answer"} {
		if !strings.Contains(textExport.Content, want) {
			t.Fatalf("expected plain text export to contain %q, got:\n%s", want, textExport.Content)
		}
	}

	jsonlExport, err := service.ExportThread(run.ThreadID, ThreadExportJSONL)
	if err != nil {
		t.Fatalf("ExportThread JSONL returned error: %v", err)
	}
	if jsonlExport.Format != ThreadExportJSONL || jsonlExport.ContentType != "application/x-ndjson; charset=utf-8" {
		t.Fatalf("unexpected JSONL export metadata: %+v", jsonlExport)
	}
	if !strings.Contains(jsonlExport.Content, `"role":"tool"`) || !strings.Contains(jsonlExport.Content, `"name":"read_file"`) {
		t.Fatalf("expected JSONL export to include tool message, got:\n%s", jsonlExport.Content)
	}

	bundle, err := service.BuildThreadShareBundle(run.ThreadID, ThreadShareBundleRequest{})
	if err != nil {
		t.Fatalf("BuildThreadShareBundle returned error: %v", err)
	}
	if bundle.Bundle.SessionID != run.ThreadID || !bundle.Bundle.Plan.Allowed {
		t.Fatalf("unexpected share bundle metadata: %+v", bundle.Bundle)
	}
	bundleText := threadBundleContent(bundle.Bundle.Messages)
	if strings.Contains(bundleText, workspaceRoot) || strings.Contains(bundleText, "sk-proj-wave2secret") {
		t.Fatalf("expected default share bundle to redact path and secret, got %q", bundleText)
	}
	if bundle.Bundle.Privacy.RedactedFindings == 0 {
		t.Fatalf("expected default share bundle to report redactions, got %+v", bundle.Bundle.Privacy)
	}

	redactSecrets := false
	publicBundle, err := service.BuildThreadShareBundle(run.ThreadID, ThreadShareBundleRequest{
		Visibility:    sessionshare.VisibilityPublic,
		Action:        sessionshare.ActionShare,
		RedactSecrets: &redactSecrets,
	})
	if !errors.Is(err, sessionshare.ErrUnsafePublicBundle) {
		t.Fatalf("expected ErrUnsafePublicBundle, got result=%+v err=%v", publicBundle, err)
	}
	if len(publicBundle.Bundle.Messages) != 0 {
		t.Fatalf("unsafe public bundle must not return transcript messages, got %+v", publicBundle.Bundle.Messages)
	}

	clearPlan, err := service.BuildThreadClearPlan(run.ThreadID)
	if err != nil {
		t.Fatalf("BuildThreadClearPlan returned error: %v", err)
	}
	if !clearPlan.Plan.ClearMessages ||
		clearPlan.Plan.Session.PreviousID != run.ThreadID ||
		clearPlan.Plan.Session.NewID == "" ||
		clearPlan.Plan.Restore.CWD != workspaceRoot ||
		clearPlan.Plan.CacheEvictionHint == nil ||
		clearPlan.Plan.CacheEvictionHint.LastRequestID != run.RunID {
		t.Fatalf("unexpected clear plan: %+v", clearPlan.Plan)
	}
}

func TestRestorePersistedStateRepairsConversationForNewAgent(t *testing.T) {
	persistenceDir := t.TempDir()
	workspaceRoot := t.TempDir()
	now := time.Now().UTC()
	threadID := "thread_repair"

	store := newFileStore(persistenceDir)
	if err := store.saveThread(persistedThread{
		Version: persistenceVersion,
		Thread: ThreadSnapshot{
			ThreadID: threadID,
			Label:    "repair me",
			Workspace: WorkspaceConfig{
				WorkspaceRoot: workspaceRoot,
				WritableRoots: []string{workspaceRoot},
			},
			CreatedAt: now,
			UpdatedAt: now,
		},
		ConversationMessages: []convcontext.Message{
			{Role: convcontext.RoleUser, Content: "Read README"},
			{
				Role:    convcontext.RoleAssistant,
				Content: "Reading now",
				ToolCalls: []convcontext.ToolCall{{
					ID:   "call_missing",
					Type: "function",
					Function: convcontext.FunctionCall{
						Name:      "read_file",
						Arguments: `{"path":"README.md"}`,
					},
				}},
			},
		},
		ConversationCreatedAt: now.Format(timeLayout),
		ConversationUpdatedAt: now.Format(timeLayout),
	}); err != nil {
		t.Fatalf("failed to persist thread: %v", err)
	}

	observed := make(chan []convcontext.Message, 1)
	service := NewRunServiceWithOptions(
		func(workspace WorkspaceConfig, threadID string) *agentcore.Agent {
			return agentcore.NewAgentWithThreadID(agentcore.AgentConfig{}, threadID)
		},
		func(agent *agentcore.Agent) Executor {
			return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
				messages := agent.GetConversation().Context.GetMessages()
				observed <- messages
				if err := convcontext.ValidateToolResultPairing(messages); err != nil {
					return "", err
				}
				agent.GetConversation().UserInput(input)
				answer := "reply:" + input
				agent.GetConversation().AssistantOutput(answer, nil)
				sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunFinishedEventType, Content: answer})
				return answer, nil
			})
		},
		WithPersistenceDir(persistenceDir),
	)

	run, err := service.StartRun(StartRunRequest{
		Input:     "follow up",
		ThreadID:  threadID,
		Workspace: WorkspaceConfig{WorkspaceRoot: workspaceRoot},
	})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}
	final := waitForRunTerminal(t, service, run.RunID)
	if final.Status != RunStatusFinished {
		t.Fatalf("expected restored run to finish, got %+v", final)
	}

	select {
	case messages := <-observed:
		if err := convcontext.ValidateToolResultPairing(messages); err != nil {
			t.Fatalf("restored agent conversation was not repaired: %v\n%+v", err, messages)
		}
		var foundSynthetic bool
		for _, msg := range messages {
			if msg.Role == convcontext.RoleTool &&
				msg.ToolCallID == "call_missing" &&
				msg.IsError &&
				strings.Contains(msg.Content, transcriptrepair.SyntheticToolResultPlaceholder) {
				foundSynthetic = true
				break
			}
		}
		if !foundSynthetic {
			t.Fatalf("expected repaired conversation to include synthetic tool result, got %+v", messages)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for restored conversation observation")
	}
}

func TestRepairConversationForRecoveryInsertsMissingToolResult(t *testing.T) {
	messages := []convcontext.Message{
		{Role: convcontext.RoleUser, Content: "Read README"},
		{
			Role:    convcontext.RoleAssistant,
			Content: "Reading now",
			ToolCalls: []convcontext.ToolCall{{
				ID:   "call_missing",
				Type: "function",
				Function: convcontext.FunctionCall{
					Name:      "read_file",
					Arguments: `{"path":"README.md"}`,
				},
			}},
		},
	}

	repaired := repairConversationForRecovery(messages, "/tmp/workspace")
	if err := convcontext.ValidateToolResultPairing(repaired); err != nil {
		t.Fatalf("expected repaired conversation to have valid tool pairing: %v\n%+v", err, repaired)
	}

	assistant := repaired[1]
	if len(assistant.ToolCalls) != 1 ||
		assistant.ToolCalls[0].Function.Name != "read_file" ||
		assistant.ToolCalls[0].Function.Arguments != `{"path":"README.md"}` {
		t.Fatalf("expected repaired assistant tool call to preserve name and args, got %+v", assistant)
	}
	var toolResult convcontext.Message
	for _, msg := range repaired {
		if msg.Role == convcontext.RoleTool && msg.ToolCallID == "call_missing" {
			toolResult = msg
			break
		}
	}
	if toolResult.Role != convcontext.RoleTool ||
		toolResult.ToolCallID != "call_missing" ||
		!toolResult.IsError ||
		!strings.Contains(toolResult.Content, transcriptrepair.SyntheticToolResultPlaceholder) {
		t.Fatalf("expected synthetic error tool result, got %+v", toolResult)
	}
}

func newWave2ToolRunService() *RunService {
	return NewRunService(
		func(workspace WorkspaceConfig, threadID string) *agentcore.Agent {
			return agentcore.NewAgentWithThreadID(agentcore.AgentConfig{}, threadID)
		},
		func(agent *agentcore.Agent) Executor {
			return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
				workspaceRoot := agent.GetConfig().Workspace.Root
				toolCallID := "call_readme"
				toolInput := `{"path":"` + workspaceRoot + `/README.md","api_key":"sk-proj-wave2secret"}`
				toolOutput := "README unique wave2 output from " + workspaceRoot + "/README.md with sk-proj-wave2secret"

				conversation := agent.GetConversation()
				conversation.UserInput(input)
				sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
				conversation.AssistantOutput("I'll inspect README.", []convcontext.ToolCall{{
					ID:   toolCallID,
					Type: "function",
					Function: convcontext.FunctionCall{
						Name:      "read_file",
						Arguments: toolInput,
					},
				}})
				sink.Emit(agentcore.AwakenEvent{
					Type:       agentcore.ToolCallStartedEventType,
					ToolCallID: toolCallID,
					ToolName:   "read_file",
				})
				sink.Emit(agentcore.AwakenEvent{
					Type:       agentcore.ToolCallArgsEventType,
					ToolCallID: toolCallID,
					ToolName:   "read_file",
					Arguments:  toolInput,
				})
				sink.Emit(agentcore.AwakenEvent{
					Type:       agentcore.ToolCallFinishedEventType,
					ToolCallID: toolCallID,
					ToolName:   "read_file",
				})
				conversation.ToolResult(toolCallID, "read_file", toolOutput)
				sink.Emit(agentcore.AwakenEvent{
					Type:       agentcore.ToolResultEventType,
					ToolCallID: toolCallID,
					ToolName:   "read_file",
					Content:    toolOutput,
				})
				answer := "final answer for README unique wave2 token"
				conversation.AssistantOutput(answer, nil)
				sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunFinishedEventType, Content: answer})
				return answer, nil
			})
		},
	)
}

func threadBundleContent(messages []sessionshare.BundleMessage) string {
	parts := make([]string, 0, len(messages))
	for _, message := range messages {
		parts = append(parts, message.Content)
		if message.ToolOutput != nil {
			parts = append(parts, message.ToolOutput.Summary)
		}
	}
	return strings.Join(parts, "\n")
}
