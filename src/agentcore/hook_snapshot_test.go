package agentcore

import (
	"context"
	"testing"

	"awaken/src/hookruntime"
	"awaken/src/tool/profile"
)

func TestPersistHookSnapshotIsRedacted(t *testing.T) {
	recorder := &forkCaptureRecorder{}
	runner := &Runner{
		threadID:  "thread-hook",
		runID:     "run-hook",
		sessionID: "session-hook",
		recorder:  recorder,
		config: AgentConfig{HookAvailabilitySnapshot: &hookruntime.AvailabilitySnapshot{
			SnapshotID: "snapshot-hook", SchemaVersion: 1,
			Sources: []hookruntime.SourceReceipt{{Kind: hookruntime.SourceProject, Path: "/secret/project/.awaken/hooks.json", Digest: "digest", Status: hookruntime.StatusEnabled, HookCount: 1}},
			HooksByEvent: map[hookruntime.Event][]hookruntime.CompiledHook{
				hookruntime.EventRunStart: {{Command: hookruntime.CommandHook{Command: []string{"/bin/sh", "-c", "secret command"}}}},
			},
		}},
	}
	if err := runner.persistHookSnapshot(context.Background()); err != nil {
		t.Fatal(err)
	}
	records := recorder.snapshot()
	if len(records) != 1 || records[0].Kind != SemanticKindHookSnapshot {
		t.Fatalf("records = %+v", records)
	}
	if got := records[0].Payload["enabled_hook_count"]; got != 1 {
		t.Fatalf("enabled_hook_count = %v", got)
	}
	sources, ok := records[0].Payload["sources"].([]map[string]any)
	if !ok || len(sources) != 1 {
		t.Fatalf("sources = %#v", records[0].Payload["sources"])
	}
	if _, exposed := sources[0]["path"]; exposed {
		t.Fatal("hook source path leaked into journal receipt")
	}
	if _, exposed := records[0].Payload["command"]; exposed {
		t.Fatal("hook command leaked into journal receipt")
	}
}

func TestChildHookRecordsKeepAgentAndAttempt(t *testing.T) {
	recorder := &forkCaptureRecorder{}
	snapshot := hookruntime.EmptySnapshot()
	runner := &Runner{threadID: "thread", runID: "run", recorder: recorder, config: AgentConfig{
		HookAvailabilitySnapshot: &snapshot, SubagentOrigin: &profile.SubagentOrigin{AgentID: "child", ParentAgentID: "parent"}, SubagentTaskID: "assignment", AttemptID: "attempt",
	}}
	if err := runner.persistHookSnapshot(context.Background()); err != nil {
		t.Fatal(err)
	}
	hooks := runner.hookRecorder(nil, "subject-task")
	if err := hooks.InvocationStarted(context.Background(), hookruntime.InputEnvelope{InvocationID: "hook-call"}, hookruntime.CompiledHook{}); err != nil {
		t.Fatal(err)
	}
	if err := hooks.InvocationCompleted(context.Background(), hookruntime.Outcome{InvocationID: "hook-call"}); err != nil {
		t.Fatal(err)
	}
	for _, record := range recorder.snapshot() {
		if record.Payload["agent_id"] != "child" || record.Payload["parent_agent_id"] != "parent" || record.Payload["subagent_task_id"] != "assignment" || record.Payload["attempt_id"] != "attempt" {
			t.Fatalf("hook record lost its execution owner: %#v", record.Payload)
		}
	}
}
