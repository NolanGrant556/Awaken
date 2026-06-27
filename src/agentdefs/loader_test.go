package agentdefs

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseMarkdownFrontmatter(t *testing.T) {
	meta, body, warnings := parseMarkdown(`---
name: reviewer
description: "Reviews code"
model: claude-sonnet
tools:
  - read_file
  - grep_search
disallowed_tools: [shell, apply_patch]
permission_mode: plan
unknown: ignored
---
Review the code carefully.
`)

	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
	if meta.Name != "reviewer" {
		t.Fatalf("expected name reviewer, got %q", meta.Name)
	}
	if meta.Description != "Reviews code" {
		t.Fatalf("expected quoted description to be unquoted, got %q", meta.Description)
	}
	if meta.Model != "claude-sonnet" {
		t.Fatalf("expected model claude-sonnet, got %q", meta.Model)
	}
	if !reflect.DeepEqual(meta.Tools, []string{"read_file", "grep_search"}) {
		t.Fatalf("unexpected tools: %v", meta.Tools)
	}
	if !reflect.DeepEqual(meta.DisallowedTools, []string{"shell", "apply_patch"}) {
		t.Fatalf("unexpected disallowed tools: %v", meta.DisallowedTools)
	}
	if meta.PermissionMode != "plan" {
		t.Fatalf("expected permission mode plan, got %q", meta.PermissionMode)
	}
	if strings.TrimSpace(body) != "Review the code carefully." {
		t.Fatalf("unexpected body: %q", body)
	}
}

func TestLoadDiscoversAwakenAndClaudeAgentDirs(t *testing.T) {
	root := t.TempDir()
	writeAgent(t, root, ".awaken/agents/architect.md", `---
name: architect
description: Designs changes
tools: read_file, grep_search
---
Think in systems.
`)
	writeAgent(t, root, ".claude/agents/reviewer.md", `---
name: reviewer
---
Review with care.
`)

	result, err := Load(root)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(result.Definitions) != 2 {
		t.Fatalf("expected 2 definitions, got %d: %+v", len(result.Definitions), result.Definitions)
	}
	defsByID := map[string]Definition{}
	for _, def := range result.Definitions {
		defsByID[def.ID] = def
	}
	if defsByID["awaken:architect"].Name != "architect" {
		t.Fatalf("expected awaken architect definition, got %+v", defsByID)
	}
	if defsByID["claude_compat:reviewer"].Source.RelativePath != ".claude/agents/reviewer.md" {
		t.Fatalf("expected claude compatibility source path, got %+v", defsByID["claude_compat:reviewer"].Source)
	}
}

func TestLoadSkipsSymlinkEscapingWorkspace(t *testing.T) {
	root := t.TempDir()
	agentDir := filepath.Join(root, ".awaken", "agents")
	if err := os.MkdirAll(agentDir, 0755); err != nil {
		t.Fatalf("failed to create agent dir: %v", err)
	}
	outside := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte("secret outside workspace"), 0644); err != nil {
		t.Fatalf("failed to write outside file: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(agentDir, "outside.md")); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	result, err := Load(root)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if len(result.Definitions) != 0 {
		t.Fatalf("expected no definitions, got %+v", result.Definitions)
	}
	if len(result.Skipped) != 1 || result.Skipped[0].Reason != "outside_workspace" {
		t.Fatalf("expected outside_workspace skip, got %+v", result.Skipped)
	}
}

func TestLoadSkipsAgentDirectorySymlinkEscapingWorkspace(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".awaken"), 0755); err != nil {
		t.Fatalf("failed to create awaken dir: %v", err)
	}
	outsideDir := filepath.Join(t.TempDir(), "agents")
	if err := os.MkdirAll(outsideDir, 0755); err != nil {
		t.Fatalf("failed to create outside dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outsideDir, "reviewer.md"), []byte("secret outside workspace"), 0644); err != nil {
		t.Fatalf("failed to write outside agent: %v", err)
	}
	if err := os.Symlink(outsideDir, filepath.Join(root, ".awaken", "agents")); err != nil {
		t.Fatalf("failed to create agent dir symlink: %v", err)
	}

	result, err := Load(root)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if len(result.Definitions) != 0 {
		t.Fatalf("expected no definitions, got %+v", result.Definitions)
	}
	if len(result.Skipped) != 1 || result.Skipped[0].Reason != "outside_workspace" {
		t.Fatalf("expected outside_workspace skip, got %+v", result.Skipped)
	}
}

func TestLoadSkipsOversizedFile(t *testing.T) {
	root := t.TempDir()
	writeAgent(t, root, ".awaken/agents/big.md", strings.Repeat("x", 33))

	result, err := LoadWithOptions(root, Options{MaxFileBytes: 32, MaxSystemPromptBytes: 32})
	if err != nil {
		t.Fatalf("LoadWithOptions failed: %v", err)
	}
	if len(result.Definitions) != 0 {
		t.Fatalf("expected oversized file to be skipped, got %+v", result.Definitions)
	}
	if len(result.Skipped) != 1 || result.Skipped[0].Reason != "file_too_large" {
		t.Fatalf("expected file_too_large skip, got %+v", result.Skipped)
	}
}

func TestLoadKeepsUnclosedFrontmatterAsPromptWithWarning(t *testing.T) {
	root := t.TempDir()
	writeAgent(t, root, ".awaken/agents/reviewer.md", `---
name: reviewer
Review the whole file.
`)

	result, err := Load(root)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if len(result.Definitions) != 1 {
		t.Fatalf("expected one definition, got %+v", result.Definitions)
	}
	def := result.Definitions[0]
	if def.Name != "reviewer" {
		t.Fatalf("expected filename fallback name reviewer, got %q", def.Name)
	}
	if !strings.Contains(def.SystemPrompt, "Review the whole file.") {
		t.Fatalf("expected full markdown to remain prompt, got %q", def.SystemPrompt)
	}
	if !reflect.DeepEqual(def.Warnings, []string{"frontmatter_not_closed"}) {
		t.Fatalf("expected frontmatter warning, got %+v", def.Warnings)
	}
}

func TestLoadSkipsSecretLikeFilenames(t *testing.T) {
	root := t.TempDir()
	writeAgent(t, root, ".awaken/agents/secrets.md", "do not read this")

	result, err := Load(root)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if len(result.Definitions) != 0 {
		t.Fatalf("expected secret-like file to be skipped, got %+v", result.Definitions)
	}
	if len(result.Skipped) != 1 || result.Skipped[0].Reason != "secret_like_path" {
		t.Fatalf("expected secret_like_path skip, got %+v", result.Skipped)
	}
}

func TestLoadRejectsWorkspaceOutsideAbsoluteDirectory(t *testing.T) {
	if _, err := Load("relative/workspace"); !errors.Is(err, ErrInvalidWorkspace) {
		t.Fatalf("expected ErrInvalidWorkspace for relative path, got %v", err)
	}
}

func TestSystemPromptIsTruncated(t *testing.T) {
	root := t.TempDir()
	writeAgent(t, root, ".awaken/agents/short.md", strings.Repeat("a", 20))

	result, err := LoadWithOptions(root, Options{MaxFileBytes: 64, MaxSystemPromptBytes: 8})
	if err != nil {
		t.Fatalf("LoadWithOptions failed: %v", err)
	}
	if len(result.Definitions) != 1 {
		t.Fatalf("expected one definition, got %+v", result.Definitions)
	}
	if result.Definitions[0].SystemPrompt != "aaaaaaaa" || !result.Definitions[0].SystemPromptTruncated {
		t.Fatalf("expected truncated prompt, got %+v", result.Definitions[0])
	}
}

func writeAgent(t *testing.T, root, relativePath, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("failed to create parent dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write agent file: %v", err)
	}
}
