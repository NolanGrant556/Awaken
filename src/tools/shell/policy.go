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

// ShellPolicyResult is the static policy summary for a command before it is
// executed. DirectAllow means the command can run through ShellExecutor.
// DenyReason is a hard deny and takes priority.
type ShellPolicyResult struct {
	Tier        ShellTier
	Lane        ShellLane
	DirectAllow bool
	DenyReason  string
}

// EvaluateShellPolicy evaluates a shell command for the zero-approval policy
// layer. Shell is a local-computer surface: it is not constrained to the
// project workspace. The only product-level hard gates are recoverable delete
// semantics and mass-message review.
func EvaluateShellPolicy(command string, cwd string, permissionContext tool.PermissionContext) ShellPolicyResult {
	return evaluateShellPolicy(command, cwd, permissionContext, false)
}

// EvaluateShellExecutionPolicy evaluates a shell command immediately before
// execution. It applies the same local-computer shell gates as the static
// policy path so direct execution cannot bypass delete/mass-message checks.
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
	if reason := hardDenyShellCommand(command, cwd, permissionContext); reason != "" {
		result.DenyReason = reason
		return result
	}
	result.Lane = ShellLaneLocalShell
	result.DirectAllow = true
	return result
}

func hardDenyShellCommand(command string, cwd string, permissionContext tool.PermissionContext) string {
	segments := splitCompoundSegments(command)
	for _, segment := range segments {
		tokens := shellTokens(segment)
		if len(tokens) == 0 {
			continue
		}
		base := filepath.Base(tokens[0])
		args := tokens[1:]
		if isDangerousRm(base, args) {
			return "destructive rm command is not allowed; file deletion must go through TrashAdapter"
		}
		if isMassMessageCLICommand(base, args) {
			return "mass message CLI command requires human review"
		}
	}
	return ""
}

func MassMessageReviewMetadataForShellCommand(command string) (*tool.MassMessageMetadata, bool) {
	for _, segment := range splitCompoundSegments(command) {
		tokens := shellTokens(segment)
		if len(tokens) == 0 {
			continue
		}
		base := filepath.Base(tokens[0])
		args := tokens[1:]
		if !isMassMessageCLICommand(base, args) {
			continue
		}
		return &tool.MassMessageMetadata{
			Channel:        strings.ToLower(base),
			RecipientScope: "cli command",
			BodyPreview:    truncateShellPreview(command, 240),
			SourceTool:     "shell",
		}, true
	}
	return nil, false
}

func isMassMessageCLICommand(base string, args []string) bool {
	base = strings.ToLower(base)
	if !isCollaborationCLI(base) {
		return false
	}
	lowerArgs := lowerShellArgs(args)
	if containsAnyShellToken(lowerArgs, "broadcast", "mass", "bulk", "群发") {
		return containsAnyShellToken(lowerArgs, "send", "message", "msg", "notify", "notification", "robot", "bot", "ding")
	}
	if containsShellSubsequence(lowerArgs, "message", "send") ||
		containsShellSubsequence(lowerArgs, "msg", "send") ||
		containsShellSubsequence(lowerArgs, "robot", "send") ||
		containsShellSubsequence(lowerArgs, "bot", "send") ||
		containsShellSubsequence(lowerArgs, "chat", "send") ||
		containsShellSubsequence(lowerArgs, "im", "+messages-send") ||
		containsShellSubsequence(lowerArgs, "ding", "send") ||
		containsShellSubsequence(lowerArgs, "notify", "send") {
		return true
	}
	return false
}

func isCollaborationCLI(base string) bool {
	switch base {
	case "dws", "feishu", "lark", "lark-cli", "ding", "dingtalk":
		return true
	}
	return false
}

func lowerShellArgs(args []string) []string {
	out := make([]string, len(args))
	for i, arg := range args {
		out[i] = strings.ToLower(arg)
	}
	return out
}

func containsShellSubsequence(args []string, first string, second string) bool {
	seenFirst := false
	for _, arg := range args {
		switch {
		case arg == first:
			seenFirst = true
		case seenFirst && arg == second:
			return true
		}
	}
	return false
}

func truncateShellPreview(value string, max int) string {
	if max <= 0 || len(value) <= max {
		return value
	}
	return value[:max] + "..."
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
