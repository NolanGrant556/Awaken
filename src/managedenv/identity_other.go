//go:build !darwin && !windows

package managedenv

import (
	"fmt"
	"os"
	"strings"
)

func processIdentity(pid int) string {
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return ""
	}
	end := strings.LastIndex(string(b), ")")
	if end < 0 {
		return ""
	}
	fields := strings.Fields(string(b[end+1:]))
	if len(fields) < 20 {
		return ""
	}
	return fields[19]
}
