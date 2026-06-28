package agentdefs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"awaken/src/tool"
)

const (
	DefaultMaxFileBytes         int64 = 64 * 1024
	DefaultMaxSystemPromptBytes       = 16 * 1024
)

var ErrInvalidWorkspace = errors.New("invalid workspace root")

type Options struct {
	MaxFileBytes         int64
	MaxSystemPromptBytes int
}

type Result struct {
	WorkspaceRoot string        `json:"workspace_root"`
	Definitions   []Definition  `json:"definitions"`
	Skipped       []SkippedFile `json:"skipped,omitempty"`
}

type Definition struct {
	ID                    string   `json:"id"`
	Name                  string   `json:"name"`
	Description           string   `json:"description,omitempty"`
	Model                 string   `json:"model,omitempty"`
	Tools                 []string `json:"tools,omitempty"`
	DisallowedTools       []string `json:"disallowed_tools,omitempty"`
	PermissionMode        string   `json:"permission_mode,omitempty"`
	SystemPrompt          string   `json:"system_prompt"`
	SystemPromptTruncated bool     `json:"system_prompt_truncated,omitempty"`
	Source                Source   `json:"source"`
	Warnings              []string `json:"warnings,omitempty"`
}

type Source struct {
	Kind         string `json:"kind"`
	RelativePath string `json:"relative_path"`
}

type SkippedFile struct {
	RelativePath string `json:"relative_path"`
	Reason       string `json:"reason"`
	Message      string `json:"message"`
}

type sourceDir struct {
	kind        string
	relativeDir string
}

var sourceDirs = []sourceDir{
	{kind: "awaken", relativeDir: ".awaken/agents"},
	{kind: "claude_compat", relativeDir: ".claude/agents"},
}

func Load(workspaceRoot string) (Result, error) {
	return LoadWithOptions(workspaceRoot, Options{})
}

func LoadWithOptions(workspaceRoot string, options Options) (Result, error) {
	opts := normalizeOptions(options)
	root, realRoot, err := normalizeWorkspaceRoot(workspaceRoot)
	if err != nil {
		return Result{}, err
	}

	result := Result{
		WorkspaceRoot: root,
		Definitions:   []Definition{},
	}

	for _, source := range sourceDirs {
		definitions, skipped := loadSourceDir(root, realRoot, source, opts)
		result.Definitions = append(result.Definitions, definitions...)
		result.Skipped = append(result.Skipped, skipped...)
	}

	sort.SliceStable(result.Definitions, func(i, j int) bool {
		if result.Definitions[i].Source.Kind != result.Definitions[j].Source.Kind {
			return result.Definitions[i].Source.Kind < result.Definitions[j].Source.Kind
		}
		return result.Definitions[i].Source.RelativePath < result.Definitions[j].Source.RelativePath
	})

	return result, nil
}

func normalizeOptions(options Options) Options {
	if options.MaxFileBytes <= 0 {
		options.MaxFileBytes = DefaultMaxFileBytes
	}
	if options.MaxSystemPromptBytes <= 0 {
		options.MaxSystemPromptBytes = DefaultMaxSystemPromptBytes
	}
	return options
}

func normalizeWorkspaceRoot(workspaceRoot string) (string, string, error) {
	root := strings.TrimSpace(workspaceRoot)
	if root == "" {
		return "", "", fmt.Errorf("%w: workspace_root is required", ErrInvalidWorkspace)
	}
	if !filepath.IsAbs(root) {
		return "", "", fmt.Errorf("%w: workspace_root must be an absolute path", ErrInvalidWorkspace)
	}
	root = filepath.Clean(root)

	info, err := os.Stat(root)
	if err != nil {
		return "", "", fmt.Errorf("%w: cannot access workspace_root: %v", ErrInvalidWorkspace, err)
	}
	if !info.IsDir() {
		return "", "", fmt.Errorf("%w: workspace_root must be a directory", ErrInvalidWorkspace)
	}

	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", "", fmt.Errorf("%w: cannot resolve workspace_root: %v", ErrInvalidWorkspace, err)
	}
	return root, filepath.Clean(realRoot), nil
}

func loadSourceDir(root, realRoot string, source sourceDir, opts Options) ([]Definition, []SkippedFile) {
	dir := filepath.Join(root, filepath.FromSlash(source.relativeDir))
	realDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, []SkippedFile{{
			RelativePath: filepath.ToSlash(source.relativeDir),
			Reason:       "unresolvable_path",
			Message:      "agent directory could not be resolved safely",
		}}
	}
	if !tool.PathWithinRoot(realDir, realRoot) {
		return nil, []SkippedFile{{
			RelativePath: filepath.ToSlash(source.relativeDir),
			Reason:       "outside_workspace",
			Message:      "agent directory resolves outside workspace",
		}}
	}

	definitions := []Definition{}
	skipped := []SkippedFile{}
	if err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			relativePath, relErr := filepath.Rel(root, path)
			if relErr != nil {
				relativePath = source.relativeDir
			}
			skipped = append(skipped, SkippedFile{
				RelativePath: filepath.ToSlash(relativePath),
				Reason:       "read_error",
				Message:      "agent directory entry could not be read",
			})
			return nil
		}
		if path == dir || entry.IsDir() {
			return nil
		}

		name := entry.Name()
		if !isCandidateMarkdownFile(name) {
			return nil
		}

		relativePath, relErr := filepath.Rel(root, path)
		if relErr != nil {
			skipped = append(skipped, SkippedFile{
				RelativePath: filepath.ToSlash(source.relativeDir),
				Reason:       "read_error",
				Message:      "agent definition path could not be resolved",
			})
			return nil
		}
		relativePath = filepath.ToSlash(relativePath)
		if isSecretLikeFilename(name) {
			skipped = append(skipped, SkippedFile{
				RelativePath: relativePath,
				Reason:       "secret_like_path",
				Message:      "secret-like agent definition filename was ignored",
			})
			return nil
		}

		definition, skip, ok := loadDefinitionFile(realRoot, source, path, relativePath, opts)
		if !ok {
			skipped = append(skipped, skip)
			return nil
		}
		definitions = append(definitions, definition)
		return nil
	}); err != nil {
		return nil, []SkippedFile{{
			RelativePath: filepath.ToSlash(source.relativeDir),
			Reason:       "read_error",
			Message:      "agent directory could not be read",
		}}
	}

	return definitions, skipped
}

func isCandidateMarkdownFile(name string) bool {
	if name == "" || name != filepath.Base(name) || strings.HasPrefix(name, ".") {
		return false
	}
	return strings.EqualFold(filepath.Ext(name), ".md")
}

func isSecretLikeFilename(name string) bool {
	lower := strings.ToLower(name)
	secretTokens := []string{
		".env",
		"apikey",
		"api-key",
		"credential",
		"credentials",
		"password",
		"private-key",
		"secret",
		"secrets",
		"token",
	}
	for _, token := range secretTokens {
		if strings.Contains(lower, token) {
			return true
		}
	}
	return false
}

func loadDefinitionFile(realRoot string, source sourceDir, fullPath, relativePath string, opts Options) (Definition, SkippedFile, bool) {
	realPath, err := filepath.EvalSymlinks(fullPath)
	if err != nil {
		return Definition{}, SkippedFile{
			RelativePath: relativePath,
			Reason:       "unresolvable_path",
			Message:      "agent definition could not be resolved safely",
		}, false
	}
	if !tool.PathWithinRoot(realPath, realRoot) {
		return Definition{}, SkippedFile{
			RelativePath: relativePath,
			Reason:       "outside_workspace",
			Message:      "agent definition resolves outside workspace",
		}, false
	}

	info, err := os.Stat(realPath)
	if err != nil || !info.Mode().IsRegular() {
		return Definition{}, SkippedFile{
			RelativePath: relativePath,
			Reason:       "not_regular_file",
			Message:      "agent definition is not a regular readable file",
		}, false
	}
	if info.Size() > opts.MaxFileBytes {
		return Definition{}, SkippedFile{
			RelativePath: relativePath,
			Reason:       "file_too_large",
			Message:      fmt.Sprintf("agent definition exceeds %d bytes", opts.MaxFileBytes),
		}, false
	}

	content, err := os.ReadFile(realPath)
	if err != nil {
		return Definition{}, SkippedFile{
			RelativePath: relativePath,
			Reason:       "read_error",
			Message:      "agent definition could not be read",
		}, false
	}
	if int64(len(content)) > opts.MaxFileBytes {
		return Definition{}, SkippedFile{
			RelativePath: relativePath,
			Reason:       "file_too_large",
			Message:      fmt.Sprintf("agent definition exceeds %d bytes", opts.MaxFileBytes),
		}, false
	}

	definition := parseDefinition(string(content), source, relativePath, opts)
	return definition, SkippedFile{}, true
}

func parseDefinition(markdown string, source sourceDir, relativePath string, opts Options) Definition {
	metadata, body, warnings := parseMarkdown(markdown)
	name := strings.TrimSpace(metadata.Name)
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(relativePath), filepath.Ext(relativePath))
	}

	systemPrompt, truncated := truncateStringBytes(strings.TrimSpace(body), opts.MaxSystemPromptBytes)
	return Definition{
		ID:                    source.kind + ":" + strings.TrimSuffix(filepath.Base(relativePath), filepath.Ext(relativePath)),
		Name:                  truncateMetadata(name, 120),
		Description:           truncateMetadata(metadata.Description, 500),
		Model:                 truncateMetadata(metadata.Model, 160),
		Tools:                 metadata.Tools,
		DisallowedTools:       metadata.DisallowedTools,
		PermissionMode:        truncateMetadata(metadata.PermissionMode, 80),
		SystemPrompt:          systemPrompt,
		SystemPromptTruncated: truncated,
		Source: Source{
			Kind:         source.kind,
			RelativePath: relativePath,
		},
		Warnings: warnings,
	}
}

func truncateMetadata(value string, limit int) string {
	value = strings.TrimSpace(strings.ToValidUTF8(value, ""))
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:safeUTF8Cut(value, limit)]
}
