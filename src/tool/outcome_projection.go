package tool

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"regexp"
	"sort"
	"unicode/utf8"
)

var sensitiveKeyPattern = regexp.MustCompile(`(?i)(authorization|api[_-]?key|access[_-]?token|refresh[_-]?token|password|passwd|secret|cookie|credential)`)

const (
	// modelStructuredFieldBudgetBytes is the maximum encoded size of each
	// structured projection added to a model-facing tool result. Keeping the
	// budget at the field boundary prevents one MCP StructuredContent value (or
	// a tool-specific details map) from bypassing the ordinary content limit.
	modelStructuredFieldBudgetBytes = 8 * 1024
	modelProjectionMaxDepth         = 64
	modelContentBudgetRunes         = 12 * 1024
	modelErrorMessageBudgetRunes    = 4 * 1024
	modelSummaryBudgetRunes         = 512
	modelPatchSummaryBudgetRunes    = 2 * 1024
	modelOperationTypeBudgetRunes   = 256
	modelOperationTargetBudgetRunes = 2 * 1024
)

type modelProjectionVisit struct {
	kind reflect.Kind
	ptr  uintptr
}

// safeModelValue recursively removes values carried under secret-bearing
// keys. It is deliberately key-based: tool-specific values should use typed,
// non-secret details instead of relying on heuristic text scrubbing.
func safeModelValue(value any) any {
	return safeModelReflectValue(reflect.ValueOf(value), make(map[modelProjectionVisit]bool), 0)
}

func safeModelReflectValue(value reflect.Value, stack map[modelProjectionVisit]bool, depth int) any {
	if !value.IsValid() {
		return nil
	}
	if depth >= modelProjectionMaxDepth {
		return "[MAX_DEPTH_EXCEEDED]"
	}

	for value.Kind() == reflect.Interface {
		if value.IsNil() {
			return nil
		}
		value = value.Elem()
	}

	switch value.Kind() {
	case reflect.Pointer:
		if value.IsNil() {
			return nil
		}
		visit := modelProjectionVisit{kind: value.Kind(), ptr: value.Pointer()}
		if stack[visit] {
			return "[CIRCULAR_REFERENCE]"
		}
		stack[visit] = true
		defer delete(stack, visit)
		return safeModelReflectValue(value.Elem(), stack, depth+1)
	case reflect.Map:
		if value.IsNil() {
			return nil
		}
		visit := modelProjectionVisit{kind: value.Kind(), ptr: value.Pointer()}
		if stack[visit] {
			return "[CIRCULAR_REFERENCE]"
		}
		stack[visit] = true
		defer delete(stack, visit)

		if value.Type().Key().Kind() != reflect.String {
			return fmt.Sprintf("[UNSUPPORTED_MAP_KEY:%s]", value.Type().Key())
		}
		keys := value.MapKeys()
		sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })
		result := make(map[string]any, len(keys))
		for _, mapKey := range keys {
			key := mapKey.String()
			if sensitiveKeyPattern.MatchString(key) {
				result[key] = "[REDACTED]"
				continue
			}
			result[key] = safeModelReflectValue(value.MapIndex(mapKey), stack, depth+1)
		}
		return result
	case reflect.Slice:
		if value.IsNil() {
			return nil
		}
		visit := modelProjectionVisit{kind: value.Kind(), ptr: value.Pointer()}
		if stack[visit] {
			return "[CIRCULAR_REFERENCE]"
		}
		stack[visit] = true
		defer delete(stack, visit)
		result := make([]any, value.Len())
		for i := 0; i < value.Len(); i++ {
			result[i] = safeModelReflectValue(value.Index(i), stack, depth+1)
		}
		return result
	case reflect.Array:
		result := make([]any, value.Len())
		for i := 0; i < value.Len(); i++ {
			result[i] = safeModelReflectValue(value.Index(i), stack, depth+1)
		}
		return result
	case reflect.Struct:
		if !value.CanInterface() {
			return fmt.Sprintf("[UNSUPPORTED_TYPE:%s]", value.Type())
		}
		encoded, err := json.Marshal(value.Interface())
		if err != nil {
			return fmt.Sprintf("[UNSUPPORTED_TYPE:%s]", value.Type())
		}
		var decoded any
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			return fmt.Sprintf("[UNSUPPORTED_TYPE:%s]", value.Type())
		}
		return safeModelReflectValue(reflect.ValueOf(decoded), stack, depth+1)
	case reflect.Bool:
		return value.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return value.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return value.Uint()
	case reflect.Float32, reflect.Float64:
		floating := value.Float()
		if math.IsInf(floating, 0) || math.IsNaN(floating) {
			return fmt.Sprintf("[UNSUPPORTED_FLOAT:%v]", floating)
		}
		return floating
	case reflect.String:
		return value.String()
	case reflect.Invalid:
		return nil
	default:
		return fmt.Sprintf("[UNSUPPORTED_TYPE:%s]", value.Type())
	}
}

// boundedSafeModelValue applies recursive redaction and then enforces an
// encoded-byte budget. Oversized values remain valid JSON and advertise that
// only a preview was projected; the original outcome is never rewritten.
func boundedSafeModelValue(value any, budget int) (any, bool) {
	safe := safeModelValue(value)
	encoded, err := json.Marshal(safe)
	if err != nil {
		encoded = []byte(fmt.Sprintf("[PROJECTION_ERROR:%s]", err))
	}
	if len(encoded) <= budget {
		return safe, false
	}
	return truncatedModelProjection(encoded, budget), true
}

func truncatedModelProjection(encoded []byte, budget int) map[string]any {
	result := map[string]any{
		"truncated":      true,
		"original_bytes": len(encoded),
		"preview":        "",
	}
	if budget <= 0 {
		return result
	}

	// JSON escaping can make the encoded preview larger than its source bytes.
	// Binary search the largest UTF-8-safe prefix whose wrapper stays in budget.
	low, high := 0, len(encoded)
	for low <= high {
		mid := low + (high-low)/2
		candidate := truncateUTF8Bytes(encoded, mid)
		result["preview"] = candidate
		wrapped, err := json.Marshal(result)
		if err == nil && len(wrapped) <= budget {
			low = mid + 1
			continue
		}
		high = mid - 1
	}
	result["preview"] = truncateUTF8Bytes(encoded, high)
	return result
}

func truncateUTF8Bytes(value []byte, limit int) string {
	if limit <= 0 {
		return ""
	}
	if limit >= len(value) {
		return string(value)
	}
	end := limit
	for end > 0 && !utf8.Valid(value[:end]) {
		end--
	}
	return string(value[:end])
}

func boundedModelText(value string, maxRunes int) (string, bool) {
	runes := []rune(value)
	if maxRunes <= 0 {
		return "", len(runes) > 0
	}
	if len(runes) <= maxRunes {
		return value, false
	}
	notice := []rune(fmt.Sprintf("\n[... omitted %d chars ...]\n", len(runes)-maxRunes))
	if len(notice) >= maxRunes {
		return string(runes[:maxRunes-1]) + "…", true
	}
	remaining := maxRunes - len(notice)
	head := remaining / 2
	tail := remaining - head
	return string(runes[:head]) + string(notice) + string(runes[len(runes)-tail:]), true
}

// cloneExecutionOutcome prevents projection/normalization from mutating maps,
// slices, or pointers owned by the caller. Structured arbitrary values are
// copied into JSON-compatible containers so nested typed maps and slices do
// not retain shared backing storage.
func cloneExecutionOutcome(outcome ExecutionOutcome) ExecutionOutcome {
	clone := outcome
	clone.ExitCode = cloneProjectionInt(outcome.ExitCode)
	clone.RecoveryOptions = append([]string(nil), outcome.RecoveryOptions...)
	clone.Diagnostic = cloneProjectionMap(outcome.Diagnostic)
	clone.AffectedPaths = append([]string(nil), outcome.AffectedPaths...)
	clone.Artifacts = append([]ArtifactDescriptor(nil), outcome.Artifacts...)
	clone.ModelContent = CloneModelContent(outcome.ModelContent)
	if outcome.Operation != nil {
		operation := *outcome.Operation
		clone.Operation = &operation
	}
	if outcome.Changes != nil {
		clone.Changes = make([]ResourceChange, len(outcome.Changes))
		for i, change := range outcome.Changes {
			clone.Changes[i] = change
			clone.Changes[i].Details = cloneProjectionMap(change.Details)
		}
	}
	if outcome.Environment != nil {
		environment := *outcome.Environment
		environment.Files = append([]string(nil), outcome.Environment.Files...)
		environment.Unobserved = append([]string(nil), outcome.Environment.Unobserved...)
		environment.Variables = append([]EnvironmentVariableChange(nil), outcome.Environment.Variables...)
		if outcome.Environment.Git != nil {
			git := *outcome.Environment.Git
			git.ModifiedFiles = append([]string(nil), outcome.Environment.Git.ModifiedFiles...)
			git.UntrackedFiles = append([]string(nil), outcome.Environment.Git.UntrackedFiles...)
			environment.Git = &git
		}
		if outcome.Environment.Processes != nil {
			environment.Processes = make([]ProcessObservation, len(outcome.Environment.Processes))
			for i, process := range outcome.Environment.Processes {
				environment.Processes[i] = process
				environment.Processes[i].ExitCode = cloneProjectionInt(process.ExitCode)
			}
		}
		clone.Environment = &environment
	}
	clone.Warnings = append([]OutcomeNotice(nil), outcome.Warnings...)
	if outcome.Errors != nil {
		clone.Errors = make([]OutcomeError, len(outcome.Errors))
		for i, outcomeError := range outcome.Errors {
			clone.Errors[i] = outcomeError
			clone.Errors[i].PossibleFixes = append([]string(nil), outcomeError.PossibleFixes...)
		}
	}
	if outcome.NextActions != nil {
		clone.NextActions = make([]NextAction, len(outcome.NextActions))
		for i, action := range outcome.NextActions {
			clone.NextActions[i] = action
			clone.NextActions[i].Preconditions = append([]string(nil), action.Preconditions...)
			clone.NextActions[i].InputHint = cloneProjectionMap(action.InputHint)
		}
	}
	clone.Details = cloneProjectionMap(outcome.Details)
	return clone
}

func cloneProjectionMap(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	cloned := cloneProjectionReflectValue(reflect.ValueOf(value), make(map[modelProjectionVisit]bool), 0)
	result, ok := cloned.(map[string]any)
	if !ok {
		return map[string]any{"value": cloned}
	}
	return result
}

func cloneProjectionReflectValue(value reflect.Value, stack map[modelProjectionVisit]bool, depth int) any {
	if !value.IsValid() {
		return nil
	}
	if depth >= modelProjectionMaxDepth {
		return "[MAX_DEPTH_EXCEEDED]"
	}
	for value.Kind() == reflect.Interface {
		if value.IsNil() {
			return nil
		}
		value = value.Elem()
	}
	switch value.Kind() {
	case reflect.Pointer:
		if value.IsNil() {
			return nil
		}
		visit := modelProjectionVisit{kind: value.Kind(), ptr: value.Pointer()}
		if stack[visit] {
			return "[CIRCULAR_REFERENCE]"
		}
		stack[visit] = true
		defer delete(stack, visit)
		return cloneProjectionReflectValue(value.Elem(), stack, depth+1)
	case reflect.Map:
		if value.IsNil() {
			return nil
		}
		if value.Type().Key().Kind() != reflect.String {
			return fmt.Sprintf("[UNSUPPORTED_MAP_KEY:%s]", value.Type().Key())
		}
		visit := modelProjectionVisit{kind: value.Kind(), ptr: value.Pointer()}
		if stack[visit] {
			return "[CIRCULAR_REFERENCE]"
		}
		stack[visit] = true
		defer delete(stack, visit)
		result := make(map[string]any, value.Len())
		iterator := value.MapRange()
		for iterator.Next() {
			result[iterator.Key().String()] = cloneProjectionReflectValue(iterator.Value(), stack, depth+1)
		}
		return result
	case reflect.Slice:
		if value.IsNil() {
			return nil
		}
		visit := modelProjectionVisit{kind: value.Kind(), ptr: value.Pointer()}
		if stack[visit] {
			return "[CIRCULAR_REFERENCE]"
		}
		stack[visit] = true
		defer delete(stack, visit)
		result := make([]any, value.Len())
		for i := 0; i < value.Len(); i++ {
			result[i] = cloneProjectionReflectValue(value.Index(i), stack, depth+1)
		}
		return result
	case reflect.Array:
		result := make([]any, value.Len())
		for i := 0; i < value.Len(); i++ {
			result[i] = cloneProjectionReflectValue(value.Index(i), stack, depth+1)
		}
		return result
	case reflect.Struct:
		if !value.CanInterface() {
			return fmt.Sprintf("[UNSUPPORTED_TYPE:%s]", value.Type())
		}
		encoded, err := json.Marshal(value.Interface())
		if err != nil {
			return fmt.Sprintf("[UNSUPPORTED_TYPE:%s]", value.Type())
		}
		var decoded any
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			return fmt.Sprintf("[UNSUPPORTED_TYPE:%s]", value.Type())
		}
		return cloneProjectionReflectValue(reflect.ValueOf(decoded), stack, depth+1)
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64, reflect.String:
		if value.CanInterface() {
			return value.Interface()
		}
		return fmt.Sprintf("[UNSUPPORTED_TYPE:%s]", value.Type())
	case reflect.Invalid:
		return nil
	default:
		return fmt.Sprintf("[UNSUPPORTED_TYPE:%s]", value.Type())
	}
}

func cloneProjectionInt(value *int) *int {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func safeResourceChanges(changes []ResourceChange) []map[string]any {
	result := make([]map[string]any, 0, len(changes))
	for _, change := range changes {
		item := map[string]any{
			"kind":          change.Kind,
			"action":        change.Action,
			"resource":      change.Resource,
			"certainty":     change.Certainty,
			"reversibility": change.Reversibility,
		}
		if change.Summary != "" {
			item["summary"] = change.Summary
		}
		if len(change.Details) > 0 {
			item["details"] = safeModelValue(change.Details)
		}
		result = append(result, item)
	}
	return result
}

// compactModelArtifacts intentionally excludes ArtifactDescriptor.Content;
// previews may be large or secret-bearing and the canonical content field is
// already subject to the tool-result limit.
func compactModelArtifacts(artifacts []ArtifactDescriptor) []map[string]any {
	result := make([]map[string]any, 0, len(artifacts))
	for _, artifact := range artifacts {
		item := map[string]any{
			"contract_version": artifact.ContractVersion,
			"role":             artifact.Role,
			"producer":         artifact.Producer,
			"action":           artifact.Action,
			"id":               artifact.ID,
			"title":            artifact.Title,
			"kind":             artifact.Kind,
			"path":             artifact.Path,
			"status":           artifact.Status,
		}
		if artifact.Description != "" {
			item["description"] = artifact.Description
		}
		result = append(result, item)
	}
	return result
}

func safeEnvironmentObservation(environment EnvironmentObservation) map[string]any {
	result := map[string]any{
		"observation": environment.Observation,
		"observed_at": environment.ObservedAt,
		"scope":       environment.Scope,
		"source":      environment.Source,
	}
	if environment.WorkspaceRoot != "" {
		result["workspace_root"] = environment.WorkspaceRoot
	}
	if environment.WorkingDirectory != "" {
		result["working_directory"] = environment.WorkingDirectory
	}
	if len(environment.Files) > 0 {
		result["files"] = append([]string(nil), environment.Files...)
	}
	if environment.Git != nil {
		result["git"] = environment.Git
	}
	if len(environment.Processes) > 0 {
		result["processes"] = environment.Processes
	}
	if len(environment.Variables) > 0 {
		// EnvironmentVariableChange deliberately contains names/actions only.
		result["environment_variables"] = environment.Variables
	}
	if len(environment.Unobserved) > 0 {
		result["unobserved"] = append([]string(nil), environment.Unobserved...)
	}
	return result
}

func safeNextActions(actions []NextAction) []map[string]any {
	result := make([]map[string]any, 0, len(actions))
	for _, action := range actions {
		item := map[string]any{
			"id":          action.ID,
			"description": action.Description,
		}
		if action.Tool != "" {
			item["tool"] = action.Tool
		}
		if len(action.Preconditions) > 0 {
			item["preconditions"] = append([]string(nil), action.Preconditions...)
		}
		if len(action.InputHint) > 0 {
			item["input_hint"] = safeModelValue(action.InputHint)
		}
		result = append(result, item)
	}
	return result
}
