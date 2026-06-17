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
	}
	if !foundDangerBadge {
		t.Fatalf("expected danger badge in generated surface")
	}
	if !foundApproveAction {
		t.Fatalf("expected approve action in generated surface")
	}
}
