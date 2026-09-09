package acp

import (
	"context"
	"testing"
	"time"

	"awaken/src/tool"
)

func TestPermissionIsResolvableWhenPublished(t *testing.T) {
	client := &fakeClient{permissionRequest: &PermissionRequest{Title: "Allow?", Options: []PermissionOption{{OptionID: "allow", Name: "Allow"}}}}
	awaiter := NewChanPermissionAwaiter()
	mgr := NewManager(newTestRegistry("codex"), func(context.Context, RunnerSnapshot, string) (Client, error) { return client, nil })
	defer mgr.CloseAll(context.Background())
	mgr.SetPermissionAwaiter(awaiter)
	base, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	var resolutionErr error
	var published bool
	ctx := tool.ContextWithACPEventEmitter(base, func(e tool.ACPEvent) {
		if e.Type == tool.ACPEventTypePermissionRequested {
			published = true
			resolutionErr = mgr.ResolvePermission("run", e.InteractionID, "allow", false)
		}
	})
	if _, err := mgr.StartSession(ctx, "thread", "codex", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	_, _ = mgr.Message(ctx, "thread", "codex", "run", "call", "test", nil, 0)
	if !published {
		t.Fatal("no request published")
	}
	if resolutionErr != nil {
		t.Fatalf("published approval is not resolvable: %v", resolutionErr)
	}
}

func TestPermissionRegistrationRejectsDuplicateAndCancelledResolution(t *testing.T) {
	for _, cancelBeforeResolve := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		awaiter := NewChanPermissionAwaiter()
		req := PermissionRequest{InteractionID: "permission", Options: []PermissionOption{{OptionID: "allow"}}}
		var first, second error
		_, err := awaiter.AwaitPermission(ctx, req, func() {
			if cancelBeforeResolve {
				cancel()
			}
			first = awaiter.Resolve(req.InteractionID, PermissionResolution{OptionID: "allow"})
			second = awaiter.Resolve(req.InteractionID, PermissionResolution{OptionID: "allow"})
		})
		cancel()
		if cancelBeforeResolve && (first == nil || err == nil) {
			t.Fatal("cancelled permission accepted")
		}
		if !cancelBeforeResolve && (first != nil || err != nil) {
			t.Fatalf("valid resolution failed: %v %v", first, err)
		}
		if second == nil {
			t.Fatal("duplicate permission accepted")
		}
		if _, ok := awaiter.Pending(req.InteractionID); ok {
			t.Fatal("pending record retained")
		}
	}
}
