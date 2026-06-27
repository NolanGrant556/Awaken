package vcrrecorder

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"
)

var ErrMissingEventType = errors.New("vcr event type is required")

type RecorderOptions struct {
	Clock           Clock
	Source          string
	SessionID       string
	RunID           string
	MaxPayloadBytes int
	DisableRedact   bool
	StartSequence   uint64
}

type Recorder struct {
	mu              sync.Mutex
	writer          io.Writer
	clock           Clock
	source          string
	sessionID       string
	runID           string
	maxPayloadBytes int
	disableRedact   bool
	nextSeq         uint64
}

func NewRecorder(writer io.Writer, opts RecorderOptions) *Recorder {
	clock := opts.Clock
	if clock == nil {
		clock = SystemClock{}
	}

	maxPayloadBytes := opts.MaxPayloadBytes
	if maxPayloadBytes == 0 {
		maxPayloadBytes = DefaultMaxPayloadBytes
	}

	return &Recorder{
		writer:          writer,
		clock:           clock,
		source:          opts.Source,
		sessionID:       opts.SessionID,
		runID:           opts.RunID,
		maxPayloadBytes: maxPayloadBytes,
		disableRedact:   opts.DisableRedact,
		nextSeq:         opts.StartSequence,
	}
}

func (r *Recorder) Append(event Event) (Event, error) {
	if event.Type == "" {
		return Event{}, ErrMissingEventType
	}
	if r == nil || r.writer == nil {
		return Event{}, errors.New("vcr recorder writer is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.nextSeq++
	event.SchemaVersion = SchemaVersion
	event.Sequence = r.nextSeq
	event.Timestamp = normalizeTimestamp(event.Timestamp, r.clock)
	if event.Source == "" {
		event.Source = r.source
	}
	if event.SessionID == "" {
		event.SessionID = r.sessionID
	}
	if event.RunID == "" {
		event.RunID = r.runID
	}

	payload, meta, err := preparePayload(event.Payload, r.maxPayloadBytes, r.disableRedact)
	if err != nil {
		return Event{}, err
	}
	event.Payload = payload
	event.PayloadMeta = mergePayloadMeta(event.PayloadMeta, meta)

	line, err := json.Marshal(event)
	if err != nil {
		return Event{}, err
	}
	line = append(line, '\n')
	if err := writeFull(r.writer, line); err != nil {
		return Event{}, err
	}
	return event, nil
}

func normalizeTimestamp(timestamp time.Time, clock Clock) time.Time {
	if timestamp.IsZero() {
		return clock.Now().UTC()
	}
	return timestamp.UTC()
}

func mergePayloadMeta(existing, next *PayloadMeta) *PayloadMeta {
	if existing == nil {
		return next
	}
	if next == nil {
		return existing
	}
	merged := *existing
	merged.Redacted = merged.Redacted || next.Redacted
	merged.Capped = merged.Capped || next.Capped
	merged.Media = merged.Media || next.Media
	if next.OriginalBytes > 0 {
		merged.OriginalBytes = next.OriginalBytes
	}
	if next.StoredBytes > 0 {
		merged.StoredBytes = next.StoredBytes
	}
	if next.SHA256 != "" {
		merged.SHA256 = next.SHA256
	}
	return &merged
}

func writeFull(writer io.Writer, data []byte) error {
	written, err := writer.Write(data)
	if err != nil {
		return err
	}
	if written != len(data) {
		return fmt.Errorf("short write: wrote %d of %d bytes", written, len(data))
	}
	return nil
}
