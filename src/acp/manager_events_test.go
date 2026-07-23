package acp

import (
	"context"
	"testing"
	"time"

	"awaken/src/tool"
)

type fakeClient struct {
	caps              AgentCapabilitiesSnapshot
	promptResult      TurnResult
	permissionRequest *PermissionRequest
}

func (c *fakeClient) Initialize(ctx context.Context) (AgentCapabilitiesSnapshot, error) {
	return c.caps, nil
}

func (c *fakeClient) NewSession(ctx context.Context, cwd string) (string, error) {
	return "acp-session-1", nil
}

func (c *fakeClient) Prompt(ctx context.Context, sessionID, turnID, text string, handler UpdateHandler, permission PermissionHandler) (TurnResult, error) {
	if c.permissionRequest != nil && permission != nil {
		_, _ = permission.RequestPermission(ctx, *c.permissionRequest)
	}
	return c.promptResult, nil
}

func (c *fakeClient) Cancel(ctx context.Context, sessionID string) error {
	return nil
}

func (c *fakeClient) CloseSession(ctx context.Context, sessionID string) error {
	return nil
}

func (c *fakeClient) Close() error {
	return nil
}

func newTestRegistry(runnerID string) *Registry {
	r := NewRegistry("")
	cfg := RunnerConfig{
		ID:      runnerID,
		Enabled: true,
		Command: "go",
		Args:    []string{"version"},
	}
	_ = cfg.Normalize()
	r.runners = map[string]RunnerConfig{runnerID: cfg}
	return r
}

func TestPromptEmitsTurnStartedAndFinished(t *testing.T) {
	client := &fakeClient{}
	mgr := NewManager(newTestRegistry("codex"), func(ctx context.Context, snapshot RunnerSnapshot, cwd string) (Client, error) {
		return client, nil
	})
	mgr.SetPermissionAwaiter(NewChanPermissionAwaiter())

	var emitted []tool.ACPEvent
	ctx := tool.ContextWithACPEventEmitter(context.Background(), func(e tool.ACPEvent) {
		emitted = append(emitted, e)
	})

	conv, err := mgr.StartSession(ctx, "thread-1", "codex", "/tmp")
	if err != nil {
		t.Fatalf("start session: %v", err)
	}
	_ = conv

	client.promptResult = TurnResult{StopReason: "stop", AssistantText: "hello"}
	_, err = mgr.Message(ctx, "thread-1", "codex", "run-1", "tool-call-1", "hi", nil, 0)
	if err != nil {
		t.Fatalf("message: %v", err)
	}

	if len(emitted) < 2 {
		t.Fatalf("expected at least 2 events, got %d: %+v", len(emitted), emitted)
	}
	if emitted[0].Type != tool.ACPEventTypeTurnStarted {
		t.Fatalf("expected turn started event first, got %s", emitted[0].Type)
	}
	if emitted[0].ToolCallID != "tool-call-1" {
		t.Fatalf("expected tool call id to be preserved, got %q", emitted[0].ToolCallID)
	}
	last := emitted[len(emitted)-1]
	if last.Type != tool.ACPEventTypeTurnFinished {
		t.Fatalf("expected turn finished event last, got %s", last.Type)
	}
	if last.Content != "hello" {
		t.Fatalf("expected assistant text in content, got %q", last.Content)
	}
}

func TestPermissionRequestEmitsRequestedAndResolved(t *testing.T) {
	client := &fakeClient{}
	awaiter := NewChanPermissionAwaiter()
	mgr := NewManager(newTestRegistry("codex"), func(ctx context.Context, snapshot RunnerSnapshot, cwd string) (Client, error) {
		return client, nil
	})
	mgr.SetPermissionAwaiter(awaiter)

	var emitted []tool.ACPEvent
	ctx := tool.ContextWithACPEventEmitter(context.Background(), func(e tool.ACPEvent) {
		emitted = append(emitted, e)
	})

	conv, err := mgr.StartSession(ctx, "thread-1", "codex", "/tmp")
	if err != nil {
		t.Fatalf("start session: %v", err)
	}
	_ = conv

	go func() {
		// Resolve the first pending permission request.
		for {
			summary := conv.Summary()
			if summary.PendingPermission != nil {
				_ = awaiter.Resolve(summary.PendingPermission.InteractionID, PermissionResolution{
					InteractionID: summary.PendingPermission.InteractionID,
					OptionID:      "allow",
				})
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()

	client.permissionRequest = &PermissionRequest{Title: "Allow edit?", Options: []PermissionOption{{OptionID: "allow", Name: "Allow"}}}
	client.promptResult = TurnResult{StopReason: "stop", AssistantText: "done"}
	_, err = mgr.Message(ctx, "thread-1", "codex", "run-1", "tool-call-1", "hi", nil, 0)
	if err != nil {
		t.Fatalf("message: %v", err)
	}

	var requested, resolved bool
	for _, e := range emitted {
		if e.Type == tool.ACPEventTypePermissionRequested {
			requested = true
			if e.Content != "Allow edit?" {
				t.Fatalf("expected permission title in content, got %q", e.Content)
			}
		}
		if e.Type == tool.ACPEventTypePermissionResolved {
			resolved = true
			if e.Content != "allow" {
				t.Fatalf("expected option id in content, got %q", e.Content)
			}
		}
	}
	if !requested {
		t.Fatalf("expected permission requested event, got %+v", emitted)
	}
	if !resolved {
		t.Fatalf("expected permission resolved event, got %+v", emitted)
	}
}

func TestCancelEmitsSessionInterrupted(t *testing.T) {
	client := &fakeClient{}
	mgr := NewManager(newTestRegistry("codex"), func(ctx context.Context, snapshot RunnerSnapshot, cwd string) (Client, error) {
		return client, nil
	})

	var emitted []tool.ACPEvent
	ctx := tool.ContextWithACPEventEmitter(context.Background(), func(e tool.ACPEvent) {
		emitted = append(emitted, e)
	})

	conv, err := mgr.StartSession(ctx, "thread-1", "codex", "/tmp")
	if err != nil {
		t.Fatalf("start session: %v", err)
	}
	_ = conv

	// Put the conversation into prompting state by starting a turn manually.
	_, _ = conv.StartTurn("run-1", "tool-call-1")
	if err := mgr.Cancel(ctx, "thread-1", "codex"); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	if len(emitted) != 1 || emitted[0].Type != tool.ACPEventTypeSessionInterrupted {
		t.Fatalf("expected session interrupted event, got %+v", emitted)
	}
}

func TestCloseSessionEmitsSessionClosed(t *testing.T) {
	client := &fakeClient{}
	mgr := NewManager(newTestRegistry("codex"), func(ctx context.Context, snapshot RunnerSnapshot, cwd string) (Client, error) {
		return client, nil
	})

	var emitted []tool.ACPEvent
	ctx := tool.ContextWithACPEventEmitter(context.Background(), func(e tool.ACPEvent) {
		emitted = append(emitted, e)
	})

	conv, err := mgr.StartSession(ctx, "thread-1", "codex", "/tmp")
	if err != nil {
		t.Fatalf("start session: %v", err)
	}
	_ = conv

	if err := mgr.CloseSession(ctx, "thread-1", "codex"); err != nil {
		t.Fatalf("close session: %v", err)
	}

	if len(emitted) != 1 || emitted[0].Type != tool.ACPEventTypeSessionClosed {
		t.Fatalf("expected session closed event, got %+v", emitted)
	}
}
