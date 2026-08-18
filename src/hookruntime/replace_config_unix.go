//go:build !windows

package hookruntime

import "os"

func replaceConfigFile(source, target string) error { return os.Rename(source, target) }

func syncConfigDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
