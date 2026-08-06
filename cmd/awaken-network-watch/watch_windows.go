//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const internetSettingsPath = `Software\Microsoft\Windows\CurrentVersion\Internet Settings`

type registryLocation struct {
	root registry.Key
	path string
}

func run() error {
	locations := []registryLocation{
		{root: registry.CURRENT_USER, path: internetSettingsPath},
		{root: registry.LOCAL_MACHINE, path: internetSettingsPath},
		{root: registry.LOCAL_MACHINE, path: `Software\Policies\Microsoft\Windows\CurrentVersion\Internet Settings`},
	}

	changes := make(chan struct{}, 1)
	stopped := make(chan struct{}, len(locations))
	opened := 0
	for _, location := range locations {
		key, err := registry.OpenKey(location.root, location.path, registry.NOTIFY)
		if err != nil {
			continue
		}
		opened++
		go func(key registry.Key) {
			defer key.Close()
			defer func() { stopped <- struct{}{} }()
			watchRegistryKey(key, changes)
		}(key)
	}
	if opened == 0 {
		return errors.New("no proxy registry key is available")
	}

	for opened > 0 {
		select {
		case <-changes:
			if _, err := fmt.Fprintln(os.Stdout, "proxy-change"); err != nil {
				return err
			}
		case <-stopped:
			opened--
		}
	}
	return errors.New("all proxy registry watchers stopped")
}

func watchRegistryKey(key registry.Key, changes chan<- struct{}) {
	event, err := windows.CreateEvent(nil, 0, 0, nil)
	if err != nil {
		return
	}
	defer windows.CloseHandle(event)
	const filter = windows.REG_NOTIFY_CHANGE_NAME |
		windows.REG_NOTIFY_CHANGE_LAST_SET |
		windows.REG_NOTIFY_THREAD_AGNOSTIC
	for {
		if err := windows.RegNotifyChangeKeyValue(windows.Handle(key), true, filter, event, true); err != nil {
			return
		}
		status, err := windows.WaitForSingleObject(event, windows.INFINITE)
		if err != nil || status != windows.WAIT_OBJECT_0 {
			return
		}
		select {
		case changes <- struct{}{}:
		default:
		}
	}
}
