package a2ui

import (
	"encoding/json"
	"testing"
)

func TestApprovalSurfaceJSONBuildsReadOnlySurface(t *testing.T) {
	content, err := ApprovalSurfaceJSON(ApprovalSurfaceRequest{
		ApprovalID:  "approval-1",
		Title:       "Approve write_file",
		Description: "Write README.md",
		RiskTone:    "danger",
		ToolName:    "write_file",
		ToolCallID:  "tool-1",
	})
	if err != nil {
		t.Fatalf("ApprovalSurfaceJSON returned error: %v", err)
	}

	var surface Surface
	if err := json.Unmarshal([]byte(content), &surface); err != nil {
		t.Fatalf("surface content is not valid JSON: %v", err)
	}

	if surface.Version != Version {
		t.Fatalf("expected version %q, got %q", Version, surface.Version)
	}
	if !surface.Metadata.ReadOnly {
		t.Fatalf("expected read-only metadata")
	}
	if surface.UpdateComponents.SurfaceID != ApprovalSurfaceID {
		t.Fatalf("unexpected surface id %q", surface.UpdateComponents.SurfaceID)
	}

	foundDangerBadge := false
	foundApproveAction := false
	foundPatchSummary := false
	foundAffectedPaths := false
	for _, component := range surface.UpdateComponents.Components {
		if component.Component == "Badge" && component.Tone == "danger" {
			foundDangerBadge = true
		}
		if component.Component == "Button" && component.Action != nil && component.Action.Event.Name == "approval.approve" {
			foundApproveAction = true
			if !component.Disabled {
				t.Fatalf("expected generated approval action button to be disabled")
			}
		}
		if component.ID == "patch-summary" && component.Label == "Summary" {
			foundPatchSummary = true
		}
		if component.ID == "affected-paths" && component.Label == "Affected paths" {
			foundAffectedPaths = true
		}
	}
	if !foundDangerBadge {
		t.Fatalf("expected danger badge in generated surface")
	}
	if !foundApproveAction {
		t.Fatalf("expected approve action in generated surface")
	}
	if !foundPatchSummary {
		t.Fatalf("expected patch summary component in generated surface")
	}
	if !foundAffectedPaths {
		t.Fatalf("expected affected paths component in generated surface")
	}
}

func TestApprovalSurfaceIncludesAffectedPathsAndPatchSummary(t *testing.T) {
	content, err := ApprovalSurfaceJSON(ApprovalSurfaceRequest{
		ApprovalID:    "approval-1",
		Title:         "Approve apply_patch",
		Description:   "Modify files",
		RiskTone:      "warning",
		ToolName:      "apply_patch",
		ToolCallID:    "tool-1",
		AffectedPaths: []string{"note.md", "README.md"},
		PatchSummary:  "新增 1 个文件，修改 1 个文件",
	})
	if err != nil {
		t.Fatalf("ApprovalSurfaceJSON returned error: %v", err)
	}

	var surface Surface
	if err := json.Unmarshal([]byte(content), &surface); err != nil {
		t.Fatalf("surface content is not valid JSON: %v", err)
	}

	for _, component := range surface.UpdateComponents.Components {
		if component.ID == "patch-summary" && component.Value != "新增 1 个文件，修改 1 个文件" {
			t.Fatalf("unexpected patch summary value %q", component.Value)
		}
		if component.ID == "affected-paths" && component.Value != "note.md\nREADME.md" {
			t.Fatalf("unexpected affected paths value %q", component.Value)
		}
	}
}
