//go:build windows

package managedenv

import (
	"fmt"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Chrome's Windows GUI executable does not provide a reliable stdout version
// probe. Read its PE version resource without starting a browser window.
func nativeFileVersion(path string) (string, error) {
	if !strings.HasPrefix(path, `\\?\`) {
		if strings.HasPrefix(path, `\\`) {
			path = `\\?\UNC\` + strings.TrimPrefix(path, `\\`)
		} else {
			path = `\\?\` + path
		}
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	library := windows.NewLazySystemDLL("version.dll")
	size, _, err := library.NewProc("GetFileVersionInfoSizeW").Call(uintptr(unsafe.Pointer(name)), 0)
	if size == 0 || size > 16<<20 {
		return "", fmt.Errorf("native version resource unavailable: %v", err)
	}
	data := make([]byte, int(size))
	ok, _, err := library.NewProc("GetFileVersionInfoW").Call(uintptr(unsafe.Pointer(name)), 0, size, uintptr(unsafe.Pointer(&data[0])))
	if ok == 0 {
		return "", fmt.Errorf("native version resource unreadable: %v", err)
	}
	query, _ := windows.UTF16PtrFromString(`\`)
	var value unsafe.Pointer
	var length uint32
	ok, _, err = library.NewProc("VerQueryValueW").Call(uintptr(unsafe.Pointer(&data[0])), uintptr(unsafe.Pointer(query)), uintptr(unsafe.Pointer(&value)), uintptr(unsafe.Pointer(&length)))
	if ok == 0 || value == nil || length < 52 {
		return "", fmt.Errorf("native version resource invalid: %v", err)
	}
	info := (*[13]uint32)(value)
	if info[0] != 0xfeef04bd {
		return "", fmt.Errorf("native version signature invalid")
	}
	return fmt.Sprintf("%d.%d.%d.%d", info[2]>>16, info[2]&0xffff, info[3]>>16, info[3]&0xffff), nil
}
