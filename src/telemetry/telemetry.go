package telemetry

import (
	"context"
	"math"
	"strings"
	"sync"
)

// SafeString marks metadata that has already been checked to contain no PII,
// source code, prompts, file paths, or other sensitive content.
type SafeString string

// String marks a string as safe for telemetry metadata.
func String(value string) SafeString {
	return SafeString(value)
}

// Metadata is a flat telemetry payload. Sanitization keeps only bools, numbers,
// and explicitly safe strings, and drops sensitive or proto-reserved fields.
type Metadata map[string]any

// Sink receives telemetry events. Implementations can route events to logs,
// files, or external analytics backends.
type Sink interface {
	Log(eventName string, metadata Metadata)
	LogAsync(ctx context.Context, eventName string, metadata Metadata) error
}

type queuedEvent struct {
	name     string
	metadata Metadata
	async    bool
}

// Logger queues events until a sink is attached, then drains them through the
// matching sync or async sink method.
type Logger struct {
	mu    sync.Mutex
	sink  Sink
	queue []queuedEvent
}

// NewLogger creates an isolated telemetry logger.
func NewLogger() *Logger {
	return &Logger{}
}

var defaultLogger = NewLogger()

// AttachSink attaches the package-level sink and drains queued events. It is
// idempotent: later calls are no-ops and return false.
func AttachSink(sink Sink) bool {
	return defaultLogger.AttachSink(sink)
}

// Log records a package-level synchronous telemetry event.
func Log(eventName string, metadata Metadata) {
	defaultLogger.Log(eventName, metadata)
}

// LogAsync records a package-level asynchronous telemetry event.
func LogAsync(ctx context.Context, eventName string, metadata Metadata) error {
	return defaultLogger.LogAsync(ctx, eventName, metadata)
}

// ResetForTesting clears the package-level sink and queue.
func ResetForTesting() {
	defaultLogger = NewLogger()
}

// AttachSink attaches the sink and drains queued events. It returns true when
// this call attached the sink.
func (l *Logger) AttachSink(sink Sink) bool {
	if sink == nil {
		return false
	}

	l.mu.Lock()
	if l.sink != nil {
		l.mu.Unlock()
		return false
	}
	l.sink = sink
	queued := append([]queuedEvent(nil), l.queue...)
	l.queue = nil
	l.mu.Unlock()

	for _, event := range queued {
		if event.async {
			_ = sink.LogAsync(context.Background(), event.name, event.metadata)
			continue
		}
		sink.Log(event.name, event.metadata)
	}

	return true
}

// Log records a synchronous event or queues it until a sink is attached.
func (l *Logger) Log(eventName string, metadata Metadata) {
	event := queuedEvent{
		name:     eventName,
		metadata: SanitizeMetadata(metadata),
	}

	l.mu.Lock()
	sink := l.sink
	if sink == nil {
		l.queue = append(l.queue, event)
		l.mu.Unlock()
		return
	}
	l.mu.Unlock()

	sink.Log(event.name, event.metadata)
}

// LogAsync records an asynchronous event. If no sink is attached yet, it queues
// the event and returns nil.
func (l *Logger) LogAsync(ctx context.Context, eventName string, metadata Metadata) error {
	event := queuedEvent{
		name:     eventName,
		metadata: SanitizeMetadata(metadata),
		async:    true,
	}

	l.mu.Lock()
	sink := l.sink
	if sink == nil {
		l.queue = append(l.queue, event)
		l.mu.Unlock()
		return nil
	}
	l.mu.Unlock()

	return sink.LogAsync(ctx, event.name, event.metadata)
}

// SanitizeMetadata returns a copy of metadata that is safe for general telemetry
// storage.
func SanitizeMetadata(metadata Metadata) Metadata {
	if len(metadata) == 0 {
		return Metadata{}
	}

	sanitized := make(Metadata, len(metadata))
	for key, value := range metadata {
		if shouldDropMetadataKey(key) {
			continue
		}

		switch typed := value.(type) {
		case bool:
			sanitized[key] = typed
		case int:
			sanitized[key] = typed
		case int8:
			sanitized[key] = typed
		case int16:
			sanitized[key] = typed
		case int32:
			sanitized[key] = typed
		case int64:
			sanitized[key] = typed
		case uint:
			sanitized[key] = typed
		case uint8:
			sanitized[key] = typed
		case uint16:
			sanitized[key] = typed
		case uint32:
			sanitized[key] = typed
		case uint64:
			sanitized[key] = typed
		case float32:
			if !math.IsNaN(float64(typed)) && !math.IsInf(float64(typed), 0) {
				sanitized[key] = typed
			}
		case float64:
			if !math.IsNaN(typed) && !math.IsInf(typed, 0) {
				sanitized[key] = typed
			}
		case SafeString:
			sanitized[key] = string(typed)
		}
	}

	return sanitized
}

func shouldDropMetadataKey(key string) bool {
	if strings.HasPrefix(key, "_PROTO_") {
		return true
	}

	normalized := strings.ToLower(strings.TrimSpace(key))
	normalized = strings.ReplaceAll(normalized, "-", "_")
	normalized = strings.ReplaceAll(normalized, " ", "_")

	if strings.HasPrefix(normalized, "proto_") {
		return true
	}

	return sensitiveMetadataKeys[normalized]
}

var sensitiveMetadataKeys = map[string]bool{
	"api_key":       true,
	"apikey":        true,
	"auth":          true,
	"authorization": true,
	"code":          true,
	"content":       true,
	"email":         true,
	"file":          true,
	"file_name":     true,
	"file_path":     true,
	"filename":      true,
	"filepath":      true,
	"input":         true,
	"message":       true,
	"name":          true,
	"output":        true,
	"password":      true,
	"path":          true,
	"prompt":        true,
	"secret":        true,
	"source_code":   true,
	"sourcecode":    true,
	"snippet":       true,
	"token":         true,
	"uri":           true,
	"url":           true,
	"user":          true,
	"user_name":     true,
	"username":      true,
}
