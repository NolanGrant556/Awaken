package prflow

import (
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

const maxPRTitleRunes = 70

type Input struct {
	Snapshot GitSnapshot
	Options  Options
}

type GitSnapshot struct {
	DefaultBranch    string
	CurrentBranch    string
	SafeUser         string
	Username         string
	Status           string
	DiffHead         string
	DiffDefault      string
	ExistingPRNumber int
}

type Options struct {
	PrAttribution      string
	CommitAttribution  string
	UserInstructions   string
	UserType           string
	Undercover         bool
	ProposedTitle      string
	ProposedBranchName string
}

type PlanResult struct {
	Actions                []Action
	AllowedToolPatterns    []string
	DisallowedToolPatterns []string
	SafetyWarnings         []SafetyWarning
	ExcludedPaths          []string
	Branch                 BranchPlan
	Commit                 CommitPlan
	PullRequest            PullRequestPlan
	AdditionalInstructions string
}

type ActionKind string

const (
	ActionCreateBranch ActionKind = "create_branch"
	ActionStage        ActionKind = "stage_changes"
	ActionCommit       ActionKind = "commit"
	ActionPush         ActionKind = "push"
	ActionCreatePR     ActionKind = "create_pr"
	ActionEditPR       ActionKind = "edit_pr"
)

type Action struct {
	Order        int
	Kind         ActionKind
	Description  string
	Command      string
	ToolPatterns []string
	ExcludePaths []string
}

type BranchPlan struct {
	DefaultBranch string
	CurrentBranch string
	TargetBranch  string
	CreateBranch  bool
}

type CommitPlan struct {
	Message string
}

type PRMode string

const (
	PRModeCreate PRMode = "create"
	PRModeEdit   PRMode = "edit"
)

type PullRequestPlan struct {
	Mode             PRMode
	ExistingNumber   int
	Title            string
	Body             string
	ReviewerArgument string
}

type SafetyWarning struct {
	ID      string
	Message string
	Paths   []string
}

var allowedToolPatterns = []string{
	"Bash(git checkout --branch:*)",
	"Bash(git checkout -b:*)",
	"Bash(git add:*)",
	"Bash(git status:*)",
	"Bash(git push:*)",
	"Bash(git commit:*)",
	"Bash(gh pr create:*)",
	"Bash(gh pr edit:*)",
	"Bash(gh pr view:*)",
	"ToolSearch",
	"mcp__slack__send_message",
	"mcp__claude_ai_Slack__slack_send_message",
}

var disallowedToolPatterns = []string{
	"Bash(git config:*)",
	"Bash(git reset --hard:*)",
	"Bash(git clean -fd:*)",
	"Bash(git push --force:*)",
	"Bash(git push -f:*)",
	"Bash(git push --force-with-lease:*)",
	"Bash(git commit --no-verify:*)",
	"Bash(git commit --no-gpg-sign:*)",
	"Bash(git add -i:*)",
	"Bash(git rebase -i:*)",
}

func Plan(input Input) PlanResult {
	snapshot := normalizeSnapshot(input.Snapshot)
	options := input.Options
	title, titleWarning := normalizeTitle(options.ProposedTitle, snapshot)
	excludedPaths := findSecretLikePaths(snapshot.Status, snapshot.DiffHead, snapshot.DiffDefault)

	targetBranch := snapshot.CurrentBranch
	createBranch := snapshot.CurrentBranch == snapshot.DefaultBranch
	if createBranch {
		targetBranch = normalizeBranchName(options.ProposedBranchName)
		if targetBranch == "" {
			targetBranch = branchName(snapshot, title)
		}
	}

	prMode := PRModeCreate
	if snapshot.ExistingPRNumber > 0 {
		prMode = PRModeEdit
	}

	warnings := baseWarnings()
	if titleWarning != nil {
		warnings = append(warnings, *titleWarning)
	}
	if len(excludedPaths) > 0 {
		warnings = append(warnings, SafetyWarning{
			ID:      "secret-like-paths-excluded",
			Message: "Likely secret-bearing paths are excluded from the staging action and must not be committed.",
			Paths:   copyStrings(excludedPaths),
		})
	}
	if isMainOrMaster(targetBranch) {
		warnings = append(warnings, SafetyWarning{
			ID:      "main-master-push-target",
			Message: "The planned push target is main/master; force push must be refused and the user should be warned before any direct push.",
		})
	}

	commitMessage := commitMessage(title, options.CommitAttribution)
	prBody := prBody(title, options.PrAttribution, options.UserInstructions)
	result := PlanResult{
		AllowedToolPatterns:    copyStrings(allowedToolPatterns),
		DisallowedToolPatterns: copyStrings(disallowedToolPatterns),
		SafetyWarnings:         warnings,
		ExcludedPaths:          copyStrings(excludedPaths),
		Branch: BranchPlan{
			DefaultBranch: snapshot.DefaultBranch,
			CurrentBranch: snapshot.CurrentBranch,
			TargetBranch:  targetBranch,
			CreateBranch:  createBranch,
		},
		Commit: CommitPlan{
			Message: commitMessage,
		},
		PullRequest: PullRequestPlan{
			Mode:           prMode,
			ExistingNumber: snapshot.ExistingPRNumber,
			Title:          title,
			Body:           prBody,
		},
		AdditionalInstructions: strings.TrimSpace(options.UserInstructions),
	}

	if createBranch {
		result.Actions = append(result.Actions, Action{
			Kind:         ActionCreateBranch,
			Description:  "Create a feature branch before staging or committing because the current branch is the default branch.",
			Command:      fmt.Sprintf("git checkout -b %s", shellQuote(targetBranch)),
			ToolPatterns: []string{"Bash(git checkout -b:*)", "Bash(git checkout --branch:*)"},
		})
	}

	result.Actions = append(result.Actions,
		Action{
			Kind:         ActionStage,
			Description:  "Stage all non-secret changes that belong in the commit.",
			Command:      gitAddCommand(excludedPaths),
			ToolPatterns: []string{"Bash(git add:*)", "Bash(git status:*)"},
			ExcludePaths: copyStrings(excludedPaths),
		},
		Action{
			Kind:         ActionCommit,
			Description:  "Create one commit with a heredoc message.",
			Command:      heredocCommand("git commit -m", commitMessage),
			ToolPatterns: []string{"Bash(git commit:*)"},
		},
		Action{
			Kind:         ActionPush,
			Description:  "Push the branch to origin without force flags.",
			Command:      fmt.Sprintf("git push -u origin %s", shellQuote(targetBranch)),
			ToolPatterns: []string{"Bash(git push:*)"},
		},
	)

	if prMode == PRModeEdit {
		result.Actions = append(result.Actions, Action{
			Kind:         ActionEditPR,
			Description:  "Update the existing pull request for this branch.",
			Command:      prEditCommand(snapshot.ExistingPRNumber, title, prBody),
			ToolPatterns: []string{"Bash(gh pr edit:*)", "Bash(gh pr view:*)"},
		})
	} else {
		result.Actions = append(result.Actions, Action{
			Kind:         ActionCreatePR,
			Description:  "Create a pull request for this branch.",
			Command:      prCreateCommand(title, prBody),
			ToolPatterns: []string{"Bash(gh pr create:*)", "Bash(gh pr view:*)"},
		})
	}

	for i := range result.Actions {
		result.Actions[i].Order = i + 1
	}
	return result
}

func normalizeSnapshot(snapshot GitSnapshot) GitSnapshot {
	snapshot.DefaultBranch = strings.TrimSpace(snapshot.DefaultBranch)
	if snapshot.DefaultBranch == "" {
		snapshot.DefaultBranch = "main"
	}
	snapshot.CurrentBranch = strings.TrimSpace(snapshot.CurrentBranch)
	if snapshot.CurrentBranch == "" {
		snapshot.CurrentBranch = snapshot.DefaultBranch
	}
	snapshot.SafeUser = strings.TrimSpace(snapshot.SafeUser)
	snapshot.Username = strings.TrimSpace(snapshot.Username)
	return snapshot
}

func baseWarnings() []SafetyWarning {
	return []SafetyWarning{
		{
			ID:      "git-config-prohibited",
			Message: "Do not update git config as part of the commit/push/PR flow.",
		},
		{
			ID:      "destructive-git-prohibited",
			Message: "Do not run destructive or irreversible git commands, including hard reset, clean, or force push, unless explicitly requested.",
		},
		{
			ID:      "skip-hooks-prohibited",
			Message: "Do not skip hooks or signing checks with flags such as --no-verify or --no-gpg-sign unless explicitly requested.",
		},
		{
			ID:      "interactive-git-prohibited",
			Message: "Do not use interactive git flags such as git add -i or git rebase -i in this flow.",
		},
	}
}

func normalizeTitle(proposed string, snapshot GitSnapshot) (string, *SafetyWarning) {
	title := strings.TrimSpace(proposed)
	if title == "" {
		title = titleFromSnapshot(snapshot)
	}
	title = strings.Join(strings.Fields(title), " ")
	runes := []rune(title)
	if len(runes) <= maxPRTitleRunes {
		return title, nil
	}
	truncated := strings.TrimRightFunc(string(runes[:maxPRTitleRunes]), func(r rune) bool {
		return unicode.IsSpace(r) || r == '-' || r == ':' || r == ',' || r == '.'
	})
	if truncated == "" {
		truncated = string(runes[:maxPRTitleRunes])
	}
	return truncated, &SafetyWarning{
		ID:      "pr-title-truncated",
		Message: "PR title exceeded 70 characters and was truncated; keep details in the body.",
	}
}

func titleFromSnapshot(snapshot GitSnapshot) string {
	if snapshot.CurrentBranch != "" && snapshot.CurrentBranch != snapshot.DefaultBranch {
		lastSegment := path.Base(snapshot.CurrentBranch)
		if title := titleFromSlug(lastSegment); title != "" {
			return title
		}
	}
	return "Update pull request changes"
}

func titleFromSlug(slug string) string {
	slug = strings.Trim(slug, "-_/ ")
	if slug == "" || slug == "." {
		return ""
	}
	parts := strings.FieldsFunc(slug, func(r rune) bool {
		return r == '-' || r == '_' || r == '/'
	})
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = capitalizeFirstRune(part)
	}
	return strings.Join(parts, " ")
}

func capitalizeFirstRune(value string) string {
	runes := []rune(value)
	if len(runes) == 0 {
		return ""
	}
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

func branchName(snapshot GitSnapshot, title string) string {
	user := snapshot.SafeUser
	if user == "" {
		user = snapshot.Username
	}
	if user == "" {
		user = "user"
	}
	return sanitizeBranchSegment(user) + "/" + sanitizeBranchSegment(title)
}

func normalizeBranchName(branch string) string {
	branch = strings.TrimSpace(branch)
	branch = strings.Trim(branch, "/")
	if branch == "" {
		return ""
	}
	segments := strings.Split(branch, "/")
	for i, segment := range segments {
		segments[i] = sanitizeBranchSegment(segment)
	}
	return strings.Join(segments, "/")
}

func sanitizeBranchSegment(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r == '-' || r == '_' || r == '.' || unicode.IsSpace(r):
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	result := strings.Trim(b.String(), "-")
	if result == "" {
		return "update"
	}
	return result
}

func commitMessage(title, attribution string) string {
	attribution = strings.TrimSpace(attribution)
	if attribution == "" {
		return title
	}
	return title + "\n\n" + attribution
}

func prBody(title, attribution, instructions string) string {
	attribution = strings.TrimSpace(attribution)
	if attribution == "" {
		attribution = "No additional attribution supplied."
	}

	sections := []string{
		"## Summary\n- " + title,
		"## Test Plan\n- [ ] Run the relevant project tests for these changes.",
		"## Changelog\n<!-- CHANGELOG:START -->\n- Remove this section if there are no user-facing changes.\n<!-- CHANGELOG:END -->",
		"## Attribution\n" + attribution,
	}
	instructions = strings.TrimSpace(instructions)
	if instructions != "" {
		sections = append(sections, "## Additional Instructions\n"+instructions)
	}
	return strings.Join(sections, "\n\n")
}

func gitAddCommand(excludedPaths []string) string {
	if len(excludedPaths) == 0 {
		return "git add -- ."
	}
	args := []string{"git add -- ."}
	for _, p := range excludedPaths {
		args = append(args, shellQuote(":!"+p))
	}
	return strings.Join(args, " ")
}

func heredocCommand(prefix, body string) string {
	return fmt.Sprintf("%s \"$(cat <<'EOF'\n%s\nEOF\n)\"", prefix, body)
}

func prCreateCommand(title, body string) string {
	return fmt.Sprintf("gh pr create --title %s --body \"$(cat <<'EOF'\n%s\nEOF\n)\"", shellQuote(title), body)
}

func prEditCommand(number int, title, body string) string {
	return fmt.Sprintf("gh pr edit %d --title %s --body \"$(cat <<'EOF'\n%s\nEOF\n)\"", number, shellQuote(title), body)
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func isMainOrMaster(branch string) bool {
	return branch == "main" || branch == "master"
}

func copyStrings(values []string) []string {
	out := make([]string, len(values))
	copy(out, values)
	return out
}

func findSecretLikePaths(contents ...string) []string {
	paths := map[string]struct{}{}
	for _, content := range contents {
		for _, p := range extractPaths(content) {
			if isSecretLikePath(p) {
				paths[p] = struct{}{}
			}
		}
	}
	result := make([]string, 0, len(paths))
	for p := range paths {
		result = append(result, p)
	}
	sort.Strings(result)
	return result
}

func extractPaths(content string) []string {
	var result []string
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "diff --git ") {
			fields := strings.Fields(line)
			if len(fields) >= 4 {
				result = append(result, trimDiffPath(fields[2]), trimDiffPath(fields[3]))
			}
			continue
		}
		if strings.HasPrefix(line, "+++ ") || strings.HasPrefix(line, "--- ") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				result = append(result, trimDiffPath(fields[1]))
			}
			continue
		}
		if strings.HasPrefix(line, "rename from ") {
			result = append(result, strings.TrimSpace(strings.TrimPrefix(line, "rename from ")))
			continue
		}
		if strings.HasPrefix(line, "rename to ") {
			result = append(result, strings.TrimSpace(strings.TrimPrefix(line, "rename to ")))
			continue
		}
		if strings.Contains(line, " -> ") {
			parts := strings.Split(line, " -> ")
			for _, part := range parts {
				result = append(result, normalizeStatusPath(part))
			}
			continue
		}
		result = append(result, normalizeStatusPath(line))
	}
	return cleanPathList(result)
}

func trimDiffPath(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "\"")
	if value == "/dev/null" {
		return ""
	}
	value = strings.TrimPrefix(value, "a/")
	value = strings.TrimPrefix(value, "b/")
	return value
}

func normalizeStatusPath(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "\"")
	if len(value) > 3 {
		status := value[:3]
		if status[2] == ' ' && isStatusRune(rune(status[0])) {
			return strings.TrimSpace(value[3:])
		}
	}
	if len(value) > 2 && value[1] == ' ' && isStatusRune(rune(value[0])) {
		return strings.TrimSpace(value[2:])
	}
	return value
}

func isStatusRune(r rune) bool {
	return strings.ContainsRune(" MADRCU?!", r)
}

func cleanPathList(paths []string) []string {
	seen := map[string]struct{}{}
	var result []string
	for _, p := range paths {
		p = strings.TrimSpace(p)
		p = strings.TrimPrefix(p, "a/")
		p = strings.TrimPrefix(p, "b/")
		p = strings.Trim(p, "\"")
		if p == "" || p == "/dev/null" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		result = append(result, p)
	}
	return result
}

var envFilePattern = regexp.MustCompile(`(^|/)\.env($|[.\-/])`)

func isSecretLikePath(p string) bool {
	p = strings.ToLower(strings.TrimSpace(p))
	if p == "" {
		return false
	}
	base := path.Base(p)
	if envFilePattern.MatchString(p) {
		return true
	}
	switch base {
	case "credentials.json", "credentials.yml", "credentials.yaml", "secret.json", "secrets.json", "id_rsa", "id_ed25519":
		return true
	}
	if strings.HasSuffix(base, ".pem") || strings.HasSuffix(base, ".p12") || strings.HasSuffix(base, ".pfx") || strings.HasSuffix(base, ".key") {
		return true
	}
	for _, segment := range strings.Split(p, "/") {
		if segment == "secrets" || segment == ".secrets" || segment == "credentials" {
			return true
		}
	}
	return false
}
