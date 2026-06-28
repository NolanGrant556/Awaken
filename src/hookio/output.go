package hookio

import (
	"encoding/json"
	"fmt"
	"strings"
)

type OutputKind string

const (
	OutputKindAsync OutputKind = "async"
	OutputKindSync  OutputKind = "sync"
)

type Decision string

const (
	DecisionApprove Decision = "approve"
	DecisionBlock   Decision = "block"
)

type HookEventName string

const (
	HookEventPreToolUse         HookEventName = "PreToolUse"
	HookEventUserPromptSubmit   HookEventName = "UserPromptSubmit"
	HookEventPermissionRequest  HookEventName = "PermissionRequest"
	HookEventElicitation        HookEventName = "Elicitation"
	HookEventElicitationResult  HookEventName = "ElicitationResult"
	HookEventPostToolUse        HookEventName = "PostToolUse"
	HookEventPostToolUseFailure HookEventName = "PostToolUseFailure"
	HookEventPermissionDenied   HookEventName = "PermissionDenied"
	HookEventNotification       HookEventName = "Notification"
	HookEventSessionStart       HookEventName = "SessionStart"
	HookEventSetup              HookEventName = "Setup"
	HookEventSubagentStart      HookEventName = "SubagentStart"
	HookEventWorktreeCreate     HookEventName = "WorktreeCreate"
	HookEventCwdChanged         HookEventName = "CwdChanged"
	HookEventFileChanged        HookEventName = "FileChanged"
)

const (
	permissionBehaviorAllow = "allow"
	permissionBehaviorDeny  = "deny"
	permissionBehaviorAsk   = "ask"
)

type Output struct {
	Kind  OutputKind
	Async *AsyncOutput
	Sync  *SyncOutput
}

type AsyncOutput struct {
	Async        bool
	AsyncTimeout *float64
}

type SyncOutput struct {
	Continue           *bool
	SuppressOutput     *bool
	StopReason         string
	Decision           Decision
	Reason             string
	SystemMessage      string
	HookSpecificOutput map[string]any
}

func ParseOutput(data []byte) (Output, error) {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return Output{}, fmt.Errorf("hook output must be a JSON object: %w", err)
	}
	if raw == nil {
		return Output{}, fmt.Errorf("hook output must be a JSON object")
	}

	if asyncValue, ok := raw["async"]; ok {
		async, ok := asyncValue.(bool)
		if !ok {
			return Output{}, fmt.Errorf("async must be boolean true when present")
		}
		if !async {
			return Output{}, fmt.Errorf("async must be true when present; async=false is not supported")
		}

		timeout, _, err := optionalNumber(raw, "asyncTimeout")
		if err != nil {
			return Output{}, err
		}
		return Output{
			Kind: OutputKindAsync,
			Async: &AsyncOutput{
				Async:        true,
				AsyncTimeout: timeout,
			},
		}, nil
	}

	return parseSyncOutput(raw)
}

func ValidateOutput(value any) (Output, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return Output{}, fmt.Errorf("hook output must be JSON serializable: %w", err)
	}
	return ParseOutput(data)
}

func parseSyncOutput(raw map[string]any) (Output, error) {
	continueValue, _, err := optionalBool(raw, "continue")
	if err != nil {
		return Output{}, err
	}
	suppressOutput, _, err := optionalBool(raw, "suppressOutput")
	if err != nil {
		return Output{}, err
	}
	stopReason, hasStopReason, err := optionalString(raw, "stopReason")
	if err != nil {
		return Output{}, err
	}
	if continueValue != nil && !*continueValue && (!hasStopReason || stopReason == "") {
		return Output{}, fmt.Errorf("stopReason is required when continue is false")
	}
	decision, _, err := optionalEnum(raw, "decision", "approve", "block")
	if err != nil {
		return Output{}, err
	}
	reason, _, err := optionalString(raw, "reason")
	if err != nil {
		return Output{}, err
	}
	systemMessage, _, err := optionalString(raw, "systemMessage")
	if err != nil {
		return Output{}, err
	}

	hookSpecificOutput, hasHookSpecificOutput, err := optionalObject(raw, "hookSpecificOutput")
	if err != nil {
		return Output{}, err
	}
	if hasHookSpecificOutput {
		if err := validateHookSpecificOutput(hookSpecificOutput); err != nil {
			return Output{}, err
		}
	}

	return Output{
		Kind: OutputKindSync,
		Sync: &SyncOutput{
			Continue:           continueValue,
			SuppressOutput:     suppressOutput,
			StopReason:         stopReason,
			Decision:           Decision(decision),
			Reason:             reason,
			SystemMessage:      systemMessage,
			HookSpecificOutput: cloneMap(hookSpecificOutput),
		},
	}, nil
}

func validateHookSpecificOutput(raw map[string]any) error {
	eventName, ok, err := requiredString(raw, "hookSpecificOutput.hookEventName")
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("hookSpecificOutput.hookEventName is required")
	}

	switch HookEventName(eventName) {
	case HookEventPreToolUse:
		return validatePreToolUse(raw)
	case HookEventUserPromptSubmit:
		return validateAdditionalContextOnly(raw)
	case HookEventPermissionRequest:
		return validatePermissionRequest(raw)
	case HookEventElicitation:
		return validateElicitation(raw)
	case HookEventElicitationResult:
		return validateElicitation(raw)
	case HookEventPostToolUse:
		return validatePostToolUse(raw)
	case HookEventPostToolUseFailure:
		return validateAdditionalContextOnly(raw)
	case HookEventPermissionDenied:
		return validatePermissionDenied(raw)
	case HookEventNotification:
		return validateAdditionalContextOnly(raw)
	case HookEventSessionStart:
		return validateSessionStart(raw)
	case HookEventSetup:
		return validateAdditionalContextOnly(raw)
	case HookEventSubagentStart:
		return validateAdditionalContextOnly(raw)
	case HookEventWorktreeCreate:
		return validateWorktreeCreate(raw)
	case HookEventCwdChanged:
		return validateWatchPathsOnly(raw)
	case HookEventFileChanged:
		return validateWatchPathsOnly(raw)
	default:
		return fmt.Errorf("unknown hookEventName %q", eventName)
	}
}

func validatePreToolUse(raw map[string]any) error {
	if _, _, err := optionalEnum(raw, "hookSpecificOutput.permissionDecision", permissionBehaviorAllow, permissionBehaviorDeny, permissionBehaviorAsk); err != nil {
		return err
	}
	if _, _, err := optionalString(raw, "hookSpecificOutput.reason"); err != nil {
		return err
	}
	if _, _, err := optionalString(raw, "hookSpecificOutput.permissionDecisionReason"); err != nil {
		return err
	}
	if _, _, err := optionalObject(raw, "hookSpecificOutput.updatedInput"); err != nil {
		return err
	}
	if _, _, err := optionalString(raw, "hookSpecificOutput.additionalContext"); err != nil {
		return err
	}
	return nil
}

func validatePermissionRequest(raw map[string]any) error {
	decision, hasDecision, err := optionalObject(raw, "hookSpecificOutput.decision")
	if err != nil {
		return err
	}
	if !hasDecision {
		return fmt.Errorf("hookSpecificOutput.decision is required for PermissionRequest")
	}

	behavior, ok, err := requiredString(decision, "hookSpecificOutput.decision.behavior")
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("hookSpecificOutput.decision.behavior is required")
	}

	switch behavior {
	case permissionBehaviorAllow:
		if _, _, err := optionalObject(decision, "hookSpecificOutput.decision.updatedInput"); err != nil {
			return err
		}
		if err := validateUpdatedPermissions(decision); err != nil {
			return err
		}
	case permissionBehaviorDeny:
		if _, _, err := optionalString(decision, "hookSpecificOutput.decision.message"); err != nil {
			return err
		}
		if _, _, err := optionalBool(decision, "hookSpecificOutput.decision.interrupt"); err != nil {
			return err
		}
	default:
		return enumError("hookSpecificOutput.decision.behavior", behavior, "allow", "deny")
	}

	return nil
}

func validateElicitation(raw map[string]any) error {
	if _, _, err := optionalEnum(raw, "hookSpecificOutput.action", "accept", "decline", "cancel"); err != nil {
		return err
	}
	if _, _, err := optionalObject(raw, "hookSpecificOutput.content"); err != nil {
		return err
	}
	return nil
}

func validatePermissionDenied(raw map[string]any) error {
	if _, _, err := optionalBool(raw, "hookSpecificOutput.retry"); err != nil {
		return err
	}
	return nil
}

func validatePostToolUse(raw map[string]any) error {
	if _, _, err := optionalString(raw, "hookSpecificOutput.additionalContext"); err != nil {
		return err
	}
	return nil
}

func validateSessionStart(raw map[string]any) error {
	if _, _, err := optionalStringArray(raw, "hookSpecificOutput.watchPaths"); err != nil {
		return err
	}
	if _, _, err := optionalString(raw, "hookSpecificOutput.additionalContext"); err != nil {
		return err
	}
	if _, _, err := optionalString(raw, "hookSpecificOutput.initialUserMessage"); err != nil {
		return err
	}
	return nil
}

func validateWorktreeCreate(raw map[string]any) error {
	_, ok, err := requiredString(raw, "hookSpecificOutput.worktreePath")
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("hookSpecificOutput.worktreePath is required for WorktreeCreate")
	}
	return nil
}

func validateAdditionalContextOnly(raw map[string]any) error {
	if _, _, err := optionalString(raw, "hookSpecificOutput.additionalContext"); err != nil {
		return err
	}
	return nil
}

func validateWatchPathsOnly(raw map[string]any) error {
	if _, _, err := optionalStringArray(raw, "hookSpecificOutput.watchPaths"); err != nil {
		return err
	}
	return nil
}

func validateUpdatedPermissions(raw map[string]any) error {
	value, ok := raw["updatedPermissions"]
	if !ok {
		return nil
	}
	items, ok := value.([]any)
	if !ok {
		return fmt.Errorf("hookSpecificOutput.decision.updatedPermissions must be an array")
	}
	for i, item := range items {
		typed, ok := item.(map[string]any)
		if !ok {
			return fmt.Errorf("hookSpecificOutput.decision.updatedPermissions[%d] must be an object", i)
		}
		if err := validatePermissionUpdate(typed, i); err != nil {
			return err
		}
	}
	return nil
}

func validatePermissionUpdate(raw map[string]any, index int) error {
	path := fmt.Sprintf("hookSpecificOutput.decision.updatedPermissions[%d]", index)
	typeValue, ok, err := requiredString(raw, path+".type")
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%s.type is required", path)
	}
	if err := validatePermissionDestination(raw, path); err != nil {
		return err
	}

	switch typeValue {
	case "addRules", "replaceRules", "removeRules":
		if _, _, err := requiredEnum(raw, path+".behavior", permissionBehaviorAllow, permissionBehaviorDeny, permissionBehaviorAsk); err != nil {
			return err
		}
		return validatePermissionRules(raw, path)
	case "setMode":
		if _, _, err := requiredEnum(raw, path+".mode", "acceptEdits", "bypassPermissions", "default", "dontAsk", "plan"); err != nil {
			return err
		}
		return nil
	case "addDirectories", "removeDirectories":
		return validatePermissionDirectories(raw, path)
	default:
		return enumError(path+".type", typeValue, "addRules", "replaceRules", "removeRules", "setMode", "addDirectories", "removeDirectories")
	}
}

func validatePermissionDestination(raw map[string]any, path string) error {
	if _, _, err := requiredEnum(raw, path+".destination", "userSettings", "projectSettings", "localSettings", "session", "cliArg"); err != nil {
		return err
	}
	return nil
}

func validatePermissionRules(raw map[string]any, path string) error {
	value, ok := fieldValue(raw, path+".rules")
	if !ok {
		return fmt.Errorf("%s.rules is required", path)
	}
	items, ok := value.([]any)
	if !ok {
		return fmt.Errorf("%s.rules must be an array", path)
	}
	for i, item := range items {
		rule, ok := item.(map[string]any)
		if !ok {
			return fmt.Errorf("%s.rules[%d] must be an object", path, i)
		}
		if _, _, err := requiredString(rule, path+".rules["+fmt.Sprint(i)+"].toolName"); err != nil {
			return err
		}
		if _, _, err := optionalString(rule, path+".rules["+fmt.Sprint(i)+"].ruleContent"); err != nil {
			return err
		}
	}
	return nil
}

func validatePermissionDirectories(raw map[string]any, path string) error {
	value, ok := fieldValue(raw, path+".directories")
	if !ok {
		return fmt.Errorf("%s.directories is required", path)
	}
	items, ok := value.([]any)
	if !ok {
		return fmt.Errorf("%s.directories must be an array", path)
	}
	for i, item := range items {
		if _, ok := item.(string); !ok {
			return fmt.Errorf("%s.directories[%d] must be a string", path, i)
		}
	}
	return nil
}

func optionalBool(raw map[string]any, path string) (*bool, bool, error) {
	value, ok := fieldValue(raw, path)
	if !ok {
		return nil, false, nil
	}
	typed, ok := value.(bool)
	if !ok {
		return nil, true, fmt.Errorf("%s must be a boolean", path)
	}
	return &typed, true, nil
}

func optionalNumber(raw map[string]any, path string) (*float64, bool, error) {
	value, ok := fieldValue(raw, path)
	if !ok {
		return nil, false, nil
	}
	typed, ok := value.(float64)
	if !ok {
		return nil, true, fmt.Errorf("%s must be a number", path)
	}
	return &typed, true, nil
}

func optionalString(raw map[string]any, path string) (string, bool, error) {
	value, ok := fieldValue(raw, path)
	if !ok {
		return "", false, nil
	}
	typed, ok := value.(string)
	if !ok {
		return "", true, fmt.Errorf("%s must be a string", path)
	}
	return typed, true, nil
}

func requiredString(raw map[string]any, path string) (string, bool, error) {
	return optionalString(raw, path)
}

func requiredEnum(raw map[string]any, path string, allowed ...string) (string, bool, error) {
	value, ok, err := optionalEnum(raw, path, allowed...)
	if err != nil {
		return value, ok, err
	}
	if !ok {
		return "", false, fmt.Errorf("%s is required", path)
	}
	return value, true, nil
}

func optionalEnum(raw map[string]any, path string, allowed ...string) (string, bool, error) {
	value, ok, err := optionalString(raw, path)
	if err != nil || !ok {
		return value, ok, err
	}
	for _, candidate := range allowed {
		if value == candidate {
			return value, true, nil
		}
	}
	return value, true, enumError(path, value, allowed...)
}

func optionalObject(raw map[string]any, path string) (map[string]any, bool, error) {
	value, ok := fieldValue(raw, path)
	if !ok {
		return nil, false, nil
	}
	typed, ok := value.(map[string]any)
	if !ok {
		return nil, true, fmt.Errorf("%s must be an object", path)
	}
	return typed, true, nil
}

func optionalObjectArray(raw map[string]any, path string) ([]map[string]any, bool, error) {
	value, ok := fieldValue(raw, path)
	if !ok {
		return nil, false, nil
	}
	items, ok := value.([]any)
	if !ok {
		return nil, true, fmt.Errorf("%s must be an array", path)
	}
	result := make([]map[string]any, 0, len(items))
	for i, item := range items {
		typed, ok := item.(map[string]any)
		if !ok {
			return nil, true, fmt.Errorf("%s[%d] must be an object", path, i)
		}
		result = append(result, typed)
	}
	return result, true, nil
}

func optionalStringArray(raw map[string]any, path string) ([]string, bool, error) {
	value, ok := fieldValue(raw, path)
	if !ok {
		return nil, false, nil
	}
	items, ok := value.([]any)
	if !ok {
		return nil, true, fmt.Errorf("%s must be an array", path)
	}
	result := make([]string, 0, len(items))
	for i, item := range items {
		typed, ok := item.(string)
		if !ok {
			return nil, true, fmt.Errorf("%s[%d] must be a string", path, i)
		}
		result = append(result, typed)
	}
	return result, true, nil
}

func fieldValue(raw map[string]any, path string) (any, bool) {
	key := path[strings.LastIndex(path, ".")+1:]
	value, ok := raw[key]
	return value, ok
}

func enumError(path string, value string, allowed ...string) error {
	return fmt.Errorf("%s must be one of %s (got %q)", path, strings.Join(allowed, ", "), value)
}

func cloneMap(raw map[string]any) map[string]any {
	if raw == nil {
		return nil
	}
	cloned := make(map[string]any, len(raw))
	for key, value := range raw {
		cloned[key] = cloneValue(value)
	}
	return cloned
}

func cloneSlice(raw []any) []any {
	if raw == nil {
		return nil
	}
	cloned := make([]any, len(raw))
	for i, value := range raw {
		cloned[i] = cloneValue(value)
	}
	return cloned
}

func cloneValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneMap(typed)
	case []any:
		return cloneSlice(typed)
	default:
		return typed
	}
}
