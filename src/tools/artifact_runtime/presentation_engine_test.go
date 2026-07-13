package artifactruntime

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"

	"awaken/src/tool"
)

func TestPresentationEngineEmbedsCJKEastAsianFonts(t *testing.T) {
	requirePresentationRuntime(t)
	root := t.TempDir()
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))
	output := filepath.Join(root, "中文演示.pptx")
	spec := presentationSpec{
		OutputPath: output,
		Title:      "中文演示",
		Theme: presentationTheme{
			FontEA:    "PingFang SC",
			FontLatin: "Calibri",
		},
		Slides: []slideData{{
			Layout:  "bullets",
			Title:   "标题页",
			Body:    "正文内容",
			Bullets: []string{"要点一", "要点二"},
			Notes:   "演讲者备注",
		}},
		RenderPreview: false,
	}
	if _, err := writePPTXWithPython(ctx, spec); err != nil {
		t.Fatalf("write pptx: %v", err)
	}
	assertPPTXHasNonEmptyEastAsianFonts(t, output)
}

func TestPresentationChineseTextUsesSingleCJKHelperAndSurvivesRenderPipeline(t *testing.T) {
	engineSource, err := os.ReadFile(filepath.Join("engines", "presentation_engine.py"))
	if err != nil {
		t.Fatalf("read presentation engine: %v", err)
	}
	source := string(engineSource)
	if got := strings.Count(source, "def set_run_font("); got != 1 {
		t.Fatalf("presentation engine must own exactly one CJK font writer, got %d", got)
	}
	if got := strings.Count(source, "etree.SubElement(r_pr, qn(tag))"); got != 1 {
		t.Fatalf("CJK OOXML nodes must be appended through the single font helper, got %d append sites", got)
	}
	if got := strings.Count(source, "run._r.get_or_add_rPr()"); got != 1 {
		t.Fatalf("run property XML must have one mutation owner, got %d", got)
	}

	installHermeticPresentationRuntime(t)
	root := t.TempDir()
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))
	result := executeArtifactToolRaw(t, ctx, presentationCreate, map[string]any{
		"output_path": "中文字体验收.pptx",
		"theme": map[string]any{
			"font_ea":    "PingFang SC",
			"font_latin": "Calibri",
		},
		"slides": []map[string]any{{
			"layout":  "bullets",
			"title":   "中文标题不会乱码",
			"body":    "正文应保持完整可读",
			"bullets": []string{"第一项：稳定生成", "第二项：字体可追踪"},
			"notes":   "中文演讲者备注",
		}},
	})
	var parsed struct {
		Path          string   `json:"path"`
		Verified      bool     `json:"verified"`
		PNGCount      int      `json:"png_count"`
		PreviewImages []string `json:"preview_images"`
		Warnings      []string `json:"warnings"`
	}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("parse Chinese presentation result: %v\n%s", err, result)
	}
	if !parsed.Verified || parsed.PNGCount != 1 || len(parsed.PreviewImages) != 1 || len(parsed.Warnings) != 0 {
		t.Fatalf("Chinese presentation did not complete the render pipeline cleanly: %s", result)
	}
	for _, marker := range []string{"中文标题不会乱码", "正文应保持完整可读", "第一项：稳定生成", "第二项：字体可追踪"} {
		if !strings.Contains(readAllSlideXML(t, parsed.Path), marker) {
			t.Fatalf("Chinese marker %q was corrupted or dropped from OOXML", marker)
		}
	}
	assertAllCJKRunsHaveEastAsianTypeface(t, parsed.Path, "PingFang SC")
}

func TestPresentationCreateRejectsUnknownLayout(t *testing.T) {
	root := t.TempDir()
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))
	_, err := presentationCreate(ctx, mustJSON(t, map[string]any{
		"output_path": "deck.pptx",
		"slides": []map[string]any{{
			"title":  "Bad",
			"layout": "magic_layout",
		}},
	}))
	if err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("expected unsupported layout error, got %v", err)
	}
}

func TestPresentationEditRequiresSourceAndEditsAndPreservesOtherSlides(t *testing.T) {
	installHermeticPresentationRuntime(t)
	root := t.TempDir()
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))
	imagePath := filepath.Join(root, "keep.png")
	writeTestPNG(t, imagePath, 24, 16)

	createResult := executeArtifactToolRaw(t, ctx, presentationCreate, map[string]any{
		"output_path":    "deck.pptx",
		"render_preview": false,
		"slides": []map[string]any{
			{"title": "Keep Me", "layout": "title"},
			{
				"title":   "Change Me",
				"layout":  "image_right",
				"body":    "KEEP_UNSPECIFIED_BODY",
				"bullets": []string{"old"},
				"images":  []map[string]any{{"path": imagePath, "caption": "KEEP_IMAGE"}},
			},
		},
	})
	var created struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(createResult), &created); err != nil {
		t.Fatalf("parse create: %v\n%s", err, createResult)
	}

	_, err := presentationEdit(ctx, mustJSON(t, map[string]any{
		"output_path": "edited.pptx",
		"edits":       []map[string]any{{"slide_index": 1, "title": "Updated"}},
	}))
	if err == nil || !strings.Contains(err.Error(), "source_path") {
		t.Fatalf("expected source_path required, got %v", err)
	}

	editResult := executeArtifactToolRaw(t, ctx, presentationEdit, map[string]any{
		"source_path":    "deck.pptx",
		"output_path":    "edited.pptx",
		"render_preview": false,
		"edits": []map[string]any{{
			"op":          "update",
			"slide_index": 1,
			"title":       "Updated Title",
			"bullets":     []string{"new"},
		}},
	})
	var edited struct {
		Path       string `json:"path"`
		SlideCount int    `json:"slide_count"`
		Verified   bool   `json:"verified"`
	}
	if err := json.Unmarshal([]byte(editResult), &edited); err != nil {
		t.Fatalf("parse edit: %v\n%s", err, editResult)
	}
	if edited.SlideCount != 2 {
		t.Fatalf("expected preserved slide count 2, got %s", editResult)
	}
	if !edited.Verified {
		t.Fatalf("expected verified edit with hermetic runtime, got %s", editResult)
	}
	assertZipContains(t, edited.Path, "ppt/slides/slide1.xml")
	assertZipContains(t, edited.Path, "ppt/slides/slide2.xml")
	assertZipHasPrefix(t, edited.Path, "ppt/media/")
	slideXML := readZipEntry(t, edited.Path, "ppt/slides/slide2.xml")
	for _, expected := range []string{"Updated Title", "KEEP_UNSPECIFIED_BODY", "new"} {
		if !strings.Contains(slideXML, expected) {
			t.Fatalf("incremental update lost %q: %s", expected, slideXML)
		}
	}
}

func TestPresentationMixedEditsUseCurrentIndexesInRequestOrder(t *testing.T) {
	installHermeticPresentationRuntime(t)
	root := t.TempDir()
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))

	createResult := executeArtifactToolRaw(t, ctx, presentationCreate, map[string]any{
		"output_path":    "source.pptx",
		"render_preview": false,
		"slides": []map[string]any{
			{"title": "A Original", "layout": "title"},
			{"title": "B Original", "layout": "title"},
			{"title": "C Original", "layout": "title"},
		},
	})
	var created struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(createResult), &created); err != nil {
		t.Fatalf("parse create: %v\n%s", err, createResult)
	}

	editResult := executeArtifactToolRaw(t, ctx, presentationEdit, map[string]any{
		"source_path":    created.Path,
		"output_path":    "mixed.pptx",
		"render_preview": false,
		"edits": []map[string]any{
			{"op": "delete", "slide_index": 0},
			{"op": "update", "slide_index": 1, "title": "C Updated"},
		},
	})
	var edited struct {
		Path       string `json:"path"`
		SlideCount int    `json:"slide_count"`
		Verified   bool   `json:"verified"`
	}
	if err := json.Unmarshal([]byte(editResult), &edited); err != nil {
		t.Fatalf("parse edit: %v\n%s", err, editResult)
	}
	if edited.SlideCount != 2 || !edited.Verified {
		t.Fatalf("expected a verified two-slide deck, got %s", editResult)
	}

	slideXML := readAllSlideXML(t, edited.Path)
	for _, expected := range []string{"B Original", "C Updated"} {
		if !strings.Contains(slideXML, expected) {
			t.Fatalf("request-order edit lost %q: %s", expected, slideXML)
		}
	}
	for _, removed := range []string{"A Original", "C Original"} {
		if strings.Contains(slideXML, removed) {
			t.Fatalf("request-order edit retained stale text %q: %s", removed, slideXML)
		}
	}
}

func TestPresentationRenderPreviewFalseStillVerifies(t *testing.T) {
	installHermeticPresentationRuntime(t)
	root := t.TempDir()
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))
	result := executeArtifactToolRaw(t, ctx, presentationCreate, map[string]any{
		"output_path":    "deck.pptx",
		"render_preview": false,
		"slides": []map[string]any{{
			"title":  "Only Verify",
			"layout": "closing",
		}},
	})
	var parsed struct {
		Verified          bool     `json:"verified"`
		VerificationLevel string   `json:"verification_level"`
		PDFPath           string   `json:"pdf_path"`
		PreviewImages     []string `json:"preview_images"`
	}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("parse result: %v\n%s", err, result)
	}
	if !parsed.Verified || parsed.VerificationLevel != "png" {
		t.Fatalf("expected technical verification to succeed, got %s", result)
	}
	if parsed.PDFPath != "" || len(parsed.PreviewImages) != 0 {
		t.Fatalf("expected preview paths cleared when render_preview=false, got %s", result)
	}
}

func TestPresentationRerenderRemovesStalePreviewPages(t *testing.T) {
	installHermeticPresentationRuntime(t)
	root := t.TempDir()
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))

	firstResult := executeArtifactToolRaw(t, ctx, presentationCreate, map[string]any{
		"output_path": "deck.pptx",
		"slides": []map[string]any{
			{"title": "One", "layout": "title"},
			{"title": "Two", "layout": "title"},
			{"title": "Three", "layout": "title"},
		},
	})
	var first struct {
		PreviewDir    string   `json:"preview_dir"`
		PreviewImages []string `json:"preview_images"`
		PNGCount      int      `json:"png_count"`
		Verified      bool     `json:"verified"`
	}
	if err := json.Unmarshal([]byte(firstResult), &first); err != nil {
		t.Fatalf("parse first render: %v\n%s", err, firstResult)
	}
	if !first.Verified || first.PNGCount != 3 || len(first.PreviewImages) != 3 {
		t.Fatalf("expected an initial three-page preview, got %s", firstResult)
	}
	stalePage := filepath.Join(first.PreviewDir, "page-3.png")
	if _, err := os.Stat(stalePage); err != nil {
		t.Fatalf("expected initial page-3.png: %v", err)
	}

	secondResult := executeArtifactToolRaw(t, ctx, presentationCreate, map[string]any{
		"output_path": "deck.pptx",
		"slides": []map[string]any{
			{"title": "One revised", "layout": "title"},
			{"title": "Two revised", "layout": "title"},
		},
	})
	var second struct {
		PreviewDir    string   `json:"preview_dir"`
		PreviewImages []string `json:"preview_images"`
		PNGCount      int      `json:"png_count"`
		Verified      bool     `json:"verified"`
	}
	if err := json.Unmarshal([]byte(secondResult), &second); err != nil {
		t.Fatalf("parse second render: %v\n%s", err, secondResult)
	}
	if !second.Verified || second.PNGCount != 2 || len(second.PreviewImages) != 2 {
		t.Fatalf("expected a clean two-page preview, got %s", secondResult)
	}
	if second.PreviewDir != first.PreviewDir {
		t.Fatalf("expected the same preview directory, first=%s second=%s", first.PreviewDir, second.PreviewDir)
	}
	if _, err := os.Stat(stalePage); !os.IsNotExist(err) {
		t.Fatalf("stale page-3.png survived the 3-to-2 rerender, stat err=%v", err)
	}
}

func TestPresentationPopplerFailureIsNotVerified(t *testing.T) {
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
    --outdir) outdir="$2"; shift 2 ;;
    --convert-to) shift 2 ;;
    --headless|--nologo|--nofirststartwizard|--norestore) shift ;;
    -env:UserInstallation=*) shift ;;
    *) input="$1"; shift ;;
  esac
done
base="$(basename "${input%.*}")"
printf '%%PDF-1.4\n%%%%EOF\n' > "${outdir}/${base}.pdf"
`)
	writeExecutable(t, pdftoppm, `#!/bin/bash
echo "poppler missing" 1>&2
exit 1
`)
	setPresentationRenderingTestOverrides(t, soffice, bin)
	t.Setenv("HOME", t.TempDir())

	root := t.TempDir()
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))
	create := typedArtifactTool{
		name:        "presentation_create",
		inputSchema: presentationCreateInputSchema(),
		execute:     presentationCreate,
	}
	outcome, err := create.ExecuteOutcome(ctx, mustJSON(t, map[string]any{
		"output_path": "deck.pptx",
		"slides":      []map[string]any{{"title": "X", "layout": "title"}},
	}), nil)
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	var parsed struct {
		Verified          bool   `json:"verified"`
		VerificationLevel string `json:"verification_level"`
		NeedsRuntime      bool   `json:"needs_runtime"`
		ArtifactStatus    string `json:"artifact_status"`
		AttemptedPath     string `json:"attempted_path"`
	}
	if err := json.Unmarshal([]byte(outcome.Content), &parsed); err != nil {
		t.Fatalf("parse result: %v\n%s", err, outcome.Content)
	}
	if parsed.Verified {
		t.Fatalf("Poppler failure must not set verified=true: %s", outcome.Content)
	}
	if parsed.VerificationLevel != "pdf" || parsed.NeedsRuntime || parsed.ArtifactStatus != "error" {
		t.Fatalf("expected pdf-level render failure with error artifact, got %s", outcome.Content)
	}
	if outcome.Status != tool.OutcomeFailed || outcome.ErrorCode != errorCodePresentationVerificationFailed || outcome.ErrorCategory != tool.ErrorCategoryDomain {
		t.Fatalf("expected typed failed outcome, got %+v", outcome)
	}
	if parsed.AttemptedPath != filepath.Join(root, "deck.pptx") {
		t.Fatalf("failed publication must retain attempted_path diagnostic, got %s", outcome.Content)
	}
	if len(outcome.Artifacts) != 0 {
		t.Fatalf("failed publication must not claim a non-existent final artifact: %+v", outcome.Artifacts)
	}
	if _, err := os.Stat(filepath.Join(root, "deck.pptx")); !os.IsNotExist(err) {
		t.Fatalf("failed create must not leave the final path, stat err=%v", err)
	}
	assertNoPresentationPublicationTemps(t, root)
}

func TestPresentationCreateInputSchemaIsRecursive(t *testing.T) {
	schema := presentationCreateInputSchema()
	if schema.Properties["slides"].Items == nil || schema.Properties["slides"].Items.Properties["images"].Items == nil {
		t.Fatalf("expected nested slides/images schema")
	}
	err := tool.ValidateInputAgainstSchema(`{"output_path":"a.pptx","slides":[{"images":[{"path":"chart.png","width":"bad"}]}]}`, *schema)
	if err == nil || !strings.Contains(err.Error(), "$.slides[0].images[0].width expected number, got string") {
		t.Fatalf("expected nested path validation error, got %v", err)
	}
}

func TestPresentationCreateNestedValidationReportsExactJSONPathBeforeRuntimeAndExecutor(t *testing.T) {
	invalidateRuntimeProbeCache()
	executed := false
	create := typedArtifactTool{
		name:                "presentation_create",
		requiresRuntime:     true,
		runtimeRequirements: append([]string(nil), presentationAuthoringRuntimeRequirements...),
		inputSchema:         presentationCreateInputSchema(),
		execute: func(context.Context, string) (string, error) {
			executed = true
			return "", nil
		},
	}
	outcome, err := create.ExecuteOutcome(context.Background(), `{"output_path":"deck.pptx","slides":[{"images":[{"path":"chart.png","width":"bad"}]}]}`, nil)
	if err != nil {
		t.Fatalf("nested validation returned transport error: %v", err)
	}
	if executed {
		t.Fatal("nested validation error reached the presentation executor")
	}
	if outcome.Status != tool.OutcomeFailed || outcome.ErrorCode != tool.ErrorCodeValidationError || outcome.ErrorCategory != tool.ErrorCategoryValidation {
		t.Fatalf("expected typed validation_error, got %+v", outcome)
	}
	want := "$.slides[0].images[0].width expected number, got string"
	if outcome.Content != want || outcome.ErrorMessage != want {
		t.Fatalf("exact JSON path was not preserved: content=%q error=%q want=%q", outcome.Content, outcome.ErrorMessage, want)
	}
	if outcome.Status == tool.OutcomeNeedsRuntime || outcome.ErrorCode == tool.ErrorCodeNeedsRuntime {
		t.Fatalf("validation was incorrectly deferred to runtime readiness: %+v", outcome)
	}
}

func TestPresentationSchemasRejectUnknownFieldsAtEveryLevel(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		schema *tool.ToolParametersSchema
		want   string
	}{
		{"create root", `{"output_path":"a.pptx","mystery":true}`, presentationCreateInputSchema(), "$ has unexpected properties"},
		{"slide", `{"output_path":"a.pptx","slides":[{"mystery":true}]}`, presentationCreateInputSchema(), "$.slides[0] has unexpected properties"},
		{"image", `{"output_path":"a.pptx","slides":[{"images":[{"path":"x.png","mystery":true}]}]}`, presentationCreateInputSchema(), "$.slides[0].images[0] has unexpected properties"},
		{"edit", `{"source_path":"a.pptx","edits":[{"slide_index":0,"mystery":true}]}`, presentationEditInputSchema(), "$.edits[0] did not match any oneOf schema"},
		{"render", `{"path":"a.pptx","mystery":true}`, presentationRenderInputSchema(), "$ has unexpected properties"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := tool.ValidateInputAgainstSchema(test.input, *test.schema)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected closed schema error containing %q, got %v", test.want, err)
			}
		})
	}
}

func TestPresentationEditSchemaAcceptsOperationSpecificPayloads(t *testing.T) {
	valid := []string{
		`{"source_path":"a.pptx","edits":[{"op":"delete","slide_index":0}]}`,
		`{"source_path":"a.pptx","edits":[{"op":"insert","slide_index":0,"slide":{"title":"Inserted"}}]}`,
		`{"source_path":"a.pptx","edits":[{"op":"replace","slide_index":0,"slide":{"title":"Replacement"}}]}`,
		`{"source_path":"a.pptx","edits":[{"op":"update","slide_index":0,"title":"Updated"}]}`,
		`{"source_path":"a.pptx","edits":[{"slide_index":0,"title":"Implicit update"}]}`,
	}
	for _, input := range valid {
		if err := tool.ValidateInputAgainstSchema(input, *presentationEditInputSchema()); err != nil {
			t.Fatalf("expected operation-specific edit schema to accept %s: %v", input, err)
		}
	}
}

func TestPresentationEditInvalidPayloadFailsBeforeRuntimeAndExecutor(t *testing.T) {
	invalidateRuntimeProbeCache()

	invalid := []struct {
		name  string
		input string
	}{
		{"delete payload", `{"source_path":"a.pptx","edits":[{"op":"delete","slide_index":0,"title":"not allowed"}]}`},
		{"insert missing slide", `{"source_path":"a.pptx","edits":[{"op":"insert","slide_index":0}]}`},
		{"insert empty slide", `{"source_path":"a.pptx","edits":[{"op":"insert","slide_index":0,"slide":{}}]}`},
		{"replace missing slide", `{"source_path":"a.pptx","edits":[{"op":"replace","slide_index":0}]}`},
		{"replace empty slide", `{"source_path":"a.pptx","edits":[{"op":"replace","slide_index":0,"slide":{}}]}`},
		{"update missing fields", `{"source_path":"a.pptx","edits":[{"op":"update","slide_index":0}]}`},
		{"implicit update missing fields", `{"source_path":"a.pptx","edits":[{"slide_index":0}]}`},
	}
	for _, test := range invalid {
		t.Run(test.name, func(t *testing.T) {
			executed := false
			editTool := typedArtifactTool{
				name:                "presentation_edit",
				requiresRuntime:     true,
				runtimeRequirements: append([]string(nil), presentationAuthoringRuntimeRequirements...),
				inputSchema:         presentationEditInputSchema(),
				execute: func(context.Context, string) (string, error) {
					executed = true
					return "", nil
				},
			}
			outcome, err := editTool.ExecuteOutcome(context.Background(), test.input, nil)
			if err != nil {
				t.Fatalf("unexpected transport error: %v", err)
			}
			if executed {
				t.Fatal("invalid edit payload reached the executor")
			}
			if outcome.Status != tool.OutcomeFailed || outcome.ErrorCode != tool.ErrorCodeValidationError || outcome.ErrorCategory != tool.ErrorCategoryValidation {
				t.Fatalf("expected validation_error before runtime lookup, got %+v", outcome)
			}
			if outcome.Status == tool.OutcomeNeedsRuntime || outcome.ErrorCode == tool.ErrorCodeNeedsRuntime {
				t.Fatalf("invalid payload was misclassified as a runtime problem: %+v", outcome)
			}
		})
	}
}

func TestPresentationMissingImageReturnsStructuredValidationOutcome(t *testing.T) {
	root := t.TempDir()
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))
	create := typedArtifactTool{
		name:        "presentation_create",
		inputSchema: presentationCreateInputSchema(),
		execute:     presentationCreate,
	}
	outcome, err := create.ExecuteOutcome(ctx, mustJSON(t, map[string]any{
		"output_path": "deck.pptx",
		"slides": []map[string]any{{
			"title":  "Missing image",
			"images": []map[string]any{{"path": "missing.png"}},
		}},
	}), nil)
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	if outcome.Status != tool.OutcomeFailed || outcome.ErrorCode != tool.ErrorCodeValidationError || outcome.ErrorCategory != tool.ErrorCategoryValidation {
		t.Fatalf("expected structured validation failure, got %+v", outcome)
	}
	if !strings.Contains(outcome.Content, "$.slides[0].images[0].path") || !strings.Contains(outcome.Content, "does not exist") {
		t.Fatalf("expected image JSON path diagnostic, got %s", outcome.Content)
	}
}

func TestPresentationToolsDeclareComponentRuntimeRequirements(t *testing.T) {
	wants := map[string][]string{
		"presentation_create": {"python", "libreoffice", "poppler"},
		"presentation_edit":   {"python", "libreoffice", "poppler"},
		"presentation_render": {"libreoffice", "poppler"},
	}
	for name, want := range wants {
		registered, err := tool.GetTool(name)
		if err != nil {
			t.Fatalf("get %s: %v", name, err)
		}
		typed, ok := registered.(typedArtifactTool)
		if !ok {
			t.Fatalf("%s is not a typedArtifactTool: %T", name, registered)
		}
		if !typed.requiresRuntime || strings.Join(typed.runtimeRequirements, ",") != strings.Join(want, ",") {
			t.Fatalf("%s runtime requirements=%v requires=%v, want %v", name, typed.runtimeRequirements, typed.requiresRuntime, want)
		}
	}
}

func TestPresentationMissingHostDependencyReturnsTypedNeedsRuntimeOutcome(t *testing.T) {
	t.Setenv("PATH", "")
	t.Setenv("AWAKEN_PYTHON", "")
	t.Setenv("AWAKEN_SOFFICE", "")
	t.Setenv("AWAKEN_POPPLER_BIN", "")
	invalidateRuntimeProbeCache()
	root := t.TempDir()
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))
	create := typedArtifactTool{
		name:                "presentation_create",
		requiresRuntime:     true,
		runtimeRequirements: append([]string(nil), presentationAuthoringRuntimeRequirements...),
		inputSchema:         presentationCreateInputSchema(),
		execute: func(context.Context, string) (string, error) {
			t.Fatal("executor must not run when runtime is missing")
			return "", nil
		},
	}
	decision := create.CheckPermission(ctx, `{"output_path":"deck.pptx"}`, tool.NewPermissionContext(root, []string{root}))
	if decision.Behavior != tool.PermissionAllow {
		t.Fatalf("missing host dependency must not become a permission gate, got %+v", decision)
	}
	outcome, err := create.ExecuteOutcome(ctx, `{"output_path":"deck.pptx"}`, nil)
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	if outcome.Status != tool.OutcomeNeedsRuntime || outcome.ErrorCode != tool.ErrorCodeNeedsRuntime || outcome.ErrorCategory != tool.ErrorCategoryRuntime {
		t.Fatalf("expected needs_runtime outcome, got %+v", outcome)
	}
	if outcome.TransportStatus != tool.TransportNotStarted || !strings.Contains(outcome.Content, `"missing_components"`) {
		t.Fatalf("expected not-started structured runtime diagnostic, got %+v", outcome)
	}
	if _, err := os.Stat(filepath.Join(root, "deck.pptx")); !os.IsNotExist(err) {
		t.Fatalf("missing runtime must not create an artifact, stat err=%v", err)
	}
}

func TestPresentationVisualWarningsBlockVerifiedUntilCorrected(t *testing.T) {
	installHermeticPresentationRuntime(t)
	root := t.TempDir()
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))
	create := typedArtifactTool{
		name:        "presentation_create",
		inputSchema: presentationCreateInputSchema(),
		execute:     presentationCreate,
	}
	first, err := create.ExecuteOutcome(ctx, mustJSON(t, map[string]any{
		"output_path": "warning.pptx",
		"slides": []map[string]any{{
			"title":  "Warning",
			"layout": "bullets",
			"body":   strings.Repeat("内容", 700),
		}},
	}), nil)
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	var parsed struct {
		Verified       bool     `json:"verified"`
		ArtifactStatus string   `json:"artifact_status"`
		Warnings       []string `json:"warnings"`
		PNGCount       int      `json:"png_count"`
		AttemptedPath  string   `json:"attempted_path"`
	}
	if err := json.Unmarshal([]byte(first.Content), &parsed); err != nil {
		t.Fatalf("parse warning result: %v\n%s", err, first.Content)
	}
	if first.Status != tool.OutcomeFailed || first.ErrorCode != errorCodePresentationVerificationFailed || first.ErrorCategory != tool.ErrorCategoryDomain {
		t.Fatalf("visual warning must produce a typed failed outcome, got %+v", first)
	}
	if parsed.Verified || parsed.ArtifactStatus != "error" || parsed.PNGCount != 1 {
		t.Fatalf("render success with a visual warning must remain unverified/error, got %s", first.Content)
	}
	if len(parsed.Warnings) == 0 || !strings.Contains(strings.Join(parsed.Warnings, " "), "text may overflow") {
		t.Fatalf("expected engine warning in failed tool result, got %s", first.Content)
	}
	if parsed.AttemptedPath != filepath.Join(root, "warning.pptx") {
		t.Fatalf("warning failure must report attempted_path, got %s", first.Content)
	}
	if len(first.Artifacts) != 0 {
		t.Fatalf("warning failure must not publish an Artifact card: %+v", first.Artifacts)
	}
	if _, err := os.Stat(filepath.Join(root, "warning.pptx")); !os.IsNotExist(err) {
		t.Fatalf("warning-bearing create must not leave final output, stat err=%v", err)
	}
	assertNoPresentationPublicationTemps(t, root)

	second, err := create.ExecuteOutcome(ctx, mustJSON(t, map[string]any{
		"output_path": "warning.pptx",
		"slides": []map[string]any{{
			"title": "Corrected", "layout": "bullets", "body": "内容已收敛，不再溢出。",
		}},
	}), nil)
	if err != nil {
		t.Fatalf("unexpected corrected transport error: %v", err)
	}
	var corrected struct {
		Verified       bool     `json:"verified"`
		ArtifactStatus string   `json:"artifact_status"`
		Warnings       []string `json:"warnings"`
	}
	if err := json.Unmarshal([]byte(second.Content), &corrected); err != nil {
		t.Fatalf("parse corrected result: %v\n%s", err, second.Content)
	}
	if second.Status != tool.OutcomeSucceeded || !corrected.Verified || corrected.ArtifactStatus != "ready" || len(corrected.Warnings) != 0 {
		t.Fatalf("only the warning-free corrected artifact may be verified/ready, outcome=%+v content=%s", second, second.Content)
	}
}

func TestPresentationBlankSlideIsNotVerified(t *testing.T) {
	installHermeticPresentationRuntime(t)
	root := t.TempDir()
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))
	create := typedArtifactTool{
		name:        "presentation_create",
		inputSchema: presentationCreateInputSchema(),
		execute:     presentationCreate,
	}
	outcome, err := create.ExecuteOutcome(ctx, mustJSON(t, map[string]any{
		"output_path": "blank.pptx",
		"slides":      []map[string]any{{"layout": "bullets"}},
	}), nil)
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	var parsed struct {
		Verified       bool     `json:"verified"`
		ArtifactStatus string   `json:"artifact_status"`
		Warnings       []string `json:"warnings"`
	}
	if err := json.Unmarshal([]byte(outcome.Content), &parsed); err != nil {
		t.Fatalf("parse blank result: %v\n%s", err, outcome.Content)
	}
	if outcome.Status != tool.OutcomeFailed || parsed.Verified || parsed.ArtifactStatus != "error" ||
		!strings.Contains(strings.Join(parsed.Warnings, " "), "appears blank") {
		t.Fatalf("blank slide must remain an unverified error artifact, outcome=%+v content=%s", outcome, outcome.Content)
	}
	if len(outcome.Artifacts) != 0 {
		t.Fatalf("blank failed publication must not expose an Artifact card: %+v", outcome.Artifacts)
	}
	if _, err := os.Stat(filepath.Join(root, "blank.pptx")); !os.IsNotExist(err) {
		t.Fatalf("blank failed publication must not leave final output, stat err=%v", err)
	}
	assertNoPresentationPublicationTemps(t, root)
}

func TestPresentationAtomicInPlaceEditPreservesOriginalOnWarningAndPublishesOnlySuccess(t *testing.T) {
	installHermeticPresentationRuntime(t)
	root := t.TempDir()
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))
	create := typedArtifactTool{
		name:        "presentation_create",
		inputSchema: presentationCreateInputSchema(),
		execute:     presentationCreate,
	}
	edit := typedArtifactTool{
		name:        "presentation_edit",
		inputSchema: presentationEditInputSchema(),
		execute:     presentationEdit,
	}

	created, err := create.ExecuteOutcome(ctx, mustJSON(t, map[string]any{
		"output_path": "atomic.pptx",
		"slides": []map[string]any{{
			"title": "Original", "layout": "bullets", "body": "ORIGINAL_BODY",
		}},
	}), nil)
	if err != nil || created.Status != tool.OutcomeSucceeded {
		t.Fatalf("create original: err=%v outcome=%+v", err, created)
	}
	finalPath := filepath.Join(root, "atomic.pptx")
	before, err := fileSHA256(finalPath)
	if err != nil {
		t.Fatalf("hash original presentation: %v", err)
	}
	if info, err := os.Stat(finalPath); err != nil || info.Mode().Perm() != 0o644 {
		t.Fatalf("new presentation must publish with 0644 permissions, info=%v err=%v", info, err)
	}
	if err := os.Chmod(finalPath, 0o640); err != nil {
		t.Fatalf("set original presentation permissions: %v", err)
	}

	failed, err := edit.ExecuteOutcome(ctx, mustJSON(t, map[string]any{
		"source_path": "atomic.pptx",
		"edits": []map[string]any{{
			"op": "update", "slide_index": 0, "body": strings.Repeat("溢出内容", 400),
		}},
	}), nil)
	if err != nil {
		t.Fatalf("unexpected failed-edit transport error: %v", err)
	}
	if failed.Status != tool.OutcomeFailed || failed.ErrorCode != errorCodePresentationVerificationFailed {
		t.Fatalf("expected warning edit to fail verification, got %+v", failed)
	}
	afterFailure, err := fileSHA256(finalPath)
	if err != nil {
		t.Fatalf("hash presentation after failed edit: %v", err)
	}
	if afterFailure != before {
		t.Fatalf("failed in-place edit changed original SHA: before=%s after=%s", before, afterFailure)
	}
	if len(failed.Artifacts) != 0 {
		t.Fatalf("failed in-place edit must not mark the unchanged original as an error Artifact: %+v", failed.Artifacts)
	}
	assertNoPresentationPublicationTemps(t, root)

	succeeded, err := edit.ExecuteOutcome(ctx, mustJSON(t, map[string]any{
		"source_path": "atomic.pptx",
		"edits": []map[string]any{{
			"op": "update", "slide_index": 0, "title": "Published", "body": "SAFE_BODY",
		}},
	}), nil)
	if err != nil || succeeded.Status != tool.OutcomeSucceeded {
		t.Fatalf("successful in-place edit: err=%v outcome=%+v", err, succeeded)
	}
	afterSuccess, err := fileSHA256(finalPath)
	if err != nil {
		t.Fatalf("hash presentation after successful edit: %v", err)
	}
	if afterSuccess == before {
		t.Fatalf("successful in-place edit did not replace original SHA: %s", afterSuccess)
	}
	var result struct {
		Path           string   `json:"path"`
		Verified       bool     `json:"verified"`
		ArtifactStatus string   `json:"artifact_status"`
		PreviewDir     string   `json:"preview_dir"`
		PDFPath        string   `json:"pdf_path"`
		PreviewImages  []string `json:"preview_images"`
	}
	if err := json.Unmarshal([]byte(succeeded.Content), &result); err != nil {
		t.Fatalf("parse successful edit: %v\n%s", err, succeeded.Content)
	}
	wantPreviewDir := filepath.Join(root, "atomic_preview")
	if result.Path != finalPath || !result.Verified || result.ArtifactStatus != "ready" {
		t.Fatalf("successful publication must be ready at final path, got %s", succeeded.Content)
	}
	if result.PreviewDir != wantPreviewDir || result.PDFPath != filepath.Join(wantPreviewDir, "atomic.pdf") {
		t.Fatalf("preview report leaked staging paths: %s", succeeded.Content)
	}
	for _, path := range append([]string{result.PDFPath}, result.PreviewImages...) {
		if !strings.HasPrefix(filepath.Clean(path), filepath.Clean(wantPreviewDir)+string(filepath.Separator)) {
			t.Fatalf("preview path escaped final preview directory: %s", path)
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("reported preview path does not exist %s: %v", path, err)
		}
	}
	if info, err := os.Stat(wantPreviewDir); err != nil || info.Mode().Perm() != 0o755 {
		t.Fatalf("published preview directory must be 0755, info=%v err=%v", info, err)
	}
	if info, err := os.Stat(finalPath); err != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("in-place publication must preserve original permissions, info=%v err=%v", info, err)
	}
	if len(result.PreviewImages) == 0 {
		t.Fatal("successful publication did not report preview images")
	}
	if info, err := os.Stat(result.PreviewImages[0]); err != nil || info.Mode().Perm() != 0o644 {
		t.Fatalf("published preview files must be 0644, info=%v err=%v", info, err)
	}
	if len(succeeded.Artifacts) == 0 {
		t.Fatalf("successful publication must expose ready artifacts: %+v", succeeded.Artifacts)
	}
	for _, artifact := range succeeded.Artifacts {
		if artifact.Status != "ready" {
			t.Fatalf("successful publication exposed non-ready artifact: %+v", succeeded.Artifacts)
		}
	}
	assertNoPresentationPublicationTemps(t, root)
}

func TestPresentationAtomicPublishPreservesUnmanagedPreviewSentinelAndFinalDeck(t *testing.T) {
	installHermeticPresentationRuntime(t)
	root := t.TempDir()
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))
	create := typedArtifactTool{
		name:        "presentation_create",
		inputSchema: presentationCreateInputSchema(),
		execute:     presentationCreate,
	}
	edit := typedArtifactTool{
		name:        "presentation_edit",
		inputSchema: presentationEditInputSchema(),
		execute:     presentationEdit,
	}

	created, err := create.ExecuteOutcome(ctx, mustJSON(t, map[string]any{
		"output_path": "guarded.pptx",
		"slides": []map[string]any{{
			"title": "Original", "layout": "bullets", "body": "ORIGINAL_BODY",
		}},
	}), nil)
	if err != nil || created.Status != tool.OutcomeSucceeded {
		t.Fatalf("create guarded presentation: err=%v outcome=%+v", err, created)
	}
	finalPath := filepath.Join(root, "guarded.pptx")
	previewDir := filepath.Join(root, "guarded_preview")
	previewPage := filepath.Join(previewDir, "page-1.png")
	beforeDeck, err := fileSHA256(finalPath)
	if err != nil {
		t.Fatalf("hash guarded deck: %v", err)
	}
	beforePreview, err := fileSHA256(previewPage)
	if err != nil {
		t.Fatalf("hash guarded preview: %v", err)
	}
	sentinelPath := filepath.Join(previewDir, "user-notes.txt")
	const sentinel = "keep this user-owned file"
	if err := os.WriteFile(sentinelPath, []byte(sentinel), 0o640); err != nil {
		t.Fatalf("write preview sentinel: %v", err)
	}

	failed, err := edit.ExecuteOutcome(ctx, mustJSON(t, map[string]any{
		"source_path": "guarded.pptx",
		"edits": []map[string]any{{
			"op": "update", "slide_index": 0, "title": "Must not publish", "body": "CHANGED_BODY",
		}},
	}), nil)
	if err != nil {
		t.Fatalf("unexpected preview ownership transport error: %v", err)
	}
	if failed.Status != tool.OutcomeFailed || !strings.Contains(failed.Content, "unrelated file") {
		t.Fatalf("unmanaged preview directory must fail closed, got %+v", failed)
	}
	if len(failed.Artifacts) != 0 {
		t.Fatalf("failed ownership gate exposed artifacts: %+v", failed.Artifacts)
	}
	afterDeck, err := fileSHA256(finalPath)
	if err != nil {
		t.Fatalf("hash guarded deck after rejected publish: %v", err)
	}
	if afterDeck != beforeDeck {
		t.Fatalf("rejected preview swap changed final deck: before=%s after=%s", beforeDeck, afterDeck)
	}
	afterPreview, err := fileSHA256(previewPage)
	if err != nil {
		t.Fatalf("hash guarded preview after rejected publish: %v", err)
	}
	if afterPreview != beforePreview {
		t.Fatalf("rejected preview swap changed managed preview: before=%s after=%s", beforePreview, afterPreview)
	}
	content, err := os.ReadFile(sentinelPath)
	if err != nil || string(content) != sentinel {
		t.Fatalf("rejected preview swap did not preserve sentinel: content=%q err=%v", content, err)
	}
	if info, err := os.Stat(sentinelPath); err != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("rejected preview swap changed sentinel permissions: info=%v err=%v", info, err)
	}
	assertNoPresentationPublicationTemps(t, root)
}

func TestPresentationRenderRejectsUnmanagedSentinelWithoutMutatingPublishedPreview(t *testing.T) {
	installHermeticPresentationRuntime(t)
	root := t.TempDir()
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))
	executeArtifactToolRaw(t, ctx, presentationCreate, map[string]any{
		"output_path":    "standalone-source.pptx",
		"render_preview": false,
		"slides": []map[string]any{{
			"title": "Standalone", "layout": "bullets", "body": "ORIGINAL_BODY",
		}},
	})

	rendered, err := presentationRender(ctx, mustJSON(t, map[string]any{
		"path":       "standalone-source.pptx",
		"output_dir": "standalone-preview",
	}))
	if err != nil {
		t.Fatalf("initial standalone presentation render: %v", err)
	}
	var result struct {
		Verified      bool     `json:"verified"`
		PDFPath       string   `json:"pdf_path"`
		PreviewImages []string `json:"preview_images"`
	}
	if err := json.Unmarshal([]byte(rendered), &result); err != nil {
		t.Fatalf("parse standalone presentation render: %v\n%s", err, rendered)
	}
	if !result.Verified || result.PDFPath == "" || len(result.PreviewImages) != 1 {
		t.Fatalf("expected one verified standalone preview, got %s", rendered)
	}

	deckPath := filepath.Join(root, "standalone-source.pptx")
	previewDir := filepath.Join(root, "standalone-preview")
	pagePath := result.PreviewImages[0]
	beforeDeck, err := fileSHA256(deckPath)
	if err != nil {
		t.Fatalf("hash standalone source deck: %v", err)
	}
	beforePDF, err := fileSHA256(result.PDFPath)
	if err != nil {
		t.Fatalf("hash standalone preview PDF: %v", err)
	}
	beforePage, err := fileSHA256(pagePath)
	if err != nil {
		t.Fatalf("hash standalone preview page: %v", err)
	}
	if err := os.Chmod(result.PDFPath, 0o600); err != nil {
		t.Fatalf("set standalone preview PDF permissions: %v", err)
	}
	if err := os.Chmod(pagePath, 0o600); err != nil {
		t.Fatalf("set standalone preview page permissions: %v", err)
	}
	sentinelPath := filepath.Join(previewDir, "user-notes.txt")
	const sentinel = "keep standalone preview notes"
	if err := os.WriteFile(sentinelPath, []byte(sentinel), 0o640); err != nil {
		t.Fatalf("write standalone preview sentinel: %v", err)
	}

	_, err = presentationRender(ctx, mustJSON(t, map[string]any{
		"path":       "standalone-source.pptx",
		"output_dir": "standalone-preview",
	}))
	if err == nil || !strings.Contains(err.Error(), "unrelated file") {
		t.Fatalf("expected standalone preview ownership rejection, got %v", err)
	}
	afterDeck, hashErr := fileSHA256(deckPath)
	if hashErr != nil || afterDeck != beforeDeck {
		t.Fatalf("rejected standalone render changed source deck: before=%s after=%s err=%v", beforeDeck, afterDeck, hashErr)
	}
	afterPDF, hashErr := fileSHA256(result.PDFPath)
	if hashErr != nil || afterPDF != beforePDF {
		t.Fatalf("rejected standalone render changed preview PDF: before=%s after=%s err=%v", beforePDF, afterPDF, hashErr)
	}
	afterPage, hashErr := fileSHA256(pagePath)
	if hashErr != nil || afterPage != beforePage {
		t.Fatalf("rejected standalone render changed preview page: before=%s after=%s err=%v", beforePage, afterPage, hashErr)
	}
	if info, statErr := os.Stat(result.PDFPath); statErr != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("rejected standalone render changed preview PDF permissions: info=%v err=%v", info, statErr)
	}
	if info, statErr := os.Stat(pagePath); statErr != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("rejected standalone render changed preview page permissions: info=%v err=%v", info, statErr)
	}
	content, readErr := os.ReadFile(sentinelPath)
	if readErr != nil || string(content) != sentinel {
		t.Fatalf("rejected standalone render changed sentinel: content=%q err=%v", content, readErr)
	}
	if info, statErr := os.Stat(sentinelPath); statErr != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("rejected standalone render changed sentinel permissions: info=%v err=%v", info, statErr)
	}
	assertNoPresentationPublicationTemps(t, root)
}

func TestPresentationRenderFailurePreservesPublishedPreviewAndDoesNotCreateFinalDir(t *testing.T) {
	installHermeticPresentationRuntime(t)
	root := t.TempDir()
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))
	executeArtifactToolRaw(t, ctx, presentationCreate, map[string]any{
		"output_path":    "failure-source.pptx",
		"render_preview": false,
		"slides": []map[string]any{{
			"title": "Failure guard", "layout": "title",
		}},
	})

	rendered, err := presentationRender(ctx, mustJSON(t, map[string]any{
		"path":       "failure-source.pptx",
		"output_dir": "failure-preview",
	}))
	if err != nil {
		t.Fatalf("initial presentation render: %v", err)
	}
	var result struct {
		PDFPath       string   `json:"pdf_path"`
		PreviewImages []string `json:"preview_images"`
	}
	if err := json.Unmarshal([]byte(rendered), &result); err != nil {
		t.Fatalf("parse initial presentation render: %v\n%s", err, rendered)
	}
	if result.PDFPath == "" || len(result.PreviewImages) != 1 {
		t.Fatalf("expected published presentation preview, got %s", rendered)
	}
	pagePath := result.PreviewImages[0]
	beforePDF, err := fileSHA256(result.PDFPath)
	if err != nil {
		t.Fatalf("hash published preview PDF: %v", err)
	}
	beforePage, err := fileSHA256(pagePath)
	if err != nil {
		t.Fatalf("hash published preview page: %v", err)
	}
	if err := os.Chmod(result.PDFPath, 0o600); err != nil {
		t.Fatalf("set published preview PDF permissions: %v", err)
	}
	if err := os.Chmod(pagePath, 0o600); err != nil {
		t.Fatalf("set published preview page permissions: %v", err)
	}

	pdftoppm := filepath.Join(presentationRuntimeTestOverride("poppler"), "pdftoppm")
	writeExecutable(t, pdftoppm, "#!/bin/bash\nexit 9\n")
	failureContent, err := presentationRender(ctx, mustJSON(t, map[string]any{
		"path":       "failure-source.pptx",
		"output_dir": "failure-preview",
	}))
	if err == nil || !strings.Contains(err.Error(), "poppler preview render failed") {
		t.Fatalf("expected standalone presentation render failure, got %v", err)
	}
	if strings.Contains(failureContent, ".awaken-stage-") {
		t.Fatalf("failed render leaked staging path: %s", failureContent)
	}
	afterPDF, hashErr := fileSHA256(result.PDFPath)
	if hashErr != nil || afterPDF != beforePDF {
		t.Fatalf("failed rerender changed preview PDF: before=%s after=%s err=%v", beforePDF, afterPDF, hashErr)
	}
	afterPage, hashErr := fileSHA256(pagePath)
	if hashErr != nil || afterPage != beforePage {
		t.Fatalf("failed rerender changed preview page: before=%s after=%s err=%v", beforePage, afterPage, hashErr)
	}
	if info, statErr := os.Stat(result.PDFPath); statErr != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("failed rerender changed preview PDF permissions: info=%v err=%v", info, statErr)
	}
	if info, statErr := os.Stat(pagePath); statErr != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("failed rerender changed preview page permissions: info=%v err=%v", info, statErr)
	}

	neverPublished := filepath.Join(root, "never-published-preview")
	_, err = presentationRender(ctx, mustJSON(t, map[string]any{
		"path":       "failure-source.pptx",
		"output_dir": "never-published-preview",
	}))
	if err == nil {
		t.Fatal("expected initial presentation render failure")
	}
	if _, statErr := os.Lstat(neverPublished); !os.IsNotExist(statErr) {
		t.Fatalf("failed initial render left final output directory, stat err=%v", statErr)
	}
	assertNoPresentationPublicationTemps(t, root)
}

func TestArtifactRenderPDFRejectsUnmanagedSentinelWithoutMutatingPublishedPreview(t *testing.T) {
	installHermeticPresentationRuntime(t)
	root := t.TempDir()
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))
	pdfPath := filepath.Join(root, "source.pdf")
	if err := os.WriteFile(pdfPath, []byte("%PDF-1.4\n%%EOF\n"), 0o640); err != nil {
		t.Fatalf("write PDF source: %v", err)
	}
	if err := os.WriteFile(pdfPath+".pagecount", []byte("1"), 0o644); err != nil {
		t.Fatalf("write PDF page-count fixture: %v", err)
	}

	rendered, err := artifactRender(ctx, mustJSON(t, map[string]any{
		"path":       "source.pdf",
		"output_dir": "pdf-preview",
	}))
	if err != nil {
		t.Fatalf("initial PDF artifact render: %v", err)
	}
	var result struct {
		Verified      bool     `json:"verified"`
		PreviewImages []string `json:"preview_images"`
	}
	if err := json.Unmarshal([]byte(rendered), &result); err != nil {
		t.Fatalf("parse PDF artifact render: %v\n%s", err, rendered)
	}
	if !result.Verified || len(result.PreviewImages) != 1 {
		t.Fatalf("expected one verified PDF preview page, got %s", rendered)
	}
	pagePath := result.PreviewImages[0]
	beforeSource, err := fileSHA256(pdfPath)
	if err != nil {
		t.Fatalf("hash PDF source: %v", err)
	}
	beforePage, err := fileSHA256(pagePath)
	if err != nil {
		t.Fatalf("hash PDF preview page: %v", err)
	}
	if err := os.Chmod(pagePath, 0o600); err != nil {
		t.Fatalf("set PDF preview page permissions: %v", err)
	}
	sentinelPath := filepath.Join(root, "pdf-preview", "user-notes.txt")
	const sentinel = "keep PDF preview notes"
	if err := os.WriteFile(sentinelPath, []byte(sentinel), 0o640); err != nil {
		t.Fatalf("write PDF preview sentinel: %v", err)
	}

	_, err = artifactRender(ctx, mustJSON(t, map[string]any{
		"path":       "source.pdf",
		"output_dir": "pdf-preview",
	}))
	if err == nil || !strings.Contains(err.Error(), "unrelated file") {
		t.Fatalf("expected PDF preview ownership rejection, got %v", err)
	}
	afterSource, hashErr := fileSHA256(pdfPath)
	if hashErr != nil || afterSource != beforeSource {
		t.Fatalf("rejected PDF render changed source: before=%s after=%s err=%v", beforeSource, afterSource, hashErr)
	}
	afterPage, hashErr := fileSHA256(pagePath)
	if hashErr != nil || afterPage != beforePage {
		t.Fatalf("rejected PDF render changed old preview: before=%s after=%s err=%v", beforePage, afterPage, hashErr)
	}
	if info, statErr := os.Stat(pagePath); statErr != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("rejected PDF render changed old preview permissions: info=%v err=%v", info, statErr)
	}
	content, readErr := os.ReadFile(sentinelPath)
	if readErr != nil || string(content) != sentinel {
		t.Fatalf("rejected PDF render changed sentinel: content=%q err=%v", content, readErr)
	}
	if info, statErr := os.Stat(sentinelPath); statErr != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("rejected PDF render changed sentinel permissions: info=%v err=%v", info, statErr)
	}
	assertNoPresentationPublicationTemps(t, root)
}

func TestArtifactRenderPDFFailurePreservesPublishedPreviewAndSourceDirectory(t *testing.T) {
	installHermeticPresentationRuntime(t)
	root := t.TempDir()
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))
	sourceDir := filepath.Join(root, "documents")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatalf("create PDF source directory: %v", err)
	}
	pdfPath := filepath.Join(sourceDir, "failure.pdf")
	if err := os.WriteFile(pdfPath, []byte("%PDF-1.4\n%%EOF\n"), 0o640); err != nil {
		t.Fatalf("write PDF failure source: %v", err)
	}
	if err := os.WriteFile(pdfPath+".pagecount", []byte("1"), 0o644); err != nil {
		t.Fatalf("write PDF failure page-count fixture: %v", err)
	}

	rendered, err := artifactRender(ctx, mustJSON(t, map[string]any{
		"path":       "documents/failure.pdf",
		"output_dir": "failure-pdf-preview",
	}))
	if err != nil {
		t.Fatalf("initial PDF failure fixture render: %v", err)
	}
	var result struct {
		PreviewImages []string `json:"preview_images"`
	}
	if err := json.Unmarshal([]byte(rendered), &result); err != nil || len(result.PreviewImages) != 1 {
		t.Fatalf("parse initial PDF failure fixture render: err=%v result=%s", err, rendered)
	}
	pagePath := result.PreviewImages[0]
	beforeSource, err := fileSHA256(pdfPath)
	if err != nil {
		t.Fatalf("hash PDF failure source: %v", err)
	}
	beforePage, err := fileSHA256(pagePath)
	if err != nil {
		t.Fatalf("hash published PDF preview: %v", err)
	}
	if err := os.Chmod(pagePath, 0o600); err != nil {
		t.Fatalf("set published PDF preview permissions: %v", err)
	}
	_, err = artifactRender(ctx, mustJSON(t, map[string]any{
		"path":       "documents/failure.pdf",
		"output_dir": "documents",
	}))
	if err == nil || !strings.Contains(err.Error(), "unrelated file") {
		t.Fatalf("expected PDF source-directory ownership rejection, got %v", err)
	}
	afterSourceDirectoryAttempt, hashErr := fileSHA256(pdfPath)
	if hashErr != nil || afterSourceDirectoryAttempt != beforeSource {
		t.Fatalf("source-directory render changed source PDF: before=%s after=%s err=%v", beforeSource, afterSourceDirectoryAttempt, hashErr)
	}
	if info, statErr := os.Stat(pdfPath); statErr != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("source-directory render changed source PDF permissions: info=%v err=%v", info, statErr)
	}
	if _, statErr := os.Stat(filepath.Join(sourceDir, "page-1.png")); !os.IsNotExist(statErr) {
		t.Fatalf("rejected source-directory render published a preview page, stat err=%v", statErr)
	}

	pdftoppm := filepath.Join(presentationRuntimeTestOverride("poppler"), "pdftoppm")
	writeExecutable(t, pdftoppm, "#!/bin/bash\nexit 9\n")
	_, err = artifactRender(ctx, mustJSON(t, map[string]any{
		"path":       "documents/failure.pdf",
		"output_dir": "failure-pdf-preview",
	}))
	if err == nil || !strings.Contains(err.Error(), "poppler preview render failed") {
		t.Fatalf("expected PDF renderer failure, got %v", err)
	}
	afterSource, hashErr := fileSHA256(pdfPath)
	if hashErr != nil || afterSource != beforeSource {
		t.Fatalf("failed PDF rerender changed source: before=%s after=%s err=%v", beforeSource, afterSource, hashErr)
	}
	afterPage, hashErr := fileSHA256(pagePath)
	if hashErr != nil || afterPage != beforePage {
		t.Fatalf("failed PDF rerender changed old preview: before=%s after=%s err=%v", beforePage, afterPage, hashErr)
	}
	if info, statErr := os.Stat(pagePath); statErr != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("failed PDF rerender changed old preview permissions: info=%v err=%v", info, statErr)
	}
	assertNoPresentationPublicationTemps(t, root)
}

func TestPresentationAllLayoutBodiesAndThreeColumnComparisonReachOOXMLAndRender(t *testing.T) {
	installHermeticPresentationRuntime(t)
	root := t.TempDir()
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))
	imagePath := filepath.Join(root, "visual.png")
	writeTestPNG(t, imagePath, 32, 20)

	result := executeArtifactToolRaw(t, ctx, presentationCreate, map[string]any{
		"output_path": "semantic-fields.pptx",
		"title":       "DOCUMENT_TITLE_MARKER",
		"slides": []map[string]any{
			{"layout": "title", "title": "Title", "subtitle": "TITLE_SUBTITLE", "body": "TITLE_BODY"},
			{"layout": "section", "title": "Section", "subtitle": "SECTION_SUBTITLE", "body": "SECTION_BODY"},
			{"layout": "bullets", "title": "Bullets", "body": "BULLETS_BODY", "bullets": []string{"BULLETS_ITEM"}},
			{
				"layout": "two_column", "title": "Two", "body": "TWO_COLUMN_BODY",
				"columns": []map[string]any{
					{"title": "LEFT_COLUMN", "bullets": []string{"LEFT_VALUE"}},
					{"title": "RIGHT_COLUMN", "bullets": []string{"RIGHT_VALUE"}},
				},
			},
			{
				"layout": "image_right", "title": "Image", "body": "IMAGE_RIGHT_BODY",
				"images": []map[string]any{{"path": imagePath, "alt": "IMAGE_ALT_MARKER", "caption": "IMAGE_CAPTION"}},
			},
			{
				"layout": "comparison", "title": "Compare", "body": "COMPARISON_BODY",
				"columns": []map[string]any{
					{"title": "CHECK_COLUMN", "bullets": []string{"CHECK_ROW_1", "CHECK_ROW_2", "CHECK_ROW_3", "CHECK_ROW_4"}},
					{"title": "STANDARD_COLUMN", "bullets": []string{"STANDARD_ROW_1", "STANDARD_ROW_2", "STANDARD_ROW_3", "STANDARD_ROW_4"}},
					{"title": "RESULT_COLUMN", "bullets": []string{"RESULT_ROW_1", "RESULT_ROW_2", "RESULT_ROW_3", "RESULT_ROW_4"}},
				},
			},
			{
				"layout": "metrics", "title": "Metrics", "body": "METRICS_BODY",
				"metrics": []map[string]any{{"label": "METRIC_LABEL", "value": "METRIC_VALUE"}},
			},
			{
				"layout": "timeline", "title": "Timeline", "body": "TIMELINE_BODY",
				"timeline": []map[string]any{{"title": "STEP_TITLE", "detail": "STEP_DETAIL", "body": "STEP_BODY"}},
			},
			{"layout": "closing", "title": "Closing", "subtitle": "CLOSING_SUBTITLE", "body": "CLOSING_BODY", "bullets": []string{"CLOSING_ITEM"}},
		},
	})
	var parsed struct {
		Path          string   `json:"path"`
		Verified      bool     `json:"verified"`
		PNGCount      int      `json:"png_count"`
		PreviewImages []string `json:"preview_images"`
		Warnings      []string `json:"warnings"`
	}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("parse result: %v\n%s", err, result)
	}
	if !parsed.Verified || parsed.PNGCount != 9 || len(parsed.PreviewImages) != 9 {
		t.Fatalf("expected the nine-slide deck to complete PDF/PNG verification, got %s", result)
	}
	if len(parsed.Warnings) != 0 {
		t.Fatalf("semantic fields should render without truncation/overflow warnings, got %v", parsed.Warnings)
	}

	xml := readAllSlideXML(t, parsed.Path)
	for _, marker := range []string{
		"TITLE_BODY", "SECTION_BODY", "BULLETS_BODY", "TWO_COLUMN_BODY", "IMAGE_RIGHT_BODY",
		"COMPARISON_BODY", "METRICS_BODY", "TIMELINE_BODY", "CLOSING_BODY", "CLOSING_ITEM",
		"RESULT_COLUMN", "RESULT_ROW_1", "RESULT_ROW_2", "RESULT_ROW_3", "RESULT_ROW_4",
		"STEP_DETAIL", "STEP_BODY", "IMAGE_CAPTION", `descr="IMAGE_ALT_MARKER"`,
	} {
		if !strings.Contains(xml, marker) {
			t.Fatalf("accepted semantic field %q was not persisted in slide OOXML", marker)
		}
	}
	coreXML := readZipEntry(t, parsed.Path, "docProps/core.xml")
	if !strings.Contains(coreXML, "DOCUMENT_TITLE_MARKER") {
		t.Fatalf("top-level presentation title was not persisted as document metadata: %s", coreXML)
	}
}

func TestPresentationRejectsFieldCombinationsThatWouldBeIgnoredAsStructuredValidation(t *testing.T) {
	installHermeticPresentationRuntime(t)
	root := t.TempDir()
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))
	create := typedArtifactTool{
		name:        "presentation_create",
		inputSchema: presentationCreateInputSchema(),
		execute:     presentationCreate,
	}
	outcome, err := create.ExecuteOutcome(ctx, mustJSON(t, map[string]any{
		"output_path": "rejected.pptx",
		"slides": []map[string]any{{
			"layout":  "metrics",
			"title":   "Would silently drop bullets",
			"bullets": []string{"MUST_NOT_BE_IGNORED"},
			"metrics": []map[string]any{{"label": "Metric", "value": "1"}},
		}},
	}), nil)
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	if outcome.Status != tool.OutcomeFailed || outcome.ErrorCode != tool.ErrorCodeValidationError || outcome.ErrorCategory != tool.ErrorCategoryValidation {
		t.Fatalf("expected structured validation outcome, got %+v", outcome)
	}
	if !strings.Contains(outcome.Content, "$.slides[0].bullets") || !strings.Contains(outcome.Content, "cannot be combined") {
		t.Fatalf("expected exact unsupported field diagnostic, got %s", outcome.Content)
	}
	if path, _ := outcome.Diagnostic["path"].(string); path != "$.slides[0].bullets" {
		t.Fatalf("expected diagnostic JSON path, got %+v", outcome.Diagnostic)
	}
	if _, err := os.Stat(filepath.Join(root, "rejected.pptx")); !os.IsNotExist(err) {
		t.Fatalf("semantic validation failure must happen before writing an artifact, stat err=%v", err)
	}
}

func TestPresentationTimelineSlideBodyCanBeUpdatedAfterCreate(t *testing.T) {
	installHermeticPresentationRuntime(t)
	root := t.TempDir()
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))
	createdJSON := executeArtifactToolRaw(t, ctx, presentationCreate, map[string]any{
		"output_path":    "timeline.pptx",
		"render_preview": false,
		"slides": []map[string]any{{
			"layout": "timeline",
			"title":  "Timeline",
			"body":   "ORIGINAL_TIMELINE_BODY",
			"timeline": []map[string]any{{
				"title": "Step", "detail": "Detail", "body": "Step body",
			}},
		}},
	})
	var created struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(createdJSON), &created); err != nil {
		t.Fatalf("parse create: %v\n%s", err, createdJSON)
	}

	editedJSON := executeArtifactToolRaw(t, ctx, presentationEdit, map[string]any{
		"source_path":    created.Path,
		"output_path":    "timeline-edited.pptx",
		"render_preview": false,
		"edits": []map[string]any{{
			"op": "update", "slide_index": 0, "body": "UPDATED_TIMELINE_BODY",
		}},
	})
	var edited struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(editedJSON), &edited); err != nil {
		t.Fatalf("parse edit: %v\n%s", err, editedJSON)
	}
	xml := readAllSlideXML(t, edited.Path)
	if !strings.Contains(xml, "UPDATED_TIMELINE_BODY") || strings.Contains(xml, "ORIGINAL_TIMELINE_BODY") {
		t.Fatalf("timeline body update was not lossless: %s", xml)
	}
}

func TestPresentationSchemaCapsStructuredLayoutItemsBeforeExecution(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "comparison columns",
			input: `{"output_path":"a.pptx","slides":[{"layout":"comparison","columns":[{},{},{},{}]}]}`,
			want:  "$.slides[0].columns expected at most 3 items, got 4",
		},
		{
			name:  "metric cards",
			input: `{"output_path":"a.pptx","slides":[{"layout":"metrics","metrics":[{},{},{},{},{}]}]}`,
			want:  "$.slides[0].metrics expected at most 4 items, got 5",
		},
		{
			name:  "timeline steps",
			input: `{"output_path":"a.pptx","slides":[{"layout":"timeline","timeline":[{},{},{},{},{},{}]}]}`,
			want:  "$.slides[0].timeline expected at most 5 items, got 6",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := tool.ValidateInputAgainstSchema(test.input, *presentationCreateInputSchema())
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected schema validation %q, got %v", test.want, err)
			}
		})
	}
}

func readZipEntry(t *testing.T, path, entryName string) string {
	t.Helper()
	reader, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("open zip %s: %v", path, err)
	}
	defer reader.Close()
	for _, entry := range reader.File {
		if entry.Name != entryName {
			continue
		}
		rc, err := entry.Open()
		if err != nil {
			t.Fatalf("open %s: %v", entryName, err)
		}
		data, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatalf("read %s: %v", entryName, err)
		}
		return string(data)
	}
	t.Fatalf("missing zip entry %s", entryName)
	return ""
}

func readAllSlideXML(t *testing.T, path string) string {
	t.Helper()
	reader, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("open zip %s: %v", path, err)
	}
	defer reader.Close()
	var slides strings.Builder
	for _, entry := range reader.File {
		if !strings.HasPrefix(entry.Name, "ppt/slides/slide") || !strings.HasSuffix(entry.Name, ".xml") || strings.Contains(entry.Name, "_rels") {
			continue
		}
		rc, err := entry.Open()
		if err != nil {
			t.Fatalf("open %s: %v", entry.Name, err)
		}
		data, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name, err)
		}
		slides.Write(data)
		slides.WriteByte('\n')
	}
	return slides.String()
}

func assertNoPresentationPublicationTemps(t *testing.T, root string) {
	t.Helper()
	patterns := []string{
		filepath.Join(root, ".*.awaken-stage-*"),
		filepath.Join(root, ".*.awaken-backup-*"),
	}
	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatalf("glob publication temps %s: %v", pattern, err)
		}
		if len(matches) != 0 {
			t.Fatalf("presentation publication left temporary paths: %v", matches)
		}
	}
}

func assertPPTXHasNonEmptyEastAsianFonts(t *testing.T, path string) {
	t.Helper()
	reader, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("open pptx: %v", err)
	}
	defer reader.Close()
	found := false
	for _, file := range reader.File {
		if !strings.HasPrefix(file.Name, "ppt/slides/slide") || !strings.HasSuffix(file.Name, ".xml") {
			continue
		}
		if strings.Contains(file.Name, "_rels") {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			t.Fatalf("open %s: %v", file.Name, err)
		}
		data, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatalf("read %s: %v", file.Name, err)
		}
		if !bytes.Contains(data, []byte("a:ea")) {
			continue
		}
		// Require non-empty typeface on an a:ea node.
		content := string(data)
		idx := strings.Index(content, "a:ea")
		for idx >= 0 {
			window := content[idx:]
			end := strings.Index(window, "/>")
			if end < 0 {
				end = strings.Index(window, ">")
			}
			if end > 0 {
				node := window[:end]
				if strings.Contains(node, `typeface="`) {
					start := strings.Index(node, `typeface="`) + len(`typeface="`)
					rest := node[start:]
					closeQuote := strings.Index(rest, `"`)
					if closeQuote > 0 {
						found = true
						break
					}
				}
			}
			next := strings.Index(content[idx+1:], "a:ea")
			if next < 0 {
				break
			}
			idx = idx + 1 + next
		}
		if found {
			break
		}
	}
	if !found {
		t.Fatalf("expected non-empty a:ea typeface in pptx XML: %s", path)
	}
}

func assertAllCJKRunsHaveEastAsianTypeface(t *testing.T, path, wantTypeface string) {
	t.Helper()
	reader, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("open pptx: %v", err)
	}
	defer reader.Close()

	cjkRuns := 0
	for _, file := range reader.File {
		if !strings.HasPrefix(file.Name, "ppt/slides/slide") || !strings.HasSuffix(file.Name, ".xml") || strings.Contains(file.Name, "_rels") {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			t.Fatalf("open %s: %v", file.Name, err)
		}
		decoder := xml.NewDecoder(rc)
		inRun := false
		depth := 0
		var runText strings.Builder
		typeface := ""
		for {
			token, decodeErr := decoder.Token()
			if decodeErr == io.EOF {
				break
			}
			if decodeErr != nil {
				_ = rc.Close()
				t.Fatalf("decode %s: %v", file.Name, decodeErr)
			}
			switch value := token.(type) {
			case xml.StartElement:
				if !inRun && value.Name.Local == "r" {
					inRun = true
					depth = 1
					runText.Reset()
					typeface = ""
					continue
				}
				if inRun {
					depth++
					if value.Name.Local == "ea" {
						for _, attr := range value.Attr {
							if attr.Name.Local == "typeface" {
								typeface = strings.TrimSpace(attr.Value)
							}
						}
					}
				}
			case xml.CharData:
				if inRun {
					runText.Write([]byte(value))
				}
			case xml.EndElement:
				if !inRun {
					continue
				}
				if depth == 1 && value.Name.Local == "r" {
					text := runText.String()
					if containsCJKText(text) {
						cjkRuns++
						if typeface != wantTypeface {
							_ = rc.Close()
							t.Fatalf("CJK run %q in %s has east-Asian typeface %q, want %q", text, file.Name, typeface, wantTypeface)
						}
					}
					inRun = false
					depth = 0
					continue
				}
				depth--
			}
		}
		_ = rc.Close()
	}
	if cjkRuns == 0 {
		t.Fatalf("expected at least one CJK text run in %s", path)
	}
}

func containsCJKText(value string) bool {
	for _, r := range value {
		if unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul) {
			return true
		}
	}
	return false
}
