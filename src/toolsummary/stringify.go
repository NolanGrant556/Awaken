package toolsummary

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
)

var (
	secretKeyFragments = []string{
		"api_key",
		"apikey",
		"authorization",
		"auth_token",
		"bearer",
		"client_secret",
		"password",
		"private_key",
		"refresh_token",
		"secret",
		"token",
	}
	bearerPattern     = regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._~+/=-]+`)
	credentialPattern = regexp.MustCompile(`(?i)(["']?\b(?:api[_-]?key|authorization|client[_-]?secret|password|refresh[_-]?token|secret|token)\b["']?\s*[:=]\s*["']?)[^"',}\]\s]+`)
	openAIKeyPattern  = regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{8,}\b`)
)

func StringifyValue(value any, opts Options) string {
	opts = opts.normalized()
	if value == nil {
		return ""
	}

	var rendered string
	func() {
		defer func() {
			if recover() != nil {
				rendered = "[unable to serialize]"
			}
		}()

		normalized := normalizeForStringify(reflect.ValueOf(value), map[uintptr]bool{}, 0, opts)
		if str, ok := normalized.(string); ok {
			rendered = str
			return
		}
		bytes, err := json.Marshal(normalized)
		if err != nil {
			rendered = fmt.Sprint(value)
			return
		}
		rendered = string(bytes)
	}()

	if !opts.DisableRedaction {
		rendered = redactString(rendered)
	}
	return truncate(rendered, opts.MaxValueChars)
}

func normalizeForStringify(value reflect.Value, seen map[uintptr]bool, depth int, opts Options) any {
	if !value.IsValid() {
		return nil
	}
	if depth > 20 {
		return "[max depth]"
	}

	if value.Kind() == reflect.Interface {
		if value.IsNil() {
			return nil
		}
		return normalizeForStringify(value.Elem(), seen, depth+1, opts)
	}

	if value.CanInterface() {
		switch v := value.Interface().(type) {
		case json.RawMessage:
			return string(v)
		case fmt.Stringer:
			if value.Kind() != reflect.Struct {
				return maybeRedactString(v.String(), opts)
			}
		}
	}

	switch value.Kind() {
	case reflect.String:
		return maybeRedactString(value.String(), opts)
	case reflect.Bool:
		return value.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return value.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return value.Uint()
	case reflect.Float32, reflect.Float64:
		return value.Float()
	case reflect.Pointer:
		if value.IsNil() {
			return nil
		}
		ptr := value.Pointer()
		if seen[ptr] {
			return "[cycle]"
		}
		seen[ptr] = true
		return normalizeForStringify(value.Elem(), seen, depth+1, opts)
	case reflect.Slice, reflect.Array:
		if value.Kind() == reflect.Slice && value.IsNil() {
			return nil
		}
		out := make([]any, 0, value.Len())
		for i := 0; i < value.Len(); i++ {
			out = append(out, normalizeForStringify(value.Index(i), seen, depth+1, opts))
		}
		return out
	case reflect.Map:
		if value.IsNil() {
			return nil
		}
		out := make(map[string]any, value.Len())
		keys := value.MapKeys()
		sort.Slice(keys, func(i, j int) bool {
			return fmt.Sprint(keys[i].Interface()) < fmt.Sprint(keys[j].Interface())
		})
		for _, key := range keys {
			keyString := fmt.Sprint(key.Interface())
			if !opts.DisableRedaction && isSecretKey(keyString) {
				out[keyString] = "[REDACTED]"
				continue
			}
			out[keyString] = normalizeForStringify(value.MapIndex(key), seen, depth+1, opts)
		}
		return out
	case reflect.Struct:
		if value.CanInterface() {
			if bytes, err := json.Marshal(value.Interface()); err == nil {
				var decoded any
				if json.Unmarshal(bytes, &decoded) == nil {
					return normalizeForStringify(reflect.ValueOf(decoded), seen, depth+1, opts)
				}
			}
			return fmt.Sprint(value.Interface())
		}
		return fmt.Sprint(value)
	case reflect.Invalid:
		return nil
	default:
		if value.CanInterface() {
			return fmt.Sprint(value.Interface())
		}
		return fmt.Sprint(value)
	}
}

func maybeRedactString(value string, opts Options) string {
	if opts.DisableRedaction {
		return value
	}
	return redactString(value)
}

func redactString(value string) string {
	value = bearerPattern.ReplaceAllString(value, "Bearer [REDACTED]")
	value = openAIKeyPattern.ReplaceAllString(value, "[REDACTED]")
	return credentialPattern.ReplaceAllString(value, "${1}[REDACTED]")
}

func isSecretKey(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "-", "_"), " ", "_"))
	for _, fragment := range secretKeyFragments {
		if strings.Contains(normalized, fragment) {
			return true
		}
	}
	return false
}

func truncate(value string, maxChars int) string {
	if maxChars <= 0 || len(value) <= maxChars {
		return value
	}
	if maxChars <= 3 {
		return value[:maxChars]
	}
	return strings.TrimSpace(value[:maxChars-3]) + "..."
}
