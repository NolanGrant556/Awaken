package skills

import (
	"sort"
	"strings"
	"sync"
)

// SkillRegistry manages the lifecycle of all discovered skills. It merges
// skills from user and project sources, tracks availability
// based on connector presence and MCP server state, and exposes a stable
// snapshot for consumers.
type SkillRegistry struct {
	mu         sync.RWMutex
	skills     map[string]*SkillDefinition
	logger     Logger
	onChange   func()
	connectors map[string]bool // available connector names
	mcpServers map[string]bool // available MCP server names
}

// Logger is the minimal logging surface consumed by the skills package.
type Logger interface {
	Debugf(format string, args ...any)
	Warnf(format string, args ...any)
}

type noopLogger struct{}

func (noopLogger) Debugf(string, ...any) {}
func (noopLogger) Warnf(string, ...any)  {}

// NewSkillRegistry creates a registry with the given logger. If logger is
// nil, warnings are silently discarded.
func NewSkillRegistry(logger Logger) *SkillRegistry {
	if logger == nil {
		logger = noopLogger{}
	}
	return &SkillRegistry{
		skills:     make(map[string]*SkillDefinition),
		logger:     logger,
		connectors: make(map[string]bool),
		mcpServers: make(map[string]bool),
	}
}

// SetOnChange registers a callback invoked whenever the skill set changes
// (load, connector status change, etc.). Only one callback is supported.
func (r *SkillRegistry) SetOnChange(fn func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onChange = fn
}

// LoadFromSources clears the registry and loads skills from the given source
// directories. dirs maps SourceOrigin to the absolute directory path.
func (r *SkillRegistry) LoadFromSources(dirs map[SourceOrigin]string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.replaceLocalSourcesLocked(dirs)
	r.fireOnChangeLocked()
}

// ReloadLocalSources reloads user and project skills from disk while keeping
// dynamic MCP-sourced skills that are managed by the MCP bridge.
func (r *SkillRegistry) ReloadLocalSources(dirs map[SourceOrigin]string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.replaceLocalSourcesLocked(dirs)
	r.fireOnChangeLocked()
}

// PreviewLocalSources returns the skill set that would be produced by loading
// the given local source directories without mutating the live registry or
// registered skill tools. MCP-sourced skills from the live registry are included.
func (r *SkillRegistry) PreviewLocalSources(dirs map[SourceOrigin]string) []*SkillDefinition {
	r.mu.Lock()
	defer r.mu.Unlock()

	defs := r.buildLocalSourcesLocked(dirs)
	r.reevaluateAvailabilityForLocked(defs)
	return sortedSkillDefinitions(defs)
}

func (r *SkillRegistry) replaceLocalSourcesLocked(dirs map[SourceOrigin]string) {
	r.skills = r.buildLocalSourcesLocked(dirs)
	r.reevaluateAvailabilityLocked()
}

func (r *SkillRegistry) buildLocalSourcesLocked(dirs map[SourceOrigin]string) map[string]*SkillDefinition {
	var sourceMaps []map[string]*SkillDefinition

	// Process sources from lowest to highest priority so merge works.
	for _, src := range []SourceOrigin{SourceUser, SourceProject} {
		dir, ok := dirs[src]
		if !ok || dir == "" {
			continue
		}
		loaded := LoadDir(dir, src)
		if len(loaded) > 0 {
			sourceMaps = append(sourceMaps, loaded)
		}
	}

	next := MergeSkills(sourceMaps...)
	for id, def := range r.skills {
		if def != nil && def.Source == SourceMcp {
			next[id] = shallowCopy(def)
		}
	}
	return next
}

// SetConnectorAvailable updates the availability status of a single
// connector and re-evaluates all skills that depend on it.
func (r *SkillRegistry) SetConnectorAvailable(name string, available bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if available {
		r.connectors[name] = true
	} else {
		delete(r.connectors, name)
	}
	r.reevaluateAvailabilityLocked()
	r.fireOnChangeLocked()
}

// SetMcpServerAvailable updates the availability status of a single MCP
// server and re-evaluates all skills that depend on it. Called by the MCP
// bridge when a server transitions between connected and other states.
func (r *SkillRegistry) SetMcpServerAvailable(name string, available bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if available {
		r.mcpServers[name] = true
	} else {
		delete(r.mcpServers, name)
	}
	r.reevaluateAvailabilityLocked()
	r.fireOnChangeLocked()
}

// reevaluateAvailabilityLocked recalculates State for every skill based on
// current connector and MCP server availability. Must be called with mu held.
func (r *SkillRegistry) reevaluateAvailabilityLocked() {
	r.reevaluateAvailabilityForLocked(r.skills)
}

func (r *SkillRegistry) reevaluateAvailabilityForLocked(defs map[string]*SkillDefinition) {
	for id, def := range defs {
		// Don't touch shadowed entries.
		if def.State == StateShadowed {
			continue
		}
		if strings.HasSuffix(id, "__shadowed") {
			continue
		}
		// Platform-contract conflicts are sticky until the package changes.
		if def.UnavailableCode == ErrorCodePlatformContractConflict {
			def.State = StateUnavailable
			if strings.TrimSpace(def.UnavailableReason) == "" {
				def.UnavailableReason = ErrorCodePlatformContractConflict
			}
			continue
		}
		if conflicts := DetectPlatformContractConflicts(def); len(conflicts) > 0 {
			def.State = StateUnavailable
			def.UnavailableCode = ErrorCodePlatformContractConflict
			def.UnavailableReason = FormatPlatformConflictReason(conflicts)
			continue
		}
		if len(def.RequiredConnectors) == 0 && len(def.RequiredMcpServers) == 0 {
			def.State = StateAvailable
			def.UnavailableReason = ""
			def.UnavailableCode = ""
			continue
		}
		var reasons []string
		missingConn := r.missingConnectorsLocked(def.RequiredConnectors)
		if len(missingConn) > 0 {
			reasons = append(reasons, "missing connectors: "+stringsJoin(missingConn, ", "))
		}
		missingMcp := r.missingMcpServersLocked(def.RequiredMcpServers)
		if len(missingMcp) > 0 {
			reasons = append(reasons, "missing mcp servers: "+stringsJoin(missingMcp, ", "))
		}
		if len(reasons) > 0 {
			def.State = StateUnavailable
			def.UnavailableReason = stringsJoin(reasons, "; ")
			def.UnavailableCode = ""
		} else {
			def.State = StateAvailable
			def.UnavailableReason = ""
			def.UnavailableCode = ""
		}
	}
}

func (r *SkillRegistry) missingConnectorsLocked(required []string) []string {
	var missing []string
	for _, name := range required {
		if !r.connectors[name] {
			missing = append(missing, name)
		}
	}
	return missing
}

func (r *SkillRegistry) missingMcpServersLocked(required []string) []string {
	var missing []string
	for _, name := range required {
		if !r.mcpServers[name] {
			missing = append(missing, name)
		}
	}
	return missing
}

// ListAvailable returns all skills with StateAvailable.
func (r *SkillRegistry) ListAvailable() []*SkillDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*SkillDefinition, 0)
	for id, def := range r.skills {
		if strings.HasSuffix(id, "__shadowed") {
			continue
		}
		if def.State == StateAvailable {
			result = append(result, def)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})
	return result
}

// ListAll returns all skills regardless of state. Shadowed entries are
// included with their ShadowedBy field set.
func (r *SkillRegistry) ListAll() []*SkillDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return sortedSkillDefinitions(r.skills)
}

func sortedSkillDefinitions(defs map[string]*SkillDefinition) []*SkillDefinition {
	result := make([]*SkillDefinition, 0, len(defs))
	for _, def := range defs {
		result = append(result, def)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})
	return result
}

// Get returns a skill by ID, or nil if not found.
func (r *SkillRegistry) Get(id string) *SkillDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.skills[id]
}

// RegisterSkill adds or replaces a single skill in the registry. Used by
// the MCP bridge to idempotently reconcile bridged skills.
func (r *SkillRegistry) RegisterSkill(def *SkillDefinition) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if def == nil || def.ID == "" {
		return
	}

	// Clear shadowed marker for this ID before inserting.
	delete(r.skills, def.ID)
	delete(r.skills, def.ID+"__shadowed")

	r.skills[def.ID] = shallowCopy(def)
	r.reevaluateAvailabilityLocked()
	r.fireOnChangeLocked()
}

// UnregisterSkill removes a single skill by ID. Used by the MCP bridge
// when a server disconnects or is disabled.
func (r *SkillRegistry) UnregisterSkill(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.skills, id)
	delete(r.skills, id+"__shadowed")
	r.fireOnChangeLocked()
}

// UnregisterSkillsByPrefix removes all skills whose ID starts with the
// given prefix. Used by the MCP bridge to clean up mcp__{server}__* skills
// in a single call.
func (r *SkillRegistry) UnregisterSkillsByPrefix(prefix string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for id := range r.skills {
		if strings.HasPrefix(id, prefix) {
			delete(r.skills, id)
		}
	}
	r.fireOnChangeLocked()
}

// FireOnChange manually triggers the registered onChange callback.
func (r *SkillRegistry) FireOnChange() {
	r.mu.RLock()
	fn := r.onChange
	r.mu.RUnlock()
	if fn != nil {
		fn()
	}
}

func (r *SkillRegistry) fireOnChangeLocked() {
	if r.onChange != nil {
		go r.onChange()
	}
}

func stringsJoin(s []string, sep string) string {
	if len(s) == 0 {
		return ""
	}
	result := s[0]
	for i := 1; i < len(s); i++ {
		result += sep + s[i]
	}
	return result
}

// Ensure strings is used.
var _ = sort.Strings
