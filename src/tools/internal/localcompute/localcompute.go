package localcompute

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"awaken/src/internal/outputlimit"
	"awaken/src/tool"
)

type CommandSpec struct {
	Backend     string
	Args        []string
	InputPaths  []string
	OutputPaths []string
	Workdir     string
	Timeout     time.Duration
}

type Result struct {
	Stdout string
	Stderr string
}

var backendCandidates = map[string][]string{
	"pdftotext": {"pdftotext", "pdftotext.exe"},
}

func Run(ctx context.Context, permCtx tool.PermissionContext, spec CommandSpec) (Result, error) {
	if strings.TrimSpace(spec.Backend) == "" {
		return Result{}, fmt.Errorf("local compute backend is required")
	}
	if strings.EqualFold(strings.TrimSpace(spec.Backend), "soffice") ||
		strings.EqualFold(strings.TrimSpace(spec.Backend), "libreoffice") {
		return Result{}, fmt.Errorf("direct soffice execution is disabled; use OfficeRenderWorker")
	}
	for _, input := range spec.InputPaths {
		if _, err := permCtx.ResolveReadablePathChecked(input); err != nil {
			return Result{}, err
		}
	}
	for _, output := range spec.OutputPaths {
		if err := validateOutputPath(permCtx, output); err != nil {
			return Result{}, err
		}
	}

	binary, err := resolveBackendBinary(spec.Backend)
	if err != nil {
		return Result{}, err
	}
	workdir := strings.TrimSpace(spec.Workdir)
	if workdir == "" {
		workdir = permCtx.TemporaryWorkRoot
	}
	if workdir == "" {
		workdir = filepath.Join(os.TempDir(), "awaken-task-scope")
	}
	if err := os.MkdirAll(workdir, 0o755); err != nil {
		return Result{}, err
	}

	if spec.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, spec.Timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, binary, spec.Args...)
	cmd.Dir = workdir
	cmd.Env = minimalEnv()
	output, err := cmd.CombinedOutput()
	result := Result{Stdout: string(output)}
	if err != nil {
		detail := strings.TrimSpace(string(output))
		if detail != "" {
			return result, fmt.Errorf("local compute %s failed: %w: %s", spec.Backend, err, truncateDetail(detail, 1200))
		}
		return result, fmt.Errorf("local compute %s failed: %w", spec.Backend, err)
	}
	return result, nil
}

func validateOutputPath(permCtx tool.PermissionContext, path string) error {
	resolved := permCtx.ResolvePath(path)
	if strings.TrimSpace(permCtx.TemporaryWorkRoot) != "" && tool.PathWithinRoot(resolved, permCtx.TemporaryWorkRoot) {
		return nil
	}
	_, err := permCtx.ResolveOutputPathChecked(path)
	return err
}

func resolveBackendBinary(name string) (string, error) {
	candidates := backendCandidates[name]
	if len(candidates) == 0 {
		candidates = []string{name}
	}
	for _, candidate := range candidates {
		if path, err := exec.LookPath(candidate); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("local compute backend %q is not available", name)
}

func truncateDetail(value string, limit int) string {
	if limit <= 0 {
		return value
	}
	prefix, truncated := outputlimit.Prefix(value, limit)
	if !truncated {
		return prefix
	}
	return prefix + "... [truncated]"
}

func minimalEnv() []string {
	allowed := []string{"PATH", "HOME", "USERPROFILE", "TMPDIR", "TMP", "TEMP", "SystemRoot", "WINDIR"}
	env := make([]string, 0, len(allowed))
	for _, key := range allowed {
		if value := os.Getenv(key); strings.TrimSpace(value) != "" {
			env = append(env, key+"="+value)
		}
	}
	return env
}
