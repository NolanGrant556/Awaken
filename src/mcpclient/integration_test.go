package mcpclient

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestHTTPTransportEndToEnd spins up a real streamable-HTTP MCP server
// using the SDK's StreamableHTTPHandler, points the manager at it, and
// verifies the full handshake → ListTools → CallTool path through the HTTP
// transport. This is the closest analogue to a real external MCP server we
// can run without an exec'd binary.
func TestHTTPTransportEndToEnd(t *testing.T) {
	serverFactory := func() *mcp.Server {
		s := mcp.NewServer(&mcp.Implementation{Name: "echo-http", Version: "v1"}, nil)
		mcp.AddTool(s, &mcp.Tool{
			Name:        "ping",
			Description: "echoes the message back",
			Annotations: &mcp.ToolAnnotations{
				ReadOnlyHint:    true,
				DestructiveHint: boolPtr(false),
				OpenWorldHint:   boolPtr(false),
			},
		}, func(ctx context.Context, req *mcp.CallToolRequest, input map[string]string) (*mcp.CallToolResult, map[string]string, error) {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "pong:" + input["message"]}},
			}, input, nil
		})
		return s
	}
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return serverFactory()
	}, nil)
	httpSrv := httptest.NewServer(handler)
	defer httpSrv.Close()

	mgr := NewManager(WithLogger(noopLogger{}))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	mgr.Start(ctx, []ScopedMcpServerConfig{
		{Name: "echo-http", Scope: ScopeProject, Config: McpServerConfig{
			HTTP: &HTTPTransportConfig{URL: httpSrv.URL},
		}},
	})
	defer mgr.Shutdown(context.Background())

	if err := waitForState(mgr, "echo-http", StateConnected, 5*time.Second); err != nil {
		snap, _ := mgr.SnapshotServer("echo-http")
		t.Fatalf("echo-http never connected: %v (state=%s reason=%q)", err, snap.State, snap.ErrorReason)
	}
	snap, _ := mgr.SnapshotServer("echo-http")
	if snap.Transport != TransportHTTP {
		t.Errorf("expected http transport, got %s", snap.Transport)
	}

	res, err := mgr.CallTool(ctx, "mcp__echo-http__ping", map[string]any{"message": "hi"})
	if err != nil {
		t.Fatalf("call ping: %v", err)
	}
	if len(res.Content) == 0 {
		t.Fatal("expected non-empty content")
	}
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok || !strings.Contains(tc.Text, "pong:hi") {
		t.Errorf("expected pong:hi, got %+v", res.Content[0])
	}
}

// TestHTTPTransportUnreachable verifies a server pointing at a closed port
// eventually transitions to failed after exhausting retries.
func TestHTTPTransportUnreachable(t *testing.T) {
	// Pick a port that is almost certainly closed by binding and immediately
	// closing a listener.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	addr := srv.URL
	srv.Close()

	mgr := NewManager(WithLogger(noopLogger{}))
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	mgr.Start(ctx, []ScopedMcpServerConfig{
		{Name: "dead", Scope: ScopeProject, Config: McpServerConfig{
			HTTP: &HTTPTransportConfig{URL: addr},
		}},
	})
	// Retries are 1+2+4+8+16 = 31s plus connect time. Wait up to 60s.
	if err := waitForState(mgr, "dead", StateFailed, 70*time.Second); err != nil {
		snap, _ := mgr.SnapshotServer("dead")
		t.Fatalf("dead never failed: %v (state=%s reason=%q)", err, snap.State, snap.ErrorReason)
	}
}

// TestMultipleServersConcurrent verifies the manager can handle multiple
// servers in parallel — one in-proc and one HTTP — without cross-talk.
func TestMultipleServersConcurrent(t *testing.T) {
	// In-proc echo server with one tool.
	registry := NewInProcRegistry()
	if err := registry.Register("inproc", echoServerWithTools("a")); err != nil {
		t.Fatalf("register inproc: %v", err)
	}

	// HTTP echo server with one tool.
	httpFactory := func() *mcp.Server {
		s := mcp.NewServer(&mcp.Implementation{Name: "http-echo", Version: "v1"}, nil)
		mcp.AddTool(s, &mcp.Tool{
			Name:        "b",
			Description: "echoes b",
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: boolPtr(false), OpenWorldHint: boolPtr(false)},
		}, func(ctx context.Context, req *mcp.CallToolRequest, input map[string]string) (*mcp.CallToolResult, map[string]string, error) {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "b:" + input["message"]}}}, input, nil
		})
		return s
	}
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return httpFactory() }, nil)
	httpSrv := httptest.NewServer(handler)
	defer httpSrv.Close()

	mgr := NewManager(WithInProcRegistry(registry), WithLogger(noopLogger{}))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	mgr.Start(ctx, []ScopedMcpServerConfig{
		{Name: "inproc", Scope: ScopeProject, Config: McpServerConfig{
			InProc: &InProcTransportConfig{ServerName: "inproc"},
		}},
		{Name: "http-echo", Scope: ScopeProject, Config: McpServerConfig{
			HTTP: &HTTPTransportConfig{URL: httpSrv.URL},
		}},
	})
	defer mgr.Shutdown(context.Background())

	if err := waitForState(mgr, "inproc", StateConnected, 3*time.Second); err != nil {
		t.Fatalf("inproc never connected: %v", err)
	}
	if err := waitForState(mgr, "http-echo", StateConnected, 5*time.Second); err != nil {
		t.Fatalf("http-echo never connected: %v", err)
	}

	// Both tools should be callable in parallel.
	type callResult struct {
		name string
		err  error
		text string
	}
	results := make(chan callResult, 2)
	go func() {
		res, err := mgr.CallTool(ctx, "mcp__inproc__a", map[string]any{"message": "x"})
		text := ""
		if err == nil && len(res.Content) > 0 {
			if tc, ok := res.Content[0].(*mcp.TextContent); ok {
				text = tc.Text
			}
		}
		results <- callResult{"inproc", err, text}
	}()
	go func() {
		res, err := mgr.CallTool(ctx, "mcp__http-echo__b", map[string]any{"message": "y"})
		text := ""
		if err == nil && len(res.Content) > 0 {
			if tc, ok := res.Content[0].(*mcp.TextContent); ok {
				text = tc.Text
			}
		}
		results <- callResult{"http-echo", err, text}
	}()
	for i := 0; i < 2; i++ {
		r := <-results
		if r.err != nil {
			t.Errorf("%s call failed: %v", r.name, r.err)
		}
		if r.name == "inproc" && !strings.Contains(r.text, "a:x") {
			t.Errorf("expected a:x, got %q", r.text)
		}
		if r.name == "http-echo" && !strings.Contains(r.text, "b:y") {
			t.Errorf("expected b:y, got %q", r.text)
		}
	}
}

// TestStdioTransportMockServer spawns a real stdio MCP server by running
// the test binary itself in a sub-mode that exposes an echo tool over
// stdin/stdout. This is the closest we can get to a real stdio server
// without bundling an external binary.
//
// The sub-mode is gated by AWAKEN_TEST_STDIO_SERVER=1; when set, main_test.go
// in this package registers a stdio MCP server and exits. The test is
// skipped if the binary does not support the sub-mode.
func TestStdioTransportMockServer(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stdio integration test in short mode")
	}
	bin := testBinaryPath(t)
	if bin == "" {
		t.Skip("test binary path unavailable; skipping stdio integration test")
	}

	mgr := NewManager(WithLogger(noopLogger{}))
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	mgr.Start(ctx, []ScopedMcpServerConfig{
		{Name: "stdio-echo", Scope: ScopeProject, Config: McpServerConfig{
			Stdio: &StdioTransportConfig{
				Command: bin,
				Args:    []string{"-test.run=TestStdioHelper", "--"},
				Env:     map[string]string{"AWAKEN_TEST_STDIO_SERVER": "1"},
			},
		}},
	})
	defer mgr.Shutdown(context.Background())

	if err := waitForState(mgr, "stdio-echo", StateConnected, 8*time.Second); err != nil {
		snap, _ := mgr.SnapshotServer("stdio-echo")
		t.Fatalf("stdio-echo never connected: %v (state=%s reason=%q)", err, snap.State, snap.ErrorReason)
	}
	res, err := mgr.CallTool(ctx, "mcp__stdio-echo__echo", map[string]any{"message": "stdio-hi"})
	if err != nil {
		t.Fatalf("call echo: %v", err)
	}
	if len(res.Content) == 0 {
		t.Fatal("expected non-empty content")
	}
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok || !strings.Contains(tc.Text, "stdio-hi") {
		t.Errorf("expected stdio-hi, got %+v", res.Content[0])
	}
}

// testBinaryPath returns the path to the running test binary, or empty if
// it cannot be resolved (e.g. when `go test` has already moved the binary).
func testBinaryPath(t *testing.T) string {
	t.Helper()
	// GO_TEST_PROCESS is set by os/exec tests using TestMain re-exec; we
	// don't have that here. Use os.Args[0] which is the compiled test
	// binary under `go test`.
	if len(osArgs0()) == 0 {
		return ""
	}
	return osArgs0()
}

func osArgs0() string {
	// Indirection so tests can stub it. Default returns the real os.Args[0]
	// via the runtime.
	return defaultArgs0()
}

func defaultArgs0() string {
	// On go test, os.Args[0] is the compiled test binary. We resolve it
	// through a helper to avoid importing os at package level in this file.
	return resolveArgs0()
}

// TestStdioHelper is the sub-mode entry point invoked when the test binary
// is exec'd with -test.run=TestStdioHelper. It is a no-op when the env var
// is unset; when set, it runs an MCP server over stdin/stdout and exits.
func TestStdioHelper(t *testing.T) {
	if !stdioHelperEnabled() {
		t.Skip("stdio helper not requested")
	}
	runStdioHelper(t)
}

func stdioHelperEnabled() bool {
	return resolveEnv("AWAKEN_TEST_STDIO_SERVER") == "1"
}

// runStdioHelper exposes a single "echo" tool over the stdio transport and
// blocks until the client disconnects.
func runStdioHelper(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "stdio-echo", Version: "v1"}, nil)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "echo",
		Description: "echoes input",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: boolPtr(false), OpenWorldHint: boolPtr(false)},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input map[string]string) (*mcp.CallToolResult, map[string]string, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: input["message"]}}}, input, nil
	})
	if err := server.Run(t.Context(), &mcp.StdioTransport{}); err != nil {
		t.Logf("stdio server exited: %v", err)
	}
}

// resolveArgs0 and resolveEnv are split into a separate file so the test
// binary can be exec'd without pulling in os at the top of this one. The
// indirection also lets a future TestMain substitute the values.
func resolveArgs0() string {
	return args0Value
}

func resolveEnv(key string) string {
	return envValue(key)
}

// args0Value is set in init() of helpers_test.go.
var args0Value string

func init() {
	args0Value = lookupArgs0()
}

// fmt is imported for the unused-error guard in TestHTTPTransportUnreachable.
var _ = fmt.Sprintf
