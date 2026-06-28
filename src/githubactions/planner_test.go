package githubactions

import (
	"fmt"
	"strings"
	"testing"
)

func TestCheckGitHubCLIMissingGHWarning(t *testing.T) {
	warnings := CheckGitHubCLI("Token scopes: repo, workflow", "")
	warning := findWarning(warnings, "missing_gh")
	if warning == nil {
		t.Fatalf("expected missing gh warning, got %+v", warnings)
	}
	if warning.Blocking {
		t.Fatalf("expected missing gh to be a warning, got blocking")
	}
}

func TestPlanMissingScopesBlockingError(t *testing.T) {
	result := Plan(validInput(func(input *Input) {
		input.GitHubAuthOutput = "Token scopes: repo"
	}))

	err := findBlockingError(result.BlockingErrors, "missing_scopes")
	if err == nil {
		t.Fatalf("expected missing scopes blocking error, got %+v", result.BlockingErrors)
	}
	if !commandContains(err.Commands, "gh auth refresh -h github.com -s repo,workflow") {
		t.Fatalf("expected gh auth refresh command, got %+v", err.Commands)
	}
}

func TestPlanExistingWorkflowConflict(t *testing.T) {
	result := Plan(validInput(func(input *Input) {
		input.SelectedWorkflows = []Workflow{WorkflowClaude}
		input.Repo.ExistingWorkflowFiles = []string{".github/workflows/claude.yml"}
	}))

	err := findBlockingError(result.BlockingErrors, "existing_workflow_conflict")
	if err == nil {
		t.Fatalf("expected workflow conflict blocking error, got %+v", result.BlockingErrors)
	}
	if !strings.Contains(strings.Join(err.Instructions, "\n"), SetupDocsURL) {
		t.Fatalf("expected manual setup docs in instructions, got %+v", err.Instructions)
	}
}

func TestBuildWorkflowFilesAPIKeySecretReplacement(t *testing.T) {
	files := BuildWorkflowFiles([]Workflow{WorkflowClaude}, "AWAKEN_ANTHROPIC_KEY", AuthTypeAPIKey)
	if len(files) != 1 {
		t.Fatalf("expected one workflow file, got %d", len(files))
	}
	if !strings.Contains(files[0].Content, "anthropic_api_key: ${{ secrets.AWAKEN_ANTHROPIC_KEY }}") {
		t.Fatalf("expected custom api key secret in workflow:\n%s", files[0].Content)
	}
	if strings.Contains(files[0].Content, "secrets."+DefaultAPIKeySecret) {
		t.Fatalf("expected default API key secret to be replaced:\n%s", files[0].Content)
	}
}

func TestBuildWorkflowFilesOAuthTokenReplacement(t *testing.T) {
	files := BuildWorkflowFiles([]Workflow{WorkflowClaude}, DefaultOAuthSecret, AuthTypeOAuthToken)
	if len(files) != 1 {
		t.Fatalf("expected one workflow file, got %d", len(files))
	}
	if !strings.Contains(files[0].Content, "claude_code_oauth_token: ${{ secrets.CLAUDE_CODE_OAUTH_TOKEN }}") {
		t.Fatalf("expected oauth token secret in workflow:\n%s", files[0].Content)
	}
	if strings.Contains(files[0].Content, "anthropic_api_key:") {
		t.Fatalf("expected anthropic_api_key input to be replaced for oauth:\n%s", files[0].Content)
	}
}

func TestSecretNameIsNormalizedBeforeRendering(t *testing.T) {
	files := BuildWorkflowFiles([]Workflow{WorkflowClaude}, " github-token: ${{ secrets.BAD }}\n", AuthTypeAPIKey)
	if len(files) != 1 {
		t.Fatalf("expected one workflow file, got %d", len(files))
	}
	if strings.Contains(files[0].Content, "${{ secrets.BAD }}") || strings.Contains(files[0].Content, "github-token:") {
		t.Fatalf("workflow content used unsanitized secret name:\n%s", files[0].Content)
	}
	if !strings.Contains(files[0].Content, "anthropic_api_key: ${{ secrets.AWAKEN_GITHUB_TOKEN_SECRETS_BAD }}") {
		t.Fatalf("workflow content did not contain normalized secret name:\n%s", files[0].Content)
	}

	result := Plan(validInput(func(input *Input) {
		input.SecretName = "9 bad-name"
		input.SecretValue = "sk-ant-real-secret-never-output"
	}))
	if result.SecretName != "SECRET_9_BAD_NAME" {
		t.Fatalf("unexpected normalized secret name: %q", result.SecretName)
	}
	if rendered := fmt.Sprintf("%#v", result); strings.Contains(rendered, "sk-ant-real-secret-never-output") {
		t.Fatalf("plan leaked secret value: %s", rendered)
	}
}

func TestBuildWorkflowFilesSelectedWorkflowsCreatesTwoFiles(t *testing.T) {
	files := BuildWorkflowFiles([]Workflow{WorkflowClaudeReview, WorkflowClaude}, DefaultAPIKeySecret, AuthTypeAPIKey)
	if len(files) != 2 {
		t.Fatalf("expected two workflow files, got %d", len(files))
	}
	if files[0].Path != ".github/workflows/claude.yml" {
		t.Fatalf("expected claude workflow first, got %s", files[0].Path)
	}
	if files[1].Path != ".github/workflows/claude-code-review.yml" {
		t.Fatalf("expected review workflow second, got %s", files[1].Path)
	}
}

func TestPlanSecretValueRedactedPlaceholder(t *testing.T) {
	result := Plan(validInput(func(input *Input) {
		input.SecretValue = "sk-ant-real-secret-never-output"
	}))

	rendered := fmt.Sprintf("%#v", result)
	if strings.Contains(rendered, "sk-ant-real-secret-never-output") {
		t.Fatalf("plan leaked the secret value: %s", rendered)
	}
	if !strings.Contains(rendered, SecretPlaceholder) {
		t.Fatalf("expected secret placeholder in plan, got %s", rendered)
	}
}

func TestPlanOrderedActionsIncludeBranchFilesSecretPR(t *testing.T) {
	result := Plan(validInput(func(input *Input) {
		input.SecretValue = "sk-ant-real-secret-never-output"
	}))
	if len(result.BlockingErrors) > 0 {
		t.Fatalf("expected no blocking errors, got %+v", result.BlockingErrors)
	}

	branchAt := firstActionIndex(result.Actions, ActionCreateBranch)
	fileAt := firstActionIndex(result.Actions, ActionPutWorkflow)
	secretAt := firstActionIndex(result.Actions, ActionSetSecret)
	prAt := firstActionIndex(result.Actions, ActionOpenPR)
	if branchAt < 0 || fileAt < 0 || secretAt < 0 || prAt < 0 {
		t.Fatalf("expected branch, workflow file, secret, and PR actions, got %+v", actionKinds(result.Actions))
	}
	if !(branchAt < fileAt && fileAt < secretAt && secretAt < prAt) {
		t.Fatalf("unexpected action order: %+v", actionKinds(result.Actions))
	}

	fileActions := 0
	for _, action := range result.Actions {
		if action.Kind == ActionPutWorkflow {
			fileActions++
		}
	}
	if fileActions != 2 {
		t.Fatalf("expected two workflow file actions, got %d", fileActions)
	}
}

func TestValidateRepoAdminMissingWarning(t *testing.T) {
	warnings := ValidateRepo(RepoState{
		Name:                 "owner/repo",
		AdminPermissionKnown: true,
		HasAdminPermission:   false,
	})
	if findWarning(warnings, "repo_admin_missing") == nil {
		t.Fatalf("expected repo admin warning, got %+v", warnings)
	}
}

func validInput(mutators ...func(*Input)) Input {
	input := Input{
		GitHubVersionOutput: "gh version 2.0.0",
		GitHubAuthOutput:    "Token scopes: repo, workflow",
		Repo: RepoState{
			Name:                 "owner/repo",
			ExistsKnown:          true,
			Exists:               true,
			AdminPermissionKnown: true,
			HasAdminPermission:   true,
			DefaultBranch:        "main",
			DefaultBranchSHA:     "abc123",
		},
		SelectedWorkflows: []Workflow{WorkflowClaude, WorkflowClaudeReview},
		SecretName:        DefaultAPIKeySecret,
		AuthType:          AuthTypeAPIKey,
	}
	for _, mutator := range mutators {
		mutator(&input)
	}
	return input
}

func findWarning(warnings []Warning, code string) *Warning {
	for i := range warnings {
		if warnings[i].Code == code {
			return &warnings[i]
		}
	}
	return nil
}

func findBlockingError(errors []BlockingError, code string) *BlockingError {
	for i := range errors {
		if errors[i].Code == code {
			return &errors[i]
		}
	}
	return nil
}

func commandContains(commands []CommandPattern, want string) bool {
	for _, command := range commands {
		if strings.Join(append([]string{command.Program}, command.Args...), " ") == want {
			return true
		}
	}
	return false
}

func firstActionIndex(actions []Action, kind ActionKind) int {
	for i, action := range actions {
		if action.Kind == kind {
			return i
		}
	}
	return -1
}

func actionKinds(actions []Action) []ActionKind {
	kinds := make([]ActionKind, 0, len(actions))
	for _, action := range actions {
		kinds = append(kinds, action.Kind)
	}
	return kinds
}
