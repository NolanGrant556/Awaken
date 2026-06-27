package contextrefs

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestResolveQuotedFileWithLineRange(t *testing.T) {
	cwd := t.TempDir()
	writeFile(t, filepath.Join(cwd, "path with space.go"), "line 1\nline 2\nline 3\n")

	message := "Please inspect @file:`path with space.go`:2-3 before answering."
	result, err := Resolve(context.Background(), message, Options{CWD: cwd})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}

	if len(result.References) != 1 {
		t.Fatalf("expected 1 reference, got %d", len(result.References))
	}
	ref := result.References[0]
	if ref.Raw != "@file:`path with space.go`:2-3" {
		t.Fatalf("unexpected raw reference: %q", ref.Raw)
	}
	if ref.Kind != KindFile || ref.Target != "path with space.go" {
		t.Fatalf("unexpected reference: %#v", ref)
	}
	if ref.LineStart != 2 || ref.LineEnd != 3 {
		t.Fatalf("expected line range 2-3, got %d-%d", ref.LineStart, ref.LineEnd)
	}
	if ref.Start != strings.Index(message, ref.Raw) || ref.End != ref.Start+len(ref.Raw) {
		t.Fatalf("unexpected start/end: %#v", ref)
	}
	if len(result.Blocks) != 1 {
		t.Fatalf("expected 1 block, got %d; warnings=%v", len(result.Blocks), result.Warnings)
	}
	if result.Blocks[0].Content != "line 2\nline 3" {
		t.Fatalf("unexpected block content: %q", result.Blocks[0].Content)
	}
	if result.Blocks[0].Language != "go" {
		t.Fatalf("expected go fence language, got %q", result.Blocks[0].Language)
	}
	if !result.Expanded || result.Blocked {
		t.Fatalf("expected expanded and unblocked result: %#v", result)
	}
}

func TestParseTrimsTrailingPunctuation(t *testing.T) {
	cwd := t.TempDir()
	writeFile(t, filepath.Join(cwd, "README.md"), "hello\n")

	message := "Read @file:README.md, then continue."
	result, err := Resolve(context.Background(), message, Options{CWD: cwd})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}

	if len(result.References) != 1 {
		t.Fatalf("expected 1 reference, got %d", len(result.References))
	}
	ref := result.References[0]
	if ref.Raw != "@file:README.md" || ref.Target != "README.md" {
		t.Fatalf("trailing punctuation was not trimmed: %#v", ref)
	}
	if result.StrippedMessage != "Read, then continue." {
		t.Fatalf("unexpected stripped message: %q", result.StrippedMessage)
	}
}

func TestResolveBlocksPathEscape(t *testing.T) {
	parent := t.TempDir()
	cwd := filepath.Join(parent, "workspace")
	if err := os.Mkdir(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(parent, "outside.txt"), "secret\n")

	result, err := Resolve(context.Background(), "attach @file:../outside.txt", Options{CWD: cwd})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}

	if len(result.Blocks) != 0 {
		t.Fatalf("expected no blocks, got %#v", result.Blocks)
	}
	if !containsWarning(result.Warnings, "outside the allowed workspace") {
		t.Fatalf("expected path escape warning, got %v", result.Warnings)
	}
}

func TestResolveBlocksSensitivePath(t *testing.T) {
	cwd := t.TempDir()
	if err := os.Mkdir(filepath.Join(cwd, ".ssh"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(cwd, ".ssh", "id_rsa"), "private key\n")

	result, err := Resolve(context.Background(), "attach @file:.ssh/id_rsa", Options{CWD: cwd})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}

	if len(result.Blocks) != 0 {
		t.Fatalf("expected no blocks, got %#v", result.Blocks)
	}
	if !containsWarning(result.Warnings, "sensitive credential path") {
		t.Fatalf("expected sensitive path warning, got %v", result.Warnings)
	}
}

func TestResolveRejectsBinaryFile(t *testing.T) {
	cwd := t.TempDir()
	if err := os.WriteFile(filepath.Join(cwd, "binary.dat"), []byte{0, 1, 2, 3}, 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := Resolve(context.Background(), "attach @file:binary.dat", Options{CWD: cwd})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}

	if len(result.Blocks) != 0 {
		t.Fatalf("expected no blocks, got %#v", result.Blocks)
	}
	if !containsWarning(result.Warnings, "binary files are not supported") {
		t.Fatalf("expected binary warning, got %v", result.Warnings)
	}
}

func TestResolveFolderListingHonorsLimit(t *testing.T) {
	cwd := t.TempDir()
	dir := filepath.Join(cwd, "docs")
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "a.txt"), "a\n")
	writeFile(t, filepath.Join(dir, "sub", "b.txt"), "b\n")
	writeFile(t, filepath.Join(dir, "z.txt"), "z\n")

	result, err := Resolve(context.Background(), "list @folder:docs", Options{CWD: cwd, FolderEntryLimit: 2})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if len(result.Blocks) != 1 {
		t.Fatalf("expected 1 block, got %d; warnings=%v", len(result.Blocks), result.Warnings)
	}

	content := result.Blocks[0].Content
	for _, want := range []string{"docs/", "- a.txt (1 lines)", "- sub/", "- ..."} {
		if !strings.Contains(content, want) {
			t.Fatalf("expected folder listing to contain %q, got:\n%s", want, content)
		}
	}
	if strings.Contains(content, "b.txt") {
		t.Fatalf("expected listing to stop at limit, got:\n%s", content)
	}
}

func TestResolveGitAndURLUseInjectedStubs(t *testing.T) {
	cwd := t.TempDir()
	git := &recordingGitRunner{}
	var fetchedURL string
	fetcher := URLFetcherFunc(func(ctx context.Context, rawURL string) (string, error) {
		fetchedURL = rawURL
		return "# fetched\nbody", nil
	})

	message := "refs @diff @staged @git:2 @url:https://example.test/doc."
	result, err := Resolve(context.Background(), message, Options{
		CWD:        cwd,
		GitRunner:  git,
		URLFetcher: fetcher,
	})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}

	wantCalls := [][]string{
		{"diff"},
		{"diff", "--staged"},
		{"log", "-2", "-p"},
	}
	if !reflect.DeepEqual(git.calls, wantCalls) {
		t.Fatalf("unexpected git calls: %#v", git.calls)
	}
	if fetchedURL != "https://example.test/doc" {
		t.Fatalf("unexpected fetched URL: %q", fetchedURL)
	}
	if len(result.Blocks) != 4 {
		t.Fatalf("expected 4 blocks, got %d; warnings=%v", len(result.Blocks), result.Warnings)
	}
	if result.References[3].Raw != "@url:https://example.test/doc" {
		t.Fatalf("URL raw reference kept trailing punctuation: %#v", result.References[3])
	}
}

func TestResolveHardTokenLimitBlocksInjection(t *testing.T) {
	cwd := t.TempDir()
	writeFile(t, filepath.Join(cwd, "large.txt"), strings.Repeat("abcd", 20))

	result, err := Resolve(context.Background(), "attach @file:large.txt", Options{CWD: cwd, HardTokenLimit: 1})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}

	if !result.Blocked {
		t.Fatalf("expected blocked result: %#v", result)
	}
	if result.Expanded {
		t.Fatalf("blocked result should not be marked expanded: %#v", result)
	}
	if len(result.Blocks) != 0 {
		t.Fatalf("blocked result should not inject blocks: %#v", result.Blocks)
	}
	if result.EstimatedTokens <= 1 {
		t.Fatalf("expected estimated tokens above limit, got %d", result.EstimatedTokens)
	}
	if !containsWarning(result.Warnings, "hard limit") {
		t.Fatalf("expected hard limit warning, got %v", result.Warnings)
	}
}

type recordingGitRunner struct {
	calls [][]string
}

func (r *recordingGitRunner) RunGit(ctx context.Context, cwd string, args ...string) (string, error) {
	r.calls = append(r.calls, append([]string(nil), args...))
	return "output for " + strings.Join(args, " "), nil
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func containsWarning(warnings []string, needle string) bool {
	for _, warning := range warnings {
		if strings.Contains(warning, needle) {
			return true
		}
	}
	return false
}
