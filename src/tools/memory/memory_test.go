package memorytool

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"awaken/src/memory"
	"awaken/src/tool"
)

func TestToolUsesHostOwnedSourceIdentity(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	t.Setenv("AWAKEN_HOME", home)
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(workspace, []string{workspace}))
	ctx = tool.ContextWithToolThreadID(ctx, "thread_host")
	ctx = tool.ContextWithToolExecutionIdentity(ctx, tool.ToolExecutionIdentity{RunID: "run_host", ToolCallID: "call_host", ToolName: ToolName})
	raw := `{"action":"create","scope":"project","type":"feedback","name":"Testing","description":"Use a real database","content":"Use a real database for integration tests."}`
	output, err := (&Tool{}).Execute(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatal(err)
	}
	if result["source_thread_id"] != "thread_host" || result["source_record_id"] != "call_host" {
		t.Fatalf("host source not applied: %s", output)
	}
	if _, err := filepath.Glob(filepath.Join(workspace, ".awaken", "memory", "*.md")); err != nil {
		t.Fatal(err)
	}
}

func TestToolRequiresExecutionIdentityAndSanitizesJournal(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("AWAKEN_HOME", t.TempDir())
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(workspace, []string{workspace}))
	if _, err := (&Tool{}).Execute(ctx, `{"action":"create"}`); err == nil || !strings.Contains(err.Error(), "host_identity") {
		t.Fatalf("expected host identity error, got %v", err)
	}
	toolInstance := &Tool{}
	arguments := toolInstance.SemanticJournalArguments(`{"action":"create","scope":"user","content":"private body","description":"private description"}`)
	if strings.Contains(arguments, "private") {
		t.Fatalf("journal arguments leaked body: %s", arguments)
	}
	result := toolInstance.SemanticJournalResult(`{"memory_id":"mem_1","content":"private body","revision":1,"source_thread_id":"thread"}`)
	if strings.Contains(result, "private") || strings.Contains(result, "source_thread") {
		t.Fatalf("journal result leaked body/source: %s", result)
	}
}

func TestToolDefinitionPromotesDurableNamingPreferences(t *testing.T) {
	definition := (&Tool{}).Definition()
	encoded, err := json.Marshal(definition)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	for _, required := range []string{"must call", "text acknowledgement", "future-facing", "form of address", "博士哥", "小田"} {
		if !strings.Contains(text, required) {
			t.Fatalf("tool definition missing %q: %s", required, text)
		}
	}
}

func TestMutationOutcomeGivesModelExplicitCompletionReceipt(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	t.Setenv("AWAKEN_HOME", home)
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(workspace, []string{workspace}))
	ctx = tool.ContextWithToolThreadID(ctx, "thread_completion")
	ctx = tool.ContextWithToolExecutionIdentity(ctx, tool.ToolExecutionIdentity{RunID: "run_completion", ToolCallID: "call_completion", ToolName: ToolName})
	outcome, err := (&Tool{}).ExecuteOutcome(ctx, `{"action":"create","scope":"user","type":"feedback","name":"Address preference","description":"Call the user Doctor","content":"Call the user Doctor in future conversations."}`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Status != tool.OutcomeSucceeded || !strings.Contains(outcome.Content, `"write_completed":true`) || !strings.Contains(outcome.Content, `"next_step"`) || !strings.Contains(outcome.Summary, "Stop additional reasoning") {
		t.Fatalf("missing explicit completion receipt: %+v", outcome)
	}
	if strings.Contains(outcome.Content, "future conversations") {
		t.Fatalf("mutation result leaked the full saved body back to the model: %s", outcome.Content)
	}
	if len(outcome.NextActions) != 1 || !strings.Contains(outcome.NextActions[0].Description, "finish") {
		t.Fatalf("missing finish guidance: %+v", outcome.NextActions)
	}
}

func TestToolResolvesLegacyIndexFilenameToCanonicalMemoryID(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	t.Setenv("AWAKEN_HOME", home)
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(workspace, []string{workspace}))
	ctx = tool.ContextWithToolThreadID(ctx, "thread_alias")
	ctx = tool.ContextWithToolExecutionIdentity(ctx, tool.ToolExecutionIdentity{RunID: "run_alias", ToolCallID: "call_create", ToolName: ToolName})
	createdRaw, err := (&Tool{}).Execute(ctx, `{"action":"create","scope":"user","type":"feedback","name":"Alias test","description":"Resolve legacy filename","content":"Use the canonical identity after resolving a filename."}`)
	if err != nil {
		t.Fatal(err)
	}
	var created map[string]any
	if err := json.Unmarshal([]byte(createdRaw), &created); err != nil {
		t.Fatal(err)
	}
	memoryID, _ := created["memory_id"].(string)
	store, err := memory.NewStore(workspace)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Get(memoryID)
	if err != nil {
		t.Fatal(err)
	}
	ctx = tool.ContextWithToolExecutionIdentity(ctx, tool.ToolExecutionIdentity{RunID: "run_alias", ToolCallID: "call_get", ToolName: ToolName})
	gotRaw, err := (&Tool{}).Execute(ctx, fmt.Sprintf(`{"action":"get","memory_id":%q}`, filepath.Base(record.Path)))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotRaw, `"memory_id":"`+memoryID+`"`) {
		t.Fatalf("legacy alias did not resolve to canonical id: %s", gotRaw)
	}
}
