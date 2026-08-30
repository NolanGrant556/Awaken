package hookruntime

import (
	"fmt"
	"testing"
	"time"
)

func TestAsyncCompletionStoreIsThreadScopedBoundedAndDrainedOnce(t *testing.T) {
	store := NewAsyncCompletionStore()
	for index := 0; index < asyncCompletionThreadCapacity+5; index++ {
		store.RecordAsyncHookCompletion("thread-a", AsyncCompletion{
			InvocationID: fmt.Sprintf("hook-%d", index), Event: EventBeforeFinish,
			AdditionalContext: fmt.Sprintf("context-%d", index), CompletedAt: time.Now().UTC(),
		})
	}
	store.RecordAsyncHookCompletion("thread-b", AsyncCompletion{InvocationID: "other", AdditionalContext: "other"})
	items := store.DrainAsyncHookCompletions("thread-a")
	if len(items) != asyncCompletionThreadCapacity || items[0].InvocationID != "hook-5" {
		t.Fatalf("items=%#v", items)
	}
	if store.HasAsyncHookCompletions("thread-a") || len(store.DrainAsyncHookCompletions("thread-a")) != 0 {
		t.Fatal("thread-a completions were not drained exactly once")
	}
	if !store.HasAsyncHookCompletions("thread-b") {
		t.Fatal("draining thread-a affected thread-b")
	}
}
