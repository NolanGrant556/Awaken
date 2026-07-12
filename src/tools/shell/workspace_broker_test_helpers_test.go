package shelltool

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"awaken/src/tool"
	"awaken/src/tool/trash"
)

type shellTestTrashAdapter struct {
	directory string
	calls     int
}

func (a *shellTestTrashAdapter) TrashPath(path string) (trash.TrashResult, error) {
	a.calls++
	if err := os.MkdirAll(a.directory, 0o700); err != nil {
		return trash.TrashResult{}, err
	}
	target := filepath.Join(a.directory, fmt.Sprintf("recovery-%d", a.calls))
	if err := os.Rename(path, target); err != nil {
		return trash.TrashResult{}, err
	}
	return trash.TrashResult{OriginalPath: path, TrashedPath: target, Method: "test-rename"}, nil
}

func newShellTestWorkspaceBroker(t *testing.T) (*tool.WorkspaceExecutionBroker, *shellTestTrashAdapter) {
	t.Helper()
	adapter := &shellTestTrashAdapter{directory: filepath.Join(t.TempDir(), "trash")}
	broker := tool.NewWorkspaceExecutionBrokerWithOptions(tool.WorkspaceExecutionBrokerOptions{
		StagingParent: t.TempDir(), TrashAdapter: adapter,
	})
	return broker, adapter
}
