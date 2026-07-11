//go:build windows

package runtime

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

func projectDirectoryIdentity(root string, _ os.FileInfo) (string, error) {
	path, err := windows.UTF16PtrFromString(root)
	if err != nil {
		return "", err
	}
	handle, err := windows.CreateFile(
		path,
		windows.FILE_READ_ATTRIBUTES,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS,
		0,
	)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(handle)
	var identity windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &identity); err != nil {
		return "", err
	}
	return fmt.Sprintf(
		"%d:%d:%d",
		identity.VolumeSerialNumber,
		identity.FileIndexHigh,
		identity.FileIndexLow,
	), nil
}
