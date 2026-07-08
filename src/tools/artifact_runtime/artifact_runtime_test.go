package artifactruntime

import (
	"archive/zip"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"awaken/src/tool"
)

func TestArtifactRuntimeCreatesOfficeArtifacts(t *testing.T) {
	root := t.TempDir()
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))

	docOut := executeArtifactTool(t, ctx, documentCreateOrEdit, map[string]any{
		"output_path": "report.docx",
		"title":       "Report",
		"paragraphs":  []string{"One", "Two"},
	})
	assertZipContains(t, docOut, "word/document.xml")

	sheetOut := executeArtifactTool(t, ctx, spreadsheetCreateOrEdit, map[string]any{
		"output_path": "table.xlsx",
		"rows":        [][]string{{"Name", "Value"}, {"A", "1"}},
	})
	assertZipContains(t, sheetOut, "xl/worksheets/sheet1.xml")
}

func TestPresentationCreateUsesRuntimeAndRenderCheck(t *testing.T) {
	requirePresentationRuntime(t)
	root := t.TempDir()
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))
	imagePath := filepath.Join(root, "chart.png")
	writeTestPNG(t, imagePath, 80, 50)

	result := executeArtifactToolRaw(t, ctx, presentationCreateOrEdit, map[string]any{
		"output_path": "演示报告.pptx",
		"theme": map[string]any{
			"primary": "#12324A",
			"accent":  "#0EA5A4",
		},
		"slides": []map[string]any{
			{
				"title":    "Intro",
				"subtitle": "ERP V7 / V8",
				"body":     "Hello",
				"bullets":  []string{"Stable content", "Image rich"},
				"notes":    "speaker note",
				"images": []map[string]any{{
					"path":    "chart.png",
					"caption": "Chart",
					"fit":     "contain",
				}},
			},
		},
	})
	var parsed struct {
		Path          string   `json:"path"`
		PDFPath       string   `json:"pdf_path"`
		PreviewDir    string   `json:"preview_dir"`
		PreviewImages []string `json:"preview_images"`
	}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("parse presentation result: %v\n%s", err, result)
	}
	deckOut := parsed.Path
	assertZipContains(t, deckOut, "ppt/slides/slide1.xml")
	assertZipContains(t, deckOut, "ppt/slideMasters/slideMaster1.xml")
	assertZipHasPrefix(t, deckOut, "ppt/media/")
	if parsed.PDFPath == "" {
		t.Fatalf("expected pdf preview path in result: %s", result)
	}
	if _, err := os.Stat(parsed.PDFPath); err != nil {
		t.Fatalf("expected rendered pdf: %v", err)
	}
	if parsed.PreviewDir == "" {
		t.Fatalf("expected preview dir in result: %s", result)
	}
	if _, err := os.Stat(parsed.PreviewDir); err != nil {
		t.Fatalf("expected preview dir: %v", err)
	}

	renderResult := executeArtifactToolRaw(t, ctx, artifactRender, map[string]any{
		"path":       "演示报告.pptx",
		"output_dir": "manual_preview",
	})
	var rendered struct {
		Path          string   `json:"path"`
		PDFPath       string   `json:"pdf_path"`
		PreviewImages []string `json:"preview_images"`
	}
	if err := json.Unmarshal([]byte(renderResult), &rendered); err != nil {
		t.Fatalf("parse artifact_render result: %v\n%s", err, renderResult)
	}
	if rendered.Path != deckOut || rendered.PDFPath == "" || len(rendered.PreviewImages) == 0 {
		t.Fatalf("expected artifact_render to return pptx/pdf/png previews, got %s", renderResult)
	}
}

func TestPresentationCreateRejectsImageOutsideReadableRoots(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	imagePath := filepath.Join(outside, "outside.png")
	writeTestPNG(t, imagePath, 10, 10)

	create := typedArtifactTool{
		name:       "presentation_create",
		readOnly:   false,
		capability: tool.CapabilityArtifactWrite,
		execute:    presentationCreateOrEdit,
	}
	decision := create.CheckPermission(
		context.Background(),
		mustJSON(t, map[string]any{
			"output_path": "deck.pptx",
			"slides": []map[string]any{{
				"title": "Bad",
				"images": []map[string]any{{
					"path": imagePath,
				}},
			}},
		}),
		tool.NewPermissionContext(root, []string{root}),
	)
	if decision.Behavior != tool.PermissionDeny || !strings.Contains(decision.Reason, "图片素材") {
		t.Fatalf("expected image boundary deny, got %+v", decision)
	}
}

func TestPresentationSpecResolvesImagesAndPreviewDefaults(t *testing.T) {
	root := t.TempDir()
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))
	imagePath := filepath.Join(root, "chart.png")
	writeTestPNG(t, imagePath, 12, 8)

	spec, err := presentationSpecFromParams(ctx, map[string]any{
		"title": "Deck",
		"theme": map[string]any{
			"background": "#FFFFFF",
			"accent":     "#00A3A3",
		},
		"slides": []any{
			map[string]any{
				"title":   "Page",
				"bullets": []any{"One", "Two"},
				"images": []any{
					map[string]any{"path": "chart.png", "fit": "cover", "width": 4.2},
				},
			},
		},
	}, filepath.Join(root, "deck.pptx"))
	if err != nil {
		t.Fatalf("presentation spec failed: %v", err)
	}
	if spec.Theme.Accent != "#00A3A3" || len(spec.Slides) != 1 || len(spec.Slides[0].Bullets) != 2 {
		t.Fatalf("unexpected spec: %+v", spec)
	}
	if got := spec.Slides[0].Images[0].Path; got != imagePath {
		t.Fatalf("expected resolved image path %s, got %s", imagePath, got)
	}
	if !strings.HasSuffix(spec.PreviewDir, "deck_preview") {
		t.Fatalf("expected default preview dir, got %s", spec.PreviewDir)
	}
}

func TestPresentationRenderChecksInputAndOutputBoundaries(t *testing.T) {
	root := t.TempDir()
	render := typedArtifactTool{
		name:       "presentation_render",
		readOnly:   false,
		capability: tool.CapabilityArtifactWrite,
		execute:    presentationRender,
	}
	decision := render.CheckPermission(
		context.Background(),
		`{"path":"missing.pptx","output_dir":"preview"}`,
		tool.NewPermissionContext(root, []string{root}),
	)
	if decision.Behavior != tool.PermissionDeny || !strings.Contains(decision.Reason, "输入文件") {
		t.Fatalf("expected missing readable input deny, got %+v", decision)
	}
}

func TestTypedArtifactToolEmitsPresentationPreviewArtifacts(t *testing.T) {
	root := t.TempDir()
	permissionContext := tool.NewPermissionContext(root, []string{root})
	output := mustJSON(t, map[string]any{
		"status":         "presentation_written",
		"path":           filepath.Join(root, "deck.pptx"),
		"pdf_path":       filepath.Join(root, "deck_preview", "deck.pdf"),
		"preview_images": []string{filepath.Join(root, "deck_preview", "page-1.png")},
	})
	create := typedArtifactTool{name: "presentation_create", readOnly: false}
	artifacts := create.Artifacts(`{}`, output, permissionContext)
	if len(artifacts) != 3 {
		t.Fatalf("expected deck, pdf and preview image artifacts, got %+v", artifacts)
	}
	if artifacts[0].Kind != "presentation" || artifacts[1].Kind != "pdf" || artifacts[2].Kind != "image" {
		t.Fatalf("unexpected artifact kinds: %+v", artifacts)
	}
}

func TestPDFExtractAcceptsRawSinglePathInput(t *testing.T) {
	root := t.TempDir()
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))
	pdfPath := filepath.Join(root, "sample.pdf")
	if err := os.WriteFile(pdfPath, []byte("%PDF-1.4\n1 0 obj\n<< /Type /Page >>\n(Hello PDF)\nendobj\n%%EOF\n"), 0o644); err != nil {
		t.Fatalf("write sample pdf: %v", err)
	}

	readTool := typedArtifactTool{
		name:       "pdf_extract",
		readOnly:   true,
		capability: tool.CapabilityArtifactRead,
		parameters: []tool.ToolParameter{{Name: "path", Required: true}},
		execute:    pdfExtract,
	}
	decision := readTool.CheckPermission(context.Background(), "sample.pdf", tool.NewPermissionContext(root, []string{root}))
	if decision.Behavior != tool.PermissionAllow {
		t.Fatalf("expected raw path permission allow, got %+v", decision)
	}

	result, err := pdfExtract(ctx, "sample.pdf")
	if err != nil {
		t.Fatalf("pdf_extract raw path failed: %v", err)
	}
	if !strings.Contains(result, "Hello PDF") {
		t.Fatalf("expected extracted text, got %s", result)
	}
}

func TestPDFExtractRejectsNonPDFWithRoutingHint(t *testing.T) {
	root := t.TempDir()
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("not a pdf"), 0o644); err != nil {
		t.Fatalf("write sample text: %v", err)
	}

	_, err := pdfExtract(ctx, "notes.txt")
	if err == nil || !strings.Contains(err.Error(), "pdf_extract") || !strings.Contains(err.Error(), "read_file") || !strings.Contains(err.Error(), "read_artifact") {
		t.Fatalf("expected pdf routing hint, got %v", err)
	}
}

func TestPDFExtractRejectsEmptyPDFWithClearDiagnostic(t *testing.T) {
	root := t.TempDir()
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))
	if err := os.WriteFile(filepath.Join(root, "empty.pdf"), nil, 0o644); err != nil {
		t.Fatalf("write empty pdf: %v", err)
	}

	_, err := pdfExtract(ctx, "empty.pdf")
	if err == nil || !strings.Contains(err.Error(), "空 PDF") || !strings.Contains(err.Error(), "0 字节") {
		t.Fatalf("expected empty pdf diagnostic, got %v", err)
	}
}

func requirePresentationRuntime(t *testing.T) {
	t.Helper()
	if _, err := resolvePresentationPython(); err != nil {
		t.Skipf("presentation runtime unavailable: %v", err)
	}
	if _, err := resolveSoffice(); err != nil {
		t.Skipf("presentation render runtime unavailable: %v", err)
	}
}

func TestArtifactRuntimeImageInspectAndConvert(t *testing.T) {
	root := t.TempDir()
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))

	source := filepath.Join(root, "source.png")
	img := image.NewRGBA(image.Rect(0, 0, 2, 3))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	out, err := os.Create(source)
	if err != nil {
		t.Fatalf("create source image: %v", err)
	}
	if err := png.Encode(out, img); err != nil {
		_ = out.Close()
		t.Fatalf("encode source image: %v", err)
	}
	if err := out.Close(); err != nil {
		t.Fatalf("close source image: %v", err)
	}

	inspect := executeArtifactToolRaw(t, ctx, imageInspect, map[string]any{"path": "source.png"})
	if !strings.Contains(inspect, `"width": 2`) || !strings.Contains(inspect, `"height": 3`) {
		t.Fatalf("unexpected inspect payload: %s", inspect)
	}

	converted := executeArtifactTool(t, ctx, imageConvert, map[string]any{
		"path":        "source.png",
		"output_path": "converted.jpg",
	})
	if !strings.Contains(converted, "converted.jpg") {
		t.Fatalf("unexpected convert payload: %s", converted)
	}
	if _, err := os.Stat(filepath.Join(root, "converted.jpg")); err != nil {
		t.Fatalf("converted image missing: %v", err)
	}
}

func TestImageInspectRejectsPDFWithRoutingHint(t *testing.T) {
	root := t.TempDir()
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))
	if err := os.WriteFile(filepath.Join(root, "sample.pdf"), []byte("%PDF-1.4\n%%EOF\n"), 0o644); err != nil {
		t.Fatalf("write sample pdf: %v", err)
	}

	_, err := imageInspect(ctx, "sample.pdf")
	if err == nil || !strings.Contains(err.Error(), "image_inspect") || !strings.Contains(err.Error(), "pdf_extract") || !strings.Contains(err.Error(), "read_artifact") {
		t.Fatalf("expected image routing hint, got %v", err)
	}
}

func TestRuntimeBackedToolsRequirePreparedRuntime(t *testing.T) {
	t.Setenv("AWAKEN_RUNTIME_MANIFEST", filepath.Join(t.TempDir(), "missing-runtime.json"))
	render := runtimeBackedTool("document_render", "Document Render", "render docx", true)
	decision := render.CheckPermission(context.Background(), `{}`, tool.NewPermissionContext(t.TempDir(), nil))
	if decision.Behavior != tool.PermissionNeedsRuntime {
		t.Fatalf("expected needs_runtime, got %+v", decision)
	}
}

func TestCreateToolsTreatPathAsWritableOutput(t *testing.T) {
	root := t.TempDir()
	create := typedArtifactTool{
		name:       "document_create",
		readOnly:   false,
		capability: tool.CapabilityArtifactWrite,
		execute:    documentCreateOrEdit,
	}
	decision := create.CheckPermission(
		context.Background(),
		`{"path":"new-report.docx","paragraphs":["hello"]}`,
		tool.NewPermissionContext(root, []string{root}),
	)
	if decision.Behavior != tool.PermissionAllow {
		t.Fatalf("expected path-as-output create to be allowed, got %+v", decision)
	}
}

func TestCreateToolsAllowOutputRoots(t *testing.T) {
	workspace := t.TempDir()
	writable := filepath.Join(workspace, "work")
	output := filepath.Join(workspace, "out")
	if err := os.MkdirAll(writable, 0o755); err != nil {
		t.Fatalf("create writable root: %v", err)
	}
	if err := os.MkdirAll(output, 0o755); err != nil {
		t.Fatalf("create output root: %v", err)
	}
	permissionContext := tool.NewTaskScopePermissionContext(
		"scope",
		workspace,
		[]string{workspace},
		[]string{writable},
		[]string{output},
		nil,
		nil,
		"",
	)
	create := typedArtifactTool{
		name:       "document_create",
		readOnly:   false,
		capability: tool.CapabilityArtifactWrite,
		execute:    documentCreateOrEdit,
	}
	decision := create.CheckPermission(
		context.Background(),
		`{"output_path":"out/report.docx","paragraphs":["hello"]}`,
		permissionContext,
	)
	if decision.Behavior != tool.PermissionAllow {
		t.Fatalf("expected output root create to be allowed, got %+v", decision)
	}
}

func executeArtifactTool(
	t *testing.T,
	ctx context.Context,
	fn func(context.Context, string) (string, error),
	payload map[string]any,
) string {
	t.Helper()
	result := executeArtifactToolRaw(t, ctx, fn, payload)
	var parsed struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(result), &parsed); err == nil && parsed.Path != "" {
		return parsed.Path
	}
	return result
}

func executeArtifactToolRaw(
	t *testing.T,
	ctx context.Context,
	fn func(context.Context, string) (string, error),
	payload map[string]any,
) string {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	result, err := fn(ctx, string(data))
	if err != nil {
		t.Fatalf("execute artifact tool: %v", err)
	}
	return result
}

func mustJSON(t *testing.T, payload map[string]any) string {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return string(data)
}

func writeTestPNG(t *testing.T, path string, width int, height int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 255), G: uint8(y % 255), B: 180, A: 255})
		}
	}
	out, err := os.Create(path)
	if err != nil {
		t.Fatalf("create png %s: %v", path, err)
	}
	if err := png.Encode(out, img); err != nil {
		_ = out.Close()
		t.Fatalf("encode png %s: %v", path, err)
	}
	if err := out.Close(); err != nil {
		t.Fatalf("close png %s: %v", path, err)
	}
}

func assertZipContains(t *testing.T, path string, entryName string) {
	t.Helper()
	reader, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("open zip %s: %v", path, err)
	}
	defer reader.Close()
	for _, file := range reader.File {
		if file.Name == entryName {
			return
		}
	}
	t.Fatalf("zip %s missing entry %s", path, entryName)
}

func assertZipHasPrefix(t *testing.T, path string, prefix string) {
	t.Helper()
	reader, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("open zip %s: %v", path, err)
	}
	defer reader.Close()
	for _, file := range reader.File {
		if strings.HasPrefix(file.Name, prefix) {
			return
		}
	}
	t.Fatalf("zip %s missing entry prefix %s", path, prefix)
}
