package connectors

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"awaken/src/mcpclient"
	"awaken/src/tool"
)

// ErrConnectorNotFound is returned when a connector ID is not registered.
var ErrConnectorNotFound = errors.New("connector not found")

// ErrConnectorAlreadyConnected is returned when Connect is called on an
// already-connected connector.
var ErrConnectorAlreadyConnected = errors.New("connector is already connected")

// ErrConnectorNotConnected is returned when Disconnect is called on a
// connector that is not connected.
var ErrConnectorNotConnected = errors.New("connector is not connected")

// ErrConnectorActionUnsupported is returned when an MCP lifecycle action is
// attempted for a Desktop-owned CLI connector.
var ErrConnectorActionUnsupported = errors.New("connector action is unsupported for this connector kind")

var ErrCLIReadinessUnauthorized = errors.New("cli readiness update is unauthorized")
var ErrCLIReadinessInvalid = errors.New("cli readiness snapshot is invalid")

// RuntimeManager abstracts the MCP manager operations that the connector
// registry needs: InjectDynamic, RemoveDynamic, Start, Remove, Snapshot, and
// SnapshotServer.
type RuntimeManager interface {
	InjectDynamic(connectorID string, config mcpclient.ScopedMcpServerConfig)
	RemoveDynamic(connectorID, serverName string)
	Start(ctx context.Context, configs []mcpclient.ScopedMcpServerConfig)
	Remove(name string) error
	Snapshot() []mcpclient.ServerSnapshot
	SnapshotServer(name string) (mcpclient.ServerSnapshot, error)
	ClearCredentialsFor(config mcpclient.ScopedMcpServerConfig) error
}

// Registry is the central connector lifecycle manager. It holds connector
// definitions from all sources, tracks which connectors are active, and
// delegates MCP server injection/removal to the runtime manager.
type Registry struct {
	logger  Logger
	manager RuntimeManager

	mu               sync.RWMutex
	active           map[string]*activeEntry        // connectorID -> runtime state
	defs             map[string]ConnectorDefinition // resolved+merged definitions
	cliReadiness     map[string]CLIConnectorReadiness
	legacyCLIReview  map[string]bool
	cliControlToken  string
	accountReadiness AccountReadinessProvider
}

func (r *Registry) SetAccountReadinessProvider(provider AccountReadinessProvider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.accountReadiness = provider
}

// activeEntry tracks runtime state for a connector that has been connected.
type activeEntry struct {
	connected  bool
	serverKeys []string // MCP server names injected for this connector
	scope      mcpclient.Scope
}

// NewRegistry creates a connector registry backed by the given runtime manager.
func NewRegistry(manager RuntimeManager, logger Logger) *Registry {
	if logger == nil {
		logger = noopLogger{}
	}
	return &Registry{
		logger:          logger,
		manager:         manager,
		active:          make(map[string]*activeEntry),
		defs:            make(map[string]ConnectorDefinition),
		cliReadiness:    make(map[string]CLIConnectorReadiness),
		legacyCLIReview: make(map[string]bool),
	}
}

// SetLegacyCLIReviewRequired records ambiguous user MCP entries without
// changing or deleting them. The marker is catalog-only and survives later
// Desktop readiness snapshots for the current Core process.
func (r *Registry) SetLegacyCLIReviewRequired(serverKeys []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, key := range serverKeys {
		switch key {
		case "dingtalk-workspace":
			r.legacyCLIReview["dingtalk-workspace"] = true
		case "lark":
			r.legacyCLIReview["lark-cli"] = true
		}
	}
}

// SetCLIControlToken configures the startup-scoped secret required for
// Desktop-to-Core CLI readiness updates.
func (r *Registry) SetCLIControlToken(token string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cliControlToken = strings.TrimSpace(token)
}

// UpdateCLIReadiness accepts only complete, non-sensitive snapshots for known
// CLI providers. The control token is compared in constant time.
func (r *Registry) UpdateCLIReadiness(token, connectorID string, snapshot CLIConnectorReadiness) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cliControlToken == "" || len(token) != len(r.cliControlToken) || subtle.ConstantTimeCompare([]byte(token), []byte(r.cliControlToken)) != 1 {
		return ErrCLIReadinessUnauthorized
	}
	def, ok := r.defs[connectorID]
	if !ok {
		return ErrConnectorNotFound
	}
	if def.Kind != KindCLI || snapshot.ProviderID != connectorID || !validCLIReadiness(snapshot) {
		return ErrCLIReadinessInvalid
	}
	r.cliReadiness[connectorID] = snapshot
	return nil
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
			d = normalizeConnectorDefinition(d)

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

// RestoreBuiltinOwnership rebuilds builtin connector ownership from the
// canonical user MCP config. Provider identity is the durable owner; server
// names alone are never enough because a manual server may use the same name.
func (r *Registry) RestoreBuiltinOwnership(userCfg mcpclient.ConfigFile) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for id, def := range r.defs {
		if def.SourceOrigin != SourceBuiltin || def.Kind != KindMCP {
			continue
		}
		providerID := def.ProviderID
		if providerID == "" {
			providerID = tool.ProviderID("connector:" + def.ID)
		}
		var serverKeys []string
		for name, cfg := range userCfg.Servers {
			if cfg.Provider == nil || cfg.Provider.ProviderID != providerID || cfg.Provider.Source != tool.SourceConnector {
				continue
			}
			serverKeys = append(serverKeys, name)
		}
		if len(serverKeys) == 0 {
			continue
		}
		sort.Strings(serverKeys)
		r.active[id] = &activeEntry{connected: true, serverKeys: serverKeys, scope: mcpclient.ScopeUser}
	}
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
	if def.Kind != KindMCP {
		return ErrConnectorActionUnsupported
	}

	if entry := r.active[connectorID]; entry != nil && entry.connected {
		// If the connector is marked connected but its servers are no longer in the
		// runtime (e.g. the user deleted the MCP server from the settings dialog),
		// or every persisted builtin server is disabled, reset the entry and allow
		// the card's Connect action to restore an enabled runtime.
		stale := false
		allDisabled := false
		recoverable := false
		if def.SourceOrigin == SourceBuiltin {
			stale = true
			allDisabled = true
			for _, key := range entry.serverKeys {
				if snapshot, err := r.manager.SnapshotServer(key); err == nil {
					stale = false
					if snapshot.State != mcpclient.StateDisabled {
						allDisabled = false
					}
					if snapshot.State == mcpclient.StateNeedsAuth || snapshot.State == mcpclient.StateFailed {
						recoverable = true
					}
				}
			}
		}
		if !stale && !allDisabled && !recoverable {
			return ErrConnectorAlreadyConnected
		}
		entry.connected = false
		entry.serverKeys = nil
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
		var err error
		scopedConfigs, err = def.McpConfigFactory(def, nil)
		if err != nil {
			return fmt.Errorf("prepare connector %q config: %w", connectorID, err)
		}
	} else {
		return fmt.Errorf("connector %q has neither a config factory nor a raw config", connectorID)
	}
	for key, scoped := range scopedConfigs {
		providerID := def.ProviderID
		if providerID == "" {
			providerID = tool.ProviderID("connector:" + def.ID)
		}
		scoped.Config.Provider = &mcpclient.ServerProviderManifest{
			ProviderID:   providerID,
			DisplayName:  def.DisplayName,
			Source:       tool.SourceConnector,
			Dependencies: append([]tool.DependencyRequirement(nil), def.Dependencies...),
		}
		scopedConfigs[key] = scoped
	}

	var serverKeys []string
	activeScope := mcpclient.ScopeDynamic
	if def.SourceOrigin == SourceBuiltin {
		// Builtin connectors are shortcuts for adding user-level MCP servers.
		// Persist the generated config and start it normally so it behaves like
		// any other server added through the MCP settings dialog.
		activeScope = mcpclient.ScopeUser
		userCfg, err := mcpclient.LoadUserConfig()
		if err != nil {
			return fmt.Errorf("load user mcp config: %w", err)
		}
		for key, scoped := range scopedConfigs {
			previous, ok := userCfg.Servers[key]
			if !ok || mcpclient.CredentialIdentity(mcpclient.ScopeUser, key, previous) == mcpclient.CredentialIdentity(mcpclient.ScopeUser, key, scoped.Config) {
				continue
			}
			if err := r.manager.ClearCredentialsFor(mcpclient.ScopedMcpServerConfig{Name: key, Scope: mcpclient.ScopeUser, Config: previous}); err != nil {
				return fmt.Errorf("clear credentials for %q: %w", key, err)
			}
		}
		if err := mcpclient.MutateUserConfig(func(userCfg *mcpclient.ConfigFile) (bool, error) {
			for key, cfg := range scopedConfigs {
				userCfg.Servers[key] = cfg.Config
				serverKeys = append(serverKeys, key)
			}
			return true, nil
		}); err != nil {
			return fmt.Errorf("save user mcp config: %w", err)
		}
		sort.Strings(serverKeys)
		for _, key := range serverKeys {
			_ = r.manager.Remove(key)
		}
		// Detach from the request context so the async connect loop keeps
		// running after the HTTP response is sent.
		r.manager.Start(context.WithoutCancel(ctx), scopedConfigsToSlice(scopedConfigs))
	} else {
		// Local connectors remain dynamic/injected.
		for key, cfg := range scopedConfigs {
			r.manager.InjectDynamic(connectorID, cfg)
			serverKeys = append(serverKeys, key)
		}
	}

	r.active[connectorID] = &activeEntry{
		connected:  true,
		serverKeys: serverKeys,
		scope:      activeScope,
	}

	r.logger.Debugf("connector %q connected with %d server(s): %v", connectorID, len(serverKeys), serverKeys)
	return nil
}

func scopedConfigsToSlice(configs map[string]mcpclient.ScopedMcpServerConfig) []mcpclient.ScopedMcpServerConfig {
	out := make([]mcpclient.ScopedMcpServerConfig, 0, len(configs))
	for _, cfg := range configs {
		out = append(out, cfg)
	}
	return out
}

// Disconnect removes a connector's MCP servers from the runtime and marks
// the connector as disconnected.
func (r *Registry) Disconnect(ctx context.Context, connectorID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	def, ok := r.defs[connectorID]
	if !ok {
		return ErrConnectorNotFound
	}
	if def.Kind != KindMCP {
		return ErrConnectorActionUnsupported
	}

	entry, ok := r.active[connectorID]
	if !ok || !entry.connected {
		return ErrConnectorNotConnected
	}

	if def.SourceOrigin == SourceBuiltin {
		// Builtin connectors are backed by user config; remove from there so the
		// server disappears from the MCP Servers list permanently.
		userCfg, err := mcpclient.LoadUserConfig()
		if err != nil {
			return fmt.Errorf("load user mcp config: %w", err)
		}
		for _, key := range entry.serverKeys {
			if cfg, ok := userCfg.Servers[key]; ok {
				if err := r.manager.ClearCredentialsFor(mcpclient.ScopedMcpServerConfig{Name: key, Scope: mcpclient.ScopeUser, Config: cfg}); err != nil {
					return fmt.Errorf("clear credentials for %q: %w", key, err)
				}
			}
		}
		if err := mcpclient.MutateUserConfig(func(userCfg *mcpclient.ConfigFile) (bool, error) {
			for _, key := range entry.serverKeys {
				delete(userCfg.Servers, key)
			}
			return true, nil
		}); err != nil {
			return fmt.Errorf("save user mcp config: %w", err)
		}
		for _, key := range entry.serverKeys {
			_ = r.manager.Remove(key)
		}
	} else {
		// Remove each injected server from the runtime.
		for _, key := range entry.serverKeys {
			r.manager.RemoveDynamic(connectorID, key)
		}
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
			ID:                def.ID,
			Kind:              def.Kind,
			DisplayName:       def.DisplayName,
			Description:       def.Description,
			Category:          def.Category,
			Provider:          def.Provider,
			ProviderID:        def.ProviderID,
			Capabilities:      append([]tool.Capability(nil), def.Capabilities...),
			Dependencies:      append([]tool.DependencyRequirement(nil), def.Dependencies...),
			SourceOrigin:      def.SourceOrigin,
			AuthType:          def.AuthType,
			State:             StateDisconnected,
			ProviderReadiness: tool.ProviderUnavailable,
		}

		if def.Kind == KindCLI {
			status.LegacyMCPReviewRequired = r.legacyCLIReview[id]
			if readiness, ok := r.cliReadiness[id]; ok {
				copy := readiness
				status.CLIReadiness = &copy
				status.State, status.ProviderReadiness = cliConnectorProjection(readiness)
				if readiness.SafeError != nil {
					status.LastError = readiness.SafeError.Message
				}
			}
			out = append(out, status)
			continue
		}

		if def.OAuth != nil && r.accountReadiness != nil {
			account := r.accountReadiness.AccountReadiness(def)
			status.Account = account.Account
			if account.State != StateConnected {
				status.State = account.State
				status.ProviderReadiness = connectorProviderReadiness(account.State)
				status.LastError = account.LastError
				out = append(out, status)
				continue
			}
		}

		entry := r.active[id]
		if entry != nil && entry.connected {
			status.ServerNames = entry.serverKeys
			// Derive state from the MCP server snapshots.
			status.State = deriveConnectorState(entry.serverKeys, entry.scope, snapByServer)
			status.ProviderReadiness = connectorProviderReadiness(status.State)
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

func normalizeConnectorDefinition(def ConnectorDefinition) ConnectorDefinition {
	if def.Kind == "" {
		def.Kind = KindMCP
	}
	if def.ProviderID == "" {
		def.ProviderID = tool.ProviderID("connector:" + def.ID)
	}
	def.Capabilities = tool.NormalizeCapabilities(def.Capabilities)
	def.Dependencies = normalizeConnectorDependencies(def.Dependencies)
	return def
}

func normalizeConnectorDependencies(values []tool.DependencyRequirement) []tool.DependencyRequirement {
	seen := make(map[string]struct{}, len(values))
	result := make([]tool.DependencyRequirement, 0, len(values))
	for _, value := range values {
		value.Name = strings.TrimSpace(value.Name)
		if value.Name == "" {
			continue
		}
		key := string(value.Kind) + "\x00" + value.Name + "\x00" + value.VersionConstraint
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	return result
}

func connectorProviderReadiness(state ConnectorState) tool.ProviderReadiness {
	switch state {
	case StateConnected:
		return tool.ProviderReady
	case StateNeedsAuth:
		return tool.ProviderNeedsAuth
	case StatePending, StateFailed, StateDisconnected:
		return tool.ProviderUnavailable
	default:
		return tool.ProviderUnavailable
	}
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
		ID:                def.ID,
		Kind:              def.Kind,
		DisplayName:       def.DisplayName,
		Description:       def.Description,
		Category:          def.Category,
		Provider:          def.Provider,
		ProviderID:        def.ProviderID,
		Capabilities:      append([]tool.Capability(nil), def.Capabilities...),
		Dependencies:      append([]tool.DependencyRequirement(nil), def.Dependencies...),
		ProviderReadiness: tool.ProviderUnavailable,
		SourceOrigin:      def.SourceOrigin,
		AuthType:          def.AuthType,
		State:             StateDisconnected,
	}
	if def.Kind == KindCLI {
		if readiness, ok := r.cliReadiness[connectorID]; ok {
			copy := readiness
			status.CLIReadiness = &copy
			status.State, status.ProviderReadiness = cliConnectorProjection(readiness)
			if readiness.SafeError != nil {
				status.LastError = readiness.SafeError.Message
			}
		}
		return status, nil
	}
	if def.OAuth != nil && r.accountReadiness != nil {
		account := r.accountReadiness.AccountReadiness(def)
		status.Account = account.Account
		if account.State != StateConnected {
			status.State = account.State
			status.ProviderReadiness = connectorProviderReadiness(account.State)
			status.LastError = account.LastError
			return status, nil
		}
	}

	entry := r.active[connectorID]
	if entry != nil && entry.connected {
		status.ServerNames = entry.serverKeys
		status.State = deriveConnectorState(entry.serverKeys, entry.scope, snapByServer)
		status.ProviderReadiness = connectorProviderReadiness(status.State)
	}

	return status, nil
}

func validCLIReadiness(value CLIConnectorReadiness) bool {
	if strings.TrimSpace(value.UpdatedAt) == "" {
		return false
	}
	validActivation := value.Activation == CLIActivationEnabled || value.Activation == CLIActivationDisabled
	validInstall := value.Install == CLIInstallMissing || value.Install == CLIInstallInstalling || value.Install == CLIInstallInstalled || value.Install == CLIInstallUnsupported || value.Install == CLIInstallFailed
	validExecutable := value.Executable == CLIExecutableUnknown || value.Executable == CLIExecutableMissing || value.Executable == CLIExecutableAvailable || value.Executable == CLIExecutableInvalid
	validConfiguration := value.Configuration == CLIConfigurationUnknown || value.Configuration == CLIConfigurationRequired || value.Configuration == CLIConfigurationReady || value.Configuration == CLIConfigurationFailed
	validAuthentication := value.Authentication == CLIAuthenticationUnknown || value.Authentication == CLIAuthenticationRequired || value.Authentication == CLIAuthenticationInProgress || value.Authentication == CLIAuthenticationAuthorized || value.Authentication == CLIAuthenticationFailed
	validOverall := value.Overall == CLIOverallDisabled || value.Overall == CLIOverallNotReady || value.Overall == CLIOverallActionRequired || value.Overall == CLIOverallWorking || value.Overall == CLIOverallReady || value.Overall == CLIOverallFailed
	return validActivation && validInstall && validExecutable && validConfiguration && validAuthentication && validOverall
}

func cliConnectorProjection(value CLIConnectorReadiness) (ConnectorState, tool.ProviderReadiness) {
	switch value.Overall {
	case CLIOverallReady:
		return StateConnected, tool.ProviderReady
	case CLIOverallWorking:
		return StatePending, tool.ProviderUnavailable
	case CLIOverallActionRequired:
		return StateNeedsAuth, tool.ProviderNeedsAuth
	case CLIOverallFailed:
		return StateFailed, tool.ProviderUnavailable
	default:
		return StateDisconnected, tool.ProviderUnavailable
	}
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
func deriveConnectorState(serverKeys []string, expectedScope mcpclient.Scope, snapByServer map[string]mcpclient.ServerSnapshot) ConnectorState {
	if len(serverKeys) == 0 {
		return StateDisconnected
	}

	hasConnected := false
	hasFailed := false
	hasNeedsAuth := false
	hasPending := false

	for _, key := range serverKeys {
		snap, ok := snapByServer[key]
		if !ok || (expectedScope != "" && snap.Scope != expectedScope) {
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
