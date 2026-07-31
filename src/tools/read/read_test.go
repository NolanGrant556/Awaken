package read

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"awaken/src/tool"
)

func readTestContext(root string, capabilities tool.ModelCapabilities) context.Context {
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))
	ctx = tool.ContextWithToolSessionState(ctx, tool.NewToolSessionState(root))
	return tool.ContextWithModelCapabilities(ctx, capabilities)
}

func TestReadTextRangeAndUnchanged(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sample.txt")
	if err := os.WriteFile(path, []byte("one\ntwo\nthree\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := readTestContext(root, tool.ModelCapabilities{})
	input := `{"file_path":"sample.txt","offset":2,"limit":1}`
	first, err := (&Tool{}).ExecuteOutcome(ctx, input, nil)
	if err != nil || first.IsError() || first.Content != "2\ttwo" || !first.Truncated {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	second, err := (&Tool{}).ExecuteOutcome(ctx, input, nil)
	if err != nil || second.IsError() || second.Content != "file_unchanged" {
		t.Fatalf("second=%+v err=%v", second, err)
	}
}

func TestReadImageCapabilityMatrix(t *testing.T) {
	root := t.TempDir()
	png := append([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, []byte("fixture")...)
	if err := os.WriteFile(filepath.Join(root, "image.png"), png, 0o644); err != nil {
		t.Fatal(err)
	}
	unsupported, _ := (&Tool{}).ExecuteOutcome(readTestContext(root, tool.ModelCapabilities{}), `{"file_path":"image.png"}`, nil)
	if unsupported.ErrorCode != "model_vision_unsupported" || unsupported.Status != tool.OutcomeUnsupported {
		t.Fatalf("unsupported=%+v", unsupported)
	}
	visible, _ := (&Tool{}).ExecuteOutcome(readTestContext(root, tool.ModelCapabilities{SupportsVision: true}), `{"file_path":"image.png"}`, nil)
	if visible.IsError() || len(visible.ModelContent) != 1 || visible.ModelContent[0].MimeType != "image/png" || visible.ModelContent[0].DataBase64 == "" {
		t.Fatalf("visible=%+v", visible)
	}
}

func TestReadNativePDFAndInvalidPages(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "sample.pdf"), []byte("%PDF-1.4\n%%EOF\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := readTestContext(root, tool.ModelCapabilities{SupportsNativePDF: true})
	outcome, _ := (&Tool{}).ExecuteOutcome(ctx, `{"file_path":"sample.pdf","pages":"1-2"}`, nil)
	if outcome.IsError() || len(outcome.ModelContent) != 1 || outcome.ModelContent[0].Type != "file" || outcome.ModelContent[0].MimeType != "application/pdf" {
		t.Fatalf("native=%+v", outcome)
	}
	invalid, _ := (&Tool{}).ExecuteOutcome(readTestContext(root, tool.ModelCapabilities{SupportsNativePDF: true}), `{"file_path":"sample.pdf","pages":"0"}`, nil)
	if invalid.ErrorCode != "read_invalid_pages" || !strings.Contains(invalid.ErrorMessage, "page") {
		t.Fatalf("invalid=%+v", invalid)
	}
}

func TestReadPDFFallsBackToPopplerImages(t *testing.T) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm unavailable")
	}
	root := t.TempDir()
	writeMinimalPDF(t, filepath.Join(root, "fallback.pdf"), 1)
	outcome, err := (&Tool{}).ExecuteOutcome(readTestContext(root, tool.ModelCapabilities{SupportsVision: true}), `{"file_path":"fallback.pdf","pages":"1"}`, nil)
	if err != nil || outcome.IsError() || len(outcome.ModelContent) != 1 || outcome.ModelContent[0].Type != "image" || outcome.ModelContent[0].Page != 1 {
		t.Fatalf("fallback=%+v err=%v", outcome, err)
	}
}

func TestReadPDFOmitsDefaultPagesAndClampsOverRange(t *testing.T) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm unavailable")
	}
	if _, err := exec.LookPath("pdfinfo"); err != nil {
		t.Skip("pdfinfo unavailable")
	}
	root := t.TempDir()
	writeMinimalPDF(t, filepath.Join(root, "one-page.pdf"), 1)
	ctx := readTestContext(root, tool.ModelCapabilities{SupportsVision: true})

	omitted, err := (&Tool{}).ExecuteOutcome(ctx, `{"file_path":"one-page.pdf"}`, nil)
	if err != nil || omitted.IsError() || len(omitted.ModelContent) != 1 || omitted.ModelContent[0].Page != 1 {
		t.Fatalf("omit pages=%+v err=%v", omitted, err)
	}
	if route, _ := omitted.Details["route"].(string); route != "poppler" {
		t.Fatalf("expected poppler route, details=%v", omitted.Details)
	}
	if count, _ := omitted.Details["page_count"].(int); count != 1 {
		t.Fatalf("expected page_count=1, details=%v", omitted.Details)
	}

	// Fresh state so the second read is not treated as unchanged.
	ctx = readTestContext(root, tool.ModelCapabilities{SupportsVision: true})
	clamped, err := (&Tool{}).ExecuteOutcome(ctx, `{"file_path":"one-page.pdf","pages":"1-5"}`, nil)
	if err != nil || clamped.IsError() || len(clamped.ModelContent) != 1 || clamped.ModelContent[0].Page != 1 {
		t.Fatalf("clamp pages=%+v err=%v", clamped, err)
	}
}

func TestReadPDFRequiresPagesForLongDocuments(t *testing.T) {
	if _, err := exec.LookPath("pdfinfo"); err != nil {
		t.Skip("pdfinfo unavailable")
	}
	root := t.TempDir()
	writeMinimalPDF(t, filepath.Join(root, "long.pdf"), 11)
	outcome, err := (&Tool{}).ExecuteOutcome(readTestContext(root, tool.ModelCapabilities{SupportsNativePDF: true}), `{"file_path":"long.pdf"}`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.ErrorCode != "read_pdf_pages_required" || !strings.Contains(outcome.ErrorMessage, "11 pages") {
		t.Fatalf("long pdf=%+v", outcome)
	}
}

func writeMinimalPDF(t *testing.T, path string, pageCount int) {
	t.Helper()
	if pageCount < 1 {
		t.Fatalf("pageCount must be >= 1")
	}
	var kids []string
	var body strings.Builder
	body.WriteString("%PDF-1.1\n")
	body.WriteString("1 0 obj << /Type /Catalog /Pages 2 0 R >> endobj\n")
	for i := 0; i < pageCount; i++ {
		pageObj := 3 + i*2
		contentObj := pageObj + 1
		kids = append(kids, fmt.Sprintf("%d 0 R", pageObj))
		fmt.Fprintf(&body, "%d 0 obj << /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Resources << >> /Contents %d 0 R >> endobj\n", pageObj, contentObj)
		fmt.Fprintf(&body, "%d 0 obj << /Length 0 >> stream\n\nendstream endobj\n", contentObj)
	}
	fmt.Fprintf(&body, "2 0 obj << /Type /Pages /Kids [%s] /Count %d >> endobj\n", strings.Join(kids, " "), pageCount)
	body.WriteString("trailer << /Root 1 0 R >>\n%%EOF\n")
	if err := os.WriteFile(path, []byte(body.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestReadReachesOutsideRootsAndRejectsOfficeBinary(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	outsidePath := filepath.Join(outside, "outside.txt")
	if err := os.WriteFile(outsidePath, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Roots say where work happens by default, not what may be opened, so an
	// absolute path the user named is read wherever it lives.
	reached, _ := (&Tool{}).ExecuteOutcome(readTestContext(root, tool.ModelCapabilities{}), `{"file_path":`+quote(outsidePath)+`}`, nil)
	if reached.IsError() || !strings.Contains(reached.Content, "secret") {
		t.Fatalf("outside=%+v", reached)
	}
	if err := os.WriteFile(filepath.Join(root, "book.xlsx"), []byte{'P', 'K', 3, 4, 0, 0}, 0o644); err != nil {
		t.Fatal(err)
	}
	office, _ := (&Tool{}).ExecuteOutcome(readTestContext(root, tool.ModelCapabilities{}), `{"file_path":"book.xlsx"}`, nil)
	if office.ErrorCode != "unsupported_media" || office.Status != tool.OutcomeUnsupported {
		t.Fatalf("office=%+v", office)
	}
}

func quote(value string) string { return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"` }
