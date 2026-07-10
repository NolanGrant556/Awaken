package skillruntime

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"awaken/src/skills"
	"awaken/src/tool"
)

// CompileEffectiveTools intersects the parent pool with each non-empty allow
// list and subtracts all deny lists. Empty allow inherits the parent pool.
func CompileEffectiveTools(parentPool []string, policies []skills.ToolPolicy) ([]string, []SkillCompatibilityError) {
	parent := normalizeToolNames(parentPool)
	parentSet := make(map[string]bool, len(parent))
	for _, name := range parent {
		parentSet[name] = true
	}

	effective := make(map[string]bool, len(parent))
	for _, name := range parent {
		effective[name] = true
	}

	var errs []SkillCompatibilityError
	for _, policy := range policies {
		allow := normalizeToolNames(policy.Allow)
		if len(allow) > 0 {
			next := make(map[string]bool, len(allow))
			for _, name := range allow {
				if !parentSet[name] {
					errs = append(errs, SkillCompatibilityError{
						Code:    "skill_tool_outside_parent_pool",
						Message: fmt.Sprintf("skill allow list includes %q which is outside the parent tool pool", name),
					})
					continue
				}
				if effective[name] {
					next[name] = true
				}
			}
			effective = next
		}
		for _, name := range normalizeToolNames(policy.Deny) {
			delete(effective, name)
		}
	}

	out := make([]string, 0, len(effective))
	for name := range effective {
		out = append(out, name)
	}
	sort.Strings(out)
	return out, errs
}

// ValidateRequiredTools ensures every required tool exists in effectiveTools.
func ValidateRequiredTools(effectiveTools, requiredTools []string) []SkillCompatibilityError {
	set := make(map[string]bool, len(effectiveTools))
	for _, name := range effectiveTools {
		set[name] = true
	}
	var errs []SkillCompatibilityError
	for _, name := range normalizeToolNames(requiredTools) {
		if !set[name] {
			errs = append(errs, SkillCompatibilityError{
				Code:    "skill_required_tool_missing",
				Message: fmt.Sprintf("required tool %q is not in the effective tool pool", name),
			})
		}
	}
	return errs
}

// ValidateRequiredCapabilities ensures each capability is satisfied by ≥1
// tool in the effective pool.
func ValidateRequiredCapabilities(
	effectiveTools []string,
	required []tool.CapabilityClass,
	lookup func(toolName string) (tool.CapabilityClass, bool),
) []SkillCompatibilityError {
	if lookup == nil {
		lookup = defaultToolCapability
	}
	present := make(map[tool.CapabilityClass]bool)
	for _, name := range effectiveTools {
		if capClass, ok := lookup(name); ok && capClass != "" {
			present[capClass] = true
		}
	}
	var errs []SkillCompatibilityError
	for _, capClass := range required {
		if strings.TrimSpace(string(capClass)) == "" {
			continue
		}
		if !present[capClass] {
			errs = append(errs, SkillCompatibilityError{
				Code:    "skill_required_capability_missing",
				Message: fmt.Sprintf("required capability %q is not satisfied by the effective tool pool", capClass),
			})
		}
	}
	return errs
}

// ValidateRequiredRuntimeComponents checks packaged runtime availability.
func ValidateRequiredRuntimeComponents(required []string, available func(component string) bool) []SkillCompatibilityError {
	if available == nil {
		available = func(string) bool { return true }
	}
	var errs []SkillCompatibilityError
	for _, name := range normalizeToolNames(required) {
		if !available(name) {
			errs = append(errs, SkillCompatibilityError{
				Code:    "skill_required_runtime_missing",
				Message: fmt.Sprintf("required runtime component %q is unavailable", name),
			})
		}
	}
	return errs
}

// IntersectToolPools returns the intersection of two tool name lists.
func IntersectToolPools(a, b []string) []string {
	setB := make(map[string]bool, len(b))
	for _, name := range normalizeToolNames(b) {
		setB[name] = true
	}
	out := make([]string, 0)
	for _, name := range normalizeToolNames(a) {
		if setB[name] {
			out = append(out, name)
		}
	}
	return out
}

// NarrowToolPool ensures child is a subset of parent. Names outside parent
// are dropped.
func NarrowToolPool(parent, child []string) []string {
	if child == nil {
		return append([]string(nil), normalizeToolNames(parent)...)
	}
	return IntersectToolPools(parent, child)
}

// MatchToolPattern reports whether toolName matches an allow/deny pattern.
// Supports exact names and trailing "*" globs (e.g. mcp__server__*).
func MatchToolPattern(pattern, toolName string) bool {
	pattern = strings.TrimSpace(pattern)
	toolName = strings.TrimSpace(toolName)
	if pattern == "" || toolName == "" {
		return false
	}
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(toolName, prefix)
	}
	return pattern == toolName || path.Base(pattern) == toolName
}
