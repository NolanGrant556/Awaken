package skillruntime

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"awaken/src/skills"
)

const defaultMaxResourceBytes int64 = 131072

// LoadSkillResources validates and optionally preloads skill package resources.
// required/preload resources are always loaded when loadPreload is true.
// Returns receipts, compatibility errors, and a map of relative path -> content
// for successfully loaded UTF-8 resources.
func LoadSkillResources(def *skills.SkillDefinition, loadPreload bool) ([]SkillResourceReceipt, []SkillCompatibilityError, map[string]string) {
	if def == nil {
		return nil, nil, nil
	}
	loaded := make(map[string]string)
	var receipts []SkillResourceReceipt
	var errs []SkillCompatibilityError
	version := strings.TrimSpace(def.Version)
	source := string(def.Source)
	if source == "" {
		source = string(skills.SourceUser)
	}

	for _, res := range def.RequiredResources {
		mode := res.Mode
		if mode == "" {
			mode = skills.ResourceModePreload
		}
		shouldLoad := res.Required || (loadPreload && mode == skills.ResourceModePreload)
		if !shouldLoad {
			continue
		}
		receipt, content, err := loadOneResource(def, res, version, source)
		if err != nil {
			if res.Required || mode == skills.ResourceModePreload {
				errs = append(errs, *err)
			}
			continue
		}
		receipts = append(receipts, receipt)
		if content != "" {
			loaded[receipt.Resource] = content
		}
	}
	return receipts, errs, loaded
}

func loadOneResource(
	def *skills.SkillDefinition,
	res skills.RequiredResource,
	version string,
	source string,
) (SkillResourceReceipt, string, *SkillCompatibilityError) {
	mode := string(res.Mode)
	if mode == "" {
		mode = string(skills.ResourceModePreload)
	}
	target, rel, pathErr := resolvePackageResource(def.RootDir, res.Path)
	if pathErr != nil {
		return SkillResourceReceipt{}, "", &SkillCompatibilityError{
			Code:    "skill_resource_invalid",
			Message: pathErr.Error(),
		}
	}
	info, err := os.Lstat(target)
	if err != nil {
		return SkillResourceReceipt{}, "", &SkillCompatibilityError{
			Code:    "skill_resource_missing",
			Message: fmt.Sprintf("required resource %q is missing", rel),
		}
	}
	if info.Mode()&os.ModeSymlink != 0 {
		resolved, resolveErr := filepath.EvalSymlinks(target)
		if resolveErr != nil {
			return SkillResourceReceipt{}, "", &SkillCompatibilityError{
				Code:    "skill_resource_invalid",
				Message: fmt.Sprintf("resource %q symlink could not be resolved", rel),
			}
		}
		rootReal, rootErr := filepath.EvalSymlinks(def.RootDir)
		if rootErr != nil {
			rootReal = filepath.Clean(def.RootDir)
		}
		back, relErr := filepath.Rel(rootReal, resolved)
		if relErr != nil || back == ".." || strings.HasPrefix(back, ".."+string(filepath.Separator)) {
			return SkillResourceReceipt{}, "", &SkillCompatibilityError{
				Code:    "skill_resource_invalid",
				Message: fmt.Sprintf("resource %q escapes the skill package via symlink", rel),
			}
		}
		target = resolved
		info, err = os.Stat(target)
		if err != nil {
			return SkillResourceReceipt{}, "", &SkillCompatibilityError{
				Code:    "skill_resource_missing",
				Message: fmt.Sprintf("required resource %q is missing", rel),
			}
		}
	}
	if info.IsDir() {
		return SkillResourceReceipt{}, "", &SkillCompatibilityError{
			Code:    "skill_resource_invalid",
			Message: fmt.Sprintf("resource %q is a directory", rel),
		}
	}
	maxBytes := res.MaxBytes
	if maxBytes <= 0 {
		maxBytes = defaultMaxResourceBytes
	}
	if info.Size() > maxBytes {
		return SkillResourceReceipt{}, "", &SkillCompatibilityError{
			Code:    "skill_resource_too_large",
			Message: fmt.Sprintf("resource %q exceeds maxBytes=%d", rel, maxBytes),
		}
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return SkillResourceReceipt{}, "", &SkillCompatibilityError{
			Code:    "skill_resource_missing",
			Message: fmt.Sprintf("failed to read resource %q: %v", rel, err),
		}
	}
	if !utf8.Valid(data) {
		return SkillResourceReceipt{}, "", &SkillCompatibilityError{
			Code:    "skill_resource_invalid",
			Message: fmt.Sprintf("resource %q is not valid UTF-8", rel),
		}
	}
	sum := sha256.Sum256(data)
	actual := hex.EncodeToString(sum[:])
	if expected := strings.TrimSpace(res.SHA256); expected != "" {
		if expected != actual {
			return SkillResourceReceipt{}, "", &SkillCompatibilityError{
				Code:    "skill_resource_hash_mismatch",
				Message: fmt.Sprintf("resource %q sha256 mismatch", rel),
			}
		}
	}
	receipt := SkillResourceReceipt{
		SkillID:      def.ID,
		SkillVersion: version,
		Resource:     rel,
		Source:       source,
		SHA256:       actual,
		Size:         int64(len(data)),
		Mode:         mode,
		LoadedAt:     time.Now().UTC(),
		Content:      string(data),
	}
	return receipt, string(data), nil
}

func resolvePackageResource(rootDir, rawPath string) (absPath string, relSlash string, err error) {
	root := filepath.Clean(strings.TrimSpace(rootDir))
	if root == "" {
		return "", "", fmt.Errorf("skill package root is empty")
	}
	rel := strings.TrimSpace(rawPath)
	if rel == "" {
		return "", "", fmt.Errorf("resource path is required")
	}
	if filepath.IsAbs(rel) {
		return "", "", fmt.Errorf("resource path must be relative to the skill package")
	}
	cleanRel := filepath.Clean(filepath.FromSlash(strings.ReplaceAll(rel, "\\", "/")))
	if cleanRel == "." || cleanRel == ".." || strings.HasPrefix(cleanRel, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("resource path escapes the skill package")
	}
	target := filepath.Join(root, cleanRel)
	back, err := filepath.Rel(root, target)
	if err != nil {
		return "", "", fmt.Errorf("resolve resource path: %w", err)
	}
	if back == ".." || strings.HasPrefix(back, ".."+string(filepath.Separator)) || filepath.IsAbs(back) {
		return "", "", fmt.Errorf("resource path escapes the skill package")
	}
	return target, filepath.ToSlash(cleanRel), nil
}
