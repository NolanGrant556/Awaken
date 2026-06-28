package remotesse

import "testing"

func TestParseFramesPartialBuffer(t *testing.T) {
	frames, remaining := ParseFrames("event: client_event\nid: 7\ndata: {")
	if len(frames) != 0 {
		t.Fatalf("expected no complete frames, got %d", len(frames))
	}
	if remaining != "event: client_event\nid: 7\ndata: {" {
		t.Fatalf("unexpected remaining buffer: %q", remaining)
	}

	frames, remaining = ParseFrames(remaining + `"ok":true}` + "\n\n")
	if remaining != "" {
		t.Fatalf("expected empty remaining buffer, got %q", remaining)
	}
	if len(frames) != 1 {
		t.Fatalf("expected one frame, got %d", len(frames))
	}
	if frames[0].Event != "client_event" || frames[0].ID != "7" || frames[0].Data != `{"ok":true}` {
		t.Fatalf("unexpected frame: %+v", frames[0])
	}
}

func TestParseFramesMultiDataLines(t *testing.T) {
	frames, remaining := ParseFrames("event: client_event\nid: 8\ndata: first\ndata: second\nretry: 1000\n\n")
	if remaining != "" {
		t.Fatalf("expected empty remaining buffer, got %q", remaining)
	}
	if len(frames) != 1 {
		t.Fatalf("expected one frame, got %d", len(frames))
	}
	if frames[0].Data != "first\nsecond" {
		t.Fatalf("expected joined data lines, got %q", frames[0].Data)
	}
}

func TestParseFramesCommentFrame(t *testing.T) {
	frames, remaining := ParseFrames(":keepalive\n\n\n\nunknown: ignored\n\n")
	if remaining != "" {
		t.Fatalf("expected empty remaining buffer, got %q", remaining)
	}
	if len(frames) != 1 {
		t.Fatalf("expected only the comment frame, got %d", len(frames))
	}
	if !frames[0].Comment || frames[0].Data != "" {
		t.Fatalf("unexpected comment frame: %+v", frames[0])
	}
}

func TestParseFramesCRLF(t *testing.T) {
	frames, remaining := ParseFrames("event: client_event\r\nid: 9\r\ndata: ok\r\n\r\n")
	if remaining != "" {
		t.Fatalf("expected empty remaining buffer, got %q", remaining)
	}
	if len(frames) != 1 {
		t.Fatalf("expected one frame, got %d", len(frames))
	}
	if frames[0].Event != "client_event" || frames[0].ID != "9" || frames[0].Data != "ok" {
		t.Fatalf("unexpected frame: %+v", frames[0])
	}
}

func TestFrameSequenceNum(t *testing.T) {
	seq, ok := Frame{ID: "42"}.SequenceNum()
	if !ok || seq != 42 {
		t.Fatalf("expected sequence 42, got %d ok=%v", seq, ok)
	}

	if _, ok := (Frame{ID: "nope"}).SequenceNum(); ok {
		t.Fatal("expected invalid sequence to return ok=false")
	}
}
