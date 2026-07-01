package mcpclient

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Session is the handle a connected server exposes. It is satisfied by the
// SDK's *mcp.ClientSession so the manager can mock it in tests.
type Session interface {
	ListTools(ctx context.Context, params *mcp.ListToolsParams) (*mcp.ListToolsResult, error)
	CallTool(ctx context.Context, params *mcp.CallToolParams) (*mcp.CallToolResult, error)
	ListPrompts(ctx context.Context, params *mcp.ListPromptsParams) (*mcp.ListPromptsResult, error)
	GetPrompt(ctx context.Context, params *mcp.GetPromptParams) (*mcp.GetPromptResult, error)
	ListResources(ctx context.Context, params *mcp.ListResourcesParams) (*mcp.ListResourcesResult, error)
	Close() error
	Wait() error
}

// transportFactory builds an SDK transport from a server config. It exists
// as a single function so the manager does not need to know which variant
// is in use; the factory inspects the config and dispatches. onInProcPanic
// is an optional callback invoked when an in-proc server factory panics;
// it receives the formatted panic message.
func buildTransport(cfg McpServerConfig, registry *InProcRegistry, authHandler auth.OAuthHandler, onInProcPanic func(string)) (mcp.Transport, TransportKind, error) {
	kind := cfg.TransportKind()
	switch kind {
	case TransportStdio:
		if strings.TrimSpace(cfg.Stdio.Command) == "" {
			return nil, TransportStdio, fmt.Errorf("stdio transport requires a non-empty command")
		}
		cmd := exec.Command(cfg.Stdio.Command, cfg.Stdio.Args...)
		if cfg.Stdio.Cwd != "" {
			cmd.Dir = cfg.Stdio.Cwd
		}
		if len(cfg.Stdio.Env) > 0 {
			cmd.Env = append(cmd.Env, envSlice(cfg.Stdio.Env)...)
		}
		return &mcp.CommandTransport{Command: cmd}, TransportStdio, nil
	case TransportHTTP:
		httpClient := buildHTTPClient(cfg.InsecureSkipVerify)
		t := &mcp.StreamableClientTransport{
			Endpoint:   cfg.HTTP.URL,
			HTTPClient: httpClient,
		}
		if authHandler != nil {
			t.OAuthHandler = authHandler
		}
		return t, TransportHTTP, nil
	case TransportSSE:
		httpClient := buildHTTPClient(cfg.InsecureSkipVerify)
		return &mcp.SSEClientTransport{
			Endpoint:   cfg.SSE.URL,
			HTTPClient: httpClient,
		}, TransportSSE, nil
	case TransportWS:
		// The official Go SDK does not ship a WebSocket client transport.
		// Awaken treats ws as a "configured but unsupported" transport in v1
		// and surfaces a clear error so the manager marks the server failed
		// rather than silently degrading. A future change may add a WebSocket
		// transport via golang.org/x/net/websocket or similar.
		return nil, TransportWS, fmt.Errorf("ws transport is not supported in v1; use http or sse")
	case TransportInProc:
		if registry == nil {
			return nil, TransportInProc, fmt.Errorf("in-proc registry not configured")
		}
		factory, err := registry.Lookup(cfg.InProc.ServerName)
		if err != nil {
			return nil, TransportInProc, err
		}
		clientTransport, serverTransport := mcp.NewInMemoryTransports()
		go runInProcServer(factory, serverTransport, onInProcPanic)
		return clientTransport, TransportInProc, nil
	}
	return nil, "", fmt.Errorf("invalid transport config")
}

// buildHTTPClient returns an *http.Client honouring the insecureSkipVerify
// flag. The timeout matches the agent's default tool-call timeout.
func buildHTTPClient(insecure bool) *http.Client {
	client := &http.Client{
		Timeout: 60 * time.Second,
	}
	if insecure {
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}
	return client
}

// connectAndInitialize performs the MCP handshake and returns the live
// session plus the negotiated capabilities. The ToolListChangedHandler,
// PromptListChangedHandler, and ResourceListChangedHandler are attached at
// the Client level per SDK design.
func connectAndInitialize(
	ctx context.Context,
	transport mcp.Transport,
	clientImpl *mcp.Implementation,
	onToolListChanged func(),
	onPromptListChanged func(),
	onResourceListChanged func(),
) (Session, *ServerCapability, error) {
	clientOpts := &mcp.ClientOptions{}
	if onToolListChanged != nil {
		clientOpts.ToolListChangedHandler = func(ctx context.Context, req *mcp.ToolListChangedRequest) {
			onToolListChanged()
		}
	}
	if onPromptListChanged != nil {
		clientOpts.PromptListChangedHandler = func(ctx context.Context, req *mcp.PromptListChangedRequest) {
			onPromptListChanged()
		}
	}
	if onResourceListChanged != nil {
		clientOpts.ResourceListChangedHandler = func(ctx context.Context, req *mcp.ResourceListChangedRequest) {
			onResourceListChanged()
		}
	}
	client := mcp.NewClient(clientImpl, clientOpts)
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("connect: %w", err)
	}
	initResult := session.InitializeResult()
	capability := &ServerCapability{}
	if initResult != nil {
		if initResult.ServerInfo != nil {
			capability.Name = initResult.ServerInfo.Name
			capability.Version = initResult.ServerInfo.Version
		}
		capability.ProtocolVersion = initResult.ProtocolVersion
		if initResult.Capabilities != nil {
			capability.SupportsTools = initResult.Capabilities.Tools != nil
			capability.SupportsPrompts = initResult.Capabilities.Prompts != nil
			capability.SupportsResources = initResult.Capabilities.Resources != nil
		}
	}
	return session, capability, nil
}

// isAuthError inspects an SDK error to decide whether the owning server
// should transition to needs-auth. The SDK surfaces 401s as plain
// fmt.Errorf-wrapped strings during Connect and as JSON-RPC errors during
// CallTool; we conservatively match by message text.
func isAuthError(err error) bool {
	if err == nil {
		return false
	}
	if IsNeedsAuth(err) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "401") ||
		strings.Contains(msg, "unauthorized") ||
		strings.Contains(msg, "needs-auth") ||
		strings.Contains(msg, "needs auth")
}

// ErrTransportClosed is returned by Session wrappers after the manager has
// shut down a server.
var ErrTransportClosed = errors.New("mcp transport closed")

func envSlice(env map[string]string) []string {
	out := make([]string, 0, len(env))
	for k, v := range env {
		out = append(out, k+"="+v)
	}
	return out
}
