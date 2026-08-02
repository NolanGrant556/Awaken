package acp_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"awaken/src/acp"
	"awaken/src/acp/protocol"
)

func TestManagerStartSessionAndPrompt(t *testing.T) {
	mockName := "mock-agent"
	if runtime.GOOS == "windows" {
		mockName = "mock-agent.exe"
	}
	mockPath, err := filepath.Abs(filepath.Join("..", "..", "testdata", "acp", "mock-agent", mockName))
	if err != nil {
		t.Fatalf("resolve mock path: %v", err)
	}
	if _, err := os.Stat(mockPath); err != nil {
		t.Skipf("mock agent not built: %v", err)
	}

	registry := acp.NewRegistry(filepath.Join(t.TempDir(), "acp-agents.json"))
	if err := registry.UpsertUser(acp.RunnerConfig{
		ID:      "mock",
		Enabled: true,
		Command: mockPath,
		Args:    []string{},
	}); err != nil {
		t.Fatalf("upsert runner: %v", err)
	}

	manager := acp.NewManager(registry, func(ctx context.Context, snapshot acp.RunnerSnapshot, cwd string) (acp.Client, error) {
		return protocol.NewClient(ctx, snapshot, cwd)
	})
	manager.SetPermissionAwaiter(acp.NewChanPermissionAwaiter())

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	threadID := "thread-1"
	workspaceRoot := t.TempDir()
	conv, err := manager.StartSession(ctx, threadID, "mock", workspaceRoot)
	if err != nil {
		t.Fatalf("start session: %v", err)
	}
	defer manager.CloseSession(context.Background(), threadID, "mock")

	if conv.ACPSessionID == "" {
		t.Fatal("expected session id")
	}
	if conv.State() != acp.ConversationStateReady {
		t.Fatalf("expected ready state, got %s", conv.State())
	}

	result, err := manager.Message(ctx, threadID, "mock", "run-1", "tool-call-1", "hello with tool", nil, 0)
	if err != nil {
		t.Fatalf("message: %v", err)
	}
	if result.StopReason != "end_turn" {
		t.Fatalf("expected end_turn, got %s", result.StopReason)
	}
	if result.AssistantText == "" {
		t.Fatal("expected assistant text")
	}
}

func TestManagerPermissionAllow(t *testing.T) {
	mockName := "mock-agent"
	if runtime.GOOS == "windows" {
		mockName = "mock-agent.exe"
	}
	mockPath, err := filepath.Abs(filepath.Join("..", "..", "testdata", "acp", "mock-agent", mockName))
	if err != nil {
		t.Fatalf("resolve mock path: %v", err)
	}
	if _, err := os.Stat(mockPath); err != nil {
		t.Skipf("mock agent not built: %v", err)
	}

	registry := acp.NewRegistry(filepath.Join(t.TempDir(), "acp-agents.json"))
	if err := registry.UpsertUser(acp.RunnerConfig{
		ID:      "mock",
		Enabled: true,
		Command: mockPath,
		Args:    []string{},
	}); err != nil {
		t.Fatalf("upsert runner: %v", err)
	}

	manager := acp.NewManager(registry, func(ctx context.Context, snapshot acp.RunnerSnapshot, cwd string) (acp.Client, error) {
		return protocol.NewClient(ctx, snapshot, cwd)
	})
	awaiter := acp.NewChanPermissionAwaiter()
	manager.SetPermissionAwaiter(awaiter)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	threadID := "thread-perm"
	workspaceRoot := t.TempDir()
	_, err = manager.StartSession(ctx, threadID, "mock", workspaceRoot)
	if err != nil {
		t.Fatalf("start session: %v", err)
	}
	defer manager.CloseSession(context.Background(), threadID, "mock")

	go func() {
		time.Sleep(200 * time.Millisecond)
		summary := manager.ListSessions()
		var interactionID string
		for _, s := range summary {
			if s.PendingPermission != nil {
				interactionID = s.PendingPermission.InteractionID
				break
			}
		}
		if interactionID == "" {
			t.Log("no pending permission found")
			return
		}
		_ = manager.ResolvePermission("run-1", interactionID, "allow-once", false)
	}()

	result, err := manager.Message(ctx, threadID, "mock", "run-1", "tool-call-perm", "trigger permission", nil, 0)
	if err != nil {
		t.Fatalf("message: %v", err)
	}
	if result.StopReason != "end_turn" {
		t.Fatalf("expected end_turn, got %s", result.StopReason)
	}
}

func TestManagerConcurrentStartDoesNotCreateDuplicateSessions(t *testing.T) {
	registry := acp.NewRegistry(filepath.Join(t.TempDir(), "acp-agents.json"))
	if err := registry.UpsertUser(acp.RunnerConfig{
		ID:      "codex",
		Enabled: true,
		Command: "go",
		Args:    []string{"version"},
	}); err != nil {
		t.Fatalf("upsert runner: %v", err)
	}

	var created int32
	manager := acp.NewManager(registry, func(ctx context.Context, snapshot acp.RunnerSnapshot, cwd string) (acp.Client, error) {
		atomic.AddInt32(&created, 1)
		// Slow down factory so both goroutines race.
		select {
		case <-time.After(100 * time.Millisecond):
		case <-ctx.Done():
		}
		return &fakeClientForConcurrency{caps: acp.AgentCapabilitiesSnapshot{SupportsTextPrompt: true}}, nil
	})

	ctx := context.Background()
	threadID := "thread-concurrent"
	workspaceRoot := t.TempDir()

	var first, second *acp.Conversation
	var firstErr, secondErr error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		first, firstErr = manager.StartSession(ctx, threadID, "codex", workspaceRoot)
	}()
	go func() {
		defer wg.Done()
		second, secondErr = manager.StartSession(ctx, threadID, "codex", workspaceRoot)
	}()
	wg.Wait()

	var sessions int
	if firstErr == nil {
		sessions++
	}
	if secondErr == nil {
		sessions++
	}
	if sessions != 1 {
		t.Fatalf("expected exactly one successful start, got %d (first=%v, second=%v)", sessions, firstErr, secondErr)
	}
	if atomic.LoadInt32(&created) != 1 {
		t.Fatalf("expected one client factory call, got %d", created)
	}

	if first != nil {
		_ = manager.CloseSession(ctx, threadID, "codex")
	}
	if second != nil {
		_ = manager.CloseSession(ctx, threadID, "codex")
	}
}

func TestManagerProbeRunnerUsesClientFactory(t *testing.T) {
	registry := acp.NewRegistry(filepath.Join(t.TempDir(), "acp-agents.json"))
	if err := registry.UpsertUser(acp.RunnerConfig{
		ID:      "codex",
		Enabled: true,
		Command: "go",
		Args:    []string{"version"},
	}); err != nil {
		t.Fatalf("upsert runner: %v", err)
	}

	var initialized int32
	manager := acp.NewManager(registry, func(ctx context.Context, snapshot acp.RunnerSnapshot, cwd string) (acp.Client, error) {
		return &probeFakeClient{onInit: func() { atomic.AddInt32(&initialized, 1) }}, nil
	})

	if err := manager.ProbeRunner(context.Background(), "codex"); err != nil {
		t.Fatalf("probe runner: %v", err)
	}
	if atomic.LoadInt32(&initialized) != 1 {
		t.Fatalf("expected client to be initialized during probe, got %d", initialized)
	}
}

func TestManagerTransportFailureInvalidatesConversationAndPreventsReuse(t *testing.T) {
	registry := acp.NewRegistry(filepath.Join(t.TempDir(), "acp-agents.json"))
	if err := registry.UpsertUser(acp.RunnerConfig{ID: "broken", Enabled: true, Command: "go"}); err != nil {
		t.Fatalf("upsert runner: %v", err)
	}
	client := &transportFailureClient{}
	manager := acp.NewManager(registry, func(context.Context, acp.RunnerSnapshot, string) (acp.Client, error) {
		return client, nil
	})
	threadID := "thread-broken"
	if _, err := manager.StartSession(context.Background(), threadID, "broken", t.TempDir()); err != nil {
		t.Fatalf("start session: %v", err)
	}

	result, err := manager.Message(context.Background(), threadID, "broken", "run-1", "call-1", "fail", nil, 0)
	if err == nil || result.ErrorCode != acp.ErrCodeTransportClosed {
		t.Fatalf("expected transport failure, result=%+v err=%v", result, err)
	}
	conv, ok := manager.GetConversation(threadID, "broken")
	if !ok {
		t.Fatal("expected failed conversation summary to remain available")
	}
	summary := conv.Summary()
	if summary.State != acp.ConversationStateFailed || summary.LastErrorCode != acp.ErrCodeTransportClosed {
		t.Fatalf("expected failed transport summary, got %+v", summary)
	}
	if client.closeCalls.Load() != 1 {
		t.Fatalf("expected failed client to close once, got %d", client.closeCalls.Load())
	}

	_, err = manager.Message(context.Background(), threadID, "broken", "run-2", "call-2", "must not reuse", nil, 0)
	if err == nil || !strings.Contains(err.Error(), acp.ErrCodeTransportClosed) {
		t.Fatalf("expected dead session rejection, got %v", err)
	}
	if client.promptCalls.Load() != 1 {
		t.Fatalf("dead client was reused; prompt calls=%d", client.promptCalls.Load())
	}
}

func TestManagerMarksIdleConversationFailedWhenProcessExits(t *testing.T) {
	registry := acp.NewRegistry(filepath.Join(t.TempDir(), "acp-agents.json"))
	if err := registry.UpsertUser(acp.RunnerConfig{ID: "idle-exit", Enabled: true, Command: "go"}); err != nil {
		t.Fatalf("upsert runner: %v", err)
	}
	client := newLifecycleFailureClient()
	manager := acp.NewManager(registry, func(context.Context, acp.RunnerSnapshot, string) (acp.Client, error) {
		return client, nil
	})
	threadID := "thread-idle-exit"
	if _, err := manager.StartSession(context.Background(), threadID, "idle-exit", t.TempDir()); err != nil {
		t.Fatalf("start session: %v", err)
	}

	client.exit(errors.New("exit status 17"))
	deadline := time.Now().Add(2 * time.Second)
	for {
		conv, ok := manager.GetConversation(threadID, "idle-exit")
		if ok {
			summary := conv.Summary()
			if summary.State == acp.ConversationStateFailed {
				if summary.LastErrorCode != acp.ErrCodeTransportClosed || !strings.Contains(summary.LastErrorMessage, "exit status 17") {
					t.Fatalf("unexpected process-exit summary: %+v", summary)
				}
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("conversation did not transition to failed after process exit")
		}
		time.Sleep(10 * time.Millisecond)
	}

	_, err := manager.Message(context.Background(), threadID, "idle-exit", "run-1", "call-1", "must not run", nil, 0)
	if err == nil || !strings.Contains(err.Error(), acp.ErrCodeTransportClosed) {
		t.Fatalf("expected failed idle session rejection, got %v", err)
	}
	if client.promptCalls.Load() != 0 {
		t.Fatalf("exited client was reused; prompt calls=%d", client.promptCalls.Load())
	}
}

type fakeClientForConcurrency struct {
	caps acp.AgentCapabilitiesSnapshot
}

func (c *fakeClientForConcurrency) Initialize(ctx context.Context) (acp.AgentCapabilitiesSnapshot, error) {
	return c.caps, nil
}
func (c *fakeClientForConcurrency) NewSession(ctx context.Context, cwd string) (string, error) {
	return "session-1", nil
}
func (c *fakeClientForConcurrency) Prompt(ctx context.Context, sessionID, turnID, text string, handler acp.UpdateHandler, permission acp.PermissionHandler) (acp.TurnResult, error) {
	return acp.TurnResult{}, nil
}
func (c *fakeClientForConcurrency) Cancel(ctx context.Context, sessionID string) error { return nil }
func (c *fakeClientForConcurrency) CloseSession(ctx context.Context, sessionID string) error {
	return nil
}
func (c *fakeClientForConcurrency) Close() error { return nil }

type probeFakeClient struct {
	onInit func()
}

func (c *probeFakeClient) Initialize(ctx context.Context) (acp.AgentCapabilitiesSnapshot, error) {
	c.onInit()
	return acp.AgentCapabilitiesSnapshot{SupportsTextPrompt: true}, nil
}
func (c *probeFakeClient) NewSession(ctx context.Context, cwd string) (string, error) {
	return "session-1", nil
}
func (c *probeFakeClient) Prompt(ctx context.Context, sessionID, turnID, text string, handler acp.UpdateHandler, permission acp.PermissionHandler) (acp.TurnResult, error) {
	return acp.TurnResult{}, nil
}
func (c *probeFakeClient) Cancel(ctx context.Context, sessionID string) error       { return nil }
func (c *probeFakeClient) CloseSession(ctx context.Context, sessionID string) error { return nil }
func (c *probeFakeClient) Close() error                                             { return nil }

type transportFailureClient struct {
	promptCalls atomic.Int32
	closeCalls  atomic.Int32
}

func (c *transportFailureClient) Initialize(context.Context) (acp.AgentCapabilitiesSnapshot, error) {
	return acp.AgentCapabilitiesSnapshot{SupportsTextPrompt: true}, nil
}
func (c *transportFailureClient) NewSession(context.Context, string) (string, error) {
	return "session-broken", nil
}
func (c *transportFailureClient) Prompt(context.Context, string, string, string, acp.UpdateHandler, acp.PermissionHandler) (acp.TurnResult, error) {
	c.promptCalls.Add(1)
	return acp.TurnResult{SideEffectState: "unknown"}, errors.New("write |1: broken pipe")
}
func (c *transportFailureClient) Cancel(context.Context, string) error       { return nil }
func (c *transportFailureClient) CloseSession(context.Context, string) error { return nil }
func (c *transportFailureClient) Close() error {
	c.closeCalls.Add(1)
	return nil
}

type lifecycleFailureClient struct {
	done        chan struct{}
	doneOnce    sync.Once
	exitErr     error
	promptCalls atomic.Int32
	closeCalls  atomic.Int32
}

func newLifecycleFailureClient() *lifecycleFailureClient {
	return &lifecycleFailureClient{done: make(chan struct{})}
}

func (c *lifecycleFailureClient) Initialize(context.Context) (acp.AgentCapabilitiesSnapshot, error) {
	return acp.AgentCapabilitiesSnapshot{SupportsTextPrompt: true}, nil
}
func (c *lifecycleFailureClient) NewSession(context.Context, string) (string, error) {
	return "session-idle-exit", nil
}
func (c *lifecycleFailureClient) Prompt(context.Context, string, string, string, acp.UpdateHandler, acp.PermissionHandler) (acp.TurnResult, error) {
	c.promptCalls.Add(1)
	return acp.TurnResult{}, nil
}
func (c *lifecycleFailureClient) Cancel(context.Context, string) error       { return nil }
func (c *lifecycleFailureClient) CloseSession(context.Context, string) error { return nil }
func (c *lifecycleFailureClient) Close() error {
	c.closeCalls.Add(1)
	c.doneOnce.Do(func() { close(c.done) })
	return nil
}
func (c *lifecycleFailureClient) Done() <-chan struct{} { return c.done }
func (c *lifecycleFailureClient) ExitError() error      { return c.exitErr }
func (c *lifecycleFailureClient) exit(err error) {
	c.exitErr = err
	c.doneOnce.Do(func() { close(c.done) })
}
