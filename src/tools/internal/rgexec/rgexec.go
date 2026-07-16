package rgexec

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"awaken/src/tool"
)

type Runtime struct {
	Executable  string
	Environment []string
	SnapshotID  string
}

func Resolve(ctx context.Context) (Runtime, error) {
	if snapshot, ok := tool.ExecutionEnvironmentSnapshotFromContext(ctx); ok {
		observed, found := snapshot.Executable("rg")
		if !found || strings.TrimSpace(observed.Path) == "" {
			return Runtime{}, fmt.Errorf("ripgrep (rg) is missing from the pinned run environment")
		}
		pathValue := snapshot.PinnedTransportPATH()
		if snapshot.PATHTruncated && pathValue == "" {
			return Runtime{}, fmt.Errorf("pinned run PATH is unavailable")
		}
		return Runtime{Executable: observed.Path, Environment: withPATH(os.Environ(), pathValue), SnapshotID: snapshot.SnapshotID}, nil
	}
	executable, err := exec.LookPath("rg")
	if err != nil {
		return Runtime{}, fmt.Errorf("ripgrep (rg) is unavailable: %w", err)
	}
	return Runtime{Executable: filepath.Clean(executable), Environment: os.Environ()}, nil
}

func (r Runtime) Command(ctx context.Context, cwd string, args ...string) *exec.Cmd {
	command := exec.CommandContext(ctx, r.Executable, args...)
	command.Dir = cwd
	command.Env = append([]string(nil), r.Environment...)
	return command
}

func withPATH(environment []string, pathValue string) []string {
	result := make([]string, 0, len(environment)+1)
	for _, entry := range environment {
		if strings.HasPrefix(strings.ToUpper(entry), "PATH=") {
			continue
		}
		result = append(result, entry)
	}
	return append(result, "PATH="+pathValue)
}
