package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"awaken/src/agentcore"
	"awaken/src/subagentruntime"
)

func TestParentContinuationRestoresChildAfterCoreRestart(t *testing.T) {
	t.Setenv("AWAKEN_HOME", t.TempDir())
	var mu sync.Mutex
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		encoded, _ := json.Marshal(body)
		mu.Lock()
		requests = append(requests, string(encoded))
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"private-child-reply\"}}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	workspace := t.TempDir()
	model := agentcore.ModelConfig{APIKey: "test", BaseURL: server.URL, Model: "test-model", ProfileID: "configured-test"}
	factory := func(ws WorkspaceConfig, id string) *agentcore.Agent {
		return agentcore.NewAgentWithThreadID(agentcore.AgentConfig{ModelConfig: model, MaxThoughts: 8,
			ResolveModelBinding: func(context.Context, string, string) (agentcore.ModelConfig, error) { return model, nil },
			Workspace:           agentcore.WorkspaceConfig{Root: ws.WorkspaceRoot, ReadableRoots: ws.ReadableRoots, WritableRoots: ws.WritableRoots}}, id)
	}
	initialChild := make(chan subagentruntime.TaskSnapshot, 1)
	executor := func(agent *agentcore.Agent) Executor {
		return ExecutorFunc(func(ctx context.Context, _ string, _ agentcore.EventSink) (string, error) {
			controller := agent.GetConfig().SubagentController
			task, err := controller.Spawn(ctx, subagentruntime.SpawnRequest{Description: "retained worker", Prompt: "remember private-value-742", AgentType: "general"})
			if err != nil {
				return "", err
			}
			result, err := controller.Wait(ctx, subagentruntime.WaitRequest{TaskIDs: []string{task.TaskID}, ReturnWhen: "all", EventDriven: true})
			if err != nil {
				return "", err
			}
			initialChild <- result.Tasks[0]
			return "root done", nil
		})
	}
	first := NewRunService(factory, executor)
	run, err := first.StartRun(StartRunRequest{Input: "root task", Workspace: WorkspaceConfig{WorkspaceRoot: workspace}})
	if err != nil {
		t.Fatal(err)
	}
	waitForRunTerminal(t, first, run.RunID)
	first.drainRunWorkers()
	journal, err := first.journals.Open(run.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := journal.AppendFlushed(context.Background(), agentcore.NewSemanticRecord(agentcore.SemanticKindAssistantMessage, run.ThreadID, run.RunID, "", "", "", map[string]any{"message_id": "root-answer", "content": "retained root answer", "role": "assistant"})); err != nil {
		t.Fatal(err)
	}
	session, _ := first.session(run.RunID)
	manager := session.SubagentController().(*subagentruntime.Manager)
	var task subagentruntime.TaskSnapshot
	select {
	case task = <-initialChild:
	default:
		t.Fatal("parent did not complete child")
	}
	if task.Status != subagentruntime.StatusCompleted {
		t.Fatal(task.Status)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := manager.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if err := first.journals.CloseThread(run.ThreadID); err != nil {
		t.Fatal(err)
	}

	continued := make(chan subagentruntime.TaskSnapshot, 1)
	second := NewRunService(factory, func(agent *agentcore.Agent) Executor {
		return ExecutorFunc(func(ctx context.Context, _ string, _ agentcore.EventSink) (string, error) {
			controller := agent.GetConfig().SubagentController.(subagentruntime.SessionController)
			next, err := controller.Followup(ctx, task.TaskID, "continue with the remembered value")
			if err != nil {
				return "", err
			}
			continued <- next
			_, err = agent.GetConfig().SubagentController.Wait(ctx, subagentruntime.WaitRequest{TaskIDs: []string{next.TaskID}, ReturnWhen: "all", EventDriven: true})
			return "parent integrated result", err
		})
	})
	root, err := second.StartRun(StartRunRequest{ThreadID: run.ThreadID, Input: "continue through parent", Workspace: WorkspaceConfig{WorkspaceRoot: workspace}})
	if err != nil {
		t.Fatal(err)
	}
	waitForRunTerminal(t, second, root.RunID)
	second.drainRunWorkers()
	var next subagentruntime.TaskSnapshot
	select {
	case next = <-continued:
	default:
		t.Fatal("parent did not continue child")
	}
	if next.AgentID != task.AgentID || next.TaskID == task.TaskID || next.AttemptID == task.AttemptID {
		t.Fatalf("identity: %+v -> %+v", task, next)
	}
	restoredSession, _ := second.session(root.RunID)
	restoredManager := restoredSession.SubagentController().(*subagentruntime.Manager)
	defer restoredManager.Close(ctx)
	result, err := restoredManager.Wait(ctx, subagentruntime.WaitRequest{TaskIDs: []string{next.TaskID}, ReturnWhen: "all", EventDriven: true})
	if err != nil || len(result.Tasks) != 1 || result.Tasks[0].Status != subagentruntime.StatusCompleted {
		t.Fatalf("continued child: %+v %v", result, err)
	}
	mu.Lock()
	captured := append([]string(nil), requests...)
	mu.Unlock()
	if len(captured) != 2 {
		t.Fatalf("expected only two child model calls, got %d", len(captured))
	}
	for _, expected := range []string{"private-value-742", "private-child-reply", "continue with the remembered value"} {
		if !strings.Contains(captured[1], expected) {
			t.Fatalf("continued context lost %q", expected)
		}
	}
	if strings.Contains(captured[1], "root task") {
		t.Fatal("fresh child inherited root history")
	}
	transcript, err := second.GetThreadTranscript(run.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	foundRoot := false
	for _, execution := range transcript.Runs {
		for _, record := range execution.SemanticRecords {
			if record.Kind == agentcore.SemanticKindAssistantMessage && record.Payload["content"] == "retained root answer" {
				foundRoot = true
			}
		}
	}
	if !foundRoot {
		t.Fatal("parent continuation hid the completed root answer")
	}
}

func TestRootRebuildPreservesPassiveCompactState(t *testing.T) {
	t.Setenv("AWAKEN_HOME", t.TempDir())
	created := make(chan agentcore.AgentConfig, 2)
	service := NewRunService(func(ws WorkspaceConfig, id string) *agentcore.Agent {
		return agentcore.NewAgentWithThreadID(agentcore.AgentConfig{Workspace: agentcore.WorkspaceConfig{Root: ws.WorkspaceRoot}}, id)
	}, func(agent *agentcore.Agent) Executor {
		created <- agent.GetConfig()
		return ExecutorFunc(func(context.Context, string, agentcore.EventSink) (string, error) { return "done", nil })
	})
	workspace := WorkspaceConfig{WorkspaceRoot: t.TempDir()}
	first, err := service.StartRun(StartRunRequest{Input: "first", Workspace: workspace})
	if err != nil {
		t.Fatal(err)
	}
	waitForRunTerminal(t, service, first.RunID)
	service.drainRunWorkers()
	<-created
	service.mu.Lock()
	thread := service.threads[first.ThreadID]
	config := thread.agent.GetConfig()
	config.CompactContextBlocks = []agentcore.ContextBlock{{ID: "checkpoint", Content: "keep the original constraint"}}
	config.CompactedMemoryContext = "retained memory references"
	config.RestoredContextBundleV2 = &agentcore.PostCompactContextBundleV2{BoundaryID: "retained-boundary", Generation: 3}
	thread.agent.Reconfigure(config)
	thread.modelProfileKey = "passive-child-host"
	service.mu.Unlock()
	next, err := service.StartRun(StartRunRequest{ThreadID: first.ThreadID, Input: "continue root", Workspace: workspace})
	if err != nil {
		t.Fatal(err)
	}
	waitForRunTerminal(t, service, next.RunID)
	service.drainRunWorkers()
	rebuilt := <-created
	if len(rebuilt.CompactContextBlocks) != 1 || rebuilt.CompactContextBlocks[0].Content != "keep the original constraint" || rebuilt.CompactedMemoryContext != "retained memory references" || rebuilt.RestoredContextBundleV2 == nil || rebuilt.RestoredContextBundleV2.BoundaryID != "retained-boundary" {
		t.Fatal("new root model lost the passive Agent's compact state")
	}
}

func TestTerminalDeliveryRestoresFromCommittedToolResult(t *testing.T) {
	terminal := agentcore.NewSemanticRecord(agentcore.SemanticKindSubagentTerminal, "thread", "run", "", "", "", map[string]any{"task_id": "task", "agent_id": "child", "status": "completed"})
	result := agentcore.NewSemanticRecord(agentcore.SemanticKindToolResult, "thread", "run", "turn", "call", "", map[string]any{"tool_name": "subagent", "terminal_subagent_task_ids": []string{"task"}, "content": "[TOOL_RESULT_OFFLOADED]"})
	terminal.Seq, result.Seq = 1, 2
	before, err := restoreCollaboration([]agentcore.SemanticRecord{terminal})
	if err != nil || len(before) != 1 || before[0].ResultDelivered {
		t.Fatalf("uncommitted result was consumed: %+v %v", before, err)
	}
	after, err := restoreCollaboration([]agentcore.SemanticRecord{terminal, result})
	if err != nil || len(after) != 1 || !after[0].ResultDelivered {
		t.Fatalf("committed receipt was replayed twice: %+v %v", after, err)
	}
}
