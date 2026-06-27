package outputstyle

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

const DefaultStyleName = "default"

type Source string

const (
	SourceBuiltin Source = "builtin"
	SourcePlugin  Source = "plugin"
	SourceManaged Source = "managed"
	SourceUser    Source = "user"
	SourceProject Source = "project"
	SourcePolicy  Source = "policy"
)

var ErrInvalidStyle = errors.New("invalid output style")

type StyleConfig struct {
	Name                   string `json:"name"`
	Description            string `json:"description"`
	Prompt                 string `json:"prompt"`
	Source                 Source `json:"source"`
	KeepCodingInstructions bool   `json:"keep_coding_instructions"`
	ForceForPlugin         bool   `json:"force_for_plugin"`
}

type Catalog struct {
	styles map[string]StyleConfig
	order  []string
}

type styleEntry struct {
	style    StyleConfig
	index    int
	priority int
}

func BuiltinStyles() []StyleConfig {
	return []StyleConfig{
		{
			Name:                   DefaultStyleName,
			Description:            "Awaken's standard concise coding style",
			Prompt:                 "Use Awaken's default coding style: be concise, accurate, and focused on the user's task.",
			Source:                 SourceBuiltin,
			KeepCodingInstructions: true,
		},
		{
			Name:                   "explanatory",
			Description:            "Adds short explanations about implementation choices",
			Prompt:                 "Explain important implementation choices briefly while staying focused on completing the task.",
			Source:                 SourceBuiltin,
			KeepCodingInstructions: true,
		},
		{
			Name:                   "learning",
			Description:            "Encourages learning through small, practical guidance",
			Prompt:                 "Help the user learn by naming meaningful trade-offs and inviting small hands-on contributions when useful.",
			Source:                 SourceBuiltin,
			KeepCodingInstructions: true,
		},
	}
}

func MergeStyles(styles ...StyleConfig) (Catalog, []string) {
	entries := make([]styleEntry, 0, len(styles))
	var warnings []string

	for i, style := range styles {
		normalized, err := normalizeStyle(style)
		if err != nil {
			warnings = append(warnings, err.Error())
			continue
		}
		entries = append(entries, styleEntry{
			style:    normalized,
			index:    i,
			priority: sourcePriority(normalized.Source),
		})
	}

	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].priority != entries[j].priority {
			return entries[i].priority < entries[j].priority
		}
		return entries[i].index < entries[j].index
	})

	winners := make(map[string]styleEntry, len(entries))
	for _, entry := range entries {
		winners[entry.style.Name] = entry
	}

	ordered := make([]styleEntry, 0, len(winners))
	for _, entry := range winners {
		ordered = append(ordered, entry)
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].priority != ordered[j].priority {
			return ordered[i].priority < ordered[j].priority
		}
		return ordered[i].index < ordered[j].index
	})

	catalog := Catalog{
		styles: make(map[string]StyleConfig, len(ordered)),
		order:  make([]string, 0, len(ordered)),
	}
	for _, entry := range ordered {
		catalog.styles[entry.style.Name] = entry.style
		catalog.order = append(catalog.order, entry.style.Name)
	}

	return catalog, warnings
}

func (c Catalog) Get(name string) (StyleConfig, bool) {
	style, ok := c.styles[name]
	return style, ok
}

func (c Catalog) List() []StyleConfig {
	styles := make([]StyleConfig, 0, len(c.order))
	for _, name := range c.order {
		if style, ok := c.styles[name]; ok {
			styles = append(styles, style)
		}
	}
	return styles
}

func ResolveStyle(catalog Catalog, selectedName string) (*StyleConfig, []string) {
	var warnings []string
	forced := forcedPluginStyles(catalog)
	if len(forced) > 0 {
		if len(forced) > 1 {
			warnings = append(warnings, fmt.Sprintf("multiple forced plugin output styles found (%s); using %q", styleNames(forced), forced[0].Name))
		}
		style := forced[0]
		return &style, warnings
	}

	name := strings.TrimSpace(selectedName)
	if name == "" {
		name = DefaultStyleName
	}

	style, ok := catalog.Get(name)
	if !ok {
		warnings = append(warnings, fmt.Sprintf("unknown output style %q", name))
		return nil, warnings
	}
	return &style, warnings
}

func ResolveSelectedStyle(catalog Catalog, selectedName string) (*StyleConfig, []string) {
	return ResolveStyle(catalog, selectedName)
}

func RenderPromptSection(style *StyleConfig) string {
	if style == nil {
		return ""
	}
	if err := ValidateStyle(*style); err != nil {
		return ""
	}
	return fmt.Sprintf("# Output Style: %s\n%s", style.Name, style.Prompt)
}

func ValidateStyle(style StyleConfig) error {
	_, err := normalizeStyle(style)
	return err
}

func forcedPluginStyles(catalog Catalog) []StyleConfig {
	var forced []StyleConfig
	for _, name := range catalog.order {
		style, ok := catalog.styles[name]
		if ok && style.Source == SourcePlugin && style.ForceForPlugin {
			forced = append(forced, style)
		}
	}
	return forced
}

func styleNames(styles []StyleConfig) string {
	names := make([]string, 0, len(styles))
	for _, style := range styles {
		names = append(names, style.Name)
	}
	return strings.Join(names, ", ")
}

func normalizeStyle(style StyleConfig) (StyleConfig, error) {
	style.Name = strings.TrimSpace(style.Name)
	style.Description = strings.TrimSpace(style.Description)
	style.Prompt = strings.TrimSpace(style.Prompt)
	style.Source = Source(strings.TrimSpace(string(style.Source)))

	if style.Name == "" {
		return StyleConfig{}, fmt.Errorf("%w: name is required", ErrInvalidStyle)
	}
	if style.Prompt == "" {
		return StyleConfig{}, fmt.Errorf("%w: prompt is required for %q", ErrInvalidStyle, style.Name)
	}
	if !isValidSource(style.Source) {
		return StyleConfig{}, fmt.Errorf("%w: source %q is not supported for %q", ErrInvalidStyle, style.Source, style.Name)
	}
	return style, nil
}

func isValidSource(source Source) bool {
	_, ok := sourcePriorities[source]
	return ok
}

func sourcePriority(source Source) int {
	priority, ok := sourcePriorities[source]
	if !ok {
		return -1
	}
	return priority
}

var sourcePriorities = map[Source]int{
	SourceBuiltin: 0,
	SourcePlugin:  1,
	SourceUser:    2,
	SourceProject: 3,
	SourceManaged: 4,
	SourcePolicy:  4,
}
