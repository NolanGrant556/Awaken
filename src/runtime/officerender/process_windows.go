//go:build windows

package officerender

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

const createNewProcessGroup = 0x00000200

func configureProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNewProcessGroup}
}

func terminateProcessGroup(process *os.Process) error {
	if process == nil {
		return os.ErrProcessDone
	}
	command := exec.Command("taskkill.exe", "/PID", fmt.Sprintf("%d", process.Pid), "/T", "/F")
	if err := command.Run(); err != nil {
		return process.Kill()
	}
	return nil
}

func forceKillProcessGroup(process *os.Process) {
	if process != nil {
		_ = exec.Command("taskkill.exe", "/PID", fmt.Sprintf("%d", process.Pid), "/T", "/F").Run()
	}
}
