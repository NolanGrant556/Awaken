package runtime

import (
	"sort"
	"strings"

	"awaken/src/hookruntime"
	"awaken/src/instructions"
)

func collectBoundPluginNames(tokens []ComposerToken, hookSnapshot hookruntime.AvailabilitySnapshot, ruleSources []instructions.ExtraSource) []string {
	seen := map[string]struct{}{}
	add := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		seen[name] = struct{}{}
	}
	for _, token := range tokens {
		add(token.Payload.PluginName)
		if strings.EqualFold(strings.TrimSpace(token.Type), "skill") && strings.EqualFold(strings.TrimSpace(token.Payload.SourceOrigin), "plugin") {
			add(token.Payload.SkillID)
		}
	}
	for _, source := range hookSnapshot.Sources {
		add(source.PluginName)
	}
	for _, hooks := range hookSnapshot.HooksByEvent {
		for _, hook := range hooks {
			add(hook.PluginName)
		}
	}
	for _, source := range ruleSources {
		add(pluginNameFromRuleDisplayPath(source.DisplayPath))
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func pluginNameFromRuleDisplayPath(path string) string {
	path = strings.TrimSpace(path)
	if !strings.HasPrefix(path, "plugin:") {
		return ""
	}
	rest := strings.TrimPrefix(path, "plugin:")
	name, _, found := strings.Cut(rest, "/")
	if !found {
		return strings.TrimSpace(rest)
	}
	return strings.TrimSpace(name)
}
