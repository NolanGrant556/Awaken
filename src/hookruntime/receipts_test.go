package hookruntime

import (
	"fmt"
	"testing"
)

func TestReceiptStoreFiltersPaginatesAndLinksRetry(t *testing.T) {
	store := NewReceiptStore()
	store.RecordHookReceipt(Outcome{InvocationID: "ignored", SourceKind: SourceUser, PluginName: "demo"})
	for index := 0; index < 3; index++ {
		store.RecordHookReceipt(Outcome{
			InvocationID: fmt.Sprintf("id-%d", index), SourceKind: SourcePlugin,
			PluginName: "demo", PluginDigest: "sha256:test", LogicalID: "hook-a", RevisionID: "rev-1",
			Event: EventPostToolUse, Status: "completed", Action: "continue",
		})
	}
	store.RecordHookReceipt(Outcome{InvocationID: "other", SourceKind: SourcePlugin, PluginName: "demo", LogicalID: "hook-b", RevisionID: "rev-1"})
	store.LinkRetry("id-2", "id-0")

	first, next := store.List("demo", "hook-a", "rev-1", 0, 2)
	if len(first) != 2 || next != 2 {
		t.Fatalf("first page=%#v next=%d", first, next)
	}
	second, next := store.List("demo", "hook-a", "rev-1", next, 2)
	if len(second) != 1 || next != 0 {
		t.Fatalf("second page=%#v next=%d", second, next)
	}
	seen := map[string]bool{}
	for _, receipt := range append(first, second...) {
		seen[receipt.ID] = true
	}
	if !seen["id-0"] || !seen["id-1"] || !seen["id-2"] {
		t.Fatalf("pagination lost or duplicated receipts: %#v", seen)
	}
	if receipt, ok := store.Get("id-2"); !ok || receipt.RetryOf != "id-0" {
		t.Fatalf("linked receipt=%#v ok=%v", receipt, ok)
	}
}

func TestReceiptStoreIsBounded(t *testing.T) {
	store := NewReceiptStore()
	for index := 0; index < receiptCapacity+8; index++ {
		store.RecordHookReceipt(Outcome{
			InvocationID: fmt.Sprintf("id-%d", index), SourceKind: SourcePlugin,
			PluginName: "demo", LogicalID: "hook-a",
		})
	}
	if _, ok := store.Get("id-0"); ok {
		t.Fatal("oldest receipt was not evicted")
	}
	if _, ok := store.Get(fmt.Sprintf("id-%d", receiptCapacity+7)); !ok {
		t.Fatal("newest receipt was evicted")
	}
}

func TestReceiptStoreEmptyListIsNonNil(t *testing.T) {
	items, next := NewReceiptStore().List("demo", "hook-a", "", 0, 20)
	if items == nil || len(items) != 0 || next != 0 {
		t.Fatalf("empty list=%#v next=%d", items, next)
	}

	var store *ReceiptStore
	items, next = store.List("demo", "hook-a", "", 0, 20)
	if items == nil || len(items) != 0 || next != 0 {
		t.Fatalf("nil-store list=%#v next=%d", items, next)
	}
}
