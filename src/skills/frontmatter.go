package skills

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

var knownFrontmatterFields = map[string]struct{}{
	"name": {}, "displayName": {}, "display_name": {}, "description": {}, "metadata": {},
	"when_to_use": {}, "whenToUse": {}, "version": {},
	"allowed-tools": {}, "allowedTools": {}, "toolPolicy": {},
	"user-invocable": {}, "userInvocable": {},
	"disable-model-invocation": {}, "disableModelInvocation": {},
	"arguments": {}, "context": {}, "agent": {}, "model": {}, "effort": {},
	"hooks": {}, "paths": {}, "shell": {}, "awaken": {},
	// Legacy Awaken fields remain parseable but are execution-time hints only.
	"requiredTools": {}, "requiredCapabilities": {}, "requiredRuntimeComponents": {},
	"requiredResources": {}, "requiredConnectors": {}, "requiredMcpServers": {},
}

var commonToolAliases = map[string]string{
	"read":  "Read",
	"write": "Write",
	"edit":  "Edit",
	"bash":  "Bash",
	"glob":  "Glob",
	"grep":  "Grep",
}

// ParseFrontmatter extracts YAML frontmatter and Markdown body from raw
// SKILL.md content. Parsing is intentionally permissive: unknown fields are
// preserved, supported field aliases are normalized, and execution-readiness
// declarations never make the package invalid.
func ParseFrontmatter(raw string) (*SkillDefinition, error) {
	fm, body, err := splitFrontmatter(raw)
	if err != nil {
		return nil, err
	}

	fields := map[string]any{}
	if err := yaml.Unmarshal([]byte(fm), &fields); err != nil {
		return nil, fmt.Errorf("parse skill frontmatter: %w", err)
	}

	def := &SkillDefinition{
		Body:          strings.TrimSpace(body),
		Context:       ContextInline,
		UserInvocable: true,
		UnknownFields: map[string]any{},
	}
	if err := decodeKnownFrontmatter(fields, def); err != nil {
		return nil, err
	}
	if strings.TrimSpace(def.Description) == "" {
		def.Description = descriptionFromBody(def.Body)
		if def.Description != "" {
			def.DeprecatedWarnings = append(def.DeprecatedWarnings, "description derived from SKILL.md body")
		}
	}
	def.ToolHints = compileToolHints(def.AllowedTools, &def.DeprecatedWarnings)
	for key, value := range fields {
		if _, known := knownFrontmatterFields[key]; !known {
			def.UnknownFields[key] = value
		}
	}
	if len(def.UnknownFields) == 0 {
		def.UnknownFields = nil
	}
	for _, warning := range def.DeprecatedWarnings {
		def.Diagnostics = append(def.Diagnostics, SkillDiagnostic{Code: "skill_frontmatter_warning", Message: warning, Severity: "warning"})
	}
	return def, nil
}

func decodeKnownFrontmatter(fields map[string]any, def *SkillDefinition) error {
	decode := func(key string, target any) error {
		value, ok := fields[key]
		if !ok {
			return nil
		}
		data, err := yaml.Marshal(value)
		if err != nil {
			return fmt.Errorf("parse skill frontmatter field %s: %w", key, err)
		}
		if err := yaml.Unmarshal(data, target); err != nil {
			return fmt.Errorf("parse skill frontmatter field %s: %w", key, err)
		}
		return nil
	}

	for key, target := range map[string]any{
		"name": &def.Name, "description": &def.Description, "version": &def.Version,
		"model": &def.Model, "effort": &def.Effort, "context": &def.Context,
		"agent": &def.Agent, "shell": &def.Shell,
	} {
		if err := decode(key, target); err != nil {
			return err
		}
	}
	for key, target := range map[string]any{
		"toolPolicy": &def.ToolPolicy, "requiredTools": &def.RequiredTools,
		"requiredCapabilities":      &def.RequiredCapabilities,
		"requiredRuntimeComponents": &def.RequiredRuntimeComponents,
		"requiredResources":         &def.RequiredResources,
		"requiredConnectors":        &def.RequiredConnectors,
		"requiredMcpServers":        &def.RequiredMcpServers, "awaken": &def.Awaken,
	} {
		if err := decode(key, target); err != nil {
			def.DeprecatedWarnings = append(def.DeprecatedWarnings, fmt.Sprintf("ignored non-universal field %s: %v", key, err))
		}
	}
	if _, ok := fields["display_name"]; ok {
		if err := decode("display_name", &def.DisplayName); err != nil {
			return err
		}
	} else if _, ok := fields["displayName"]; ok {
		if err := decode("displayName", &def.DisplayName); err != nil {
			return err
		}
	} else if metadata, ok := fields["metadata"].(map[string]any); ok {
		displayName, _ := metadata["display_name"].(string)
		compatDisplayName, _ := metadata["displayName"].(string)
		def.DisplayName = firstNonEmpty(displayName, compatDisplayName)
	}

	if _, canonical := fields["when_to_use"]; canonical {
		if err := decode("when_to_use", &def.WhenToUse); err != nil {
			return err
		}
		if _, duplicate := fields["whenToUse"]; duplicate {
			def.DeprecatedWarnings = append(def.DeprecatedWarnings, "when_to_use overrides whenToUse")
		}
	} else if _, compat := fields["whenToUse"]; compat {
		if err := decode("whenToUse", &def.WhenToUse); err != nil {
			return err
		}
		def.DeprecatedWarnings = append(def.DeprecatedWarnings, "whenToUse normalized to when_to_use")
	}

	allowedKey := "allowed-tools"
	if _, ok := fields[allowedKey]; !ok {
		allowedKey = "allowedTools"
	}
	if raw, ok := fields[allowedKey]; ok {
		allowed, err := decodeAllowedTools(raw)
		if err != nil {
			def.DeprecatedWarnings = append(def.DeprecatedWarnings, fmt.Sprintf("ignored invalid %s hint: %v", allowedKey, err))
		} else {
			def.AllowedTools = allowed
		}
		if allowedKey == "allowedTools" {
			def.DeprecatedWarnings = append(def.DeprecatedWarnings, "allowedTools normalized to allowed-tools")
		}
		if _, duplicate := fields["allowedTools"]; duplicate && allowedKey == "allowed-tools" {
			def.DeprecatedWarnings = append(def.DeprecatedWarnings, "allowed-tools overrides allowedTools")
		}
	} else if len(def.ToolPolicy.Allow) > 0 {
		def.AllowedTools = append([]string(nil), def.ToolPolicy.Allow...)
		def.DeprecatedWarnings = append(def.DeprecatedWarnings, "toolPolicy.allow retained as non-enforcing allowed-tools hint")
	}

	if raw, ok := firstField(fields, "user-invocable", "userInvocable"); ok {
		if err := decodeAny(raw, &def.UserInvocable); err != nil {
			return fmt.Errorf("parse user-invocable: %w", err)
		}
		def.UserInvocableSet = true
	}
	if raw, ok := firstField(fields, "disable-model-invocation", "disableModelInvocation"); ok {
		if err := decodeAny(raw, &def.DisableModelInvocation); err != nil {
			return fmt.Errorf("parse disable-model-invocation: %w", err)
		}
	}

	if raw, ok := fields["arguments"]; ok {
		switch value := raw.(type) {
		case string:
			def.ArgumentHint = strings.TrimSpace(value)
		default:
			if err := decodeAny(value, &def.Arguments); err != nil {
				return fmt.Errorf("parse skill frontmatter field arguments: %w", err)
			}
		}
	}
	if raw, ok := fields["paths"]; ok {
		paths, err := decodeStringList(raw)
		if err != nil {
			return fmt.Errorf("parse skill frontmatter field paths: %w", err)
		}
		def.Paths = paths
	}
	if raw, ok := fields["hooks"]; ok {
		def.HooksRaw = raw
		_ = decodeAny(raw, &def.Hooks) // compatibility view; unknown hook keys remain in HooksRaw.
	}
	return nil
}

func decodeAllowedTools(value any) ([]string, error) {
	switch typed := value.(type) {
	case nil:
		return nil, nil
	case string:
		normalized := strings.NewReplacer(",", " ", "\n", " ").Replace(typed)
		return strings.Fields(normalized), nil
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			text, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("expected string list item, got %T", item)
			}
			if text = strings.TrimSpace(text); text != "" {
				out = append(out, text)
			}
		}
		return out, nil
	default:
		var out []string
		if err := decodeAny(value, &out); err != nil {
			return nil, err
		}
		return out, nil
	}
}

func firstField(fields map[string]any, keys ...string) (any, bool) {
	for _, key := range keys {
		if value, ok := fields[key]; ok {
			return value, true
		}
	}
	return nil, false
}

func decodeAny(value any, target any) error {
	data, err := yaml.Marshal(value)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(data, target)
}

func decodeStringList(value any) ([]string, error) {
	switch typed := value.(type) {
	case nil:
		return nil, nil
	case string:
		parts := strings.FieldsFunc(typed, func(r rune) bool { return r == ',' || r == '\n' })
		out := make([]string, 0, len(parts))
		for _, part := range parts {
			if part = strings.TrimSpace(part); part != "" {
				out = append(out, part)
			}
		}
		return out, nil
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			text, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("expected string list item, got %T", item)
			}
			if text = strings.TrimSpace(text); text != "" {
				out = append(out, text)
			}
		}
		return out, nil
	default:
		var out []string
		if err := decodeAny(value, &out); err != nil {
			return nil, err
		}
		return out, nil
	}
}

func descriptionFromBody(body string) string {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimSpace(strings.TrimLeft(line, "#>*-+0123456789. "))
		if line == "" || strings.HasPrefix(line, "<!--") {
			continue
		}
		if len([]rune(line)) > 250 {
			return string([]rune(line)[:249]) + "…"
		}
		return line
	}
	return ""
}

func compileToolHints(names []string, warnings *[]string) []ToolHint {
	seen := map[string]bool{}
	out := make([]ToolHint, 0, len(names))
	for _, declared := range names {
		declared = strings.TrimSpace(declared)
		if declared == "" || seen[declared] {
			continue
		}
		seen[declared] = true
		hint := ToolHint{Declared: declared, Canonical: declared}
		if canonical, ok := commonToolAliases[strings.ToLower(declared)]; ok {
			hint.Canonical = canonical
			hint.Aliased = canonical != declared
			if hint.Aliased && warnings != nil {
				*warnings = append(*warnings, fmt.Sprintf("allowed-tools hint %q maps to %q", declared, canonical))
			}
		}
		out = append(out, hint)
	}
	return out
}

// CanonicalFrontmatterMap returns the normalized, round-trippable metadata
// view used by installers and diagnostics. Unknown fields are retained.
func (d *SkillDefinition) CanonicalFrontmatterMap() map[string]any {
	if d == nil {
		return nil
	}
	out := make(map[string]any, len(d.UnknownFields)+12)
	for key, value := range d.UnknownFields {
		out[key] = value
	}
	if d.Name != "" {
		out["name"] = d.Name
	}
	if d.DisplayName != "" {
		out["display_name"] = d.DisplayName
	}
	if d.Description != "" {
		out["description"] = d.Description
	}
	if d.WhenToUse != "" {
		out["when_to_use"] = d.WhenToUse
	}
	if d.Version != "" {
		out["version"] = d.Version
	}
	if len(d.AllowedTools) > 0 {
		out["allowed-tools"] = append([]string(nil), d.AllowedTools...)
	}
	if d.UserInvocableSet || !d.UserInvocable {
		out["user-invocable"] = d.UserInvocable
	}
	if d.DisableModelInvocation {
		out["disable-model-invocation"] = true
	}
	if d.Context != "" && d.Context != ContextInline {
		out["context"] = d.Context
	}
	if d.Agent != "" {
		out["agent"] = d.Agent
	}
	if d.Model != "" {
		out["model"] = d.Model
	}
	if d.Effort != "" {
		out["effort"] = d.Effort
	}
	if len(d.Paths) > 0 {
		out["paths"] = append([]string(nil), d.Paths...)
	}
	if d.Shell != "" {
		out["shell"] = d.Shell
	}
	return out
}

// ValidateNameReserved checks whether the given name starts with a reserved
// identity prefix. Local packages cannot impersonate MCP-owned or plugin-owned
// catalog IDs.
const reservedMcpPrefix = "mcp__"
const reservedPluginPrefix = "plugin__"

func ValidateNameReserved(name string) error {
	if strings.HasPrefix(name, reservedMcpPrefix) {
		return fmt.Errorf("skill name %q starts with reserved prefix %q", name, reservedMcpPrefix)
	}
	if strings.HasPrefix(name, reservedPluginPrefix) {
		return fmt.Errorf("skill name %q starts with reserved prefix %q", name, reservedPluginPrefix)
	}
	return nil
}

// ValidateSchema checks only the universal package contract. Runtime/tool
// declarations are intentionally not validated here.
func ValidateSchema(def *SkillDefinition) error {
	if def == nil {
		return fmt.Errorf("skill definition is nil")
	}
	if strings.TrimSpace(def.Name) == "" {
		return fmt.Errorf("skill name is required")
	}
	if strings.TrimSpace(def.Description) == "" {
		return fmt.Errorf("skill description is required (id=%q)", def.ID)
	}
	for i, arg := range def.Arguments {
		if strings.TrimSpace(arg.Name) == "" {
			return fmt.Errorf("skill %q argument[%d]: name is required", def.ID, i)
		}
		if !isValidArgType(arg.Type) {
			return fmt.Errorf("skill %q argument %q: invalid type %q (must be string, number, integer, boolean)", def.ID, arg.Name, arg.Type)
		}
	}
	if def.Context != "" && def.Context != ContextInline && def.Context != ContextFork {
		return fmt.Errorf("skill %q: invalid context mode %q (must be inline or fork)", def.ID, def.Context)
	}
	return nil
}

func isValidArgType(t string) bool {
	switch t {
	case "string", "number", "integer", "boolean":
		return true
	default:
		return false
	}
}

// splitFrontmatter extracts the YAML block between the first two --- lines.
func splitFrontmatter(raw string) (string, string, error) {
	trimmed := strings.TrimSpace(raw)
	if !strings.HasPrefix(trimmed, "---") {
		return "", "", fmt.Errorf("skill file must start with --- frontmatter delimiter")
	}
	rest := trimmed[3:]
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		idx = strings.Index(rest, "---")
		if idx < 0 {
			return "", "", fmt.Errorf("skill file missing closing --- frontmatter delimiter")
		}
	}
	fm := rest[:idx]
	bodyStart := idx + 4
	if bodyStart > len(rest) {
		bodyStart = idx + 3
	}
	return strings.TrimSpace(fm), rest[bodyStart:], nil
}
