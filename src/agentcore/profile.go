package agentcore

import (
	"fmt"
	"regexp"
	"strings"

	"awaken/src/tool"
)

type PermissionMode string

const (
	PermissionModeDefault PermissionMode = ""
	PermissionModeAsk     PermissionMode = "ask"
	PermissionModeAllow   PermissionMode = "allow"
	PermissionModeDeny    PermissionMode = "deny"
)

type AgentDefinition struct {
	ID              string         `json:"id,omitempty"`
	Name            string         `json:"name,omitempty"`
	Description     string         `json:"description,omitempty"`
	SystemPrompt    string         `json:"system_prompt,omitempty"`
	Model           string         `json:"model,omitempty"`
	MaxTurns        int            `json:"max_turns,omitempty"`
	Tools           []string       `json:"tools,omitempty"`
	DisallowedTools []string       `json:"disallowed_tools,omitempty"`
	PermissionMode  PermissionMode `json:"permission_mode,omitempty"`
}

type RunProfile struct {
	ID              string         `json:"id,omitempty"`
	Name            string         `json:"name,omitempty"`
	Description     string         `json:"description,omitempty"`
	SystemPrompt    string         `json:"system_prompt,omitempty"`
	Model           string         `json:"model,omitempty"`
	MaxTurns        int            `json:"max_turns,omitempty"`
	Tools           []string       `json:"tools,omitempty"`
	DisallowedTools []string       `json:"disallowed_tools,omitempty"`
	PermissionMode  PermissionMode `json:"permission_mode,omitempty"`
}

type ToolAccessPolicy struct {
	hasAllowlist bool
	allowAll     bool
	allowed      map[string]struct{}
	disallowed   map[string]struct{}
}

var runProfileToolNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

func (p RunProfile) Normalize() RunProfile {
	return RunProfile{
		ID:              strings.TrimSpace(p.ID),
		Name:            strings.TrimSpace(p.Name),
		Description:     strings.TrimSpace(p.Description),
		SystemPrompt:    strings.TrimSpace(p.SystemPrompt),
		Model:           strings.TrimSpace(p.Model),
		MaxTurns:        p.MaxTurns,
		Tools:           normalizeToolNames(p.Tools),
		DisallowedTools: normalizeToolNames(p.DisallowedTools),
		PermissionMode:  PermissionMode(strings.ToLower(strings.TrimSpace(string(p.PermissionMode)))),
	}
}

func (p RunProfile) Validate() error {
	normalized := p.Normalize()
	if normalized.MaxTurns < 0 {
		return fmt.Errorf("max_turns must be greater than or equal to zero")
	}
	if !normalized.PermissionMode.IsValid() {
		return fmt.Errorf("invalid permission_mode %q", normalized.PermissionMode)
	}
	for _, name := range append([]string{}, normalized.Tools...) {
		if name == "*" {
			continue
		}
		if !toolNamePatternValid(name) {
			return fmt.Errorf("invalid tool name %q", name)
		}
	}
	for _, name := range normalized.DisallowedTools {
		if !toolNamePatternValid(name) {
			return fmt.Errorf("invalid disallowed tool name %q", name)
		}
	}
	return nil
}

func (p RunProfile) ToolAccessPolicy() ToolAccessPolicy {
	normalized := p.Normalize()
	policy := ToolAccessPolicy{
		allowed:    make(map[string]struct{}, len(normalized.Tools)),
		disallowed: make(map[string]struct{}, len(normalized.DisallowedTools)),
	}
	if normalized.Tools != nil {
		policy.hasAllowlist = true
	}
	if len(normalized.Tools) == 1 && normalized.Tools[0] == "*" {
		policy.allowAll = true
	}
	for _, name := range normalized.Tools {
		if name == "*" {
			continue
		}
		policy.allowed[name] = struct{}{}
	}
	for _, name := range normalized.DisallowedTools {
		policy.disallowed[name] = struct{}{}
	}
	return policy
}

func (p ToolAccessPolicy) Allows(name string) bool {
	if strings.TrimSpace(name) == "" {
		return false
	}
	if _, blocked := p.disallowed[name]; blocked {
		return false
	}
	if !p.hasAllowlist || p.allowAll {
		return true
	}
	_, allowed := p.allowed[name]
	return allowed
}

func (p ToolAccessPolicy) FilterDefinitions(definitions []tool.ToolDefinition) []tool.ToolDefinition {
	if len(definitions) == 0 {
		return definitions
	}
	filtered := make([]tool.ToolDefinition, 0, len(definitions))
	for _, definition := range definitions {
		if p.Allows(definition.Function.Name) {
			filtered = append(filtered, definition)
		}
	}
	return filtered
}

func (c AgentConfig) WithRunProfile(profile *RunProfile) (AgentConfig, error) {
	if profile == nil {
		return c, nil
	}
	normalized := profile.Normalize()
	if err := normalized.Validate(); err != nil {
		return AgentConfig{}, err
	}

	cfg := c
	cfg.RunProfile = normalized
	if normalized.SystemPrompt != "" {
		cfg.SystemPrompt = normalized.SystemPrompt
	}
	if normalized.Model != "" {
		cfg.ModelConfig.Model = normalized.Model
	}
	if normalized.MaxTurns > 0 {
		cfg.MaxThoughts = normalized.MaxTurns
	}
	return cfg, nil
}

func (c AgentConfig) ToolAccessPolicy() ToolAccessPolicy {
	return c.RunProfile.ToolAccessPolicy()
}

func (c AgentConfig) PermissionMode() PermissionMode {
	return c.RunProfile.Normalize().PermissionMode
}

func normalizeToolNames(values []string) []string {
	if values == nil {
		return nil
	}
	normalized := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		name := strings.TrimSpace(value)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		normalized = append(normalized, name)
	}
	if len(normalized) == 0 {
		return []string{}
	}
	return normalized
}

func toolNamePatternValid(name string) bool {
	return runProfileToolNamePattern.MatchString(name)
}

func (m PermissionMode) IsValid() bool {
	switch m {
	case PermissionModeDefault, PermissionModeAsk, PermissionModeAllow, PermissionModeDeny:
		return true
	default:
		return false
	}
}
