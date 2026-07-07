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

	deckOut := executeArtifactTool(t, ctx, presentationCreateOrEdit, map[string]any{
		"output_path": "deck.pptx",
		"slides": []map[string]string{
			{"title": "Intro", "body": "Hello"},
		},
	})
	assertZipContains(t, deckOut, "ppt/slides/slide1.xml")
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
