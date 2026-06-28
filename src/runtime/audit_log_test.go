package runtime

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"awaken/src/agentcore"
)

func TestAuditLogRingBufferLimitAndFilter(t *testing.T) {
	log := NewAuditLog(2)
	log.RecordSecurityAuditEvent(agentcore.SecurityAuditEvent{
		Type:   agentcore.SecurityAuditRunStarted,
		RunID:  "run-1",
		Action: agentcore.SecurityAuditActionRun,
	})
	log.RecordSecurityAuditEvent(agentcore.SecurityAuditEvent{
		Type:   agentcore.SecurityAuditRunStarted,
		RunID:  "run-2",
		Action: agentcore.SecurityAuditActionRun,
	})
	log.RecordSecurityAuditEvent(agentcore.SecurityAuditEvent{
		Type:   agentcore.SecurityAuditRunFinished,
		RunID:  "run-2",
		Action: agentcore.SecurityAuditActionRun,
	})

	all := log.Events(AuditEventFilter{})
	if len(all) != 2 {
		t.Fatalf("expected ring buffer to retain 2 events, got %+v", all)
	}
	if all[0].RunID != "run-2" || all[1].Type != agentcore.SecurityAuditRunFinished {
		t.Fatalf("unexpected retained audit events: %+v", all)
	}

	filtered := log.Events(AuditEventFilter{RunID: "run-2", Limit: 1})
	if len(filtered) != 1 || filtered[0].Type != agentcore.SecurityAuditRunFinished {
		t.Fatalf("unexpected filtered audit events: %+v", filtered)
	}
}

func TestRunServiceRecordsRunAuditEvents(t *testing.T) {
	service := NewRunService(nil, func(agent *agentcore.Agent) Executor {
		return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunFinishedEventType, Content: "done"})
			return "done", nil
		})
	})

	run, err := service.StartRun(StartRunRequest{Input: "hello secret sk-secret123456", Workspace: WorkspaceConfig{WorkspaceRoot: t.TempDir()}})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}
	waitForRunTerminal(t, service, run.RunID)

	events := service.AuditEvents(AuditEventFilter{RunID: run.RunID})
	if len(events) != 2 {
		t.Fatalf("expected run started and finished audit events, got %+v", events)
	}
	if events[0].Type != agentcore.SecurityAuditRunStarted || events[1].Type != agentcore.SecurityAuditRunFinished {
		t.Fatalf("unexpected run audit events: %+v", events)
	}
	for _, event := range events {
		if event.RunID != run.RunID || event.ThreadID != run.ThreadID {
			t.Fatalf("expected audit event to carry run/thread ids, got %+v", event)
		}
		if strings.Contains(event.Summary+event.InputPreview+event.Error, "sk-secret123456") {
			t.Fatalf("run audit event leaked input secret: %+v", event)
		}
	}
}

func TestRunServiceRecordsApprovalAuditEventsWithRedaction(t *testing.T) {
	approvalReady := make(chan struct{})
	service := NewRunService(nil, func(agent *agentcore.Agent) Executor {
		return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			awaiter, ok := sink.(agentcore.ApprovalAwaiter)
			if !ok {
				return "", errors.New("expected approval awaiter")
			}
			close(approvalReady)
			decision, err := awaiter.AwaitApproval(ctx, agentcore.ApprovalRequest{
				ID:            "approval-1",
				ToolCallID:    "tool-1",
				ToolName:      "shell",
				Title:         "Approve shell",
				Description:   "Authorization: Bearer token-secret-123456",
				RiskTone:      agentcore.ApprovalRiskDanger,
				AffectedPaths: []string{"secret/sk-proj-abc123456789"},
			})
			if err != nil {
				return "", err
			}
			if decision != agentcore.ApprovalDecisionApproved {
				return "", nil
			}
			return "approved", nil
		})
	})

	run, err := service.StartRun(StartRunRequest{Input: "needs approval", Workspace: WorkspaceConfig{WorkspaceRoot: t.TempDir()}})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}
	<-approvalReady
	deadline := time.Now().Add(2 * time.Second)
	awaiting := false
	for time.Now().Before(deadline) {
		snapshot, err := service.GetRun(run.RunID)
		if err != nil {
			t.Fatalf("GetRun returned error: %v", err)
		}
		if snapshot.Status == RunStatusAwaitingApproval {
			awaiting = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !awaiting {
		t.Fatalf("run did not reach awaiting approval")
	}
	if err := service.ResolveApproval(run.RunID, "approval-1", agentcore.ApprovalDecisionApproved); err != nil {
		t.Fatalf("ResolveApproval returned error: %v", err)
	}
	waitForRunTerminal(t, service, run.RunID)

	events := service.AuditEvents(AuditEventFilter{RunID: run.RunID})
	var requested, resolved bool
	for _, event := range events {
		if event.Type == agentcore.SecurityAuditApprovalRequested {
			requested = true
			if event.Action != agentcore.SecurityAuditActionApproval || event.RiskTone != agentcore.ApprovalRiskDanger {
				t.Fatalf("unexpected approval requested audit event: %+v", event)
			}
		}
		if event.Type == agentcore.SecurityAuditApprovalResolved {
			resolved = true
			if event.Outcome != agentcore.SecurityAuditOutcomeApproved || event.ApprovalDecision != agentcore.ApprovalDecisionApproved {
				t.Fatalf("unexpected approval resolved audit event: %+v", event)
			}
		}
		combined := event.Summary + event.InputPreview + event.Error + strings.Join(event.AffectedPaths, "")
		for _, leaked := range []string{"token-secret-123456", "sk-proj-abc123456789"} {
			if strings.Contains(combined, leaked) {
				t.Fatalf("approval audit event leaked %q: %+v", leaked, event)
			}
		}
	}
	if !requested || !resolved {
		t.Fatalf("expected approval requested and resolved audit events, got %+v", events)
	}
}
