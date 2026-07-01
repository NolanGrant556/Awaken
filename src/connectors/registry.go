package connectors

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"

	"awaken/src/mcpclient"
)

// ErrConnectorNotFound is returned when a connector ID is not registered.
var ErrConnectorNotFound = errors.New("connector not found")

// ErrConnectorAlreadyConnected is returned when Connect is called on an
// already-connected connector.
var ErrConnectorAlreadyConnected = errors.New("connector is already connected")

// ErrConnectorNotConnected is returned when Disconnect is called on a
// connector that is not connected.
var ErrConnectorNotConnected = errors.New("connector is not connected")

// RuntimeManager abstracts the MCP manager operations that the connector
// registry needs: InjectDynamic, RemoveDynamic, and Snapshot.
type RuntimeManager interface {
	InjectDynamic(connectorID string, config mcpclient.ScopedMcpServerConfig)
	RemoveDynamic(connectorID, serverName string)
	Snapshot() []mcpclient.ServerSnapshot
}

// Registry is the central connector lifecycle manager. It holds connector
// definitions from all sources, tracks which connectors are active, and
// delegates MCP server injection/removal to the runtime manager.
type Registry struct {
	logger  Logger
	manager RuntimeManager

	mu     sync.RWMutex
	active map[string]*activeEntry            // connectorID -> runtime state
	defs   map[string]ConnectorDefinition     // resolved+merged definitions
}

// activeEntry tracks runtime state for a connector that has been connected.
type activeEntry struct {
	connected  bool
	serverKeys []string // MCP server names injected for this connector
}

// NewRegistry creates a connector registry backed by the given runtime manager.
func NewRegistry(manager RuntimeManager, logger Logger) *Registry {
	if logger == nil {
		logger = noopLogger{}
	}
	return &Registry{
		logger:  logger,
		manager: manager,
		active:  make(map[string]*activeEntry),
		defs:   make(map[string]ConnectorDefinition),
	}
}

// LoadSources loads connector definitions from all supplied sources and merges
// them by priority (lower priority number = higher precedence). Sources should
// be sorted by priority before calling.
func (r *Registry) LoadSources(ctx context.Context, sources []ConnectorSource) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Load all definitions from all sources, deduplicating by ID.
	type sourcedDef struct {
		def    ConnectorDefinition
		source SourceOrigin
		prio   int
	}
	all := make(map[string]sourcedDef)

	for _, src := range sources {
		defs, err := src.List(ctx)
		if err != nil {
			r.logger.Warnf("connector source %s failed to list: %v", src.Origin(), err)
			continue
		}
		for _, d := range defs {
			// Ensure SourceOrigin is set correctly.
			d.SourceOrigin = src.Origin()

			existing, ok := all[d.ID]
			if ok {
				// Lower priority number = higher precedence.
				if src.Priority() < existing.prio {
					r.logger.Warnf("connector %q from %s (priority %d) shadows existing from %s (priority %d)",
						d.ID, src.Origin(), src.Priority(), existing.source, existing.prio)
					all[d.ID] = sourcedDef{def: d, source: src.Origin(), prio: src.Priority()}
				} else if src.Priority() == existing.prio && existing.source == SourceLocal {
					// Local allows overwrite with same priority (user edited the file).
					r.logger.Debugf("local connector %q updated from same source", d.ID)
					all[d.ID] = sourcedDef{def: d, source: src.Origin(), prio: src.Priority()}
				} else {
					r.logger.Warnf("connector %q from %s (priority %d) is shadowed by %s (priority %d), skipping",
						d.ID, src.Origin(), src.Priority(), existing.source, existing.prio)
				}
			} else {
				all[d.ID] = sourcedDef{def: d, source: src.Origin(), prio: src.Priority()}
			}
		}
	}

	// Flatten into the registry's definition map.
	r.defs = make(map[string]ConnectorDefinition, len(all))
	for id, sd := range all {
		d := sd.def
		d.SourceOrigin = sd.source
		r.defs[id] = d
	}
	return nil
}

// Connect initiates a connector. It generates MCP server configs from the
// connector definition (via factory or raw config), injects them into the
// MCP runtime, and marks the connector as connected.
//
// For OAuth2 connectors with no existing credential, the caller should first
// complete the OAuth flow. Connect is idempotent: calling it on an already-
// connected connector returns ErrConnectorAlreadyConnected.
func (r *Registry) Connect(ctx context.Context, connectorID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	def, ok := r.defs[connectorID]
	if !ok {
		return ErrConnectorNotFound
	}

	if entry := r.active[connectorID]; entry != nil && entry.connected {
		return ErrConnectorAlreadyConnected
	}

	// Generate MCP server configs.
	var scopedConfigs map[string]mcpclient.ScopedMcpServerConfig
	if def.MCPRawConfig != nil {
		// Local connector: use inline config directly.
		raw := *def.MCPRawConfig
		scopedConfigs = map[string]mcpclient.ScopedMcpServerConfig{
			connectorID: {
				Name:   connectorID,
				Scope:  mcpclient.ScopeDynamic,
				Config: raw,
			},
		}
	} else if def.McpConfigFactory != nil {
		// Builtin connector: use config factory.
		scopedConfigs = def.McpConfigFactory(def, nil)
	} else {
		return fmt.Errorf("connector %q has neither a config factory nor a raw config", connectorID)
	}

	// Inject each generated config into the runtime.
	var serverKeys []string
	for key, cfg := range scopedConfigs {
		r.manager.InjectDynamic(connectorID, cfg)
		serverKeys = append(serverKeys, key)
	}

	r.active[connectorID] = &activeEntry{
		connected:  true,
		serverKeys: serverKeys,
	}

	r.logger.Debugf("connector %q connected with %d server(s): %v", connectorID, len(serverKeys), serverKeys)
	return nil
}

// Disconnect removes a connector's MCP servers from the runtime and marks
// the connector as disconnected.
func (r *Registry) Disconnect(_ context.Context, connectorID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	entry, ok := r.active[connectorID]
	if !ok || !entry.connected {
		return ErrConnectorNotConnected
	}

	// Remove each injected server from the runtime.
	for _, key := range entry.serverKeys {
		r.manager.RemoveDynamic(connectorID, key)
	}

	entry.connected = false
	entry.serverKeys = nil
	r.logger.Debugf("connector %q disconnected", connectorID)
	return nil
}

// List returns the status of every known connector, derived from the merged
// definitions and the MCP runtime's current server snapshots.
func (r *Registry) List() []ConnectorStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()

	snapshots := r.manager.Snapshot()
	snapByServer := make(map[string]mcpclient.ServerSnapshot, len(snapshots))
	for _, s := range snapshots {
		snapByServer[s.Name] = s
	}

	// Build a map of connectorID -> list of the MCP server names it owns.
	ownedServers := make(map[string]map[string]bool)
	for id, entry := range r.active {
		if !entry.connected {
			continue
		}
		owned := make(map[string]bool)
		for _, sk := range entry.serverKeys {
			owned[sk] = true
		}
		ownedServers[id] = owned
	}

	var out []ConnectorStatus
	for id, def := range r.defs {
		status := ConnectorStatus{
			ID:          def.ID,
			DisplayName: def.DisplayName,
			Category:    def.Category,
			Provider:    def.Provider,
			SourceOrigin: def.SourceOrigin,
			AuthType:     def.AuthType,
			State:        StateDisconnected,
		}

		entry := r.active[id]
		if entry != nil && entry.connected {
			status.ServerNames = entry.serverKeys
			// Derive state from the MCP server snapshots.
			status.State = deriveConnectorState(entry.serverKeys, snapByServer)
		}

		// Check for shadowing by inspecting ServerSnapshot.SourceConnectorID.
		// If a server with the connector's ID exists in the snapshot but its
		// SourceConnectorID does not match this connector, it is shadowed.
		for _, s := range snapshots {
			if s.Name == id && s.SourceConnectorID != nil && *s.SourceConnectorID != "" && *s.SourceConnectorID != id {
				status.ShadowedBy = *s.SourceConnectorID
				break
			}
		}

		out = append(out, status)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out
}

// Status returns the derived status for a single connector.
func (r *Registry) Status(connectorID string) (ConnectorStatus, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	def, ok := r.defs[connectorID]
	if !ok {
		return ConnectorStatus{}, ErrConnectorNotFound
	}

	snapshots := r.manager.Snapshot()
	snapByServer := make(map[string]mcpclient.ServerSnapshot, len(snapshots))
	for _, s := range snapshots {
		snapByServer[s.Name] = s
	}

	status := ConnectorStatus{
		ID:          def.ID,
		DisplayName: def.DisplayName,
		Category:    def.Category,
		Provider:    def.Provider,
		SourceOrigin: def.SourceOrigin,
		AuthType:     def.AuthType,
		State:        StateDisconnected,
	}

	entry := r.active[connectorID]
	if entry != nil && entry.connected {
		status.ServerNames = entry.serverKeys
		status.State = deriveConnectorState(entry.serverKeys, snapByServer)
	}

	return status, nil
}

// Definition returns the resolved ConnectorDefinition for the given ID.
func (r *Registry) Definition(connectorID string) (ConnectorDefinition, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	def, ok := r.defs[connectorID]
	if !ok {
		return ConnectorDefinition{}, ErrConnectorNotFound
	}
	return def, nil
}

// SetCredentialStore delegates to the MCP manager. This is a passthrough for
// the case where the caller has access to the manager directly.
func (r *Registry) SetManager(manager RuntimeManager) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.manager = manager
}

// deriveConnectorState computes a connector's state from its MCP server states.
// If any server is connected -> connected. If any needs auth -> needs-auth.
// If any failed -> failed. If all pending -> pending. Otherwise disconnected.
func deriveConnectorState(serverKeys []string, snapByServer map[string]mcpclient.ServerSnapshot) ConnectorState {
	if len(serverKeys) == 0 {
		return StateDisconnected
	}

	hasConnected := false
	hasFailed := false
	hasNeedsAuth := false
	hasPending := false

	for _, key := range serverKeys {
		snap, ok := snapByServer[key]
		if !ok {
			// Server not yet started → treat as disconnected.
			continue
		}
		switch snap.State {
		case mcpclient.StateConnected:
			hasConnected = true
		case mcpclient.StateFailed:
			hasFailed = true
		case mcpclient.StateNeedsAuth:
			hasNeedsAuth = true
		case mcpclient.StatePending:
			hasPending = true
		case mcpclient.StateDisabled:
			// Disabled servers are treated as disconnected.
		}
	}

	if hasConnected {
		return StateConnected
	}
	if hasNeedsAuth {
		return StateNeedsAuth
	}
	if hasFailed {
		return StateFailed
	}
	if hasPending {
		return StatePending
	}
	return StateDisconnected
}