package outputpersist

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestExtensionForMIMETypeStripsParameters(t *testing.T) {
	tests := []struct {
		name string
		mime string
		want string
	}{
		{name: "pdf with charset", mime: "Application/PDF; charset=utf-8", want: "pdf"},
		{name: "csv with parameter", mime: "text/csv; header=present", want: "csv"},
		{name: "json with parameter", mime: "application/json; charset=utf-8", want: "json"},
		{name: "unknown", mime: "application/x-custom-binary", want: "bin"},
		{name: "empty", mime: "", want: "bin"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExtensionForMIMEType(tt.mime); got != tt.want {
				t.Fatalf("ExtensionForMIMEType(%q) = %q, want %q", tt.mime, got, tt.want)
			}
		})
	}
}

func TestIsBinaryContentType(t *testing.T) {
	textTypes := []string{
		"",
		"text/plain; charset=utf-8",
		"text/html",
		"application/json",
		"application/vnd.api+json",
		"application/xml",
		"application/atom+xml",
		"application/javascript; charset=utf-8",
		"application/x-www-form-urlencoded",
	}
	for _, contentType := range textTypes {
		t.Run("text_"+contentType, func(t *testing.T) {
			if IsBinaryContentType(contentType) {
				t.Fatalf("expected %q to be text-like", contentType)
			}
		})
	}

	binaryTypes := []string{
		"application/octet-stream",
		"application/pdf",
		"application/zip",
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		"image/png",
		"image/svg+xml",
		"audio/mpeg",
		"video/mp4",
	}
	for _, contentType := range binaryTypes {
		t.Run("binary_"+contentType, func(t *testing.T) {
			if !IsBinaryContentType(contentType) {
				t.Fatalf("expected %q to be binary", contentType)
			}
		})
	}
}

func TestBuildBinaryBlobMessageFormatsSize(t *testing.T) {
	message := BuildBinaryBlobMessage("/tmp/result.bin", "application/octet-stream", 1536, "MCP ")

	if !strings.Contains(message, "MCP Binary content") {
		t.Fatalf("expected source prefix in message, got %q", message)
	}
	if !strings.Contains(message, "1.5KB") {
		t.Fatalf("expected formatted size in message, got %q", message)
	}
}

func TestPlanBinaryPersistCleansPersistID(t *testing.T) {
	baseDir := filepath.Join("tool-results", "session")
	plan := PlanBinaryPersist(baseDir, "../escape/../../call.1:bad id", "application/pdf", 1024)

	if plan.Ext != "pdf" {
		t.Fatalf("Ext = %q, want pdf", plan.Ext)
	}
	if !strings.HasPrefix(plan.Path, baseDir+string(filepath.Separator)) {
		t.Fatalf("path %q should stay under base dir %q", plan.Path, baseDir)
	}

	filename := filepath.Base(plan.Path)
	if strings.Contains(filename, "..") || strings.ContainsAny(filename, `/\:`) {
		t.Fatalf("filename %q was not conservatively cleaned", filename)
	}
	if filename != "escape_call_1_bad_id.pdf" {
		t.Fatalf("filename = %q, want escape_call_1_bad_id.pdf", filename)
	}
	if !strings.Contains(plan.Message, plan.Path) {
		t.Fatalf("message should include planned path, got %q", plan.Message)
	}
}

func TestPlanBinaryPersistUsesFallbackID(t *testing.T) {
	plan := PlanBinaryPersist("tool-results", "../../../", "application/x-unknown", 10)

	if filepath.Base(plan.Path) != "output.bin" {
		t.Fatalf("fallback filename = %q, want output.bin", filepath.Base(plan.Path))
	}
}

func TestLargeOutputInstructionsRequiresChunkedFullRead(t *testing.T) {
	instructions := LargeOutputInstructions("/tmp/output.json", 1234567, "JSON", 32000)

	required := []string{
		"1,234,567 characters",
		"Format: JSON",
		"You MUST read the content from the file at /tmp/output.json in sequential chunks until 100% of the content has been read.",
		"32,000 chars",
		"Before producing ANY summary or analysis",
	}
	for _, want := range required {
		if !strings.Contains(instructions, want) {
			t.Fatalf("instructions missing %q:\n%s", want, instructions)
		}
	}
}
