package vcrrecorder

import "time"

const (
	SchemaVersion          = 1
	DefaultMaxPayloadBytes = 64 * 1024
	RedactedValue          = "[REDACTED]"

	MarkerKindMedia           = "media_marker"
	MarkerKindPayloadTooLarge = "payload_too_large"
)

type Event struct {
	SchemaVersion int          `json:"schema_version"`
	Sequence      uint64       `json:"sequence"`
	Timestamp     time.Time    `json:"timestamp"`
	Type          string       `json:"type"`
	Source        string       `json:"source"`
	SessionID     string       `json:"session_id,omitempty"`
	RunID         string       `json:"run_id,omitempty"`
	Payload       any          `json:"payload,omitempty"`
	PayloadMeta   *PayloadMeta `json:"payload_meta,omitempty"`
}

type PayloadMeta struct {
	Redacted      bool   `json:"redacted,omitempty"`
	Capped        bool   `json:"capped,omitempty"`
	Media         bool   `json:"media,omitempty"`
	OriginalBytes int    `json:"original_bytes,omitempty"`
	StoredBytes   int    `json:"stored_bytes,omitempty"`
	SHA256        string `json:"sha256,omitempty"`
}

type Recording struct {
	SchemaVersion int     `json:"schema_version"`
	Events        []Event `json:"events"`
}

type Clock interface {
	Now() time.Time
}

type SystemClock struct{}

func (SystemClock) Now() time.Time {
	return time.Now().UTC()
}

type Filter struct {
	SessionID string
	RunID     string
	Types     []string
	Sources   []string
}

func (f Filter) Match(event Event) bool {
	if f.SessionID != "" && event.SessionID != f.SessionID {
		return false
	}
	if f.RunID != "" && event.RunID != f.RunID {
		return false
	}
	if len(f.Types) > 0 && !containsString(f.Types, event.Type) {
		return false
	}
	if len(f.Sources) > 0 && !containsString(f.Sources, event.Source) {
		return false
	}
	return true
}

type MediaPayload struct {
	MIMEType string
	Name     string
	Data     []byte
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
