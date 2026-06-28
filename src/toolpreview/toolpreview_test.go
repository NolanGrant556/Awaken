package toolpreview

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestVerboseZeroDoesNotTruncate(t *testing.T) {
	preview := strings.Repeat("a", 80)

	result := Format("shell", preview, Policy{Mode: ModeVerbose})

	if result.Truncated {
		t.Fatalf("expected verbose zero length not to truncate: %+v", result)
	}
	if result.Preview != preview || result.Text != "shell: "+preview {
		t.Fatalf("unexpected preview: %+v", result)
	}
}

func TestVerbosePositiveTruncates(t *testing.T) {
	result := Format("read", "abcdefghijklmnopqrstuvwxyz", Policy{Mode: ModeVerbose, PreviewLength: 10})

	if !result.Truncated {
		t.Fatalf("expected truncated preview")
	}
	if result.Preview != "abcdefg..." {
		t.Fatalf("unexpected preview %q", result.Preview)
	}
	if result.Text != "read: abcdefg..." {
		t.Fatalf("unexpected text %q", result.Text)
	}
}

func TestAllAndNewZeroUseDefaultShortPreview(t *testing.T) {
	for _, mode := range []Mode{ModeAll, ModeNew} {
		t.Run(string(mode), func(t *testing.T) {
			result := Format("search", strings.Repeat("x", 60), Policy{Mode: mode})

			if !result.Truncated {
				t.Fatalf("expected truncated preview")
			}
			if got := utf8.RuneCountInString(result.Preview); got != DefaultShortLength {
				t.Fatalf("expected default length %d, got %d in %q", DefaultShortLength, got, result.Preview)
			}
			if !strings.HasSuffix(result.Preview, ellipsis) {
				t.Fatalf("expected ellipsis suffix, got %q", result.Preview)
			}
		})
	}
}

func TestResolveLengthUsesPlatformOverride(t *testing.T) {
	global := 100
	platform := 12

	resolution := ResolveLength(&global, &platform, ModeVerbose)

	if resolution.Length != 12 || resolution.ConfiguredLength != 12 {
		t.Fatalf("expected platform override length, got %+v", resolution)
	}
	if resolution.Source != LengthSourcePlatformOverride {
		t.Fatalf("expected platform source, got %+v", resolution)
	}
}

func TestNegativeLengthNormalizesToZeroOrDefault(t *testing.T) {
	negative := -1

	verbose := ResolveLength(&negative, nil, ModeVerbose)
	if verbose.Length != 0 || !verbose.Unlimited {
		t.Fatalf("expected negative verbose length to become unlimited zero, got %+v", verbose)
	}

	short := ResolveLength(&negative, nil, ModeNew)
	if short.Length != DefaultShortLength || !short.Defaulted {
		t.Fatalf("expected negative new length to use default, got %+v", short)
	}
}

func TestShortCapTruncatesRobustly(t *testing.T) {
	for _, tc := range []struct {
		name string
		cap  int
		want string
	}{
		{name: "one", cap: 1, want: "."},
		{name: "two", cap: 2, want: ".."},
		{name: "three", cap: 3, want: "..."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := Format("shell", "abcdef", Policy{Mode: ModeVerbose, PreviewLength: tc.cap})

			if !result.Truncated {
				t.Fatalf("expected truncated preview")
			}
			if result.Preview != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, result.Preview)
			}
		})
	}
}

func TestUnicodeTruncationIsRuneSafe(t *testing.T) {
	result := Format("grep", "你好世界abcd", Policy{Mode: ModeVerbose, PreviewLength: 5})

	if !result.Truncated {
		t.Fatalf("expected truncated preview")
	}
	if result.Preview != "你好..." {
		t.Fatalf("unexpected unicode preview %q", result.Preview)
	}
	if got := utf8.RuneCountInString(result.Preview); got != 5 {
		t.Fatalf("expected 5 runes, got %d", got)
	}
}

func TestEmptyPreviewRendersToolNameOnly(t *testing.T) {
	result := Format("read", "", Policy{Mode: ModeAll})

	if result.Truncated || result.Preview != "" || result.Text != "read" || result.OriginalRunes != 0 {
		t.Fatalf("unexpected empty preview result: %+v", result)
	}
}

func TestOffModeSuppressesPreview(t *testing.T) {
	result := Format("shell", "hidden", Policy{Mode: ModeOff, PreviewLength: 10})

	if result.Truncated || result.Preview != "" || result.Text != "shell" || result.OriginalRunes != 6 {
		t.Fatalf("unexpected off preview result: %+v", result)
	}
}
