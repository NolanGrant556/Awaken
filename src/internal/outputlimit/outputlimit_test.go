package outputlimit

import (
	"strings"
	"testing"
)

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
