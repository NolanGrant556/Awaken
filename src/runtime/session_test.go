package runtime

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"awaken/src/agentcore"
	"awaken/src/tool"
)

func TestSessionAwaitApprovalTransitionsStatus(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	session := NewSession("run-1", "thread-1", "", cancel, nil)

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

	sub := session.Subscribe(0)
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

func TestSessionApprovalEventsCarryDiffReviewMetadata(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	session := NewSession("run-1", "thread-1", "", cancel, nil)
	request := agentcore.ApprovalRequest{
		ID:            "approval-1",
		ToolCallID:    "tool-1",
		ToolName:      "apply_patch",
		Title:         "Approve apply_patch",
		Description:   "Modify note.md",
		RiskTone:      agentcore.ApprovalRiskWarning,
		AffectedPaths: []string{"note.md"},
		PatchSummary:  "新增 1 个文件",
		DiffReview: &tool.DiffReviewMetadata{
			AffectedPaths: []string{"note.md"},
			FileCounts:    tool.DiffReviewFileCounts{Added: 1},
			LineCounts:    tool.DiffReviewLineCounts{Added: 1},
			Risk:          tool.DiffReviewRiskSummary{Level: "warning", Summary: "1 file changes, +1/-0 lines. Stays within writable roots."},
		},
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
		history := session.History(0)
		if len(history) >= 3 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for approval history, got %+v", history)
		}
		time.Sleep(10 * time.Millisecond)
	}

	if err := session.ResolveApproval("approval-1", agentcore.ApprovalDecisionRejected); err != nil {
		t.Fatalf("ResolveApproval returned error: %v", err)
	}

	select {
	case decision := <-decisionCh:
		if decision != agentcore.ApprovalDecisionRejected {
			t.Fatalf("expected rejected decision, got %s", decision)
		}
	case err := <-errCh:
		t.Fatalf("AwaitApproval returned error: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for decision")
	}

	history := session.History(0)
	var (
		requested agentcore.AwakenEvent
		resolved  agentcore.AwakenEvent
		artifact  agentcore.AwakenEvent
	)
	for _, event := range history {
		switch event.AgentEvent.Type {
		case agentcore.ApprovalRequestedEventType:
			requested = event.AgentEvent
		case agentcore.ApprovalResolvedEventType:
			resolved = event.AgentEvent
		case agentcore.ArtifactCreatedEventType:
			if event.AgentEvent.ArtifactKind == "a2ui_surface" {
				artifact = event.AgentEvent
			}
		}
	}

	if requested.DiffReview == nil || requested.DiffReview.LineCounts.Added != 1 {
		t.Fatalf("expected requested diff review metadata, got %+v", requested.DiffReview)
	}
	if resolved.DiffReview == nil || resolved.DiffReview.Risk.Level != "warning" {
		t.Fatalf("expected resolved diff review metadata, got %+v", resolved.DiffReview)
	}
	if artifact.DiffReview == nil || !strings.Contains(artifact.ArtifactContent, "\"line-counts\"") || !strings.Contains(artifact.ArtifactContent, "Stays within writable roots") {
		t.Fatalf("expected a2ui artifact content to include diff review metadata, got %+v", artifact)
	}
}

func TestSessionAwaitApprovalAtomicWithCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	session := NewSession("run-1", "thread-1", "", cancel, nil)
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

	sub := session.Subscribe(0)
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
	if statusChanges[1].Status != string(RunStatusCancelling) {
		t.Fatalf("expected second status cancelling, got %s", statusChanges[1].Status)
	}
}

func TestSessionResolveApprovalAfterCancelReturnsCancelling(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	session := NewSession("run-1", "thread-1", "", cancel, nil)
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

	sub := session.Subscribe(0)
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

	session := NewSession("run-1", "thread-1", "", cancel, nil)
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

	sub := session.Subscribe(0)
	if sub.Close != nil {
		defer sub.Close()
	}

	for _, event := range sub.History {
		if event.Kind == EventKindRunStatusChanged && event.Status == string(RunStatusRunning) {
			t.Fatalf("unexpected RUN_STATUS_CHANGED: running after terminal")
		}
	}
}

func TestSessionApprovalTimeoutRejectsAndEmitsResolvedEvent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	timeoutCh := make(chan time.Time, 1)
	session := NewSessionWithConfig("run-1", "thread-1", "", cancel, SessionConfig{
		ApprovalPolicy: agentcore.ApprovalPolicy{TimeoutMs: 1},
		After: func(time.Duration) <-chan time.Time {
			return timeoutCh
		},
	})
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

	timeoutCh <- time.Now()

	select {
	case decision := <-decisionCh:
		if decision != agentcore.ApprovalDecisionRejected {
			t.Fatalf("expected rejected timeout decision, got %s", decision)
		}
	case err := <-errCh:
		t.Fatalf("AwaitApproval returned error: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for timeout rejection")
	}

	if session.Snapshot().Status != RunStatusRunning {
		t.Fatalf("expected running status after timeout resolve, got %s", session.Snapshot().Status)
	}

	sub := session.Subscribe(0)
	if sub.Close != nil {
		defer sub.Close()
	}

	foundTimeoutResolve := false
	for _, event := range sub.History {
		if event.AgentEvent.Type == agentcore.ApprovalResolvedEventType &&
			event.AgentEvent.ApprovalDecision == agentcore.ApprovalDecisionRejected &&
			event.AgentEvent.Code == "approval_timeout" {
			foundTimeoutResolve = true
			break
		}
	}
	if !foundTimeoutResolve {
		t.Fatalf("expected timeout approval resolved event, got %+v", sub.History)
	}
}
