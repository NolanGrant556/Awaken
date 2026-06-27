package outputstyle

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadStyleFileParsesFrontmatterAndBody(t *testing.T) {
	path := writeStyleFile(t, "mentor.md", `---
name: mentor
description: Explains the reasoning
keep_coding_instructions: true
force_for_plugin: true
source: plugin
---

Explain the why.
Keep it practical.
`)

	style, err := LoadStyleFile(path)
	if err != nil {
		t.Fatalf("expected style file to load, got %v", err)
	}

	if style.Name != "mentor" {
		t.Fatalf("expected name from frontmatter, got %q", style.Name)
	}
	if style.Description != "Explains the reasoning" {
		t.Fatalf("expected description from frontmatter, got %q", style.Description)
	}
	if style.Source != SourcePlugin {
		t.Fatalf("expected plugin source, got %q", style.Source)
	}
	if !style.KeepCodingInstructions {
		t.Fatal("expected keep coding instructions to parse true")
	}
	if !style.ForceForPlugin {
		t.Fatal("expected force for plugin to parse true")
	}
	if style.Prompt != "Explain the why.\nKeep it practical." {
		t.Fatalf("expected trimmed body prompt, got %q", style.Prompt)
	}
}

func TestLoadStyleFileUsesFilenameFallbackAndDefaultSource(t *testing.T) {
	path := writeStyleFile(t, "quiet.md", "Keep the answer quiet and direct.\n")

	style, err := LoadStyleFile(path)
	if err != nil {
		t.Fatalf("expected style file to load, got %v", err)
	}
	if style.Name != "quiet" {
		t.Fatalf("expected filename fallback name, got %q", style.Name)
	}
	if style.Source != SourceUser {
		t.Fatalf("expected user source fallback, got %q", style.Source)
	}
	if style.KeepCodingInstructions {
		t.Fatal("expected absent keep_coding_instructions to remain false")
	}
}

func TestLoadStyleFileRejectsInvalidAndEmptyStyle(t *testing.T) {
	emptyPath := writeStyleFile(t, "empty.md", "  \n")
	if _, err := LoadStyleFile(emptyPath); !errors.Is(err, ErrInvalidStyle) {
		t.Fatalf("expected ErrInvalidStyle for empty file, got %v", err)
	}

	badFrontmatterPath := writeStyleFile(t, "bad.md", `---
name: bad
keep_coding_instructions: maybe
---
prompt
`)
	if _, err := LoadStyleFile(badFrontmatterPath); !errors.Is(err, ErrInvalidStyle) {
		t.Fatalf("expected ErrInvalidStyle for invalid frontmatter bool, got %v", err)
	}

	blankPromptPath := writeStyleFile(t, "blank.md", `---
name: blank
source: user
---
`)
	if _, err := LoadStyleFile(blankPromptPath); !errors.Is(err, ErrInvalidStyle) {
		t.Fatalf("expected ErrInvalidStyle for missing prompt, got %v", err)
	}
}

func TestLoadStyleFileSupportsQuotedScalarsAndKebabKeys(t *testing.T) {
	path := writeStyleFile(t, "quoted.md", `---
name: "quoted"
description: 'It''s quoted'
keep-coding-instructions: false
force-for-plugin: false
source: "project"
---
Body
`)

	style, err := LoadStyleFile(path)
	if err != nil {
		t.Fatalf("expected quoted style file to load, got %v", err)
	}
	if style.Name != "quoted" || style.Description != "It's quoted" || style.Source != SourceProject {
		t.Fatalf("unexpected parsed style: %+v", style)
	}
	if style.KeepCodingInstructions || style.ForceForPlugin {
		t.Fatalf("expected false bool fields, got %+v", style)
	}
}

func TestLoadStyleFileRejectsUnsupportedFrontmatterKey(t *testing.T) {
	path := writeStyleFile(t, "bad-key.md", `---
name: bad
extra: value
---
prompt
`)

	_, err := LoadStyleFile(path)
	if err == nil || !strings.Contains(err.Error(), "unsupported frontmatter key") {
		t.Fatalf("expected unsupported key error, got %v", err)
	}
}

func writeStyleFile(t *testing.T, name string, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write style file: %v", err)
	}
	return path
}
