package agentcore

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"awaken/src/internal/outputlimit"
	"awaken/src/tool"
)

func normalizeArtifactStatus(status string) ArtifactStatus {
	switch ArtifactStatus(strings.TrimSpace(status)) {
	case ArtifactStatusPending, ArtifactStatusError:
		return ArtifactStatus(strings.TrimSpace(status))
	default:
		return ArtifactStatusReady
	}
}

func fallbackArtifactTitle(path string, toolName string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return fmt.Sprintf("%s artifact", toolName)
	}
	return filepath.Base(trimmed)
}

func excerptString(value string, max int) string {
	if max <= 0 {
		return value
	}
	if normalized, truncated := outputlimit.Prefix(value, max); !truncated {
		return normalized
	}
	prefix, _ := outputlimit.Prefix(value, max-1)
	return prefix + "…"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func timeNowUTC() time.Time {
	return time.Now().UTC()
}

func normalizeToolInputWithSchema(arguments string, selectedTool tool.Tool, schema tool.ToolParametersSchema, preserveObject bool) (string, error) {
	if strings.TrimSpace(schema.Type) == "" {
		schema = tool.ResolveInputSchema(selectedTool)
	}
	if err := tool.ValidateInputAgainstSchema(arguments, schema); err != nil {
		return "", err
	}

	// The immutable registry snapshot decides input shaping at registration.
	// Authored v2/recursive tools preserve the model-visible JSON object;
	// compatibility-only legacy tools retain Parameters()-based shaping.
	if preserveObject {
		trimmed := strings.TrimSpace(arguments)
		if trimmed == "" {
			trimmed = "{}"
		}
		var decoded map[string]any
		if err := json.Unmarshal([]byte(trimmed), &decoded); err != nil {
			return "", fmt.Errorf("arguments must be a JSON object: %w", err)
		}
		data, err := json.Marshal(decoded)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
	return normalizeToolInput(arguments, selectedTool.Parameters())
}

func normalizeToolInput(arguments string, params []tool.ToolParameter) (string, error) {
	trimmed := strings.TrimSpace(arguments)
	if len(params) == 0 {
		if trimmed == "" {
			return "", nil
		}
		if !json.Valid([]byte(trimmed)) {
			return "", fmt.Errorf("arguments must be valid JSON")
		}
		return trimmed, nil
	}
	if trimmed == "" {
		trimmed = "{}"
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal([]byte(trimmed), &decoded); err != nil {
		return "", fmt.Errorf("arguments must be a JSON object: %w", err)
	}
	if decoded == nil {
		return "", fmt.Errorf("arguments must be a JSON object")
	}

	for _, param := range params {
		if param.Required {
			if _, ok := decoded[param.Name]; !ok {
				return "", fmt.Errorf("missing required parameter %q", param.Name)
			}
		}
	}

	if len(params) == 1 {
		value, ok := decoded[params[0].Name]
		if !ok {
			return "", nil
		}
		if str, ok := value.(string); ok {
			return str, nil
		}
		data, err := json.Marshal(value)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}

	data, err := json.Marshal(decoded)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func argumentsForXMLFallback(toolName, input string) string {
	selectedTool, err := tool.GetTool(toolName)
	if err != nil {
		return input
	}

	params := selectedTool.Parameters()
	paramName := singleRequiredStringParam(params)
	if paramName == "" {
		return input
	}

	data, err := json.Marshal(map[string]interface{}{
		paramName: input,
	})
	if err != nil {
		return input
	}
	return string(data)
}

func singleRequiredStringParam(params []tool.ToolParameter) string {
	requiredName := ""
	for _, param := range params {
		if !param.Required {
			continue
		}
		if param.Type != "string" {
			return ""
		}
		if requiredName != "" {
			return ""
		}
		requiredName = param.Name
	}
	return requiredName
}
