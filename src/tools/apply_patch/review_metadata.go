package applypatch

import (
	"fmt"
	"strings"

	"awaken/src/tool"
)

func buildDiffReviewMetadata(patch *patchDocument, permissionContext tool.PermissionContext) *tool.DiffReviewMetadata {
	if patch == nil {
		return nil
	}

	metadata := &tool.DiffReviewMetadata{
		AffectedPaths: patchAffectedPaths(patch, permissionContext),
		Risk: tool.DiffReviewRiskSummary{
			Level: patchRiskTone(patch, permissionContext),
		},
	}

	var (
		hasDelete          bool
		hasMove            bool
		hasOutsideWritable bool
	)
	for _, op := range patch.operations {
		switch op.kind {
		case patchKindAdd:
			metadata.FileCounts.Added++
			metadata.LineCounts.Added += strings.Count(op.content, "\n")
		case patchKindDelete:
			metadata.FileCounts.Deleted++
			hasDelete = true
		case patchKindUpdate:
			metadata.FileCounts.Modified++
			if strings.TrimSpace(op.movePath) != "" {
				hasMove = true
			}
			for _, chunk := range op.chunks {
				metadata.LineCounts.Added += chunk.addedLines
				metadata.LineCounts.Deleted += chunk.deletedLines
			}
		}

		if validateMutationPath(op.path, permissionContext) != nil {
			hasOutsideWritable = true
		}
		if strings.TrimSpace(op.movePath) != "" && validateMutationPath(op.movePath, permissionContext) != nil {
			hasOutsideWritable = true
		}
	}

	metadata.Risk.Reasons = buildRiskReasons(hasDelete, hasMove, hasOutsideWritable)
	metadata.Risk.Summary = buildRiskSummary(metadata, hasDelete, hasMove, hasOutsideWritable)
	return metadata
}

func buildRiskReasons(hasDelete, hasMove, hasOutsideWritable bool) []string {
	reasons := make([]string, 0, 3)
	if hasDelete {
		reasons = append(reasons, "deletes_files")
	}
	if hasMove {
		reasons = append(reasons, "moves_files")
	}
	if hasOutsideWritable {
		reasons = append(reasons, "touches_paths_outside_writable_roots")
	}
	if len(reasons) == 0 {
		reasons = append(reasons, "writes_within_writable_roots")
	}
	return reasons
}

func buildRiskSummary(metadata *tool.DiffReviewMetadata, hasDelete, hasMove, hasOutsideWritable bool) string {
	base := fmt.Sprintf(
		"%d file changes, +%d/-%d lines.",
		totalFileChanges(metadata.FileCounts),
		metadata.LineCounts.Added,
		metadata.LineCounts.Deleted,
	)
	if hasOutsideWritable {
		return base + " Includes paths outside writable roots."
	}
	if hasDelete && hasMove {
		return base + " Includes deletes and moves."
	}
	if hasDelete {
		return base + " Includes file deletions."
	}
	if hasMove {
		return base + " Includes file moves."
	}
	return base + " Stays within writable roots."
}

func totalFileChanges(counts tool.DiffReviewFileCounts) int {
	return counts.Added + counts.Modified + counts.Deleted
}
