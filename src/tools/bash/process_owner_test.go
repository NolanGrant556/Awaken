//go:build !windows

package bash

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// This helper is a separate Core-like owner process. SIGKILL deliberately
// prevents all Go defers and registry cleanup from running.
func TestProcessOwnerHostHelper(t *testing.T) {
	root := os.Getenv("SA_BASH_OWNER_TEST_ROOT")
	if root == "" {
		t.Skip("subprocess helper")
	}
	quote := func(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }
	command := "printf '%s %s\\n' \"$$\" \"$(ps -o pgid= -p $$)\" > " + quote(filepath.Join(root, "worker.pid")) +
		"; sleep 30; printf 'UNEXPECTED-END' > " + quote(filepath.Join(root, "unexpected-end"))
	background := os.Getenv("SA_BASH_OWNER_TEST_BACKGROUND") == "1"
	input, _ := json.Marshal(map[string]any{"command": command, "timeout": 60000, "run_in_background": background})
	outcome, err := (&Tool{}).ExecuteOutcome(bashContext(root, ownerTestThreadID(root), "owned-command"), string(input), nil)
	if err != nil || outcome.IsError() {
		t.Fatalf("helper command failed: %+v %v", outcome, err)
	}
	if background {
		select {}
	}
}

func TestCommandsStopWhenTheirOwnerProcessDies(t *testing.T) {
	for _, mode := range []string{"foreground", "background"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			executable, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			owner := exec.Command(executable, "-test.run=^TestProcessOwnerHostHelper$")
			owner.Env = append(os.Environ(), "SA_BASH_OWNER_TEST_ROOT="+root)
			if mode == "background" {
				owner.Env = append(owner.Env, "SA_BASH_OWNER_TEST_BACKGROUND=1")
			}
			if err := owner.Start(); err != nil {
				t.Fatal(err)
			}
			var workerPID, groupID int
			t.Cleanup(func() {
				_ = owner.Process.Kill()
				_ = owner.Wait()
				if workerPID > 0 && processStillRunning(workerPID) {
					_ = syscall.Kill(-groupID, syscall.SIGKILL)
				}
				_ = os.RemoveAll(filepath.Dir(backgroundTaskRoot(ownerTestThreadID(root), "owned-command")))
			})
			deadline := time.Now().Add(8 * time.Second)
			for time.Now().Before(deadline) {
				data, _ := os.ReadFile(filepath.Join(root, "worker.pid"))
				parts := strings.Fields(string(data))
				if len(parts) == 2 {
					workerPID, _ = strconv.Atoi(parts[0])
					groupID, _ = strconv.Atoi(parts[1])
					if workerPID > 0 && groupID > 0 {
						break
					}
				}
				time.Sleep(10 * time.Millisecond)
			}
			if workerPID == 0 || groupID == 0 {
				t.Fatal("owner did not start the controlled command")
			}
			if err := owner.Process.Kill(); err != nil {
				t.Fatal(err)
			}
			_ = owner.Wait()
			deadline = time.Now().Add(2 * time.Second)
			for processStillRunning(workerPID) && time.Now().Before(deadline) {
				time.Sleep(10 * time.Millisecond)
			}
			if processStillRunning(workerPID) {
				t.Fatalf("command pid %d / group %d survived SIGKILL of its owner", workerPID, groupID)
			}
			if _, err := os.Stat(filepath.Join(root, "unexpected-end")); !os.IsNotExist(err) {
				t.Fatalf("command performed a late side effect after owner death: %v", err)
			}
		})
	}
}

func ownerTestThreadID(root string) string {
	return fmt.Sprintf("owner-crash-%x", sha256.Sum256([]byte(root)))
}

func processStillRunning(pid int) bool {
	output, err := exec.Command("ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output()
	state := strings.TrimSpace(string(output))
	return err == nil && state != "" && !strings.HasPrefix(state, "Z")
}
