package officerender

import "testing"

func TestSharedHostWorkerIsProcessWide(t *testing.T) {
	first := SharedHostWorker()
	second := SharedHostWorker()
	if first != second {
		t.Fatal("expected one process-wide host LibreOffice worker")
	}
}
