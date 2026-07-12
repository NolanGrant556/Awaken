package agui

import "encoding/json"

// applyExecutionMetadata copies the provider/environment facts shared by live
// ToolOutcome diagnostics and durable semantic payloads. Both snake_case
// (Go/Journals) and camelCase (already-normalized external providers) are
// accepted at this boundary; the Event serializer remains camelCase authority.
func applyExecutionMetadata(dst *Event, payload map[string]any) {
	if dst == nil || len(payload) == 0 {
		return
	}
	if value := firstStringValue(payload, "environment_snapshot_id", "environmentSnapshotId"); value != "" {
		dst.EnvironmentSnapshotID = value
	}
	if value := firstStringValue(payload, "tool_availability_snapshot_id", "toolAvailabilitySnapshotId", "tool_snapshot_id", "toolSnapshotId"); value != "" {
		dst.ToolAvailabilitySnapshotID = value
	}
	if value := firstStringValue(payload, "provider_id", "providerId"); value != "" {
		dst.ProviderID = value
	}
	if value := firstStringValue(payload, "capability"); value != "" {
		dst.Capability = value
	}
	if values := stringSliceFromAny(firstPresentValue(payload, "capabilities")); len(values) > 0 {
		dst.Capabilities = values
		if dst.Capability == "" {
			dst.Capability = values[0]
		}
	}
	if value := firstStringValue(payload, "readiness", "provider_readiness", "providerReadiness"); value != "" {
		dst.Readiness = value
	}
	if value := firstStringValue(payload, "side_effect_state", "sideEffectState"); value != "" {
		dst.SideEffectState = value
	}
	if dependency, ok := mapStringAnyValue(firstPresentValue(payload, "dependency")); ok {
		dst.Dependency = dependency
	}
	if dependencies, ok := mapStringAnySliceValue(firstPresentValue(
		payload,
		"dependencies",
		"dependency_observations",
		"dependencyObservations",
		"dependency_requirements",
		"dependencyRequirements",
	)); ok {
		dst.Dependencies = dependencies
	}
	if operation, ok := mapStringAnyValue(firstPresentValue(payload, "operation")); ok {
		if value := firstStringValue(operation, "side_effect_state", "sideEffectState"); value != "" {
			dst.SideEffectState = value
		}
	}
}

func firstStringValue(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := payload[key]
		if !ok {
			continue
		}
		if stringValue, ok := value.(string); ok && stringValue != "" {
			return stringValue
		}
		raw, err := json.Marshal(value)
		if err != nil {
			continue
		}
		var stringValue string
		if err := json.Unmarshal(raw, &stringValue); err == nil && stringValue != "" {
			return stringValue
		}
	}
	return ""
}

func firstPresentValue(payload map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := payload[key]; ok {
			return value
		}
	}
	return nil
}

func mapStringAnyValue(value any) (map[string]any, bool) {
	if value == nil {
		return nil, false
	}
	if typed, ok := value.(map[string]any); ok {
		return cloneStringAnyMap(typed), true
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, false
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, false
	}
	return decoded, true
}

func mapStringAnySliceValue(value any) ([]map[string]any, bool) {
	if value == nil {
		return nil, false
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, false
	}
	var decoded []map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, false
	}
	return decoded, true
}

func cloneStringAnyMap(value map[string]any) map[string]any {
	cloned := make(map[string]any, len(value))
	for key, item := range value {
		cloned[key] = item
	}
	return cloned
}
