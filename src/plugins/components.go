package plugins

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"awaken/src/plugincommands"
	"awaken/src/pluginhooks"
	"awaken/src/plugins/ir"
	pluginvariables "awaken/src/plugins/variables"
	"awaken/src/skills"
)

var ErrComponentNotSwitchable = errors.New("component is not switchable")
var ErrToolNotFound = errors.New("plugin MCP tool not found")

func componentConfiguration(component ir.Component, record Record) string {
	if component.Variable == nil || !component.Variable.Required {
		return "ready"
	}
	value, ok := record.Variables[component.Variable.Key]
	if !ok || value.Value == nil && value.SecretRef == "" {
		return "needs_configuration"
	}
	return "ready"
}

func componentSwitchable(component ir.Component, record Record) bool {
	return component.Validation != "invalid" &&
		componentConfiguration(component, record) == "ready" &&
		component.Kind != ir.KindVariable &&
		(component.Mapping == ir.MappingNative || component.Mapping == ir.MappingAdapted)
}

func componentStates(components []ir.Component, record Record) ([]ComponentState, string, string) {
	states := make([]ComponentState, 0, len(components))
	configurationState := "ready"
	compatibilityState := "compatible"
	for _, component := range components {
		desired := "enabled"
		if override, ok := record.Components[component.LogicalID]; ok && override.Desired != "" {
			desired = override.Desired
		}
		ruleActivation := ""
		if component.Kind == ir.KindRule && component.Rule != nil {
			ruleActivation = component.Rule.Activation
			if override, ok := record.Components[component.LogicalID]; ok && override.ActivationOverride != "" {
				ruleActivation = override.ActivationOverride
			}
		}
		if component.Kind == ir.KindMCPServer && component.MCP != nil {
			if toggle, ok := record.MCPServers[component.MCP.ShortName]; ok {
				if toggle.Enabled {
					desired = "enabled"
				} else {
					desired = "disabled"
				}
			}
		}
		// The plugin is the lifecycle owner. Component/server preferences remain
		// persisted while it is off, but no child can be effectively enabled.
		if !record.Enabled {
			desired = "disabled"
		}
		configuration := componentConfiguration(component, record)
		if configuration == "needs_configuration" {
			configurationState = "needs_configuration"
		}
		runtime := "not_applicable"
		if component.Kind == ir.KindMCPServer {
			runtime = "stopped"
		}
		switchable := componentSwitchable(component, record)
		if component.Mapping == ir.MappingRecognizedUnsupported || component.Mapping == ir.MappingInvalid {
			compatibilityState = "partial"
		}
		states = append(states, ComponentState{
			Component: component, DesiredState: desired, RuleActivation: ruleActivation, Configuration: configuration, Runtime: runtime,
			Integrity: "intact", Mapping: component.Mapping, Switchable: switchable,
		})
	}
	return states, configurationState, compatibilityState
}

func (s *Store) ComponentInventory(name string) ([]ComponentState, error) {
	plugin, err := s.Get(name)
	if err != nil {
		return nil, err
	}
	if !plugin.IntegrityValid {
		return nil, ErrDigestChanged
	}
	return append([]ComponentState(nil), plugin.ComponentState...), nil
}

func (s *Store) ComponentDetail(name, logicalID string) (ComponentDetail, error) {
	plugin, err := s.Get(name)
	if err != nil {
		return ComponentDetail{}, err
	}
	var state *ComponentState
	for index := range plugin.ComponentState {
		if plugin.ComponentState[index].Component.LogicalID == logicalID {
			state = &plugin.ComponentState[index]
			break
		}
	}
	if state == nil {
		return ComponentDetail{}, fmt.Errorf("%w: component %s", ErrNotFound, logicalID)
	}
	detail := ComponentDetail{State: *state, Properties: componentProperties(state.Component)}
	sourcePath := componentSourceRelativePath(plugin, state.Component)
	if sourcePath != "" {
		clean := filepath.Clean(filepath.FromSlash(sourcePath))
		path := filepath.Join(plugin.Root, clean)
		if stayErr := resolvedStayWithin(plugin.Root, path); stayErr == nil && isRegularFile(path) {
			raw, readErr := os.ReadFile(path)
			if readErr == nil {
				if state.Component.Kind == ir.KindMCPServer {
					detail.Source = string(raw)
					detail.SourceEditable = true
				} else if state.Component.Kind == ir.KindHook {
					detail.Source = redactHookSource(raw)
				} else {
					detail.Source = string(raw)
				}
				if state.Component.Kind == ir.KindSkill || state.Component.Kind == ir.KindRule || state.Component.Kind == ir.KindAgent || state.Component.Kind == ir.KindCommand {
					_, detail.Preview = skills.SplitSkillPreview(string(raw))
				}
			}
		}
	}
	if state.Component.Skill != nil {
		detail.Resources = componentResources(plugin.Root, state.Component.Skill.Path)
	}
	return detail, nil
}

func componentSourceRelativePath(plugin *InstalledPlugin, component ir.Component) string {
	sourcePath := strings.Split(component.SourcePath, "#")[0]
	if sourcePath == "" || sourcePath == "variable/" {
		sourcePath = plugin.ManifestPath
	}
	return filepath.ToSlash(filepath.Clean(filepath.FromSlash(sourcePath)))
}

func componentProperties(component ir.Component) map[string]any {
	properties := map[string]any{
		"displayName":     component.DisplayName,
		"componentType":   component.Kind,
		"sourceDialect":   component.SourceDialect,
		"sourcePath":      component.SourcePath,
		"mappingState":    component.Mapping,
		"validationState": component.Validation,
	}
	if component.Skill != nil {
		properties["skill"] = map[string]any{"packagePath": component.Skill.Path}
	}
	if component.MCP != nil {
		properties["mcp"] = map[string]any{
			"shortName":               component.MCP.ShortName,
			"transport":               component.MCP.Transport,
			"environment":             map[bool]string{true: "local_process", false: "remote_endpoint"}[component.MCP.Transport == "stdio"],
			"oauthResourceConfigured": strings.TrimSpace(component.MCP.OAuthResource) != "",
		}
	}
	if component.Rule != nil {
		properties["rule"] = map[string]any{
			"activation":  component.Rule.Activation,
			"scope":       component.Rule.Scope,
			"globs":       append([]string(nil), component.Rule.Globs...),
			"description": component.Rule.Description,
		}
	}
	if component.Agent != nil {
		properties["agent"] = map[string]any{
			"model":               component.Agent.Model,
			"effort":              component.Agent.Effort,
			"readOnly":            component.Agent.ReadonlyHint,
			"maxTurns":            component.Agent.MaxTurns,
			"timeoutSeconds":      component.Agent.TimeoutSeconds,
			"toolCount":           len(component.Agent.Tools),
			"skillReferenceCount": len(component.Agent.Skills),
			"mcpReferenceCount":   len(component.Agent.MCPServers),
		}
	}
	if component.Command != nil {
		acceptsArguments, argumentsRequired := plugincommands.ArgumentPolicy(component.Command)
		properties["command"] = map[string]any{
			"invocationName":    component.Command.InvocationName,
			"argumentHint":      component.Command.ArgumentHint,
			"acceptsArguments":  acceptsArguments,
			"argumentsRequired": argumentsRequired,
			"model":             component.Command.Model,
			"manualOnly":        component.Command.ManualOnly,
		}
	}
	if component.Hook != nil {
		async, asyncRewake := pluginhooks.EffectiveAsync(component.Hook)
		properties["hook"] = map[string]any{
			"sourceEvent":    component.Hook.SourceEvent,
			"mappedEvent":    component.Hook.MappedEvent,
			"matcher":        component.Hook.Matcher,
			"condition":      component.Hook.Condition,
			"handlerType":    component.Hook.HandlerType,
			"scope":          component.Hook.Scope,
			"timeoutSeconds": component.Hook.TimeoutSeconds,
			"async":          async,
			"asyncRewake":    asyncRewake,
			"failurePolicy":  "continue",
		}
	}
	if component.Variable != nil {
		properties["variable"] = map[string]any{
			"key":         component.Variable.Key,
			"type":        component.Variable.Type,
			"title":       component.Variable.Title,
			"description": component.Variable.Description,
			"required":    component.Variable.Required,
			"hasDefault":  component.Variable.Default != nil,
			"secret":      component.Variable.Secret,
		}
	}
	if component.App != nil {
		properties["app"] = map[string]any{"connectorId": component.App.ConnectorID}
	}
	return properties
}

func redactHookSource(raw []byte) string {
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return "Hook source is not available because it could not be safely redacted."
	}
	redactHookSecrets(value)
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "Hook source is not available because it could not be safely redacted."
	}
	return string(encoded)
}

func redactHookSecrets(value any) {
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			if key == "command" || key == "args" || key == "prompt" {
				typed[key] = "<configured>"
				continue
			}
			if key == "secretHeaders" || key == "headers" {
				if values, ok := item.(map[string]any); ok {
					for header := range values {
						values[header] = "***configured***"
					}
					continue
				}
			}
			redactHookSecrets(item)
		}
	case []any:
		for _, item := range typed {
			redactHookSecrets(item)
		}
	}
}

func componentResources(root, relative string) []string {
	base := filepath.Join(root, filepath.FromSlash(relative))
	var resources []string
	_ = filepath.WalkDir(base, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return err
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr == nil {
			resources = append(resources, filepath.ToSlash(rel))
		}
		return nil
	})
	sort.Strings(resources)
	return resources
}

func (s *Store) SetComponentDesired(name, logicalID string, enabled bool) (*InstalledPlugin, error) {
	var plugin *InstalledPlugin
	err := s.withMutationLock(func() error {
		state, err := s.readStateUnlocked()
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		record, ok := state.Plugins[name]
		if !ok {
			return fmt.Errorf("%w: %s", ErrNotFound, name)
		}
		loaded := s.loadRecord(name, record)
		var target *ir.Component
		for index := range loaded.Components {
			if loaded.Components[index].LogicalID == logicalID {
				target = &loaded.Components[index]
				break
			}
		}
		if target == nil {
			return fmt.Errorf("%w: component %s", ErrNotFound, logicalID)
		}
		if !componentSwitchable(*target, record) {
			return fmt.Errorf("%w: %s", ErrComponentNotSwitchable, logicalID)
		}
		if record.Components == nil {
			record.Components = map[string]ComponentOverride{}
		}
		desired := "disabled"
		if enabled {
			desired = "enabled"
		}
		override := record.Components[logicalID]
		override.Desired = desired
		override.BaseRevisionID = target.RevisionID
		record.Components[logicalID] = override
		state.Plugins[name] = record
		if err := s.writeStateUnlocked(state); err != nil {
			return err
		}
		plugin = s.loadRecord(name, record)
		return nil
	})
	if err == nil {
		s.notify()
	}
	return plugin, err
}

func (s *Store) SetMCPToolEnabled(name, logicalID, toolName string, enabled bool) (*InstalledPlugin, error) {
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		return nil, fmt.Errorf("%w: empty tool name", ErrToolNotFound)
	}
	var plugin *InstalledPlugin
	err := s.withMutationLock(func() error {
		state, err := s.readStateUnlocked()
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		record, ok := state.Plugins[name]
		if !ok {
			return fmt.Errorf("%w: %s", ErrNotFound, name)
		}
		loaded := s.loadRecord(name, record)
		var target *ir.Component
		for index := range loaded.Components {
			if loaded.Components[index].LogicalID == logicalID {
				target = &loaded.Components[index]
				break
			}
		}
		if target == nil || target.Kind != ir.KindMCPServer || target.MCP == nil {
			return fmt.Errorf("%w: component %s", ErrNotFound, logicalID)
		}
		if target.Validation == "invalid" || target.Mapping != ir.MappingNative && target.Mapping != ir.MappingAdapted {
			return fmt.Errorf("%w: %s", ErrComponentNotSwitchable, logicalID)
		}
		if record.Components == nil {
			record.Components = map[string]ComponentOverride{}
		}
		override := record.Components[logicalID]
		if override.ToolOverrides == nil {
			override.ToolOverrides = map[string]bool{}
		}
		override.ToolOverrides[toolName] = enabled
		override.BaseRevisionID = target.RevisionID
		record.Components[logicalID] = override
		state.Plugins[name] = record
		if err := s.writeStateUnlocked(state); err != nil {
			return err
		}
		plugin = s.loadRecord(name, record)
		return nil
	})
	if err == nil {
		s.notify()
	}
	return plugin, err
}

func (s *Store) SetRuleActivation(name, logicalID, activation string) (*InstalledPlugin, error) {
	activation = strings.TrimSpace(activation)
	switch activation {
	case "always", "file_globs", "manual", "model_decides":
	default:
		return nil, fmt.Errorf("invalid Rule activation %q", activation)
	}
	var plugin *InstalledPlugin
	err := s.withMutationLock(func() error {
		state, err := s.readStateUnlocked()
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		record, ok := state.Plugins[name]
		if !ok {
			return fmt.Errorf("%w: %s", ErrNotFound, name)
		}
		loaded := s.loadRecord(name, record)
		var target *ir.Component
		for index := range loaded.Components {
			if loaded.Components[index].LogicalID == logicalID {
				target = &loaded.Components[index]
				break
			}
		}
		if target == nil || target.Kind != ir.KindRule || target.Rule == nil {
			return fmt.Errorf("%w: rule %s", ErrNotFound, logicalID)
		}
		if target.Validation == "invalid" || target.Mapping != ir.MappingNative && target.Mapping != ir.MappingAdapted {
			return fmt.Errorf("%w: %s", ErrComponentNotSwitchable, logicalID)
		}
		if record.Components == nil {
			record.Components = map[string]ComponentOverride{}
		}
		override := record.Components[logicalID]
		override.ActivationOverride = activation
		override.BaseRevisionID = target.RevisionID
		record.Components[logicalID] = override
		state.Plugins[name] = record
		if err := s.writeStateUnlocked(state); err != nil {
			return err
		}
		plugin = s.loadRecord(name, record)
		return nil
	})
	if err == nil {
		s.notify()
	}
	return plugin, err
}

func (s *Store) Configuration(name string) ([]VariableField, error) {
	var fields []VariableField
	err := s.withLock(func() error {
		state, err := s.readStateUnlocked()
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		record, ok := state.Plugins[name]
		if !ok {
			return fmt.Errorf("%w: %s", ErrNotFound, name)
		}
		plugin := s.loadRecord(name, record)
		for _, component := range plugin.Components {
			if component.Variable == nil {
				continue
			}
			stored, configured := record.Variables[component.Variable.Key]
			field := VariableField{Key: component.Variable.Key, Schema: *component.Variable, Configured: configured, Masked: component.Variable.Secret}
			for _, dependent := range plugin.Components {
				if dependent.LogicalID == component.LogicalID || !componentReferencesVariable(dependent, component.Variable.Key) {
					continue
				}
				field.Dependencies = append(field.Dependencies, VariableDependency{
					LogicalID: dependent.LogicalID,
					Kind:      dependent.Kind,
					Name:      firstNonEmptyString(dependent.DisplayName, dependent.DeclaredName, dependent.DerivedName, dependent.LogicalID),
				})
			}
			if configured && !component.Variable.Secret {
				field.Value = stored.Value
			}
			fields = append(fields, field)
		}
		sort.Slice(fields, func(i, j int) bool { return fields[i].Key < fields[j].Key })
		return nil
	})
	return fields, err
}

func componentReferencesVariable(component ir.Component, key string) bool {
	markers := []string{"${" + key + "}", "${user_config." + key + "}"}
	contains := func(values ...string) bool {
		for _, value := range values {
			for _, marker := range markers {
				if strings.Contains(value, marker) {
					return true
				}
			}
		}
		return false
	}
	if component.MCP != nil {
		values := []string{component.MCP.Command, component.MCP.Cwd, component.MCP.URL, component.MCP.OAuthResource}
		values = append(values, component.MCP.Args...)
		for envKey, value := range component.MCP.Env {
			values = append(values, envKey, value)
		}
		for header, value := range component.MCP.Headers {
			values = append(values, header, value)
		}
		if contains(values...) {
			return true
		}
	}
	if component.Hook != nil && contains(component.Hook.Command, component.Hook.URL, component.Hook.Prompt) {
		return true
	}
	if component.Agent != nil && contains(component.Agent.Prompt, component.Agent.Description) {
		return true
	}
	return component.Command != nil && contains(component.Command.PromptTemplate)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func (s *Store) UpdateConfiguration(name string, values map[string]any, reset []string) ([]VariableField, error) {
	err := s.withMutationLock(func() error {
		type secretMutation struct {
			key, ref, oldValue string
			hadOld             bool
		}
		var secretMutations []secretMutation
		var retiredSecretRefs []string
		committed := false
		defer func() {
			if committed || s.secrets == nil {
				return
			}
			for index := len(secretMutations) - 1; index >= 0; index-- {
				mutation := secretMutations[index]
				if mutation.hadOld {
					_, _ = s.secrets.Save(name, mutation.key, mutation.ref, mutation.oldValue)
				} else {
					_ = s.secrets.Delete(mutation.ref)
				}
			}
		}()
		state, err := s.readStateUnlocked()
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		record, ok := state.Plugins[name]
		if !ok {
			return fmt.Errorf("%w: %s", ErrNotFound, name)
		}
		plugin := s.loadRecord(name, record)
		specs := map[string]ir.VariableSpec{}
		for _, component := range plugin.Components {
			if component.Variable != nil {
				specs[component.Variable.Key] = *component.Variable
			}
		}
		if record.Variables == nil {
			record.Variables = map[string]VariableValueRef{}
		}
		for _, key := range reset {
			if previous, ok := record.Variables[key]; ok && previous.SecretRef != "" {
				retiredSecretRefs = append(retiredSecretRefs, previous.SecretRef)
			}
			delete(record.Variables, key)
		}
		for key, raw := range values {
			spec, ok := specs[key]
			if !ok {
				return fmt.Errorf("VARIABLE_UNKNOWN: unknown plugin variable %s", key)
			}
			validated, err := pluginvariables.ValidateValue(spec, raw)
			if err != nil {
				return err
			}
			stored := VariableValueRef{UpdatedAt: time.Now().UTC()}
			if spec.Secret {
				text, ok := validated.(string)
				if !ok || s.secrets == nil {
					return fmt.Errorf("VARIABLE_SECRET_STORE_UNAVAILABLE: secret variable %s requires a string secret store", key)
				}
				previous := record.Variables[key]
				mutation := secretMutation{key: key, ref: previous.SecretRef}
				if previous.SecretRef != "" {
					oldValue, loadErr := s.secrets.Load(previous.SecretRef)
					if loadErr != nil {
						return fmt.Errorf("VARIABLE_SECRET_LOAD_FAILED: %w", loadErr)
					}
					mutation.oldValue, mutation.hadOld = oldValue, true
				}
				ref, err := s.secrets.Save(name, key, previous.SecretRef, text)
				if err != nil {
					return fmt.Errorf("VARIABLE_SECRET_SAVE_FAILED: %w", err)
				}
				mutation.ref = ref
				secretMutations = append(secretMutations, mutation)
				stored.SecretRef = ref
			} else {
				stored.Value = validated
			}
			record.Variables[key] = stored
		}
		state.Plugins[name] = record
		if err := s.writeStateUnlocked(state); err != nil {
			return err
		}
		committed = true
		if s.secrets != nil {
			activeSecretRefs := make(map[string]struct{}, len(record.Variables))
			for _, value := range record.Variables {
				if value.SecretRef != "" {
					activeSecretRefs[value.SecretRef] = struct{}{}
				}
			}
			for _, ref := range retiredSecretRefs {
				if _, stillActive := activeSecretRefs[ref]; !stillActive {
					_ = s.secrets.Delete(ref)
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.notify()
	return s.Configuration(name)
}
