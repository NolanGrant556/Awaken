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
