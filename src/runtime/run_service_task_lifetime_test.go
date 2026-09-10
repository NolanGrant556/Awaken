package runtime

import (
	"awaken/src/agentcore"
	"awaken/src/subagentruntime"
	"context"
	"testing"
	"time"
)

func TestCancelRunWaitsForChildExitBeforePublishingTerminal(t *testing.T) {
	t.Setenv("AWAKEN_HOME", t.TempDir())
	parentContext := make(chan context.Context, 1)
	childStarted, childCancelled, allowExit := make(chan struct{}), make(chan struct{}), make(chan struct{})
	defer func() {
		select {
		case <-allowExit:
		default:
			close(allowExit)
		}
	}()
	service := NewRunService(func(w WorkspaceConfig, id string) *agentcore.Agent {
		return agentcore.NewAgentWithThreadID(agentcore.AgentConfig{}, id)
	}, func(*agentcore.Agent) Executor {
		return ExecutorFunc(func(ctx context.Context, input string, _ agentcore.EventSink) (string, error) {
			if input == "next" {
				return "next done", nil
			}
			parentContext <- ctx
			<-ctx.Done()
			return "", ctx.Err()
		})
	})
	workspace := WorkspaceConfig{WorkspaceRoot: t.TempDir()}
	run, err := service.StartRun(StartRunRequest{Input: "parent", Workspace: workspace})
	if err != nil {
		t.Fatal(err)
	}
	ctx := <-parentContext
	session, _ := service.session(run.RunID)
	group := session.SubagentController().(*subagentruntime.Manager)
	controller := group.Bind(ctx, "root", 0, subagentruntime.Config{RunID: run.RunID, ThreadID: run.ThreadID, Starter: func(ctx context.Context, _ subagentruntime.StartRequest) (string, error) {
		close(childStarted)
		<-ctx.Done()
		close(childCancelled)
		<-allowExit
		return "", ctx.Err()
	}})
	child, err := controller.Spawn(ctx, subagentruntime.SpawnRequest{Description: "slow cleanup", Prompt: "wait", Background: true})
	if err != nil {
		t.Fatal(err)
	}
	<-childStarted
	if _, err := service.CancelRun(run.RunID); err != nil {
		t.Fatal(err)
	}
	<-childCancelled
	time.Sleep(20 * time.Millisecond)
	pending, _ := service.GetRun(run.RunID)
	if pending.Status != RunStatusCancelling {
		t.Fatalf("root terminal before child exit: %s", pending.Status)
	}
	if !group.HasLiveWork(run.RunID) {
		t.Fatal("actual child disappeared")
	}
	if _, err := service.StartRun(StartRunRequest{Input: "next", ThreadID: run.ThreadID, Workspace: workspace}); err == nil {
		t.Fatal("new run started while stopping")
	}
	close(allowExit)
	waitForRunTerminal(t, service, run.RunID)
	service.drainRunWorkers()
	if got, _ := group.Status(child.TaskID); got.Status != subagentruntime.StatusCancelled {
		t.Fatal(got.Status)
	}
	if group.HasLiveWork(run.RunID) {
		t.Fatal("root finished with live descendants")
	}
	next, err := service.StartRun(StartRunRequest{Input: "next", ThreadID: run.ThreadID, Workspace: workspace})
	if err != nil {
		t.Fatal(err)
	}
	waitForRunTerminal(t, service, next.RunID)
	service.drainRunWorkers()
}
