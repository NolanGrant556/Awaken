package artifactruntime

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"path/filepath"
	"strings"
	"testing"

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
			Layout:   "bullets",
			Title:    "标题页",
			Body:     "正文内容",
			Bullets:  []string{"要点一", "要点二"},
			Notes:    "演讲者备注",
		}},
		RenderPreview: false,
	}
	if err := writePPTXWithPython(ctx, spec); err != nil {
		t.Fatalf("write pptx: %v", err)
	}
	assertPPTXHasNonEmptyEastAsianFonts(t, output)
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

	createResult := executeArtifactToolRaw(t, ctx, presentationCreate, map[string]any{
		"output_path":    "deck.pptx",
		"render_preview": false,
		"slides": []map[string]any{
			{"title": "Keep Me", "layout": "title"},
			{"title": "Change Me", "layout": "bullets", "bullets": []string{"old"}},
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
			"layout":      "bullets",
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
	t.Setenv("AWAKEN_SOFFICE", soffice)
	t.Setenv("AWAKEN_POPPLER_BIN", bin)
	t.Setenv("HOME", t.TempDir())

	root := t.TempDir()
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))
	result := executeArtifactToolRaw(t, ctx, presentationCreate, map[string]any{
		"output_path": "deck.pptx",
		"slides":      []map[string]any{{"title": "X", "layout": "title"}},
	})
	var parsed struct {
		Verified          bool   `json:"verified"`
		VerificationLevel string `json:"verification_level"`
		NeedsRuntime      bool   `json:"needs_runtime"`
	}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("parse result: %v\n%s", err, result)
	}
	if parsed.Verified {
		t.Fatalf("Poppler failure must not set verified=true: %s", result)
	}
	if parsed.VerificationLevel != "pdf" || !parsed.NeedsRuntime {
		t.Fatalf("expected pdf-level needs_runtime result, got %s", result)
	}
}

func TestPresentationCreateInputSchemaIsRecursive(t *testing.T) {
	schema := presentationCreateInputSchema()
	if schema.Properties["slides"].Items == nil || schema.Properties["slides"].Items.Properties["images"].Items == nil {
		t.Fatalf("expected nested slides/images schema")
	}
	err := tool.ValidateInputAgainstSchema(`{"output_path":"a.pptx","slides":[{"images":[{"width":"bad"}]}]}`, *schema)
	if err == nil || !strings.Contains(err.Error(), "$.slides[0].images[0].width expected number, got string") {
		t.Fatalf("expected nested path validation error, got %v", err)
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
