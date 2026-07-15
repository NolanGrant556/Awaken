package tool

import "testing"

func TestClassifyLegacyArtifactRole(t *testing.T) {
	tests := []struct {
		name     string
		producer string
		kind     string
		path     string
		want     ArtifactRole
	}{
		{name: "created document patch is deliverable", producer: "apply_patch", kind: "added", path: "report.xlsx", want: ArtifactRoleDeliverable},
		{name: "modified markdown edit is deliverable", producer: "text_edit", kind: "modified", path: "report.md", want: ArtifactRoleDeliverable},
		{name: "deleted document stays support", producer: "apply_patch", kind: "deleted", path: "report.md", want: ArtifactRoleSupport},
		{name: "moved document stays support", producer: "apply_patch", kind: "moved", path: "renamed.md", want: ArtifactRoleSupport},
		{name: "source patch stays support", producer: "apply_patch", kind: "added", path: "main.go", want: ArtifactRoleSupport},
		{name: "presentation is deliverable", producer: "presentation_create", kind: "presentation", path: "deck.pptx", want: ArtifactRoleDeliverable},
		{name: "automatic presentation pdf is preview", producer: "presentation_create", kind: "pdf", path: "deck_preview/deck.pdf", want: ArtifactRolePreview},
		{name: "render is preview", producer: "artifact_render", kind: "pdf", path: "report.pdf", want: ArtifactRolePreview},
		{name: "office exported pdf is deliverable", producer: "office_export_pdf", kind: "pdf", path: "report.pdf", want: ArtifactRoleDeliverable},
		{name: "legacy markdown create is deliverable", producer: "text_file_create", kind: "text", path: "report.md", want: ArtifactRoleDeliverable},
		{name: "known legacy write file is deliverable", producer: "write_file", kind: "file", path: "report.pdf", want: ArtifactRoleDeliverable},
		{name: "known legacy write file unsupported extension", producer: "write_file", kind: "file", path: "notes.txt", want: ArtifactRoleSupport},
		{name: "unknown supported extension stays support", producer: "custom_writer", kind: "file", path: "report.pdf", want: ArtifactRoleSupport},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ClassifyLegacyArtifactRole(test.producer, test.kind, test.path); got != test.want {
				t.Fatalf("ClassifyLegacyArtifactRole()=%q want %q", got, test.want)
			}
		})
	}
}

func TestResolveArtifactRolePreservesExplicitRole(t *testing.T) {
	if got := ResolveArtifactRole(ArtifactRoleSupport, "presentation_create", "presentation", "deck.pptx"); got != ArtifactRoleSupport {
		t.Fatalf("explicit v2 role must win, got %q", got)
	}
}

func TestPublishableArtifactPathAllowlist(t *testing.T) {
	for _, path := range []string{"report.md", "report.DOCX", "table.xlsx", "data.csv", "final.pdf", "deck.pptx"} {
		if !IsPublishableArtifactPath(path) {
			t.Fatalf("expected %q to be publishable", path)
		}
	}
	for _, path := range []string{"script.py", "preview.png", "notes.txt", "report"} {
		if IsPublishableArtifactPath(path) {
			t.Fatalf("expected %q to be rejected", path)
		}
	}
}
