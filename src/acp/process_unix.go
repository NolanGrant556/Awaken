//go:build !windows

package acp

import (
	"os/exec"
	"syscall"
	"time"
)

func ConfigureProcessTree(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
	cmd.Cancel = func() error {
		return killProcessTree(cmd)
	}
	cmd.WaitDelay = 2 * time.Second
}

func killProcessTree(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	// Kill the entire process group.
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
		return cmd.Process.Kill()
	}
	return nil
}

func envKeyIsCaseInsensitive() bool { return false }

func normalizeEnvKey(k string) string { return k }

func shouldOverrideKey(key, existing string) bool { return true }
