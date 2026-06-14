package applypatch

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type patchSummary struct {
	added    []string
	modified []string
	deleted  []string
}

func (summary patchSummary) String() string {
	var builder strings.Builder
	builder.WriteString("Success. Updated the following files:\n")
	for _, path := range summary.added {
		fmt.Fprintf(&builder, "A %s\n", path)
	}
	for _, path := range summary.modified {
		fmt.Fprintf(&builder, "M %s\n", path)
	}
	for _, path := range summary.deleted {
		fmt.Fprintf(&builder, "D %s\n", path)
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
		if err := writeFileWithParents(op.path, []byte(op.content)); err != nil {
			return fmt.Errorf("failed to add file %s: %w", op.path, err)
		}
		summary.added = append(summary.added, op.summaryPath())
	case patchKindDelete:
		if err := deleteFile(op.path); err != nil {
			return err
		}
		summary.deleted = append(summary.deleted, op.summaryPath())
	case patchKindUpdate:
		newContent, err := deriveUpdatedContent(op.path, op.chunks)
		if err != nil {
			return err
		}
		if op.movePath != "" {
			if err := writeFileWithParents(op.movePath, []byte(newContent)); err != nil {
				return fmt.Errorf("failed to write moved file %s: %w", op.movePath, err)
			}
			if !sameCleanPath(op.path, op.movePath) {
				if err := deleteFile(op.path); err != nil {
					return fmt.Errorf("failed to remove original %s: %w", op.path, err)
				}
			}
		} else if err := writeFileWithParents(op.path, []byte(newContent)); err != nil {
			return fmt.Errorf("failed to update file %s: %w", op.path, err)
		}
		summary.modified = append(summary.modified, op.summaryPath())
	default:
		return fmt.Errorf("unknown patch operation %q", op.kind)
	}
	return nil
}

func writeFileWithParents(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, content, 0644)
}

func deleteFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("failed to delete file %s: file does not exist", path)
		}
		return fmt.Errorf("failed to inspect file %s: %w", path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("failed to delete file %s: path is a directory", path)
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("failed to delete file %s: %w", path, err)
	}
	return nil
}

func sameCleanPath(left, right string) bool {
	leftAbs, leftErr := filepath.Abs(left)
	rightAbs, rightErr := filepath.Abs(right)
	if leftErr == nil && rightErr == nil {
		return filepath.Clean(leftAbs) == filepath.Clean(rightAbs)
	}
	return filepath.Clean(left) == filepath.Clean(right)
}
