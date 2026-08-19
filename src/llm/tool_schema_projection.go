package llm

import "awaken/src/tool"

// projectOpenAICompatibleToolParameters lowers Awaken's canonical tool schema
// into the stricter root-object subset accepted by OpenAI-compatible APIs.
// The canonical schema remains unchanged and is still used for pre-execution
// validation, so removing a root composition keyword here does not weaken the
// runtime contract.
func projectOpenAICompatibleToolParameters(schema tool.ToolParametersSchema) map[string]any {
	projected := tool.SchemaAsMap(schema)
	projected["type"] = "object"

	properties, _ := projected["properties"].(map[string]any)
	if properties == nil {
		properties = make(map[string]any)
	}
	for _, keyword := range []string{"oneOf", "anyOf", "allOf"} {
		mergeRootObjectProperties(properties, projected[keyword])
		delete(projected, keyword)
	}
	for _, keyword := range []string{"enum", "const", "not"} {
		delete(projected, keyword)
	}
	projected["properties"] = properties
	return projected
}

func mergeRootObjectProperties(target map[string]any, rawBranches any) {
	branches, ok := rawBranches.([]any)
	if !ok {
		return
	}
	for _, rawBranch := range branches {
		branch, ok := rawBranch.(map[string]any)
		if !ok {
			continue
		}
		branchProperties, ok := branch["properties"].(map[string]any)
		if !ok {
			continue
		}
		for name, property := range branchProperties {
			if _, exists := target[name]; !exists {
				target[name] = property
			}
		}
	}
}
