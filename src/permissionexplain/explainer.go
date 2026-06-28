package permissionexplain

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

const maxDisplayedInput = 140

func Explain(request Request) Result {
	toolName := strings.TrimSpace(request.ToolName)
	normalizedTool := normalizeToolName(toolName)

	switch normalizedTool {
	case "shell", "bash", "terminal", "exec", "command":
		return explainShell(request)
	case "read_file", "file_read", "list_files", "grep_search", "calculator":
		return redactResult(Result{
			RiskLevel:   RiskLevelLow,
			Explanation: fmt.Sprintf("%s will read local information.", displayToolName(toolName)),
			Risk:        "No expected changes.",
			Reasoning:   reasoningFor(request, "I need to inspect existing information without changing files."),
		})
	case "write_file", "edit_file", "apply_patch", "file_write":
		return explainFileMutation(request)
	case "delete_file", "remove_file":
		return redactResult(Result{
			RiskLevel:   RiskLevelHigh,
			Explanation: fmt.Sprintf("%s will delete local files.", displayToolName(toolName)),
			Risk:        "Data may be lost.",
			Reasoning:   reasoningFor(request, "I need approval before a destructive file operation."),
		})
	case "web_search", "web_fetch", "fetch", "http_request", "network":
		return redactResult(Result{
			RiskLevel:   RiskLevelMedium,
			Explanation: fmt.Sprintf("%s will contact an external network service.", displayToolName(toolName)),
			Risk:        "Request data may leave the workspace.",
			Reasoning:   reasoningFor(request, "I need to retrieve information from outside the local workspace."),
		})
	default:
		return redactResult(Result{
			RiskLevel:   RiskLevelMedium,
			Explanation: fmt.Sprintf("%s is not recognized by the local risk engine.", displayToolName(toolName)),
			Risk:        "Behavior is uncertain.",
			Reasoning:   reasoningFor(request, "I need approval because this tool has no deterministic local rule yet."),
		})
	}
}

func explainShell(request Request) Result {
	command := strings.TrimSpace(extractInputValue(request.Input, "command"))
	if command == "" {
		command = strings.TrimSpace(request.Input)
	}
	if command == "" {
		return redactResult(Result{
			RiskLevel:   RiskLevelMedium,
			Explanation: "The shell tool is missing a command.",
			Risk:        "Behavior is uncertain.",
			Reasoning:   reasoningFor(request, "I need a command before I can explain the shell risk precisely."),
		})
	}

	rules := append(defaultShellRules(), request.ShellRules...)
	matcher := NewMatcher(rules)
	if match, ok := matcher.MatchShell(command); ok {
		explanation := match.Rule.Explanation
		if explanation == "" {
			explanation = shellExplanationForLevel(command, match.Rule.RiskLevel)
		}
		risk := match.Rule.Risk
		if risk == "" {
			risk = shellRiskForLevel(match.Rule.RiskLevel)
		}
		return redactResult(Result{
			RiskLevel:   match.Rule.RiskLevel,
			Explanation: explanation,
			Risk:        risk,
			Reasoning:   reasoningFor(request, fmt.Sprintf("I need to run `%s` and it matched the %s shell rule `%s`.", shortInput(command), match.ParsedRule.Type, match.Rule.Pattern)),
		})
	}

	level := classifyShellCommand(command)
	return redactResult(Result{
		RiskLevel:   level,
		Explanation: shellExplanationForLevel(command, level),
		Risk:        shellRiskForLevel(level),
		Reasoning:   reasoningFor(request, fmt.Sprintf("I need to run `%s`; the local classifier selected %s risk from deterministic shell patterns.", shortInput(command), level)),
	})
}

func explainFileMutation(request Request) Result {
	input := request.Input
	level := RiskLevelMedium
	risk := "Files may be changed."
	if strings.Contains(input, "*** Delete File:") ||
		strings.Contains(strings.ToLower(input), `"delete"`) ||
		strings.Contains(strings.ToLower(input), `"remove"`) {
		level = RiskLevelHigh
		risk = "Data may be lost."
	}
	return redactResult(Result{
		RiskLevel:   level,
		Explanation: fmt.Sprintf("%s will modify files in the workspace.", displayToolName(request.ToolName)),
		Risk:        risk,
		Reasoning:   reasoningFor(request, "I need approval because this operation can change repository contents."),
	})
}

func defaultShellRules() []ShellRule {
	return []ShellRule{
		{Pattern: "rm *", RiskLevel: RiskLevelHigh, Explanation: "The command can delete files.", Risk: "Data may be lost."},
		{Pattern: "sudo *", RiskLevel: RiskLevelHigh, Explanation: "The command asks for elevated system privileges.", Risk: "System state may change."},
		{Pattern: "dd *", RiskLevel: RiskLevelHigh, Explanation: "The command can overwrite raw data or devices.", Risk: "Data may be destroyed."},
		{Pattern: "mkfs *", RiskLevel: RiskLevelHigh, Explanation: "The command can format storage.", Risk: "Data may be destroyed."},
		{Pattern: "diskutil erase*", RiskLevel: RiskLevelHigh, Explanation: "The command can erase disks or volumes.", Risk: "Data may be destroyed."},
		{Pattern: "git reset --hard*", RiskLevel: RiskLevelHigh, Explanation: "The command can discard local Git changes.", Risk: "Local work may be lost."},
		{Pattern: "git clean *", RiskLevel: RiskLevelHigh, Explanation: "The command can remove untracked files.", Risk: "Local files may be lost."},
		{Pattern: "git push *--force*", RiskLevel: RiskLevelHigh, Explanation: "The command can rewrite remote history.", Risk: "Shared history may be overwritten."},
		{Pattern: "curl *|*sh*", RiskLevel: RiskLevelHigh, Explanation: "The command downloads and executes a script.", Risk: "Untrusted code may run."},
		{Pattern: "wget *|*sh*", RiskLevel: RiskLevelHigh, Explanation: "The command downloads and executes a script.", Risk: "Untrusted code may run."},
		{Pattern: "kubectl delete *", RiskLevel: RiskLevelHigh, Explanation: "The command can delete Kubernetes resources.", Risk: "Services may be disrupted."},
		{Pattern: "docker system prune*", RiskLevel: RiskLevelHigh, Explanation: "The command can delete Docker data.", Risk: "Local containers or caches may be lost."},
		{Pattern: "npm publish*", RiskLevel: RiskLevelHigh, Explanation: "The command can publish a package.", Risk: "Public release state may change."},
		{Pattern: "touch *", RiskLevel: RiskLevelMedium, Explanation: "The command can create or update files.", Risk: "Files may be changed."},
		{Pattern: "mkdir *", RiskLevel: RiskLevelMedium, Explanation: "The command can create directories.", Risk: "Filesystem state may change."},
		{Pattern: "mv *", RiskLevel: RiskLevelMedium, Explanation: "The command can move or rename files.", Risk: "Paths may change."},
		{Pattern: "cp *", RiskLevel: RiskLevelMedium, Explanation: "The command can copy files.", Risk: "Files may be changed."},
		{Pattern: "git add *", RiskLevel: RiskLevelMedium, Explanation: "The command can stage repository changes.", Risk: "Commit contents may change."},
		{Pattern: "git commit *", RiskLevel: RiskLevelMedium, Explanation: "The command can create a Git commit.", Risk: "Repository history may change."},
		{Pattern: "git push *", RiskLevel: RiskLevelMedium, Explanation: "The command can publish commits.", Risk: "Remote state may change."},
		{Pattern: "curl *", RiskLevel: RiskLevelMedium, Explanation: "The command can contact a network endpoint.", Risk: "Request data may leave the workspace."},
		{Pattern: "wget *", RiskLevel: RiskLevelMedium, Explanation: "The command can download network content.", Risk: "Remote content may be trusted incorrectly."},
		{Pattern: "pwd", RiskLevel: RiskLevelLow, Explanation: "The command prints the current directory.", Risk: "No expected changes."},
		{Pattern: "ls *", RiskLevel: RiskLevelLow, Explanation: "The command lists files.", Risk: "No expected changes."},
		{Pattern: "cat *", RiskLevel: RiskLevelLow, Explanation: "The command reads file contents.", Risk: "No expected changes."},
		{Pattern: "grep *", RiskLevel: RiskLevelLow, Explanation: "The command searches text.", Risk: "No expected changes."},
		{Pattern: "rg *", RiskLevel: RiskLevelLow, Explanation: "The command searches text.", Risk: "No expected changes."},
		{Pattern: "git status *", RiskLevel: RiskLevelLow, Explanation: "The command inspects repository status.", Risk: "No expected changes."},
		{Pattern: "git diff *", RiskLevel: RiskLevelLow, Explanation: "The command inspects repository differences.", Risk: "No expected changes."},
		{Pattern: "git log *", RiskLevel: RiskLevelLow, Explanation: "The command inspects repository history.", Risk: "No expected changes."},
		{Pattern: "git show *", RiskLevel: RiskLevelLow, Explanation: "The command inspects Git objects.", Risk: "No expected changes."},
	}
}

func classifyShellCommand(command string) RiskLevel {
	lower := strings.ToLower(strings.TrimSpace(command))
	if lower == "" {
		return RiskLevelMedium
	}
	if hasHighRiskShellShape(lower) {
		return RiskLevelHigh
	}
	if hasShellControlOperator(lower) {
		return RiskLevelMedium
	}
	base := shellBaseName(lower)
	switch base {
	case "pwd", "ls", "cat", "head", "tail", "wc", "stat", "file", "strings", "grep", "rg", "find", "date", "whoami", "which", "whereis":
		return RiskLevelLow
	case "touch", "mkdir", "mv", "cp", "tee", "chmod", "chown", "git", "npm", "pnpm", "yarn", "go", "make", "curl", "wget":
		return RiskLevelMedium
	default:
		return RiskLevelMedium
	}
}

func hasHighRiskShellShape(command string) bool {
	highRiskFragments := []string{
		"rm -rf", "rm -fr", "sudo ", "dd ", "mkfs", "diskutil erase",
		"git reset --hard", "git clean -", "git push --force", "git push -f",
		"docker system prune", "kubectl delete", "drop database", "npm publish",
		"shutdown", "reboot",
	}
	for _, fragment := range highRiskFragments {
		if strings.Contains(command, fragment) {
			return true
		}
	}
	return (strings.Contains(command, "curl ") || strings.Contains(command, "wget ")) &&
		strings.Contains(command, "|") &&
		(strings.Contains(command, " sh") || strings.Contains(command, " bash"))
}

func hasShellControlOperator(command string) bool {
	operators := []string{"&&", "||", ";", "|", ">", "<", "`", "$("}
	for _, operator := range operators {
		if strings.Contains(command, operator) {
			return true
		}
	}
	return false
}

func shellExplanationForLevel(command string, level RiskLevel) string {
	switch level {
	case RiskLevelLow:
		return fmt.Sprintf("The shell command inspects local state: `%s`.", shortInput(command))
	case RiskLevelHigh:
		return fmt.Sprintf("The shell command can make destructive or privileged changes: `%s`.", shortInput(command))
	default:
		return fmt.Sprintf("The shell command may change state or use external effects: `%s`.", shortInput(command))
	}
}

func shellRiskForLevel(level RiskLevel) string {
	switch level {
	case RiskLevelLow:
		return "No expected changes."
	case RiskLevelHigh:
		return "Changes may be destructive."
	default:
		return "State may change."
	}
}

func reasoningFor(request Request, fallback string) string {
	context := strings.TrimSpace(request.Context)
	if context == "" {
		return fallback
	}
	return fmt.Sprintf("%s Context: %s", fallback, shortInput(context))
}

func extractInputValue(input string, key string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return ""
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(input), &decoded); err != nil {
		return ""
	}
	value, ok := decoded[key]
	if !ok {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return fmt.Sprint(typed)
	}
}

func normalizeToolName(toolName string) string {
	toolName = strings.ToLower(strings.TrimSpace(toolName))
	toolName = strings.ReplaceAll(toolName, "-", "_")
	return filepath.Base(toolName)
}

func displayToolName(toolName string) string {
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		return "The requested tool"
	}
	return toolName
}

func shellBaseName(command string) string {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return ""
	}
	return filepath.Base(fields[0])
}

func shortInput(value string) string {
	value = RedactSecrets(strings.TrimSpace(value))
	value = strings.Join(strings.Fields(value), " ")
	if len([]rune(value)) <= maxDisplayedInput {
		return value
	}
	runes := []rune(value)
	return string(runes[:maxDisplayedInput]) + "..."
}

func redactResult(result Result) Result {
	result.Explanation = RedactSecrets(result.Explanation)
	result.Reasoning = RedactSecrets(result.Reasoning)
	result.Risk = RedactSecrets(result.Risk)
	if !isRiskLevel(result.RiskLevel) {
		result.RiskLevel = RiskLevelMedium
	}
	return result
}
