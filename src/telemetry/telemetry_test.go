package telemetry

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type sinkCall struct {
	name     string
	metadata Metadata
	async    bool
}

type recordingSink struct {
	calls    []sinkCall
	asyncErr error
}

func (s *recordingSink) Log(eventName string, metadata Metadata) {
	s.calls = append(s.calls, sinkCall{name: eventName, metadata: metadata})
}

func (s *recordingSink) LogAsync(_ context.Context, eventName string, metadata Metadata) error {
	s.calls = append(s.calls, sinkCall{name: eventName, metadata: metadata, async: true})
	return s.asyncErr
}

func TestLoggerQueuesEventsUntilSinkAttached(t *testing.T) {
	logger := NewLogger()
	sink := &recordingSink{}

	logger.Log("startup_sync", Metadata{"queued": true})
	if err := logger.LogAsync(context.Background(), "startup_async", Metadata{"count": 2}); err != nil {
		t.Fatalf("LogAsync before attach returned error: %v", err)
	}

	if len(sink.calls) != 0 {
		t.Fatalf("sink received calls before attach: %#v", sink.calls)
	}

	if attached := logger.AttachSink(sink); !attached {
		t.Fatal("AttachSink returned false for first sink")
	}

	if len(sink.calls) != 2 {
		t.Fatalf("expected 2 drained calls, got %d", len(sink.calls))
	}
	if sink.calls[0].name != "startup_sync" || sink.calls[0].async {
		t.Fatalf("first call did not drain as sync event: %#v", sink.calls[0])
	}
	if sink.calls[1].name != "startup_async" || !sink.calls[1].async {
		t.Fatalf("second call did not drain as async event: %#v", sink.calls[1])
	}
}

func TestLoggerAttachIsIdempotentAndKeepsFirstSink(t *testing.T) {
	logger := NewLogger()
	first := &recordingSink{}
	second := &recordingSink{}

	if attached := logger.AttachSink(first); !attached {
		t.Fatal("first AttachSink returned false")
	}
	if attached := logger.AttachSink(second); attached {
		t.Fatal("second AttachSink returned true")
	}

	logger.Log("after_attach", Metadata{"ok": true})

	if len(first.calls) != 1 {
		t.Fatalf("first sink got %d calls, want 1", len(first.calls))
	}
	if len(second.calls) != 0 {
		t.Fatalf("second sink should not receive calls: %#v", second.calls)
	}
}

func TestSanitizeMetadataStripsProtoSensitiveAndUnsafeValues(t *testing.T) {
	metadata := SanitizeMetadata(Metadata{
		"ok_bool":       true,
		"ok_int":        12,
		"ok_float":      1.5,
		"safe_string":   String("linux"),
		"plain_string":  "possibly /a/file/path.go",
		"_PROTO_email":  String("user@example.com"),
		"proto_name":    String("private name"),
		"filePath":      String("/tmp/secret.go"),
		"code":          String("fmt.Println(secret)"),
		"nested":        map[string]string{"x": "y"},
		"unsupported":   []string{"x"},
		"authorization": String("Bearer token"),
	})

	want := Metadata{
		"ok_bool":     true,
		"ok_int":      12,
		"ok_float":    1.5,
		"safe_string": "linux",
	}
	if !reflect.DeepEqual(metadata, want) {
		t.Fatalf("sanitized metadata mismatch:\n got %#v\nwant %#v", metadata, want)
	}
}

func TestPackageResetForTesting(t *testing.T) {
	ResetForTesting()
	first := &recordingSink{}
	AttachSink(first)
	Log("first", nil)

	ResetForTesting()
	second := &recordingSink{}
	Log("queued_after_reset", nil)
	AttachSink(second)

	if len(first.calls) != 1 {
		t.Fatalf("first sink calls changed after reset: %#v", first.calls)
	}
	if len(second.calls) != 1 || second.calls[0].name != "queued_after_reset" {
		t.Fatalf("second sink did not receive queued event after reset: %#v", second.calls)
	}
}

func TestLogAsyncReturnsSinkErrorWhenAttached(t *testing.T) {
	logger := NewLogger()
	wantErr := errors.New("async sink failed")
	sink := &recordingSink{asyncErr: wantErr}
	logger.AttachSink(sink)

	err := logger.LogAsync(context.Background(), "async", nil)
	if !errors.Is(err, wantErr) {
		t.Fatalf("LogAsync error = %v, want %v", err, wantErr)
	}
}
