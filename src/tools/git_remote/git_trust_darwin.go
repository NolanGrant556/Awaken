//go:build darwin

package gitremote

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"awaken/src/tool"
)

func validateHostTrustedGitRuntime(observation tool.ExecutableObservation) (gitRuntimeTrust, error) {
	if observation.Provenance != "host_trusted_developer_tool" {
		return gitRuntimeTrust{}, fmt.Errorf("Darwin typed Git requires host_trusted_developer_tool provenance")
	}
	gitPath, err := filepath.EvalSymlinks(observation.Path)
	if err != nil || !allowedDarwinDeveloperGitPath(gitPath) {
		return gitRuntimeTrust{}, fmt.Errorf("Darwin typed Git executable is outside trusted Developer toolchains")
	}
	if err := validateRootOwnedExecutable(gitPath); err != nil {
		return gitRuntimeTrust{}, err
	}
	usrRoot := filepath.Dir(filepath.Dir(gitPath))
	execPath, err := filepath.EvalSymlinks(filepath.Join(usrRoot, "libexec", "git-core"))
	if err != nil || !tool.PathWithinRoot(execPath, usrRoot) {
		return gitRuntimeTrust{}, fmt.Errorf("Darwin Git exec path is unavailable or outside its Developer toolchain")
	}
	if err := validateRootOwnedDirectory(execPath); err != nil {
		return gitRuntimeTrust{}, err
	}
	remoteHelper, err := filepath.EvalSymlinks(filepath.Join(execPath, "git-remote-https"))
	if err != nil || !tool.PathWithinRoot(remoteHelper, execPath) {
		return gitRuntimeTrust{}, fmt.Errorf("Darwin Git HTTPS helper is unavailable or outside trusted exec path")
	}
	if err := validateRootOwnedExecutable(remoteHelper); err != nil {
		return gitRuntimeTrust{}, err
	}
	fingerprint, err := tool.FingerprintExecutable(remoteHelper)
	if err != nil {
		return gitRuntimeTrust{}, fmt.Errorf("fingerprint Darwin Git HTTPS helper: %w", err)
	}
	return gitRuntimeTrust{ExecPath: execPath, RemoteHelper: remoteHelper, HelperFingerprint: fingerprint}, nil
}

func allowedDarwinDeveloperGitPath(path string) bool {
	path = filepath.Clean(path)
	if path == "/Library/Developer/CommandLineTools/usr/bin/git" {
		return true
	}
	relative, err := filepath.Rel("/Applications", path)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return false
	}
	parts := strings.Split(relative, string(os.PathSeparator))
	if len(parts) != 6 {
		return false
	}
	appName := strings.ToLower(parts[0])
	return strings.HasPrefix(appName, "xcode") && strings.HasSuffix(appName, ".app") &&
		parts[1] == "Contents" && parts[2] == "Developer" && parts[3] == "usr" && parts[4] == "bin" && parts[5] == "git"
}

func validateRootOwnedExecutable(path string) error {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 || info.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("trusted Git executable/helper must be executable, regular, and not group/world writable")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != 0 {
		return fmt.Errorf("trusted Git executable/helper must be root-owned")
	}
	return nil
}

func validateRootOwnedDirectory(path string) error {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("trusted Git exec path must be a non-group/world-writable directory")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != 0 {
		return fmt.Errorf("trusted Git exec path must be root-owned")
	}
	return nil
}
