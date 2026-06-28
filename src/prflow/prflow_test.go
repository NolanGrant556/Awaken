package prflow

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestPlanCreatesBranchOnDefaultBranch(t *testing.T) {
	plan := Plan(Input{
		Snapshot: GitSnapshot{
			DefaultBranch: "main",
			CurrentBranch: "main",
			SafeUser:      "nolan",
		},
		Options: Options{
			ProposedTitle: "Add GitHub PR flow",
		},
	})

	if !plan.Branch.CreateBranch {
		t.Fatalf("expected branch creation on default branch")
	}
	if plan.Branch.TargetBranch != "nolan/add-github-pr-flow" {
		t.Fatalf("unexpected branch name: %q", plan.Branch.TargetBranch)
	}
	if len(plan.Actions) == 0 || plan.Actions[0].Kind != ActionCreateBranch {
		t.Fatalf("expected create branch to be the first action, got %#v", plan.Actions)
	}
}

func TestPlanDoesNotCreateBranchOnFeatureBranch(t *testing.T) {
	plan := Plan(Input{
		Snapshot: GitSnapshot{
			DefaultBranch: "main",
			CurrentBranch: "nolan/github-pr-flow",
		},
		Options: Options{
			ProposedTitle: "Add GitHub PR flow",
		},
	})

	if plan.Branch.CreateBranch {
		t.Fatalf("did not expect branch creation on feature branch")
	}
	if plan.Branch.TargetBranch != "nolan/github-pr-flow" {
		t.Fatalf("expected push target to stay on feature branch, got %q", plan.Branch.TargetBranch)
	}
	if containsAction(plan.Actions, ActionCreateBranch) {
		t.Fatalf("did not expect a create branch action: %#v", plan.Actions)
	}
}

func TestPlanEditsExistingPR(t *testing.T) {
	plan := Plan(Input{
		Snapshot: GitSnapshot{
			DefaultBranch:    "main",
			CurrentBranch:    "nolan/github-pr-flow",
			ExistingPRNumber: 42,
		},
		Options: Options{
			ProposedTitle: "Add GitHub PR flow",
		},
	})

	if plan.PullRequest.Mode != PRModeEdit {
		t.Fatalf("expected edit mode, got %q", plan.PullRequest.Mode)
	}
	last := plan.Actions[len(plan.Actions)-1]
	if last.Kind != ActionEditPR {
		t.Fatalf("expected final action to edit PR, got %q", last.Kind)
	}
	if !strings.Contains(last.Command, "gh pr edit 42") {
		t.Fatalf("expected edit command to target PR 42, got %q", last.Command)
	}
}

func TestPlanCreatesNewPR(t *testing.T) {
	plan := Plan(Input{
		Snapshot: GitSnapshot{
			DefaultBranch: "main",
			CurrentBranch: "nolan/github-pr-flow",
		},
		Options: Options{
			ProposedTitle: "Add GitHub PR flow",
		},
	})

	if plan.PullRequest.Mode != PRModeCreate {
		t.Fatalf("expected create mode, got %q", plan.PullRequest.Mode)
	}
	last := plan.Actions[len(plan.Actions)-1]
	if last.Kind != ActionCreatePR {
		t.Fatalf("expected final action to create PR, got %q", last.Kind)
	}
	for _, section := range []string{"## Summary", "## Test Plan", "## Changelog", "## Attribution"} {
		if !strings.Contains(plan.PullRequest.Body, section) {
			t.Fatalf("expected PR body to contain %q, body:\n%s", section, plan.PullRequest.Body)
		}
	}
}

func TestPlanWarnsAndExcludesSecretLikePaths(t *testing.T) {
	plan := Plan(Input{
		Snapshot: GitSnapshot{
			DefaultBranch: "main",
			CurrentBranch: "nolan/github-pr-flow",
			Status:        " M .env\nA  config/credentials.json\n?? src/main.go\n",
			DiffHead:      "diff --git a/secrets/token.txt b/secrets/token.txt\n+++ b/secrets/token.txt\n",
		},
		Options: Options{
			ProposedTitle: "Add GitHub PR flow",
		},
	})

	want := []string{".env", "config/credentials.json", "secrets/token.txt"}
	if strings.Join(plan.ExcludedPaths, ",") != strings.Join(want, ",") {
		t.Fatalf("unexpected excluded paths: got %#v want %#v", plan.ExcludedPaths, want)
	}
	if !hasWarning(plan.SafetyWarnings, "secret-like-paths-excluded") {
		t.Fatalf("expected secret-like path warning, got %#v", plan.SafetyWarnings)
	}

	stage := actionByKind(plan.Actions, ActionStage)
	if stage == nil {
		t.Fatalf("expected stage action")
	}
	if strings.Join(stage.ExcludePaths, ",") != strings.Join(want, ",") {
		t.Fatalf("unexpected stage exclusions: got %#v want %#v", stage.ExcludePaths, want)
	}
	if !strings.Contains(stage.Command, "':!.env'") {
		t.Fatalf("expected git add command to exclude .env, got %q", stage.Command)
	}
}

func TestPlanAllowedToolsList(t *testing.T) {
	plan := Plan(Input{
		Snapshot: GitSnapshot{
			DefaultBranch: "main",
			CurrentBranch: "nolan/github-pr-flow",
		},
	})

	for _, pattern := range []string{"Bash(git push:*)", "Bash(gh pr create:*)", "Bash(gh pr edit:*)", "ToolSearch"} {
		if !containsString(plan.AllowedToolPatterns, pattern) {
			t.Fatalf("expected allowed tools to contain %q, got %#v", pattern, plan.AllowedToolPatterns)
		}
	}
	if containsString(plan.AllowedToolPatterns, "Bash(git config:*)") {
		t.Fatalf("git config must not be allowed")
	}
	for _, pattern := range []string{"Bash(git config:*)", "Bash(git push --force:*)", "Bash(git commit --no-verify:*)"} {
		if !containsString(plan.DisallowedToolPatterns, pattern) {
			t.Fatalf("expected disallowed tools to contain %q, got %#v", pattern, plan.DisallowedToolPatterns)
		}
	}
}

func TestPlanTruncatesLongPRTitle(t *testing.T) {
	plan := Plan(Input{
		Snapshot: GitSnapshot{
			DefaultBranch: "main",
			CurrentBranch: "nolan/github-pr-flow",
		},
		Options: Options{
			ProposedTitle: strings.Repeat("long-title-", 12),
		},
	})

	if utf8.RuneCountInString(plan.PullRequest.Title) > 70 {
		t.Fatalf("expected title to be at most 70 runes, got %d: %q", utf8.RuneCountInString(plan.PullRequest.Title), plan.PullRequest.Title)
	}
	if !hasWarning(plan.SafetyWarnings, "pr-title-truncated") {
		t.Fatalf("expected title truncation warning, got %#v", plan.SafetyWarnings)
	}
}

func TestPlanDerivesUnicodeSafeTitleFromBranch(t *testing.T) {
	plan := Plan(Input{
		Snapshot: GitSnapshot{
			DefaultBranch: "main",
			CurrentBranch: "nolan/功能-测试",
		},
	})

	if !utf8.ValidString(plan.PullRequest.Title) {
		t.Fatalf("expected derived title to be valid UTF-8, got %q", plan.PullRequest.Title)
	}
	if plan.PullRequest.Title != "功能 测试" {
		t.Fatalf("unexpected derived title: %q", plan.PullRequest.Title)
	}
	if containsString(plan.AllowedToolPatterns, "Bash(gh pr merge:*)") {
		t.Fatalf("PR flow planning should not allow merge commands")
	}
}

func TestPlanAppendsUserInstructions(t *testing.T) {
	plan := Plan(Input{
		Snapshot: GitSnapshot{
			DefaultBranch: "main",
			CurrentBranch: "nolan/github-pr-flow",
		},
		Options: Options{
			ProposedTitle:    "Add GitHub PR flow",
			UserInstructions: "Mention that the package is planning-only.",
		},
	})

	if !strings.Contains(plan.PullRequest.Body, "## Additional Instructions") {
		t.Fatalf("expected additional instructions section, body:\n%s", plan.PullRequest.Body)
	}
	if !strings.Contains(plan.PullRequest.Body, "Mention that the package is planning-only.") {
		t.Fatalf("expected user instructions to be appended, body:\n%s", plan.PullRequest.Body)
	}
	if plan.AdditionalInstructions != "Mention that the package is planning-only." {
		t.Fatalf("unexpected additional instructions: %q", plan.AdditionalInstructions)
	}
}

func containsAction(actions []Action, kind ActionKind) bool {
	return actionByKind(actions, kind) != nil
}

func actionByKind(actions []Action, kind ActionKind) *Action {
	for i := range actions {
		if actions[i].Kind == kind {
			return &actions[i]
		}
	}
	return nil
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func hasWarning(warnings []SafetyWarning, id string) bool {
	for _, warning := range warnings {
		if warning.ID == id {
			return true
		}
	}
	return false
}
