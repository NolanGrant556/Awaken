package remotesse

import (
	"net/url"
	"testing"
)

func TestCursorAcceptDuplicateSequence(t *testing.T) {
	cursor := NewCursor(0)
	if !cursor.Accept(10) {
		t.Fatal("expected first sequence to be accepted")
	}
	if cursor.Accept(10) {
		t.Fatal("expected duplicate sequence to be rejected")
	}
	if got := cursor.LastSequenceNum(); got != 10 {
		t.Fatalf("expected high-water 10, got %d", got)
	}
}

func TestCursorAcceptOutOfOrderOldFrame(t *testing.T) {
	cursor := NewCursor(0)
	if !cursor.Accept(20) {
		t.Fatal("expected sequence 20 to be accepted")
	}
	if !cursor.Accept(18) {
		t.Fatal("expected unseen out-of-order sequence to be accepted")
	}
	if got := cursor.LastSequenceNum(); got != 20 {
		t.Fatalf("expected high-water to remain 20, got %d", got)
	}
	if cursor.Accept(18) {
		t.Fatal("expected repeated out-of-order sequence to be rejected")
	}
}

func TestCursorBoundedPrune(t *testing.T) {
	cursor := NewCursor(0)
	for seq := int64(1); seq <= defaultSeenLimit+50; seq++ {
		if !cursor.Accept(seq) {
			t.Fatalf("expected sequence %d to be accepted", seq)
		}
	}

	if got := cursor.SeenCount(); got > defaultSeenLimit {
		t.Fatalf("expected seen count <= %d, got %d", defaultSeenLimit, got)
	}
	if cursor.Accept(defaultSeenLimit + 50) {
		t.Fatal("expected recent high-water sequence to remain duplicate")
	}
	if !cursor.Accept(1) {
		t.Fatal("expected old pruned sequence 1 to be accepted again")
	}
}

func TestCursorResumeParams(t *testing.T) {
	cursor := NewCursor(42)

	query := cursor.ResumeQuery()
	if got := query.Get("from_sequence_num"); got != "42" {
		t.Fatalf("expected from_sequence_num=42, got %q", got)
	}

	headers := cursor.Headers()
	if got := headers["Last-Event-ID"]; got != "42" {
		t.Fatalf("expected Last-Event-ID=42, got %q", got)
	}

	resumed, err := cursor.ApplyResume("https://example.test/stream?existing=1")
	if err != nil {
		t.Fatalf("ApplyResume returned error: %v", err)
	}
	parsed, err := url.Parse(resumed)
	if err != nil {
		t.Fatalf("resumed URL did not parse: %v", err)
	}
	if got := parsed.Query().Get("from_sequence_num"); got != "42" {
		t.Fatalf("expected resumed URL from_sequence_num=42, got %q", got)
	}
	if got := parsed.Query().Get("existing"); got != "1" {
		t.Fatalf("expected existing query param to be preserved, got %q", got)
	}
}

func TestCursorResumeParamsEmptyWhenUnset(t *testing.T) {
	cursor := NewCursor(0)

	if got := cursor.ResumeQuery().Encode(); got != "" {
		t.Fatalf("expected empty resume query, got %q", got)
	}
	if got := cursor.Headers(); len(got) != 0 {
		t.Fatalf("expected empty headers, got %+v", got)
	}
	resumed, err := cursor.ApplyResume("https://example.test/stream")
	if err != nil {
		t.Fatalf("ApplyResume returned error: %v", err)
	}
	if resumed != "https://example.test/stream" {
		t.Fatalf("expected URL unchanged, got %q", resumed)
	}
}
