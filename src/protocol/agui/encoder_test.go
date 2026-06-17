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
