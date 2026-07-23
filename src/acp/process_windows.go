//go:build windows

package acp

import (
	"fmt"
	"os/exec"
	"syscall"
	"time"
)

func ConfigureProcessTree(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | syscall.CREATE_UNICODE_ENVIRONMENT,
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
	killer := exec.Command("taskkill", "/PID", fmt.Sprintf("%d", cmd.Process.Pid), "/T", "/F")
	killer.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
	if err := killer.Run(); err == nil {
		return nil
	}
	return cmd.Process.Kill()
}

func envKeyIsCaseInsensitive() bool { return true }

func normalizeEnvKey(k string) string { return k }

func shouldOverrideKey(key, existing string) bool { return true }
