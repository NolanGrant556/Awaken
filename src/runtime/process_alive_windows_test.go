//go:build windows

package runtime

import (
	"os"
	"os/exec"
	"testing"
)

func TestProcessAliveChecksProcessExitState(t *testing.T) {
	if !processAlive(os.Getpid()) {
		t.Fatal("current process must be alive")
	}

	cmd := exec.Command("cmd.exe", "/d", "/c", "exit", "0")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start short-lived process: %v", err)
	}
	pid := cmd.Process.Pid
	if err := cmd.Wait(); err != nil {
		t.Fatalf("wait for short-lived process: %v", err)
	}
	if processAlive(pid) {
		t.Fatalf("exited process %d must not be reported alive", pid)
	}
}
