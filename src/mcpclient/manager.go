package mcpclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"awaken/src/tool"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// toolRegisterForce and toolUnregisterByServer are package-level
// indirections over the tool registry so tests can substitute fakes.
// The default values route to the real registry in awaken/src/tool.
var (
	toolRegisterForce      = func(t tool.Tool) error { return tool.RegisterToolForce(t) }
	toolUnregisterByServer = tool.UnregisterMCPToolsByServer
	toolRegistryHooksMu    sync.RWMutex
)

func registerToolThroughHook(value tool.Tool) error {
	toolRegistryHooksMu.RLock()
	hook := toolRegisterForce
	toolRegistryHooksMu.RUnlock()
	return hook(value)
}

func unregisterToolsThroughHook(serverName string) int {
	toolRegistryHooksMu.RLock()
	hook := toolUnregisterByServer
	toolRegistryHooksMu.RUnlock()
	return hook(serverName)
}

func setToolRegistryHooksForTest(register func(tool.Tool) error, unregister func(string) int) func() {
	toolRegistryHooksMu.Lock()
	previousRegister := toolRegisterForce
	previousUnregister := toolUnregisterByServer
	if register != nil {
		toolRegisterForce = register
	}
	if unregister != nil {
		toolUnregisterByServer = unregister
	}
	toolRegistryHooksMu.Unlock()
	return func() {
		toolRegistryHooksMu.Lock()
		toolRegisterForce = previousRegister
		toolUnregisterByServer = previousUnregister
		toolRegistryHooksMu.Unlock()
	}
}

// Manager owns the lifecycle of every MCP server connection. It is the
// single entry point for the server layer and is safe for concurrent use.
type Manager struct {
	logger   Logger
	registry *InProcRegistry

	mu      sync.RWMutex
	servers map[string]*managedServer
	// onToolsChanged is invoked whenever any server's tool pool changes —
	// either because a server connected, disconnected, or sent a
	// tools/list_changed notification. The callback lets the tool registry
	// refresh its MCP-sourced entries without coupling to the manager.
	onToolsChanged func(serverName string)
	clientImpl     *mcp.Implementation
	// authFetchers holds the per-server AuthorizationCodeFetcher installed
	// by the desktop UI. Populated via Manager.InstallAuthFetcher.
	authFetchers map[string]auth.AuthorizationCodeFetcher
	// credentials holds per-server OAuth token sources. Defaults to an
	// in-memory store; persistent stores can be installed via
	// Manager.SetCredentialStore.
	credentials CredentialStore
	// elicitationHandler bridges server-initiated MCP form/URL requests into
	// the active interactive run. Calls without an interactive run are safely
	// cancelled by the host handler.
	elicitationHandler ElicitationHandler
	// subscribers is the set of active ServerEvent subscribers. Each
	// subscriber has a buffered channel that receives ServerEvent values.
	// The channel is closed when the subscriber's context is cancelled.
	subscribers   map[chan ServerEvent]context.Context
	subscribersMu sync.Mutex

	// nextSeq is a monotonic counter used to preserve the order in which servers
	// were added to the manager. It is incremented atomically when a server is
	// created so Snapshot can return servers in installation order.
	nextSeq atomic.Uint64
}

// managedServer holds the per-server mutable state under the manager's lock.
type managedServer struct {
	name        string
	scope       Scope
	config      McpServerConfig
	transport   TransportKind
	state       ServerState
	capability  *ServerCapability
	errorReason string
	lastConn    *time.Time

	// seq records the order in which this server was added to the manager.
	seq uint64

	// sourceConnectorID is set when this server was injected by a connector
	// via InjectDynamic. It holds the connector's ID.
	sourceConnectorID string

	// shadowedBy is set when another server with higher priority covers the
	// same name. Holds the winning connector ID.
	shadowedBy  string
	tools       []*mcp.Tool
	toolsLoaded bool

	session     Session
	cancelCtx   context.CancelFunc
	authHandler auth.OAuthHandler
	// running guards the connect goroutine so we never double-start.
	running bool
	// MCP does not include a parent tool-call id on elicitation/create. Calls
	// are serialized per server so an incoming elicitation can be bound to the
	// exact active CallTool context without cross-run leakage.
	callMu            sync.Mutex
	elicitationMu     sync.RWMutex
	activeCallContext context.Context
}

// ManagerOption configures a Manager at construction.
type ManagerOption func(*Manager)

// WithLogger injects a logger. Defaults to a noop logger.
func WithLogger(l Logger) ManagerOption {
	return func(m *Manager) { m.logger = l }
}

// WithInProcRegistry injects the registry used to resolve in-process
// transports. Without it, in-process servers fail to connect.
func WithInProcRegistry(r *InProcRegistry) ManagerOption {
	return func(m *Manager) { m.registry = r }
}

// WithClientImpl overrides the client implementation advertised in the
// MCP initialize handshake. Defaults to a generic "awaken" client.
func WithClientImpl(impl *mcp.Implementation) ManagerOption {
	return func(m *Manager) { m.clientImpl = impl }
}

// WithToolsChangedCallback registers a callback invoked whenever a server's
// tool pool changes. The callback receives the affected server name.
func WithToolsChangedCallback(fn func(serverName string)) ManagerOption {
	return func(m *Manager) { m.onToolsChanged = fn }
}

// NewManager builds an empty manager. Servers are not started until
// Start is called.
func NewManager(opts ...ManagerOption) *Manager {
	m := &Manager{
		logger:      noopLogger{},
		servers:     make(map[string]*managedServer),
		credentials: NewInMemoryCredentialStore(),
	}
	for _, opt := range opts {
		opt(m)
	}
	if m.clientImpl == nil {
		m.clientImpl = &mcp.Implementation{Name: "awaken", Version: "0.1.0"}
	}
	return m
}

// SetCredentialStore replaces the default in-memory credential store.
// Used by downstream changes that persist OAuth tokens across sessions.
func (m *Manager) SetCredentialStore(store CredentialStore) {
	if store == nil {
		return
	}
	m.mu.Lock()
	m.credentials = store
	m.mu.Unlock()
}

func (m *Manager) SetElicitationHandler(handler ElicitationHandler) {
	m.mu.Lock()
	m.elicitationHandler = handler
	m.mu.Unlock()
}

// nextServerSeq returns a monotonically increasing sequence number for a newly
// created server. Callers must still assign the value to the managedServer.
func (m *Manager) nextServerSeq() uint64 {
	return m.nextSeq.Add(1)
}

// Start kicks off connection attempts for every loaded server in parallel.
// Servers that fail to start are recorded in the failed state; the manager
// continues with the rest.
func (m *Manager) Start(ctx context.Context, servers []ScopedMcpServerConfig) {
	var wg sync.WaitGroup
	for _, scoped := range servers {
		seq := m.nextServerSeq()
		wg.Add(1)
		go func(scoped ScopedMcpServerConfig, seq uint64) {
			defer wg.Done()
			m.startOne(ctx, scoped, seq)
		}(scoped, seq)
	}
	wg.Wait()
}

// startOne wires a single server into the manager and kicks off the
// connect goroutine. The function returns once the initial state has been
// recorded (pending, or failed if the config is unusable).
func (m *Manager) startOne(ctx context.Context, scoped ScopedMcpServerConfig, seq uint64) {
	kind := scoped.Config.TransportKind()
	if kind == "" {
		m.mu.Lock()
		m.servers[scoped.Name] = &managedServer{
			name:        scoped.Name,
			scope:       scoped.Scope,
			config:      scoped.Config,
			state:       StateFailed,
			errorReason: "invalid transport config",
			seq:         seq,
		}
		m.mu.Unlock()
		m.logger.Warnf("mcp server %q has invalid transport, marking failed", scoped.Name)
		return
	}
	if scoped.Config.Enabled != nil && !*scoped.Config.Enabled {
		m.mu.Lock()
		m.servers[scoped.Name] = &managedServer{
			name:      scoped.Name,
			scope:     scoped.Scope,
			config:    scoped.Config,
			transport: kind,
			state:     StateDisabled,
			seq:       seq,
		}
		m.mu.Unlock()
		m.logger.Debugf("mcp server %q restored in disabled state", scoped.Name)
		return
	}
	if kind == TransportWS {
		m.mu.Lock()
		m.servers[scoped.Name] = &managedServer{
			name:        scoped.Name,
			scope:       scoped.Scope,
			config:      scoped.Config,
			transport:   kind,
			state:       StateFailed,
			errorReason: "ws transport not supported in v1",
			seq:         seq,
		}
		m.mu.Unlock()
		m.logger.Warnf("mcp server %q uses unsupported ws transport, marking failed", scoped.Name)
		return
	}

	server := &managedServer{
		name:      scoped.Name,
		scope:     scoped.Scope,
		config:    scoped.Config,
		transport: kind,
		state:     StatePending,
		seq:       seq,
	}
	m.mu.Lock()
	m.servers[scoped.Name] = server
	m.mu.Unlock()

	go m.connectLoop(ctx, server)
}

// connectLoop drives the state machine for a single server. It attempts
// the handshake with exponential backoff (1s, 2s, 4s, 8s, 16s) up to 5
// times before transitioning to failed.
func (m *Manager) connectLoop(parentCtx context.Context, server *managedServer) {
	ctx, cancel := context.WithCancel(parentCtx)
	defer cancel()

	m.mu.Lock()
	if server.running {
		m.mu.Unlock()
		return
	}
	server.running = true
	server.cancelCtx = cancel
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		server.running = false
		m.mu.Unlock()
	}()

	if server.config.InsecureSkipVerify {
		m.logger.Warnf("mcp server %q has insecureSkipVerify enabled; TLS certificate verification is disabled", server.name)
	}

	backoff := 1 * time.Second
	maxAttempts := 5
	m.mu.Lock()
	authHandler := server.authHandler
	if authHandler == nil && serverAuthURL(server.config) != "" {
		authHandler = newPersistentOAuthHandler(CredentialIdentity(server.scope, server.name, server.config), AuthFlowConfig{
			ServerURL: serverAuthURL(server.config),
		}, m.credentials)
		server.authHandler = authHandler
	}
	m.mu.Unlock()
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		transport, _, err := buildTransport(server.config, m.registry, authHandler, func(panicMsg string) {
			m.recordFailure(server, fmt.Errorf("%s", panicMsg))
		})
		if err != nil {
			m.recordFailure(server, fmt.Errorf("build transport: %w", err))
			return
		}
		onToolListChanged := func() {
			m.logger.Debugf("mcp server %q signalled tools/list_changed", server.name)
			if err := m.RefreshTools(ctx, server.name); err != nil {
				m.logger.Warnf("refresh tools for %q failed: %v", server.name, err)
			}
			if m.onToolsChanged != nil {
				m.onToolsChanged(server.name)
			}
		}
		onPromptListChanged := func() {
			m.logger.Debugf("mcp server %q signalled prompts/list_changed", server.name)
			m.emitEvent(ServerEvent{Type: EventPromptsListChanged, ServerName: server.name})
		}
		onResourceListChanged := func() {
			m.logger.Debugf("mcp server %q signalled resources/list_changed", server.name)
			m.emitEvent(ServerEvent{Type: EventResourcesListChanged, ServerName: server.name})
		}
		onElicitation := func(_ context.Context, request *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			return m.handleElicitation(server, request)
		}
		session, capability, err := connectAndInitialize(ctx, transport, m.clientImpl, onToolListChanged, onPromptListChanged, onResourceListChanged, onElicitation)
		if err == nil {
			m.mu.Lock()
			server.session = session
			server.capability = capability
			server.state = StateConnected
			server.errorReason = ""
			now := time.Now().UTC()
			server.lastConn = &now
			m.mu.Unlock()
			m.logger.Debugf("mcp server %q connected", server.name)
			if err := m.registerTools(ctx, server); err != nil {
				m.logger.Warnf("mcp server %q tool registration failed: %v", server.name, err)
			}
			if m.onToolsChanged != nil {
				m.onToolsChanged(server.name)
			}
			m.emitEvent(ServerEvent{Type: EventConnected, ServerName: server.name})
			// Block until the session terminates.
			_ = session.Wait()
			m.mu.Lock()
			server.session = nil
			server.tools = nil
			server.toolsLoaded = false
			if server.state == StateConnected {
				server.state = StateFailed
				server.errorReason = "session ended"
			}
			m.mu.Unlock()
			m.unregisterTools(server.name)
			if m.onToolsChanged != nil {
				m.onToolsChanged(server.name)
			}
			m.emitEvent(ServerEvent{Type: EventDisconnected, ServerName: server.name})
			return
		}
		if isAuthError(err) {
			m.mu.Lock()
			server.session = nil
			server.tools = nil
			server.toolsLoaded = false
			server.state = StateNeedsAuth
			server.errorReason = err.Error()
			m.mu.Unlock()
			m.logger.Warnf("mcp server %q needs auth: %v", server.name, err)
			if m.onToolsChanged != nil {
				m.onToolsChanged(server.name)
			}
			m.emitEvent(ServerEvent{Type: EventDisconnected, ServerName: server.name})
			return
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			m.recordFailure(server, err)
			return
		}
		m.logger.Warnf("mcp server %q connect attempt %d failed: %v", server.name, attempt, err)
		if attempt == maxAttempts {
			m.recordFailure(server, fmt.Errorf("connect failed after %d attempts: %w", attempt, err))
			return
		}
		select {
		case <-ctx.Done():
			m.recordFailure(server, ctx.Err())
			return
		case <-time.After(backoff):
		}
		backoff *= 2
	}
}

func (m *Manager) recordFailure(server *managedServer, err error) {
	m.mu.Lock()
	if server.state == StateDisabled {
		m.mu.Unlock()
		return
	}
	server.session = nil
	server.tools = nil
	server.toolsLoaded = false
	server.state = StateFailed
	server.errorReason = err.Error()
	m.mu.Unlock()
	m.logger.Warnf("mcp server %q marked failed: %v", server.name, err)
	if m.onToolsChanged != nil {
		m.onToolsChanged(server.name)
	}
	m.emitEvent(ServerEvent{Type: EventDisconnected, ServerName: server.name})
}

// Disable moves a server to the disabled state and closes its session.
// The original config is preserved so Enable can restore the server.
func (m *Manager) Disable(name string) error {
	m.mu.Lock()
	server, ok := m.servers[name]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("%w: %q", ErrServerNotFound, name)
	}
	if server.state == StateDisabled {
		m.mu.Unlock()
		return nil
	}
	if server.cancelCtx != nil {
		server.cancelCtx()
	}
	if server.session != nil {
		_ = server.session.Close()
		server.session = nil
	}
	server.tools = nil
	server.toolsLoaded = false
	server.state = StateDisabled
	server.errorReason = ""
	disabled := false
	server.config.Enabled = &disabled
	m.mu.Unlock()
	m.unregisterTools(name)
	if m.onToolsChanged != nil {
		m.onToolsChanged(name)
	}
	m.emitEvent(ServerEvent{Type: EventDisabled, ServerName: name})
	return nil
}

// Enable moves a previously-disabled server back to pending and starts a
// fresh connect loop. The server config must still be present.
func (m *Manager) Enable(ctx context.Context, name string) error {
	m.mu.Lock()
	server, ok := m.servers[name]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("%w: %q", ErrServerNotFound, name)
	}
	if server.state != StateDisabled && server.state != StateFailed {
		m.mu.Unlock()
		return fmt.Errorf("%w: server %q is in state %s; enable requires disabled or failed", ErrIllegalState, name, server.state)
	}
	server.state = StatePending
	server.errorReason = ""
	server.running = false
	enabled := true
	server.config.Enabled = &enabled
	m.mu.Unlock()
	go m.connectLoop(ctx, server)
	return nil
}

// Reconnect is a convenience that re-arms a failed server. Equivalent to
// Enable for failed servers; for needs-auth servers it returns an error
// because the user must complete the auth flow first. Reconnect on a server
// already in pending state is a no-op; wait for the initial connect loop to
// progress.
func (m *Manager) Reconnect(ctx context.Context, name string) error {
	m.mu.Lock()
	server, ok := m.servers[name]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("%w: %q", ErrServerNotFound, name)
	}
	state := server.state
	m.mu.Unlock()
	if state == StateNeedsAuth {
		return fmt.Errorf("mcp server %q needs auth; complete the auth flow first", name)
	}
	if state != StateFailed && state != StateDisabled {
		return nil
	}
	return m.Enable(ctx, name)
}

// ReconnectFailedForNetworkRoute re-arms failed network-sensitive servers
// after the host reports a route change. Pending servers already rebuild their
// transport on every retry, and connected servers keep their current session;
// neither is interrupted. In-process servers do not depend on host networking.
func (m *Manager) ReconnectFailedForNetworkRoute(ctx context.Context) int {
	m.mu.RLock()
	names := make([]string, 0)
	for name, server := range m.servers {
		if server.state != StateFailed {
			continue
		}
		switch server.transport {
		case TransportStdio, TransportHTTP, TransportSSE:
			names = append(names, name)
		}
	}
	m.mu.RUnlock()
	sort.Strings(names)

	reconnected := 0
	for _, name := range names {
		if err := m.Reconnect(ctx, name); err != nil {
			m.logger.Warnf("mcp server %q network-route reconnect failed: %v", name, err)
			continue
		}
		reconnected++
	}
	return reconnected
}

// RestartWithConfig stops an existing server (if any), unregisters its tools,
// and starts a fresh connection loop with the provided config. Used when the
// operator edits a server's configuration from the UI.
//
// The server entry stays in the manager map for the whole rebuild so Snapshot
// never briefly omits it. Catalog UIs that poll list endpoints must not flash
// an empty "user installed" group during enable/disable or config restart.
func (m *Manager) RestartWithConfig(ctx context.Context, scoped ScopedMcpServerConfig) error {
	m.mu.Lock()
	server, exists := m.servers[scoped.Name]
	var seq uint64
	if exists {
		seq = server.seq
		if server.cancelCtx != nil {
			server.cancelCtx()
		}
		if server.session != nil {
			_ = server.session.Close()
			server.session = nil
		}
	}
	m.mu.Unlock()

	if exists {
		m.unregisterTools(scoped.Name)
		if m.onToolsChanged != nil {
			m.onToolsChanged(scoped.Name)
		}
		m.emitEvent(ServerEvent{Type: EventDisconnected, ServerName: scoped.Name})
		m.startOne(ctx, scoped, seq)
		return nil
	}

	m.Start(ctx, []ScopedMcpServerConfig{scoped})
	return nil
}

// Snapshot returns the current state of every server, sorted by installation
// order (the order in which they were added to the manager).
func (m *Manager) Snapshot() []ServerSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	servers := make([]*managedServer, 0, len(m.servers))
	for _, server := range m.servers {
		servers = append(servers, server)
	}
	sort.Slice(servers, func(i, j int) bool { return servers[i].seq < servers[j].seq })
	out := make([]ServerSnapshot, 0, len(servers))
	for _, server := range servers {
		out = append(out, m.snapshotServerLocked(server))
	}
	return out
}

// SnapshotServer returns the state of a single server by name, or
// ErrServerNotFound.
func (m *Manager) SnapshotServer(name string) (ServerSnapshot, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	server, ok := m.servers[name]
	if !ok {
		return ServerSnapshot{}, fmt.Errorf("mcp server %q not found", name)
	}
	return m.snapshotServerLocked(server), nil
}

// ServerTools returns the last successful tools/list projection for one
// server, including host-disabled tools that are intentionally absent from
// the central registry.
func (m *Manager) ServerTools(name string) ([]ServerToolSnapshot, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	server, ok := m.servers[name]
	if !ok {
		return nil, fmt.Errorf("mcp server %q not found", name)
	}
	disabled := make(map[string]bool, len(server.config.DisabledTools))
	for _, toolName := range server.config.DisabledTools {
		disabled[toolName] = true
	}
	out := make([]ServerToolSnapshot, 0, len(server.tools))
	for _, item := range server.tools {
		if item == nil {
			continue
		}
		out = append(out, ServerToolSnapshot{Name: item.Name, Description: item.Description, Enabled: !disabled[item.Name]})
	}
	return out, nil
}

func (m *Manager) snapshotServerLocked(server *managedServer) ServerSnapshot {
	snap := ServerSnapshot{
		Name:        server.name,
		DisplayName: server.config.DisplayName,
		State:       server.state,
		Transport:   server.transport,
		Scope:       server.scope,
		ErrorReason: server.errorReason,
		ShadowedBy:  server.shadowedBy,
		ToolsLoaded: server.toolsLoaded,
		ToolCount:   len(server.tools),
	}
	if server.sourceConnectorID != "" {
		cid := server.sourceConnectorID
		snap.SourceConnectorID = &cid
	}
	if server.lastConn != nil {
		t := *server.lastConn
		snap.LastConnectedAt = &t
	}
	if server.capability != nil {
		cap := *server.capability
		snap.Capabilities = &cap
	}
	return snap
}

// Session returns the live session for a connected server. Returns an
// error if the server is not in the connected state.
func (m *Manager) Session(name string) (Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	server, ok := m.servers[name]
	if !ok {
		return nil, fmt.Errorf("mcp server %q not found", name)
	}
	if server.state != StateConnected || server.session == nil {
		return nil, &NeedsAuthError{
			ServerName: name,
			Reason:     "server is " + string(server.state) + ", not connected",
		}
	}
	return server.session, nil
}

// CallTool is a convenience that resolves the server owning a tool name
// (encoded as mcp__{server}__{tool}) and forwards the call. It returns
// NeedsAuthError when the server is in the needs-auth state so the agent
// layer can translate it to the needs_auth stage.
func (m *Manager) CallTool(ctx context.Context, prefixedName string, arguments map[string]any) (*mcp.CallToolResult, error) {
	serverName, toolName, ok := m.resolveMcpToolName(prefixedName)
	if !ok {
		return nil, &ToolNotFoundError{ToolName: prefixedName}
	}
	return m.callServerTool(ctx, serverName, toolName, prefixedName, arguments)
}

func (m *Manager) callServerTool(ctx context.Context, serverName, toolName, displayName string, arguments map[string]any) (*mcp.CallToolResult, error) {
	m.mu.RLock()
	server, exists := m.servers[serverName]
	m.mu.RUnlock()
	if !exists {
		return nil, &ToolNotFoundError{ToolName: displayName}
	}
	if server.state == StateNeedsAuth {
		return nil, &NeedsAuthError{ServerName: serverName, Reason: server.errorReason}
	}
	if server.state != StateConnected || server.session == nil {
		return nil, fmt.Errorf("mcp server %q is %s; cannot call %s", serverName, server.state, displayName)
	}
	server.callMu.Lock()
	defer server.callMu.Unlock()
	server.elicitationMu.Lock()
	server.activeCallContext = ctx
	server.elicitationMu.Unlock()
	defer func() {
		server.elicitationMu.Lock()
		server.activeCallContext = nil
		server.elicitationMu.Unlock()
	}()
	return server.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      toolName,
		Arguments: arguments,
	})
}

func (m *Manager) handleElicitation(server *managedServer, request *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
	if server == nil || request == nil || request.Params == nil {
		return &mcp.ElicitResult{Action: "cancel"}, nil
	}
	server.elicitationMu.RLock()
	callContext := server.activeCallContext
	server.elicitationMu.RUnlock()
	m.mu.RLock()
	handler := m.elicitationHandler
	m.mu.RUnlock()
	if callContext == nil {
		m.logger.Warnf("mcp server %q requested elicitation without an active tool call", server.name)
		return &mcp.ElicitResult{Action: "cancel"}, nil
	}
	var schema map[string]any
	if request.Params.RequestedSchema != nil {
		raw, err := json.Marshal(request.Params.RequestedSchema)
		if err != nil {
			return nil, fmt.Errorf("encode MCP elicitation schema: %w", err)
		}
		if err := json.Unmarshal(raw, &schema); err != nil {
			return nil, fmt.Errorf("decode MCP elicitation schema: %w", err)
		}
	}
	mode := strings.TrimSpace(request.Params.Mode)
	if mode == "" {
		mode = "form"
	}
	if local, ok := tool.ElicitationHandlerFromContext(callContext); ok {
		m.logger.Debugf("mcp server %q routed elicitation to the active run", server.name)
		answer, err := local(callContext, tool.ElicitationRequest{
			Provider: server.name, Mode: mode, Message: request.Params.Message,
			RequestedSchema: schema, URL: request.Params.URL, RequestID: request.Params.ElicitationID,
		})
		if err != nil {
			return nil, err
		}
		return &mcp.ElicitResult{Action: answer.Action, Content: answer.Content}, nil
	}
	if handler == nil {
		m.logger.Warnf("mcp server %q requested elicitation outside an interactive run", server.name)
		return &mcp.ElicitResult{Action: "cancel"}, nil
	}
	m.logger.Debugf("mcp server %q routed elicitation to the host fallback", server.name)
	result, err := handler(callContext, ElicitationRequest{
		ServerName: server.name, Mode: mode, Message: request.Params.Message,
		RequestedSchema: schema, URL: request.Params.URL, ElicitationID: request.Params.ElicitationID,
	})
	if err != nil {
		return nil, err
	}
	return &mcp.ElicitResult{Action: result.Action, Content: result.Content}, nil
}

// resolveMcpToolName uses the live server registry to disambiguate server
// identities that themselves contain "__" (notably Agent Plugin identities).
// The longest matching model-facing server prefix wins, and the final
// component is decoded back to the exact MCP-authored tool name.
func (m *Manager) resolveMcpToolName(prefixed string) (server, toolName string, ok bool) {
	if !strings.HasPrefix(prefixed, "mcp__") {
		return "", "", false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	matchedPrefixLength := 0
	for candidate := range m.servers {
		prefix := McpToolPrefix(candidate)
		if !strings.HasPrefix(prefixed, prefix) || len(prefix) <= matchedPrefixLength {
			continue
		}
		decodedToolName, decoded := decodeMCPNameComponent(strings.TrimPrefix(prefixed, prefix))
		if decoded {
			server = candidate
			toolName = decodedToolName
			matchedPrefixLength = len(prefix)
		}
	}
	if server != "" && toolName != "" {
		return server, toolName, true
	}

	// Compatibility for callers that persisted or constructed the historical
	// raw-qualified mcp__<raw server>__<raw tool> spelling.
	rest := strings.TrimPrefix(prefixed, "mcp__")
	matchedPrefixLength = 0
	for candidate := range m.servers {
		prefix := candidate + "__"
		if strings.HasPrefix(rest, prefix) && len(prefix) > matchedPrefixLength {
			server = candidate
			toolName = strings.TrimPrefix(rest, prefix)
			matchedPrefixLength = len(prefix)
		}
	}
	return server, toolName, server != "" && toolName != ""
}

// InjectDynamic creates or replaces a server entry in the manager from a
// connector. The server is tagged with scope dynamic and records the connector
// ID. If a server with the same name already exists that was injected by a
// different connector, the existing entry is shadowed (its shadowedBy field is
// set) and the new one takes its place.
//
// InjectDynamic is called by the connector registry after a user chooses to
// connect a connector. Unlike Start, it starts the connect loop inside a
// goroutine so the server appears in snapshots after the first state change.
func (m *Manager) InjectDynamic(connectorID string, config ScopedMcpServerConfig) {
	m.mu.Lock()
	existing, exists := m.servers[config.Name]
	if exists {
		// If the existing entry was injected by a connector with different ID,
		// shadow it.
		if existing.sourceConnectorID != "" && existing.sourceConnectorID != connectorID {
			existing.shadowedBy = connectorID
			m.logger.Warnf("mcp server %q (connector %s) shadowed by connector %s", config.Name, existing.sourceConnectorID, connectorID)
		}
		// Shut down the existing server before replacing.
		if existing.cancelCtx != nil {
			existing.cancelCtx()
		}
		if existing.session != nil {
			_ = existing.session.Close()
		}
	}
	// Unregister any existing tools for this server name before replacing.
	if exists {
		m.unregisterTools(config.Name)
	}
	server := &managedServer{
		name:              config.Name,
		scope:             ScopeDynamic,
		config:            config.Config,
		transport:         config.Config.TransportKind(),
		state:             StatePending,
		seq:               m.nextServerSeq(),
		sourceConnectorID: connectorID,
	}
	m.servers[config.Name] = server
	m.mu.Unlock()

	m.logger.Debugf("mcp server %q injected from connector %s", config.Name, connectorID)
	go m.connectLoop(context.Background(), server)
}

// RemoveDynamic removes a server entry added via InjectDynamic. It closes
// the session, unregisters any tools, and removes the entry from the manager.
// The connectorID parameter ensures only the owning connector can remove its
// servers.
func (m *Manager) RemoveDynamic(connectorID, serverName string) {
	m.mu.Lock()
	server, ok := m.servers[serverName]
	if !ok {
		m.mu.Unlock()
		return
	}
	if server.sourceConnectorID != connectorID {
		m.mu.Unlock()
		m.logger.Warnf("refusing to remove server %q: owned by connector %s, not %s", serverName, server.sourceConnectorID, connectorID)
		return
	}
	if server.cancelCtx != nil {
		server.cancelCtx()
	}
	if server.session != nil {
		_ = server.session.Close()
	}
	delete(m.servers, serverName)
	m.mu.Unlock()

	m.unregisterTools(serverName)
	if m.onToolsChanged != nil {
		m.onToolsChanged(serverName)
	}
	m.logger.Debugf("mcp server %q removed (connector %s)", serverName, connectorID)
}

// Remove permanently stops a server, closes its session, unregisters its tools,
// and removes it from the manager. Used when the operator deletes a server's
// configuration from the UI.
func (m *Manager) Remove(name string) error {
	m.mu.Lock()
	server, exists := m.servers[name]
	if exists {
		if server.cancelCtx != nil {
			server.cancelCtx()
		}
		if server.session != nil {
			_ = server.session.Close()
		}
		delete(m.servers, name)
	}
	m.mu.Unlock()

	if exists {
		m.unregisterTools(name)
		if m.onToolsChanged != nil {
			m.onToolsChanged(name)
		}
		m.emitEvent(ServerEvent{Type: EventDisconnected, ServerName: name})
	}
	return nil
}

// ClearCredentialsFor permanently removes OAuth credentials owned by one
// scoped MCP configuration. Credential ownership includes scope, server name,
// and canonical endpoint, so a project override cannot reuse a user token and
// an endpoint edit cannot inherit credentials from the previous endpoint.
func (m *Manager) ClearCredentialsFor(scoped ScopedMcpServerConfig) error {
	m.mu.RLock()
	store := m.credentials
	m.mu.RUnlock()
	if store == nil || serverAuthURL(scoped.Config) == "" {
		return nil
	}
	return store.ClearTokenSource(CredentialIdentity(scoped.Scope, scoped.Name, scoped.Config))
}

// Shutdown closes every connection. Used by tests and on graceful host
// shutdown. It also closes all subscriber channels.
func (m *Manager) Shutdown(ctx context.Context) {
	m.mu.Lock()
	for _, server := range m.servers {
		if server.cancelCtx != nil {
			server.cancelCtx()
		}
		if server.session != nil {
			_ = server.session.Close()
		}
	}
	m.mu.Unlock()

	m.subscribersMu.Lock()
	for ch := range m.subscribers {
		close(ch)
	}
	m.subscribers = nil
	m.subscribersMu.Unlock()
}

// Subscribe returns a buffered channel that receives ServerEvent values for
// every server lifecycle transition. The channel is closed when ctx is
// cancelled. Slow subscribers may miss events — the channel is non-blocking
// with a buffer of 64 events per subscriber.
func (m *Manager) Subscribe(ctx context.Context) <-chan ServerEvent {
	ch := make(chan ServerEvent, 64)
	m.subscribersMu.Lock()
	if m.subscribers == nil {
		m.subscribers = make(map[chan ServerEvent]context.Context)
	}
	m.subscribers[ch] = ctx
	m.subscribersMu.Unlock()

	// Start a goroutine that closes the channel when ctx is done.
	go func() {
		<-ctx.Done()
		m.subscribersMu.Lock()
		delete(m.subscribers, ch)
		m.subscribersMu.Unlock()
		close(ch)
	}()

	return ch
}

// emitEvent sends a ServerEvent to all active subscribers. Non-blocking:
// if a subscriber's buffer is full, the event is dropped.
func (m *Manager) emitEvent(evt ServerEvent) {
	m.subscribersMu.Lock()
	defer m.subscribersMu.Unlock()

	for ch, ctx := range m.subscribers {
		select {
		case <-ctx.Done():
			// Subscriber context cancelled; skip.
		default:
			select {
			case ch <- evt:
			default:
				// Buffer full, drop event.
			}
		}
	}
}

// registerTools queries the server's current tool list and registers each
// tool into the central tool registry under the mcp__{server}__{tool}
// naming scheme. Existing registrations from the same server are
// replaced atomically.
func (m *Manager) registerTools(ctx context.Context, server *managedServer) error {
	if server.session == nil {
		return fmt.Errorf("server %q has no session", server.name)
	}
	result, err := server.session.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		return fmt.Errorf("list tools: %w", err)
	}
	// Clear any prior registrations for this server before re-registering
	// so the registry reflects exactly the server's current tool list.
	m.unregisterTools(server.name)
	m.mu.Lock()
	server.tools = append([]*mcp.Tool(nil), result.Tools...)
	server.toolsLoaded = true
	m.mu.Unlock()
	disabled := make(map[string]bool, len(server.config.DisabledTools))
	for _, toolName := range server.config.DisabledTools {
		disabled[toolName] = true
	}
	for _, mcpTool := range result.Tools {
		if mcpTool == nil || disabled[mcpTool.Name] {
			continue
		}
		adapter := NewMcpToolAdapter(m, server.name, mcpTool)
		if err := registerToolThroughHook(adapter); err != nil {
			m.logger.Warnf("register mcp tool %q failed: %v", adapter.Name(), err)
			continue
		}
	}
	return nil
}

// unregisterTools removes every tool registered by the named server. It
// is a no-op when the server has no tools registered.
func (m *Manager) unregisterTools(serverName string) {
	unregisterToolsThroughHook(serverName)
}

// RefreshTools re-queries the server's tool list and updates the
// registry. Called when the server emits a tools/list_changed
// notification.
func (m *Manager) RefreshTools(ctx context.Context, serverName string) error {
	m.mu.RLock()
	server, ok := m.servers[serverName]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("mcp server %q not found", serverName)
	}
	if server.state != StateConnected || server.session == nil {
		return fmt.Errorf("mcp server %q is %s, cannot refresh tools", serverName, server.state)
	}
	return m.registerTools(ctx, server)
}
