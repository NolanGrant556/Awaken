package onboarding

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	StepWorkspace     = "workspace"
	StepInstructions  = "instructions"
	StepProjectSignal = "project_signal"
)

var instructionCandidates = []string{"AGENTS.md", "CLAUDE.md"}

type Step struct {
	Key         string
	Text        string
	Complete    bool
	Completable bool
	Enabled     bool
}

type State struct {
	WorkspaceRoot     string
	WorkspaceExists   bool
	WorkspaceReadable bool
	WorkspaceEmpty    bool
	InstructionFiles  []string
	ProjectSignals    []string
	Steps             []Step
	Complete          bool
	Problem           string
}

func InspectWorkspace(root string) State {
	state := State{
		WorkspaceRoot: cleanRoot(root),
	}

	info, err := os.Stat(state.WorkspaceRoot)
	if err != nil {
		state.Problem = workspaceStatProblem(err)
		state.Steps = buildSteps(state)
		state.Complete = completeSteps(state.Steps)
		return state
	}
	if !info.IsDir() {
		state.WorkspaceExists = true
		state.Problem = "workspace root is not a directory"
		state.Steps = buildSteps(state)
		state.Complete = completeSteps(state.Steps)
		return state
	}

	state.WorkspaceExists = true
	entries, err := os.ReadDir(state.WorkspaceRoot)
	if err != nil {
		state.Problem = "workspace root is not readable"
		state.Steps = buildSteps(state)
		state.Complete = completeSteps(state.Steps)
		return state
	}

	state.WorkspaceReadable = true
	state.WorkspaceEmpty = len(entries) == 0
	state.InstructionFiles = existingFiles(state.WorkspaceRoot, instructionCandidates)
	state.ProjectSignals = existingProjectSignals(state.WorkspaceRoot)
	state.Steps = buildSteps(state)
	state.Complete = completeSteps(state.Steps)
	return state
}

func (s State) Summary() string {
	if s.Complete {
		return fmt.Sprintf(
			"complete; instructions=%s; project signals=%s",
			joinOrNone(s.InstructionFiles),
			joinOrNone(s.ProjectSignals),
		)
	}

	if !s.WorkspaceExists {
		return "incomplete; workspace root does not exist"
	}
	if !s.WorkspaceReadable {
		if s.Problem != "" {
			return "incomplete; " + s.Problem
		}
		return "incomplete; workspace root is not readable"
	}
	if s.WorkspaceEmpty {
		return "incomplete; workspace is empty; create or clone a project, then add AGENTS.md or CLAUDE.md"
	}

	missing := make([]string, 0, 2)
	if len(s.InstructionFiles) == 0 {
		missing = append(missing, "missing AGENTS.md/CLAUDE.md")
	}
	if len(s.ProjectSignals) == 0 {
		missing = append(missing, "missing README.md or .git")
	}
	if len(missing) == 0 {
		return "incomplete; project onboarding is not complete"
	}
	return "incomplete; " + strings.Join(missing, "; ")
}

func cleanRoot(root string) string {
	root = strings.TrimSpace(root)
	if root == "" {
		root = "."
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return filepath.Clean(root)
	}
	return filepath.Clean(abs)
}

func workspaceStatProblem(err error) string {
	if os.IsNotExist(err) {
		return "workspace root does not exist"
	}
	if os.IsPermission(err) {
		return "workspace root is not readable"
	}
	return "workspace root cannot be inspected"
}

func buildSteps(state State) []Step {
	workspaceReady := state.WorkspaceExists && state.WorkspaceReadable && !state.WorkspaceEmpty
	projectStepsEnabled := state.WorkspaceExists && state.WorkspaceReadable && !state.WorkspaceEmpty

	return []Step{
		{
			Key:         StepWorkspace,
			Text:        "Workspace contains project files",
			Complete:    workspaceReady,
			Completable: true,
			Enabled:     true,
		},
		{
			Key:         StepInstructions,
			Text:        "Project instructions exist (AGENTS.md or CLAUDE.md)",
			Complete:    len(state.InstructionFiles) > 0,
			Completable: true,
			Enabled:     projectStepsEnabled,
		},
		{
			Key:         StepProjectSignal,
			Text:        "Project signal exists (README.md or .git)",
			Complete:    len(state.ProjectSignals) > 0,
			Completable: true,
			Enabled:     projectStepsEnabled,
		},
	}
}

func completeSteps(steps []Step) bool {
	for _, step := range steps {
		if step.Enabled && step.Completable && !step.Complete {
			return false
		}
	}
	return true
}

func existingFiles(root string, names []string) []string {
	found := make([]string, 0, len(names))
	for _, name := range names {
		info, err := os.Stat(filepath.Join(root, name))
		if err == nil && info.Mode().IsRegular() {
			found = append(found, name)
		}
	}
	return found
}

func existingProjectSignals(root string) []string {
	found := existingFiles(root, []string{"README.md"})
	if _, err := os.Stat(filepath.Join(root, ".git")); err == nil {
		found = append(found, ".git")
	}
	return found
}

func joinOrNone(values []string) string {
	if len(values) == 0 {
		return "none"
	}
	return strings.Join(values, ",")
}
