package shelltool

import (
	"path/filepath"
	"strings"

	"awaken/src/tool"
)

// ShellLane is the product-level observation label for a shell command.
// Shell now executes as a local-computer surface; lane no longer routes to
// separate restricted executors.
type ShellLane string

const (
	ShellLaneInspectReadOnly ShellLane = "inspect_read_only"
	ShellLaneLocalShell      ShellLane = "local_shell"
	ShellLaneUnknown         ShellLane = "unknown"
)

// ShellPolicyResult is the static observation summary for a command before it
// is executed. The default local-user Shell does not impose Awaken-specific
// path, network, package-manager, delete, or collaboration-CLI gates.
type ShellPolicyResult struct {
	Tier        ShellTier
	Lane        ShellLane
	DirectAllow bool
	DenyReason  string
}

// EvaluateShellPolicy classifies a command for observation. Shell is a
// local-computer surface and runs with the current user's host permissions.
func EvaluateShellPolicy(command string, cwd string, permissionContext tool.PermissionContext) ShellPolicyResult {
	return evaluateShellPolicy(command, cwd, permissionContext, false)
}

// EvaluateShellExecutionPolicy evaluates a shell command immediately before
// execution. It intentionally keeps the same current-user allow contract as
// the static policy path; optional broker restrictions are evaluated later.
func EvaluateShellExecutionPolicy(command string, cwd string, permissionContext tool.PermissionContext) ShellPolicyResult {
	return evaluateShellPolicy(command, cwd, permissionContext, true)
}

func evaluateShellPolicy(command string, cwd string, permissionContext tool.PermissionContext, executionMode bool) ShellPolicyResult {
	command = strings.TrimSpace(command)
	permissionContext = permissionContext.Normalize()
	if strings.TrimSpace(cwd) == "" {
		cwd = permissionContext.WorkspaceRootOrCWD()
	}

	result := ShellPolicyResult{
		Tier: ClassifyShellCommand(command, cwd),
		Lane: ShellLaneUnknown,
	}
	if command == "" {
		result.Lane = ShellLaneInspectReadOnly
		result.DirectAllow = true
		return result
	}
	result.Lane = ShellLaneLocalShell
	result.DirectAllow = true
	return result
}

func classifyExplicitDeleteTargets(command string, cwd string, permissionContext tool.PermissionContext) []string {
	segments := splitCompoundSegments(command)
	var deleteTargets []string
	deleteSegments := 0
	nonDeleteSegments := 0
	for _, segment := range segments {
		tokens := shellTokens(segment)
		if len(tokens) == 0 {
			continue
		}
		base := filepath.Base(tokens[0])
		args := tokens[1:]
		if isDangerousRm(base, args) {
			deleteSegments++
			targets, ok := directDeleteTargets(base, args, cwd, permissionContext)
			if !ok || len(targets) == 0 {
				return nil
			}
			deleteTargets = append(deleteTargets, targets...)
			continue
		}
		nonDeleteSegments++
	}
	if deleteSegments > 0 {
		if nonDeleteSegments > 0 {
			return nil
		}
		return normalizeShellTargets(deleteTargets)
	}
	return nil
}

func directDeleteTargets(base string, args []string, cwd string, permissionContext tool.PermissionContext) ([]string, bool) {
	base = strings.ToLower(filepath.Base(base))
	var rawTargets []string
	for index := 0; index < len(args); index++ {
		arg := strings.TrimSpace(args[index])
		if arg == "" {
			continue
		}
		if arg == "--" {
			rawTargets = append(rawTargets, args[index+1:]...)
			break
		}
		if base == "remove-item" || base == "ri" {
			switch strings.ToLower(arg) {
			case "-path", "-literalpath":
				if index+1 >= len(args) {
					return nil, false
				}
				index++
				rawTargets = append(rawTargets, args[index])
				continue
			}
		}
		if strings.HasPrefix(arg, "-") || ((base == "del" || base == "erase" || base == "rd") && strings.HasPrefix(arg, "/")) {
			continue
		}
		rawTargets = append(rawTargets, arg)
	}
	if len(rawTargets) == 0 {
		return nil, false
	}
	resolved := make([]string, 0, len(rawTargets))
	for _, target := range rawTargets {
		target = strings.Trim(strings.TrimSpace(target), `"'`)
		if target == "" || strings.ContainsAny(target, "*?[]{}$`\x00") || strings.HasPrefix(target, "~") && target != "~" && !strings.HasPrefix(target, "~/") && !strings.HasPrefix(target, `~\`) {
			return nil, false
		}
		if filepath.IsAbs(target) || strings.HasPrefix(target, "~/") || strings.HasPrefix(target, `~\`) || target == "~" {
			target = permissionContext.ResolvePath(target)
		} else {
			target = filepath.Join(cwd, target)
		}
		absolute, err := filepath.Abs(target)
		if err != nil {
			return nil, false
		}
		resolved = append(resolved, filepath.Clean(absolute))
	}
	return normalizeShellTargets(resolved), true
}

func normalizeShellTargets(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = filepath.Clean(strings.TrimSpace(value))
		if value == "" || value == "." {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

// ClassifyShellBrokerOperation returns the strongest semantics the restricted
// broker can prove from the command. Unknown interpreters, command substitution,
// scripts, writes, network access and package managers are deliberately not
// treated as read-only.
func ClassifyShellBrokerOperation(command string, cwd string, permissionContext tool.PermissionContext) (tool.BrokerOperation, []string) {
	policy := EvaluateShellPolicy(command, cwd, permissionContext)
	// Default policy allows raw host execution, but an explicitly injected
	// restricted broker still receives a precise delete operation when the
	// command is statically convertible. The default LocalUserExecutionBroker
	// authorizes it without claiming a Trash transform.
	if deleteTargets := classifyExplicitDeleteTargets(command, cwd, permissionContext); len(deleteTargets) > 0 {
		return tool.BrokerOperationDelete, append([]string(nil), deleteTargets...)
	}
	if policy.DenyReason != "" {
		return tool.BrokerOperationUnknown, nil
	}
	if hasUnboundedShellEvaluation(command) {
		return tool.BrokerOperationUnknown, nil
	}
	switch policy.Tier {
	case ShellTierInspectReadOnly:
		return tool.BrokerOperationInspect, nil
	case ShellTierWorkspaceMutation:
		return tool.BrokerOperationMutation, nil
	case ShellTierOpenWorldSystem:
		return tool.BrokerOperationNetwork, nil
	default:
		return tool.BrokerOperationUnknown, nil
	}
}

func hasUnboundedShellEvaluation(command string) bool {
	return strings.Contains(command, "$(") || strings.Contains(command, "`") ||
		strings.Contains(command, "<(") || strings.Contains(command, ">(") ||
		strings.Contains(command, "<<") || strings.Contains(command, "${")
}

func shellTokens(segment string) []string {
	var tokens []string
	var current strings.Builder
	inSingle := false
	inDouble := false
	escaped := false
	flush := func() {
		if current.Len() == 0 {
			return
		}
		tokens = append(tokens, current.String())
		current.Reset()
	}
	for _, r := range segment {
		switch {
		case escaped:
			current.WriteRune(r)
			escaped = false
		case r == '\\' && !inSingle:
			escaped = true
		case r == '\'' && !inDouble:
			inSingle = !inSingle
		case r == '"' && !inSingle:
			inDouble = !inDouble
		case !inSingle && !inDouble && (r == ' ' || r == '\t' || r == '\n' || r == '\r'):
			flush()
		default:
			current.WriteRune(r)
		}
	}
	flush()
	return tokens
}

func isDangerousRm(base string, args []string) bool {
	switch strings.ToLower(base) {
	case "rm", "rmdir", "unlink", "del", "erase", "remove-item", "ri", "rd":
	default:
		return false
	}
	return true
}
