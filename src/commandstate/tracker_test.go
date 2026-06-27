package commandstate

import (
	"errors"
	"reflect"
	"testing"
)

func TestTrackerStartNotifiesListener(t *testing.T) {
	tracker := NewTracker()
	var events []Event
	tracker.SetListener(func(event Event) {
		if event.Metadata != nil {
			event.Metadata["listener"] = "mutated"
		}
		events = append(events, event)
	})

	err := tracker.Start("command-1", Metadata{"tool": "shell"})
	if err != nil {
		t.Fatalf("expected start to succeed, got %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	event := events[0]
	if event.UUID != "command-1" {
		t.Fatalf("expected event uuid command-1, got %q", event.UUID)
	}
	if event.State != StateStarted {
		t.Fatalf("expected state %q, got %q", StateStarted, event.State)
	}
	if event.Time.IsZero() {
		t.Fatal("expected event time to be set")
	}
	if event.Metadata["tool"] != "shell" {
		t.Fatalf("expected event metadata to include tool=shell, got %v", event.Metadata)
	}

	snapshot := tracker.Snapshot()
	command := snapshot["command-1"]
	if command.State != StateStarted {
		t.Fatalf("expected snapshot state %q, got %q", StateStarted, command.State)
	}
	if command.StartedAt.IsZero() {
		t.Fatal("expected snapshot started time to be set")
	}
	if !command.CompletedAt.IsZero() {
		t.Fatalf("expected snapshot completed time to be zero, got %v", command.CompletedAt)
	}
	if _, exists := command.Metadata["listener"]; exists {
		t.Fatalf("listener mutation leaked into snapshot metadata: %v", command.Metadata)
	}
}

func TestTrackerCompleteNotifiesListener(t *testing.T) {
	tracker := NewTracker()
	if err := tracker.Start("command-1", Metadata{"tool": "shell"}); err != nil {
		t.Fatalf("expected start to succeed, got %v", err)
	}

	var events []Event
	tracker.SetListener(func(event Event) {
		events = append(events, event)
	})

	err := tracker.Complete("command-1", Metadata{"exit_code": "0"})
	if err != nil {
		t.Fatalf("expected complete to succeed, got %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	event := events[0]
	if event.UUID != "command-1" {
		t.Fatalf("expected event uuid command-1, got %q", event.UUID)
	}
	if event.State != StateCompleted {
		t.Fatalf("expected state %q, got %q", StateCompleted, event.State)
	}
	if event.Time.IsZero() {
		t.Fatal("expected event time to be set")
	}
	if !reflect.DeepEqual(event.Metadata, Metadata{"exit_code": "0"}) {
		t.Fatalf("expected transition metadata only, got %v", event.Metadata)
	}

	command := tracker.Snapshot()["command-1"]
	if command.State != StateCompleted {
		t.Fatalf("expected snapshot state %q, got %q", StateCompleted, command.State)
	}
	if command.StartedAt.IsZero() {
		t.Fatal("expected snapshot started time to remain set")
	}
	if command.CompletedAt.IsZero() {
		t.Fatal("expected snapshot completed time to be set")
	}
	expectedMetadata := Metadata{"tool": "shell", "exit_code": "0"}
	if !reflect.DeepEqual(command.Metadata, expectedMetadata) {
		t.Fatalf("expected merged snapshot metadata %v, got %v", expectedMetadata, command.Metadata)
	}
}

func TestTrackerRejectsCompleteBeforeStart(t *testing.T) {
	tracker := NewTracker()
	var events []Event
	tracker.SetListener(func(event Event) {
		events = append(events, event)
	})

	err := tracker.Complete("command-1", Metadata{"exit_code": "1"})
	if !errors.Is(err, ErrCommandNotStarted) {
		t.Fatalf("expected ErrCommandNotStarted, got %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected no events, got %d", len(events))
	}
	if len(tracker.Snapshot()) != 0 {
		t.Fatalf("expected empty snapshot, got %v", tracker.Snapshot())
	}
}

func TestTrackerRejectsRepeatedTransitions(t *testing.T) {
	tests := []struct {
		name            string
		run             func(*testing.T, *Tracker) error
		wantErr         error
		wantEventCount  int
		wantFinalState  State
		wantSnapshotLen int
	}{
		{
			name: "duplicate start while started",
			run: func(t *testing.T, tracker *Tracker) error {
				if err := tracker.Start("command-1", nil); err != nil {
					t.Fatalf("setup start failed: %v", err)
				}
				return tracker.Start("command-1", nil)
			},
			wantErr:         ErrCommandAlreadyStarted,
			wantEventCount:  1,
			wantFinalState:  StateStarted,
			wantSnapshotLen: 1,
		},
		{
			name: "start after completed",
			run: func(t *testing.T, tracker *Tracker) error {
				if err := tracker.Start("command-1", nil); err != nil {
					t.Fatalf("setup start failed: %v", err)
				}
				if err := tracker.Complete("command-1", nil); err != nil {
					t.Fatalf("setup complete failed: %v", err)
				}
				return tracker.Start("command-1", nil)
			},
			wantErr:         ErrCommandAlreadyCompleted,
			wantEventCount:  2,
			wantFinalState:  StateCompleted,
			wantSnapshotLen: 1,
		},
		{
			name: "duplicate complete",
			run: func(t *testing.T, tracker *Tracker) error {
				if err := tracker.Start("command-1", nil); err != nil {
					t.Fatalf("setup start failed: %v", err)
				}
				if err := tracker.Complete("command-1", nil); err != nil {
					t.Fatalf("setup complete failed: %v", err)
				}
				return tracker.Complete("command-1", nil)
			},
			wantErr:         ErrCommandAlreadyCompleted,
			wantEventCount:  2,
			wantFinalState:  StateCompleted,
			wantSnapshotLen: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tracker := NewTracker()
			eventCount := 0
			tracker.SetListener(func(event Event) {
				eventCount++
			})

			err := test.run(t, tracker)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("expected %v, got %v", test.wantErr, err)
			}

			snapshot := tracker.Snapshot()
			if eventCount != test.wantEventCount {
				t.Fatalf("expected %d events, got %d", test.wantEventCount, eventCount)
			}
			if len(snapshot) != test.wantSnapshotLen {
				t.Fatalf("expected %d snapshot entries, got %d", test.wantSnapshotLen, len(snapshot))
			}
			if snapshot["command-1"].State != test.wantFinalState {
				t.Fatalf("expected final state %q, got %q", test.wantFinalState, snapshot["command-1"].State)
			}
		})
	}
}

func TestTrackerCopiesMetadataAndSnapshot(t *testing.T) {
	tracker := NewTracker()
	startMetadata := Metadata{"tool": "shell"}
	completeMetadata := Metadata{"exit_code": "0"}

	if err := tracker.Start("command-1", startMetadata); err != nil {
		t.Fatalf("expected start to succeed, got %v", err)
	}
	if err := tracker.Complete("command-1", completeMetadata); err != nil {
		t.Fatalf("expected complete to succeed, got %v", err)
	}

	startMetadata["tool"] = "mutated"
	startMetadata["leak"] = "true"
	completeMetadata["exit_code"] = "mutated"

	snapshot := tracker.Snapshot()
	command := snapshot["command-1"]
	expectedMetadata := Metadata{"tool": "shell", "exit_code": "0"}
	if !reflect.DeepEqual(command.Metadata, expectedMetadata) {
		t.Fatalf("expected copied metadata %v, got %v", expectedMetadata, command.Metadata)
	}

	command.Metadata["tool"] = "snapshot-mutated"
	command.Metadata["snapshot"] = "leak"

	nextSnapshot := tracker.Snapshot()
	if !reflect.DeepEqual(nextSnapshot["command-1"].Metadata, expectedMetadata) {
		t.Fatalf("snapshot mutation leaked into tracker metadata: %v", nextSnapshot["command-1"].Metadata)
	}

	delete(snapshot, "command-1")
	if len(tracker.Snapshot()) != 1 {
		t.Fatalf("snapshot map mutation leaked into tracker")
	}
}
