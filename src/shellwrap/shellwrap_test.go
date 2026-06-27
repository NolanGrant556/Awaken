package shellwrap

import (
	"reflect"
	"strings"
	"testing"
)

func TestStripReturnsUnchangedCopyForNonWrapper(t *testing.T) {
	input := []string{"git", "status"}
	result := Strip(input)
	input[0] = "rm"

	assertArgv(t, result, []string{"git", "status"})
	if result.Changed {
		t.Fatalf("expected Changed=false, got %+v", result)
	}
	if result.UnchangedReason == "" {
		t.Fatalf("expected UnchangedReason, got %+v", result)
	}
}

func TestStripTimeout(t *testing.T) {
	cases := []struct {
		name string
		argv []string
		want []string
	}{
		{
			name: "duration only",
			argv: []string{"timeout", "10", "git", "status"},
			want: []string{"git", "status"},
		},
		{
			name: "safe flags",
			argv: []string{"timeout", "--foreground", "-s", "TERM", "-k", "1.5s", "10.5s", "git", "status"},
			want: []string{"git", "status"},
		},
		{
			name: "double dash before duration",
			argv: []string{"timeout", "--", "5m", "git", "status"},
			want: []string{"git", "status"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := Strip(tc.argv)
			assertChangedArgv(t, result, tc.want)
			assertStrippedNames(t, result, []string{"timeout"})
		})
	}
}

func TestStripTimeoutFailsClosed(t *testing.T) {
	cases := []struct {
		name string
		argv []string
	}{
		{
			name: "unsupported flag",
			argv: []string{"timeout", "--shell=/bin/sh", "10", "rm", "-rf", "/"},
		},
		{
			name: "unsafe flag value",
			argv: []string{"timeout", "--signal=$(rm)", "10", "git", "status"},
		},
		{
			name: "leading decimal duration",
			argv: []string{"timeout", ".5", "git", "status"},
		},
		{
			name: "signed duration",
			argv: []string{"timeout", "+5", "git", "status"},
		},
		{
			name: "infinite duration",
			argv: []string{"timeout", "inf", "git", "status"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := Strip(tc.argv)
			assertBlockedOriginal(t, result, tc.argv)
		})
	}
}

func TestStripNiceForms(t *testing.T) {
	cases := []struct {
		name string
		argv []string
		want []string
	}{
		{
			name: "bare",
			argv: []string{"nice", "git", "status"},
			want: []string{"git", "status"},
		},
		{
			name: "-n value",
			argv: []string{"nice", "-n", "10", "git", "status"},
			want: []string{"git", "status"},
		},
		{
			name: "-N compact",
			argv: []string{"nice", "-10", "git", "status"},
			want: []string{"git", "status"},
		},
		{
			name: "double dash",
			argv: []string{"nice", "--", "git", "status"},
			want: []string{"git", "status"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := Strip(tc.argv)
			assertChangedArgv(t, result, tc.want)
			assertStrippedNames(t, result, []string{"nice"})
		})
	}
}

func TestStripEnvFlagsAndAssignments(t *testing.T) {
	cases := []struct {
		name string
		argv []string
		want []string
	}{
		{
			name: "short flags and vars",
			argv: []string{"env", "-i", "-u", "OLD", "FOO=bar", "BAR=baz", "git", "status"},
			want: []string{"git", "status"},
		},
		{
			name: "long flags and var",
			argv: []string{"env", "--ignore-environment", "--unset=OLD", "FOO=bar", "git", "status"},
			want: []string{"git", "status"},
		},
		{
			name: "double dash before var",
			argv: []string{"env", "--", "FOO=bar", "git", "status"},
			want: []string{"git", "status"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := Strip(tc.argv)
			assertChangedArgv(t, result, tc.want)
			assertStrippedNames(t, result, []string{"env"})
		})
	}
}

func TestStripEnvFailsClosedForUnsupportedFlag(t *testing.T) {
	input := []string{"env", "-S", "rm -rf /", "git", "status"}
	result := Strip(input)
	assertBlockedOriginal(t, result, input)
}

func TestStripStdbuf(t *testing.T) {
	cases := []struct {
		name string
		argv []string
		want []string
	}{
		{
			name: "compact modes",
			argv: []string{"stdbuf", "-oL", "-e0", "git", "status"},
			want: []string{"git", "status"},
		},
		{
			name: "long modes and double dash",
			argv: []string{"stdbuf", "--output=L", "--error=0", "--", "git", "status"},
			want: []string{"git", "status"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := Strip(tc.argv)
			assertChangedArgv(t, result, tc.want)
			assertStrippedNames(t, result, []string{"stdbuf"})
		})
	}
}

func TestStripNestedWrappers(t *testing.T) {
	input := []string{
		"timeout", "10",
		"nice", "-n", "5",
		"env", "-i", "FOO=bar",
		"nohup",
		"time", "-p",
		"stdbuf", "-oL",
		"git", "status",
	}
	result := Strip(input)

	assertChangedArgv(t, result, []string{"git", "status"})
	assertStrippedNames(t, result, []string{"timeout", "nice", "env", "nohup", "time", "stdbuf"})
}

func TestStripNestedWrapperFailureReturnsOriginal(t *testing.T) {
	input := []string{"timeout", "10", "env", "-S", "rm -rf /", "git", "status"}
	result := Strip(input)

	assertBlockedOriginal(t, result, input)
}

func TestStripNohupAndTimeDoubleDash(t *testing.T) {
	cases := []struct {
		name     string
		argv     []string
		want     []string
		wrappers []string
	}{
		{
			name:     "nohup",
			argv:     []string{"nohup", "--", "git", "status"},
			want:     []string{"git", "status"},
			wrappers: []string{"nohup"},
		},
		{
			name:     "nohup command starting with dash",
			argv:     []string{"nohup", "--", "-cmd"},
			want:     []string{"-cmd"},
			wrappers: []string{"nohup"},
		},
		{
			name:     "time",
			argv:     []string{"time", "-p", "--", "git", "status"},
			want:     []string{"git", "status"},
			wrappers: []string{"time"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := Strip(tc.argv)
			assertChangedArgv(t, result, tc.want)
			assertStrippedNames(t, result, tc.wrappers)
		})
	}
}

func TestStripMakesDefensiveCopies(t *testing.T) {
	input := []string{"timeout", "10", "git", "status"}
	original := append([]string(nil), input...)

	result := Strip(input)
	if !reflect.DeepEqual(input, original) {
		t.Fatalf("Strip mutated input: got %v want %v", input, original)
	}

	input[2] = "rm"
	assertArgv(t, result, []string{"git", "status"})
	if len(result.Stripped) != 1 || !reflect.DeepEqual(result.Stripped[0].Args, []string{"10"}) {
		t.Fatalf("unexpected stripped wrappers: %+v", result.Stripped)
	}
}

func TestNormalizeAliasesStrip(t *testing.T) {
	result := Normalize([]string{"nice", "git", "status"})
	assertChangedArgv(t, result, []string{"git", "status"})
	assertStrippedNames(t, result, []string{"nice"})
}

func assertChangedArgv(t *testing.T, result Result, want []string) {
	t.Helper()
	assertArgv(t, result, want)
	if !result.Changed {
		t.Fatalf("expected Changed=true, got %+v", result)
	}
	if result.BlockedReason != "" || result.ParseError != "" {
		t.Fatalf("expected no parse error, got %+v", result)
	}
}

func assertArgv(t *testing.T, result Result, want []string) {
	t.Helper()
	if !reflect.DeepEqual(result.Argv, want) {
		t.Fatalf("unexpected argv: got %v want %v", result.Argv, want)
	}
}

func assertStrippedNames(t *testing.T, result Result, want []string) {
	t.Helper()
	var got []string
	for _, wrapper := range result.Stripped {
		got = append(got, wrapper.Name)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected stripped wrappers: got %v want %v; result=%+v", got, want, result)
	}
}

func assertBlockedOriginal(t *testing.T, result Result, original []string) {
	t.Helper()
	assertArgv(t, result, original)
	if result.Changed {
		t.Fatalf("expected Changed=false for blocked parse, got %+v", result)
	}
	if len(result.Stripped) != 0 {
		t.Fatalf("expected no stripped wrappers on blocked parse, got %+v", result.Stripped)
	}
	if strings.TrimSpace(result.BlockedReason) == "" || strings.TrimSpace(result.ParseError) == "" {
		t.Fatalf("expected blocked parse reason, got %+v", result)
	}
}
