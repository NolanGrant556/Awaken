// Package pluginrules assembles enabled Plugin Rule components into bounded
// instruction fragments. It never owns the final run snapshot; src/instructions
// applies priority, budgets, digest and freeze semantics.
package pluginrules

import (
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"awaken/src/instructions"
	"awaken/src/plugins/ir"
)

type Entry struct {
	PluginName         string
	PluginDigest       string
	PluginRoot         string
	Component          ir.Component
	DesiredState       string
	ActivationOverride string
}

type Selection struct {
	PluginName   string
	PluginDigest string
	LogicalID    string
}

type Request struct {
	Entries   []Entry
	Manual    []Selection
	FilePaths []string
}

func Assemble(request Request) []instructions.ExtraSource {
	manual := make(map[string]bool, len(request.Manual))
	for _, selection := range request.Manual {
		manual[selection.PluginName+"\x00"+selection.PluginDigest+"\x00"+selection.LogicalID] = true
	}
	entries := append([]Entry(nil), request.Entries...)
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].PluginName != entries[j].PluginName {
			return entries[i].PluginName < entries[j].PluginName
		}
		return entries[i].Component.LogicalID < entries[j].Component.LogicalID
	})
	var out []instructions.ExtraSource
	for _, entry := range entries {
		component := entry.Component
		if entry.DesiredState != "enabled" || component.Kind != ir.KindRule || component.Rule == nil || component.Validation == "invalid" || component.Mapping != ir.MappingNative && component.Mapping != ir.MappingAdapted {
			continue
		}
		isManual := manual[entry.PluginName+"\x00"+entry.PluginDigest+"\x00"+component.LogicalID]
		activation := firstNonEmpty(entry.ActivationOverride, component.Rule.Activation)
		applies := isManual || activation == "always" || activation == "file_globs" && matchesAny(component.Rule.Globs, request.FilePaths)
		if !applies || strings.TrimSpace(component.Rule.Body) == "" {
			continue
		}
		scope, placement := instructions.ScopePluginUser, instructions.ExtraBeforeGlobal
		if component.Rule.Scope == "project" && !isManual {
			scope, placement = instructions.ScopePluginProject, instructions.ExtraAfterProject
		} else if isManual {
			scope, placement = instructions.ScopePluginManual, instructions.ExtraAfterProject
		}
		out = append(out, instructions.ExtraSource{
			Scope: scope, Placement: placement, ScopeRoot: filepath.Clean(entry.PluginRoot),
			DisplayPath: "plugin:" + entry.PluginName + "/" + component.LogicalID,
			Content:     component.Rule.Body,
		})
	}
	return out
}

func matchesAny(patterns, values []string) bool {
	for _, pattern := range patterns {
		expression, err := globRegexp(pattern)
		if err != nil {
			continue
		}
		for _, value := range values {
			normalized := filepath.ToSlash(value)
			if expression.MatchString(normalized) || expression.MatchString(strings.TrimPrefix(normalized, "./")) {
				return true
			}
		}
	}
	return false
}

func globRegexp(pattern string) (*regexp.Regexp, error) {
	pattern = filepath.ToSlash(strings.TrimSpace(pattern))
	var expression strings.Builder
	expression.WriteString("^")
	for index := 0; index < len(pattern); index++ {
		switch pattern[index] {
		case '*':
			if index+1 < len(pattern) && pattern[index+1] == '*' {
				index++
				if index+1 < len(pattern) && pattern[index+1] == '/' {
					index++
					expression.WriteString("(?:.*/)?")
				} else {
					expression.WriteString(".*")
				}
			} else {
				expression.WriteString("[^/]*")
			}
		case '?':
			expression.WriteString("[^/]")
		default:
			expression.WriteString(regexp.QuoteMeta(string(pattern[index])))
		}
	}
	expression.WriteString("$")
	return regexp.Compile(expression.String())
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
