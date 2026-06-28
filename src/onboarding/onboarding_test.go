package onboarding

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInspectWorkspaceEmptyDirectory(t *testing.T) {
	root := t.TempDir()

	state := InspectWorkspace(root)

	if state.Complete {
		t.Fatal("expected empty workspace to be incomplete")
	}
	if !state.WorkspaceExists || !state.WorkspaceReadable || !state.WorkspaceEmpty {
		t.Fatalf("unexpected workspace state: %+v", state)
	}
	if !stepByKey(t, state, StepWorkspace).Enabled || stepByKey(t, state, StepWorkspace).Complete {
		t.Fatalf("expected workspace step to be enabled and incomplete: %+v", state.Steps)
	}
	if stepByKey(t, state, StepInstructions).Enabled {
		t.Fatalf("expected instructions step to be disabled for empty workspace: %+v", state.Steps)
	}
	if !strings.Contains(state.Summary(), "workspace is empty") {
		t.Fatalf("expected empty workspace summary, got %q", state.Summary())
	}
}

func TestInspectWorkspaceCompleteProject(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "README.md"))
	writeFile(t, filepath.Join(root, "AGENTS.md"))
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatalf("failed to create .git: %v", err)
	}

	state := InspectWorkspace(root)

	if !state.Complete {
		t.Fatalf("expected project onboarding to be complete: %+v", state)
	}
	if got := strings.Join(state.InstructionFiles, ","); got != "AGENTS.md" {
		t.Fatalf("unexpected instruction files: %q", got)
	}
	if got := strings.Join(state.ProjectSignals, ","); got != "README.md,.git" {
		t.Fatalf("unexpected project signals: %q", got)
	}
	if !strings.Contains(state.Summary(), "complete") {
		t.Fatalf("expected complete summary, got %q", state.Summary())
	}
}

func TestInspectWorkspaceMissingInstructions(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "README.md"))
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatalf("failed to create .git: %v", err)
	}

	state := InspectWorkspace(root)

	if state.Complete {
		t.Fatal("expected missing instructions to be incomplete")
	}
	instructions := stepByKey(t, state, StepInstructions)
	if !instructions.Enabled || instructions.Complete {
		t.Fatalf("expected instructions step to be enabled and incomplete: %+v", instructions)
	}
	if !strings.Contains(state.Summary(), "missing AGENTS.md/CLAUDE.md") {
		t.Fatalf("expected missing instructions summary, got %q", state.Summary())
	}
}

func TestInspectWorkspaceMissingPath(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing")

	state := InspectWorkspace(root)

	if state.Complete {
		t.Fatal("expected missing workspace to be incomplete")
	}
	if state.WorkspaceExists || state.WorkspaceReadable {
		t.Fatalf("unexpected workspace state for missing path: %+v", state)
	}
	if !strings.Contains(state.Summary(), "workspace root does not exist") {
		t.Fatalf("expected missing workspace summary, got %q", state.Summary())
	}
}

func TestInspectWorkspaceUnreadablePath(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatalf("failed to create workspace: %v", err)
	}
	if err := os.Chmod(root, 0); err != nil {
		t.Fatalf("failed to make workspace unreadable: %v", err)
	}
	defer func() {
		if err := os.Chmod(root, 0o755); err != nil {
			t.Fatalf("failed to restore workspace permissions: %v", err)
		}
	}()

	state := InspectWorkspace(root)
	if state.WorkspaceReadable {
		t.Skip("workspace remained readable after chmod")
	}
	if state.Complete {
		t.Fatal("expected unreadable workspace to be incomplete")
	}
	if !state.WorkspaceExists {
		t.Fatalf("expected unreadable workspace to exist: %+v", state)
	}
	if !strings.Contains(state.Summary(), "not readable") {
		t.Fatalf("expected unreadable workspace summary, got %q", state.Summary())
	}
}

func TestInspectWorkspaceAcceptsClaudeInstructions(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "CLAUDE.md"))
	writeFile(t, filepath.Join(root, "README.md"))

	state := InspectWorkspace(root)

	if !state.Complete {
		t.Fatalf("expected CLAUDE.md instructions to complete onboarding: %+v", state)
	}
	if got := strings.Join(state.InstructionFiles, ","); got != "CLAUDE.md" {
		t.Fatalf("unexpected instruction files: %q", got)
	}
}

func stepByKey(t *testing.T, state State, key string) Step {
	t.Helper()
	for _, step := range state.Steps {
		if step.Key == key {
			return step
		}
	}
	t.Fatalf("missing step %q in %+v", key, state.Steps)
	return Step{}
}

func writeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("test"), 0o644); err != nil {
		t.Fatalf("failed to write %s: %v", path, err)
	}
}
