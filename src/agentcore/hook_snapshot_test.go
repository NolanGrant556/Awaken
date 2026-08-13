package agentcore

import (
	"context"
	"testing"

	"awaken/src/hookruntime"
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
