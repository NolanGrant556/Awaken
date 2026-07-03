package agui

import "testing"

func TestEncoderEncodeSSE(t *testing.T) {
	frame, err := (Encoder{}).EncodeSSE(Event{
		Type:      EventTextMessageContent,
		MessageID: "msg-1",
		Delta:     "Hello",
	})
	if err != nil {
		t.Fatalf("EncodeSSE returned error: %v", err)
	}

	got := string(frame)
	want := "data: {\"type\":\"TEXT_MESSAGE_CONTENT\",\"messageId\":\"msg-1\",\"delta\":\"Hello\"}\n\n"
	if got != want {
		t.Fatalf("unexpected SSE frame:\nwant %q\ngot  %q", want, got)
	}
}

func TestEncoderEncodeSSEWithSeq(t *testing.T) {
	frame, err := (Encoder{}).EncodeSSE(Event{
		Type:      EventTextMessageContent,
		Seq:       42,
		MessageID: "msg-1",
		Delta:     "Hello",
	})
	if err != nil {
		t.Fatalf("EncodeSSE returned error: %v", err)
	}

	got := string(frame)
	want := "id: 42\ndata: {\"type\":\"TEXT_MESSAGE_CONTENT\",\"seq\":42,\"messageId\":\"msg-1\",\"delta\":\"Hello\"}\n\n"
	if got != want {
		t.Fatalf("unexpected SSE frame:\nwant %q\ngot  %q", want, got)
	}
}

func TestEncoderEncodeSSEIncludesOptionalCompactFalseFlags(t *testing.T) {
	fallbackUsed := false

	frame, err := (Encoder{}).EncodeSSE(Event{
		Type:                     EventAwakenCompactFinished,
		ContextCompactBoundaryID: "compact_1",
		CompactBoundaryID:        "compact_1",
		CompactFallbackUsed:      &fallbackUsed,
	})
	if err != nil {
		t.Fatalf("EncodeSSE returned error: %v", err)
	}

	got := string(frame)
	want := "data: {\"type\":\"AWAKEN_COMPACT_FINISHED\",\"contextCompactBoundaryId\":\"compact_1\",\"compactBoundaryId\":\"compact_1\",\"compactFallbackUsed\":false}\n\n"
	if got != want {
		t.Fatalf("unexpected SSE frame:\nwant %q\ngot  %q", want, got)
	}
}
