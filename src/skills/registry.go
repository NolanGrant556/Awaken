package skills

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
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
	mu                    sync.RWMutex
	skills                map[string]*SkillDefinition
	logger                Logger
	onChange              func()
	catalogListeners      map[uint64]func()
	nextCatalogListenerID uint64
	connectors            map[string]bool // available connector names
	mcpServers            map[string]bool // available MCP server names
	disabledByPolicy      map[string]string
	disabledStorePath     string
	lastInvoked           map[string]time.Time
	// toolStateFingerprint is the last model-visible Catalog and invocation
	// binding state published through onChange. Keeping the previous digest is
	// important: by the time a reload starts, an edited SKILL.md has already
	// replaced the bytes reachable through the old registry paths.
	toolStateFingerprint       string
	composerCatalogFingerprint string
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
	registry := &SkillRegistry{
		skills:           make(map[string]*SkillDefinition),
		logger:           logger,
		connectors:       make(map[string]bool),
		mcpServers:       make(map[string]bool),
		disabledByPolicy: make(map[string]string),
		lastInvoked:      make(map[string]time.Time),
		catalogListeners: make(map[uint64]func()),
	}
	registry.toolStateFingerprint = registry.computeToolStateFingerprintLocked()
	registry.composerCatalogFingerprint = registry.computeComposerCatalogFingerprintLocked()
	return registry
}

// SetOnChange registers a callback invoked when the model-visible Skill
// Catalog or one of its invocation bindings changes. Only one callback is
// supported; presentation-only metadata does not publish through this hook.
func (r *SkillRegistry) SetOnChange(fn func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onChange = fn
}

// SubscribeCatalogChanges registers a presentation-catalog listener without
// replacing the model-facing Manager callback owned by SetOnChange. The
// returned function is idempotent and removes only this listener.
func (r *SkillRegistry) SubscribeCatalogChanges(fn func()) func() {
	if r == nil || fn == nil {
		return func() {}
	}
	r.mu.Lock()
	r.nextCatalogListenerID++
	id := r.nextCatalogListenerID
	r.catalogListeners[id] = fn
	r.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			r.mu.Lock()
			delete(r.catalogListeners, id)
			r.mu.Unlock()
		})
	}
}

// LoadFromSources clears the registry and loads skills from the given source
// directories. dirs maps SourceOrigin to the absolute directory path.
func (r *SkillRegistry) LoadFromSources(dirs map[SourceOrigin]string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.replaceLocalSourcesLocked(dirs)
	r.fireOnToolStateChangeLocked()
}

// ReloadLocalSources reloads user and project skills from disk while keeping
// dynamic MCP-sourced skills that are managed by the MCP bridge.
func (r *SkillRegistry) ReloadLocalSources(dirs map[SourceOrigin]string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.replaceLocalSourcesLocked(dirs)
	r.fireOnToolStateChangeLocked()
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
	r.loadDisabledLocked()
	r.reevaluateAvailabilityLocked()
}

func (r *SkillRegistry) buildLocalSourcesLocked(dirs map[SourceOrigin]string) map[string]*SkillDefinition {
	type skillSourceRoot struct {
		src SourceOrigin
		dir string
	}

	// Process sources from lowest to highest priority so merge works.
	ordered := make([]skillSourceRoot, 0, 8)
	for _, src := range []SourceOrigin{SourceLegacy, SourceBundled, SourceMcp, SourceAgents, SourcePlugin, SourceUser, SourceAdditional, SourceProject, SourceManaged} {
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

// ToolStateFingerprint returns the currently published model-visible Catalog
// and invocation-binding identity. It is intentionally opaque; consumers use
// it only to coalesce callbacks that observe the same final registry state.
func (r *SkillRegistry) ToolStateFingerprint() string {
	if r == nil {
		return ""
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.toolStateFingerprint
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
	r.fireOnToolStateChangeLocked()
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
	r.fireOnToolStateChangeLocked()
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

// SetDisabledByPolicy is the runtime mutation that makes a structurally valid
// package unavailable in the Catalog. When a persistent store path has been
// configured, the disabled set is also written to disk so the state survives
// restarts.
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
	r.fireOnToolStateChangeLocked()
	r.persistDisabledLocked()
}

// SetDisabledStorePath configures the file used to persist the disabled-by-user
// skill set. The path is created on first write if its parent directory exists.
func (r *SkillRegistry) SetDisabledStorePath(path string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.disabledStorePath = strings.TrimSpace(path)
	if r.disabledStorePath != "" {
		r.loadDisabledLocked()
	}
}

func (r *SkillRegistry) loadDisabledLocked() {
	if r.disabledStorePath == "" {
		return
	}
	data, err := os.ReadFile(r.disabledStorePath)
	if err != nil {
		if !os.IsNotExist(err) {
			r.logger.Warnf("failed to read disabled skills store: %v", err)
		}
		return
	}
	var ids []string
	if err := json.Unmarshal(data, &ids); err != nil {
		r.logger.Warnf("failed to parse disabled skills store: %v", err)
		return
	}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := r.disabledByPolicy[id]; !ok {
			r.disabledByPolicy[id] = "disabled by user"
		}
	}
}

func (r *SkillRegistry) persistDisabledLocked() {
	if r.disabledStorePath == "" {
		return
	}
	ids := make([]string, 0, len(r.disabledByPolicy))
	for id := range r.disabledByPolicy {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	data, err := json.Marshal(ids)
	if err != nil {
		r.logger.Warnf("failed to encode disabled skills store: %v", err)
		return
	}
	if err := os.WriteFile(r.disabledStorePath, data, 0o600); err != nil {
		r.logger.Warnf("failed to write disabled skills store: %v", err)
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
	r.fireOnToolStateChangeLocked()
}

// UnregisterSkill removes a single skill by ID. Used by the MCP bridge
// when a server disconnects or is disabled.
func (r *SkillRegistry) UnregisterSkill(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.skills, id)
	deleteShadowedEntries(r.skills, id)
	r.fireOnToolStateChangeLocked()
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
	r.fireOnToolStateChangeLocked()
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

// fireOnToolStateChangeLocked publishes only changes that can alter the
// model-facing Skill schema/catalog or the runtime binding used by a model
// invocation. UI-only metadata such as LastInvokedAt and readiness diagnostics
// deliberately does not churn the process tool registry.
func (r *SkillRegistry) fireOnToolStateChangeLocked() {
	r.fireComposerCatalogChangeLocked()
	next := r.computeToolStateFingerprintLocked()
	if next == r.toolStateFingerprint {
		return
	}
	r.toolStateFingerprint = next
	r.fireOnChangeLocked()
}

type skillComposerCatalogState struct {
	CatalogEntryID    string
	ID                string
	Name              string
	DisplayName       string
	Description       string
	WhenToUse         string
	State             SkillState
	UnavailableReason string
	Source            SourceOrigin
	RootDir           string
	SkillFilePath     string
	ProjectPath       string
	UserInvocable     bool
	ModelInvocable    bool
}

func (r *SkillRegistry) computeComposerCatalogFingerprintLocked() string {
	states := make([]skillComposerCatalogState, 0, len(r.skills))
	for _, definition := range r.skills {
		if definition == nil {
			continue
		}
		states = append(states, skillComposerCatalogState{
			CatalogEntryID: CatalogEntryID(definition),
			ID:             definition.ID, Name: definition.Name, DisplayName: definition.DisplayName,
			Description: definition.Description, WhenToUse: definition.WhenToUse,
			State: definition.State, UnavailableReason: definition.UnavailableReason,
			Source: definition.Source, RootDir: definition.RootDir,
			SkillFilePath: definition.SkillFilePath, ProjectPath: definition.ProjectPath,
			UserInvocable: definition.UserInvocable, ModelInvocable: !definition.DisableModelInvocation,
		})
	}
	sort.Slice(states, func(i, j int) bool {
		if states[i].CatalogEntryID != states[j].CatalogEntryID {
			return states[i].CatalogEntryID < states[j].CatalogEntryID
		}
		return states[i].ID < states[j].ID
	})
	encoded, _ := json.Marshal(states)
	sum := sha256.Sum256(encoded)
	return fmt.Sprintf("%x", sum[:])
}

func (r *SkillRegistry) fireComposerCatalogChangeLocked() {
	next := r.computeComposerCatalogFingerprintLocked()
	if next == r.composerCatalogFingerprint {
		return
	}
	r.composerCatalogFingerprint = next
	listeners := make([]func(), 0, len(r.catalogListeners))
	for _, listener := range r.catalogListeners {
		listeners = append(listeners, listener)
	}
	for _, listener := range listeners {
		go listener()
	}
}

type skillToolState struct {
	ID                        string
	Description               string
	WhenToUse                 string
	Source                    SourceOrigin
	ProjectPath               string
	Version                   string
	RequiredTools             []string
	RequiredCapabilities      []string
	RequiredRuntimeComponents []string
	Arguments                 []SkillArgument
	DisableModelInvocation    bool
	Model                     string
	Effort                    string
	Context                   ContextMode
	Agent                     string
	ToolHints                 []ToolHint
	RootDir                   string
	ResolvedRootDir           string
	SkillFilePath             string
	Body                      string
	SkillBodySHA256           string
	McpBinding                *McpBinding
	BodyFetcherIdentity       string
}

func (r *SkillRegistry) computeToolStateFingerprintLocked() string {
	states := make([]skillToolState, 0, len(r.skills))
	for key, def := range r.skills {
		if def == nil || isShadowedMapKey(key) || def.State != StateLoaded || def.DisableModelInvocation {
			continue
		}
		states = append(states, skillToolState{
			ID:                        def.ID,
			Description:               normalizeCatalogText(def.Description),
			WhenToUse:                 normalizeCatalogText(def.WhenToUse),
			Source:                    def.Source,
			ProjectPath:               strings.TrimSpace(def.ProjectPath),
			Version:                   strings.TrimSpace(def.Version),
			RequiredTools:             append([]string(nil), def.RequiredTools...),
			RequiredCapabilities:      append([]string(nil), def.RequiredCapabilities...),
			RequiredRuntimeComponents: append([]string(nil), def.RequiredRuntimeComponents...),
			Arguments:                 append([]SkillArgument(nil), def.Arguments...),
			DisableModelInvocation:    def.DisableModelInvocation,
			Model:                     def.Model,
			Effort:                    def.Effort,
			Context:                   def.Context,
			Agent:                     def.Agent,
			ToolHints:                 append([]ToolHint(nil), def.ToolHints...),
			RootDir:                   filepath.Clean(def.RootDir),
			ResolvedRootDir:           filepath.Clean(def.ResolvedRootDir),
			SkillFilePath:             filepath.Clean(def.SkillFilePath),
			Body:                      def.Body,
			SkillBodySHA256:           skillBodyDigest(def.SkillFilePath),
			McpBinding:                cloneMcpBinding(def.McpBinding),
			BodyFetcherIdentity:       bodyFetcherIdentity(def.BodyFetcher),
		})
	}
	sort.Slice(states, func(i, j int) bool { return states[i].ID < states[j].ID })
	encoded, err := json.Marshal(states)
	if err != nil {
		// All fields above are deliberately JSON-safe. Retaining a deterministic
		// fallback keeps a future incompatible field from turning every reload
		// into a false change storm.
		return fmt.Sprintf("skill_tool_state_error:%T:%v", states, err)
	}
	digest := sha256.Sum256(encoded)
	return fmt.Sprintf("%x", digest[:])
}

func normalizeCatalogText(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func skillBodyDigest(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "error:" + err.Error()
	}
	parsed, err := ParseFrontmatter(string(raw))
	if err != nil {
		return "error:" + err.Error()
	}
	digest := sha256.Sum256([]byte(parsed.Body))
	return fmt.Sprintf("%x", digest[:])
}

func cloneMcpBinding(binding *McpBinding) *McpBinding {
	if binding == nil {
		return nil
	}
	clone := *binding
	return &clone
}

func bodyFetcherIdentity(fetcher McpBodyFetcher) string {
	if fetcher == nil {
		return ""
	}
	value := reflect.ValueOf(fetcher)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Map, reflect.Pointer, reflect.Slice, reflect.UnsafePointer:
		if value.IsNil() {
			return fmt.Sprintf("%T:nil", fetcher)
		}
		return fmt.Sprintf("%T:%x", fetcher, value.Pointer())
	default:
		return fmt.Sprintf("%T:%v", fetcher, fetcher)
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
