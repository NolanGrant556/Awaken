//go:build !windows

package managedenv

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"time"
)

func processAlive(pid int) bool {
	return pid > 0 && (syscall.Kill(pid, 0) == nil || errors.Is(syscall.Kill(pid, 0), syscall.EPERM))
}

// The pipe writer is held only by Core; its death terminates the install group.
const ownerScript = `
( IFS= read -r awaken_owner_signal <&3 || kill -s KILL 0 ) </dev/null >/dev/null 2>&1 &
awaken_monitor_pid=$!
"$@" 3<&- <&0 &
awaken_command_pid=$!
wait "$awaken_command_pid"
awaken_status=$?
kill "$awaken_monitor_pid" 2>/dev/null
wait "$awaken_monitor_pid" 2>/dev/null
exit "$awaken_status"
`

func startOwned(cmd *exec.Cmd) (func(), error) {
	r, w, e := os.Pipe()
	if e != nil {
		return nil, e
	}
	args := append([]string{"/bin/sh", "-c", ownerScript, "awaken-environment-owner", cmd.Path}, cmd.Args[1:]...)
	cmd.Path = "/bin/sh"
	cmd.Args = args
	cmd.ExtraFiles = []*os.File{r}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = 2 * time.Second
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		e := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(e, syscall.ESRCH) {
			return nil
		}
		return e
	}
	release := func() { r.Close(); w.Close() }
	if e = cmd.Start(); e != nil {
		release()
		return nil, e
	}
	return release, nil
}
