package runtime

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"awaken/src/tool"
)

func TestToolResultStorePersistsDescriptorsWithoutInlineBase64(t *testing.T) {
	home := t.TempDir()
	t.Setenv("AWAKEN_HOME", home)
	store := NewToolResultStore()
	part := tool.ModelContentPart{Type: "image", MimeType: "image/png", DisplayName: "image.png", ByteSize: 5, DataBase64: base64.StdEncoding.EncodeToString([]byte("image"))}
	persisted, err := store.PersistToolResult(context.Background(), "thread-a", tool.ToolExecutionIdentity{AgentID: "root", ToolCallID: "call-a"}, []tool.ModelContentPart{part})
	if err != nil {
		t.Fatal(err)
	}
	if len(persisted) != 1 || persisted[0].StorageKey == "" || persisted[0].DataBase64 == "" {
		t.Fatalf("persisted=%+v", persisted)
	}
	if !strings.HasPrefix(persisted[0].StorageKey, "thread-a/tool-results/root/call-a/") {
		t.Fatalf("storage key=%q", persisted[0].StorageKey)
	}
	hydrated, warnings := store.HydrateToolResult(context.Background(), "thread-a", persisted)
	if len(warnings) != 0 || len(hydrated) != 1 || hydrated[0].DataBase64 != part.DataBase64 {
		t.Fatalf("hydrated=%+v warnings=%v", hydrated, warnings)
	}
}

func TestToolResultStoreOffloadsTextAsThreadOwned0600Blob(t *testing.T) {
	setRuntimeUserDataDir(t)
	store := NewToolResultStore()
	content := strings.Repeat("large-result-", 4000)
	receipt, err := store.OffloadToolResult(context.Background(), "thread-text", tool.ToolExecutionIdentity{SubagentTaskID: "task-text", ToolCallID: "call-text"}, content)
	if err != nil {
		t.Fatal(err)
	}
	path, err := ownedToolResultPath(receipt.StorageKey)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != content {
		t.Fatalf("offloaded blob mismatch: err=%v bytes=%d", err, len(data))
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("offloaded blob mode=%v err=%v", info.Mode().Perm(), err)
	}
	if receipt.ByteSize != len(content) || receipt.Digest == "" || receipt.Preview == "" {
		t.Fatalf("invalid receipt: %+v", receipt)
	}
	hydrated, err := store.HydrateOffloadedToolResult(context.Background(), "thread-text", receipt)
	if err != nil || hydrated != content {
		t.Fatalf("hydrate mismatch: err=%v bytes=%d", err, len(hydrated))
	}
}

func TestToolResultStoreIsolatesSameToolCallIDAcrossAgents(t *testing.T) {
	home := t.TempDir()
	t.Setenv("AWAKEN_HOME", home)
	store := NewToolResultStore()
	part := func(value string) tool.ModelContentPart {
		return tool.ModelContentPart{Type: "image", MimeType: "image/png", DisplayName: value + ".png", DataBase64: base64.StdEncoding.EncodeToString([]byte(value))}
	}
	root, err := store.PersistToolResult(context.Background(), "thread-a", tool.ToolExecutionIdentity{AgentID: "root", ToolCallID: "Read_0"}, []tool.ModelContentPart{part("root")})
	if err != nil {
		t.Fatal(err)
	}
	child, err := store.PersistToolResult(context.Background(), "thread-a", tool.ToolExecutionIdentity{SubagentTaskID: "task-child", ToolCallID: "Read_0"}, []tool.ModelContentPart{part("child")})
	if err != nil {
		t.Fatal(err)
	}
	if root[0].StorageKey == child[0].StorageKey {
		t.Fatalf("agent-scoped storage keys collided: %q", root[0].StorageKey)
	}
	root[0].DataBase64, child[0].DataBase64 = "", ""
	hydratedRoot, rootWarnings := store.HydrateToolResult(context.Background(), "thread-a", root)
	hydratedChild, childWarnings := store.HydrateToolResult(context.Background(), "thread-a", child)
	if len(rootWarnings) != 0 || len(childWarnings) != 0 {
		t.Fatalf("warnings root=%v child=%v", rootWarnings, childWarnings)
	}
	if got, _ := base64.StdEncoding.DecodeString(hydratedRoot[0].DataBase64); string(got) != "root" {
		t.Fatalf("root blob overwritten: %q", got)
	}
	if got, _ := base64.StdEncoding.DecodeString(hydratedChild[0].DataBase64); string(got) != "child" {
		t.Fatalf("child blob overwritten: %q", got)
	}
}

func TestToolResultStoreIsolatesReusedToolCallIDAcrossTurns(t *testing.T) {
	home := t.TempDir()
	t.Setenv("AWAKEN_HOME", home)
	store := NewToolResultStore()
	part := func(value string) tool.ModelContentPart {
		return tool.ModelContentPart{Type: "image", MimeType: "image/png", DisplayName: value + ".png", DataBase64: base64.StdEncoding.EncodeToString([]byte(value))}
	}
	first, err := store.PersistToolResult(context.Background(), "thread-a", tool.ToolExecutionIdentity{AgentID: "root", RunID: "run-a", TurnID: "turn-1", ToolCallID: "Read_0"}, []tool.ModelContentPart{part("first")})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.PersistToolResult(context.Background(), "thread-a", tool.ToolExecutionIdentity{AgentID: "root", RunID: "run-a", TurnID: "turn-2", ToolCallID: "Read_0"}, []tool.ModelContentPart{part("second")})
	if err != nil {
		t.Fatal(err)
	}
	if first[0].StorageKey == second[0].StorageKey {
		t.Fatalf("turn-scoped storage keys collided: %q", first[0].StorageKey)
	}
}

func TestToolResultStoreMissingBlobWarnsWithoutDroppingDescriptor(t *testing.T) {
	home := t.TempDir()
	t.Setenv("AWAKEN_HOME", home)
	store := NewToolResultStore()
	parts, err := store.PersistToolResult(context.Background(), "thread-a", tool.ToolExecutionIdentity{ToolCallID: "call-a"}, []tool.ModelContentPart{{Type: "file", MimeType: "application/pdf", DisplayName: "a.pdf", DataBase64: base64.StdEncoding.EncodeToString([]byte("pdf"))}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(activeSessionsDir(), filepath.FromSlash(parts[0].StorageKey))); err != nil {
		t.Fatal(err)
	}
	parts[0].DataBase64 = ""
	hydrated, warnings := store.HydrateToolResult(context.Background(), "thread-a", parts)
	if len(hydrated) != 1 || hydrated[0].StorageKey == "" || hydrated[0].DataBase64 != "" || len(warnings) != 1 || warnings[0].Code != "tool_media_blob_missing" {
		t.Fatalf("hydrated=%+v warnings=%v", hydrated, warnings)
	}
}
