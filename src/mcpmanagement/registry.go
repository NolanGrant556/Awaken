package mcpmanagement

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	ErrServerNotFound  = errors.New("mcp server not found")
	ErrDuplicateServer = errors.New("mcp server already exists")
	ErrBuiltinDelete   = errors.New("builtin mcp server cannot be deleted")
)

type Registry struct {
	mu      sync.RWMutex
	servers map[string]MCPServer
	now     func() time.Time
}

func NewRegistry(initial ...MCPServer) (*Registry, error) {
	registry := &Registry{
		servers: make(map[string]MCPServer),
		now:     time.Now,
	}
	for _, server := range initial {
		if _, err := registry.Add(server); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

func (r *Registry) Add(server MCPServer) (MCPServer, error) {
	normalized, err := NormalizeServer(server)
	if err != nil {
		return MCPServer{}, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.servers[normalized.ID]; exists {
		return MCPServer{}, fmt.Errorf("%w: id %q", ErrDuplicateServer, normalized.ID)
	}
	if existing := r.findByNameLocked(normalized.Name, ""); existing != "" {
		return MCPServer{}, fmt.Errorf("%w: name %q", ErrDuplicateServer, normalized.Name)
	}

	now := r.now().UTC()
	if normalized.CreatedAt.IsZero() {
		normalized.CreatedAt = now
	}
	normalized.UpdatedAt = now
	r.servers[normalized.ID] = cloneServer(normalized)
	return cloneServer(normalized), nil
}

func (r *Registry) Update(server MCPServer) (MCPServer, error) {
	normalized, err := NormalizeServer(server)
	if err != nil {
		return MCPServer{}, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	existing, exists := r.servers[normalized.ID]
	if !exists {
		return MCPServer{}, fmt.Errorf("%w: %s", ErrServerNotFound, normalized.ID)
	}
	if existingID := r.findByNameLocked(normalized.Name, normalized.ID); existingID != "" {
		return MCPServer{}, fmt.Errorf("%w: name %q", ErrDuplicateServer, normalized.Name)
	}

	if existing.Builtin {
		normalized.Builtin = true
	}
	if normalized.CreatedAt.IsZero() {
		normalized.CreatedAt = existing.CreatedAt
	}
	normalized.UpdatedAt = r.now().UTC()
	r.servers[normalized.ID] = cloneServer(normalized)
	return cloneServer(normalized), nil
}

func (r *Registry) Delete(id string) error {
	id = strings.TrimSpace(id)
	r.mu.Lock()
	defer r.mu.Unlock()

	server, exists := r.servers[id]
	if !exists {
		return fmt.Errorf("%w: %s", ErrServerNotFound, id)
	}
	if server.Builtin {
		return fmt.Errorf("%w: %s", ErrBuiltinDelete, id)
	}
	delete(r.servers, id)
	return nil
}

func (r *Registry) List() []MCPServer {
	r.mu.RLock()
	defer r.mu.RUnlock()

	servers := make([]MCPServer, 0, len(r.servers))
	for _, server := range r.servers {
		servers = append(servers, cloneServer(server))
	}
	sortServers(servers)
	return servers
}

func (r *Registry) Get(id string) (MCPServer, error) {
	id = strings.TrimSpace(id)
	r.mu.RLock()
	defer r.mu.RUnlock()

	server, exists := r.servers[id]
	if !exists {
		return MCPServer{}, fmt.Errorf("%w: %s", ErrServerNotFound, id)
	}
	return cloneServer(server), nil
}

func (r *Registry) Enable(id string, enabled bool) (MCPServer, error) {
	id = strings.TrimSpace(id)
	r.mu.Lock()
	defer r.mu.Unlock()

	server, exists := r.servers[id]
	if !exists {
		return MCPServer{}, fmt.Errorf("%w: %s", ErrServerNotFound, id)
	}
	server.Enabled = enabled
	server.UpdatedAt = r.now().UTC()
	r.servers[id] = cloneServer(server)
	return cloneServer(server), nil
}

func (r *Registry) TestPlan(id string) (ConnectionTestPlan, error) {
	server, err := r.Get(id)
	if err != nil {
		return ConnectionTestPlan{}, err
	}

	plan := ConnectionTestPlan{
		ServerID:   server.ID,
		ServerName: server.Name,
		Transport:  server.Transport.Type,
		Checks:     []string{"validate configuration", "initialize MCP client", "list tools"},
	}
	switch server.Transport.Type {
	case TransportStdio:
		plan.Command = server.Transport.Command
		plan.Args = append([]string(nil), server.Transport.Args...)
		plan.Env = cloneStringMap(server.Transport.Env)
	case TransportHTTP, TransportSSE, TransportStreamableHTTP:
		plan.URL = server.Transport.URL
		plan.Headers = cloneStringMap(server.Transport.Headers)
	}
	return plan, nil
}

func (r *Registry) SetConnectionStatus(id string, status ConnectionStatus) (MCPServer, error) {
	id = strings.TrimSpace(id)
	normalized, err := normalizeConnectionStatus(status)
	if err != nil {
		return MCPServer{}, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	server, exists := r.servers[id]
	if !exists {
		return MCPServer{}, fmt.Errorf("%w: %s", ErrServerNotFound, id)
	}
	server.Connection = normalized
	server.UpdatedAt = r.now().UTC()
	r.servers[id] = cloneServer(server)
	return cloneServer(server), nil
}

func (r *Registry) SetOAuthState(id string, state OAuthState) (MCPServer, error) {
	id = strings.TrimSpace(id)
	normalized, err := normalizeOAuthState(state)
	if err != nil {
		return MCPServer{}, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	server, exists := r.servers[id]
	if !exists {
		return MCPServer{}, fmt.Errorf("%w: %s", ErrServerNotFound, id)
	}
	server.OAuth = normalized
	server.UpdatedAt = r.now().UTC()
	r.servers[id] = cloneServer(server)
	return cloneServer(server), nil
}

func (r *Registry) ImportCandidates(source ImportSource, inputs []ImportCandidate) (ImportPlan, error) {
	if source == "" {
		source = ImportSourceExternal
	}

	r.mu.RLock()
	existingIDs := make(map[string]struct{}, len(r.servers))
	existingNames := make(map[string]struct{}, len(r.servers))
	for _, server := range r.servers {
		existingIDs[strings.ToLower(server.ID)] = struct{}{}
		existingNames[strings.ToLower(server.Name)] = struct{}{}
	}
	r.mu.RUnlock()

	plan := ImportPlan{
		Source:     source,
		Candidates: make([]ImportCandidate, 0, len(inputs)),
	}
	seenIDs := make(map[string]struct{}, len(inputs))
	seenNames := make(map[string]struct{}, len(inputs))

	for _, input := range inputs {
		candidate := ImportCandidate{
			Source: source,
			Server: cloneServer(input.Server),
		}
		normalized, err := NormalizeServer(input.Server)
		if err != nil {
			candidate.Importable = false
			candidate.SkipReason = err.Error()
			plan.SkippedCount++
			plan.Candidates = append(plan.Candidates, candidate)
			continue
		}
		candidate.Server = normalized

		idKey := strings.ToLower(normalized.ID)
		nameKey := strings.ToLower(normalized.Name)
		switch {
		case input.SkipReason != "":
			candidate.Importable = false
			candidate.SkipReason = strings.TrimSpace(input.SkipReason)
		case hasKey(existingIDs, idKey) || hasKey(existingNames, nameKey):
			candidate.Importable = false
			candidate.SkipReason = "already exists"
		case hasKey(seenIDs, idKey) || hasKey(seenNames, nameKey):
			candidate.Importable = false
			candidate.SkipReason = "duplicate candidate"
		default:
			candidate.Importable = true
			seenIDs[idKey] = struct{}{}
			seenNames[nameKey] = struct{}{}
		}

		if candidate.Importable {
			plan.ImportableCount++
		} else {
			plan.SkippedCount++
		}
		plan.Candidates = append(plan.Candidates, candidate)
	}

	return plan, nil
}

func (r *Registry) Snapshot() ServerSnapshot {
	return ServerSnapshot{
		GeneratedAt: r.now().UTC(),
		Servers:     r.List(),
	}
}

func (r *Registry) findByNameLocked(name string, exceptID string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	for id, server := range r.servers {
		if id == exceptID {
			continue
		}
		if strings.ToLower(server.Name) == name {
			return id
		}
	}
	return ""
}

func normalizeConnectionStatus(status ConnectionStatus) (ConnectionStatus, error) {
	if status.State == "" {
		status.State = ConnectionUnknown
	}
	switch status.State {
	case ConnectionUnknown, ConnectionDisabled, ConnectionPending, ConnectionTesting, ConnectionConnected, ConnectionFailed, ConnectionNeedsAuth:
	default:
		return ConnectionStatus{}, fmt.Errorf("%w: connection state %q", ErrInvalidServer, status.State)
	}
	status.Message = strings.TrimSpace(status.Message)
	status.Tools = dedupeTools(status.Tools)
	return status, nil
}

func normalizeOAuthState(state OAuthState) (OAuthState, error) {
	if state.Status == "" {
		state.Status = OAuthUnknown
	}
	switch state.Status {
	case OAuthUnknown, OAuthNotRequired, OAuthRequired, OAuthAuthorizing, OAuthAuthorized, OAuthError:
	default:
		return OAuthState{}, fmt.Errorf("%w: oauth status %q", ErrInvalidServer, state.Status)
	}
	state.AuthURL = strings.TrimSpace(state.AuthURL)
	state.Message = strings.TrimSpace(state.Message)
	return state, nil
}

func cloneServer(server MCPServer) MCPServer {
	server.Transport.Args = append([]string(nil), server.Transport.Args...)
	server.Transport.Env = cloneStringMap(server.Transport.Env)
	server.Transport.Headers = cloneStringMap(server.Transport.Headers)
	server.Tools = cloneTools(server.Tools)
	server.Connection.Tools = cloneTools(server.Connection.Tools)
	return server
}

func cloneTools(tools []ToolInfo) []ToolInfo {
	if len(tools) == 0 {
		return nil
	}
	cloned := make([]ToolInfo, len(tools))
	for i, tool := range tools {
		cloned[i] = tool
		if len(tool.Meta) > 0 {
			cloned[i].Meta = make(map[string]any, len(tool.Meta))
			for key, value := range tool.Meta {
				cloned[i].Meta[key] = value
			}
		}
	}
	return cloned
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func hasKey(values map[string]struct{}, key string) bool {
	_, exists := values[key]
	return exists
}

func sortServers(servers []MCPServer) {
	sort.Slice(servers, func(i, j int) bool {
		if servers[i].Name == servers[j].Name {
			return servers[i].ID < servers[j].ID
		}
		return servers[i].Name < servers[j].Name
	})
}
