//go:build !windows

package managedenv

func nativeFileVersion(string) (string, error) {
	return "", failure("environment_platform_unsupported", "PE version probing requires Windows")
}
