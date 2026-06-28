package sessionstate

import "testing"

func TestReporterDefaultStateIsIdle(t *testing.T) {
	reporter := NewReporter()

	if reporter.State() != StateIdle {
		t.Fatalf("expected default state %q, got %q", StateIdle, reporter.State())
	}
}

func TestReporterNotifiesRunningState(t *testing.T) {
	reporter := NewReporter()

	var stateCalls int
	var gotState State
	var gotDetails *RequiresActionDetails
	var metadataCalls int

	reporter.SetStateListener(func(state State, details *RequiresActionDetails) {
		stateCalls++
		gotState = state
		gotDetails = details
	})
	reporter.SetMetadataListener(func(Metadata) {
		metadataCalls++
	})

	reporter.Transition(StateRunning, nil)

	if reporter.State() != StateRunning {
		t.Fatalf("expected current state %q, got %q", StateRunning, reporter.State())
	}
	if stateCalls != 1 {
		t.Fatalf("expected 1 state call, got %d", stateCalls)
	}
	if gotState != StateRunning {
		t.Fatalf("expected state listener to receive %q, got %q", StateRunning, gotState)
	}
	if gotDetails != nil {
		t.Fatalf("expected nil running details, got %#v", gotDetails)
	}
	if metadataCalls != 0 {
		t.Fatalf("expected no metadata calls for running transition, got %d", metadataCalls)
	}
}

func TestReporterMetadataTransitions(t *testing.T) {
	details := &RequiresActionDetails{
		ToolName:          "apply_patch",
		ActionDescription: "Editing README.md",
		ToolUseID:         "tool-1",
		RequestID:         "request-1",
		Input:             map[string]any{"path": "README.md"},
	}

	tests := []struct {
		name        string
		transitions []transition
		want        []wantMetadata
	}{
		{
			name: "requires_action sends pending action",
			transitions: []transition{
				{state: StateRequiresAction, details: details},
			},
			want: []wantMetadata{
				{pendingChanged: true, pendingTool: "apply_patch"},
			},
		},
		{
			name: "leaving requires_action for running clears pending action",
			transitions: []transition{
				{state: StateRequiresAction, details: details},
				{state: StateRunning},
			},
			want: []wantMetadata{
				{pendingChanged: true, pendingTool: "apply_patch"},
				{pendingChanged: true},
			},
		},
		{
			name: "leaving requires_action for idle clears pending action and task summary",
			transitions: []transition{
				{state: StateRequiresAction, details: details},
				{state: StateIdle},
			},
			want: []wantMetadata{
				{pendingChanged: true, pendingTool: "apply_patch"},
				{pendingChanged: true},
				{taskSummaryChanged: true},
			},
		},
		{
			name: "idle clears task summary",
			transitions: []transition{
				{state: StateRunning},
				{state: StateIdle},
			},
			want: []wantMetadata{
				{taskSummaryChanged: true},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reporter := NewReporter()
			var got []Metadata
			reporter.SetMetadataListener(func(metadata Metadata) {
				got = append(got, metadata)
			})

			for _, transition := range test.transitions {
				reporter.Transition(transition.state, transition.details)
			}

			assertMetadataEvents(t, got, test.want)
		})
	}
}

func TestReporterCopiesDetailsInput(t *testing.T) {
	reporter := NewReporter()
	input := map[string]any{
		"path":   "README.md",
		"nested": map[string]any{"answer": "yes"},
		"items":  []any{"one"},
	}
	details := &RequiresActionDetails{
		ToolName:          "file_read",
		ActionDescription: "Reading README.md",
		ToolUseID:         "tool-1",
		RequestID:         "request-1",
		Input:             input,
	}

	var stateDetails *RequiresActionDetails
	var pendingAction *RequiresActionDetails
	reporter.SetStateListener(func(_ State, details *RequiresActionDetails) {
		stateDetails = details
	})
	reporter.SetMetadataListener(func(metadata Metadata) {
		if metadata.PendingActionChanged && metadata.PendingAction != nil {
			pendingAction = metadata.PendingAction
		}
	})

	reporter.Transition(StateRequiresAction, details)

	if stateDetails == nil {
		t.Fatal("expected state listener to receive details")
	}
	if pendingAction == nil {
		t.Fatal("expected metadata listener to receive pending action")
	}

	details.Input["path"] = "changed.md"
	details.Input["new"] = "value"
	details.Input["nested"].(map[string]any)["answer"] = "no"
	details.Input["items"].([]any)[0] = "two"

	if stateDetails.Input["path"] != "README.md" {
		t.Fatalf("state details input was modified by caller: %#v", stateDetails.Input)
	}
	if stateDetails.Input["new"] != nil {
		t.Fatalf("state details input includes caller-added key: %#v", stateDetails.Input)
	}
	if stateDetails.Input["nested"].(map[string]any)["answer"] != "yes" {
		t.Fatalf("state details nested input was modified by caller: %#v", stateDetails.Input)
	}
	if stateDetails.Input["items"].([]any)[0] != "one" {
		t.Fatalf("state details slice input was modified by caller: %#v", stateDetails.Input)
	}

	stateDetails.Input["path"] = "listener-mutated.md"
	if pendingAction.Input["path"] != "README.md" {
		t.Fatalf("metadata pending action shared input with state listener: %#v", pendingAction.Input)
	}
}

type transition struct {
	state   State
	details *RequiresActionDetails
}

type wantMetadata struct {
	pendingChanged     bool
	pendingTool        string
	taskSummaryChanged bool
}

func assertMetadataEvents(t *testing.T, got []Metadata, want []wantMetadata) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("expected %d metadata events, got %d: %#v", len(want), len(got), got)
	}

	for i := range want {
		if got[i].PendingActionChanged != want[i].pendingChanged {
			t.Fatalf("event %d: expected pending changed %v, got %v", i, want[i].pendingChanged, got[i].PendingActionChanged)
		}
		if got[i].TaskSummaryChanged != want[i].taskSummaryChanged {
			t.Fatalf("event %d: expected task summary changed %v, got %v", i, want[i].taskSummaryChanged, got[i].TaskSummaryChanged)
		}
		if want[i].pendingTool == "" {
			if got[i].PendingAction != nil {
				t.Fatalf("event %d: expected nil pending action, got %#v", i, got[i].PendingAction)
			}
		} else {
			if got[i].PendingAction == nil {
				t.Fatalf("event %d: expected pending action", i)
			}
			if got[i].PendingAction.ToolName != want[i].pendingTool {
				t.Fatalf("event %d: expected pending tool %q, got %q", i, want[i].pendingTool, got[i].PendingAction.ToolName)
			}
		}
		if got[i].TaskSummary != nil {
			t.Fatalf("event %d: expected nil task summary, got %q", i, *got[i].TaskSummary)
		}
	}
}
