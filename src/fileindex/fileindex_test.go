package fileindex

import "testing"

func TestLoadNormalizesFiltersAndDedupes(t *testing.T) {
	var idx Index
	idx.Load([]string{
		"",
		"src\\main.go",
		"src/main.go",
		"./docs//readme.md",
		"docs/readme.md",
	})

	got := pathsOf(idx.Search("", 10))
	want := []string{"src", "docs"}
	assertPaths(t, got, want)

	got = pathsOf(idx.Search("src/main", 10))
	want = []string{"src/main.go"}
	assertPaths(t, got, want)
}

func TestSearchLimit(t *testing.T) {
	tests := []struct {
		name  string
		query string
		limit int
		want  int
	}{
		{name: "empty query limit", query: "", limit: 2, want: 2},
		{name: "fuzzy query limit", query: "go", limit: 1, want: 1},
		{name: "zero limit", query: "go", limit: 0, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var idx Index
			idx.Load([]string{"a.go", "b.go", "c.go"})

			if got := len(idx.Search(tt.query, tt.limit)); got != tt.want {
				t.Fatalf("len(Search(%q, %d)) = %d, want %d", tt.query, tt.limit, got, tt.want)
			}
		})
	}
}

func TestEmptyQueryReturnsTopLevelEntries(t *testing.T) {
	var idx Index
	idx.Load([]string{
		"long/root.go",
		"a/main.go",
		"aa/main.go",
		"b/main.go",
		"a/other.go",
	})

	got := pathsOf(idx.Search("", 3))
	want := []string{"a", "b", "aa"}
	assertPaths(t, got, want)
}

func TestSearchSmartCase(t *testing.T) {
	var idx Index
	idx.Load([]string{"src/FooBar.go", "src/foobar.go"})

	got := pathsOf(idx.Search("fb", 10))
	want := []string{"src/FooBar.go", "src/foobar.go"}
	assertPaths(t, got, want)

	got = pathsOf(idx.Search("FB", 10))
	want = []string{"src/FooBar.go"}
	assertPaths(t, got, want)
}

func TestSearchRankingBonuses(t *testing.T) {
	tests := []struct {
		name      string
		paths     []string
		query     string
		wantFirst string
	}{
		{
			name:      "boundary beats plain gap",
			paths:     []string{"src/foobaz.go", "src/foo/bar.go"},
			query:     "fb",
			wantFirst: "src/foo/bar.go",
		},
		{
			name:      "camelCase beats plain gap",
			paths:     []string{"src/foobar.go", "src/fooBar.go"},
			query:     "fb",
			wantFirst: "src/fooBar.go",
		},
		{
			name:      "consecutive beats separated match",
			paths:     []string{"src/far/beta.go", "src/fb.go"},
			query:     "fb",
			wantFirst: "src/fb.go",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var idx Index
			idx.Load(tt.paths)

			got := idx.Search(tt.query, 2)
			if len(got) == 0 {
				t.Fatalf("Search(%q) returned no matches", tt.query)
			}
			if got[0].Path != tt.wantFirst {
				t.Fatalf("Search(%q)[0] = %q, want %q", tt.query, got[0].Path, tt.wantFirst)
			}
		})
	}
}

func TestSearchDownranksTestPaths(t *testing.T) {
	var idx Index
	idx.Load([]string{"src/auth_test.go", "src/authentication.go"})

	got := idx.Search("auth", 2)
	assertPaths(t, pathsOf(got), []string{"src/authentication.go", "src/auth_test.go"})
	if got[0].Score != 0 {
		t.Fatalf("best result score = %v, want 0", got[0].Score)
	}
	if got[1].Score <= got[0].Score {
		t.Fatalf("second result score = %v, want greater than first score %v", got[1].Score, got[0].Score)
	}
}

func TestSearchNoMatch(t *testing.T) {
	var idx Index
	idx.Load([]string{"src/main.go", "README.md"})

	if got := idx.Search("zz", 10); len(got) != 0 {
		t.Fatalf("Search returned %d matches, want 0", len(got))
	}
}

func pathsOf(results []Result) []string {
	paths := make([]string, len(results))
	for i, result := range results {
		paths[i] = result.Path
	}
	return paths
}

func assertPaths(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("paths = %#v, want %#v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("paths = %#v, want %#v", got, want)
		}
	}
}
