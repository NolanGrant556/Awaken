//go:build windows

package managedenv

import (
	"os/exec"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	h, e := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if e != nil {
		return false
	}
	defer windows.CloseHandle(h)
	var code uint32
	return windows.GetExitCodeProcess(h, &code) == nil && code == 259
}

// Suspend before assignment: no package script can escape the job between
// CreateProcess and AssignProcessToJobObject. Closing Core's handle kills it.
func startOwned(cmd *exec.Cmd) (func(), error) {
	job, e := windows.CreateJobObject(nil, nil)
	if e != nil {
		return nil, e
	}
	release := func() { windows.CloseHandle(job) }
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, e = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); e != nil {
		release()
		return nil, e
	}
	cmd.SysProcAttr = &windows.SysProcAttr{CreationFlags: windows.CREATE_SUSPENDED | windows.CREATE_NEW_PROCESS_GROUP}
	cmd.WaitDelay = 2 * time.Second
	cmd.Cancel = func() error { return windows.TerminateJobObject(job, 1) }
	if e = cmd.Start(); e != nil {
		release()
		return nil, e
	}
	fail := func(err error) (func(), error) { _ = cmd.Process.Kill(); _ = cmd.Wait(); release(); return nil, err }
	process, e := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if e != nil {
		return fail(e)
	}
	e = windows.AssignProcessToJobObject(job, process)
	windows.CloseHandle(process)
	if e != nil {
		return fail(e)
	}
	snapshot, e := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if e != nil {
		return fail(e)
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ThreadEntry32{Size: uint32(unsafe.Sizeof(windows.ThreadEntry32{}))}
	e = windows.Thread32First(snapshot, &entry)
	for e == nil {
		if entry.OwnerProcessID == uint32(cmd.Process.Pid) {
			thread, err := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, entry.ThreadID)
			if err != nil {
				return fail(err)
			}
			_, err = windows.ResumeThread(thread)
			windows.CloseHandle(thread)
			if err != nil {
				return fail(err)
			}
			return release, nil
		}
		e = windows.Thread32Next(snapshot, &entry)
	}
	return fail(e)
}
