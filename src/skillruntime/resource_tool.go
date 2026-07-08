package skillruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"awaken/src/skills"
	"awaken/src/tool"
)

const (
	ResourceReadToolName     = "skill_resource_read"
	maxSkillResourceReadSize = 2 << 20
)

// ResourceReadTool exposes read-only access to files bundled inside a local
// skill package. Paths are always resolved below the selected skill RootDir.
type ResourceReadTool struct {
	registry *skills.SkillRegistry
}

type resourceReadInput struct {
	SkillID string `json:"skill_id"`
	Path    string `json:"path"`
}

func (t *ResourceReadTool) Name() string { return ResourceReadToolName }

func (t *ResourceReadTool) Description() string {
	return "Read a file bundled inside a local skill package by skill_id and relative path."
}

func (t *ResourceReadTool) Parameters() []tool.ToolParameter {
	return []tool.ToolParameter{
		{Name: "skill_id", Type: "string", Description: "Skill ID whose package should be read.", Required: true},
		{Name: "path", Type: "string", Description: "Relative file path inside the skill package.", Required: true},
	}
}

func (t *ResourceReadTool) Definition() tool.ToolDefinition {
	return tool.ToolDefinition{
		Name:        ResourceReadToolName,
		DisplayName: "Skill Resource Read",
		Source:      tool.SourceSkill,
		Description: t.Description(),
		Annotations: tool.ToolAnnotations{
			ReadOnly:    true,
			Destructive: false,
			OpenWorld:   false,
		},
	}
}

func (t *ResourceReadTool) IsReadOnly() bool { return true }

func (t *ResourceReadTool) Execute(ctx context.Context, input string) (string, error) {
	_ = ctx
	if t.registry == nil {
		return "", fmt.Errorf("skill registry unavailable")
	}
	var req resourceReadInput
	if err := json.Unmarshal([]byte(strings.TrimSpace(input)), &req); err != nil {
		return "", fmt.Errorf("arguments must be a JSON object: %w", err)
	}
	skillID := strings.TrimSpace(req.SkillID)
	if skillID == "" {
		return "", fmt.Errorf("skill_id is required")
	}
	def := t.registry.Get(skillID)
	if def == nil {
		return "", fmt.Errorf("skill %q not found", skillID)
	}
	if strings.TrimSpace(def.RootDir) == "" {
		return "", fmt.Errorf("skill %q has no local package root", skillID)
	}
	target, err := resolveSkillResourcePath(def.RootDir, req.Path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(target)
	if err != nil {
		return "", fmt.Errorf("stat skill resource: %w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("skill resource path is a directory")
	}
	if info.Size() > maxSkillResourceReadSize {
		return "", fmt.Errorf("skill resource exceeds %d bytes", maxSkillResourceReadSize)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return "", fmt.Errorf("read skill resource: %w", err)
	}
	rel, _ := filepath.Rel(def.RootDir, target)
	return fmt.Sprintf("[skill resource %q:%s]\n\n%s", skillID, filepath.ToSlash(rel), string(data)), nil
}

func resolveSkillResourcePath(rootDir string, rawPath string) (string, error) {
	root := filepath.Clean(rootDir)
	rel := strings.TrimSpace(rawPath)
	if rel == "" {
		return "", fmt.Errorf("path is required")
	}
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("path must be relative to the skill package")
	}
	cleanRel := filepath.Clean(filepath.FromSlash(strings.ReplaceAll(rel, "\\", "/")))
	if cleanRel == "." || cleanRel == ".." || strings.HasPrefix(cleanRel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes the skill package")
	}
	target := filepath.Join(root, cleanRel)
	back, err := filepath.Rel(root, target)
	if err != nil {
		return "", fmt.Errorf("resolve skill resource: %w", err)
	}
	if back == ".." || strings.HasPrefix(back, ".."+string(filepath.Separator)) || filepath.IsAbs(back) {
		return "", fmt.Errorf("path escapes the skill package")
	}
	return target, nil
}
