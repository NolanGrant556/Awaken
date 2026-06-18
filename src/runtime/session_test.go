package runtime

import (
	"context"
	"errors"
	"testing"
	"time"

	"awaken/src/agentcore"
)

func TestSessionAwaitApprovalTransitionsStatus(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	session := NewSession("run-1", "thread-1", cancel)

	request := agentcore.ApprovalRequest{
		ID:          "approval-1",
		ToolCallID:  "tool-1",
		ToolName:    "write_file",
		Title:       "Approve write_file",
		Description: "Write README.md",
		RiskTone:    agentcore.ApprovalRiskWarning,
	}

	decisionCh := make(chan agentcore.ApprovalDecision, 1)
	errCh := make(chan error, 1)
	go func() {
		decision, err := session.AwaitApproval(ctx, request)
		if err != nil {
			errCh <- err
			return
		}
		decisionCh <- decision
	}()

	deadline := time.Now().Add(2 * time.Second)
	for {
		if session.Snapshot().Status == RunStatusAwaitingApproval {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for awaiting_approval status")
		}
		time.Sleep(10 * time.Millisecond)
	}

	if err := session.ResolveApproval("approval-1", agentcore.ApprovalDecisionApproved); err != nil {
		t.Fatalf("ResolveApproval returned error: %v", err)
	}

	select {
	case decision := <-decisionCh:
		if decision != agentcore.ApprovalDecisionApproved {
			t.Fatalf("expected approved decision, got %s", decision)
		}
	case err := <-errCh:
		t.Fatalf("AwaitApproval returned error: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for approval decision")
	}

	if session.Snapshot().Status != RunStatusRunning {
		t.Fatalf("expected running status after resolve, got %s", session.Snapshot().Status)
	}

	sub := session.Subscribe()
	if sub.Close != nil {
		defer sub.Close()
	}

	var statusChanges []Event
	for _, event := range sub.History {
		if event.Kind == EventKindRunStatusChanged {
			statusChanges = append(statusChanges, event)
		}
	}

	if len(statusChanges) != 2 {
		t.Fatalf("expected 2 RUN_STATUS_CHANGED events, got %+v", statusChanges)
	}
	if statusChanges[0].Status != string(RunStatusAwaitingApproval) {
		t.Fatalf("expected first status awaiting_approval, got %s", statusChanges[0].Status)
	}
	if statusChanges[1].Status != string(RunStatusRunning) {
		t.Fatalf("expected second status running, got %s", statusChanges[1].Status)
	}
}

func TestSessionAwaitApprovalAtomicWithCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	session := NewSession("run-1", "thread-1", cancel)
	request := agentcore.ApprovalRequest{
		ID:          "approval-1",
		ToolCallID:  "tool-1",
		ToolName:    "write_file",
		Title:       "Approve write_file",
		Description: "Write README.md",
		RiskTone:    agentcore.ApprovalRiskWarning,
	}

	errCh := make(chan error, 1)
	go func() {
		_, err := session.AwaitApproval(ctx, request)
		errCh <- err
	}()

	deadline := time.Now().Add(2 * time.Second)
	for {
		if session.Snapshot().Status == RunStatusAwaitingApproval {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for awaiting_approval status")
		}
		time.Sleep(10 * time.Millisecond)
	}

	session.RequestCancel()

	if session.Snapshot().Status != RunStatusCancelling {
		t.Fatalf("expected cancelling status after cancel, got %s", session.Snapshot().Status)
	}

	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for AwaitApproval to return")
	}

	sub := session.Subscribe()
	if sub.Close != nil {
		defer sub.Close()
	}

	var statusChanges []Event
	for _, event := range sub.History {
		if event.Kind == EventKindRunStatusChanged {
			statusChanges = append(statusChanges, event)
		}
	}

	if len(statusChanges) != 1 {
		t.Fatalf("expected 1 RUN_STATUS_CHANGED event, got %+v", statusChanges)
	}
	if statusChanges[0].Status != string(RunStatusAwaitingApproval) {
		t.Fatalf("expected status awaiting_approval, got %s", statusChanges[0].Status)
	}
}

func TestSessionResolveApprovalAfterCancelReturnsCancelling(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	session := NewSession("run-1", "thread-1", cancel)
	request := agentcore.ApprovalRequest{
		ID:          "approval-1",
		ToolCallID:  "tool-1",
		ToolName:    "write_file",
		Title:       "Approve write_file",
		Description: "Write README.md",
		RiskTone:    agentcore.ApprovalRiskWarning,
	}

	errCh := make(chan error, 1)
	go func() {
		_, err := session.AwaitApproval(ctx, request)
		errCh <- err
	}()

	deadline := time.Now().Add(2 * time.Second)
	for {
		if session.Snapshot().Status == RunStatusAwaitingApproval {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for awaiting_approval status")
		}
		time.Sleep(10 * time.Millisecond)
	}

	session.RequestCancel()

	// Resolve while the run is cancelling. The approval is still in the map
	// because ResolveApproval now checks status before deleting it, so the
	// error should be consistently ErrRunCancelling.
	resolveErr := session.ResolveApproval("approval-1", agentcore.ApprovalDecisionApproved)
	if !errors.Is(resolveErr, ErrRunCancelling) {
		t.Fatalf("expected ErrRunCancelling, got %v", resolveErr)
	}

	// A second resolve on the same approval must keep returning ErrRunCancelling
	// instead of flipping to ErrApprovalNotFound.
	resolveErr = session.ResolveApproval("approval-1", agentcore.ApprovalDecisionApproved)
	if !errors.Is(resolveErr, ErrRunCancelling) {
		t.Fatalf("expected ErrRunCancelling on repeated resolve, got %v", resolveErr)
	}

	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for AwaitApproval to return")
	}

	if session.Snapshot().Status != RunStatusCancelling {
		t.Fatalf("expected cancelling status after resolve, got %s", session.Snapshot().Status)
	}

	sub := session.Subscribe()
	if sub.Close != nil {
		defer sub.Close()
	}

	for _, event := range sub.History {
		if event.Kind == EventKindRunStatusChanged && event.Status == string(RunStatusRunning) {
			t.Fatalf("unexpected RUN_STATUS_CHANGED: running after cancel")
		}
	}
}

func TestSessionResolveApprovalAfterTerminalReturnsTerminal(t *testing.T) {
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	session := NewSession("run-1", "thread-1", cancel)
	request := agentcore.ApprovalRequest{
		ID:          "approval-1",
		ToolCallID:  "tool-1",
		ToolName:    "write_file",
		Title:       "Approve write_file",
		Description: "Write README.md",
		RiskTone:    agentcore.ApprovalRiskWarning,
	}

	// Simulate an approval that survived past terminal (should not happen in
	// practice, but ResolveApproval must guard against it defensively).
	session.approvals["approval-1"] = &pendingApproval{
		request:  request,
		decision: make(chan agentcore.ApprovalDecision, 1),
	}
	session.CompleteFinished("done")

	err := session.ResolveApproval("approval-1", agentcore.ApprovalDecisionApproved)
	if !errors.Is(err, ErrRunTerminal) {
		t.Fatalf("expected ErrRunTerminal, got %v", err)
	}

	if session.Snapshot().Status != RunStatusFinished {
		t.Fatalf("expected finished status after resolve, got %s", session.Snapshot().Status)
	}

	sub := session.Subscribe()
	if sub.Close != nil {
		defer sub.Close()
	}

	for _, event := range sub.History {
		if event.Kind == EventKindRunStatusChanged && event.Status == string(RunStatusRunning) {
			t.Fatalf("unexpected RUN_STATUS_CHANGED: running after terminal")
		}
	}
}
