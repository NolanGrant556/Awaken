package trashfile

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"awaken/src/tool"
	"awaken/src/tool/trash"
)

type recordingTrashAdapter struct {
	calls []string
	dir   string
}

func (a *recordingTrashAdapter) TrashPath(path string) (trash.TrashResult, error) {
	a.calls = append(a.calls, path)
	if err := os.MkdirAll(a.dir, 0o700); err != nil {
		return trash.TrashResult{}, err
	}
	target := filepath.Join(a.dir, filepath.Base(path))
	if err := os.Rename(path, target); err != nil {
		return trash.TrashResult{}, err
	}
	return trash.TrashResult{
		OriginalPath: path,
		TrashedPath:  target,
		Method:       "test-rename",
	}, nil
}

func TestTrashFileMovesPathThroughTrashAdapter(t *testing.T) {
	adapter := &recordingTrashAdapter{dir: filepath.Join(t.TempDir(), "trash")}
	trash.SetAdapter(adapter)
	t.Cleanup(func() { trash.SetAdapter(nil) })

	outsideRoot := t.TempDir()
	target := filepath.Join(outsideRoot, "outside.txt")
	if err := os.WriteFile(target, []byte("bye"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}

	trashTool := &TrashFileTool{}
	decision := trashTool.CheckPermission(
		context.Background(),
		`{"path":`+mustJSONQuote(t, target)+`}`,
		toolpkgPermissionContext(t),
	)
	if decision.Behavior != toolpkgPermissionAllow() {
		t.Fatalf("expected trash_file to be allowed for project-external path, got %+v", decision)
	}

	output, err := trashTool.Execute(context.Background(), `{"path":`+mustJSONQuote(t, target)+`}`)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("expected original path to be moved, stat err=%v", err)
	}
	var result trashFileResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if result.TrashedPath == "" || result.Method != "test-rename" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if len(adapter.calls) != 1 {
		t.Fatalf("expected one trash adapter call, got %d", len(adapter.calls))
	}
}

func TestTrashFileOutcomeCarriesTypedTrashChange(t *testing.T) {
	adapter := &recordingTrashAdapter{dir: filepath.Join(t.TempDir(), "trash")}
	trash.SetAdapter(adapter)
	t.Cleanup(func() { trash.SetAdapter(nil) })

	target := filepath.Join(t.TempDir(), "obsolete.txt")
	if err := os.WriteFile(target, []byte("bye"), 0o644); err != nil {
		t.Fatal(err)
	}
	outcome, err := (&TrashFileTool{}).ExecuteOutcome(context.Background(), `{"path":`+mustJSONQuote(t, target)+`}`, nil)
	if err != nil {
		t.Fatalf("ExecuteOutcome transport error: %v", err)
	}
	if outcome.Status != tool.OutcomeSucceeded || len(outcome.Changes) != 1 {
		t.Fatalf("expected canonical trash success, got %+v", outcome)
	}
	change := outcome.Changes[0]
	if change.Action != "trashed" || change.Resource != target || change.Certainty != tool.ChangeObserved {
		t.Fatalf("unexpected trash change: %+v", change)
	}
	if change.Details["trashed_path"] == "" || outcome.Operation.SideEffectState != tool.OutcomeSideEffectApplied {
		t.Fatalf("missing trash destination or side effect state: %+v", outcome)
	}
}

func TestTrashFileEmitsDeletedArtifactTombstones(t *testing.T) {
	workspace := t.TempDir()
	permissionContext := tool.NewPermissionContext(workspace, []string{workspace})
	artifacts := (&TrashFileTool{}).Artifacts(
		`{"paths":["output/first.md","output/second.pdf"]}`,
		"",
		permissionContext,
	)
	if len(artifacts) != 2 {
		t.Fatalf("expected two deletion tombstones, got %+v", artifacts)
	}
	for index, artifact := range artifacts {
		if artifact.Role != tool.ArtifactRoleSupport || artifact.Kind != "deleted" || artifact.Status != "ready" {
			t.Fatalf("artifact %d is not a deletion tombstone: %+v", index, artifact)
		}
	}
	if artifacts[0].Path != "output/first.md" || artifacts[1].Path != "output/second.pdf" {
		t.Fatalf("unexpected tombstone paths: %+v", artifacts)
	}
}

func TestTrashFileCanonicalSchemaAndParserRequireExactlyOneInput(t *testing.T) {
	trashTool := &TrashFileTool{}
	schema := trashTool.InputSchema()
	if len(schema.OneOf) != 2 {
		t.Fatalf("expected canonical path/paths oneOf, got %+v", schema)
	}
	for _, input := range []string{`{}`, `{"path":"one.txt","paths":["two.txt"]}`} {
		if err := tool.ValidateInputAgainstSchema(input, schema); err == nil {
			t.Fatalf("expected canonical validation to reject %s", input)
		}
		if _, err := parseTrashFileParams(input); err == nil {
			t.Fatalf("expected executor parser to reject %s", input)
		}
	}
	for _, input := range []string{`{"path":"one.txt"}`, `{"paths":["one.txt","two.txt"]}`} {
		if err := tool.ValidateInputAgainstSchema(input, schema); err != nil {
			t.Fatalf("expected canonical validation to accept %s: %v", input, err)
		}
		if _, err := parseTrashFileParams(input); err != nil {
			t.Fatalf("expected executor parser to accept %s: %v", input, err)
		}
	}
}

func TestTrashFileResolvesRelativeDirectoryThroughPermissionContext(t *testing.T) {
	adapter := &recordingTrashAdapter{dir: filepath.Join(t.TempDir(), "trash")}
	trash.SetAdapter(adapter)
	t.Cleanup(func() { trash.SetAdapter(nil) })

	workspace := t.TempDir()
	target := filepath.Join(workspace, "build")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("create target dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(target, "artifact.txt"), []byte("bye"), 0o644); err != nil {
		t.Fatalf("write target file: %v", err)
	}

	permCtx := tool.NewPermissionContext(workspace, []string{workspace})
	ctx := tool.ContextWithPermissionContext(context.Background(), permCtx)
	trashTool := &TrashFileTool{}
	decision := trashTool.CheckPermission(context.Background(), `{"path":"build"}`, permCtx)
	if decision.Behavior != tool.PermissionAllow {
		t.Fatalf("expected trash_file relative directory to be allowed, got %+v", decision)
	}
	if decision.Behavior != tool.PermissionAllow {
		t.Fatalf("expected allow decision for %q, got %+v", target, decision)
	}

	output, err := trashTool.Execute(ctx, `{"path":"build"}`)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("expected original directory to be moved, stat err=%v", err)
	}
	var result trashFileResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if result.OriginalPath != target || result.TrashedPath == "" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if _, err := os.Stat(filepath.Join(result.TrashedPath, "artifact.txt")); err != nil {
		t.Fatalf("expected directory contents to move to trash: %v", err)
	}
}

func TestTrashFileRejectsSensitiveAndProtectedPathsBeforeAdapter(t *testing.T) {
	adapter := &recordingTrashAdapter{dir: filepath.Join(t.TempDir(), "trash")}
	trash.SetAdapter(adapter)
	t.Cleanup(func() { trash.SetAdapter(nil) })

	workspace := t.TempDir()
	sensitive := filepath.Join(workspace, ".env")
	protected := filepath.Join(workspace, "protected")
	if err := os.WriteFile(sensitive, []byte("SECRET=value"), 0o600); err != nil {
		t.Fatalf("write sensitive target: %v", err)
	}
	if err := os.MkdirAll(protected, 0o755); err != nil {
		t.Fatalf("create protected target: %v", err)
	}
	permissionContext := tool.NewTaskScopePermissionContext(
		"test", workspace, nil, []string{workspace}, nil, nil, []string{protected}, "",
	)
	ctx := tool.ContextWithPermissionContext(context.Background(), permissionContext)
	for _, target := range []string{sensitive, protected} {
		t.Run(filepath.Base(target), func(t *testing.T) {
			input := `{"path":` + mustJSONQuote(t, target) + `}`
			decision := (&TrashFileTool{}).CheckPermission(context.Background(), input, permissionContext)
			if decision.Behavior != tool.PermissionDeny {
				t.Fatalf("expected safe-path denial, got %+v", decision)
			}
			outcome, err := (&TrashFileTool{}).ExecuteOutcome(ctx, input, nil)
			if err != nil {
				t.Fatalf("transport error: %v", err)
			}
			if outcome.Status != tool.OutcomeDenied || outcome.ErrorCode != tool.ErrorCodeUnsafeRecoverableDeletePath || outcome.TransportStatus != tool.TransportNotStarted {
				t.Fatalf("unexpected denial outcome: %+v", outcome)
			}
			if _, err := os.Lstat(target); err != nil {
				t.Fatalf("denied target changed: %v", err)
			}
		})
	}
	if len(adapter.calls) != 0 {
		t.Fatalf("adapter ran for denied paths: %+v", adapter.calls)
	}
}

func TestTrashFileRejectsLeafAndResolvedSensitiveSymlinks(t *testing.T) {
	adapter := &recordingTrashAdapter{dir: filepath.Join(t.TempDir(), "trash")}
	trash.SetAdapter(adapter)
	t.Cleanup(func() { trash.SetAdapter(nil) })

	workspace := t.TempDir()
	realFile := filepath.Join(workspace, "real.txt")
	leafLink := filepath.Join(workspace, "real-link.txt")
	gitDir := filepath.Join(workspace, ".git")
	gitConfig := filepath.Join(gitDir, "config")
	gitAlias := filepath.Join(workspace, "git-alias")
	if err := os.WriteFile(realFile, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realFile, leafLink); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(gitConfig, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(gitDir, gitAlias); err != nil {
		t.Fatal(err)
	}
	permissionContext := tool.NewPermissionContext(workspace, []string{workspace})
	ctx := tool.ContextWithPermissionContext(context.Background(), permissionContext)
	for _, target := range []string{leafLink, filepath.Join(gitAlias, "config")} {
		input := `{"path":` + mustJSONQuote(t, target) + `}`
		outcome, err := (&TrashFileTool{}).ExecuteOutcome(ctx, input, nil)
		if err != nil || outcome.Status != tool.OutcomeDenied || outcome.ErrorCode != tool.ErrorCodeUnsafeRecoverableDeletePath {
			t.Fatalf("expected symlink-safe denial for %q, outcome=%+v err=%v", target, outcome, err)
		}
	}
	if content, err := os.ReadFile(realFile); err != nil || string(content) != "keep" {
		t.Fatalf("leaf symlink target changed: content=%q err=%v", content, err)
	}
	if content, err := os.ReadFile(gitConfig); err != nil || string(content) != "keep" {
		t.Fatalf("resolved sensitive target changed: content=%q err=%v", content, err)
	}
	if len(adapter.calls) != 0 {
		t.Fatalf("adapter ran for symlink paths: %+v", adapter.calls)
	}
}

func mustJSONQuote(t *testing.T, value string) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal JSON: %v", err)
	}
	return string(data)
}

func toolpkgPermissionContext(t *testing.T) tool.PermissionContext {
	t.Helper()
	root := t.TempDir()
	return tool.NewPermissionContext(root, []string{root})
}

func toolpkgPermissionAllow() tool.PermissionBehavior {
	return tool.PermissionAllow
}
