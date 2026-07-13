package officerender

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// HostRuntimeResolver resolves the user's existing LibreOffice installation.
// It does not copy, mutate, sign, or version the application.
type HostRuntimeResolver struct{}

func NewHostRuntimeResolver() *HostRuntimeResolver { return &HostRuntimeResolver{} }

func hostSofficeCandidates() []string {
	candidates := []string{strings.TrimSpace(os.Getenv("AWAKEN_SOFFICE"))}
	for _, name := range []string{"soffice", "libreoffice"} {
		if path, err := exec.LookPath(name); err == nil {
			candidates = append(candidates, path)
		}
	}
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "darwin":
		candidates = append(candidates,
			"/Applications/LibreOffice.app/Contents/MacOS/soffice",
			filepath.Join(home, "Applications", "LibreOffice.app", "Contents", "MacOS", "soffice"),
		)
	case "windows":
		for _, root := range []string{os.Getenv("PROGRAMFILES"), os.Getenv("PROGRAMFILES(X86)"), os.Getenv("LOCALAPPDATA")} {
			if strings.TrimSpace(root) != "" {
				candidates = append(candidates, filepath.Join(root, "LibreOffice", "program", "soffice.exe"))
			}
		}
	default:
		candidates = append(candidates, "/usr/bin/libreoffice", "/usr/bin/soffice", "/usr/local/bin/libreoffice")
	}
	return candidates
}

func resolveHostSoffice() (string, error) {
	for _, candidate := range hostSofficeCandidates() {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		absolute, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		if canonical, err := filepath.EvalSymlinks(absolute); err == nil {
			absolute = canonical
		}
		info, err := os.Stat(absolute)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
			continue
		}
		return absolute, nil
	}
	return "", fmt.Errorf("LibreOffice dependency missing: soffice was not found in PATH or standard application locations")
}

func hostRuntimeForPath(path string) (Runtime, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Runtime{}, err
	}
	identity := fmt.Sprintf("%s\x00%d\x00%d", path, info.Size(), info.ModTime().UnixNano())
	digest := sha256.Sum256([]byte(identity))
	fingerprint := hex.EncodeToString(digest[:])
	return Runtime{
		Revision:    fingerprint,
		Root:        filepath.Dir(path),
		SOfficePath: path,
		Attestation: fingerprint,
	}, nil
}

func (r *HostRuntimeResolver) Resolve(ctx context.Context) (Runtime, error) {
	if err := ctx.Err(); err != nil {
		return Runtime{}, err
	}
	path, err := resolveHostSoffice()
	if err != nil {
		return Runtime{}, err
	}
	return hostRuntimeForPath(path)
}

func (r *HostRuntimeResolver) Attest(ctx context.Context, expected Runtime) error {
	actual, err := r.Resolve(ctx)
	if err != nil {
		return err
	}
	if expected.Revision != actual.Revision || expected.Attestation != actual.Attestation ||
		filepath.Clean(expected.SOfficePath) != filepath.Clean(actual.SOfficePath) {
		return fmt.Errorf("%w: LibreOffice changed before execution", ErrRuntimeAttestation)
	}
	return nil
}

func (r *HostRuntimeResolver) NewWorker(config Config) *OfficeRenderWorker {
	extraAttestor := config.Attest
	config.ResolveRuntime = r.Resolve
	config.Attest = func(ctx context.Context, runtimeSpec Runtime) error {
		if err := r.Attest(ctx, runtimeSpec); err != nil {
			return err
		}
		if extraAttestor != nil {
			return extraAttestor(ctx, runtimeSpec)
		}
		return nil
	}
	return NewOfficeRenderWorker(config)
}
