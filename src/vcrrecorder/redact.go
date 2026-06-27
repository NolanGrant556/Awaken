package vcrrecorder

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"time"
)

var (
	secretKeyPattern    = regexp.MustCompile(`(?i)(api[-_]?key|authorization|auth[-_]?token|access[-_]?token|refresh[-_]?token|password|passwd|secret|credential)`)
	secretValuePatterns = []*regexp.Regexp{
		regexp.MustCompile(`sk-[A-Za-z0-9_-]{8,}`),
		regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._~+/=-]{8,}`),
		regexp.MustCompile(`(?i)(api[-_]?key|token|password|secret)\s*[:=]\s*["']?[^"',\s}]+`),
	}
)

func preparePayload(payload any, maxPayloadBytes int, disableRedact bool) (any, *PayloadMeta, error) {
	if payload == nil {
		return nil, nil, nil
	}

	meta := &PayloadMeta{}
	safe := sanitizeValue(payload, !disableRedact, meta)
	data, err := json.Marshal(safe)
	if err != nil {
		return nil, nil, err
	}

	if maxPayloadBytes > 0 && len(data) > maxPayloadBytes {
		sum := sha256.Sum256(data)
		hash := hex.EncodeToString(sum[:])
		marker := map[string]any{
			"kind":           MarkerKindPayloadTooLarge,
			"original_bytes": len(data),
			"sha256":         hash,
		}
		markerBytes, err := json.Marshal(marker)
		if err != nil {
			return nil, nil, err
		}
		meta.Capped = true
		meta.OriginalBytes = len(data)
		meta.StoredBytes = len(markerBytes)
		meta.SHA256 = hash
		return marker, compactMeta(meta), nil
	}

	meta.StoredBytes = len(data)
	return safe, compactMeta(meta), nil
}

func compactMeta(meta *PayloadMeta) *PayloadMeta {
	if meta == nil {
		return nil
	}
	if !meta.Redacted && !meta.Capped && !meta.Media {
		return nil
	}
	return meta
}

func sanitizeValue(value any, redact bool, meta *PayloadMeta) any {
	switch typed := value.(type) {
	case nil:
		return nil
	case string:
		return redactString(typed, redact, meta)
	case []byte:
		meta.Media = true
		return mediaMarker("application/octet-stream", "", typed)
	case MediaPayload:
		meta.Media = true
		return mediaMarker(typed.MIMEType, typed.Name, typed.Data)
	case *MediaPayload:
		if typed == nil {
			return nil
		}
		meta.Media = true
		return mediaMarker(typed.MIMEType, typed.Name, typed.Data)
	case map[string]any:
		return sanitizeStringMap(typed, redact, meta)
	}

	return sanitizeReflect(reflect.ValueOf(value), redact, meta)
}

func sanitizeReflect(value reflect.Value, redact bool, meta *PayloadMeta) any {
	if !value.IsValid() {
		return nil
	}
	for value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface {
		if value.IsNil() {
			return nil
		}
		value = value.Elem()
	}
	if value.CanInterface() {
		if timestamp, ok := value.Interface().(time.Time); ok {
			return timestamp.UTC().Format(time.RFC3339Nano)
		}
	}

	switch value.Kind() {
	case reflect.String:
		return redactString(value.String(), redact, meta)
	case reflect.Bool:
		return value.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return value.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return value.Uint()
	case reflect.Float32, reflect.Float64:
		return value.Float()
	case reflect.Slice, reflect.Array:
		if value.Type().Elem().Kind() == reflect.Uint8 {
			data := make([]byte, value.Len())
			reflect.Copy(reflect.ValueOf(data), value)
			meta.Media = true
			return mediaMarker("application/octet-stream", "", data)
		}
		out := make([]any, 0, value.Len())
		for i := 0; i < value.Len(); i++ {
			out = append(out, sanitizeReflect(value.Index(i), redact, meta))
		}
		return out
	case reflect.Map:
		out := make(map[string]any, value.Len())
		iter := value.MapRange()
		for iter.Next() {
			key := fmt.Sprint(iter.Key().Interface())
			if redact && secretKeyPattern.MatchString(key) {
				meta.Redacted = true
				out[key] = RedactedValue
				continue
			}
			out[key] = sanitizeReflect(iter.Value(), redact, meta)
		}
		return out
	case reflect.Struct:
		if value.CanInterface() {
			var decoded any
			if data, err := json.Marshal(value.Interface()); err == nil {
				if err := json.Unmarshal(data, &decoded); err == nil {
					return sanitizeValue(decoded, redact, meta)
				}
			}
		}
	}

	if value.CanInterface() {
		return fmt.Sprint(value.Interface())
	}
	return nil
}

func sanitizeStringMap(input map[string]any, redact bool, meta *PayloadMeta) map[string]any {
	out := make(map[string]any, len(input))
	for key, value := range input {
		if redact && secretKeyPattern.MatchString(key) {
			meta.Redacted = true
			out[key] = RedactedValue
			continue
		}
		out[key] = sanitizeValue(value, redact, meta)
	}
	return out
}

func redactString(value string, redact bool, meta *PayloadMeta) string {
	if !redact || value == "" {
		return value
	}

	redacted := value
	for _, pattern := range secretValuePatterns {
		redacted = pattern.ReplaceAllStringFunc(redacted, func(match string) string {
			meta.Redacted = true
			if strings.HasPrefix(strings.ToLower(match), "bearer ") {
				return "Bearer " + RedactedValue
			}
			if strings.Contains(match, "=") {
				parts := strings.SplitN(match, "=", 2)
				return strings.TrimSpace(parts[0]) + "=" + RedactedValue
			}
			if strings.Contains(match, ":") {
				parts := strings.SplitN(match, ":", 2)
				return strings.TrimSpace(parts[0]) + ": " + RedactedValue
			}
			return RedactedValue
		})
	}
	return redacted
}

func mediaMarker(mimeType, name string, data []byte) map[string]any {
	if strings.TrimSpace(mimeType) == "" {
		mimeType = "application/octet-stream"
	}
	sum := sha256.Sum256(data)
	marker := map[string]any{
		"kind":       MarkerKindMedia,
		"media_type": mimeType,
		"bytes":      len(data),
		"sha256":     hex.EncodeToString(sum[:]),
	}
	if strings.TrimSpace(name) != "" {
		marker["name"] = name
	}
	return marker
}
