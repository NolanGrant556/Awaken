package runtime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSteeringQueuesAttachmentOnceAndSeals(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	t.Setenv(runtimeUserDataDirEnv, dir)
	journal, err := NewExecutionJournal(filepath.Join(dir, "test.journal"), "thread_steering")
	if err != nil {
		t.Fatal(err)
	}
	session := NewSession("run_steering", "thread_steering", "original", nil, nil, nil)
	session.steering.enabled = true
	session.SetSemanticRecorder(newLiveSemanticRecorder(journal, session))
	source := filepath.Join(dir, "input.txt")
	if err := os.WriteFile(source, []byte("original file"), 0o600); err != nil {
		t.Fatal(err)
	}
	request := SteeringRequest{ThreadID: "thread_steering", RequestID: "request-one",
		Attachments: []RunInputAttachment{{Kind: RunInputAttachmentKindFile, Path: source}}}
	first, err := session.submitSteering(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := session.submitSteering(ctx, request)
	if err != nil || duplicate != first {
		t.Fatalf("retry = %+v, %v, want %+v", duplicate, err, first)
	}
	if err := os.WriteFile(source, []byte("modified file"), 0o600); err != nil {
		t.Fatal(err)
	}
	sink := sessionEventSink{session: session}
	if !sink.SealSteering(true) {
		t.Fatal("pending input must keep the run open")
	}
	inputs, err := sink.TakeSteering(ctx)
	if err != nil || len(inputs) != 1 || inputs[0].ID != first.ID || len(inputs[0].Attachments) != 1 {
		t.Fatalf("unexpected drained input: %+v, %v", inputs, err)
	}
	copyPath := inputs[0].Attachments[0].Path
	data, err := os.ReadFile(copyPath)
	if err != nil || string(data) != "original file" || copyPath == source {
		t.Fatalf("expected frozen copy, got %q, %v", data, err)
	}
	if sink.SealSteering(true) {
		t.Fatal("empty inbox must allow termination")
	}
	request.RequestID = "request-two"
	if _, err := session.submitSteering(ctx, request); !errors.Is(err, ErrSteeringUnavailable) {
		t.Fatalf("late submission = %v", err)
	}
}

func TestSteeringRejectsImageForTextOnlyRun(t *testing.T) {
	session := NewSession("run_steering", "thread_steering", "original", nil, nil, nil)
	session.steering.enabled = true
	recorder := newLiveSemanticRecorder(&flushRecoveryRecorder{}, session)
	session.SetSemanticRecorder(recorder)
	_, err := session.submitSteering(context.Background(), SteeringRequest{
		ThreadID: "thread_steering", RequestID: "image",
		Attachments: []RunInputAttachment{{Kind: RunInputAttachmentKindImage, MimeType: "image/png"}},
	})
	if !errors.Is(err, ErrInvalidInput) || len(session.steering.pending) != 0 {
		t.Fatalf("image was accepted for a text-only run: %v", err)
	}
}
