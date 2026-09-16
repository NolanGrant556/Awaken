package managedenv

import (
	"fmt"
	"golang.org/x/sys/windows"
)

func officeHostOSVersion() (string, error) {
	v := windows.RtlGetVersion()
	return fmt.Sprintf("%d.%d.%d", v.MajorVersion, v.MinorVersion, v.BuildNumber), nil
}
