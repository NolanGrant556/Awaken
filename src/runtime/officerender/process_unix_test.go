//go:build !windows

package officerender

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestTimeoutCleansEntireProcessGroup(t *testing.T) {
	runtimeSpec := writeFakeSoffice(t, t.TempDir())
	workspace := t.TempDir()
	childPIDFile := filepath.Join(t.TempDir(), "child.pid")
	worker := NewOfficeRenderWorker(Config{
		// The full repository suite starts many Go package processes at once.
		// Give the helper enough time to be scheduled before exercising the
		// worker-owned timeout; a missing child is otherwise a test-harness
		// scheduling failure rather than evidence about process-tree cleanup.
		Timeout:     8 * time.Second,
		Environment: []string{"AWAKEN_FAKE_CHILD_PID_FILE=" + childPIDFile},
	})
	input := writeInput(t, workspace, "deck.pptx")
	renderDone := make(chan error, 1)
	go func() {
		_, err := worker.Render(context.Background(), Job{
			Runtime:    runtimeSpec,
			InputPath:  input,
			OutputPath: filepath.Join(workspace, "deck.pdf"),
			Format:     "pdf",
		})
		renderDone <- err
	}()
	pid := readChildPID(t, childPIDFile, 7*time.Second)
	select {
	case err := <-renderDone:
		if !errors.Is(err, ErrRenderTimeout) {
			t.Fatalf("expected render timeout, got %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("render did not return after worker timeout")
	}
	assertProcessGone(t, pid)
}

func TestCancellationCleansEntireProcessGroup(t *testing.T) {
	runtimeSpec := writeFakeSoffice(t, t.TempDir())
	workspace := t.TempDir()
	childPIDFile := filepath.Join(t.TempDir(), "child.pid")
	worker := NewOfficeRenderWorker(Config{
		Timeout:     30 * time.Second,
		Environment: []string{"AWAKEN_FAKE_CHILD_PID_FILE=" + childPIDFile},
	})
	input := writeInput(t, workspace, "deck.pptx")
	ctx, cancel := context.WithCancel(context.Background())
	renderDone := make(chan error, 1)
	go func() {
		_, err := worker.Render(ctx, Job{
			Runtime:    runtimeSpec,
			InputPath:  input,
			OutputPath: filepath.Join(workspace, "deck.pdf"),
			Format:     "pdf",
		})
		renderDone <- err
	}()
	pid := readChildPID(t, childPIDFile, 10*time.Second)
	cancel()
	select {
	case err := <-renderDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected cancellation, got %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("render did not return after cancellation")
	}
	assertProcessGone(t, pid)
}

func readChildPID(t *testing.T, path string, timeout time.Duration) int {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		payload, err := os.ReadFile(path)
		if err == nil {
			pid, parseErr := strconv.Atoi(strings.TrimSpace(string(payload)))
			if parseErr != nil {
				t.Fatal(parseErr)
			}
			return pid
		}
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("child pid file was not created within %s", timeout)
	return 0
}

func assertProcessGone(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		killErr := syscall.Kill(pid, 0)
		if errors.Is(killErr, syscall.ESRCH) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("child process %d survived timeout cleanup", pid)
}
