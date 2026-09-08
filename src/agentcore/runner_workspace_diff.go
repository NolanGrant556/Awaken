package agentcore

import (
	"awaken/src/internal/outputlimit"
	"bytes"
	stdcontext "context"
	"errors"
	"os"
	"os/exec"
	"strings"
)

func (e *runEventEmitter) emitTurnDiff(ctx stdcontext.Context, workspaceRoot string) {
	if !e.mutationOccurred {
		return
	}
	diff, truncated, ok := collectWorkspaceDiff(ctx, workspaceRoot)
	if !ok || strings.TrimSpace(diff) == "" {
		return
	}
	event := AwakenEvent{
		Type:      TurnDiffEventType,
		Diff:      diff,
		Truncated: truncated,
	}
	e.fillIDs(&event)
	if e.recorder != nil {
		if err := e.persistDurable(ctx, "turn_diff", NewSemanticRecord(
			SemanticKindTurnDiff,
			e.threadID,
			e.runID,
			e.turnID(),
			"",
			e.sessionID,
			map[string]any{
				"diff":      diff,
				"truncated": truncated,
			},
		), event); err != nil {
			return
		}
		return
	}
	e.sink.Emit(event)
}

func collectWorkspaceDiff(ctx stdcontext.Context, workspaceRoot string) (string, bool, bool) {
	if strings.TrimSpace(workspaceRoot) == "" {
		return "", false, false
	}
	trackedDiff, ok := gitOutput(ctx, workspaceRoot, "diff", "--")
	if !ok {
		return "", false, false
	}
	untrackedDiff, _ := collectUntrackedWorkspaceDiff(ctx, workspaceRoot)
	diff := trackedDiff
	if diff != "" && untrackedDiff != "" && !strings.HasSuffix(diff, "\n") {
		diff += "\n"
	}
	diff += untrackedDiff
	limited := outputlimit.Limit(diff, maxTurnDiffChars)
	return limited.Content, limited.Truncated, true
}

func collectUntrackedWorkspaceDiff(ctx stdcontext.Context, workspaceRoot string) (string, bool) {
	untracked, ok := gitOutput(ctx, workspaceRoot, "ls-files", "--others", "--exclude-standard", "-z")
	if !ok || untracked == "" {
		return "", ok
	}
	var combined strings.Builder
	for _, relPath := range strings.Split(untracked, "\x00") {
		if relPath == "" {
			continue
		}
		diff, ok := gitOutputAllowDiffExit(ctx, workspaceRoot, "diff", "--no-index", "--", os.DevNull, relPath)
		if !ok {
			return combined.String(), false
		}
		if combined.Len() > 0 && diff != "" && !strings.HasSuffix(combined.String(), "\n") {
			combined.WriteString("\n")
		}
		combined.WriteString(diff)
	}
	return combined.String(), true
}

func gitOutput(ctx stdcontext.Context, workspaceRoot string, args ...string) (string, bool) {
	cmdArgs := append([]string{"-C", workspaceRoot}, args...)
	cmd := exec.CommandContext(ctx, "git", cmdArgs...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return "", false
	}
	return stdout.String(), true
}

func gitOutputAllowDiffExit(ctx stdcontext.Context, workspaceRoot string, args ...string) (string, bool) {
	cmdArgs := append([]string{"-C", workspaceRoot}, args...)
	cmd := exec.CommandContext(ctx, "git", cmdArgs...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	err := cmd.Run()
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
			return "", false
		}
	}
	return stdout.String(), true
}
