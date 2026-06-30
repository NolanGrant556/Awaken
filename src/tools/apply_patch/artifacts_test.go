package applypatch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"awaken/src/tool"
)

// newTestPermissionContext builds a PermissionContext rooted at a temp dir,
// matching the口径 used by the runtime so artifact paths stay consistent
// with approval affected paths.
func newTestPermissionContext(t *testing.T) (tool.PermissionContext, string) {
	t.Helper()
	root := t.TempDir()
	return tool.NewPermissionContext(root, nil), root
}

func TestArtifactsAddOperation(t *testing.T) {
	permissionContext, _ := newTestPermissionContext(t)
	input := "*** Begin Patch\n" +
		"*** Add File: note.md\n" +
		"+hello\n" +
		"*** End Patch"
	artifacts := (&ApplyPatchTool{}).Artifacts(input, "", permissionContext)
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(artifacts))
	}
	a := artifacts[0]
	if a.Kind != "added" {
		t.Fatalf("expected kind added, got %q", a.Kind)
	}
	if a.Action != tool.ArtifactActionCreated {
		t.Fatalf("expected action created, got %q", a.Action)
	}
	if a.Path != "note.md" {
		t.Fatalf("expected path note.md, got %q", a.Path)
	}
	if a.Title != "note.md" {
		t.Fatalf("expected title note.md, got %q", a.Title)
	}
	if a.Description != "Added note.md" {
		t.Fatalf("expected description 'Added note.md', got %q", a.Description)
	}
	if a.Status != "ready" {
		t.Fatalf("expected status ready, got %q", a.Status)
	}
	if a.ID != "apply_patch:added:note.md" {
		t.Fatalf("expected ID 'apply_patch:added:note.md', got %q", a.ID)
	}
	if a.Content != "" {
		t.Fatalf("expected empty content, got %q", a.Content)
	}
}

func TestArtifactsUpdateOperation(t *testing.T) {
	permissionContext, _ := newTestPermissionContext(t)
	input := "*** Begin Patch\n" +
		"*** Update File: note.md\n" +
		"@@\n" +
		"-old\n" +
		"+new\n" +
		"*** End Patch"
	artifacts := (&ApplyPatchTool{}).Artifacts(input, "", permissionContext)
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(artifacts))
	}
	a := artifacts[0]
	if a.Kind != "modified" {
		t.Fatalf("expected kind modified, got %q", a.Kind)
	}
	if a.Action != tool.ArtifactActionUpdated {
		t.Fatalf("expected action updated, got %q", a.Action)
	}
	if a.Path != "note.md" {
		t.Fatalf("expected path note.md, got %q", a.Path)
	}
	if a.Title != "note.md" {
		t.Fatalf("expected title note.md, got %q", a.Title)
	}
	if a.Description != "Modified note.md" {
		t.Fatalf("expected description 'Modified note.md', got %q", a.Description)
	}
	if a.ID != "apply_patch:modified:note.md" {
		t.Fatalf("expected ID 'apply_patch:modified:note.md', got %q", a.ID)
	}
	if a.Status != "ready" {
		t.Fatalf("expected status ready, got %q", a.Status)
	}
}

func TestArtifactsDeleteOperation(t *testing.T) {
	permissionContext, _ := newTestPermissionContext(t)
	input := "*** Begin Patch\n" +
		"*** Delete File: note.md\n" +
		"*** End Patch"
	artifacts := (&ApplyPatchTool{}).Artifacts(input, "", permissionContext)
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(artifacts))
	}
	a := artifacts[0]
	if a.Kind != "deleted" {
		t.Fatalf("expected kind deleted, got %q", a.Kind)
	}
	if a.Action != tool.ArtifactActionUpdated {
		t.Fatalf("expected action updated, got %q", a.Action)
	}
	if a.Path != "note.md" {
		t.Fatalf("expected path note.md, got %q", a.Path)
	}
	if a.Title != "note.md" {
		t.Fatalf("expected title note.md, got %q", a.Title)
	}
	if a.Description != "Deleted note.md" {
		t.Fatalf("expected description 'Deleted note.md', got %q", a.Description)
	}
	if a.ID != "apply_patch:deleted:note.md" {
		t.Fatalf("expected ID 'apply_patch:deleted:note.md', got %q", a.ID)
	}
}

func TestArtifactsMoveOperation(t *testing.T) {
	permissionContext, _ := newTestPermissionContext(t)
	input := "*** Begin Patch\n" +
		"*** Update File: old.md\n" +
		"*** Move to: new.md\n" +
		"@@\n" +
		"-old\n" +
		"+new\n" +
		"*** End Patch"
	artifacts := (&ApplyPatchTool{}).Artifacts(input, "", permissionContext)
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(artifacts))
	}
	a := artifacts[0]
	if a.Kind != "moved" {
		t.Fatalf("expected kind moved, got %q", a.Kind)
	}
	if a.Action != tool.ArtifactActionUpdated {
		t.Fatalf("expected action updated, got %q", a.Action)
	}
	if a.Path != "new.md" {
		t.Fatalf("expected path new.md (movePath), got %q", a.Path)
	}
	if a.Title != "new.md" {
		t.Fatalf("expected title new.md (movePath), got %q", a.Title)
	}
	if a.Description != "Moved old.md to new.md" {
		t.Fatalf("expected description 'Moved old.md to new.md', got %q", a.Description)
	}
	if a.ID != "apply_patch:moved:new.md" {
		t.Fatalf("expected ID 'apply_patch:moved:new.md', got %q", a.ID)
	}
}

func TestArtifactsMultipleOperations(t *testing.T) {
	permissionContext, _ := newTestPermissionContext(t)
	input := "*** Begin Patch\n" +
		"*** Add File: a.md\n" +
		"+content a\n" +
		"*** Update File: b.md\n" +
		"@@\n" +
		"-old\n" +
		"+new\n" +
		"*** Delete File: c.md\n" +
		"*** End Patch"
	artifacts := (&ApplyPatchTool{}).Artifacts(input, "", permissionContext)
	if len(artifacts) != 3 {
		t.Fatalf("expected 3 artifacts, got %d", len(artifacts))
	}
	if artifacts[0].Kind != "added" {
		t.Fatalf("expected first artifact kind added, got %q", artifacts[0].Kind)
	}
	if artifacts[0].Action != tool.ArtifactActionCreated {
		t.Fatalf("expected first artifact action created, got %q", artifacts[0].Action)
	}
	if artifacts[0].Path != "a.md" {
		t.Fatalf("expected first artifact path a.md, got %q", artifacts[0].Path)
	}
	if artifacts[1].Kind != "modified" {
		t.Fatalf("expected second artifact kind modified, got %q", artifacts[1].Kind)
	}
	if artifacts[1].Action != tool.ArtifactActionUpdated {
		t.Fatalf("expected second artifact action updated, got %q", artifacts[1].Action)
	}
	if artifacts[1].Path != "b.md" {
		t.Fatalf("expected second artifact path b.md, got %q", artifacts[1].Path)
	}
	if artifacts[2].Kind != "deleted" {
		t.Fatalf("expected third artifact kind deleted, got %q", artifacts[2].Kind)
	}
	if artifacts[2].Action != tool.ArtifactActionUpdated {
		t.Fatalf("expected third artifact action updated, got %q", artifacts[2].Action)
	}
	if artifacts[2].Path != "c.md" {
		t.Fatalf("expected third artifact path c.md, got %q", artifacts[2].Path)
	}
}

func TestArtifactsInvalidPatchReturnsNil(t *testing.T) {
	permissionContext, _ := newTestPermissionContext(t)
	artifacts := (&ApplyPatchTool{}).Artifacts("not a valid patch", "", permissionContext)
	if artifacts != nil {
		t.Fatalf("expected nil for invalid patch, got %v", artifacts)
	}
}

func TestArtifactsEmptyInputReturnsNil(t *testing.T) {
	permissionContext, _ := newTestPermissionContext(t)
	artifacts := (&ApplyPatchTool{}).Artifacts("", "", permissionContext)
	if artifacts != nil {
		t.Fatalf("expected nil for empty input, got %v", artifacts)
	}
}

func TestPatchPreviewIncludesPatchSummaryAndAffectedPaths(t *testing.T) {
	permissionContext, _ := newTestPermissionContext(t)
	input := "*** Begin Patch\n" +
		"*** Add File: note.md\n" +
		"+hello\n" +
		"*** End Patch"

	preview, ok := (&ApplyPatchTool{}).PatchPreview(input, permissionContext)
	if !ok {
		t.Fatal("expected patch preview")
	}
	if preview.Patch != input {
		t.Fatalf("expected raw patch to be preserved, got %q", preview.Patch)
	}
	if preview.Summary != "新增 1 个文件" {
		t.Fatalf("unexpected summary %q", preview.Summary)
	}
	if len(preview.AffectedPaths) != 1 || preview.AffectedPaths[0] != "note.md" {
		t.Fatalf("unexpected affected paths %+v", preview.AffectedPaths)
	}
}

// TestArtifactsPathConsistencyWithApprovalAffectedPaths verifies that artifact
// paths match the corresponding entries in approval affectedPaths, using the
// same normalization (workspace-relative + forward slash).
func TestArtifactsPathConsistencyWithApprovalAffectedPaths(t *testing.T) {
	permissionContext, root := newTestPermissionContext(t)

	// Create sub dir so we can test nested relative paths.
	subDir := filepath.Join(root, "sub")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("failed to create sub dir: %v", err)
	}

	tests := []struct {
		name          string
		input         string
		wantKind      string
		wantPath      string
		affectedIndex int // index into affectedPaths that should match artifact path
	}{
		{
			name: "absolute path within workspace",
			input: "*** Begin Patch\n" +
				"*** Add File: " + filepath.Join(root, "note.md") + "\n" +
				"+hello\n" +
				"*** End Patch",
			wantKind:      "added",
			wantPath:      "note.md",
			affectedIndex: 0,
		},
		{
			name: "dotdot path normalized",
			input: "*** Begin Patch\n" +
				"*** Update File: sub/../note.md\n" +
				"@@\n" +
				"-old\n" +
				"+new\n" +
				"*** End Patch",
			wantKind:      "modified",
			wantPath:      "note.md",
			affectedIndex: 0,
		},
		{
			name: "nested relative path",
			input: "*** Begin Patch\n" +
				"*** Add File: sub/deep.md\n" +
				"+content\n" +
				"*** End Patch",
			wantKind:      "added",
			wantPath:      "sub/deep.md",
			affectedIndex: 0,
		},
		{
			name: "move uses destination path",
			input: "*** Begin Patch\n" +
				"*** Update File: sub/old.md\n" +
				"*** Move to: sub/new.md\n" +
				"@@\n" +
				"-old\n" +
				"+new\n" +
				"*** End Patch",
			wantKind:      "moved",
			wantPath:      "sub/new.md",
			affectedIndex: 1, // affectedPaths has [sub/old.md, sub/new.md]
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			artifacts := (&ApplyPatchTool{}).Artifacts(tt.input, "", permissionContext)
			if len(artifacts) != 1 {
				t.Fatalf("expected 1 artifact, got %d", len(artifacts))
			}
			a := artifacts[0]

			// Artifact path must be workspace-relative and forward-slashed.
			if a.Path != tt.wantPath {
				t.Fatalf("expected path %q, got %q", tt.wantPath, a.Path)
			}
			if a.Title != tt.wantPath {
				t.Fatalf("expected title %q, got %q", tt.wantPath, a.Title)
			}
			if a.ID != "apply_patch:"+tt.wantKind+":"+tt.wantPath {
				t.Fatalf("expected ID 'apply_patch:%s:%s', got %q", tt.wantKind, tt.wantPath, a.ID)
			}
			if filepath.IsAbs(a.Path) {
				t.Fatalf("artifact path should not be absolute, got %q", a.Path)
			}
			if strings.Contains(a.Path, "\\") {
				t.Fatalf("artifact path should not contain backslashes, got %q", a.Path)
			}

			// Cross-check with approval affectedPaths.
			patch, err := parsePatch(tt.input)
			if err != nil {
				t.Fatalf("failed to parse patch: %v", err)
			}
			affected := patchAffectedPaths(patch, permissionContext)
			if tt.affectedIndex >= len(affected) {
				t.Fatalf("affectedIndex %d out of range for affected %v", tt.affectedIndex, affected)
			}
			if a.Path != affected[tt.affectedIndex] {
				t.Fatalf("artifact path %q does not match affectedPaths[%d]=%q (full affected=%v)",
					a.Path, tt.affectedIndex, affected[tt.affectedIndex], affected)
			}
		})
	}
}
