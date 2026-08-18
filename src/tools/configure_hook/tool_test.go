package configurehook

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"awaken/src/hookruntime"
	"awaken/src/tool"
)

func TestConfigureHookFromModelInputWritesProjectLocalWithoutTrust(t *testing.T) {
	home, project := t.TempDir(), t.TempDir()
	store, _ := hookruntime.NewStore(home)
	runtimeTool := &Tool{store: store}
	permission := tool.NewTaskScopePermissionContext("scope", project, []string{project}, []string{project}, []string{project}, nil, nil, "")
	permission.ProjectID = "project-1"
	ctx := tool.ContextWithPermissionContext(context.Background(), permission)
	raw := encode(t, map[string]any{
		"action": "upsert", "scope": "project_local", "event": "PreToolUse", "tool_names": []string{"Write"},
		"hook": map[string]any{"id": "check-write", "type": "command", "command": []string{"./check.sh"}, "cwd": "project_root", "on_failure": "block"},
	})
	output, err := runtimeTool.Execute(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, `"requiresTrust": true`) || !strings.Contains(output, "hooks.local.json") {
		t.Fatalf("output = %s", output)
	}
	status, err := store.Inspect("project-1", []string{project}, false)
	if err != nil || len(status.Sources) != 1 || status.Sources[0].Status != hookruntime.StatusUntrusted {
		t.Fatalf("status=%+v err=%v", status, err)
	}
}

func TestConfigureHookOutcomeMatchesStrictOutputSchema(t *testing.T) {
	home := t.TempDir()
	store, _ := hookruntime.NewStore(home)
	runtimeTool := &Tool{store: store}
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(home, []string{home}))
	outcome, err := runtimeTool.ExecuteOutcome(ctx, encode(t, map[string]any{
		"action": "list", "scope": "user",
	}), nil)
	if err != nil {
		t.Fatal(err)
	}
	definition := runtimeTool.Definition()
	normalized := outcome.Normalize(tool.OutcomeNormalizationContext{
		OperationType: ToolName, OutputSchema: definition.OutputSchema, StrictOutputSchema: true,
	})
	if normalized.IsError() || normalized.ErrorCode == "tool_output_schema_mismatch" {
		t.Fatalf("normalized outcome = %+v", normalized)
	}
}

func TestConfigureHookRejectsUnattachedProjectAndSessionWorkspace(t *testing.T) {
	home, project, outside := t.TempDir(), t.TempDir(), t.TempDir()
	store, _ := hookruntime.NewStore(home)
	runtimeTool := &Tool{store: store}
	raw := encode(t, map[string]any{"action": "list", "scope": "project", "project_root": outside})
	permission := tool.NewTaskScopePermissionContext("scope", project, []string{project}, []string{project}, nil, nil, nil, "")
	permission.ProjectID = "project-1"
	ctx := tool.ContextWithPermissionContext(context.Background(), permission)
	if _, err := runtimeTool.Execute(ctx, raw); err == nil || !strings.Contains(err.Error(), "scope_unavailable") {
		t.Fatalf("expected unattached root rejection, got %v", err)
	}
	permission.SessionWorkspace = true
	ctx = tool.ContextWithPermissionContext(context.Background(), permission)
	if _, err := runtimeTool.Execute(ctx, encode(t, map[string]any{"action": "list", "scope": "project"})); err == nil {
		t.Fatal("expected session workspace rejection")
	}
	if _, err := filepath.Abs(project); err != nil {
		t.Fatal(err)
	}
}

func TestConfigureHookReviewNeverReturnsSecretReference(t *testing.T) {
	home := t.TempDir()
	store, _ := hookruntime.NewStore(home)
	runtimeTool := &Tool{store: store}
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(home, []string{home}))
	output, err := runtimeTool.Execute(ctx, encode(t, map[string]any{
		"action": "upsert", "scope": "user", "event": "RunStart",
		"hook": map[string]any{"id": "notify", "type": "http", "url": "https://example.com/hook", "secret_headers": map[string]string{"Authorization": "private-token-ref"}},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output, "private-token-ref") || !strings.Contains(output, "Authorization") {
		t.Fatalf("unsafe review output: %s", output)
	}
}

func TestConfigureHookSemanticJournalStripsConfigurationBodies(t *testing.T) {
	runtimeTool := &Tool{}
	arguments := runtimeTool.SemanticJournalArguments(encode(t, map[string]any{
		"action": "upsert", "scope": "user", "event": "RunStart",
		"hook": map[string]any{"id": "private", "type": "prompt", "prompt": "private prompt", "secret_headers": map[string]string{"Authorization": "private-ref"}},
	}))
	if strings.Contains(arguments, "private prompt") || strings.Contains(arguments, "private-ref") || !strings.Contains(arguments, "private") {
		t.Fatalf("arguments receipt = %s", arguments)
	}
	result := runtimeTool.SemanticJournalResult(`{"action":"list","source":"user","path":"/private/config/hooks.json","exists":true,"enabled":true,"hookCount":1,"requiresTrust":false,"hooks":[{"id":"private","prompt":"private prompt"}]}`)
	if strings.Contains(result, "/private/config") || strings.Contains(result, "private prompt") || strings.Contains(result, `"hooks"`) {
		t.Fatalf("result receipt = %s", result)
	}
}

func encode(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
