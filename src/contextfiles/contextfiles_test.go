package contextfiles

import (
	"testing"
	"time"
)

func TestFormatEmptyOutput(t *testing.T) {
	if got := Format(nil, "/work/project", Options{}); got != "No files in context" {
		t.Fatalf("expected empty context message, got %q", got)
	}
}

func TestFormatRelativeList(t *testing.T) {
	files := []FileEntry{
		{Path: "src/z.go"},
		{Path: "README.md"},
	}

	got := Format(files, "/work/project", Options{})
	want := "Files in context:\nREADME.md\nsrc/z.go"
	if got != want {
		t.Fatalf("unexpected output:\nwant:\n%s\ngot:\n%s", want, got)
	}
}

func TestRelativePathAbsoluteUnderCWDBecomesRelative(t *testing.T) {
	got := RelativePath("/work/project", "/work/project/src/main.go")
	if got != "src/main.go" {
		t.Fatalf("expected relative path, got %q", got)
	}
}

func TestRelativePathOutsideCWDMarked(t *testing.T) {
	tests := []struct {
		name string
		cwd  string
		file string
		want string
	}{
		{
			name: "absolute outside",
			cwd:  "/work/project",
			file: "/work/other/file.go",
			want: "[outside cwd] /work/other/file.go",
		},
		{
			name: "relative parent escape",
			cwd:  "/work/project",
			file: "../secret.txt",
			want: "[outside cwd] ../secret.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RelativePath(tt.cwd, tt.file); got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestSortAndDedupeStable(t *testing.T) {
	files := []FileEntry{
		{Path: "src/z.go"},
		{Path: "./README.md", Source: "first"},
		{Path: "README.md", Source: "duplicate"},
		{Path: `C:\Repo\App\Main.go`, Source: "windows first"},
		{Path: "c:/repo/app/main.go", Source: "windows duplicate"},
		{Path: "."},
		{Path: ""},
	}

	got := SortAndDedupe(files)
	wantPaths := []string{"README.md", "C:/Repo/App/Main.go", "src/z.go"}
	if len(got) != len(wantPaths) {
		t.Fatalf("expected %d files, got %d: %#v", len(wantPaths), len(got), got)
	}
	for i, want := range wantPaths {
		if got[i].Path != want {
			t.Fatalf("path %d: expected %q, got %q", i, want, got[i].Path)
		}
	}
	if got[0].Source != "first" {
		t.Fatalf("expected first duplicate metadata to win, got %q", got[0].Source)
	}
	if got[1].Source != "windows first" {
		t.Fatalf("expected first windows duplicate metadata to win, got %q", got[1].Source)
	}
}

func TestFormatVerboseMetadata(t *testing.T) {
	lastRead := time.Date(2026, 6, 28, 12, 30, 0, 0, time.FixedZone("CST", 8*60*60))
	files := []FileEntry{{
		Path:         "src/main.go",
		Source:       "read_file",
		Tokens:       128,
		LastReadTime: lastRead,
	}}

	plain := Format(files, "/work/project", Options{})
	if plain != "Files in context:\nsrc/main.go" {
		t.Fatalf("default output should stay compact, got %q", plain)
	}

	verbose := Format(files, "/work/project", Options{Verbose: true})
	want := "Files in context:\nsrc/main.go (source: read_file, tokens: 128, last read: 2026-06-28T04:30:00Z)"
	if verbose != want {
		t.Fatalf("unexpected verbose output:\nwant:\n%s\ngot:\n%s", want, verbose)
	}
}

func TestPathCleaning(t *testing.T) {
	files := []FileEntry{
		{Path: "./src//../README.md"},
		{Path: "."},
		{Path: "   "},
	}

	got := Format(files, "/work//project/.", Options{})
	want := "Files in context:\nREADME.md"
	if got != want {
		t.Fatalf("unexpected cleaned output:\nwant:\n%s\ngot:\n%s", want, got)
	}
}

func TestWindowsPathCase(t *testing.T) {
	got := RelativePath(`C:\Users\Nolan\Awaken`, `c:\users\nolan\awaken\Src\Main.go`)
	if got != "Src/Main.go" {
		t.Fatalf("expected windows path under cwd to be relative, got %q", got)
	}

	outside := RelativePath(`C:\Users\Nolan\Awaken`, `D:\Other\File.go`)
	if outside != "[outside cwd] D:/Other/File.go" {
		t.Fatalf("expected windows outside marker, got %q", outside)
	}
}
