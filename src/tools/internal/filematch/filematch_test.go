package filematch

import "testing"

func TestMatchGlobstarAllowsZeroOrMoreDirectories(t *testing.T) {
	tests := []struct {
		pattern string
		path    string
		want    bool
	}{
		{pattern: "**/*.go", path: "main.go", want: true},
		{pattern: "**/*.go", path: "src/main.go", want: true},
		{pattern: "src/**/*.go", path: "src/main.go", want: true},
		{pattern: "src/**/*.go", path: "src/tools/main.go", want: true},
		{pattern: "src/**/*.go", path: "docs/main.go", want: false},
	}

	for _, tt := range tests {
		if got := Match(tt.pattern, tt.path); got != tt.want {
			t.Fatalf("Match(%q, %q) = %v, want %v", tt.pattern, tt.path, got, tt.want)
		}
	}
}
