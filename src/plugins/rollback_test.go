package plugins

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"awaken/src/tool/trash"
)

type rollbackFailureTrash struct{ blockParent, heldParent string }

func (a *rollbackFailureTrash) TrashPath(string) (trash.TrashResult, error) {
	// Replacing the parent directory with a regular file makes restoration fail
	// on every platform, including CI processes that can bypass chmod as root.
	a.heldParent = a.blockParent + ".held"
	if err := os.Rename(a.blockParent, a.heldParent); err != nil {
		return trash.TrashResult{}, err
	}
	if err := os.WriteFile(a.blockParent, []byte("injected unavailable directory"), 0600); err != nil {
		return trash.TrashResult{}, err
	}
	return trash.TrashResult{}, errors.New("injected Trash failure plus unavailable restore directory")
}
func (a *rollbackFailureTrash) restoreParent() error {
	if a.heldParent == "" {
		return nil
	}
	if err := os.Remove(a.blockParent); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(a.heldParent, a.blockParent); err != nil {
		return err
	}
	a.heldParent = ""
	return nil
}

func TestFailedRollbackPreservesBackup(t *testing.T) {
	for _, operation := range []string{"uninstall", "replace", "source-edit"} {
		t.Run(operation, func(t *testing.T) {
			adapter := &rollbackFailureTrash{}
			store := NewStoreAtWithTrash(t.TempDir(), adapter)
			source := t.TempDir()
			writePluginFixture(t, source, "rollback-plugin", true)
			installed, err := store.Install(context.Background(), InstallRequest{Source: Source{Kind: SourceLocalPath, Path: source}})
			if err != nil {
				t.Fatal(err)
			}
			previousDigest := installed.Plugin.Digest
			adapter.blockParent = store.packagesRoot
			t.Cleanup(func() { _ = adapter.restoreParent() })
			pattern := ""
			switch operation {
			case "uninstall":
				_, err = store.Uninstall("rollback-plugin")
				pattern = filepath.Join("uninstall-*", "package")
			case "replace":
				writePluginFixture(t, source, "rollback-plugin", true)
				if err := os.WriteFile(filepath.Join(source, "skills", "lint", "SKILL.md"), []byte("---\nname: lint\ndescription: new description\n---\nNew body\n"), 0600); err != nil {
					t.Fatal(err)
				}
				_, err = store.Install(context.Background(), InstallRequest{Source: Source{Kind: SourceLocalPath, Path: source}, Replace: true})
				pattern = filepath.Join("replace-*", "rollback-plugin")
			case "source-edit":
				_, err = store.UpdateMCPSource("rollback-plugin", "mcp_server/mcp.json#local", previousDigest, `{"mcpServers":{"local":{"type":"stdio","command":"npx","args":["--updated"]}}}`)
				pattern = filepath.Join("source-edit-*", "previous")
			}
			if err == nil {
				t.Fatal("expected compound failure")
			}
			matches, err := filepath.Glob(filepath.Join(store.stagingRoot, pattern, "plugin.json"))
			if err != nil || len(matches) != 1 {
				t.Fatalf("only backup was not retained: %v %v", matches, err)
			}
			if _, err := os.Stat(filepath.Join(filepath.Dir(filepath.Dir(matches[0])), rollbackRecordName)); err != nil {
				t.Fatal("recovery metadata missing", err)
			}
			if err := adapter.restoreParent(); err != nil {
				t.Fatal(err)
			}
			reopened := NewStoreAtWithTrash(store.home, adapter)
			if err := reopened.RecoverPendingOperations(); err != nil {
				t.Fatal(err)
			}
			if _, matches, err := verifyTreeDigest(filepath.Join(store.packagesRoot, "rollback-plugin"), previousDigest); err != nil || !matches {
				t.Fatalf("startup did not restore previous package: %t %v", matches, err)
			}
		})
	}
}
