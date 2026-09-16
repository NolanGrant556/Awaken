package managedenv

import (
	"fmt"
	"golang.org/x/sys/unix"
)

func processIdentity(pid int) string {
	info, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%d:%d", info.Proc.P_starttime.Sec, info.Proc.P_starttime.Usec)
}
