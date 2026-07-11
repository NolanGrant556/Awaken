package applypatch

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"awaken/src/tool/trash"
)

type deletedEntry struct {
	path        string
	trashedPath string
}

type patchSummary struct {
	added                []string
	overwritten          []string
	modified             []string
	replacedDestinations []string
	deleted              []deletedEntry
	partial              []partialWriteEntry
}

// partialWriteEntry records a resource mutation that was observed inside an
// operation which later failed. It exists so the result protocol can report
// the real partial state instead of flattening the whole operation to unknown.
type partialWriteEntry struct {
	sourcePath      string
	destinationPath string
	action          string
	cause           string
}

func (summary patchSummary) String() string {
	var builder strings.Builder
	builder.WriteString("Success. Updated the following files:\n")
	for _, path := range summary.added {
		fmt.Fprintf(&builder, "A %s\n", path)
	}
	for _, path := range summary.overwritten {
		fmt.Fprintf(&builder, "O %s (Add File replaced existing target)\n", path)
	}
	for _, path := range summary.modified {
		if summaryContainsPath(summary.replacedDestinations, path) {
			fmt.Fprintf(&builder, "M %s (replaced existing destination)\n", path)
		} else {
			fmt.Fprintf(&builder, "M %s\n", path)
		}
	}
	for _, entry := range summary.deleted {
		if entry.trashedPath != "" {
			fmt.Fprintf(&builder, "D %s (已移到回收站: %s)\n", entry.path, entry.trashedPath)
		} else {
			fmt.Fprintf(&builder, "D %s (已移到回收站)\n", entry.path)
		}
	}
	return builder.String()
}

func applyPatch(ctx context.Context, patch *patchDocument) (patchSummary, error) {
	var summary patchSummary
	for _, op := range patch.operations {
		if err := ctx.Err(); err != nil {
			return summary, err
		}
		if err := applyOperation(op, &summary); err != nil {
			return summary, err
		}
	}
	return summary, nil
}

func applyOperation(op patchOperation, summary *patchSummary) error {
	switch op.kind {
	case patchKindAdd:
		destinationAction := destinationWriteAction(op.path)
		if err := writeFileWithParents(op.path, []byte(op.content)); err != nil {
			return fmt.Errorf("failed to add file %s: %w", op.path, err)
		}
		if destinationAction == "overwritten" {
			summary.overwritten = append(summary.overwritten, op.summaryPath())
		} else {
			summary.added = append(summary.added, op.summaryPath())
		}
	case patchKindDelete:
		result, err := deleteFile(op.path)
		if err != nil {
			return err
		}
		summary.deleted = append(summary.deleted, deletedEntry{
			path:        op.summaryPath(),
			trashedPath: result.TrashedPath,
		})
	case patchKindUpdate:
		newContent, err := deriveUpdatedContent(op.path, op.chunks)
		if err != nil {
			return err
		}
		destinationAction := ""
		if op.movePath != "" {
			destinationAction = destinationWriteAction(op.movePath)
			if err := writeFileWithParents(op.movePath, []byte(newContent)); err != nil {
				return fmt.Errorf("failed to write moved file %s: %w", op.movePath, err)
			}
			if !sameCleanPath(op.path, op.movePath) {
				if _, err := deleteFile(op.path); err != nil {
					summary.partial = append(summary.partial, partialWriteEntry{
						sourcePath: op.path, destinationPath: op.movePath,
						action: destinationAction, cause: err.Error(),
					})
					return fmt.Errorf("failed to remove original %s: %w", op.path, err)
				}
			}
		} else if err := writeFileWithParents(op.path, []byte(newContent)); err != nil {
			return fmt.Errorf("failed to update file %s: %w", op.path, err)
		}
		summary.modified = append(summary.modified, op.summaryPath())
		if strings.TrimSpace(op.movePath) != "" && destinationAction == "overwritten" {
			summary.replacedDestinations = append(summary.replacedDestinations, op.summaryPath())
		}
	default:
		return fmt.Errorf("unknown patch operation %q", op.kind)
	}
	return nil
}

func destinationWriteAction(path string) string {
	if _, err := os.Stat(path); err == nil {
		return "overwritten"
	} else if os.IsNotExist(err) {
		return "created"
	}
	return "written"
}

func summaryContainsPath(paths []string, want string) bool {
	for _, path := range paths {
		if sameCleanPath(path, want) {
			return true
		}
	}
	return false
}

func writeFileWithParents(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, content, 0644)
}

// deleteFile moves the file at path to the OS trash via the configured
// TrashAdapter. It does NOT fall back to os.Remove — if the trash adapter
// fails, the deletion fails (fail-closed) so the file remains in place and
// the caller can surface the error.
//
// Sensitive-path and symlink-escape checks are enforced upstream by
// validateMutationPath in apply_patch.go, which runs for every operation
// (including delete) before applyPatch is invoked.
func deleteFile(path string) (trash.TrashResult, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return trash.TrashResult{}, fmt.Errorf("failed to delete file %s: file does not exist", path)
		}
		return trash.TrashResult{}, fmt.Errorf("failed to inspect file %s: %w", path, err)
	}
	if info.IsDir() {
		return trash.TrashResult{}, fmt.Errorf("failed to delete file %s: path is a directory", path)
	}
	result, err := trash.DefaultAdapter().TrashPath(path)
	if err != nil {
		return trash.TrashResult{}, fmt.Errorf("failed to delete file %s: %w", path, err)
	}
	return result, nil
}

func sameCleanPath(left, right string) bool {
	leftAbs, leftErr := filepath.Abs(left)
	rightAbs, rightErr := filepath.Abs(right)
	if leftErr == nil && rightErr == nil {
		return filepath.Clean(leftAbs) == filepath.Clean(rightAbs)
	}
	return filepath.Clean(left) == filepath.Clean(right)
}
