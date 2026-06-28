package githubactions

import (
	"net/url"
	"regexp"
	"sort"
	"strings"
)

const (
	DefaultAPIKeySecret = "ANTHROPIC_API_KEY"
	DefaultOAuthSecret  = "CLAUDE_CODE_OAUTH_TOKEN"
	SecretPlaceholder   = "<secret-value>"
	WorkflowPlaceholder = "<base64-workflow-content>"
	DefaultBranchName   = "add-claude-github-actions"

	InstallAppURL = "https://github.com/apps/claude"
	SetupDocsURL  = "https://github.com/anthropics/claude-code-action/blob/main/docs/setup.md"
	PRTitle       = "Add Claude Code GitHub Workflow"
)

type AuthType string

const (
	AuthTypeAPIKey     AuthType = "api_key"
	AuthTypeOAuthToken AuthType = "oauth_token"
)

type Workflow string

const (
	WorkflowClaude       Workflow = "claude"
	WorkflowClaudeReview Workflow = "claude-review"
)

type Input struct {
	GitHubAuthOutput    string
	GitHubVersionOutput string
	CurrentRepo         string
	Repo                RepoState
	SelectedWorkflows   []Workflow
	SecretName          string
	SecretValue         string
	AuthType            AuthType
	UseExistingSecret   bool
	SkipWorkflowFiles   bool
	BranchName          string
}

type RepoState struct {
	Name                    string
	Exists                  bool
	ExistsKnown             bool
	HasAdminPermission      bool
	AdminPermissionKnown    bool
	DefaultBranch           string
	DefaultBranchSHA        string
	ExistingWorkflowFiles   []string
	ExistingActionSecretIDs []string
}

type PlanResult struct {
	Repository     string
	BranchName     string
	SecretName     string
	AuthType       AuthType
	WorkflowFiles  []WorkflowFile
	Actions        []Action
	Warnings       []Warning
	BlockingErrors []BlockingError
}

type Warning struct {
	Code         string
	Title        string
	Message      string
	Instructions []string
	Commands     []CommandPattern
	Blocking     bool
}

type BlockingError struct {
	Code         string
	Title        string
	Message      string
	Instructions []string
	Commands     []CommandPattern
}

type ActionKind string

const (
	ActionCheckGitHubCLI ActionKind = "check_github_cli"
	ActionUseCurrentRepo ActionKind = "use_current_repo"
	ActionCheckRepo      ActionKind = "check_repository"
	ActionInstallApp     ActionKind = "install_github_app"
	ActionCreateBranch   ActionKind = "create_branch"
	ActionPutWorkflow    ActionKind = "put_workflow_file"
	ActionSetSecret      ActionKind = "set_secret"
	ActionOpenPR         ActionKind = "open_pull_request"
)

type Action struct {
	Kind           ActionKind
	Title          string
	Detail         string
	Path           string
	URL            string
	Commands       []CommandPattern
	ManualFallback []string
}

type CommandPattern struct {
	Program          string
	Args             []string
	StdinPlaceholder string
}

type WorkflowFile struct {
	Workflow Workflow
	Path     string
	Content  string
}

func Plan(input Input) PlanResult {
	authType := input.AuthType
	if authType == "" {
		authType = AuthTypeAPIKey
	}

	secretName := strings.TrimSpace(input.SecretName)
	if secretName == "" {
		secretName = DefaultAPIKeySecret
		if authType == AuthTypeOAuthToken {
			secretName = DefaultOAuthSecret
		}
	}
	secretName = NormalizeSecretName(secretName, authType)

	selected := normalizeWorkflows(input.SelectedWorkflows)
	branchName := strings.TrimSpace(input.BranchName)
	if branchName == "" {
		branchName = DefaultBranchName
	}

	repoName := strings.TrimSpace(input.Repo.Name)
	if repoName == "" {
		repoName = strings.TrimSpace(input.CurrentRepo)
	}
	normalizedRepo, repoOK := NormalizeRepoName(repoName)
	if repoOK {
		repoName = normalizedRepo
	}

	repo := input.Repo
	repo.Name = repoName

	result := PlanResult{
		Repository: repoName,
		BranchName: branchName,
		SecretName: secretName,
		AuthType:   authType,
		Warnings:   CheckGitHubCLI(input.GitHubAuthOutput, input.GitHubVersionOutput),
	}
	result.Warnings = append(result.Warnings, ValidateRepo(repo)...)
	result.Warnings = append(result.Warnings, workflowConflictWarnings(repo, selected)...)
	result.Warnings = append(result.Warnings, existingSecretWarning(repo, secretName, input.UseExistingSecret)...)
	result.BlockingErrors = blockingErrors(result.Warnings)

	if strings.TrimSpace(repoName) == "" {
		result.BlockingErrors = append(result.BlockingErrors, BlockingError{
			Code:    "repo_missing",
			Title:   "No GitHub repository selected",
			Message: "Awaken could not plan setup without a current or selected GitHub repository.",
			Instructions: []string{
				"Run from a GitHub-backed repository or provide a repository in owner/repo format.",
				"For manual setup, use the Claude Code GitHub Actions setup guide.",
			},
		})
	}

	if len(result.BlockingErrors) > 0 {
		return result
	}

	result.WorkflowFiles = BuildWorkflowFiles(selected, secretName, authType)
	result.Actions = buildActions(input, repo, result.WorkflowFiles, secretName, branchName)
	return result
}

func CheckGitHubCLI(authOutput, versionOutput string) []Warning {
	var warnings []Warning

	versionOutput = strings.TrimSpace(versionOutput)
	if versionOutput == "" || !strings.Contains(strings.ToLower(versionOutput), "gh version") {
		warnings = append(warnings, Warning{
			Code:    "missing_gh",
			Title:   "GitHub CLI not found",
			Message: "GitHub CLI (gh) does not appear to be installed or accessible.",
			Instructions: []string{
				"Install GitHub CLI from https://cli.github.com/.",
				"macOS: brew install gh",
				"After installing, run gh auth login.",
			},
		})
	}

	authOutput = strings.TrimSpace(authOutput)
	lowerAuth := strings.ToLower(authOutput)
	if authOutput == "" || strings.Contains(lowerAuth, "not logged") || strings.Contains(lowerAuth, "not authenticated") {
		warnings = append(warnings, Warning{
			Code:    "gh_auth_missing",
			Title:   "GitHub CLI not authenticated",
			Message: "GitHub CLI does not appear to be authenticated.",
			Instructions: []string{
				"Run gh auth login and follow the prompts.",
				"Then rerun setup planning.",
			},
			Commands: []CommandPattern{{Program: "gh", Args: []string{"auth", "login"}}},
		})
		return warnings
	}

	scopes, ok := parseTokenScopes(authOutput)
	if !ok {
		warnings = append(warnings, Warning{
			Code:    "gh_scopes_unknown",
			Title:   "GitHub CLI token scopes not found",
			Message: "Awaken could not verify whether gh has the repo and workflow scopes.",
			Instructions: []string{
				"Refresh gh auth scopes before writing workflows or secrets.",
			},
			Commands: []CommandPattern{refreshScopesCommand()},
		})
		return warnings
	}

	missing := missingScopes(scopes, []string{"repo", "workflow"})
	if len(missing) > 0 {
		warnings = append(warnings, Warning{
			Code:     "missing_scopes",
			Title:    "GitHub CLI is missing required scopes",
			Message:  "GitHub CLI needs repo and workflow scopes to create workflow files and set repository secrets.",
			Blocking: true,
			Instructions: []string{
				"Refresh GitHub CLI authentication with the required scopes.",
				"Rerun setup after gh reports repo and workflow in Token scopes.",
			},
			Commands: []CommandPattern{refreshScopesCommand()},
		})
	}

	return warnings
}

func ValidateRepo(repo RepoState) []Warning {
	var warnings []Warning

	name := strings.TrimSpace(repo.Name)
	if name != "" {
		if normalized, ok := NormalizeRepoName(name); !ok || !strings.Contains(normalized, "/") {
			warnings = append(warnings, Warning{
				Code:     "invalid_repo",
				Title:    "Repository format warning",
				Message:  "Repository should be in owner/repo format.",
				Blocking: true,
				Instructions: []string{
					"Use format owner/repo, for example anthropics/claude-code.",
					"GitHub URLs are accepted only when they point to github.com/owner/repo.",
				},
			})
		}
	}

	if repo.ExistsKnown && !repo.Exists {
		warnings = append(warnings, Warning{
			Code:     "repo_not_found",
			Title:    "Repository not found",
			Message:  "The selected repository was not found or the current GitHub token cannot access it.",
			Blocking: true,
			Instructions: []string{
				"Check the repository name.",
				"For private repositories, ensure gh has the repo scope.",
			},
			Commands: []CommandPattern{refreshScopesCommand()},
		})
	}

	if repo.AdminPermissionKnown && !repo.HasAdminPermission {
		warnings = append(warnings, Warning{
			Code:    "repo_admin_missing",
			Title:   "Admin permissions may be required",
			Message: "Repository admin permission was not detected; installing the app and setting secrets may fail.",
			Instructions: []string{
				"Ask a repository admin to run setup if writes fail.",
				"Use the manual setup guide if automated setup is unavailable.",
			},
		})
	}

	return warnings
}

func BuildWorkflowFiles(selected []Workflow, secretName string, authType AuthType) []WorkflowFile {
	secretName = strings.TrimSpace(secretName)
	if secretName == "" {
		secretName = DefaultAPIKeySecret
		if authType == AuthTypeOAuthToken {
			secretName = DefaultOAuthSecret
		}
	}
	secretName = NormalizeSecretName(secretName, authType)

	selected = normalizeWorkflows(selected)
	files := make([]WorkflowFile, 0, len(selected))
	for _, workflow := range selected {
		switch workflow {
		case WorkflowClaude:
			files = append(files, WorkflowFile{
				Workflow: WorkflowClaude,
				Path:     ".github/workflows/claude.yml",
				Content:  workflowWithSecret(claudeWorkflowContent, secretName, authType),
			})
		case WorkflowClaudeReview:
			files = append(files, WorkflowFile{
				Workflow: WorkflowClaudeReview,
				Path:     ".github/workflows/claude-code-review.yml",
				Content:  workflowWithSecret(claudeReviewWorkflowContent, secretName, authType),
			})
		}
	}
	return files
}

func NormalizeSecretName(secretName string, authType AuthType) string {
	secretName = strings.TrimSpace(secretName)
	if secretName == "" {
		if authType == AuthTypeOAuthToken {
			return DefaultOAuthSecret
		}
		return DefaultAPIKeySecret
	}

	secretName = strings.ToUpper(secretName)
	var builder strings.Builder
	lastUnderscore := false
	for _, r := range secretName {
		switch {
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			builder.WriteRune(r)
			lastUnderscore = false
		case r == '_', r == '-', r == '.', r == ' ':
			if !lastUnderscore {
				builder.WriteByte('_')
				lastUnderscore = true
			}
		default:
			if !lastUnderscore {
				builder.WriteByte('_')
				lastUnderscore = true
			}
		}
	}

	normalized := strings.Trim(builder.String(), "_")
	if normalized == "" {
		if authType == AuthTypeOAuthToken {
			return DefaultOAuthSecret
		}
		return DefaultAPIKeySecret
	}
	if normalized[0] >= '0' && normalized[0] <= '9' {
		normalized = "SECRET_" + normalized
	}
	if strings.HasPrefix(normalized, "GITHUB_") {
		normalized = "AWAKEN_" + normalized
	}
	return normalized
}

func NormalizeRepoName(repo string) (string, bool) {
	repo = strings.TrimSpace(repo)
	repo = strings.TrimSuffix(repo, ".git")
	if repo == "" {
		return "", false
	}
	if strings.HasPrefix(repo, "git@github.com:") {
		repo = strings.TrimPrefix(repo, "git@github.com:")
		parts := strings.Split(repo, "/")
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return "", false
		}
		return parts[0] + "/" + strings.TrimSuffix(parts[1], ".git"), true
	}
	if strings.Contains(repo, "github.com") {
		u, err := url.Parse(repo)
		if err != nil || u.Host != "github.com" {
			return "", false
		}
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return "", false
		}
		return parts[0] + "/" + strings.TrimSuffix(parts[1], ".git"), true
	}
	parts := strings.Split(repo, "/")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return repo, false
	}
	return parts[0] + "/" + strings.TrimSuffix(parts[1], ".git"), true
}

func buildActions(input Input, repo RepoState, files []WorkflowFile, secretName string, branchName string) []Action {
	repoName := repo.Name
	defaultBranch := repo.DefaultBranch
	if strings.TrimSpace(defaultBranch) == "" {
		defaultBranch = "<default-branch>"
	}
	defaultBranchSHA := repo.DefaultBranchSHA
	if strings.TrimSpace(defaultBranchSHA) == "" {
		defaultBranchSHA = "<default-branch-sha>"
	}

	actions := []Action{
		{
			Kind:   ActionCheckGitHubCLI,
			Title:  "Check GitHub CLI",
			Detail: "Verify gh is installed, authenticated, and has repo/workflow scopes.",
			Commands: []CommandPattern{
				{Program: "gh", Args: []string{"--version"}},
				{Program: "gh", Args: []string{"auth", "status", "-a"}},
			},
		},
	}

	if strings.TrimSpace(input.Repo.Name) == "" && strings.TrimSpace(input.CurrentRepo) != "" {
		actions = append(actions, Action{
			Kind:   ActionUseCurrentRepo,
			Title:  "Use current repository",
			Detail: "Use the GitHub repository detected from the local checkout.",
		})
	}

	actions = append(actions,
		Action{
			Kind:   ActionCheckRepo,
			Title:  "Check repository access",
			Detail: "Confirm the repository exists, read its default branch, and verify admin permissions when available.",
			Commands: []CommandPattern{
				{Program: "gh", Args: []string{"api", "repos/" + repoName, "--jq", ".id"}},
				{Program: "gh", Args: []string{"api", "repos/" + repoName, "--jq", ".default_branch"}},
				{Program: "gh", Args: []string{"api", "repos/" + repoName + "/git/ref/heads/" + defaultBranch, "--jq", ".object.sha"}},
				{Program: "gh", Args: []string{"api", "repos/" + repoName, "--jq", ".permissions.admin"}},
			},
		},
		Action{
			Kind:   ActionInstallApp,
			Title:  "Open Claude GitHub App installation",
			Detail: "Install or confirm the Claude app has access to the selected repository.",
			URL:    InstallAppURL,
			ManualFallback: []string{
				"Open " + InstallAppURL + " if automatic browser launch is unavailable.",
				"Use " + SetupDocsURL + " for manual workflow setup.",
			},
		},
	)

	if !input.SkipWorkflowFiles {
		actions = append(actions, Action{
			Kind:   ActionCreateBranch,
			Title:  "Create setup branch",
			Detail: "Create a branch from the repository default branch for the workflow changes.",
			Commands: []CommandPattern{{
				Program: "gh",
				Args: []string{
					"api", "--method", "POST", "repos/" + repoName + "/git/refs",
					"-f", "ref=refs/heads/" + branchName,
					"-f", "sha=" + defaultBranchSHA,
				},
			}},
		})

		for _, file := range files {
			actions = append(actions, Action{
				Kind:   ActionPutWorkflow,
				Title:  "Put workflow file",
				Detail: "Create " + file.Path + " on the setup branch.",
				Path:   file.Path,
				Commands: []CommandPattern{{
					Program: "gh",
					Args: []string{
						"api", "--method", "PUT", "repos/" + repoName + "/contents/" + file.Path,
						"-f", "message=" + workflowMessage(file.Workflow),
						"-f", "content=" + WorkflowPlaceholder,
						"-f", "branch=" + branchName,
					},
				}},
				ManualFallback: []string{
					"If the file already exists, merge the template manually instead of overwriting it.",
					"Manual setup guide: " + SetupDocsURL,
				},
			})
		}
	}

	if !input.UseExistingSecret {
		detail := "Set the GitHub Actions secret with a redacted placeholder body."
		if input.SecretValue == "" {
			detail = "Set the GitHub Actions secret after collecting the secret value; the plan keeps the body redacted."
		}
		actions = append(actions, Action{
			Kind:   ActionSetSecret,
			Title:  "Set GitHub Actions secret",
			Detail: detail,
			Commands: []CommandPattern{{
				Program:          "gh",
				Args:             []string{"secret", "set", secretName, "--body", SecretPlaceholder, "--repo", repoName},
				StdinPlaceholder: SecretPlaceholder,
			}},
		})
	}

	if !input.SkipWorkflowFiles {
		compareURL := pullRequestURL(repoName, defaultBranch, branchName)
		actions = append(actions, Action{
			Kind:   ActionOpenPR,
			Title:  "Open pull request",
			Detail: "Open GitHub's compare page to create the setup pull request.",
			URL:    compareURL,
			Commands: []CommandPattern{{
				Program: "open",
				Args:    []string{compareURL},
			}},
			ManualFallback: []string{
				"Open the compare URL manually if browser launch fails.",
				"Manual setup guide: " + SetupDocsURL,
			},
		})
	}

	return actions
}

func workflowConflictWarnings(repo RepoState, selected []Workflow) []Warning {
	existing := make(map[string]bool, len(repo.ExistingWorkflowFiles))
	for _, path := range repo.ExistingWorkflowFiles {
		existing[strings.TrimSpace(path)] = true
	}

	var warnings []Warning
	for _, file := range BuildWorkflowFiles(selected, DefaultAPIKeySecret, AuthTypeAPIKey) {
		if existing[file.Path] {
			warnings = append(warnings, Warning{
				Code:     "existing_workflow_conflict",
				Title:    "Workflow file already exists",
				Message:  file.Path + " already exists in the repository.",
				Blocking: true,
				Instructions: []string{
					"Delete or rename the existing workflow file before automated setup.",
					"Or merge the Claude Code workflow template manually.",
					"Manual setup guide: " + SetupDocsURL,
				},
			})
		}
	}
	return warnings
}

func existingSecretWarning(repo RepoState, secretName string, useExisting bool) []Warning {
	if !containsString(repo.ExistingActionSecretIDs, secretName) {
		return nil
	}
	message := "GitHub Actions secret " + secretName + " already exists."
	if useExisting {
		message += " The plan will reuse it and skip setting a new secret."
	} else {
		message += " The plan will update it with a redacted secret body placeholder."
	}
	return []Warning{{
		Code:    "existing_secret",
		Title:   "GitHub Actions secret already exists",
		Message: message,
		Instructions: []string{
			"Confirm whether to reuse the existing secret or replace it before executing the plan.",
		},
	}}
}

func blockingErrors(warnings []Warning) []BlockingError {
	errors := make([]BlockingError, 0)
	seen := map[string]bool{}
	for _, warning := range warnings {
		if !warning.Blocking || seen[warning.Code] {
			continue
		}
		seen[warning.Code] = true
		errors = append(errors, BlockingError{
			Code:         warning.Code,
			Title:        warning.Title,
			Message:      warning.Message,
			Instructions: warning.Instructions,
			Commands:     warning.Commands,
		})
	}
	return errors
}

func parseTokenScopes(output string) (map[string]bool, bool) {
	re := regexp.MustCompile(`(?im)Token scopes:\s*(.+)$`)
	match := re.FindStringSubmatch(output)
	if len(match) != 2 {
		return nil, false
	}

	scopes := map[string]bool{}
	for _, raw := range strings.Split(match[1], ",") {
		scope := strings.Trim(raw, " \t\r\n'\"")
		if scope != "" {
			scopes[scope] = true
		}
	}
	return scopes, true
}

func missingScopes(scopes map[string]bool, required []string) []string {
	missing := make([]string, 0)
	for _, requiredScope := range required {
		if !scopes[requiredScope] {
			missing = append(missing, requiredScope)
		}
	}
	return missing
}

func refreshScopesCommand() CommandPattern {
	return CommandPattern{Program: "gh", Args: []string{"auth", "refresh", "-h", "github.com", "-s", "repo,workflow"}}
}

func workflowWithSecret(content, secretName string, authType AuthType) string {
	apiKeyLine := "anthropic_api_key: ${{ secrets.ANTHROPIC_API_KEY }}"
	if authType == AuthTypeOAuthToken {
		return strings.ReplaceAll(content, apiKeyLine, "claude_code_oauth_token: ${{ secrets."+secretName+" }}")
	}
	return strings.ReplaceAll(content, apiKeyLine, "anthropic_api_key: ${{ secrets."+secretName+" }}")
}

func normalizeWorkflows(selected []Workflow) []Workflow {
	if len(selected) == 0 {
		return []Workflow{WorkflowClaude, WorkflowClaudeReview}
	}
	seen := map[Workflow]bool{}
	normalized := make([]Workflow, 0, len(selected))
	for _, workflow := range selected {
		if workflow != WorkflowClaude && workflow != WorkflowClaudeReview {
			continue
		}
		if seen[workflow] {
			continue
		}
		seen[workflow] = true
		normalized = append(normalized, workflow)
	}
	sort.SliceStable(normalized, func(i, j int) bool {
		order := map[Workflow]int{WorkflowClaude: 0, WorkflowClaudeReview: 1}
		return order[normalized[i]] < order[normalized[j]]
	})
	if len(normalized) == 0 {
		return []Workflow{WorkflowClaude, WorkflowClaudeReview}
	}
	return normalized
}

func workflowMessage(workflow Workflow) string {
	switch workflow {
	case WorkflowClaudeReview:
		return "Claude Code Review workflow"
	default:
		return "Claude PR Assistant workflow"
	}
}

func pullRequestURL(repoName, defaultBranch, branchName string) string {
	values := url.Values{}
	values.Set("quick_pull", "1")
	values.Set("title", PRTitle)
	values.Set("body", prBody)
	return "https://github.com/" + repoName + "/compare/" + defaultBranch + "..." + branchName + "?" + values.Encode()
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

const claudeWorkflowContent = `name: Claude Code

on:
  issue_comment:
    types: [created]
  pull_request_review_comment:
    types: [created]
  issues:
    types: [opened, assigned]
  pull_request_review:
    types: [submitted]

jobs:
  claude:
    if: |
      (github.event_name == 'issue_comment' && contains(github.event.comment.body, '@claude')) ||
      (github.event_name == 'pull_request_review_comment' && contains(github.event.comment.body, '@claude')) ||
      (github.event_name == 'pull_request_review' && contains(github.event.review.body, '@claude')) ||
      (github.event_name == 'issues' && (contains(github.event.issue.body, '@claude') || contains(github.event.issue.title, '@claude')))
    runs-on: ubuntu-latest
    permissions:
      contents: read
      pull-requests: read
      issues: read
      id-token: write
      actions: read
    steps:
      - name: Checkout repository
        uses: actions/checkout@v4
        with:
          fetch-depth: 1

      - name: Run Claude Code
        id: claude
        uses: anthropics/claude-code-action@v1
        with:
          anthropic_api_key: ${{ secrets.ANTHROPIC_API_KEY }}

          additional_permissions: |
            actions: read

          # Optional: Give a custom prompt to Claude.
          # prompt: 'Update the pull request description to include a summary of changes.'

          # Optional: Add claude_args to customize behavior and configuration.
          # claude_args: '--allowed-tools Bash(gh pr:*)'
`

const claudeReviewWorkflowContent = `name: Claude Code Review

on:
  pull_request:
    types: [opened, synchronize, ready_for_review, reopened]

jobs:
  claude-review:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      pull-requests: read
      issues: read
      id-token: write

    steps:
      - name: Checkout repository
        uses: actions/checkout@v4
        with:
          fetch-depth: 1

      - name: Run Claude Code Review
        id: claude-review
        uses: anthropics/claude-code-action@v1
        with:
          anthropic_api_key: ${{ secrets.ANTHROPIC_API_KEY }}
          plugin_marketplaces: 'https://github.com/anthropics/claude-code.git'
          plugins: 'code-review@claude-code-plugins'
          prompt: '/code-review:code-review ${{ github.repository }}/pull/${{ github.event.pull_request.number }}'
`

const prBody = `## Installing Claude Code GitHub App

This PR adds a GitHub Actions workflow that enables Claude Code integration in this repository.

Once this PR is merged, users can mention @claude in a pull request or issue comment to trigger the workflow.

Security notes:
- The API key or OAuth token is stored as a GitHub Actions secret.
- The workflow does not take effect until the PR is merged.
- Use the Claude Code action documentation to tune allowed tools and permissions.
`
