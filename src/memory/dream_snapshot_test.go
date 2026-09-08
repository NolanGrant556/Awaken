package memory

import (
	"encoding/json"
	"sync"
	"testing"
)

func TestDreamSnapshotIsolation(t *testing.T) {
	r := NewDreamRunnerWithRoot(t.TempDir())
	r.tasks["audit"] = DreamTask{TaskID: "audit", Reports: map[Scope]MaintenanceReport{}}
	snapshot, _ := r.Get("audit")
	if e := r.update("audit", func(task *DreamTask) { task.Reports[ScopeUser] = MaintenanceReport{Updated: 1} }); e != nil {
		t.Fatal(e)
	}
	if len(snapshot.Reports) != 0 {
		t.Fatal("previously returned snapshot changed after manager update: shared Reports map")
	}
}
func TestDreamSnapshotMarshalRace(t *testing.T) {
	r := NewDreamRunnerWithRoot(t.TempDir())
	r.tasks["audit"] = DreamTask{TaskID: "audit", Reports: map[Scope]MaintenanceReport{ScopeUser: {Updated: 0}}}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			r.update("audit", func(task *DreamTask) { task.Reports[ScopeUser] = MaintenanceReport{Updated: i} })
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 10000; i++ {
			snapshot, _ := r.Get("audit")
			json.Marshal(snapshot)
		}
	}()
	wg.Wait()
}
