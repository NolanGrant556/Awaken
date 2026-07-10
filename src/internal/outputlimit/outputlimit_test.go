package outputlimit

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestPrefixTruncatesUnicodeByRuneWithoutSplittingUTF8(t *testing.T) {
	input := strings.Repeat("中", 4) + "🚀tail"
	got, truncated := Prefix(input, 5)
	if !truncated {
		t.Fatalf("expected prefix to report truncation")
	}
	if got != strings.Repeat("中", 4)+"🚀" {
		t.Fatalf("unexpected Unicode prefix %q", got)
	}
	if !utf8.ValidString(got) {
		t.Fatalf("expected valid UTF-8, got %q", got)
	}
}

func TestPrefixNormalizesMalformedUTF8(t *testing.T) {
	input := string([]byte{'o', 'k', 0xff})
	got, truncated := Prefix(input, 3)
	if truncated {
		t.Fatalf("did not expect truncation, got %q", got)
	}
	if !utf8.ValidString(got) || got != "ok�" {
		t.Fatalf("expected malformed input normalized to valid UTF-8, got %q", got)
	}
}

func TestLimitLeavesSmallContentUnchanged(t *testing.T) {
	result := Limit("hello", 10)
	if result.Truncated || result.Content != "hello" || result.OriginalChars != 5 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestLimitUsesHeadTailAndNotice(t *testing.T) {
	result := Limit("abcdefghijklmnopqrstuvwxyz"+strings.Repeat("x", 80)+"z", 50)
	if !result.Truncated {
		t.Fatalf("expected truncated result")
	}
	if !strings.Contains(result.Content, "truncated") {
		t.Fatalf("expected truncation notice, got %q", result.Content)
	}
	if !strings.HasPrefix(result.Content, "a") || !strings.HasSuffix(result.Content, "z") {
		t.Fatalf("expected head and tail to be preserved, got %q", result.Content)
	}
}

func TestCappedBufferLimitsOutput(t *testing.T) {
	buffer := NewCappedBuffer(8)
	if _, err := buffer.Write([]byte("hello")); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	if _, err := buffer.Write([]byte(" world")); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	result := buffer.Result()
	if !result.Truncated || !strings.Contains(result.Content, "truncated") {
		t.Fatalf("expected capped output, got %+v", result)
	}
}
