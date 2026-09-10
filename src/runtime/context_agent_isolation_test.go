package runtime

import (
	"context"
	"fmt"
	"testing"
)

func TestContextEpochAgentsCompactIndependentlyAndRestore(t *testing.T) {
	t.Setenv(awakenHomeEnv, t.TempDir())
	store := NewContextEpochStore()
	ids := []string{"", "child-one", "child-two"}
	epochs := make(map[string]string)
	for pass := 0; pass < 4; pass++ {
		for _, id := range ids {
			candidate := testContextEpochCandidate()
			candidate.AgentID = id
			candidate.CompactBoundaryID = fmt.Sprintf("compact-%s-%d", id, pass)
			projection, err := store.Project(context.Background(), candidate)
			if err != nil {
				t.Fatal(err)
			}
			if projection.Receipt.EpochID == epochs[id] {
				t.Fatal("compact did not rotate own epoch")
			}
			epochs[id] = projection.Receipt.EpochID
			for _, sibling := range ids {
				if id != sibling && epochs[sibling] == epochs[id] {
					t.Fatal("agents share an epoch identity")
				}
			}
			restored, err := NewContextEpochStore().Project(context.Background(), candidate)
			if err != nil || restored.Receipt.EpochID != epochs[id] || restored.SystemPrompt != projection.SystemPrompt {
				t.Fatalf("restart mismatch for %s: %v", id, err)
			}
		}
	}
	if len(store.locks) != 0 {
		t.Fatal("inactive context locks leaked")
	}
}
