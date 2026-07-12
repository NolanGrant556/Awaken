//go:build windows

package trash

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// TrashAdapterWindows sends files and directories to the Windows Recycle Bin
// through the Microsoft.VisualBasic FileIO shell integration. The target path
// is passed only through an environment variable, never interpolated into the
// PowerShell program. Missing PowerShell/Recycle Bin support fails closed.
type TrashAdapterWindows struct {
	run func(path string, isDirectory bool) error
}

func defaultPlatformAdapter() TrashAdapter {
	return TrashAdapterWindows{}
}

func (a TrashAdapterWindows) TrashPath(path string) (TrashResult, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return TrashResult{}, fmt.Errorf("trash: resolve absolute path for %q: %w", path, err)
	}

	info, err := os.Lstat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return TrashResult{}, fmt.Errorf("trash: path does not exist: %s", absPath)
		}
		return TrashResult{}, fmt.Errorf("trash: stat %s: %w", absPath, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || (!info.Mode().IsRegular() && !info.IsDir()) {
		return TrashResult{}, fmt.Errorf("trash: refusing symlink or special filesystem object: %s", absPath)
	}
	run := a.run
	if run == nil {
		run = recycleWindowsPath
	}
	if err := run(absPath, info.IsDir()); err != nil {
		return TrashResult{}, fmt.Errorf("trash: send %s to Windows Recycle Bin: %w", absPath, err)
	}
	if _, err := os.Lstat(absPath); err == nil || !os.IsNotExist(err) {
		return TrashResult{}, fmt.Errorf("trash: Windows Recycle Bin operation did not remove the source path")
	}

	return TrashResult{OriginalPath: absPath, Method: "windows_recycle_bin"}, nil
}

func recycleWindowsPath(path string, isDirectory bool) error {
	method := "DeleteFile"
	if isDirectory {
		method = "DeleteDirectory"
	}
	program := fmt.Sprintf(`$ErrorActionPreference='Stop'; Add-Type -AssemblyName Microsoft.VisualBasic; [Microsoft.VisualBasic.FileIO.FileSystem]::%s($env:AWAKEN_TRASH_TARGET, [Microsoft.VisualBasic.FileIO.UIOption]::OnlyErrorDialogs, [Microsoft.VisualBasic.FileIO.RecycleOption]::SendToRecycleBin)`, method)
	command := exec.Command("powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", program)
	command.Env = append(os.Environ(), "AWAKEN_TRASH_TARGET="+path)
	if output, err := command.CombinedOutput(); err != nil {
		if len(output) > 1024 {
			output = output[:1024]
		}
		return fmt.Errorf("powershell recycle failed: %w: %s", err, output)
	}
	return nil
}
