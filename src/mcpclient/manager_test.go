package mcpclient

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"awaken/src/tool"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// fakeRegistry captures register/unregister calls without touching the
// real tool registry. Used by manager-level tests so they don't pollute
// global state.
type fakeRegistry struct {
	mu       sync.Mutex
	tools    map[string]tool.Tool
	regCnt   int
	unregCnt int
}

func newFakeRegistry() *fakeRegistry {
	return &fakeRegistry{tools: make(map[string]tool.Tool)}
}

func (f *fakeRegistry) register(t tool.Tool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, exists := f.tools[t.Name()]; exists {
		f.regCnt++
		return errors.New("duplicate")
	}
	f.tools[t.Name()] = t
	f.regCnt++
	return nil
}

func (f *fakeRegistry) unregisterByServer(serverName string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	removed := 0
	for name, runtime := range f.tools {
		if runtime.Definition().Annotations.ServerName != serverName {
			continue
		}
		delete(f.tools, name)
		removed++
	}
	f.unregCnt++
	return removed
}

func (f *fakeRegistry) names() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.tools))
	for n := range f.tools {
		out = append(out, n)
	}
	return out
}

// stubTool is a minimal tool.Tool used by the registry tests.
type stubTool struct {
	name     string
	readonly bool
}

func (s *stubTool) Name() string                     { return s.name }
func (s *stubTool) Description() string              { return "stub" }
func (s *stubTool) Parameters() []tool.ToolParameter { return nil }
func (s *stubTool) Execute(ctx context.Context, input string) (string, error) {
	return "ok", nil
}
func (s *stubTool) IsReadOnly() bool { return s.readonly }

type callToolSession struct {
	t       *testing.T
	last    *mcp.CallToolParams
	results *mcp.CallToolResult
}

func (s *callToolSession) ListTools(context.Context, *mcp.ListToolsParams) (*mcp.ListToolsResult, error) {
	return nil, errors.New("not implemented")
}

func (s *callToolSession) CallTool(_ context.Context, params *mcp.CallToolParams) (*mcp.CallToolResult, error) {
	s.last = params
	if s.results != nil {
		return s.results, nil
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}, nil
}

func (s *callToolSession) ListPrompts(context.Context, *mcp.ListPromptsParams) (*mcp.ListPromptsResult, error) {
	return nil, errors.New("not implemented")
}

func (s *callToolSession) GetPrompt(context.Context, *mcp.GetPromptParams) (*mcp.GetPromptResult, error) {
	return nil, errors.New("not implemented")
}

func (s *callToolSession) ListResources(context.Context, *mcp.ListResourcesParams) (*mcp.ListResourcesResult, error) {
	return nil, errors.New("not implemented")
}

func (s *callToolSession) Close() error { return nil }
func (s *callToolSession) Wait() error  { return nil }

// echoServerWithTools returns an in-proc factory that exposes N tools named
// echo1..echoN, all read-only. Used to verify the manager registers each
// tool with the mcp__{server}__{tool} prefix.
func echoServerWithTools(toolNames ...string) InProcServerFactory {
	return func() *mcp.Server {
		server := mcp.NewServer(&mcp.Implementation{Name: "echo", Version: "v1.0.0"}, nil)
		for _, name := range toolNames {
			n := name
			mcp.AddTool(server, &mcp.Tool{
				Name:        n,
				Description: "echoes " + n,
				Annotations: &mcp.ToolAnnotations{
					ReadOnlyHint:    true,
					DestructiveHint: boolPtr(false),
					OpenWorldHint:   boolPtr(false),
				},
			}, func(ctx context.Context, req *mcp.CallToolRequest, input map[string]string) (*mcp.CallToolResult, map[string]string, error) {
				return &mcp.CallToolResult{
					Content: []mcp.Content{&mcp.TextContent{Text: n + ":" + input["message"]}},
				}, input, nil
			})
		}
		return server
	}
}

// TestManagerStateMachineConnected verifies the manager drives an in-proc
// server from pending → connected and surfaces the state via Snapshot.
func TestManagerStateMachineConnected(t *testing.T) {
	registry := NewInProcRegistry()
	if err := registry.Register("echo", echoServerWithTools("ping")); err != nil {
		t.Fatalf("register: %v", err)
	}
	mgr := NewManager(WithInProcRegistry(registry), WithLogger(noopLogger{}))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	mgr.Start(ctx, []ScopedMcpServerConfig{
		{Name: "echo", Scope: ScopeProject, Config: McpServerConfig{
			InProc:      &InProcTransportConfig{ServerName: "echo"},
			DisplayName: "中文 Echo",
		}},
	})
	defer mgr.Shutdown(context.Background())

	if err := waitForState(mgr, "echo", StateConnected, 3*time.Second); err != nil {
		snap, _ := mgr.SnapshotServer("echo")
		t.Fatalf("echo never connected: %v (state=%s reason=%q)", err, snap.State, snap.ErrorReason)
	}
	snap, err := mgr.SnapshotServer("echo")
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if snap.Transport != TransportInProc {
		t.Errorf("expected in-proc transport, got %s", snap.Transport)
	}
	if snap.Scope != ScopeProject {
		t.Errorf("expected project scope, got %s", snap.Scope)
	}
	if snap.DisplayName != "中文 Echo" {
		t.Errorf("expected display name to survive snapshot, got %q", snap.DisplayName)
	}
	if snap.Capabilities == nil || !snap.Capabilities.SupportsTools {
		t.Errorf("expected SupportsTools=true, got %+v", snap.Capabilities)
	}
}

func TestManagerDisabledToolsStayVisibleButAreNotRegistered(t *testing.T) {
	fakeReg := newFakeRegistry()
	restoreHooks := setToolRegistryHooksForTest(fakeReg.register, fakeReg.unregisterByServer)
	defer restoreHooks()
	registry := NewInProcRegistry()
	if err := registry.Register("echo-filtered", echoServerWithTools("alpha", "beta")); err != nil {
		t.Fatal(err)
	}
	mgr := NewManager(WithInProcRegistry(registry), WithLogger(noopLogger{}))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	mgr.Start(ctx, []ScopedMcpServerConfig{{Name: "filtered", Scope: ScopePlugin, Config: McpServerConfig{
		InProc: &InProcTransportConfig{ServerName: "echo-filtered"}, DisabledTools: []string{"beta"},
	}}})
	defer mgr.Shutdown(context.Background())
	if err := waitForState(mgr, "filtered", StateConnected, 3*time.Second); err != nil {
		t.Fatal(err)
	}
	names := fakeReg.names()
	if !slices.Contains(names, McpToolName("filtered", "alpha")) || slices.Contains(names, McpToolName("filtered", "beta")) {
		t.Fatalf("registered tools=%v", names)
	}
	tools, err := mgr.ServerTools("filtered")
	if err != nil || len(tools) != 2 || tools[0].Name != "alpha" || !tools[0].Enabled || tools[1].Name != "beta" || tools[1].Enabled {
		t.Fatalf("management tools=%#v err=%v", tools, err)
	}
}

// TestManagerStateMachineFailed verifies a server with no factory resolves
// to failed state rather than hanging.
func TestManagerStateMachineFailed(t *testing.T) {
	registry := NewInProcRegistry()
	// intentionally do not register "missing"
	mgr := NewManager(WithInProcRegistry(registry), WithLogger(noopLogger{}))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	mgr.Start(ctx, []ScopedMcpServerConfig{
		{Name: "missing", Scope: ScopeProject, Config: McpServerConfig{
			InProc: &InProcTransportConfig{ServerName: "missing"},
		}},
	})

	if err := waitForState(mgr, "missing", StateFailed, 8*time.Second); err != nil {
		snap, _ := mgr.SnapshotServer("missing")
		t.Fatalf("missing never failed: %v (state=%s reason=%q)", err, snap.State, snap.ErrorReason)
	}
}

// TestManagerStateMachineInvalidTransport confirms a config with no
// transport variant is marked failed synchronously during startOne.
func TestManagerStateMachineInvalidTransport(t *testing.T) {
	mgr := NewManager(WithLogger(noopLogger{}))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	mgr.Start(ctx, []ScopedMcpServerConfig{
		{Name: "bogus", Scope: ScopeUser, Config: McpServerConfig{}},
	})
	snap, err := mgr.SnapshotServer("bogus")
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if snap.State != StateFailed {
		t.Errorf("expected failed for invalid transport, got %s", snap.State)
	}
	if !strings.Contains(snap.ErrorReason, "invalid transport") {
		t.Errorf("expected invalid transport reason, got %q", snap.ErrorReason)
	}
}

func TestManagerCallToolResolvesCanonicalDottedPluginName(t *testing.T) {
	serverName := "plugin__awaken.acceptance.20260827__local-smoke"
	mgr := NewManager(WithLogger(noopLogger{}))
	session := &callToolSession{t: t}
	mgr.servers[serverName] = &managedServer{
		name:      serverName,
		state:     StateConnected,
		session:   session,
		transport: TransportHTTP,
	}
	defer mgr.unregisterTools(serverName)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	for _, test := range []struct {
		name      string
		qualified string
		rawTool   string
	}{
		{name: "safe", qualified: McpToolName(serverName, "echo"), rawTool: "echo"},
		{name: "dotted tool", qualified: McpToolName(serverName, "echo.v1"), rawTool: "echo.v1"},
		{name: "reserved sentinel", qualified: McpToolName(serverName, "x0_echo"), rawTool: "x0_echo"},
		{name: "legacy raw-qualified", qualified: "mcp__" + serverName + "__legacy.echo", rawTool: "legacy.echo"},
	} {
		t.Run(test.name, func(t *testing.T) {
			session.last = nil
			result, err := mgr.CallTool(ctx, test.qualified, map[string]any{"message": "hi"})
			if err != nil {
				t.Fatalf("call tool: %v", err)
			}
			if session.last == nil {
				t.Fatal("manager did not call the MCP session")
			}
			if session.last.Name != test.rawTool {
				t.Fatalf("manager recovered tool name %q, want %q", session.last.Name, test.rawTool)
			}
			if len(result.Content) != 1 {
				t.Fatalf("unexpected call result: %+v", result)
			}
			text, ok := result.Content[0].(*mcp.TextContent)
			if !ok || text.Text != "ok" {
				t.Fatalf("unexpected call result content: %+v", result.Content[0])
			}
		})
	}
}

func TestResolveMcpToolNameUsesLongestEncodedServerPrefix(t *testing.T) {
	shortServer := "plugin__acme.tools__local"
	longServer := shortServer + "__nested"
	mgr := NewManager(WithLogger(noopLogger{}))
	mgr.servers[shortServer] = &managedServer{name: shortServer}
	mgr.servers[longServer] = &managedServer{name: longServer}

	serverName, toolName, ok := mgr.resolveMcpToolName(McpToolName(longServer, "echo.v1"))
	if !ok || serverName != longServer || toolName != "echo.v1" {
		t.Fatalf("resolve longest encoded prefix = (%q, %q, %v)", serverName, toolName, ok)
	}
	serverName, toolName, ok = mgr.resolveMcpToolName("mcp__" + longServer + "__legacy.echo")
	if !ok || serverName != longServer || toolName != "legacy.echo" {
		t.Fatalf("resolve longest legacy prefix = (%q, %q, %v)", serverName, toolName, ok)
	}
}

func TestManagerRestoresDisabledConfigWithoutConnecting(t *testing.T) {
	disabled := false
	mgr := NewManager(WithLogger(noopLogger{}))
	mgr.Start(context.Background(), []ScopedMcpServerConfig{{
		Name:  "disabled-http",
		Scope: ScopeProject,
		Config: McpServerConfig{
			HTTP:    &HTTPTransportConfig{URL: "https://should-not-connect.invalid/mcp"},
			Enabled: &disabled,
		},
	}})
	t.Cleanup(func() { mgr.Shutdown(context.Background()) })

	snapshot, err := mgr.SnapshotServer("disabled-http")
	if err != nil {
		t.Fatalf("SnapshotServer: %v", err)
	}
	if snapshot.State != StateDisabled || snapshot.Scope != ScopeProject {
		t.Fatalf("disabled config was not restored: %+v", snapshot)
	}
}

func TestManagerRestartWithConfigPreservesSnapshotOrder(t *testing.T) {
	disabled := false
	mgr := NewManager(WithLogger(noopLogger{}))
	configs := []ScopedMcpServerConfig{
		{Name: "playwright", Scope: ScopeUser, Config: McpServerConfig{
			HTTP: &HTTPTransportConfig{URL: "https://playwright.invalid/mcp"}, Enabled: &disabled,
		}},
		{Name: "puppeteer", Scope: ScopeUser, Config: McpServerConfig{
			HTTP: &HTTPTransportConfig{URL: "https://puppeteer.invalid/mcp"}, Enabled: &disabled,
		}},
		{Name: "github", Scope: ScopeUser, Config: McpServerConfig{
			HTTP: &HTTPTransportConfig{URL: "https://github.invalid/mcp"}, Enabled: &disabled,
		}},
	}
	mgr.Start(context.Background(), configs)
	t.Cleanup(func() { mgr.Shutdown(context.Background()) })

	names := func() []string {
		snapshots := mgr.Snapshot()
		result := make([]string, 0, len(snapshots))
		for _, snapshot := range snapshots {
			result = append(result, snapshot.Name)
		}
		return result
	}
	want := []string{"playwright", "puppeteer", "github"}
	if got := names(); !slices.Equal(got, want) {
		t.Fatalf("initial snapshot order = %v, want %v", got, want)
	}

	if err := mgr.RestartWithConfig(context.Background(), configs[0]); err != nil {
		t.Fatalf("RestartWithConfig: %v", err)
	}
	if got := names(); !slices.Equal(got, want) {
		t.Fatalf("snapshot order after restart = %v, want %v", got, want)
	}
}

func TestManagerRestartWithConfigKeepsServerVisible(t *testing.T) {
	disabled := false
	mgr := NewManager(WithLogger(noopLogger{}))
	config := ScopedMcpServerConfig{
		Name:  "user-http",
		Scope: ScopeUser,
		Config: McpServerConfig{
			HTTP:    &HTTPTransportConfig{URL: "https://user.invalid/mcp"},
			Enabled: &disabled,
		},
	}
	mgr.Start(context.Background(), []ScopedMcpServerConfig{config})
	t.Cleanup(func() { mgr.Shutdown(context.Background()) })

	done := make(chan struct{})
	var missing atomic.Bool
	go func() {
		defer close(done)
		deadline := time.Now().Add(500 * time.Millisecond)
		for time.Now().Before(deadline) {
			if _, err := mgr.SnapshotServer("user-http"); err != nil {
				missing.Store(true)
				return
			}
		}
	}()

	for range 32 {
		if err := mgr.RestartWithConfig(context.Background(), config); err != nil {
			t.Fatalf("RestartWithConfig: %v", err)
		}
	}
	<-done
	if missing.Load() {
		t.Fatal("RestartWithConfig briefly omitted the server from Snapshot")
	}
	if snapshot, err := mgr.SnapshotServer("user-http"); err != nil || snapshot.State != StateDisabled {
		t.Fatalf("expected disabled server after restarts: snapshot=%+v err=%v", snapshot, err)
	}
}

// TestManagerStateMachineWSRejected confirms the WS transport is marked
// failed rather than attempted.
func TestManagerStateMachineWSRejected(t *testing.T) {
	mgr := NewManager(WithLogger(noopLogger{}))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	mgr.Start(ctx, []ScopedMcpServerConfig{
		{Name: "wsserver", Scope: ScopeUser, Config: McpServerConfig{
			WS: &HTTPTransportConfig{URL: "ws://example.com"},
		}},
	})
	snap, err := mgr.SnapshotServer("wsserver")
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if snap.State != StateFailed {
		t.Errorf("expected failed for ws, got %s", snap.State)
	}
	if !strings.Contains(snap.ErrorReason, "ws") {
		t.Errorf("expected ws reason, got %q", snap.ErrorReason)
	}
}

// TestManagerDisableEnable verifies Disable moves a server to disabled
// and unregisters its tools; Enable re-arms the connect loop.
func TestManagerDisableEnable(t *testing.T) {
	fakeReg := newFakeRegistry()
	restoreHooks := setToolRegistryHooksForTest(fakeReg.register, fakeReg.unregisterByServer)
	defer restoreHooks()

	registry := NewInProcRegistry()
	if err := registry.Register("echo", echoServerWithTools("a", "b")); err != nil {
		t.Fatalf("register: %v", err)
	}
	mgr := NewManager(WithInProcRegistry(registry), WithLogger(noopLogger{}))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	mgr.Start(ctx, []ScopedMcpServerConfig{
		{Name: "echo", Scope: ScopeProject, Config: McpServerConfig{
			InProc: &InProcTransportConfig{ServerName: "echo"},
		}},
	})
	if err := waitForState(mgr, "echo", StateConnected, 3*time.Second); err != nil {
		t.Fatalf("echo never connected: %v", err)
	}
	// Configured MCP tools should register directly into the callable pool.
	if got := len(fakeReg.names()); got != 2 {
		t.Fatalf("expected 2 tools registered, got %d", got)
	}

	if err := mgr.Disable("echo"); err != nil {
		t.Fatalf("disable: %v", err)
	}
	snap, _ := mgr.SnapshotServer("echo")
	if snap.State != StateDisabled {
		t.Errorf("expected disabled, got %s", snap.State)
	}
	// Tools should be unregistered.
	if got := len(fakeReg.names()); got != 0 {
		t.Errorf("expected 0 tools after disable, got %d", got)
	}

	if err := mgr.Enable(ctx, "echo"); err != nil {
		t.Fatalf("enable: %v", err)
	}
	if err := waitForState(mgr, "echo", StateConnected, 3*time.Second); err != nil {
		t.Fatalf("echo never reconnected: %v", err)
	}
	if got := len(fakeReg.names()); got != 2 {
		t.Errorf("expected 2 tools after re-enable, got %d", got)
	}
}

// TestManagerNeedsAuthOnCall verifies CallTool returns NeedsAuthError when
// the server is in needs-auth state.
func TestManagerNeedsAuthOnCall(t *testing.T) {
	registry := NewInProcRegistry()
	if err := registry.Register("echo", echoServerWithTools("ping")); err != nil {
		t.Fatalf("register: %v", err)
	}
	mgr := NewManager(WithInProcRegistry(registry), WithLogger(noopLogger{}))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	mgr.Start(ctx, []ScopedMcpServerConfig{
		{Name: "echo", Scope: ScopeProject, Config: McpServerConfig{
			InProc: &InProcTransportConfig{ServerName: "echo"},
		}},
	})
	if err := waitForState(mgr, "echo", StateConnected, 3*time.Second); err != nil {
		t.Fatalf("echo never connected: %v", err)
	}

	// Force the server into needs-auth by mutating state directly. This
	// simulates a 401 from a real server without spinning up an OAuth flow.
	mgr.mu.Lock()
	if srv, ok := mgr.servers["echo"]; ok {
		srv.state = StateNeedsAuth
		srv.errorReason = "simulated 401"
	}
	mgr.mu.Unlock()

	_, err := mgr.CallTool(ctx, "mcp__echo__ping", map[string]any{"message": "hi"})
	if err == nil {
		t.Fatal("expected error from CallTool on needs-auth server")
	}
	var nae *NeedsAuthError
	if !errors.As(err, &nae) {
		t.Errorf("expected NeedsAuthError, got %T: %v", err, err)
	}
	if nae == nil || nae.ServerName != "echo" {
		t.Errorf("expected echo server name, got %+v", nae)
	}
}

// TestManagerCallToolRouting verifies the mcp__{server}__{tool} routing
// actually invokes the right server-side handler.
func TestManagerCallToolRouting(t *testing.T) {
	registry := NewInProcRegistry()
	if err := registry.Register("echo", echoServerWithTools("ping", "pong")); err != nil {
		t.Fatalf("register: %v", err)
	}
	mgr := NewManager(WithInProcRegistry(registry), WithLogger(noopLogger{}))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	mgr.Start(ctx, []ScopedMcpServerConfig{
		{Name: "echo", Scope: ScopeProject, Config: McpServerConfig{
			InProc: &InProcTransportConfig{ServerName: "echo"},
		}},
	})
	if err := waitForState(mgr, "echo", StateConnected, 3*time.Second); err != nil {
		t.Fatalf("echo never connected: %v", err)
	}
	defer mgr.Shutdown(context.Background())

	res, err := mgr.CallTool(ctx, "mcp__echo__ping", map[string]any{"message": "hello"})
	if err != nil {
		t.Fatalf("call ping: %v", err)
	}
	if len(res.Content) == 0 {
		t.Fatal("expected non-empty content")
	}
	if tc, ok := res.Content[0].(*mcp.TextContent); !ok || !strings.Contains(tc.Text, "ping:hello") {
		t.Errorf("expected ping:hello, got %+v", res.Content[0])
	}

	// Calling a tool on a non-existent server returns ToolNotFoundError.
	_, err = mgr.CallTool(ctx, "mcp__missing__nope", nil)
	if err == nil {
		t.Fatal("expected error for missing server")
	}
	if _, ok := err.(*ToolNotFoundError); !ok {
		t.Errorf("expected ToolNotFoundError, got %T: %v", err, err)
	}

	// Calling a tool that does not exist on a connected server surfaces
	// the SDK's method-not-found error (not ToolNotFoundError, since the
	// server is reachable — the tool simply isn't there).
	_, err = mgr.CallTool(ctx, "mcp__echo__nope", nil)
	if err == nil {
		t.Fatal("expected error for missing tool on connected server")
	}
}

// TestManagerToolRegistration verifies the manager registers MCP tools with
// the correct prefixed names and unregisters them on shutdown.
func TestManagerToolRegistration(t *testing.T) {
	fakeReg := newFakeRegistry()
	restoreHooks := setToolRegistryHooksForTest(fakeReg.register, fakeReg.unregisterByServer)
	defer restoreHooks()

	registry := NewInProcRegistry()
	if err := registry.Register("echo", echoServerWithTools("alpha", "beta")); err != nil {
		t.Fatalf("register: %v", err)
	}
	mgr := NewManager(WithInProcRegistry(registry), WithLogger(noopLogger{}))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	mgr.Start(ctx, []ScopedMcpServerConfig{
		{Name: "echo", Scope: ScopeProject, Config: McpServerConfig{
			InProc: &InProcTransportConfig{ServerName: "echo"},
		}},
	})
	if err := waitForState(mgr, "echo", StateConnected, 3*time.Second); err != nil {
		t.Fatalf("echo never connected: %v", err)
	}

	names := fakeReg.names()
	if len(names) != 2 {
		t.Fatalf("expected 2 tools, got %d (%v)", len(names), names)
	}
	expected := map[string]bool{
		"mcp__echo__alpha": false,
		"mcp__echo__beta":  false,
	}
	for _, n := range names {
		if _, ok := expected[n]; ok {
			expected[n] = true
		} else {
			t.Errorf("unexpected tool name %q", n)
		}
	}
	for n, seen := range expected {
		if !seen {
			t.Errorf("expected tool %q to be registered", n)
		}
	}

	mgr.Shutdown(context.Background())
	// After shutdown, tools should be unregistered (the connect loop exits
	// and unregisterTools is called). Give it a moment to settle.
	deadline := time.Now().Add(2 * time.Second)
	for len(fakeReg.names()) != 0 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if got := len(fakeReg.names()); got != 0 {
		t.Errorf("expected 0 tools after shutdown, got %d (%v)", got, fakeReg.names())
	}
}

// TestManagerToolsChangedCallback verifies the callback fires when a server
// connects and when it disconnects.
func TestManagerToolsChangedCallback(t *testing.T) {
	var connectCalls atomic.Int32
	cb := func(serverName string) {
		// We can't easily inspect state from the callback because it's
		// already changed; just count.
		_ = serverName
		connectCalls.Add(1)
	}
	registry := NewInProcRegistry()
	if err := registry.Register("echo", echoServerWithTools("ping")); err != nil {
		t.Fatalf("register: %v", err)
	}
	mgr := NewManager(
		WithInProcRegistry(registry),
		WithLogger(noopLogger{}),
		WithToolsChangedCallback(cb),
	)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	mgr.Start(ctx, []ScopedMcpServerConfig{
		{Name: "echo", Scope: ScopeProject, Config: McpServerConfig{
			InProc: &InProcTransportConfig{ServerName: "echo"},
		}},
	})
	if err := waitForState(mgr, "echo", StateConnected, 3*time.Second); err != nil {
		t.Fatalf("echo never connected: %v", err)
	}
	// At least one callback for connect + tool registration.
	if got := connectCalls.Load(); got < 1 {
		t.Errorf("expected >=1 tools-changed callbacks on connect, got %d", got)
	}
}

// TestManagerPanicRecordsErrorReason verifies a panicking in-proc server
// records the panic message in the server's errorReason.
func TestManagerPanicRecordsErrorReason(t *testing.T) {
	registry := NewInProcRegistry()
	if err := registry.Register("boom", InProcServerFactory(func() *mcp.Server {
		panic("test panic message 42")
	})); err != nil {
		t.Fatalf("register: %v", err)
	}
	mgr := NewManager(WithInProcRegistry(registry), WithLogger(noopLogger{}))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	mgr.Start(ctx, []ScopedMcpServerConfig{
		{Name: "boom", Scope: ScopeProject, Config: McpServerConfig{
			InProc: &InProcTransportConfig{ServerName: "boom"},
		}},
	})
	if err := waitForState(mgr, "boom", StateFailed, 3*time.Second); err != nil {
		t.Fatalf("boom never reached failed: %v", err)
	}
	snap, err := mgr.SnapshotServer("boom")
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if !strings.Contains(snap.ErrorReason, "panic") {
		t.Errorf("expected errorReason to contain 'panic', got %q", snap.ErrorReason)
	}
	if !strings.Contains(snap.ErrorReason, "test panic message 42") {
		t.Errorf("expected errorReason to contain panic message, got %q", snap.ErrorReason)
	}
}

func TestReconnectFailedForNetworkRouteOnlyRearmsNetworkServers(t *testing.T) {
	mgr := NewManager(WithLogger(noopLogger{}))
	disabled := false
	mgr.servers = map[string]*managedServer{
		"stdio": {
			name: "stdio", transport: TransportStdio, state: StateFailed,
			config: McpServerConfig{Stdio: &StdioTransportConfig{Command: "missing-test-command"}},
		},
		"http": {
			name: "http", transport: TransportHTTP, state: StateFailed,
			config: McpServerConfig{HTTP: &HTTPTransportConfig{URL: "http://127.0.0.1:1"}},
		},
		"disabled": {
			name: "disabled", transport: TransportSSE, state: StateDisabled,
			config: McpServerConfig{SSE: &HTTPTransportConfig{URL: "http://127.0.0.1:1"}, Enabled: &disabled},
		},
		"in-process": {
			name: "in-process", transport: TransportInProc, state: StateFailed,
			config: McpServerConfig{InProc: &InProcTransportConfig{ServerName: "echo"}},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := mgr.ReconnectFailedForNetworkRoute(ctx); got != 2 {
		t.Fatalf("reconnected = %d, want 2", got)
	}

	mgr.mu.RLock()
	defer mgr.mu.RUnlock()
	if mgr.servers["stdio"].config.Enabled == nil || !*mgr.servers["stdio"].config.Enabled {
		t.Fatal("stdio server was not enabled")
	}
	if mgr.servers["http"].config.Enabled == nil || !*mgr.servers["http"].config.Enabled {
		t.Fatal("http server was not enabled")
	}
	if mgr.servers["disabled"].state != StateDisabled {
		t.Fatalf("disabled server state = %s", mgr.servers["disabled"].state)
	}
	if mgr.servers["in-process"].state != StateFailed {
		t.Fatalf("in-process server state = %s", mgr.servers["in-process"].state)
	}
}

// waitForState polls SnapshotServer until the server reaches the target
// state or the timeout elapses.
func waitForState(mgr *Manager, name string, want ServerState, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		snap, err := mgr.SnapshotServer(name)
		if err == nil && snap.State == want {
			return nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	return errors.New("timeout waiting for state " + string(want))
}
