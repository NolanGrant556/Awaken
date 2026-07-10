package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	legacyInstallPattern = regexp.MustCompile(`(?i)\b(pip(?:3)?\s+install|npm\s+install|pnpm\s+add|yarn\s+add)\b`)
	legacyGeneratePattern = regexp.MustCompile(`(?i)\bgenerate\.py\b`)
	legacyShellBinaryPattern = regexp.MustCompile(`(?i)\b(soffice|pdftoppm)\b`)
	legacyOfficeZipPattern = regexp.MustCompile(`(?i)(zipfile|ZipFile|Content_Types\.xml|\[Content_Types\]\.xml|ppt/slides/|word/document\.xml|xl/worksheets/)`)
)

// DetectPlatformContractConflicts returns human-readable conflict reasons when
// a skill package uses legacy Artifact patterns that conflict with the
// platform typed-tool contract. It never rewrites or deletes skill files.
func DetectPlatformContractConflicts(def *SkillDefinition) []string {
	if def == nil {
		return nil
	}
	var reasons []string
	scanText := func(label, text string) {
		if text == "" {
			return
		}
		if legacyInstallPattern.MatchString(text) {
			reasons = append(reasons, label+": package manager install (pip/npm/pnpm/yarn)")
		}
		if legacyGeneratePattern.MatchString(text) {
			reasons = append(reasons, label+": dynamic generate.py engine")
		}
		if legacyShellBinaryPattern.MatchString(text) {
			reasons = append(reasons, label+": direct shell soffice/pdftoppm")
		}
		if legacyOfficeZipPattern.MatchString(text) {
			reasons = append(reasons, label+": hand-written Office ZIP/XML")
		}
	}
	scanText("SKILL.md", def.Body)
	if root := strings.TrimSpace(def.RootDir); root != "" {
		_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil || info == nil || info.IsDir() {
				return nil
			}
			name := strings.ToLower(info.Name())
			if name == "generate.py" {
				reasons = append(reasons, "package: dynamic generate.py engine")
			}
			ext := strings.ToLower(filepath.Ext(name))
			switch ext {
			case ".md", ".py", ".sh", ".js", ".ts", ".json", ".txt", ".yml", ".yaml":
			default:
				return nil
			}
			data, readErr := os.ReadFile(path)
			if readErr != nil || len(data) > 512*1024 {
				return nil
			}
			rel, _ := filepath.Rel(root, path)
			scanText(filepath.ToSlash(rel), string(data))
			return nil
		})
	}
	policy := def.EffectiveToolPolicy()
	for _, name := range append(append([]string{}, policy.Allow...), def.RequiredTools...) {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		// Presence of unknown tools is checked at activation against the live
		// registry; here we only flag clearly non-platform shell/office paths.
		if strings.EqualFold(name, "soffice") || strings.EqualFold(name, "pdftoppm") {
			reasons = append(reasons, "toolPolicy: non-platform binary "+name)
		}
	}
	return uniqueStrings(reasons)
}

// ApplyPlatformContractConflict marks the skill unavailable when conflicts
// are detected. Existing UnavailableCode is preserved when already set.
func ApplyPlatformContractConflict(def *SkillDefinition) bool {
	conflicts := DetectPlatformContractConflicts(def)
	if len(conflicts) == 0 || def == nil {
		return false
	}
	def.State = StateUnavailable
	def.UnavailableCode = ErrorCodePlatformContractConflict
	def.UnavailableReason = ErrorCodePlatformContractConflict + ": " + strings.Join(conflicts, "; ")
	return true
}

func uniqueStrings(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// FormatPlatformConflictReason builds a stable unavailable reason string.
func FormatPlatformConflictReason(conflicts []string) string {
	if len(conflicts) == 0 {
		return ErrorCodePlatformContractConflict
	}
	return fmt.Sprintf("%s: %s", ErrorCodePlatformContractConflict, strings.Join(conflicts, "; "))
}
