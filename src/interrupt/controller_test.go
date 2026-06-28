package interrupt

import (
	"errors"
	"testing"
)

func TestControllerStartInterruptComplete(t *testing.T) {
	controller := NewController()
	var events []Event
	controller.AddListener(func(event Event) {
		events = append(events, event)
	})

	snapshot, err := controller.StartRun("run-1")
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}
	if snapshot.RunID != "run-1" || snapshot.State != StateRunning {
		t.Fatalf("unexpected start snapshot: %+v", snapshot)
	}

	snapshot, err = controller.Interrupt("user pressed escape")
	if err != nil {
		t.Fatalf("Interrupt returned error: %v", err)
	}
	if snapshot.State != StateInterrupted || snapshot.Reason != "user pressed escape" {
		t.Fatalf("unexpected interrupt snapshot: %+v", snapshot)
	}

	snapshot, err = controller.CompleteRun()
	if err != nil {
		t.Fatalf("CompleteRun returned error: %v", err)
	}
	if snapshot.State != StateCompleted || snapshot.Reason != "" {
		t.Fatalf("unexpected complete snapshot: %+v", snapshot)
	}

	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
	want := []struct {
		kind   EventKind
		state  State
		reason string
	}{
		{kind: EventRunStarted, state: StateRunning},
		{kind: EventInterrupted, state: StateInterrupted, reason: "user pressed escape"},
		{kind: EventRunCompleted, state: StateCompleted},
	}
	for i, expected := range want {
		event := events[i]
		if event.Kind != expected.kind || event.RunID != "run-1" || event.State != expected.state || event.Reason != expected.reason {
			t.Fatalf("event %d mismatch: got %+v want %+v", i, event, expected)
		}
		if event.Time.IsZero() {
			t.Fatalf("event %d has zero time", i)
		}
	}
}

func TestControllerGuardedModesDoNotInterrupt(t *testing.T) {
	tests := []struct {
		name    string
		options Options
		wantErr error
	}{
		{
			name:    "viewer only",
			options: Options{ViewerOnly: true},
			wantErr: ErrViewerOnly,
		},
		{
			name:    "read only",
			options: Options{ReadOnly: true},
			wantErr: ErrReadOnly,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			controller := NewControllerWithOptions(tt.options)
			var events []Event
			controller.AddListener(func(event Event) {
				events = append(events, event)
			})

			if _, err := controller.StartRun("run-1"); err != nil {
				t.Fatalf("StartRun returned error: %v", err)
			}

			snapshot, err := controller.Interrupt("stop")
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Interrupt error = %v, want %v", err, tt.wantErr)
			}
			if snapshot.State != StateRunning {
				t.Fatalf("expected state to remain running, got %+v", snapshot)
			}

			if got := controller.State(); got != StateRunning {
				t.Fatalf("State() = %s, want %s", got, StateRunning)
			}
			if len(events) != 2 {
				t.Fatalf("expected start and ignored interrupt events, got %d", len(events))
			}
			ignored := events[1]
			if ignored.Kind != EventInterruptIgnored || ignored.State != StateRunning || !errors.Is(ignored.Err, tt.wantErr) {
				t.Fatalf("unexpected ignored event: %+v", ignored)
			}
		})
	}
}

func TestControllerRepeatedTransitions(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(*Controller)
		action  func(*Controller) error
		wantErr error
	}{
		{
			name: "start while running",
			setup: func(controller *Controller) {
				mustStart(t, controller, "run-1")
			},
			action: func(controller *Controller) error {
				_, err := controller.StartRun("run-2")
				return err
			},
			wantErr: ErrRunAlreadyActive,
		},
		{
			name: "start while interrupted",
			setup: func(controller *Controller) {
				mustStart(t, controller, "run-1")
				mustInterrupt(t, controller, "stop")
			},
			action: func(controller *Controller) error {
				_, err := controller.StartRun("run-2")
				return err
			},
			wantErr: ErrRunAlreadyActive,
		},
		{
			name: "interrupt while idle",
			action: func(controller *Controller) error {
				_, err := controller.Interrupt("stop")
				return err
			},
			wantErr: ErrNoActiveRun,
		},
		{
			name: "interrupt while interrupted",
			setup: func(controller *Controller) {
				mustStart(t, controller, "run-1")
				mustInterrupt(t, controller, "stop")
			},
			action: func(controller *Controller) error {
				_, err := controller.Interrupt("stop again")
				return err
			},
			wantErr: ErrAlreadyInterrupted,
		},
		{
			name: "interrupt after complete",
			setup: func(controller *Controller) {
				mustStart(t, controller, "run-1")
				mustComplete(t, controller)
			},
			action: func(controller *Controller) error {
				_, err := controller.Interrupt("stop")
				return err
			},
			wantErr: ErrRunCompleted,
		},
		{
			name: "complete while idle",
			action: func(controller *Controller) error {
				_, err := controller.CompleteRun()
				return err
			},
			wantErr: ErrNoActiveRun,
		},
		{
			name: "complete after complete",
			setup: func(controller *Controller) {
				mustStart(t, controller, "run-1")
				mustComplete(t, controller)
			},
			action: func(controller *Controller) error {
				_, err := controller.CompleteRun()
				return err
			},
			wantErr: ErrRunCompleted,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			controller := NewController()
			if tt.setup != nil {
				tt.setup(controller)
			}

			err := tt.action(controller)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestControllerPendingPermissionCancel(t *testing.T) {
	controller := NewController()
	var events []Event
	controller.AddListener(func(event Event) {
		events = append(events, event)
	})
	mustStart(t, controller, "run-1")

	if err := controller.AddPermission("req-1", "tool-1"); err != nil {
		t.Fatalf("AddPermission returned error: %v", err)
	}
	if err := controller.AddPermission("req-1", "tool-2"); !errors.Is(err, ErrPermissionExists) {
		t.Fatalf("duplicate AddPermission error = %v, want %v", err, ErrPermissionExists)
	}

	toolUseID, err := controller.CancelPermission("req-1")
	if err != nil {
		t.Fatalf("CancelPermission returned error: %v", err)
	}
	if toolUseID != "tool-1" {
		t.Fatalf("toolUseID = %q, want %q", toolUseID, "tool-1")
	}

	snapshot := controller.Snapshot()
	if _, ok := snapshot.PendingPermissions["req-1"]; ok {
		t.Fatalf("expected req-1 to be removed from pending permissions")
	}
	if _, err := controller.CancelPermission("req-1"); !errors.Is(err, ErrPermissionNotFound) {
		t.Fatalf("unknown CancelPermission error = %v, want %v", err, ErrPermissionNotFound)
	}

	if len(events) != 2 {
		t.Fatalf("expected start and permission cancel events, got %d", len(events))
	}
	cancelled := events[1]
	if cancelled.Kind != EventPermissionCancelled || cancelled.RunID != "run-1" || cancelled.State != StateRunning {
		t.Fatalf("unexpected permission cancel event: %+v", cancelled)
	}
	if cancelled.RequestID != "req-1" || cancelled.ToolUseID != "tool-1" {
		t.Fatalf("unexpected permission cancel ids: %+v", cancelled)
	}
}

func TestControllerSnapshotReturnsCopy(t *testing.T) {
	controller := NewController()
	mustStart(t, controller, "run-1")
	if err := controller.AddPermission("req-1", "tool-1"); err != nil {
		t.Fatalf("AddPermission returned error: %v", err)
	}

	snapshot := controller.Snapshot()
	snapshot.PendingPermissions["req-1"] = "changed"
	snapshot.PendingPermissions["req-2"] = "tool-2"

	next := controller.Snapshot()
	if next.PendingPermissions["req-1"] != "tool-1" {
		t.Fatalf("snapshot mutation leaked into controller: %+v", next.PendingPermissions)
	}
	if _, ok := next.PendingPermissions["req-2"]; ok {
		t.Fatalf("snapshot added permission leaked into controller: %+v", next.PendingPermissions)
	}
}

func mustStart(t *testing.T, controller *Controller, runID string) {
	t.Helper()
	if _, err := controller.StartRun(runID); err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}
}

func mustInterrupt(t *testing.T, controller *Controller, reason string) {
	t.Helper()
	if _, err := controller.Interrupt(reason); err != nil {
		t.Fatalf("Interrupt returned error: %v", err)
	}
}

func mustComplete(t *testing.T, controller *Controller) {
	t.Helper()
	if _, err := controller.CompleteRun(); err != nil {
		t.Fatalf("CompleteRun returned error: %v", err)
	}
}
