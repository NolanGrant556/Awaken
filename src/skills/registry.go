package skills

import (
	"fmt"
	"sort"
	"strings"
)

type Definition struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Instructions  string   `json:"instructions,omitempty"`
	PromptSnippet string   `json:"prompt_snippet,omitempty"`
	DefaultTools  []string `json:"default_tools,omitempty"`
	Enabled       bool     `json:"enabled"`
	Source        string   `json:"source,omitempty"`
}

type Summary struct {
	SkillCount        int `json:"skill_count"`
	EnabledSkillCount int `json:"enabled_skill_count"`
}

type Snapshot struct {
	Skills  []Definition `json:"skills"`
	Summary Summary      `json:"summary"`
}

type Loader interface {
	Load() ([]Definition, error)
}

type Registry interface {
	Snapshot() Snapshot
	List() []Definition
}

type StaticLoader struct {
	Definitions []Definition
	Err         error
}

func (l StaticLoader) Load() ([]Definition, error) {
	if l.Err != nil {
		return nil, l.Err
	}

	definitions := make([]Definition, 0, len(l.Definitions))
	for _, definition := range l.Definitions {
		definitions = append(definitions, cloneDefinition(definition))
	}
	return definitions, nil
}

type StaticRegistry struct {
	definitions []Definition
}

func NewRegistry(loader Loader) (*StaticRegistry, error) {
	if loader == nil {
		return &StaticRegistry{definitions: []Definition{}}, nil
	}

	definitions, err := loader.Load()
	if err != nil {
		return nil, err
	}

	normalized := make([]Definition, 0, len(definitions))
	seen := make(map[string]struct{}, len(definitions))
	for _, definition := range definitions {
		if err := validateDefinition(definition); err != nil {
			return nil, err
		}
		if _, exists := seen[definition.ID]; exists {
			return nil, fmt.Errorf("duplicate skill id %q", definition.ID)
		}
		seen[definition.ID] = struct{}{}
		normalized = append(normalized, cloneDefinition(definition))
	}

	sort.Slice(normalized, func(i, j int) bool {
		return normalized[i].ID < normalized[j].ID
	})
	return &StaticRegistry{definitions: normalized}, nil
}

func NewStaticRegistry(definitions []Definition) (*StaticRegistry, error) {
	return NewRegistry(StaticLoader{Definitions: definitions})
}

func (r *StaticRegistry) Snapshot() Snapshot {
	if r == nil {
		return Snapshot{Skills: []Definition{}, Summary: Summary{}}
	}

	definitions := make([]Definition, 0, len(r.definitions))
	summary := Summary{}
	for _, definition := range r.definitions {
		definitions = append(definitions, cloneDefinition(definition))
		summary.SkillCount++
		if definition.Enabled {
			summary.EnabledSkillCount++
		}
	}

	return Snapshot{
		Skills:  definitions,
		Summary: summary,
	}
}

func (r *StaticRegistry) List() []Definition {
	return r.Snapshot().Skills
}

func validateDefinition(definition Definition) error {
	if strings.TrimSpace(definition.ID) == "" {
		return fmt.Errorf("skill id is required")
	}
	if strings.TrimSpace(definition.Name) == "" {
		return fmt.Errorf("skill %q name is required", definition.ID)
	}
	if strings.TrimSpace(definition.Description) == "" {
		return fmt.Errorf("skill %q description is required", definition.ID)
	}
	return nil
}

func cloneDefinition(definition Definition) Definition {
	cloned := definition
	cloned.DefaultTools = append([]string(nil), definition.DefaultTools...)
	sort.Strings(cloned.DefaultTools)
	return cloned
}
