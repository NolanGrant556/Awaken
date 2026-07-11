package shelltool

import (
	"path/filepath"
	"strings"

	"awaken/src/internal/outputlimit"
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
// project workspace. Product-level hard gates cover recoverable delete
// semantics, mass-message review, and typed-runtime executables that must not
// be launched outside their supervised worker.
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
	if directlyLaunchesOfficeRuntime(command, 0) {
		return "direct soffice/libreoffice execution is disabled; use typed Office artifact tools backed by OfficeRenderWorker"
	}
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

const maxOfficeWrapperDepth = 6

// directlyLaunchesOfficeRuntime recognizes direct LibreOffice process starts,
// including the common shell wrappers that otherwise hide the executable in a
// later argument. Keeping this check in hardDenyShellCommand makes both the
// permission decision and the executor's last-moment policy check fail closed.
// Read-only references such as `command -v soffice`, `rg soffice`, and
// `echo soffice` intentionally remain available for diagnostics.
func directlyLaunchesOfficeRuntime(command string, depth int) bool {
	if depth > maxOfficeWrapperDepth {
		return true
	}
	for _, segment := range splitCompoundSegments(command) {
		if officeInvocationTokens(shellTokens(segment), depth) {
			return true
		}
	}
	return false
}

func officeInvocationTokens(tokens []string, depth int) bool {
	if depth > maxOfficeWrapperDepth {
		return true
	}
	if len(tokens) == 0 {
		return false
	}
	base := strings.ToLower(filepath.Base(strings.ReplaceAll(tokens[0], `\`, "/")))
	if isOfficeRuntimeExecutable(tokens[0]) {
		return true
	}

	switch base {
	case "env":
		return officeInvocationAfterEnv(tokens[1:], depth+1)
	case "command":
		return officeInvocationAfterCommand(tokens[1:], depth+1)
	case "xargs":
		return officeInvocationTokens(xargsCommand(tokens[1:]), depth+1)
	case "exec", "nohup":
		return officeInvocationTokens(argumentsAfterSimpleOptions(tokens[1:]), depth+1)
	case "sh", "bash", "zsh", "dash", "ksh":
		return officeInvocationInsideShell(tokens[1:], depth+1)
	}
	return false
}

func isOfficeRuntimeExecutable(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(strings.ReplaceAll(value, `\`, "/")))
	base := filepath.Base(normalized)
	base = strings.TrimSuffix(base, ".exe")
	base = strings.TrimSuffix(base, ".com")
	if base == "soffice" || base == "soffice.bin" || base == "libreoffice" {
		return true
	}
	// shellTokens follows POSIX backslash escaping even when it is inspecting a
	// Windows command string. A quoted `C:\Program Files\LibreOffice\...` may
	// therefore arrive without separators; retain a bounded fallback for that
	// platform spelling without treating arbitrary names containing "soffice"
	// as the real executable.
	return (strings.Contains(normalized, "libreoffice") || strings.Contains(normalized, ":")) &&
		(strings.HasSuffix(normalized, "soffice") ||
			strings.HasSuffix(normalized, "soffice.bin") ||
			strings.HasSuffix(normalized, "soffice.exe") ||
			strings.HasSuffix(normalized, "soffice.com") ||
			strings.HasSuffix(normalized, "soffice.bin.exe") ||
			strings.HasSuffix(normalized, "libreoffice.exe"))
}

func officeInvocationAfterEnv(args []string, depth int) bool {
	for index := 0; index < len(args); {
		arg := args[index]
		switch {
		case arg == "--":
			return officeInvocationTokens(args[index+1:], depth)
		case arg == "-S" || arg == "--split-string":
			if index+1 >= len(args) {
				return false
			}
			return directlyLaunchesOfficeRuntime(args[index+1], depth)
		case strings.HasPrefix(arg, "--split-string="):
			return directlyLaunchesOfficeRuntime(strings.TrimPrefix(arg, "--split-string="), depth)
		case arg == "-u" || arg == "--unset" || arg == "-C" || arg == "--chdir":
			index += 2
		case strings.HasPrefix(arg, "--unset=") || strings.HasPrefix(arg, "--chdir="):
			index++
		case strings.HasPrefix(arg, "-"):
			index++
		case isEnvironmentAssignment(arg):
			index++
		default:
			return officeInvocationTokens(args[index:], depth)
		}
	}
	return false
}

func isEnvironmentAssignment(value string) bool {
	name, _, ok := strings.Cut(value, "=")
	if !ok || name == "" {
		return false
	}
	for index, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_' || (index > 0 && r >= '0' && r <= '9') {
			continue
		}
		return false
	}
	return true
}

func officeInvocationAfterCommand(args []string, depth int) bool {
	for index, arg := range args {
		switch arg {
		case "-v", "-V":
			return false
		case "--":
			return officeInvocationTokens(args[index+1:], depth)
		case "-p":
			continue
		default:
			if strings.HasPrefix(arg, "-") {
				continue
			}
			return officeInvocationTokens(args[index:], depth)
		}
	}
	return false
}

func xargsCommand(args []string) []string {
	for index := 0; index < len(args); {
		arg := args[index]
		if arg == "--" {
			return args[index+1:]
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			return args[index:]
		}
		if strings.Contains(arg, "=") || xargsOptionHasInlineValue(arg) {
			index++
			continue
		}
		if xargsOptionTakesNextValue(arg) {
			index += 2
			continue
		}
		index++
	}
	return nil
}

func xargsOptionTakesNextValue(option string) bool {
	switch option {
	case "-a", "-E", "-I", "-J", "-L", "-n", "-P", "-R", "-S", "-s",
		"--arg-file", "--delimiter", "--eof", "--max-args", "--max-chars",
		"--max-lines", "--max-procs", "--process-slot-var", "--replace":
		return true
	}
	return false
}

func xargsOptionHasInlineValue(option string) bool {
	if len(option) <= 2 || !strings.HasPrefix(option, "-") || strings.HasPrefix(option, "--") {
		return false
	}
	switch option[1] {
	case 'a', 'E', 'I', 'J', 'L', 'n', 'P', 'R', 'S', 's':
		return true
	}
	return false
}

func argumentsAfterSimpleOptions(args []string) []string {
	for index, arg := range args {
		if arg == "--" {
			return args[index+1:]
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			return args[index:]
		}
	}
	return nil
}

func officeInvocationInsideShell(args []string, depth int) bool {
	for index, arg := range args {
		if arg == "-c" || (strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") && strings.Contains(arg[1:], "c")) {
			if index+1 >= len(args) {
				return false
			}
			return directlyLaunchesOfficeRuntime(args[index+1], depth)
		}
	}
	return false
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
	if max <= 0 {
		return value
	}
	prefix, truncated := outputlimit.Prefix(value, max)
	if !truncated {
		return prefix
	}
	return prefix + "..."
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
