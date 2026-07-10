package artifactruntime

import (
	"archive/zip"
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

//go:embed engines/presentation_engine.py
var presentationEngineScript []byte

func writePPTXWithPython(ctx context.Context, spec presentationSpec) error {
	payload := map[string]any{
		"mode":           "create",
		"output_path":    spec.OutputPath,
		"title":          spec.Title,
		"theme":          spec.Theme,
		"slides":         spec.Slides,
		"render_preview": spec.RenderPreview,
		"preview_dir":    spec.PreviewDir,
	}
	return runPresentationEngine(ctx, payload, spec.OutputPath)
}

func editPPTXWithPython(ctx context.Context, spec presentationEditSpec) error {
	payload := map[string]any{
		"mode":        "edit",
		"source_path": spec.SourcePath,
		"output_path": spec.OutputPath,
		"theme":       spec.Theme,
		"edits":       spec.Edits,
	}
	return runPresentationEngine(ctx, payload, spec.OutputPath)
}

func runPresentationEngine(ctx context.Context, payload map[string]any, outputPath string) error {
	python, err := resolvePresentationPython()
	if err != nil {
		return err
	}
	scriptPath, cleanup, err := materializePresentationEngine()
	if err != nil {
		return err
	}
	defer cleanup()

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	runCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(runCtx, python, scriptPath)
	cmd.Stdin = bytes.NewReader(body)
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if runCtx.Err() != nil {
		return fmt.Errorf("presentation engine timed out: %w", runCtx.Err())
	}
	if err != nil {
		return fmt.Errorf("presentation engine failed: %w: %s", err, excerptOutput(output))
	}
	info, err := os.Stat(outputPath)
	if err != nil {
		return fmt.Errorf("generated pptx missing: %w", err)
	}
	if info.Size() == 0 {
		return fmt.Errorf("generated pptx is empty")
	}
	return nil
}

func materializePresentationEngine() (string, func(), error) {
	dir, err := os.MkdirTemp("", "awaken-presentation-engine-*")
	if err != nil {
		return "", func() {}, err
	}
	path := filepath.Join(dir, "presentation_engine.py")
	if err := os.WriteFile(path, presentationEngineScript, 0o644); err != nil {
		_ = os.RemoveAll(dir)
		return "", func() {}, err
	}
	return path, func() { _ = os.RemoveAll(dir) }, nil
}

func countPPTXSlides(path string) (int, error) {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return 0, err
	}
	defer reader.Close()
	count := 0
	for _, file := range reader.File {
		name := file.Name
		if !strings.HasPrefix(name, "ppt/slides/slide") || !strings.HasSuffix(name, ".xml") {
			continue
		}
		if strings.Contains(name, "_rels") {
			continue
		}
		base := filepath.Base(name)
		if strings.HasPrefix(base, "slide") && strings.HasSuffix(base, ".xml") {
			count++
		}
	}
	return count, nil
}
