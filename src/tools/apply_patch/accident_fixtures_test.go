package applypatch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAccidentFixturesReproducePatchFailures(t *testing.T) {
	root := t.TempDir()
	sample := filepath.Join(root, "notes", "sample.txt")
	if err := os.MkdirAll(filepath.Dir(sample), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(sample, []byte("hello\nworld\n"), 0o644); err != nil {
		t.Fatalf("seed sample: %v", err)
	}

	cases := []struct {
		name      string
		file      string
		wantSub   string
		rewrite   bool
		seedAlpha bool
	}{
		{
			name:    "missing_begin_patch",
			file:    "missing_begin_patch.txt",
			wantSub: "Begin Patch",
		},
		{
			name:    "hunk_line_missing_prefix",
			file:    "hunk_line_missing_prefix.txt",
			wantSub: "must start with",
			rewrite: true,
		},
		{
			name:    "end_of_file_misuse",
			file:    "end_of_file_misuse.txt",
			wantSub: "End of File",
			rewrite: true,
		},
		{
			name:    "context_mismatch",
			file:    "context_mismatch.patch.txt",
			wantSub: "failed to find expected lines",
			rewrite: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join("testdata", tc.file))
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			patch := string(raw)
			if tc.rewrite {
				patch = strings.ReplaceAll(patch, "notes/sample.txt", sample)
			}
			_, err = (&ApplyPatchTool{}).Execute(t.Context(), patch)
			if err == nil {
				t.Fatalf("expected failure for %s", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("expected error containing %q, got %v", tc.wantSub, err)
			}
		})
	}
}

func TestAccidentDuplicatePatchFixturesAreExactCopies(t *testing.T) {
	a, err := os.ReadFile(filepath.Join("testdata", "duplicate_patch_a.txt"))
	if err != nil {
		t.Fatalf("read a: %v", err)
	}
	b, err := os.ReadFile(filepath.Join("testdata", "duplicate_patch_b.txt"))
	if err != nil {
		t.Fatalf("read b: %v", err)
	}
	if string(a) != string(b) {
		t.Fatalf("duplicate fixtures must be exact copies")
	}
	if !strings.Contains(string(a), "*** Begin Patch") || !strings.Contains(string(a), "*** End Patch") {
		t.Fatalf("duplicate fixture missing patch markers")
	}

	root := t.TempDir()
	sample := filepath.Join(root, "notes", "sample.txt")
	if err := os.MkdirAll(filepath.Dir(sample), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(sample, []byte("gamma\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	patch := strings.ReplaceAll(string(a), "notes/sample.txt", sample)
	_, err1 := (&ApplyPatchTool{}).Execute(t.Context(), patch)
	_, err2 := (&ApplyPatchTool{}).Execute(t.Context(), patch)
	if err1 == nil || err2 == nil {
		t.Fatalf("expected both duplicate submissions to fail against mismatched context, got %v / %v", err1, err2)
	}
	if err1.Error() != err2.Error() {
		t.Fatalf("exact duplicate payloads should produce identical failures:\n1: %v\n2: %v", err1, err2)
	}
}
