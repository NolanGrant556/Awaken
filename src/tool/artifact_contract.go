package tool

import (
	"path/filepath"
	"strings"
)

var publishableArtifactExtensions = map[string]struct{}{
	".csv":  {},
	".doc":  {},
	".docx": {},
	".md":   {},
	".pdf":  {},
	".ppt":  {},
	".pptx": {},
	".tsv":  {},
	".xls":  {},
	".xlsx": {},
}

// ArtifactContractVersionOrLegacy preserves an explicitly recorded version and
// identifies records predating the v2 role contract as legacy v1 records.
func ArtifactContractVersionOrLegacy(version int) int {
	if version > 0 {
		return version
	}
	return ArtifactLegacyContractVersion
}

// IsArtifactRole reports whether role is part of the v2 public contract.
func IsArtifactRole(role ArtifactRole) bool {
	switch role {
	case ArtifactRoleDeliverable, ArtifactRolePreview, ArtifactRoleSupport:
		return true
	default:
		return false
	}
}

// ResolveArtifactRole returns an explicit valid v2 role, otherwise applying the
// single canonical legacy classifier used by live mapping and replay.
func ResolveArtifactRole(role ArtifactRole, producer, kind, path string) ArtifactRole {
	if IsArtifactRole(role) {
		return role
	}
	return ClassifyLegacyArtifactRole(producer, kind, path)
}

// ClassifyLegacyArtifactRole adapts artifacts recorded before v2 without
// rewriting history. Producer-specific rules take precedence over extensions so
// generic edits never become final deliverables merely because they touched an
// Office or Markdown file.
func ClassifyLegacyArtifactRole(producer, kind, path string) ArtifactRole {
	producer = strings.ToLower(strings.TrimSpace(producer))
	kind = strings.ToLower(strings.TrimSpace(kind))
	ext := strings.ToLower(filepath.Ext(strings.TrimSpace(path)))

	switch producer {
	case "apply_patch", "text_edit":
		return ArtifactRoleSupport
	}
	switch kind {
	case "added", "deleted", "modified", "moved", "patch", "file_change":
		return ArtifactRoleSupport
	}

	if producer == "artifact_publish" {
		return ArtifactRoleDeliverable
	}
	if isRenderProducer(producer) {
		return ArtifactRolePreview
	}

	switch producer {
	case "presentation_create", "presentation_edit":
		switch ext {
		case ".ppt", ".pptx":
			return ArtifactRoleDeliverable
		case ".pdf", ".png", ".jpg", ".jpeg", ".webp":
			return ArtifactRolePreview
		default:
			return ArtifactRoleSupport
		}
	case "document_create", "document_edit":
		switch ext {
		case ".doc", ".docx", ".md":
			return ArtifactRoleDeliverable
		case ".pdf", ".png", ".jpg", ".jpeg", ".webp":
			return ArtifactRolePreview
		default:
			return ArtifactRoleSupport
		}
	case "spreadsheet_create", "spreadsheet_edit":
		switch ext {
		case ".xls", ".xlsx", ".csv", ".tsv":
			return ArtifactRoleDeliverable
		case ".pdf", ".png", ".jpg", ".jpeg", ".webp":
			return ArtifactRolePreview
		default:
			return ArtifactRoleSupport
		}
	case "artifact_export":
		if IsPublishableArtifactPath(path) {
			return ArtifactRoleDeliverable
		}
		return ArtifactRoleSupport
	case "write_file":
		// Historical Awaken builds exposed write_file as a dedicated producer.
		// The explicit producer name is required; unknown producers stay hidden.
		if IsPublishableArtifactPath(path) {
			return ArtifactRoleDeliverable
		}
		return ArtifactRoleSupport
	case "text_file_create":
		// Only the compatibility adapter promotes legacy Markdown. New calls
		// declare support unless publish_as_deliverable was explicitly set.
		if ext == ".md" {
			return ArtifactRoleDeliverable
		}
		return ArtifactRoleSupport
	case "spreadsheet_chart", "image_convert":
		return ArtifactRolePreview
	}

	switch kind {
	case "preview", "render", "image", "thumbnail":
		return ArtifactRolePreview
	}
	return ArtifactRoleSupport
}

func isRenderProducer(producer string) bool {
	return producer == "artifact_render" ||
		producer == "presentation_render" ||
		producer == "document_render" ||
		producer == "pdf_render" ||
		strings.HasSuffix(producer, "_render")
}

// IsPublishableArtifactPath is the shared allowlist for explicit final-file
// publication. It intentionally excludes source code and render images.
func IsPublishableArtifactPath(path string) bool {
	_, ok := publishableArtifactExtensions[strings.ToLower(filepath.Ext(strings.TrimSpace(path)))]
	return ok
}

// ArtifactKindForPath returns the stable legacy kind retained alongside role.
func ArtifactKindForPath(path string) string {
	switch strings.ToLower(filepath.Ext(strings.TrimSpace(path))) {
	case ".ppt", ".pptx":
		return "presentation"
	case ".doc", ".docx":
		return "document"
	case ".xls", ".xlsx", ".csv", ".tsv":
		return "spreadsheet"
	case ".pdf":
		return "pdf"
	case ".md":
		return "markdown"
	default:
		return "file"
	}
}
