package deeplink

import (
	"net/url"
	"reflect"
	"strings"
	"testing"
)

func TestParseAcceptsDefaultProtocolForms(t *testing.T) {
	tests := []struct {
		name string
		uri  string
		want Action
	}{
		{
			name: "slash form",
			uri:  "awaken-cli://open?q=hello+world&cwd=%2Ftmp%2Fproject&repo=owner%2Frepo",
			want: Action{Query: "hello world", CWD: "/tmp/project", Repo: "owner/repo"},
		},
		{
			name: "colon form",
			uri:  "awaken-cli:open?q=%E4%BD%A0%E5%A5%BD",
			want: Action{Query: "你好"},
		},
		{
			name: "empty query omitted",
			uri:  "awaken-cli://open?q=+++",
			want: Action{},
		},
		{
			name: "query trimmed",
			uri:  "awaken-cli://open?q=++fix+tests++",
			want: Action{Query: "fix tests"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.uri)
			if err != nil {
				t.Fatalf("Parse returned error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("expected %+v, got %+v", tt.want, got)
			}
		})
	}
}

func TestParseWithOptionsUsesCustomProtocol(t *testing.T) {
	got, err := ParseWithOptions("awaken://open?q=hi", Options{Protocol: "awaken", MaxQueryLength: 2})
	if err != nil {
		t.Fatalf("ParseWithOptions returned error: %v", err)
	}
	if got.Query != "hi" {
		t.Fatalf("expected query %q, got %q", "hi", got.Query)
	}

	if _, err := Parse("awaken://open?q=hi"); err == nil {
		t.Fatalf("expected default parser to reject custom protocol")
	}
}

func TestParseRejectsInvalidURI(t *testing.T) {
	invalid := []string{
		"claude-cli://open",
		"awaken-cli://run?q=x",
		"awaken-cli:run?q=x",
		"awaken-cli://open/path?q=x",
		"not a uri",
		"awaken-cli://open?q=%zz",
	}

	for _, uri := range invalid {
		t.Run(uri, func(t *testing.T) {
			if _, err := Parse(uri); err == nil {
				t.Fatalf("expected Parse(%q) to fail", uri)
			}
		})
	}
}

func TestParseRejectsControlChars(t *testing.T) {
	invalid := []string{
		"awaken-cli://open?q=hello%0Aworld",
		"awaken-cli://open?cwd=%2Ftmp%0Dproject",
		"awaken-cli://open?repo=owner%7Frepo",
		"awaken-cli://open\n?q=x",
	}

	for _, uri := range invalid {
		t.Run(uri, func(t *testing.T) {
			if _, err := Parse(uri); err == nil {
				t.Fatalf("expected Parse(%q) to fail", uri)
			}
		})
	}
}

func TestParseSanitizesHiddenUnicodeInQuery(t *testing.T) {
	got, err := Parse("awaken-cli://open?q=a%E2%80%8Bb%E2%80%AE")
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if got.Query != "ab" {
		t.Fatalf("expected sanitized query %q, got %q", "ab", got.Query)
	}
}

func TestParseValidatesCWDAbsolute(t *testing.T) {
	valid := []string{"/tmp/project", "C:/Users/nolan", `D:\work\repo`}
	for _, cwd := range valid {
		t.Run("valid "+cwd, func(t *testing.T) {
			got, err := Parse("awaken-cli://open?cwd=" + url.QueryEscape(cwd))
			if err != nil {
				t.Fatalf("Parse returned error: %v", err)
			}
			if got.CWD != cwd {
				t.Fatalf("expected cwd %q, got %q", cwd, got.CWD)
			}
		})
	}

	invalid := []string{"", "relative/path", "../repo", "C:relative", `\Users\nolan`}
	for _, cwd := range invalid {
		t.Run("invalid "+cwd, func(t *testing.T) {
			if _, err := Parse("awaken-cli://open?cwd=" + url.QueryEscape(cwd)); err == nil {
				t.Fatalf("expected cwd %q to fail", cwd)
			}
		})
	}
}

func TestParseValidatesRepoSlug(t *testing.T) {
	valid := []string{"owner/repo", "owner.name/repo_name-1"}
	for _, repo := range valid {
		t.Run("valid "+repo, func(t *testing.T) {
			got, err := Parse("awaken-cli://open?repo=" + url.QueryEscape(repo))
			if err != nil {
				t.Fatalf("Parse returned error: %v", err)
			}
			if got.Repo != repo {
				t.Fatalf("expected repo %q, got %q", repo, got.Repo)
			}
		})
	}

	invalid := []string{
		"owner/../repo",
		"../repo",
		"owner/..",
		"owner/repo/extra",
		"owner repo/name",
		"owner/",
		`owner\repo`,
	}
	for _, repo := range invalid {
		t.Run("invalid "+repo, func(t *testing.T) {
			if _, err := Parse("awaken-cli://open?repo=" + url.QueryEscape(repo)); err == nil {
				t.Fatalf("expected repo %q to fail", repo)
			}
		})
	}
}

func TestParseRejectsLengthOverflow(t *testing.T) {
	if _, err := Parse("awaken-cli://open?q=" + strings.Repeat("a", DefaultMaxQueryLength+1)); err == nil {
		t.Fatalf("expected overlong query to fail")
	}

	longCWD := "/" + strings.Repeat("a", DefaultMaxCWDLength)
	if _, err := Parse("awaken-cli://open?cwd=" + url.QueryEscape(longCWD)); err == nil {
		t.Fatalf("expected overlong cwd to fail")
	}

	if _, err := ParseWithOptions("awaken-cli://open?q=abcd", Options{MaxQueryLength: 3}); err == nil {
		t.Fatalf("expected custom query length limit to fail")
	}
}

func TestBuildRoundTrip(t *testing.T) {
	uri := Build(Action{
		Query: " fix tests ",
		CWD:   "/tmp/my project",
		Repo:  "owner/repo",
	})

	got, err := Parse(uri)
	if err != nil {
		t.Fatalf("Parse(Build(action)) returned error: %v", err)
	}
	want := Action{Query: "fix tests", CWD: "/tmp/my project", Repo: "owner/repo"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %+v, got %+v from %q", want, got, uri)
	}

	if got := Build(Action{Query: "   "}); got != "awaken-cli://open" {
		t.Fatalf("expected empty query to be omitted, got %q", got)
	}
}

func TestResolveCWD(t *testing.T) {
	existing := map[string]bool{
		"/repos/second": true,
	}
	exists := func(path string) bool {
		return existing[path]
	}

	cwd, resolvedRepo := ResolveCWD(Action{CWD: "/explicit", Repo: "owner/repo"}, []string{"/repos/second"}, exists, "/home/nolan")
	if cwd != "/explicit" || resolvedRepo {
		t.Fatalf("expected explicit cwd without repo resolution, got cwd=%q resolvedRepo=%v", cwd, resolvedRepo)
	}

	cwd, resolvedRepo = ResolveCWD(Action{Repo: "owner/repo"}, []string{"/repos/missing", "/repos/second"}, exists, "/home/nolan")
	if cwd != "/repos/second" || !resolvedRepo {
		t.Fatalf("expected first existing repo path, got cwd=%q resolvedRepo=%v", cwd, resolvedRepo)
	}

	cwd, resolvedRepo = ResolveCWD(Action{Repo: "owner/repo"}, []string{"/repos/missing"}, exists, "/home/nolan")
	if cwd != "/home/nolan" || resolvedRepo {
		t.Fatalf("expected home fallback, got cwd=%q resolvedRepo=%v", cwd, resolvedRepo)
	}
}

func TestTerminalPreferenceFromTermProgram(t *testing.T) {
	tests := []struct {
		name        string
		platform    string
		termProgram string
		wantApp     string
		wantOK      bool
	}{
		{name: "iterm", platform: "darwin", termProgram: "iTerm.app", wantApp: "iTerm", wantOK: true},
		{name: "ghostty", platform: "darwin", termProgram: "Ghostty", wantApp: "Ghostty", wantOK: true},
		{name: "kitty", platform: "darwin", termProgram: "kitty", wantApp: "kitty", wantOK: true},
		{name: "alacritty", platform: "darwin", termProgram: "Alacritty", wantApp: "Alacritty", wantOK: true},
		{name: "wezterm", platform: "darwin", termProgram: "WezTerm", wantApp: "WezTerm", wantOK: true},
		{name: "terminal", platform: "darwin", termProgram: "Apple_Terminal", wantApp: "Terminal", wantOK: true},
		{name: "non mac", platform: "linux", termProgram: "iTerm.app", wantOK: false},
		{name: "unknown", platform: "darwin", termProgram: "unknown", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotApp, gotOK := TerminalPreferenceFromTermProgram(tt.platform, tt.termProgram)
			if gotApp != tt.wantApp || gotOK != tt.wantOK {
				t.Fatalf("expected app=%q ok=%v, got app=%q ok=%v", tt.wantApp, tt.wantOK, gotApp, gotOK)
			}
		})
	}
}
