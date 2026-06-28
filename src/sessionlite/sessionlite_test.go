package sessionlite

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadHeadTail(t *testing.T) {
	tests := []struct {
		name    string
		content []byte
		limit   int
		want    HeadTail
	}{
		{
			name:    "small content uses same tail",
			content: []byte("hello"),
			limit:   10,
			want:    HeadTail{Head: "hello", Tail: "hello", Size: 5},
		},
		{
			name:    "large content uses head and tail",
			content: []byte("0123456789"),
			limit:   4,
			want:    HeadTail{Head: "0123", Tail: "6789", Size: 10},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ReadHeadTail(tt.content, tt.limit)
			if got != tt.want {
				t.Fatalf("ReadHeadTail() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestReadHeadTailUsesDefaultLimit(t *testing.T) {
	content := append(bytes.Repeat([]byte("a"), LiteReadBufSize), 'z')

	got := ReadHeadTail(content, 0)
	if got.Size != LiteReadBufSize+1 {
		t.Fatalf("Size = %d, want %d", got.Size, LiteReadBufSize+1)
	}
	if len(got.Head) != LiteReadBufSize {
		t.Fatalf("head length = %d, want %d", len(got.Head), LiteReadBufSize)
	}
	if len(got.Tail) != LiteReadBufSize {
		t.Fatalf("tail length = %d, want %d", len(got.Tail), LiteReadBufSize)
	}
	if !strings.HasSuffix(got.Tail, "z") {
		t.Fatalf("tail should include final byte, got suffix %q", got.Tail[len(got.Tail)-1:])
	}
}

func TestExtractJSONStringField(t *testing.T) {
	tests := []struct {
		name string
		text string
		key  string
		want string
		ok   bool
	}{
		{
			name: "first field",
			text: `{"title":"first","title":"second"}`,
			key:  "title",
			want: "first",
			ok:   true,
		},
		{
			name: "escaped quote and backslash",
			text: `{"title":"say \"hi\" on C:\\tmp"}`,
			key:  "title",
			want: `say "hi" on C:\tmp`,
			ok:   true,
		},
		{
			name: "whitespace around colon",
			text: "{\n  \"title\" : \"spaced\"\n}",
			key:  "title",
			want: "spaced",
			ok:   true,
		},
		{
			name: "broken string",
			text: `{"title":"unterminated}`,
			key:  "title",
			ok:   false,
		},
		{
			name: "non string value",
			text: `{"title":123}`,
			key:  "title",
			ok:   false,
		},
		{
			name: "missing field",
			text: `{"other":"value"}`,
			key:  "title",
			ok:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ExtractJSONStringField(tt.text, tt.key)
			if ok != tt.ok || got != tt.want {
				t.Fatalf("ExtractJSONStringField() = %q, %v; want %q, %v", got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestExtractLastJSONStringField(t *testing.T) {
	got, ok := ExtractLastJSONStringField(`{"title":"first"}{"title": "second"}`, "title")
	if !ok || got != "second" {
		t.Fatalf("ExtractLastJSONStringField() = %q, %v; want second, true", got, ok)
	}
}

func TestSanitizePath(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "special characters",
			in:   "/Users/me/my project:1",
			want: "-Users-me-my-project-1",
		},
		{
			name: "keeps alphanumeric",
			in:   "abcXYZ123",
			want: "abcXYZ123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SanitizePath(tt.in); got != tt.want {
				t.Fatalf("SanitizePath(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestSanitizePathLongDeterministic(t *testing.T) {
	in := strings.Repeat("a", MaxSanitizedLength+20) + ":tail"

	first := SanitizePath(in)
	second := SanitizePath(in)
	if first != second {
		t.Fatalf("SanitizePath should be deterministic: %q != %q", first, second)
	}
	if !strings.HasPrefix(first, strings.Repeat("a", MaxSanitizedLength)+"-") {
		t.Fatalf("long sanitized path should keep truncated prefix and hash suffix, got %q", first)
	}
	if len(first) <= MaxSanitizedLength {
		t.Fatalf("long sanitized path should include hash suffix, got %q", first)
	}
}

func TestProjectDir(t *testing.T) {
	root := filepath.Join("home", "me", "projects")

	got := ProjectDir(root, "/Users/me//repo/../repo2/")
	want := filepath.Join(root, "-Users-me-repo2")
	if got != want {
		t.Fatalf("ProjectDir() = %q, want %q", got, want)
	}
}

func TestResolveSessionFileCandidatesOrderAndDedupe(t *testing.T) {
	root := filepath.Join("home", "me", "projects")

	got := ResolveSessionFileCandidates("session-1", "/Users/me/repo", []string{
		"/Users/me/repo",
		"/Users/me/repo-wt",
		"/Users/me/repo wt",
		"/Users/me/other//",
	}, root)

	want := []Candidate{
		{
			ProjectPath: "/Users/me/repo",
			ProjectDir:  filepath.Join(root, "-Users-me-repo"),
			FilePath:    filepath.Join(root, "-Users-me-repo", "session-1.jsonl"),
		},
		{
			ProjectPath: "/Users/me/repo-wt",
			ProjectDir:  filepath.Join(root, "-Users-me-repo-wt"),
			FilePath:    filepath.Join(root, "-Users-me-repo-wt", "session-1.jsonl"),
		},
		{
			ProjectPath: "/Users/me/other",
			ProjectDir:  filepath.Join(root, "-Users-me-other"),
			FilePath:    filepath.Join(root, "-Users-me-other", "session-1.jsonl"),
		},
	}

	if len(got) != len(want) {
		t.Fatalf("candidate count = %d, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("candidate %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}
