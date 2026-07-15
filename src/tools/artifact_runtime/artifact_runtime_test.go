package artifactruntime

import (
	"archive/zip"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
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

func TestDocumentCreateRejectsMisleadingPDFExtension(t *testing.T) {
	root := t.TempDir()
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))
	_, err := documentCreateOrEdit(ctx, mustJSON(t, map[string]any{
		"output_path": "report.pdf",
		"paragraphs":  []string{"This must not become a DOCX disguised as PDF"},
	}))
	if err == nil || !strings.Contains(err.Error(), "只支持 .docx") {
		t.Fatalf("expected explicit extension rejection, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, "report.pdf")); !os.IsNotExist(statErr) {
		t.Fatalf("misleading PDF must not be created, stat error=%v", statErr)
	}
}

func TestOfficeExportPDFRejectsNonOfficeInputBeforeRuntime(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "notes.md"), []byte("notes"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))
	_, err := officeExportPDF(ctx, mustJSON(t, map[string]any{"path": "notes.md", "output_path": "notes.pdf"}))
	if err == nil || !strings.Contains(err.Error(), "只支持 Word、Excel 或 PowerPoint") {
		t.Fatalf("expected non-Office input rejection, got %v", err)
	}
}

func TestPresentationCreateUsesRuntimeAndRenderCheck(t *testing.T) {
	installHermeticPresentationRuntime(t)
	root := t.TempDir()
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))
	imagePath := filepath.Join(root, "chart.png")
	writeTestPNG(t, imagePath, 80, 50)

	result := executeArtifactToolRaw(t, ctx, presentationCreate, map[string]any{
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
				"layout":   "image_right",
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
		Path              string   `json:"path"`
		PDFPath           string   `json:"pdf_path"`
		PreviewDir        string   `json:"preview_dir"`
		PreviewImages     []string `json:"preview_images"`
		Verified          bool     `json:"verified"`
		VerificationLevel string   `json:"verification_level"`
		SlideCount        int      `json:"slide_count"`
		PNGCount          int      `json:"png_count"`
	}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("parse presentation result: %v\n%s", err, result)
	}
	deckOut := parsed.Path
	assertZipContains(t, deckOut, "ppt/slides/slide1.xml")
	assertZipContains(t, deckOut, "ppt/slideMasters/slideMaster1.xml")
	assertZipHasPrefix(t, deckOut, "ppt/media/")
	if !parsed.Verified || parsed.VerificationLevel != "png" {
		t.Fatalf("expected verified png-level result, got %s", result)
	}
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
	if parsed.SlideCount != 1 || parsed.PNGCount != 1 || len(parsed.PreviewImages) != 1 {
		t.Fatalf("expected consistent slide/png counts, got %s", result)
	}

	renderResult := executeArtifactToolRaw(t, ctx, artifactRender, map[string]any{
		"path":       "演示报告.pptx",
		"output_dir": "manual_preview",
	})
	var rendered struct {
		Path          string   `json:"path"`
		PDFPath       string   `json:"pdf_path"`
		PreviewImages []string `json:"preview_images"`
		Verified      bool     `json:"verified"`
	}
	if err := json.Unmarshal([]byte(renderResult), &rendered); err != nil {
		t.Fatalf("parse artifact_render result: %v\n%s", err, renderResult)
	}
	if rendered.Path != deckOut || rendered.PDFPath == "" || len(rendered.PreviewImages) == 0 || !rendered.Verified {
		t.Fatalf("expected artifact_render to return pptx/pdf/png previews, got %s", renderResult)
	}
}

func TestPresentationCreateAllowsImageOutsideReadableRoots(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	imagePath := filepath.Join(outside, "outside.png")
	writeTestPNG(t, imagePath, 10, 10)

	create := typedArtifactTool{
		name:       "presentation_create",
		readOnly:   false,
		capability: tool.CapabilityArtifactWrite,
		execute:    presentationCreate,
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
	if decision.Behavior != tool.PermissionAllow {
		t.Fatalf("expected outside image input to be allowed, got %+v", decision)
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

func TestPresentationRenderChecksOutputBoundaries(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	render := typedArtifactTool{
		name:       "presentation_render",
		readOnly:   false,
		capability: tool.CapabilityArtifactWrite,
		execute:    presentationRender,
	}
	decision := render.CheckPermission(
		context.Background(),
		mustJSON(t, map[string]any{"path": "missing.pptx", "output_dir": filepath.Join(outside, "preview")}),
		tool.NewPermissionContext(root, []string{root}),
	)
	if decision.Behavior != tool.PermissionDeny || !strings.Contains(decision.Reason, "outside writable/output roots") {
		t.Fatalf("expected output boundary deny, got %+v", decision)
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
	for _, artifact := range artifacts {
		if artifact.ContractVersion != tool.ArtifactContractVersion || artifact.Producer != "presentation_create" {
			t.Fatalf("artifact lacks a complete v2 producer contract: %+v", artifact)
		}
		if artifact.Action != tool.ArtifactActionCreated {
			t.Fatalf("presentation_create artifact must be created: %+v", artifact)
		}
	}
	if artifacts[0].Role != tool.ArtifactRoleDeliverable || artifacts[1].Role != tool.ArtifactRolePreview || artifacts[2].Role != tool.ArtifactRolePreview {
		t.Fatalf("unexpected artifact roles: %+v", artifacts)
	}
}

func TestOfficeExportPDFPublishesFinalDeliverable(t *testing.T) {
	root := t.TempDir()
	permissionContext := tool.NewPermissionContext(root, []string{root})
	output := mustJSON(t, map[string]any{
		"status":   "pdf_exported",
		"path":     filepath.Join(root, "report.pdf"),
		"pdf_path": filepath.Join(root, "report.pdf"),
	})
	export := typedArtifactTool{name: "office_export_pdf", readOnly: false}
	artifacts := export.Artifacts(`{}`, output, permissionContext)
	if len(artifacts) != 1 {
		t.Fatalf("expected one exported PDF artifact, got %+v", artifacts)
	}
	if artifacts[0].Role != tool.ArtifactRoleDeliverable || artifacts[0].Kind != "pdf" {
		t.Fatalf("Office-exported PDF must be a final deliverable: %+v", artifacts[0])
	}
}

func TestTypedArtifactCreateAndEditDeclareAccurateMutations(t *testing.T) {
	create := typedArtifactTool{name: "document_create", description: "create", readOnly: false, capability: tool.CapabilityArtifactWrite}
	edit := typedArtifactTool{name: "document_edit", description: "edit", readOnly: false, capability: tool.CapabilityArtifactWrite}

	createDefinition := create.Definition()
	editDefinition := edit.Definition()
	if len(createDefinition.SideEffects) != 1 || createDefinition.SideEffects[0].Kind != tool.SideEffectCreate {
		t.Fatalf("document_create side effect=%+v, want create", createDefinition.SideEffects)
	}
	if len(editDefinition.SideEffects) != 1 || editDefinition.SideEffects[0].Kind != tool.SideEffectUpdate {
		t.Fatalf("document_edit side effect=%+v, want update", editDefinition.SideEffects)
	}

	root := t.TempDir()
	permissionContext := tool.NewPermissionContext(root, []string{root})
	output := mustJSON(t, map[string]any{"path": filepath.Join(root, "report.docx")})
	created := create.Artifacts(`{}`, output, permissionContext)
	updated := edit.Artifacts(`{}`, output, permissionContext)
	if len(created) != 1 || created[0].Action != tool.ArtifactActionCreated {
		t.Fatalf("document_create descriptor=%+v, want created", created)
	}
	if len(updated) != 1 || updated[0].Action != tool.ArtifactActionUpdated {
		t.Fatalf("document_edit descriptor=%+v, want updated", updated)
	}
	for _, artifact := range append(created, updated...) {
		if artifact.ContractVersion != tool.ArtifactContractVersion || artifact.Role != tool.ArtifactRoleDeliverable || artifact.Producer == "" {
			t.Fatalf("descriptor lacks v2 contract facts: %+v", artifact)
		}
	}
}

func TestTypedArtifactOutcomeProjectsStructuredExecutorDetails(t *testing.T) {
	runtimeTool := typedArtifactTool{
		name: "contract_test_create", description: "create test artifact", readOnly: false,
		capability: tool.CapabilityArtifactWrite,
		execute: func(context.Context, string) (string, error) {
			return `{"status":"ready","path":"report.docx","custom_count":3}`, nil
		},
	}
	outcome, err := runtimeTool.ExecuteOutcome(context.Background(), `{}`, nil)
	if err != nil || outcome.IsError() {
		t.Fatalf("structured artifact outcome failed: outcome=%+v err=%v", outcome, err)
	}
	result, ok := outcome.Details["result"].(map[string]any)
	if !ok || result["custom_count"] != float64(3) || outcome.Details["path"] != "report.docx" {
		t.Fatalf("executor result was not projected into typed details: %+v", outcome.Details)
	}
	definition := runtimeTool.Definition()
	normalized := outcome.Normalize(tool.OutcomeNormalizationContext{
		OperationType: runtimeTool.name, DeclaredSideEffects: definition.SideEffects,
		OutputSchema: definition.OutputSchema, StrictOutputSchema: true,
	})
	if normalized.IsError() || normalized.Status != tool.OutcomeSucceeded {
		t.Fatalf("typed artifact details failed their registered output schema: %+v", normalized)
	}
}

func TestArtifactRenderDoesNotClaimExistingPDFWasCreated(t *testing.T) {
	root := t.TempDir()
	permissionContext := tool.NewPermissionContext(root, []string{root})
	source := filepath.Join(root, "existing.pdf")
	preview := filepath.Join(root, "preview", "page-1.png")
	output := mustJSON(t, map[string]any{
		"path": source, "pdf_path": source, "preview_images": []string{preview},
	})
	render := typedArtifactTool{name: "artifact_render", readOnly: false}
	artifacts := render.Artifacts(`{}`, output, permissionContext)
	if len(artifacts) != 1 {
		t.Fatalf("render must emit only generated preview artifacts, got %+v", artifacts)
	}
	if artifacts[0].Path != "preview/page-1.png" || artifacts[0].Action != tool.ArtifactActionCreated || artifacts[0].Role != tool.ArtifactRolePreview {
		t.Fatalf("unexpected render preview descriptor: %+v", artifacts[0])
	}
}

func TestUnimplementedArtifactStubsAreNotRegistered(t *testing.T) {
	for _, name := range []string{"spreadsheet_chart", "document_render", "pdf_render"} {
		if registered, err := tool.GetTool(name); err == nil {
			t.Fatalf("unimplemented tool %s must not be advertised: %T", name, registered)
		}
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
	if path, err := resolvePresentationPython(); err == nil {
		setPresentationPythonTestOverride(t, path)
		return
	}
	// Hermetic tests may bootstrap from PATH through an in-process override;
	// production execution never reads prepare-only source variables.
	for _, name := range []string{"python3", "python"} {
		if path, err := exec.LookPath(name); err == nil && pythonCanImportPPTX(path) {
			setPresentationPythonTestOverride(t, path)
			return
		}
	}
	t.Skipf("presentation runtime unavailable: python with python-pptx not found")
}

func setPresentationPythonTestOverride(t *testing.T, path string) {
	t.Helper()
	presentationRuntimeTestOverrides.Lock()
	previous := presentationRuntimeTestOverrides.python
	presentationRuntimeTestOverrides.python = path
	presentationRuntimeTestOverrides.Unlock()
	t.Cleanup(func() {
		presentationRuntimeTestOverrides.Lock()
		presentationRuntimeTestOverrides.python = previous
		presentationRuntimeTestOverrides.Unlock()
	})
}

func setPresentationRenderingTestOverrides(t *testing.T, soffice, popplerBin string) {
	t.Helper()
	presentationRuntimeTestOverrides.Lock()
	previousSoffice := presentationRuntimeTestOverrides.soffice
	previousPoppler := presentationRuntimeTestOverrides.popplerBin
	presentationRuntimeTestOverrides.soffice = soffice
	presentationRuntimeTestOverrides.popplerBin = popplerBin
	presentationRuntimeTestOverrides.Unlock()
	t.Cleanup(func() {
		presentationRuntimeTestOverrides.Lock()
		presentationRuntimeTestOverrides.soffice = previousSoffice
		presentationRuntimeTestOverrides.popplerBin = previousPoppler
		presentationRuntimeTestOverrides.Unlock()
	})
}

func writeExecutable(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write executable %s: %v", path, err)
	}
}

func installHermeticPresentationRuntime(t *testing.T) {
	t.Helper()
	requirePresentationRuntime(t)
	bin := t.TempDir()
	soffice := filepath.Join(bin, "soffice")
	pdftoppm := filepath.Join(bin, "pdftoppm")
	writeExecutable(t, soffice, `#!/bin/bash
set -euo pipefail
outdir=""
input=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --outdir)
      outdir="$2"
      shift 2
      ;;
    --convert-to)
      shift 2
      ;;
    --headless|--nologo|--nofirststartwizard|--norestore)
      shift
      ;;
    -env:UserInstallation=*)
      shift
      ;;
    *)
      input="$1"
      shift
      ;;
  esac
done
python3 - "$input" "$outdir" <<'PY'
import os, sys, zipfile
input_path, outdir = sys.argv[1], sys.argv[2]
base = os.path.splitext(os.path.basename(input_path))[0]
count = 0
with zipfile.ZipFile(input_path) as zf:
    for name in zf.namelist():
        if name.startswith("ppt/slides/slide") and name.endswith(".xml") and "_rels" not in name:
            count += 1
if count <= 0:
    count = 1
pdf_path = os.path.join(outdir, base + ".pdf")
with open(pdf_path, "w", encoding="utf-8") as f:
    f.write("%PDF-1.4\n")
    for i in range(count):
        f.write(f"% page {i+1}\n")
    f.write("%%EOF\n")
with open(pdf_path + ".pagecount", "w", encoding="utf-8") as f:
    f.write(str(count))
PY
`)
	writeExecutable(t, pdftoppm, `#!/bin/bash
set -euo pipefail
pdf=""
prefix=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    -png|-r)
      if [[ "$1" == "-r" ]]; then
        shift 2
      else
        shift
      fi
      ;;
    *)
      if [[ -z "$pdf" ]]; then
        pdf="$1"
      else
        prefix="$1"
      fi
      shift
      ;;
  esac
done
python3 - "$pdf" "$prefix" <<'PY'
import os, struct, sys, zlib
pdf, prefix = sys.argv[1], sys.argv[2]
count_path = pdf + ".pagecount"
count = 1
if os.path.exists(count_path):
    count = max(1, int(open(count_path, encoding="utf-8").read().strip() or "1"))
else:
    with open(pdf, encoding="utf-8", errors="ignore") as source:
        count = max(1, sum(1 for line in source if line.startswith("% page ")))
def chunk(tag, data):
    return struct.pack(">I", len(data)) + tag + data + struct.pack(">I", zlib.crc32(tag + data) & 0xffffffff)
ihdr = struct.pack(">IIBBBBB", 1, 1, 8, 2, 0, 0, 0)
png = b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", ihdr) + chunk(b"IDAT", zlib.compress(b"\x00\x00\x00\x00")) + chunk(b"IEND", b"")
for i in range(1, count + 1):
    open(f"{prefix}-{i}.png", "wb").write(png)
PY
`)
	setPresentationRenderingTestOverrides(t, soffice, bin)
	t.Setenv("HOME", t.TempDir())
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

func TestRuntimeBackedToolsReportMissingHostDependenciesAtExecution(t *testing.T) {
	t.Setenv("PATH", "")
	t.Setenv("AWAKEN_PYTHON", "")
	t.Setenv("AWAKEN_SOFFICE", "")
	t.Setenv("AWAKEN_POPPLER_BIN", "")
	invalidateRuntimeProbeCache()
	render := runtimeBackedTool("document_render", "Document Render", "render docx", true)
	input := `{"path":"missing.docx"}`
	decision := render.CheckPermission(context.Background(), input, tool.NewPermissionContext(t.TempDir(), nil))
	if decision.Behavior != tool.PermissionAllow {
		t.Fatalf("host dependency discovery must not become a permission gate, got %+v", decision)
	}
	outcome, err := render.ExecuteOutcome(context.Background(), input, nil)
	if err != nil || outcome.Status != tool.OutcomeNeedsRuntime {
		t.Fatalf("expected structured missing dependency outcome, outcome=%+v err=%v", outcome, err)
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
