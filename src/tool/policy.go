package tool

import (
	"sort"
	"strings"
)

type PermissionMode string

const (
	PermissionModeAsk   PermissionMode = "ask"
	PermissionModeAllow PermissionMode = "allow"
	PermissionModeDeny  PermissionMode = "deny"
)

type PermissionPolicy struct {
	Mode       PermissionMode `json:"mode,omitempty"`
	AllowTools []string       `json:"allow_tools,omitempty"`
	DenyTools  []string       `json:"deny_tools,omitempty"`
}

func DefaultPermissionPolicy() PermissionPolicy {
	return PermissionPolicy{Mode: PermissionModeAsk}
}

func (p PermissionPolicy) Normalize() PermissionPolicy {
	mode := p.Mode
	switch mode {
	case PermissionModeAllow, PermissionModeDeny:
	case PermissionModeAsk, "":
		mode = PermissionModeAsk
	default:
		mode = PermissionModeAsk
	}

	return PermissionPolicy{
		Mode:       mode,
		AllowTools: normalizeToolNames(p.AllowTools),
		DenyTools:  normalizeToolNames(p.DenyTools),
	}
}

func (p PermissionPolicy) AllowsTool(name string) bool {
	return containsToolName(p.Normalize().AllowTools, name)
}

func (p PermissionPolicy) DeniesTool(name string) bool {
	return containsToolName(p.Normalize().DenyTools, name)
}

func ApplyPermissionPolicy(
	base PermissionDecision,
	policy PermissionPolicy,
	toolName string,
	readOnly bool,
) PermissionDecision {
	if base.Behavior == PermissionDeny {
		return base
	}

	normalized := policy.Normalize()
	if normalized.DeniesTool(toolName) {
		return DenyPermission("permission policy denied execution")
	}
	if normalized.AllowsTool(toolName) {
		return AllowPermission()
	}

	switch normalized.Mode {
	case PermissionModeAllow:
		return AllowPermission()
	case PermissionModeDeny:
		if readOnly {
			return base
		}
		return DenyPermission("permission policy denied execution")
	default:
		return base
	}
}

func normalizeToolNames(names []string) []string {
	if len(names) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(names))
	normalized := make([]string, 0, len(names))
	for _, candidate := range names {
		name := normalizeToolName(candidate)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		normalized = append(normalized, name)
	}
	sort.Strings(normalized)
	return normalized
}

func containsToolName(names []string, name string) bool {
	target := normalizeToolName(name)
	if target == "" {
		return false
	}
	for _, candidate := range names {
		if candidate == target {
			return true
		}
	}
	return false
}

func normalizeToolName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
