package textfilecreate

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"awaken/src/tool"
)

func TestTextFileCreateWritesMarkdown(t *testing.T) {
	root := t.TempDir()
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))

	out, err := (&TextFileCreateTool{}).Execute(ctx, `{"path":"reports/ERP差异报告.md","content":"# 标题\n正文\n"}`)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(out, "ERP差异报告.md") {
		t.Fatalf("unexpected output: %q", out)
	}
	data, err := os.ReadFile(filepath.Join(root, "reports", "ERP差异报告.md"))
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	if string(data) != "# 标题\n正文\n" {
		t.Fatalf("unexpected content: %q", string(data))
	}
}

func TestTextFileCreateOutcomeReportsObservedChange(t *testing.T) {
	root := t.TempDir()
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))

	outcome, err := (&TextFileCreateTool{}).ExecuteOutcome(ctx, `{"path":"notes/new.md","content":"hello"}`, nil)
	if err != nil {
		t.Fatalf("ExecuteOutcome transport error: %v", err)
	}
	if outcome.Status != tool.OutcomeSucceeded || len(outcome.Changes) != 1 {
		t.Fatalf("expected successful observed change, got %+v", outcome)
	}
	change := outcome.Changes[0]
	if change.Action != "created" || change.Resource != "notes/new.md" || change.Certainty != tool.ChangeObserved {
		t.Fatalf("unexpected change: %+v", change)
	}
	if outcome.Details["bytes_written"] != 5 || outcome.Details["action"] != "created" {
		t.Fatalf("missing typed details: %+v", outcome.Details)
	}
}

func TestTextFileCreateValidationFailureIsNotStarted(t *testing.T) {
	outcome, err := (&TextFileCreateTool{}).ExecuteOutcome(context.Background(), `{"path":"","content":"x"}`, nil)
	if err != nil {
		t.Fatalf("ExecuteOutcome transport error: %v", err)
	}
	normalized := outcome.Normalize(tool.OutcomeNormalizationContext{OperationType: "text_file_create"})
	if normalized.Status != tool.OutcomeFailed || normalized.Operation.SideEffectState != tool.OutcomeSideEffectNotStarted || len(normalized.Changes) != 0 {
		t.Fatalf("validation failure must not claim changes: %+v", normalized)
	}
}

func TestTextFileCreateRejectsSensitivePath(t *testing.T) {
	root := t.TempDir()
	decision := (&TextFileCreateTool{}).CheckPermission(
		context.Background(),
		`{"path":".env","content":"SECRET=1"}`,
		tool.NewPermissionContext(root, []string{root}),
	)
	if decision.Behavior != tool.PermissionDeny {
		t.Fatalf("expected deny for sensitive path, got %+v", decision)
	}
}

func TestTextFileCreateArtifactRolesRequireExplicitPublication(t *testing.T) {
	root := t.TempDir()
	permissionContext := tool.NewPermissionContext(root, []string{root})
	toolInstance := &TextFileCreateTool{}

	defaultArtifacts := toolInstance.Artifacts(`{"path":"report.md","content":"# report"}`, "", permissionContext)
	if len(defaultArtifacts) != 1 || defaultArtifacts[0].Role != tool.ArtifactRoleSupport {
		t.Fatalf("default text creation must be support, got %+v", defaultArtifacts)
	}
	publishedArtifacts := toolInstance.Artifacts(`{"path":"report.md","content":"# report","publish_as_deliverable":true}`, "", permissionContext)
	if len(publishedArtifacts) != 1 || publishedArtifacts[0].Role != tool.ArtifactRoleDeliverable {
		t.Fatalf("explicitly published Markdown must be deliverable, got %+v", publishedArtifacts)
	}
	if publishedArtifacts[0].ContractVersion != tool.ArtifactContractVersion || publishedArtifacts[0].Producer != "text_file_create" {
		t.Fatalf("missing v2 producer metadata: %+v", publishedArtifacts[0])
	}
}

func TestTextFileCreateArtifactActionMatchesCreateAppendAndOverwrite(t *testing.T) {
	root := t.TempDir()
	permissionContext := tool.NewPermissionContext(root, []string{root})
	ctx := tool.ContextWithPermissionContext(context.Background(), permissionContext)
	toolInstance := &TextFileCreateTool{}

	createdInput := `{"path":"created.md","content":"first","append":true}`
	created, err := toolInstance.ExecuteOutcome(ctx, createdInput, nil)
	if err != nil || created.Details["action"] != "created" {
		t.Fatalf("append-to-missing should create: outcome=%+v err=%v", created, err)
	}
	createdArtifacts := toolInstance.Artifacts(createdInput, created.Content, permissionContext)
	if len(createdArtifacts) != 1 || createdArtifacts[0].Action != tool.ArtifactActionCreated {
		t.Fatalf("create artifact action drifted: %+v", createdArtifacts)
	}

	appendInput := `{"path":"created.md","content":" second","append":true}`
	appended, err := toolInstance.ExecuteOutcome(ctx, appendInput, nil)
	if err != nil || appended.Details["action"] != "appended" {
		t.Fatalf("append-to-existing failed: outcome=%+v err=%v", appended, err)
	}
	appendedArtifacts := toolInstance.Artifacts(appendInput, appended.Content, permissionContext)
	if len(appendedArtifacts) != 1 || appendedArtifacts[0].Action != tool.ArtifactActionUpdated {
		t.Fatalf("append artifact must be updated: %+v", appendedArtifacts)
	}

	overwriteInput := `{"path":"created.md","content":"replacement"}`
	overwritten, err := toolInstance.ExecuteOutcome(ctx, overwriteInput, nil)
	if err != nil || overwritten.Details["action"] != "overwritten" {
		t.Fatalf("overwrite-existing failed: outcome=%+v err=%v", overwritten, err)
	}
	overwrittenArtifacts := toolInstance.Artifacts(overwriteInput, overwritten.Content, permissionContext)
	if len(overwrittenArtifacts) != 1 || overwrittenArtifacts[0].Action != tool.ArtifactActionUpdated {
		t.Fatalf("overwrite artifact must be updated: %+v", overwrittenArtifacts)
	}
}

func TestTextFileCreateRejectsUnsupportedExplicitPublication(t *testing.T) {
	root := t.TempDir()
	decision := (&TextFileCreateTool{}).CheckPermission(
		context.Background(),
		`{"path":"notes.txt","content":"notes","publish_as_deliverable":true}`,
		tool.NewPermissionContext(root, []string{root}),
	)
	if decision.Behavior != tool.PermissionDeny {
		t.Fatalf("expected unsupported deliverable denial, got %+v", decision)
	}
}

func TestTextFileCreateExplicitPublicationRequiresOutputRoot(t *testing.T) {
	root := t.TempDir()
	outputRoot := filepath.Join(root, "outputs")
	if err := os.MkdirAll(outputRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	permissionContext := tool.NewTaskScopePermissionContext(
		"scope",
		root,
		[]string{root},
		[]string{root},
		[]string{outputRoot},
		nil,
		nil,
		filepath.Join(root, ".tmp"),
	)
	decision := (&TextFileCreateTool{}).CheckPermission(
		context.Background(),
		`{"path":"private/report.md","content":"# report","publish_as_deliverable":true}`,
		permissionContext,
	)
	if decision.Behavior != tool.PermissionDeny || !strings.Contains(decision.Reason, "outside output roots") {
		t.Fatalf("expected output-root denial, got %+v", decision)
	}
}
