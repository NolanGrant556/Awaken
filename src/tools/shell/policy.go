package shelltool

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"awaken/src/tool"
)

// ShellLane is the product-level execution lane a shell command belongs to.
// It is finer than ShellTier: the tier captures blast radius, while the lane
// tells the policy/runtime which safe executor path should own the command.
type ShellLane string

const (
	ShellLaneInspectReadOnly    ShellLane = "inspect_read_only"
	ShellLaneLocalCLI           ShellLane = "local_cli"
	ShellLaneWorkspaceFS        ShellLane = "workspace_fs"
	ShellLaneTestBuildCodegen   ShellLane = "test_build_codegen"
	ShellLaneDependencyNetwork  ShellLane = "dependency_network"
	ShellLaneOpenWorldHighTrust ShellLane = "open_world_high_trust"
	ShellLaneUnknown            ShellLane = "unknown"
)

// ShellPolicyResult is the static policy summary for a command before it is
// executed. DirectAllow means the command can run through ShellExecutor under
// the current TaskScope/profile. DenyReason is a hard deny and takes priority.
type ShellPolicyResult struct {
	Tier        ShellTier
	Lane        ShellLane
	DirectAllow bool
	DenyReason  string
}

// EvaluateShellPolicy evaluates a shell command for the zero-approval policy
// layer. Transformable workspace writes are not denied here; the AUTO policy
// gets a chance to rewrite them to apply_patch before denying.
func EvaluateShellPolicy(command string, cwd string, permissionContext tool.PermissionContext) ShellPolicyResult {
	return evaluateShellPolicy(command, cwd, permissionContext, false)
}

// EvaluateShellExecutionPolicy evaluates a shell command immediately before
// execution. At this point transform has already had its chance, so any
// workspace mutation that is not a direct-allow lane is denied fail-closed.
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
	if !pathWithinAnyTaskRoot(cwd, permissionContext.ReadableRoots) {
		result.DenyReason = fmt.Sprintf("shell workdir %q is outside readable roots", cwd)
		return result
	}
	if reason := hardDenyShellCommand(command, cwd, permissionContext); reason != "" {
		result.DenyReason = reason
		return result
	}
	if ok, reason := isSafeWorkspaceFSCommand(command, cwd, permissionContext); ok {
		result.Lane = ShellLaneWorkspaceFS
		result.DirectAllow = true
		return result
	} else if reason != "" {
		result.Lane = ShellLaneWorkspaceFS
		result.DenyReason = reason
		return result
	}
	if isSafeTestBuildCommand(command) {
		result.Lane = ShellLaneTestBuildCodegen
		result.DirectAllow = true
		return result
	}
	if isDependencyNetworkCommand(command) {
		result.Lane = ShellLaneDependencyNetwork
		if !executionMode {
			result.DenyReason = "dependency or network shell command requires a project profile or typed dependency/network executor; use apply_patch for file writes"
		}
		return result
	}

	switch result.Tier {
	case ShellTierInspectReadOnly:
		result.Lane = ShellLaneInspectReadOnly
		result.DirectAllow = true
	case ShellTierLocalCLI:
		result.Lane = ShellLaneLocalCLI
		result.DirectAllow = true
	case ShellTierOpenWorldSystem:
		result.Lane = ShellLaneOpenWorldHighTrust
		if !executionMode {
			result.DenyReason = "open-world shell command is not allowed in zero-approval shell; use a typed network/dependency tool or apply_patch for file writes"
		}
	case ShellTierWorkspaceMutation:
		result.Lane = ShellLaneWorkspaceFS
		if executionMode {
			result.DenyReason = "shell workspace mutation must be transformed to a typed tool or match an allowed shell lane"
		}
	}
	return result
}

func hardDenyShellCommand(command string, cwd string, permissionContext tool.PermissionContext) string {
	segments := splitCompoundSegments(command)
	for index, segment := range segments {
		tokens := shellTokens(segment)
		if len(tokens) == 0 {
			continue
		}
		base := filepath.Base(tokens[0])
		args := tokens[1:]
		switch base {
		case "sudo", "doas", "pkexec":
			return "privileged shell wrappers are not allowed"
		case "bash", "sh", "zsh", "fish", "xargs", "eval":
			return "shell wrapper or eval commands are not allowed in zero-approval shell"
		}
		if index > 0 && isExecutorCommand(base) {
			return "piping or chaining data into an executor is not allowed"
		}
		if isDangerousRm(base, args) {
			return "destructive rm command is not allowed; file deletion must go through TrashAdapter"
		}
		if isDangerousChmod(base, args) {
			return "destructive chmod/chown command is not allowed"
		}
		if isMassMessageCLICommand(base, args) {
			return "mass message CLI command requires human review"
		}
		if reason := denySensitiveOrOutOfScopeArgs(base, args, cwd, permissionContext); reason != "" {
			return reason
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

func isExecutorCommand(base string) bool {
	switch base {
	case "sh", "bash", "zsh", "fish", "python", "python3", "node", "ruby", "perl", "php", "osascript":
		return true
	}
	return false
}

func isDangerousRm(base string, args []string) bool {
	if base != "rm" && base != "rmdir" {
		return false
	}
	return true
}

func isDangerousChmod(base string, args []string) bool {
	if base != "chmod" && base != "chown" && base != "chgrp" {
		return false
	}
	for _, arg := range args {
		if arg == "-R" || arg == "--recursive" || strings.HasPrefix(arg, "/") {
			return true
		}
	}
	return false
}

func denySensitiveOrOutOfScopeArgs(base string, args []string, cwd string, permissionContext tool.PermissionContext) string {
	for index, arg := range args {
		if !looksLikePathArg(arg) {
			continue
		}
		path := cleanShellPathArg(arg)
		if path == "" {
			continue
		}
		if isNullDevicePath(path) && isNullDeviceRedirectionArg(arg, previousShellArg(args, index)) {
			continue
		}
		if strings.HasPrefix(path, "~") {
			return fmt.Sprintf("shell command touches home path %q, which is outside the task scope", path)
		}
		resolved := resolveShellArgPath(path, cwd)
		if err := permissionContext.CheckSensitivePath(resolved); err != nil {
			return err.Error()
		}
		if filepath.IsAbs(resolved) && !pathWithinAnyTaskRoot(resolved, permissionContext.ReadableRoots) {
			return fmt.Sprintf("shell command path %q is outside readable roots", path)
		}
	}
	return ""
}

func looksLikePathArg(arg string) bool {
	arg = cleanShellPathArg(arg)
	if arg == "" || strings.HasPrefix(arg, "-") || strings.Contains(arg, "=") {
		return false
	}
	if _, err := url.ParseRequestURI(arg); err == nil && strings.Contains(arg, "://") {
		return false
	}
	return strings.HasPrefix(arg, ".") ||
		strings.HasPrefix(arg, "/") ||
		strings.HasPrefix(arg, "~") ||
		strings.Contains(arg, "/") ||
		strings.HasPrefix(filepath.Base(arg), ".env") ||
		filepath.Ext(arg) != ""
}

func cleanShellPathArg(arg string) string {
	arg = strings.TrimSpace(arg)
	arg = strings.Trim(arg, `"'`)
	arg = strings.TrimSpace(stripInlineRedirectionPrefix(arg))
	arg = strings.Trim(arg, `"'`)
	return arg
}

func stripInlineRedirectionPrefix(arg string) string {
	i := 0
	for i < len(arg) && arg[i] >= '0' && arg[i] <= '9' {
		i++
	}
	if i >= len(arg) {
		return arg
	}
	switch {
	case strings.HasPrefix(arg[i:], ">>"):
		return strings.TrimLeft(arg[i+2:], " \t")
	case strings.HasPrefix(arg[i:], ">"):
		return strings.TrimLeft(arg[i+1:], " \t")
	case strings.HasPrefix(arg[i:], "<"):
		return strings.TrimLeft(arg[i+1:], " \t")
	default:
		return arg
	}
}

func previousShellArg(args []string, index int) string {
	if index <= 0 || index > len(args)-1 {
		return ""
	}
	return args[index-1]
}

func isNullDeviceRedirectionArg(raw string, previous string) bool {
	if isInlineRedirectionArg(raw) {
		return true
	}
	return isStandaloneRedirectionToken(previous)
}

func isInlineRedirectionArg(arg string) bool {
	arg = strings.TrimSpace(arg)
	i := 0
	for i < len(arg) && arg[i] >= '0' && arg[i] <= '9' {
		i++
	}
	return i < len(arg) && (strings.HasPrefix(arg[i:], ">") || strings.HasPrefix(arg[i:], "<"))
}

func isStandaloneRedirectionToken(arg string) bool {
	switch strings.TrimSpace(arg) {
	case ">", ">>", "<":
		return true
	default:
		return false
	}
}

func isSafeWorkspaceFSCommand(command string, cwd string, permissionContext tool.PermissionContext) (bool, string) {
	if hasShellCompoundOperator(command) || strings.Contains(command, ">") || strings.Contains(command, "<") {
		return false, ""
	}
	tokens := shellTokens(command)
	if len(tokens) == 0 {
		return false, ""
	}
	base := filepath.Base(tokens[0])
	args := tokens[1:]
	switch base {
	case "mkdir":
		paths := nonFlagArgs(args, "-p", "--parents")
		if len(paths) == 0 {
			return false, "mkdir requires at least one workspace path"
		}
		return validateWorkspaceMutationArgs(paths, cwd, permissionContext)
	case "touch":
		paths := nonFlagArgs(args, "-c")
		if len(paths) == 0 {
			return false, "touch requires at least one workspace path"
		}
		return validateWorkspaceMutationArgs(paths, cwd, permissionContext)
	case "cp":
		if containsUnsupportedFlag(args, "-R", "-r", "--recursive", "-a") {
			return false, "recursive cp is not allowed through shell"
		}
		paths := nonFlagArgs(args)
		if len(paths) != 2 {
			return false, "cp through shell only supports one source and one destination"
		}
		if reason := validateReadableArg(paths[0], cwd, permissionContext); reason != "" {
			return false, reason
		}
		if ok, reason := validateWorkspaceMutationArgs([]string{paths[1]}, cwd, permissionContext); !ok {
			return false, reason
		}
		if _, err := os.Stat(resolveShellArgPath(paths[1], cwd)); err == nil {
			return false, "cp destination already exists; use apply_patch or a typed file tool to avoid overwriting"
		}
		return true, ""
	case "mv":
		paths := nonFlagArgs(args)
		if len(paths) != 2 {
			return false, "mv through shell only supports one source and one destination"
		}
		if reason := validateReadableArg(paths[0], cwd, permissionContext); reason != "" {
			return false, reason
		}
		if ok, reason := validateWorkspaceMutationArgs([]string{paths[1]}, cwd, permissionContext); !ok {
			return false, reason
		}
		if _, err := os.Stat(resolveShellArgPath(paths[1], cwd)); err == nil {
			return false, "mv destination already exists; use apply_patch or a typed file tool to avoid overwriting"
		}
		return true, ""
	case "rm", "rmdir":
		return false, "shell delete is not allowed; deletion must go through TrashAdapter"
	}
	return false, ""
}

func nonFlagArgs(args []string, allowedFlags ...string) []string {
	allowed := map[string]bool{}
	for _, flag := range allowedFlags {
		allowed[flag] = true
	}
	var out []string
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			if allowed[arg] {
				continue
			}
			out = append(out, arg)
			continue
		}
		out = append(out, arg)
	}
	return out
}

func containsUnsupportedFlag(args []string, flags ...string) bool {
	for _, arg := range args {
		for _, flag := range flags {
			if arg == flag {
				return true
			}
		}
	}
	return false
}

func validateWorkspaceMutationArgs(paths []string, cwd string, permissionContext tool.PermissionContext) (bool, string) {
	for _, raw := range paths {
		if strings.HasPrefix(raw, "-") {
			return false, fmt.Sprintf("unsupported workspace file flag %q", raw)
		}
		if strings.ContainsAny(raw, "*?[") {
			return false, "glob workspace mutation is not allowed through shell"
		}
		path := resolveShellArgPath(raw, cwd)
		if _, err := permissionContext.ResolveMutationPathChecked(path); err != nil {
			return false, err.Error()
		}
	}
	return true, ""
}

func validateReadableArg(raw string, cwd string, permissionContext tool.PermissionContext) string {
	if strings.HasPrefix(raw, "-") {
		return fmt.Sprintf("unsupported file argument flag %q", raw)
	}
	path := resolveShellArgPath(raw, cwd)
	if err := permissionContext.CheckSensitivePath(path); err != nil {
		return err.Error()
	}
	if !pathWithinAnyTaskRoot(path, permissionContext.ReadableRoots) {
		return fmt.Sprintf("path %q is outside readable roots", raw)
	}
	return ""
}

func resolveShellArgPath(path string, cwd string) string {
	path = cleanShellPathArg(path)
	if filepath.IsAbs(path) || strings.HasPrefix(path, "~") {
		return filepath.Clean(path)
	}
	if strings.TrimSpace(cwd) == "" {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(cwd, path))
}

func pathWithinAnyTaskRoot(path string, roots []string) bool {
	path = filepath.Clean(path)
	for _, root := range roots {
		if tool.PathWithinRoot(path, root) {
			return true
		}
	}
	return false
}

func isSafeTestBuildCommand(command string) bool {
	if hasShellCompoundOperator(command) || strings.Contains(command, ">") || strings.Contains(command, "<") {
		return false
	}
	tokens := shellTokens(command)
	if len(tokens) == 0 {
		return false
	}
	base := filepath.Base(tokens[0])
	args := tokens[1:]
	switch base {
	case "go":
		if len(args) == 0 {
			return false
		}
		switch args[0] {
		case "test", "build", "vet", "fmt":
			return true
		}
	case "npm", "pnpm", "yarn":
		return isSafePackageScript(args)
	case "pytest":
		return true
	case "python", "python3":
		return len(args) >= 2 && args[0] == "-m" && args[1] == "pytest"
	case "cargo":
		return len(args) > 0 && containsAnyShellToken([]string{args[0]}, "test", "build", "check", "fmt")
	case "make":
		if len(args) == 0 {
			return false
		}
		return containsAnyShellToken([]string{args[0]}, "test", "build", "check", "lint", "fmt", "format")
	}
	return false
}

func hasShellCompoundOperator(command string) bool {
	inSingle := false
	inDouble := false
	for i := 0; i < len(command); i++ {
		c := command[i]
		switch {
		case c == '\'' && !inDouble:
			inSingle = !inSingle
		case c == '"' && !inSingle:
			inDouble = !inDouble
		case inSingle || inDouble:
			continue
		case c == ';':
			return true
		case c == '&' && i+1 < len(command) && command[i+1] == '&':
			return true
		case c == '|' && i+1 < len(command) && command[i+1] == '|':
			return true
		case c == '|':
			return true
		}
	}
	return false
}

func isSafePackageScript(args []string) bool {
	if len(args) == 0 {
		return false
	}
	if args[0] == "test" {
		return true
	}
	if args[0] == "run" && len(args) >= 2 {
		return containsAnyShellToken([]string{args[1]}, "test", "build", "check", "lint", "typecheck", "fmt", "format")
	}
	return false
}

func isDependencyNetworkCommand(command string) bool {
	tokens := shellTokens(command)
	if len(tokens) == 0 {
		return false
	}
	base := filepath.Base(tokens[0])
	args := tokens[1:]
	switch base {
	case "curl", "wget", "ssh", "scp", "rsync":
		return true
	case "npm", "pnpm", "yarn":
		return containsAnyShellToken(args, "install", "i", "add", "update", "upgrade", "remove", "uninstall")
	case "pip", "pip3", "pipx", "conda":
		return true
	case "go":
		return containsAnyShellToken(args, "get", "install")
	case "git":
		return containsAnyShellToken(args, "clone", "pull", "fetch", "push")
	}
	return false
}
