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
	persisted, err := store.PersistToolResult(context.Background(), "thread-a", tool.ToolExecutionIdentity{ToolCallID: "call-a"}, []tool.ModelContentPart{part})
	if err != nil {
		t.Fatal(err)
	}
	if len(persisted) != 1 || persisted[0].StorageKey == "" || persisted[0].DataBase64 == "" {
		t.Fatalf("persisted=%+v", persisted)
	}
	if !strings.HasPrefix(persisted[0].StorageKey, "thread-a/tool-results/call-a/") {
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
	receipt, err := store.OffloadToolResult(context.Background(), "thread-text", "call-text", content)
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
