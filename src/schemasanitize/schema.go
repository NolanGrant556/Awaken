package schemasanitize

// Tool is the minimal tool shape sanitized by this package.
//
// Parameters is intentionally any so callers can pass malformed or missing
// parameter schemas and still receive a valid object schema back.
type Tool struct {
	Name       string `json:"name,omitempty"`
	Parameters any    `json:"parameters,omitempty"`
}

// SanitizeTool returns a deep-copied tool whose Parameters field is always an
// object JSON schema with a properties map.
func SanitizeTool(tool Tool) Tool {
	out := Tool{
		Name:       tool.Name,
		Parameters: deepCopy(tool.Parameters),
	}

	params, ok := asMap(tool.Parameters)
	if !ok {
		out.Parameters = emptyObjectSchema()
		return out
	}

	sanitized, ok := SanitizeSchema(params).(map[string]any)
	if !ok {
		out.Parameters = emptyObjectSchema()
		return out
	}

	ensureObjectSchema(sanitized)
	pruneRequired(sanitized)
	out.Parameters = sanitized
	return out
}

// SanitizeSchema returns a deep-copied JSON-schema fragment with known
// provider-hostile shapes normalized.
func SanitizeSchema(schema any) any {
	return StripNullableUnions(sanitizeNode(deepCopy(schema)), true)
}

// StripNullableUnions collapses anyOf/oneOf unions that only add a null branch.
// It preserves outer metadata on the surviving non-null schema.
func StripNullableUnions(schema any, keepNullableHint bool) any {
	return stripNullableUnions(deepCopy(schema), keepNullableHint)
}

func sanitizeNode(node any) any {
	if s, ok := node.(string); ok {
		if s == "object" {
			return emptyObjectSchema()
		}
		return map[string]any{"type": s}
	}

	if items, ok := asSlice(node); ok {
		out := make([]any, 0, len(items))
		for _, item := range items {
			out = append(out, sanitizeNode(item))
		}
		return out
	}

	nodeMap, ok := asMap(node)
	if !ok {
		return node
	}

	out := make(map[string]any, len(nodeMap))
	for key, value := range nodeMap {
		switch key {
		case "type":
			if normalized, handled := normalizeTypeArray(value); handled {
				for normalizedKey, normalizedValue := range normalized {
					out[normalizedKey] = normalizedValue
				}
				continue
			}
			out[key] = deepCopy(value)
		case "properties", "$defs", "definitions":
			if properties, ok := asMap(value); ok {
				sanitized := make(map[string]any, len(properties))
				for name, propertySchema := range properties {
					sanitized[name] = sanitizeNode(propertySchema)
				}
				out[key] = sanitized
				continue
			}
			out[key] = deepCopy(value)
		case "items", "additionalProperties":
			if _, ok := value.(bool); ok {
				out[key] = value
				continue
			}
			out[key] = sanitizeNode(value)
		case "allOf", "anyOf", "oneOf":
			if variants, ok := asSlice(value); ok {
				sanitized := make([]any, 0, len(variants))
				for _, variant := range variants {
					sanitized = append(sanitized, sanitizeNode(variant))
				}
				out[key] = sanitized
				continue
			}
			out[key] = deepCopy(value)
		case "required", "enum", "examples", "default", "const":
			out[key] = deepCopy(value)
		default:
			if _, ok := asMap(value); ok {
				out[key] = sanitizeNode(value)
				continue
			}
			if _, ok := asSlice(value); ok {
				out[key] = sanitizeNode(value)
				continue
			}
			out[key] = value
		}
	}

	if out["type"] == "object" {
		ensureObjectSchema(out)
		pruneRequired(out)
	}

	return out
}

func stripNullableUnions(schema any, keepNullableHint bool) any {
	if items, ok := asSlice(schema); ok {
		out := make([]any, 0, len(items))
		for _, item := range items {
			out = append(out, stripNullableUnions(item, keepNullableHint))
		}
		return out
	}

	nodeMap, ok := asMap(schema)
	if !ok {
		return schema
	}

	out := make(map[string]any, len(nodeMap))
	for key, value := range nodeMap {
		switch key {
		case "properties", "$defs", "definitions":
			if schemas, ok := asMap(value); ok {
				sanitized := make(map[string]any, len(schemas))
				for name, child := range schemas {
					sanitized[name] = stripNullableUnions(child, keepNullableHint)
				}
				out[key] = sanitized
				continue
			}
			out[key] = deepCopy(value)
		case "items", "additionalProperties":
			if _, ok := value.(bool); ok {
				out[key] = value
				continue
			}
			out[key] = stripNullableUnions(value, keepNullableHint)
		case "allOf", "anyOf", "oneOf":
			if variants, ok := asSlice(value); ok {
				sanitized := make([]any, 0, len(variants))
				for _, variant := range variants {
					sanitized = append(sanitized, stripNullableUnions(variant, keepNullableHint))
				}
				out[key] = sanitized
				continue
			}
			out[key] = deepCopy(value)
		case "required", "enum", "examples", "default", "const":
			out[key] = deepCopy(value)
		default:
			if _, ok := asMap(value); ok {
				out[key] = stripNullableUnions(value, keepNullableHint)
				continue
			}
			if _, ok := asSlice(value); ok {
				out[key] = stripNullableUnions(value, keepNullableHint)
				continue
			}
			out[key] = value
		}
	}

	for _, unionKey := range []string{"anyOf", "oneOf"} {
		variants, ok := asSlice(out[unionKey])
		if !ok {
			continue
		}

		nonNull := make([]any, 0, len(variants))
		droppedNull := false
		for _, variant := range variants {
			if isNullSchema(variant) {
				droppedNull = true
				continue
			}
			nonNull = append(nonNull, variant)
		}

		if droppedNull && len(nonNull) == 1 {
			replacement := schemaMapForReplacement(nonNull[0])
			if keepNullableHint {
				if _, exists := replacement["nullable"]; !exists {
					replacement["nullable"] = true
				}
			}
			for _, metadataKey := range []string{"title", "description", "default", "examples"} {
				if metadataValue, exists := out[metadataKey]; exists {
					if _, alreadySet := replacement[metadataKey]; !alreadySet {
						replacement[metadataKey] = deepCopy(metadataValue)
					}
				}
			}
			return stripNullableUnions(replacement, keepNullableHint)
		}
	}

	return out
}

func normalizeTypeArray(value any) (map[string]any, bool) {
	values, ok := asSlice(value)
	if !ok {
		return nil, false
	}

	var firstNonNull string
	nonNullCount := 0
	sawNull := false
	for _, item := range values {
		itemString, ok := item.(string)
		if !ok {
			continue
		}
		if itemString == "null" {
			sawNull = true
			continue
		}
		if firstNonNull == "" {
			firstNonNull = itemString
		}
		nonNullCount++
	}

	if firstNonNull == "" {
		return map[string]any{"type": "object"}, true
	}

	out := map[string]any{"type": firstNonNull}
	if sawNull && nonNullCount == 1 {
		out["nullable"] = true
	}
	return out, true
}

func schemaMapForReplacement(value any) map[string]any {
	if schema, ok := asMap(value); ok {
		out := make(map[string]any, len(schema))
		for key, child := range schema {
			out[key] = deepCopy(child)
		}
		return out
	}

	sanitized := sanitizeNode(value)
	if schema, ok := asMap(sanitized); ok {
		return schema
	}

	return emptyObjectSchema()
}

func isNullSchema(value any) bool {
	if value == "null" {
		return true
	}
	schema, ok := asMap(value)
	if !ok {
		return false
	}
	return schema["type"] == "null"
}

func ensureObjectSchema(schema map[string]any) {
	schema["type"] = "object"
	if _, ok := asMap(schema["properties"]); !ok {
		schema["properties"] = map[string]any{}
	}
}

func pruneRequired(schema map[string]any) {
	required, ok := stringList(schema["required"])
	if !ok {
		return
	}
	properties, ok := asMap(schema["properties"])
	if !ok {
		return
	}

	valid := make([]string, 0, len(required))
	for _, name := range required {
		if _, exists := properties[name]; exists {
			valid = append(valid, name)
		}
	}

	if len(valid) == 0 {
		delete(schema, "required")
		return
	}
	if len(valid) != len(required) {
		schema["required"] = valid
	}
}

func emptyObjectSchema() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}

func asMap(value any) (map[string]any, bool) {
	switch typed := value.(type) {
	case map[string]any:
		return typed, true
	case map[string]string:
		out := make(map[string]any, len(typed))
		for key, child := range typed {
			out[key] = child
		}
		return out, true
	default:
		return nil, false
	}
}

func asSlice(value any) ([]any, bool) {
	switch typed := value.(type) {
	case []any:
		return typed, true
	case []string:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, item)
		}
		return out, true
	default:
		return nil, false
	}
}

func stringList(value any) ([]string, bool) {
	switch typed := value.(type) {
	case []string:
		out := make([]string, len(typed))
		copy(out, typed)
		return out, true
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			itemString, ok := item.(string)
			if !ok {
				continue
			}
			out = append(out, itemString)
		}
		return out, true
	default:
		return nil, false
	}
}

func deepCopy(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, child := range typed {
			out[key] = deepCopy(child)
		}
		return out
	case map[string]string:
		out := make(map[string]string, len(typed))
		for key, child := range typed {
			out[key] = child
		}
		return out
	case []any:
		out := make([]any, 0, len(typed))
		for _, child := range typed {
			out = append(out, deepCopy(child))
		}
		return out
	case []string:
		out := make([]string, len(typed))
		copy(out, typed)
		return out
	default:
		return value
	}
}
