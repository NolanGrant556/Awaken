package shelltool

import (
	"path/filepath"
	"strings"
)

// ShellTier grades a shell command by its blast radius.
//
// The three tiers mirror the "零审批自动执行策略" spec's grading model:
//   - inspect_read_only:   read-only inspection commands that do not mutate state.
//   - workspace_mutation:  commands that mutate files within the workspace.
//   - open_world_system:   commands that reach outside the workspace or affect the system.
type ShellTier string

const (
	ShellTierInspectReadOnly   ShellTier = "inspect_read_only"
	ShellTierWorkspaceMutation ShellTier = "workspace_mutation"
	ShellTierOpenWorldSystem   ShellTier = "open_world_system"
)

// tierRank orders the tiers from least to most risky so compound commands
// can be classified by their highest-risk segment.
func tierRank(t ShellTier) int {
	switch t {
	case ShellTierInspectReadOnly:
		return 0
	case ShellTierWorkspaceMutation:
		return 1
	case ShellTierOpenWorldSystem:
		return 2
	default:
		return 2
	}
}

// ClassifyShellCommand parses the command string and returns its ShellTier.
// cwd is the working directory the command would run in; it is used to decide
// whether redirect targets fall inside the workspace.
//
// Compound commands (joined by &&, ||, ;, |) are classified by the
// highest-risk segment. Redirects (> or >>) to a workspace path raise the
// tier to at least workspace_mutation; redirects to a path outside the
// workspace raise it to open_world_system.
func ClassifyShellCommand(command string, cwd string) ShellTier {
	command = strings.TrimSpace(command)
	if command == "" {
		return ShellTierInspectReadOnly
	}

	segments := splitCompoundSegments(command)
	if len(segments) == 0 {
		return ShellTierInspectReadOnly
	}

	highest := ShellTierInspectReadOnly
	for _, seg := range segments {
		tier := classifySegment(strings.TrimSpace(seg), cwd)
		if tierRank(tier) > tierRank(highest) {
			highest = tier
		}
	}
	return highest
}

// IsInspectReadOnlyCommand reports whether command is classified as inspect_read_only.
func IsInspectReadOnlyCommand(command string, cwd string) bool {
	return ClassifyShellCommand(command, cwd) == ShellTierInspectReadOnly
}

// IsWorkspaceMutationCommand reports whether command is classified as workspace_mutation.
func IsWorkspaceMutationCommand(command string, cwd string) bool {
	return ClassifyShellCommand(command, cwd) == ShellTierWorkspaceMutation
}

// IsOpenWorldCommand reports whether command is classified as open_world_system.
func IsOpenWorldCommand(command string, cwd string) bool {
	return ClassifyShellCommand(command, cwd) == ShellTierOpenWorldSystem
}

// splitCompoundSegments splits a command on shell compound operators
// (&&, ||, ;, |) while leaving quoted strings intact. This is a deliberately
// conservative splitter — it does not try to fully parse shell syntax.
func splitCompoundSegments(command string) []string {
	var segments []string
	var current strings.Builder
	inSingle := false
	inDouble := false
	i := 0
	for i < len(command) {
		c := command[i]
		switch {
		case c == '\'' && !inDouble:
			inSingle = !inSingle
			current.WriteByte(c)
			i++
		case c == '"' && !inSingle:
			inDouble = !inDouble
			current.WriteByte(c)
			i++
		case inSingle || inDouble:
			current.WriteByte(c)
			i++
		case c == ';':
			segments = append(segments, current.String())
			current.Reset()
			i++
		case c == '&' && i+1 < len(command) && command[i+1] == '&':
			segments = append(segments, current.String())
			current.Reset()
			i += 2
		case c == '|' && i+1 < len(command) && command[i+1] == '|':
			segments = append(segments, current.String())
			current.Reset()
			i += 2
		case c == '|':
			segments = append(segments, current.String())
			current.Reset()
			i++
		default:
			current.WriteByte(c)
			i++
		}
	}
	segments = append(segments, current.String())
	return segments
}

// classifySegment classifies a single (non-compound) shell segment, taking
// redirect operators into account.
func classifySegment(segment string, cwd string) ShellTier {
	segment = strings.TrimSpace(segment)
	if segment == "" {
		return ShellTierInspectReadOnly
	}

	if target, ok := extractRedirectTarget(segment); ok {
		if isPathOutsideWorkspace(target, cwd) {
			return ShellTierOpenWorldSystem
		}
		base := classifyBaseCommand(stripRedirect(segment), cwd)
		if tierRank(base) < tierRank(ShellTierWorkspaceMutation) {
			return ShellTierWorkspaceMutation
		}
		return base
	}

	return classifyBaseCommand(segment, cwd)
}

// extractRedirectTarget returns the path of the first > or >> redirect target
// found in the segment, ignoring quoted strings. ok=false if no file redirect
// is present. File descriptor redirects (>&1, >&2) are not treated as file
// redirects.
func extractRedirectTarget(segment string) (string, bool) {
	inSingle := false
	inDouble := false
	i := 0
	for i < len(segment) {
		c := segment[i]
		switch {
		case c == '\'' && !inDouble:
			inSingle = !inSingle
			i++
		case c == '"' && !inSingle:
			inDouble = !inDouble
			i++
		case inSingle || inDouble:
			i++
		case c == '>':
			rest := segment[i+1:]
			if strings.HasPrefix(rest, ">") {
				rest = rest[1:]
			}
			rest = strings.TrimLeft(rest, " \t")
			if rest == "" {
				return "", true
			}
			target := readPathToken(rest)
			if target == "" {
				return "", true
			}
			if strings.HasPrefix(target, "&") {
				// File descriptor redirect (>&1, >&2) — not a file mutation.
				return "", false
			}
			return target, true
		default:
			i++
		}
	}
	return "", false
}

// readPathToken reads the first whitespace-delimited token from s.
func readPathToken(s string) string {
	end := strings.IndexAny(s, " \t")
	if end == -1 {
		return s
	}
	return s[:end]
}

// stripRedirect returns the segment with the first > or >> redirect clause removed.
func stripRedirect(segment string) string {
	inSingle := false
	inDouble := false
	i := 0
	for i < len(segment) {
		c := segment[i]
		switch {
		case c == '\'' && !inDouble:
			inSingle = !inSingle
			i++
		case c == '"' && !inSingle:
			inDouble = !inDouble
			i++
		case inSingle || inDouble:
			i++
		case c == '>':
			start := i
			rest := segment[i+1:]
			if strings.HasPrefix(rest, ">") {
				rest = rest[1:]
			}
			trimmed := strings.TrimLeft(rest, " \t")
			end := strings.IndexAny(trimmed, " \t")
			if end == -1 {
				return strings.TrimSpace(segment[:start])
			}
			return strings.TrimSpace(segment[:start] + trimmed[end:])
		default:
			i++
		}
	}
	return segment
}

// isPathOutsideWorkspace reports whether path resolves outside cwd.
// Relative paths without a leading ".." are considered inside the workspace.
func isPathOutsideWorkspace(path string, cwd string) bool {
	path = strings.TrimSpace(path)
	if path == "" {
		return false
	}
	if filepath.IsAbs(path) {
		if cwd == "" {
			return true
		}
		rel, err := filepath.Rel(cwd, path)
		if err != nil {
			return true
		}
		return strings.HasPrefix(rel, "..")
	}
	if strings.HasPrefix(path, "..") {
		return true
	}
	return false
}

// classifyBaseCommand classifies a single command (no compound operators, no
// file redirects) by inspecting its first token and arguments.
func classifyBaseCommand(command string, cwd string) ShellTier {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return ShellTierInspectReadOnly
	}
	base := filepath.Base(fields[0])
	args := fields[1:]

	if isOpenWorldCommand(base, args) {
		return ShellTierOpenWorldSystem
	}
	if isWorkspaceMutationCommand(base, args) {
		return ShellTierWorkspaceMutation
	}
	if isInspectReadOnlyCommand(base, args) {
		return ShellTierInspectReadOnly
	}
	// Unknown command — conservative middle tier. The spec does not define a
	// default; workspace_mutation ensures the caller will still gate the
	// command without assuming it reaches outside the workspace.
	return ShellTierWorkspaceMutation
}

func isOpenWorldCommand(base string, args []string) bool {
	switch base {
	case "curl", "wget", "ssh", "scp", "rsync", "brew", "apt", "apt-get", "yum",
		"dnf", "pip", "pip3", "pipx", "conda", "docker", "podman", "kubectl",
		"sudo", "doas", "pkexec", "kill", "killall", "pkill", "launchctl",
		"systemctl", "service", "networksetup", "open":
		return true
	case "git":
		return isOpenWorldGitCommand(args)
	case "npm", "yarn", "pnpm":
		return containsAnyShellToken(args, "-g", "--global") &&
			containsAnyShellToken(args, "install", "i", "update", "upgrade", "add")
	case "go":
		return containsAnyShellToken(args, "install", "get")
	case "chmod", "chown", "chgrp":
		return isOpenWorldChmodTarget(args)
	}
	return false
}

func isOpenWorldGitCommand(args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch args[0] {
	case "push", "pull", "fetch", "clone":
		return true
	case "remote":
		if containsAnyShellToken(args[1:], "add", "remove", "rm", "set-url", "rename") {
			return true
		}
		return false
	case "config":
		return containsAnyShellToken(args, "--global", "--system")
	}
	return false
}

func isOpenWorldChmodTarget(args []string) bool {
	// chmod/chown on absolute paths are treated as open_world since they
	// typically target system files.
	for _, arg := range args {
		if strings.HasPrefix(arg, "/") {
			return true
		}
	}
	return false
}

func isWorkspaceMutationCommand(base string, args []string) bool {
	switch base {
	case "mkdir", "touch", "cp", "mv", "rm", "rmdir", "ln", "install":
		return true
	case "sed":
		return containsAnyShellToken(args, "-i", "--in-place")
	case "npm", "yarn", "pnpm":
		return containsAnyShellToken(args, "install", "i", "update", "upgrade", "add", "remove", "uninstall")
	case "go":
		return containsAnyShellToken(args, "build", "test", "vet", "fmt", "mod")
	case "make", "cargo", "cmake", "gradle", "mvn":
		return true
	case "git":
		return isWorkspaceMutationGitCommand(args)
	case "chmod", "chown", "chgrp":
		// chmod/chown on relative paths mutate workspace files.
		return !isOpenWorldChmodTarget(args)
	}
	return false
}

func isWorkspaceMutationGitCommand(args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch args[0] {
	case "add", "commit", "checkout", "switch", "stash", "reset", "restore",
		"merge", "rebase", "cherry-pick", "revert", "clean", "mv", "rm",
		"init", "apply", "am", "tag", "branch":
		return true
	}
	return false
}

func isInspectReadOnlyCommand(base string, args []string) bool {
	switch base {
	case "pwd", "ls", "cat", "head", "tail", "wc", "stat", "file", "strings",
		"rg", "grep", "tree", "du", "df", "which", "whereis", "whoami", "id",
		"uname", "ps", "top", "htop", "date", "echo", "printf":
		return true
	case "env":
		// `env` without args prints env vars (read-only). With arguments it
		// may execute a command — fall through to the conservative default.
		return len(args) == 0
	case "find":
		return !containsAnyShellToken(args, "-delete", "-exec", "-execdir", "-ok")
	case "git":
		return isInspectReadOnlyGitCommand(args)
	}
	return false
}

func isInspectReadOnlyGitCommand(args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch args[0] {
	case "status", "diff", "log", "show", "rev-parse", "ls-files", "blame",
		"cat-file", "ls-tree", "describe", "shortlog", "reflog", "symbolic-ref":
		return true
	case "branch":
		return !containsAnyShellToken(args, "-d", "-D", "-m", "-M", "--delete")
	case "remote":
		return !containsAnyShellToken(args, "add", "remove", "rm", "set-url", "rename")
	case "config":
		return containsAnyShellToken(args, "--get", "--list", "-l", "--get-all") &&
			!containsAnyShellToken(args, "--global", "--system")
	}
	return false
}
