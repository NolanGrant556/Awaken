package skills

import (
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// SkillRegistry manages the lifecycle of all discovered skills. It merges
// skills from user and project sources, tracks availability
// based on connector presence and MCP server state, and exposes a stable
// snapshot for consumers.
type SkillRegistry struct {
	mu               sync.RWMutex
	skills           map[string]*SkillDefinition
	logger           Logger
	onChange         func()
	connectors       map[string]bool // available connector names
	mcpServers       map[string]bool // available MCP server names
	disabledByPolicy map[string]string
	lastInvoked      map[string]time.Time
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
		skills:           make(map[string]*SkillDefinition),
		logger:           logger,
		connectors:       make(map[string]bool),
		mcpServers:       make(map[string]bool),
		disabledByPolicy: make(map[string]string),
		lastInvoked:      make(map[string]time.Time),
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
	type skillSourceRoot struct {
		src SourceOrigin
		dir string
	}

	// Process sources from lowest to highest priority so merge works.
	ordered := make([]skillSourceRoot, 0, 8)
	for _, src := range []SourceOrigin{SourceLegacy, SourceBundled, SourceMcp, SourcePlugin, SourceUser, SourceAdditional, SourceProject, SourceManaged} {
		dirList, ok := dirs[src]
		if !ok || dirList == "" {
			continue
		}
		for _, dir := range splitSourceDirList(dirList) {
			ordered = append(ordered, skillSourceRoot{src: src, dir: dir})
		}
	}

	// Canonical realpath dedup: the same real directory reached through more
	// than one source configuration (e.g. project workspace equal to the user
	// home makes project and user skills the same directory) must produce
	// exactly one user-facing skill set, never a duplicate batch marked
	// shadowed. Iteration is lowest-to-highest priority, so the last
	// occurrence wins.
	keeperIndex := make(map[string]int, len(ordered))
	for index, root := range ordered {
		keeperIndex[canonicalSkillSourceRoot(root.dir)] = index
	}

	var sourceMaps []map[string]*SkillDefinition
	for index, root := range ordered {
		if keeperIndex[canonicalSkillSourceRoot(root.dir)] != index {
			continue
		}
		loaded := LoadDir(root.dir, root.src)
		if root.src == SourceLegacy {
			loaded = LoadLegacyCommandsDir(root.dir)
		}
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
	for _, def := range next {
		if def == nil {
			continue
		}
		if invokedAt, ok := r.lastInvoked[def.ID]; ok {
			copy := invokedAt
			def.LastInvokedAt = &copy
		}
	}
	return next
}

// MarkInvoked records successful Skill activation independently from the
// reloadable package snapshot.
func (r *SkillRegistry) MarkInvoked(id string, invokedAt time.Time) {
	if r == nil {
		return
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return
	}
	if invokedAt.IsZero() {
		invokedAt = time.Now().UTC()
	} else {
		invokedAt = invokedAt.UTC()
	}
	r.mu.Lock()
	r.lastInvoked[id] = invokedAt
	for _, def := range r.skills {
		if def != nil && def.ID == id {
			copy := invokedAt
			def.LastInvokedAt = &copy
		}
	}
	r.fireOnChangeLocked()
	r.mu.Unlock()
}

func (r *SkillRegistry) LastInvokedAt(id string) (time.Time, bool) {
	if r == nil {
		return time.Time{}, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	value, ok := r.lastInvoked[strings.TrimSpace(id)]
	return value, ok
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

// reevaluateAvailabilityLocked recalculates package policy state. Connector,
// MCP, runtime, resource, and tool readiness are execution-time facts and never
// make an otherwise valid local package unavailable.
func (r *SkillRegistry) reevaluateAvailabilityLocked() {
	r.reevaluateAvailabilityForLocked(r.skills)
}

func (r *SkillRegistry) reevaluateAvailabilityForLocked(defs map[string]*SkillDefinition) {
	for id, def := range defs {
		// Don't touch shadowed entries.
		if def.State == StateShadowed {
			continue
		}
		if isShadowedMapKey(id) {
			continue
		}
		if def.State == StateInvalid {
			continue
		}
		def.Diagnostics = filterDiagnostics(def.Diagnostics, "skill_disabled_by_policy", "skill_connector_not_ready", "skill_mcp_not_ready")
		if reason, disabled := r.disabledByPolicy[def.ID]; disabled {
			def.State = StateDisabledByPolicy
			def.UnavailableReason = reason
			def.UnavailableCode = "skill_disabled_by_policy"
			def.Diagnostics = appendUniqueDiagnostic(def.Diagnostics, SkillDiagnostic{Code: "skill_disabled_by_policy", Message: reason, Severity: "error"})
			continue
		}
		def.State = StateLoaded
		def.UnavailableReason = ""
		def.UnavailableCode = ""
		// Preserve dependency observations as diagnostics only. They help the
		// agent plan remediation without turning Catalog into a readiness gate.
		missingConn := r.missingConnectorsLocked(def.RequiredConnectors)
		if len(missingConn) > 0 {
			def.DeprecatedWarnings = appendUnique(def.DeprecatedWarnings, "execution hint: missing connectors: "+stringsJoin(missingConn, ", "))
			def.Diagnostics = appendUniqueDiagnostic(def.Diagnostics, SkillDiagnostic{Code: "skill_connector_not_ready", Message: "execution hint: missing connectors: " + stringsJoin(missingConn, ", "), Severity: "warning"})
		}
		missingMcp := r.missingMcpServersLocked(def.RequiredMcpServers)
		if len(missingMcp) > 0 {
			def.DeprecatedWarnings = appendUnique(def.DeprecatedWarnings, "execution hint: missing mcp servers: "+stringsJoin(missingMcp, ", "))
			def.Diagnostics = appendUniqueDiagnostic(def.Diagnostics, SkillDiagnostic{Code: "skill_mcp_not_ready", Message: "execution hint: missing mcp servers: " + stringsJoin(missingMcp, ", "), Severity: "warning"})
		}
	}
}

// SetDisabledByPolicy is the only runtime mutation that can make a structurally
// valid package unavailable in the Catalog.
func (r *SkillRegistry) SetDisabledByPolicy(id string, disabled bool, reason string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	id = strings.TrimSpace(id)
	if id == "" {
		return
	}
	if disabled {
		if strings.TrimSpace(reason) == "" {
			reason = "disabled by configured Skill source policy"
		}
		r.disabledByPolicy[id] = reason
	} else {
		delete(r.disabledByPolicy, id)
	}
	r.reevaluateAvailabilityLocked()
	r.fireOnChangeLocked()
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

// ListLoaded returns all model/user usable package definitions.
func (r *SkillRegistry) ListLoaded() []*SkillDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*SkillDefinition, 0)
	for id, def := range r.skills {
		if isShadowedMapKey(id) || def.State != StateLoaded {
			continue
		}
		result = append(result, shallowCopy(def))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

// ListModelInvocable excludes Skills that explicitly opt out of proactive
// model invocation while keeping them available to explicit user entrypoints.
func (r *SkillRegistry) ListModelInvocable() []*SkillDefinition {
	loaded := r.ListLoaded()
	result := loaded[:0]
	for _, def := range loaded {
		if !def.DisableModelInvocation {
			result = append(result, def)
		}
	}
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
		result = append(result, shallowCopy(def))
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
	def := r.skills[id]
	if def == nil {
		return nil
	}
	return shallowCopy(def)
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
	deleteShadowedEntries(r.skills, def.ID)

	next := shallowCopy(def)
	if invokedAt, ok := r.lastInvoked[def.ID]; ok {
		copy := invokedAt
		next.LastInvokedAt = &copy
	}
	r.skills[def.ID] = next
	r.reevaluateAvailabilityLocked()
	r.fireOnChangeLocked()
}

// UnregisterSkill removes a single skill by ID. Used by the MCP bridge
// when a server disconnects or is disabled.
func (r *SkillRegistry) UnregisterSkill(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.skills, id)
	deleteShadowedEntries(r.skills, id)
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

// canonicalSkillSourceRoot resolves a configured skills root to its canonical
// real path so identical directories can be discovered exactly once. When the
// directory does not exist (or resolution fails) the cleaned literal path is
// used, which still dedupes textual duplicates.
func canonicalSkillSourceRoot(dir string) string {
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		return filepath.Clean(resolved)
	}
	return filepath.Clean(dir)
}

func isShadowedMapKey(id string) bool {
	return strings.Contains(id, "__shadowed__") || strings.HasSuffix(id, "__shadowed")
}

func deleteShadowedEntries(defs map[string]*SkillDefinition, id string) {
	for key := range defs {
		if strings.HasPrefix(key, id+"__shadowed") {
			delete(defs, key)
		}
	}
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func appendUniqueDiagnostic(values []SkillDiagnostic, value SkillDiagnostic) []SkillDiagnostic {
	for _, existing := range values {
		if existing.Code == value.Code && existing.Message == value.Message {
			return values
		}
	}
	return append(values, value)
}

func filterDiagnostics(values []SkillDiagnostic, codes ...string) []SkillDiagnostic {
	blocked := make(map[string]bool, len(codes))
	for _, code := range codes {
		blocked[code] = true
	}
	out := values[:0]
	for _, value := range values {
		if !blocked[value.Code] {
			out = append(out, value)
		}
	}
	return out
}
