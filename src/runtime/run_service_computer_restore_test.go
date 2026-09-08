package runtime

import (
	"awaken/src/agentcore"
	"awaken/src/computer"
	"awaken/src/computer/fake"
	"context"
	"testing"
)

func TestRunServiceBindsComputerForNewContinuedAndRestoredThreads(t *testing.T) {
	t.Setenv(awakenHomeEnv, t.TempDir())
	workspace := WorkspaceConfig{WorkspaceRoot: t.TempDir()}
	profile := &ModelProfileConfig{ProfileID: "review-fixture", SupportsVision: true}
	newService := func(calls *int) *RunService {
		s := NewRunServiceWithModelProfiles(func(w WorkspaceConfig, id string, p *ModelProfileConfig) *agentcore.Agent {
			return agentcore.NewAgentWithThreadID(agentcore.AgentConfig{Workspace: agentcore.WorkspaceConfig{Root: w.WorkspaceRoot}}, id)
		}, func(a *agentcore.Agent) Executor {
			return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
				a.GetConversation().UserInput(input)
				a.GetConversation().AssistantOutput("done", nil)
				if err := persistTestRunFacts(ctx, a, input, "done"); err != nil {
					return "", err
				}
				return "done", nil
			})
		})
		s.SetComputerSettings(computer.Settings{Enabled: true})
		s.SetComputerDriverFactory(func(computer.Binding) (computer.Driver, error) { *calls++; return fake.NewDriver(), nil })
		return s
	}
	firstCalls := 0
	first := newService(&firstCalls)
	run, err := first.StartRun(StartRunRequest{Input: "first", Title: "review fixture", Workspace: workspace, ModelProfile: profile})
	if err != nil {
		t.Fatal(err)
	}
	waitForRunTerminal(t, first, run.RunID)
	first.drainRunWorkers()
	if firstCalls != 1 {
		t.Fatalf("fresh run factory calls: %d", firstCalls)
	}
	if cr, err := first.ComputerRuntime(run.RunID); err != nil || cr == nil {
		t.Fatalf("fresh run binding: %v", err)
	}
	continued, err := first.StartRun(StartRunRequest{Input: "continue in memory", ThreadID: run.ThreadID, Workspace: workspace, ModelProfile: profile})
	if err != nil {
		t.Fatal(err)
	}
	waitForRunTerminal(t, first, continued.RunID)
	first.drainRunWorkers()
	if firstCalls != 2 {
		t.Fatalf("continued run factory calls: %d", firstCalls)
	}
	if cr, err := first.ComputerRuntime(continued.RunID); err != nil || cr == nil {
		t.Fatalf("continued run binding: %v", err)
	}
	if err := first.journals.CloseThread(run.ThreadID); err != nil {
		t.Fatal(err)
	}
	restoredCalls := 0
	restored := newService(&restoredCalls)
	next, err := restored.StartRun(StartRunRequest{Input: "follow up", ThreadID: run.ThreadID, Workspace: workspace, ModelProfile: profile})
	if err != nil {
		t.Fatal(err)
	}
	waitForRunTerminal(t, restored, next.RunID)
	restored.drainRunWorkers()
	defer restored.journals.CloseThread(next.ThreadID)
	cr, bindingErr := restored.ComputerRuntime(next.RunID)
	t.Logf("fresh factory calls=%d; restored factory calls=%d; restored has binding=%v; error=%v", firstCalls, restoredCalls, cr != nil, bindingErr)
	if restoredCalls != 1 || cr == nil || bindingErr != nil {
		t.Fatal("restored run skipped Computer runtime binding")
	}
}
