package officerender

import "testing"

func TestSharedManagedWorkerCachesByRuntimeRoot(t *testing.T) {
	firstRoot := t.TempDir()
	t.Setenv("AWAKEN_PRIMARY_RUNTIME_ROOT", firstRoot)
	first, err := SharedManagedWorker()
	if err != nil {
		t.Fatal(err)
	}
	again, err := SharedManagedWorker()
	if err != nil {
		t.Fatal(err)
	}
	if first != again {
		t.Fatal("same runtime root must reuse the process-wide worker")
	}

	secondRoot := t.TempDir()
	t.Setenv("AWAKEN_PRIMARY_RUNTIME_ROOT", secondRoot)
	second, err := SharedManagedWorker()
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("runtime root change must select a new process-wide worker")
	}
}
