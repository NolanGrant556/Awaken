package managedenv

import (
	"fmt"
	"golang.org/x/sys/windows"
)

func processIdentity(pid int) string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	var created, exit, kernel, user windows.Filetime
	if windows.GetProcessTimes(h, &created, &exit, &kernel, &user) != nil {
		return ""
	}
	return fmt.Sprintf("%d:%d", created.HighDateTime, created.LowDateTime)
}
