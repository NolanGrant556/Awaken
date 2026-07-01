package skills

import (
	"sort"
	"strings"
	"sync"
)

// SkillRegistry manages the lifecycle of all discovered skills. It merges
// skills from bundled, user, and project sources, tracks availability
// based on connector presence, and exposes a stable snapshot for consumers.
type SkillRegistry struct {
	mu         sync.RWMutex
	skills     map[string]*SkillDefinition
	logger     Logger
	onChange   func()
	connectors map[string]bool // available connector names
}

// Logger is the minimal logging surface consumed by the skills package.
type Logger interface {
	Warnf(format string, args ...any)
}

type noopLogger struct{}

func (noopLogger) Warnf(string, ...any) {}

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
	}
}

// SetOnChange registers a callback invoked whenever the skill set changes
// (load, connector status change, etc.). Only one callback is supported.
func (r *SkillRegistry) SetOnChange(fn func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onChange = fn
}

// LoadFromSources clears the registry and loads skills from the given
// source directories. dirs maps SourceOrigin to the absolute directory
// path. Bundled skills are passed in via the bundled map.
func (r *SkillRegistry) LoadFromSources(dirs map[SourceOrigin]string, bundled map[string]*SkillDefinition) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var sourceMaps []map[string]*SkillDefinition

	// Process sources from lowest to highest priority so merge works.
	for _, src := range []SourceOrigin{SourceBundled, SourceUser, SourceProject} {
		if src == SourceBundled {
			if bundled != nil {
				// Mark all bundled as SourceBundled.
				for _, def := range bundled {
					def.Source = SourceBundled
				}
				sourceMaps = append(sourceMaps, bundled)
			}
			continue
		}
		dir, ok := dirs[src]
		if !ok || dir == "" {
			continue
		}
		loaded := LoadDir(dir, src)
		if len(loaded) > 0 {
			sourceMaps = append(sourceMaps, loaded)
		}
	}

	r.skills = MergeSkills(sourceMaps...)
	r.reevaluateAvailabilityLocked()
	r.fireOnChangeLocked()
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

// reevaluateAvailabilityLocked recalculates State for every skill based on
// current connector availability. Must be called with mu held.
func (r *SkillRegistry) reevaluateAvailabilityLocked() {
	for id, def := range r.skills {
		// Don't touch shadowed entries.
		if def.State == StateShadowed {
			continue
		}
		if strings.HasSuffix(id, "__shadowed") {
			continue
		}
		if len(def.RequiredConnectors) == 0 {
			def.State = StateAvailable
			def.UnavailableReason = ""
			continue
		}
		missing := r.missingConnectorsLocked(def.RequiredConnectors)
		if len(missing) > 0 {
			def.State = StateUnavailable
			def.UnavailableReason = "missing connectors: " + stringsJoin(missing, ", ")
		} else {
			def.State = StateAvailable
			def.UnavailableReason = ""
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
	result := make([]*SkillDefinition, 0)
	for id, def := range r.skills {
		if strings.HasSuffix(id, "__shadowed") {
			// Shadowed entries have synthetic keys, but we include them.
		}
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