//go:build !windows

package bash

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// The write end stays exclusively in the owner process. A small shell in the
// command's own group waits for EOF, so SIGKILL of Core cannot orphan an active
// command. Normal completion reaps the monitor before the group leader exits;
// it never sends a signal using a PID from an earlier execution.
const processOwnerScript = `
( IFS= read -r awaken_owner_signal <&3 || kill -s KILL 0 ) </dev/null >/dev/null 2>&1 &
awaken_monitor_pid=$!
"$@" 3<&- <&0 &
awaken_command_pid=$!
wait "$awaken_command_pid"
awaken_command_status=$?
kill "$awaken_monitor_pid" 2>/dev/null
wait "$awaken_monitor_pid" 2>/dev/null
exit "$awaken_command_status"
`

func configureProcessTree(command *exec.Cmd) (func(), error) {
	if len(command.ExtraFiles) != 0 {
		return nil, fmt.Errorf("command owner pipe requires an unused descriptor 3")
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("create command owner pipe: %w", err)
	}
	args := []string{"/bin/sh", "-c", processOwnerScript, "awaken-command-owner", command.Path}
	command.Path, command.Args = "/bin/sh", append(args, command.Args[1:]...)
	command.ExtraFiles = []*os.File{reader}
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error {
		if command.Process == nil {
			return nil
		}
		err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return nil
		}
		return err
	}
	command.WaitDelay = 2 * time.Second
	return func() {
		_ = writer.Close()
		_ = reader.Close()
	}, nil
}
