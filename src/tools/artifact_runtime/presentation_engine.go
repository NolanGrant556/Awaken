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

type presentationEngineReport struct {
	Path     string   `json:"path"`
	Slides   int      `json:"slides"`
	Size     int64    `json:"size"`
	Warnings []string `json:"warnings"`
}

type presentationEngineErrorReport struct {
	ErrorCode string `json:"error_code"`
	Path      string `json:"path"`
	Reason    string `json:"reason"`
}

func writePPTXWithPython(ctx context.Context, spec presentationSpec) (presentationEngineReport, error) {
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

func editPPTXWithPython(ctx context.Context, spec presentationEditSpec) (presentationEngineReport, error) {
	payload := map[string]any{
		"mode":        "edit",
		"source_path": spec.SourcePath,
		"output_path": spec.OutputPath,
		"theme":       spec.Theme,
		"edits":       spec.Edits,
	}
	return runPresentationEngine(ctx, payload, spec.OutputPath)
}

func runPresentationEngine(ctx context.Context, payload map[string]any, outputPath string) (presentationEngineReport, error) {
	python, err := resolvePresentationPython()
	if err != nil {
		return presentationEngineReport{}, err
	}
	scriptPath, cleanup, err := materializePresentationEngine()
	if err != nil {
		return presentationEngineReport{}, err
	}
	defer cleanup()

	body, err := json.Marshal(payload)
	if err != nil {
		return presentationEngineReport{}, err
	}
	runCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(runCtx, python, scriptPath)
	cmd.Stdin = bytes.NewReader(body)
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if runCtx.Err() != nil {
		return presentationEngineReport{}, fmt.Errorf("presentation engine timed out: %w", runCtx.Err())
	}
	if err != nil {
		var failure presentationEngineErrorReport
		if json.Unmarshal(bytes.TrimSpace(output), &failure) == nil &&
			strings.TrimSpace(failure.ErrorCode) != "" &&
			strings.TrimSpace(failure.Path) != "" &&
			strings.TrimSpace(failure.Reason) != "" {
			return presentationEngineReport{}, &presentationInputError{
				Path:   failure.Path,
				Reason: failure.Reason,
			}
		}
		return presentationEngineReport{}, fmt.Errorf("presentation engine failed: %w: %s", err, excerptOutput(output))
	}
	var report presentationEngineReport
	if err := json.Unmarshal(output, &report); err != nil {
		return presentationEngineReport{}, fmt.Errorf("presentation engine returned invalid report: %w: %s", err, excerptOutput(output))
	}
	info, err := os.Stat(outputPath)
	if err != nil {
		return presentationEngineReport{}, fmt.Errorf("generated pptx missing: %w", err)
	}
	if info.Size() == 0 {
		return presentationEngineReport{}, fmt.Errorf("generated pptx is empty")
	}
	if strings.TrimSpace(report.Path) == "" {
		report.Path = outputPath
	}
	if report.Size <= 0 {
		report.Size = info.Size()
	}
	return report, nil
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
