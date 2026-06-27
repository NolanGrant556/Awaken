package structuredio

import "testing"

func TestLineReaderReadsSplitChunksAndSkipsEmptyLines(t *testing.T) {
	reader := NewLineReader()

	result := reader.Push([]byte(`{"type":"us`))
	if len(result.Envelopes) != 0 || len(result.Diagnostics) != 0 {
		t.Fatalf("partial chunk produced output: %+v", result)
	}

	result = reader.Push([]byte("er\",\"message\":{\"content\":\"one\"}}\n\n{\"type\":\"assistant\"}\n"))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", result.Diagnostics)
	}
	if got, want := envelopeTypes(result.Envelopes), []EnvelopeType{EnvelopeUser, EnvelopeAssistant}; !sameEnvelopeTypes(got, want) {
		t.Fatalf("unexpected envelope types: want %v got %v", want, got)
	}
}

func TestLineReaderReadsFinalFragment(t *testing.T) {
	reader := NewLineReader()

	result := reader.Push([]byte(`{"type":"stream_event","event":{"name":"tick"}}`))
	if len(result.Envelopes) != 0 {
		t.Fatalf("unterminated fragment produced output: %+v", result.Envelopes)
	}

	result = reader.Close()
	if len(result.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", result.Diagnostics)
	}
	if got, want := envelopeTypes(result.Envelopes), []EnvelopeType{EnvelopeStreamEvent}; !sameEnvelopeTypes(got, want) {
		t.Fatalf("unexpected envelope types: want %v got %v", want, got)
	}
}

func TestLineReaderReturnsDiagnosticForJSONParseError(t *testing.T) {
	reader := NewLineReader()

	result := reader.Push([]byte("{bad json}\n{\"type\":\"user\"}\n"))
	if len(result.Diagnostics) != 1 {
		t.Fatalf("expected one diagnostic, got %+v", result.Diagnostics)
	}
	if len(result.Envelopes) != 1 || result.Envelopes[0].Type != EnvelopeUser {
		t.Fatalf("expected valid line after diagnostic to still parse, got %+v", result.Envelopes)
	}
}

func TestLineReaderPrependUserMessageBeforeNextInput(t *testing.T) {
	reader := NewLineReader()

	result := reader.Push([]byte(`{"type":"assistant","message":{"content":"after"}`))
	if len(result.Envelopes) != 0 {
		t.Fatalf("partial chunk produced output: %+v", result.Envelopes)
	}
	if err := reader.PrependUserMessage("queued"); err != nil {
		t.Fatalf("PrependUserMessage returned error: %v", err)
	}

	result = reader.Push([]byte("}\n"))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", result.Diagnostics)
	}
	if got, want := envelopeTypes(result.Envelopes), []EnvelopeType{EnvelopeUser, EnvelopeAssistant}; !sameEnvelopeTypes(got, want) {
		t.Fatalf("unexpected envelope order: want %v got %v", want, got)
	}
	content, ok := EnvelopeMessageContent(result.Envelopes[0])
	if !ok || content != "queued" {
		t.Fatalf("prepended message content mismatch: ok=%v content=%q", ok, content)
	}
}

func TestLineReaderFiltersAndRecognizesKeepAlive(t *testing.T) {
	filteringReader := NewLineReader()
	result := filteringReader.Push([]byte("{\"type\":\"keep_alive\"}\n{\"type\":\"error\"}\n"))
	if got, want := envelopeTypes(result.Envelopes), []EnvelopeType{EnvelopeError}; !sameEnvelopeTypes(got, want) {
		t.Fatalf("unexpected filtered envelope types: want %v got %v", want, got)
	}

	recognizingReader := NewLineReader(EmitKeepAlive())
	result = recognizingReader.Push([]byte("{\"type\":\"keep_alive\"}\n"))
	if got, want := envelopeTypes(result.Envelopes), []EnvelopeType{EnvelopeKeepAlive}; !sameEnvelopeTypes(got, want) {
		t.Fatalf("unexpected recognized envelope types: want %v got %v", want, got)
	}
	if !result.Envelopes[0].IsKeepAlive() {
		t.Fatalf("keep_alive envelope was not recognized")
	}
}

func envelopeTypes(envelopes []Envelope) []EnvelopeType {
	types := make([]EnvelopeType, 0, len(envelopes))
	for _, envelope := range envelopes {
		types = append(types, envelope.Type)
	}
	return types
}

func sameEnvelopeTypes(a, b []EnvelopeType) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
