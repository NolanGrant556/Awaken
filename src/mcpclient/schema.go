package mcpclient

import (
	"sort"
	"strconv"

	"awaken/src/tool"
)

func ToolDefinitionForRemote(serverName string, remote RemoteTool, config ServerConfig) tool.ToolDefinition {
	return tool.ToolDefinition{
		Type: "function",
		Function: tool.ToolFunctionSchema{
			Name:        BuildToolName(serverName, remote.Name),
			Description: CapDescription(RedactConfigSecrets(remote.Description, config)),
			Parameters:  ParametersFromInputSchema(remote.InputSchema),
		},
	}
}

func ParametersFromInputSchema(schema map[string]any) tool.ToolParametersSchema {
	params := tool.ToolParametersSchema{
		Type:       "object",
		Properties: map[string]tool.ToolParam{},
	}
	if len(schema) == 0 {
		return params
	}

	properties, _ := schema["properties"].(map[string]any)
	required := requiredNames(schema["required"])
	requiredSet := make(map[string]bool, len(required))
	for _, name := range required {
		requiredSet[name] = true
	}

	keys := make([]string, 0, len(properties))
	for key := range properties {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	nameMap := normalizedParameterNames(keys)
	for _, originalName := range keys {
		safeName := nameMap[originalName]
		paramSchema, _ := properties[originalName].(map[string]any)
		params.Properties[safeName] = tool.ToolParam{
			Type:        jsonSchemaType(paramSchema),
			Description: CapDescription(stringField(paramSchema, "description")),
		}
	}

	for _, originalName := range required {
		safeName := nameMap[originalName]
		if _, exists := params.Properties[safeName]; exists && requiredSet[originalName] {
			params.Required = append(params.Required, safeName)
		}
	}
	return params
}

func ParameterNameMap(schema map[string]any) map[string]string {
	properties, _ := schema["properties"].(map[string]any)
	if len(properties) == 0 {
		return nil
	}
	keys := make([]string, 0, len(properties))
	for originalName := range properties {
		keys = append(keys, originalName)
	}
	sort.Strings(keys)

	result := make(map[string]string, len(properties))
	for originalName, safeName := range normalizedParameterNames(keys) {
		if safeName != originalName {
			result[safeName] = originalName
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func normalizedParameterNames(originalNames []string) map[string]string {
	result := make(map[string]string, len(originalNames))
	used := make(map[string]struct{}, len(originalNames))
	for _, originalName := range originalNames {
		base := ProviderSafeName(originalName)
		safeName := base
		for i := 2; ; i++ {
			if _, exists := used[safeName]; !exists {
				break
			}
			suffix := "_" + shortHash(originalName)
			if i > 2 {
				suffix = "_" + strconv.Itoa(i) + "_" + shortHash(originalName)
			}
			safeName = truncateWithHash(base, MaxProviderNameLen-len(suffix)) + suffix
			if len(safeName) > MaxProviderNameLen {
				safeName = truncateWithHash(safeName, MaxProviderNameLen)
			}
		}
		used[safeName] = struct{}{}
		result[originalName] = safeName
	}
	return result
}

func requiredNames(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if name, ok := item.(string); ok {
				result = append(result, name)
			}
		}
		return result
	default:
		return nil
	}
}

func jsonSchemaType(schema map[string]any) string {
	if len(schema) == 0 {
		return "string"
	}
	if value, ok := schema["type"].(string); ok && isSupportedToolParamType(value) {
		return value
	}
	if values, ok := schema["type"].([]any); ok {
		for _, value := range values {
			if s, ok := value.(string); ok && isSupportedToolParamType(s) {
				return s
			}
		}
	}
	if _, ok := schema["properties"].(map[string]any); ok {
		return "object"
	}
	if _, ok := schema["items"]; ok {
		return "array"
	}
	return "string"
}

func stringField(source map[string]any, key string) string {
	if source == nil {
		return ""
	}
	value, _ := source[key].(string)
	return value
}

func isSupportedToolParamType(value string) bool {
	switch value {
	case "string", "number", "integer", "boolean", "object", "array":
		return true
	default:
		return false
	}
}
