package vcrrecorder

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time {
	return c.now
}

func TestJSONLRoundtrip(t *testing.T) {
	now := time.Date(2026, 6, 28, 10, 0, 0, 0, time.UTC)
	var buf bytes.Buffer
	recorder := NewRecorder(&buf, RecorderOptions{
		Clock:     fixedClock{now: now},
		Source:    "agent",
		SessionID: "session-1",
		RunID:     "run-1",
	})

	if _, err := recorder.Append(Event{
		Type:    "text_delta",
		Payload: map[string]any{"text": "hello"},
	}); err != nil {
		t.Fatalf("append first event: %v", err)
	}
	if _, err := recorder.Append(Event{
		Type:    "run_finished",
		Payload: map[string]any{"result": "done"},
	}); err != nil {
		t.Fatalf("append second event: %v", err)
	}

	recording, err := ReadJSONL(bytes.NewReader(buf.Bytes()), Filter{})
	if err != nil {
		t.Fatalf("read JSONL: %v", err)
	}
	if recording.SchemaVersion != SchemaVersion {
		t.Fatalf("expected schema version %d, got %d", SchemaVersion, recording.SchemaVersion)
	}
	if len(recording.Events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(recording.Events))
	}

	first := recording.Events[0]
	if first.SchemaVersion != SchemaVersion || first.Sequence != 1 {
		t.Fatalf("unexpected first event schema/seq: %+v", first)
	}
	if first.Type != "text_delta" || first.Source != "agent" || first.SessionID != "session-1" || first.RunID != "run-1" {
		t.Fatalf("unexpected first event identity: %+v", first)
	}
	if !first.Timestamp.Equal(now) {
		t.Fatalf("expected timestamp %s, got %s", now, first.Timestamp)
	}
	payload := first.Payload.(map[string]any)
	if payload["text"] != "hello" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}

func TestRedactionRemovesSecrets(t *testing.T) {
	var buf bytes.Buffer
	recorder := NewRecorder(&buf, RecorderOptions{Source: "agent"})

	if _, err := recorder.Append(Event{
		Type: "tool_result",
		Payload: map[string]any{
			"api_key": "sk-secret-value-123456789",
			"nested": map[string]any{
				"message": "request failed with Authorization: Bearer abcdefghijklmnop",
			},
		},
	}); err != nil {
		t.Fatalf("append event: %v", err)
	}

	raw := buf.String()
	for _, secret := range []string{"sk-secret-value-123456789", "abcdefghijklmnop"} {
		if strings.Contains(raw, secret) {
			t.Fatalf("raw JSONL leaked secret %q: %s", secret, raw)
		}
	}
	if !strings.Contains(raw, RedactedValue) {
		t.Fatalf("expected redacted marker in raw JSONL: %s", raw)
	}

	recording, err := ReadJSONL(bytes.NewReader(buf.Bytes()), Filter{})
	if err != nil {
		t.Fatalf("read JSONL: %v", err)
	}
	event := recording.Events[0]
	if event.PayloadMeta == nil || !event.PayloadMeta.Redacted {
		t.Fatalf("expected redaction metadata, got %+v", event.PayloadMeta)
	}
	payload := event.Payload.(map[string]any)
	if payload["api_key"] != RedactedValue {
		t.Fatalf("expected key-based redaction, got %+v", payload)
	}
}

func TestPayloadSizeCapUsesMarker(t *testing.T) {
	var buf bytes.Buffer
	recorder := NewRecorder(&buf, RecorderOptions{
		Source:          "agent",
		MaxPayloadBytes: 32,
	})
	large := strings.Repeat("x", 256)

	if _, err := recorder.Append(Event{
		Type:    "tool_result",
		Payload: map[string]any{"output": large},
	}); err != nil {
		t.Fatalf("append event: %v", err)
	}

	if strings.Contains(buf.String(), large) {
		t.Fatalf("raw JSONL kept oversized payload: %s", buf.String())
	}
	recording, err := ReadJSONL(bytes.NewReader(buf.Bytes()), Filter{})
	if err != nil {
		t.Fatalf("read JSONL: %v", err)
	}
	event := recording.Events[0]
	if event.PayloadMeta == nil || !event.PayloadMeta.Capped || event.PayloadMeta.OriginalBytes == 0 {
		t.Fatalf("expected cap metadata, got %+v", event.PayloadMeta)
	}
	payload := event.Payload.(map[string]any)
	if payload["kind"] != MarkerKindPayloadTooLarge {
		t.Fatalf("expected payload cap marker, got %+v", payload)
	}
}

func TestMediaPayloadUsesMarker(t *testing.T) {
	var buf bytes.Buffer
	recorder := NewRecorder(&buf, RecorderOptions{Source: "agent"})

	if _, err := recorder.Append(Event{
		Type: "artifact_created",
		Payload: map[string]any{
			"image": MediaPayload{
				MIMEType: "image/png",
				Name:     "shot.png",
				Data:     []byte{0, 1, 2, 3, 4},
			},
		},
	}); err != nil {
		t.Fatalf("append event: %v", err)
	}

	if strings.Contains(buf.String(), "AAECAwQ") {
		t.Fatalf("raw JSONL contains base64 media payload: %s", buf.String())
	}
	recording, err := ReadJSONL(bytes.NewReader(buf.Bytes()), Filter{})
	if err != nil {
		t.Fatalf("read JSONL: %v", err)
	}
	event := recording.Events[0]
	if event.PayloadMeta == nil || !event.PayloadMeta.Media {
		t.Fatalf("expected media metadata, got %+v", event.PayloadMeta)
	}
	payload := event.Payload.(map[string]any)
	image := payload["image"].(map[string]any)
	if image["kind"] != MarkerKindMedia || image["media_type"] != "image/png" || image["name"] != "shot.png" {
		t.Fatalf("unexpected media marker: %+v", image)
	}
}

func TestReadJSONLFilter(t *testing.T) {
	var buf bytes.Buffer
	recorder := NewRecorder(&buf, RecorderOptions{Source: "agent"})

	events := []Event{
		{Type: "delta", SessionID: "session-1", RunID: "run-1"},
		{Type: "delta", SessionID: "session-2", RunID: "run-2"},
		{Type: "finished", SessionID: "session-1", RunID: "run-1"},
	}
	for _, event := range events {
		if _, err := recorder.Append(event); err != nil {
			t.Fatalf("append event: %v", err)
		}
	}

	recording, err := ReadJSONL(bytes.NewReader(buf.Bytes()), Filter{
		SessionID: "session-1",
		RunID:     "run-1",
		Types:     []string{"finished"},
	})
	if err != nil {
		t.Fatalf("read JSONL: %v", err)
	}
	if len(recording.Events) != 1 {
		t.Fatalf("expected 1 filtered event, got %d", len(recording.Events))
	}
	if recording.Events[0].Type != "finished" || recording.Events[0].Sequence != 3 {
		t.Fatalf("unexpected filtered event: %+v", recording.Events[0])
	}
}

func TestReplayOrderAndTimingOptions(t *testing.T) {
	base := time.Date(2026, 6, 28, 10, 0, 0, 0, time.UTC)
	recording := Recording{
		SchemaVersion: SchemaVersion,
		Events: []Event{
			{SchemaVersion: SchemaVersion, Sequence: 1, Timestamp: base, Type: "first"},
			{SchemaVersion: SchemaVersion, Sequence: 2, Timestamp: base.Add(25 * time.Millisecond), Type: "second"},
			{SchemaVersion: SchemaVersion, Sequence: 3, Timestamp: base.Add(40 * time.Millisecond), Type: "third"},
		},
	}

	var slept []time.Duration
	replayer := NewReplayer(recording, ReplayOptions{
		PreserveIntervals: true,
		Sleep: func(ctx context.Context, delay time.Duration) error {
			slept = append(slept, delay)
			return nil
		},
	})
	var got []string
	if err := replayer.Replay(context.Background(), func(event Event) error {
		got = append(got, event.Type)
		return nil
	}); err != nil {
		t.Fatalf("replay with timing: %v", err)
	}
	if strings.Join(got, ",") != "first,second,third" {
		t.Fatalf("unexpected replay order: %v", got)
	}
	if len(slept) != 2 || slept[0] != 25*time.Millisecond || slept[1] != 15*time.Millisecond {
		t.Fatalf("unexpected replay sleeps: %v", slept)
	}

	slept = nil
	replayer = NewReplayer(recording, ReplayOptions{
		Sleep: func(ctx context.Context, delay time.Duration) error {
			slept = append(slept, delay)
			return nil
		},
	})
	if err := replayer.Replay(context.Background(), func(event Event) error { return nil }); err != nil {
		t.Fatalf("replay without timing: %v", err)
	}
	if len(slept) != 0 {
		t.Fatalf("expected no sleeps when intervals are ignored, got %v", slept)
	}
}

func TestBadSequenceReportsLocation(t *testing.T) {
	input := `{"schema_version":1,"sequence":1,"timestamp":"2026-06-28T10:00:00Z","type":"first","source":"agent"}` + "\n" +
		`{"schema_version":1,"sequence":3,"timestamp":"2026-06-28T10:00:01Z","type":"third","source":"agent"}` + "\n"

	_, err := ReadJSONL(strings.NewReader(input), Filter{})
	if err == nil {
		t.Fatal("expected integrity error")
	}
	var integrityErr *IntegrityError
	if !errors.As(err, &integrityErr) {
		t.Fatalf("expected IntegrityError, got %T: %v", err, err)
	}
	if integrityErr.Line != 2 || integrityErr.Expected != 2 || integrityErr.Got != 3 {
		t.Fatalf("unexpected integrity error: %+v", integrityErr)
	}
}

func TestBadJSONReportsLineAndOffset(t *testing.T) {
	first := `{"schema_version":1,"sequence":1,"timestamp":"2026-06-28T10:00:00Z","type":"first","source":"agent"}` + "\n"
	input := first + `{"schema_version":1,"sequence":2,` + "\n"

	_, err := ReadJSONL(strings.NewReader(input), Filter{})
	if err == nil {
		t.Fatal("expected read error")
	}
	var readErr *ReadError
	if !errors.As(err, &readErr) {
		t.Fatalf("expected ReadError, got %T: %v", err, err)
	}
	if readErr.Line != 2 || readErr.Offset != int64(len(first)) {
		t.Fatalf("unexpected read error location: %+v", readErr)
	}
	if !strings.Contains(err.Error(), "line 2") {
		t.Fatalf("expected line number in error, got %v", err)
	}
}
