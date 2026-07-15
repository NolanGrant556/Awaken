package textedit

import (
	"fmt"
	"path/filepath"
	"strings"

	"awaken/src/tool"
)

func buildTextEditArtifacts(relPath string, replacementCount int) []tool.ArtifactDescriptor {
	path := filepath.ToSlash(relPath)
	return []tool.ArtifactDescriptor{{
		ContractVersion: tool.ArtifactContractVersion,
		Role:            tool.ResolveArtifactRole("", "text_edit", "modified", path),
		Producer:        "text_edit",
		Action:          tool.ArtifactActionUpdated,
		ID:              "text_edit:" + path,
		Title:           path,
		Description:     fmt.Sprintf("Modified %s (%d replacement(s))", path, replacementCount),
		Kind:            "modified",
		Path:            path,
		Status:          "ready",
	}}
}

func relativeEditPath(path string, root string) string {
	if strings.TrimSpace(root) == "" {
		return filepath.Clean(path)
	}
	absPath, err1 := filepath.Abs(path)
	absRoot, err2 := filepath.Abs(root)
	if err1 != nil || err2 != nil {
		return filepath.Clean(path)
	}
	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return absPath
	}
	return rel
}
