// Package plugincommands expands normalized Plugin Command templates without
// side effects. Starting a run remains owned by runtime.RunService.
package plugincommands

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"awaken/src/plugins/ir"
)

var ErrArgumentsRequired = errors.New("plugin command arguments required")

var positionalPattern = regexp.MustCompile(`\$([0-9]+)`)
var indexedArgumentsPattern = regexp.MustCompile(`\$ARGUMENTS\[([0-9]+)\]`)
var requiredHintPattern = regexp.MustCompile(`<[^>]+>`)

type Expansion struct {
	InvocationName string          `json:"invocationName"`
	Prompt         string          `json:"prompt"`
	Arguments      []string        `json:"arguments"`
	Diagnostics    []ir.Diagnostic `json:"diagnostics,omitempty"`
	Revision       uint64          `json:"revision,omitempty"`
}

func ArgumentPolicy(spec *ir.CommandSpec) (accepts bool, required bool) {
	if spec == nil {
		return false, false
	}
	template := spec.PromptTemplate
	hasPlaceholders := strings.Contains(template, "$ARGUMENTS") || positionalPattern.MatchString(template) || indexedArgumentsPattern.MatchString(template)
	accepts = hasPlaceholders || strings.TrimSpace(spec.ArgumentHint) != ""
	required = hasPlaceholders || requiredHintPattern.MatchString(spec.ArgumentHint)
	return accepts, required
}

func Expand(component ir.Component, arguments []string) (Expansion, error) {
	if component.Kind != ir.KindCommand || component.Command == nil || component.Validation == "invalid" || component.Mapping != ir.MappingNative && component.Mapping != ir.MappingAdapted {
		return Expansion{}, fmt.Errorf("command is not runnable")
	}
	template := component.Command.PromptTemplate
	hasPlaceholders := strings.Contains(template, "$ARGUMENTS") || positionalPattern.MatchString(template) || indexedArgumentsPattern.MatchString(template)
	_, requiresArguments := ArgumentPolicy(component.Command)
	if requiresArguments && len(arguments) == 0 {
		return Expansion{}, fmt.Errorf("%w: %s", ErrArgumentsRequired, component.Command.ArgumentHint)
	}
	joined := strings.Join(arguments, " ")
	prompt := indexedArgumentsPattern.ReplaceAllStringFunc(template, func(match string) string {
		parts := indexedArgumentsPattern.FindStringSubmatch(match)
		var index int
		_, _ = fmt.Sscanf(parts[1], "%d", &index)
		if index < 0 || index >= len(arguments) {
			return match
		}
		return arguments[index]
	})
	prompt = strings.ReplaceAll(prompt, "$ARGUMENTS", joined)
	prompt = positionalPattern.ReplaceAllStringFunc(prompt, func(match string) string {
		var index int
		_, _ = fmt.Sscanf(strings.TrimPrefix(match, "$"), "%d", &index)
		argumentIndex := index - 1
		if index == 0 {
			argumentIndex = 0
		}
		if argumentIndex < 0 || argumentIndex >= len(arguments) {
			return match
		}
		return arguments[argumentIndex]
	})
	var diagnostics []ir.Diagnostic
	if positionalPattern.MatchString(prompt) || indexedArgumentsPattern.MatchString(prompt) {
		diagnostics = append(diagnostics, ir.Diagnostic{Code: "command_argument_missing", Message: "one or more positional arguments were not supplied", Severity: "error", ComponentKind: ir.KindCommand, ComponentID: component.LogicalID})
		return Expansion{InvocationName: component.Command.InvocationName, Prompt: prompt, Arguments: append([]string(nil), arguments...), Diagnostics: diagnostics}, ErrArgumentsRequired
	}
	if len(arguments) > 0 && !hasPlaceholders {
		prompt += "\n\nARGUMENTS: " + joined
	}
	return Expansion{InvocationName: component.Command.InvocationName, Prompt: prompt, Arguments: append([]string(nil), arguments...), Diagnostics: diagnostics}, nil
}
