package asynchooks

import (
	"reflect"
	"testing"
	"time"
)

func TestRegisterPendingReturnsCopy(t *testing.T) {
	var registry Registry
	registry.Register(testHook("process-1"))

	pending := registry.Pending()
	if len(pending) != 1 {
		t.Fatalf("expected one pending hook, got %d", len(pending))
	}
	if pending[0].Status != StatusRunning {
		t.Fatalf("expected registered hook to be running, got %q", pending[0].Status)
	}

	pending[0].HookName = "mutated"
	again := registry.Pending()
	if again[0].HookName == "mutated" {
		t.Fatal("expected Pending to return hook copies")
	}
}

func TestCheckResponsesCompletedCases(t *testing.T) {
	tests := []struct {
		name          string
		stdout        string
		stderr        string
		exitCode      int
		wantResponses int
		wantSync      map[string]any
		wantOutcome   Outcome
		wantRemoved   bool
	}{
		{
			name:          "completed JSON response",
			stdout:        "log line\n{\"decision\":\"allow\",\"nested\":{\"ok\":true}}\n",
			stderr:        "warning",
			exitCode:      0,
			wantResponses: 1,
			wantSync:      map[string]any{"decision": "allow", "nested": map[string]any{"ok": true}},
			wantOutcome:   OutcomeSuccess,
		},
		{
			name:          "async JSON ignored",
			stdout:        "{\"async\":true,\"timeout\":1000}\n",
			exitCode:      0,
			wantResponses: 0,
			wantRemoved:   true,
		},
		{
			name:          "empty stdout removed without response",
			stdout:        " \n\t",
			exitCode:      0,
			wantResponses: 0,
			wantRemoved:   true,
		},
		{
			name:          "nonzero exit is error outcome",
			stdout:        "{\"decision\":\"block\"}\n",
			stderr:        "failed",
			exitCode:      2,
			wantResponses: 1,
			wantSync:      map[string]any{"decision": "block"},
			wantOutcome:   OutcomeError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := NewRegistry()
			registry.Register(testHook("process-1"))
			registry.Complete("process-1", tt.stdout, tt.stderr, tt.exitCode)

			responses := registry.CheckResponses()
			if len(responses) != tt.wantResponses {
				t.Fatalf("expected %d responses, got %d", tt.wantResponses, len(responses))
			}
			if tt.wantRemoved {
				if _, ok := registry.hooks["process-1"]; ok {
					t.Fatal("expected hook to be removed")
				}
				return
			}

			response := responses[0]
			if response.ProcessID != "process-1" {
				t.Fatalf("expected process id process-1, got %q", response.ProcessID)
			}
			if response.Stderr != tt.stderr {
				t.Fatalf("expected stderr %q, got %q", tt.stderr, response.Stderr)
			}
			if response.ExitCode != tt.exitCode {
				t.Fatalf("expected exit code %d, got %d", tt.exitCode, response.ExitCode)
			}
			if response.Outcome != tt.wantOutcome {
				t.Fatalf("expected outcome %q, got %q", tt.wantOutcome, response.Outcome)
			}
			if !reflect.DeepEqual(response.SyncResponse, tt.wantSync) {
				t.Fatalf("expected sync response %#v, got %#v", tt.wantSync, response.SyncResponse)
			}
		})
	}
}

func TestCheckResponsesSkipsAlreadyDelivered(t *testing.T) {
	registry := NewRegistry()
	registry.Register(testHook("process-1"))
	registry.Complete("process-1", "{\"decision\":\"allow\"}\n", "", 0)

	first := registry.CheckResponses()
	if len(first) != 1 {
		t.Fatalf("expected first check to return response, got %d", len(first))
	}
	second := registry.CheckResponses()
	if len(second) != 0 {
		t.Fatalf("expected delivered hook to be skipped, got %d responses", len(second))
	}
}

func TestKillRemovesHook(t *testing.T) {
	registry := NewRegistry()
	registry.Register(testHook("process-1"))

	registry.Kill("process-1")

	if pending := registry.Pending(); len(pending) != 0 {
		t.Fatalf("expected no pending hooks after kill, got %d", len(pending))
	}
	if responses := registry.CheckResponses(); len(responses) != 0 {
		t.Fatalf("expected no responses after kill, got %d", len(responses))
	}
}

func TestRemoveDelivered(t *testing.T) {
	registry := NewRegistry()
	registry.Register(testHook("process-1"))
	registry.Complete("process-1", "{\"decision\":\"allow\"}\n", "", 0)
	if responses := registry.CheckResponses(); len(responses) != 1 {
		t.Fatalf("expected one delivered response, got %d", len(responses))
	}

	registry.RemoveDelivered([]string{"process-1"})

	if len(registry.hooks) != 0 {
		t.Fatalf("expected delivered hook to be removed, got %d hooks", len(registry.hooks))
	}
}

func TestRemoveDeliveredDoesNotRemoveRunningHooks(t *testing.T) {
	registry := NewRegistry()
	registry.Register(testHook("process-1"))

	registry.RemoveDelivered([]string{"process-1"})

	if pending := registry.Pending(); len(pending) != 1 {
		t.Fatalf("expected running hook to remain pending, got %d", len(pending))
	}
}

func TestFinalizePendingCancelled(t *testing.T) {
	registry := NewRegistry()
	registry.Register(testHook("process-1"))

	responses := registry.FinalizePending()
	if len(responses) != 1 {
		t.Fatalf("expected one finalized response, got %d", len(responses))
	}
	response := responses[0]
	if response.Outcome != OutcomeCancelled {
		t.Fatalf("expected cancelled outcome, got %q", response.Outcome)
	}
	if response.ExitCode != 1 {
		t.Fatalf("expected cancelled exit code 1, got %d", response.ExitCode)
	}
	if pending := registry.Pending(); len(pending) != 0 {
		t.Fatalf("expected finalize to clear pending hooks, got %d", len(pending))
	}
}

func TestSessionStartResponseInvalidatesSessionEnv(t *testing.T) {
	registry := NewRegistry()
	hook := testHook("process-1")
	hook.HookEvent = "SessionStart"
	registry.Register(hook)
	registry.Complete("process-1", "{\"status\":\"ok\"}\n", "", 0)

	responses := registry.CheckResponses()
	if len(responses) != 1 {
		t.Fatalf("expected one response, got %d", len(responses))
	}
	if !responses[0].InvalidateSessionEnv {
		t.Fatal("expected SessionStart completion to invalidate session env")
	}
}

func TestResponseSyncMapIsCopied(t *testing.T) {
	registry := NewRegistry()
	registry.Register(testHook("process-1"))
	registry.Complete("process-1", "{\"nested\":{\"ok\":true}}\n", "", 0)

	response := registry.CheckResponses()[0]
	response.SyncResponse["nested"].(map[string]any)["ok"] = false

	registry.RemoveDelivered([]string{"process-1"})
	registry.Register(testHook("process-2"))
	registry.Complete("process-2", "{\"nested\":{\"ok\":true}}\n", "", 0)
	next := registry.CheckResponses()[0]
	if next.SyncResponse["nested"].(map[string]any)["ok"] != true {
		t.Fatal("expected returned sync response maps to be independent")
	}
}

func testHook(processID string) Hook {
	return Hook{
		ProcessID: processID,
		HookID:    "hook-" + processID,
		HookName:  "TestHook",
		HookEvent: "PreToolUse",
		Command:   "echo hook",
		Timeout:   15 * time.Second,
		ToolName:  "shell",
		PluginID:  "plugin-1",
	}
}
