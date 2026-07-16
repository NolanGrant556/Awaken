//go:build windows

package bash

import (
	"fmt"
	"os/exec"
	"time"

	"golang.org/x/sys/windows"
)

func configureProcessTree(command *exec.Cmd) {
	command.SysProcAttr = &windows.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
	command.Cancel = func() error {
		if command.Process == nil {
			return nil
		}
		killer := exec.Command("taskkill", "/PID", fmt.Sprintf("%d", command.Process.Pid), "/T", "/F")
		if err := killer.Run(); err == nil {
			return nil
		}
		return command.Process.Kill()
	}
	command.WaitDelay = 2 * time.Second
}
