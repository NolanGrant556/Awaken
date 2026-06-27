package shellhooks

import (
	"encoding/json"
	"strings"
)

func ParseOutput(stdout string) ParsedOutput {
	trimmed := strings.TrimSpace(stdout)
	if trimmed == "" {
		return ParsedOutput{}
	}
	if !strings.HasPrefix(trimmed, "{") {
		return ParsedOutput{PlainText: stdout}
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(trimmed), &data); err != nil {
		return ParsedOutput{PlainText: stdout, JSONError: err.Error()}
	}
	if data == nil {
		return ParsedOutput{PlainText: stdout}
	}

	decision := decisionFromJSON(data)
	return ParsedOutput{JSON: data, Decision: decision}
}

func decisionFromJSON(data map[string]any) Decision {
	decision := Decision{Raw: data}

	if reason := stringField(data, "reason"); reason != "" {
		decision.Reason = reason
	}
	if message := stringField(data, "message"); message != "" {
		decision.Message = message
	}
	if context := firstString(data, "context", "additional_context", "systemMessage", "system_message"); context != "" {
		decision.Context = context
	}
	if rewrite, ok := firstValue(data, "rewrite", "updated_input", "updatedInput", "tool_input"); ok {
		decision.Rewrite = rewrite
	}
	if transformed, ok := firstValue(data, "transform_result", "updated_result", "updated_output", "updatedMCPToolOutput"); ok {
		decision.TransformResult = transformed
	}

	if specific, ok := data["hookSpecificOutput"].(map[string]any); ok {
		applyHookSpecificOutput(&decision, specific)
	}

	switch normalizeDecisionName(firstString(data, "decision", "action")) {
	case "approve", "allow":
		decision.Type = DecisionAllow
	case "block", "deny":
		decision.Type = DecisionBlock
	case "rewrite":
		decision.Type = DecisionRewrite
	case "context":
		decision.Type = DecisionContext
	case "transform_result", "transform":
		decision.Type = DecisionTransformResult
	}

	if decision.Type == DecisionNone {
		switch {
		case decision.TransformResult != nil:
			decision.Type = DecisionTransformResult
		case decision.Rewrite != nil:
			decision.Type = DecisionRewrite
		case decision.Context != "":
			decision.Type = DecisionContext
		}
	}

	if decision.Message == "" {
		decision.Message = decision.Reason
	}
	if decision.Reason == "" {
		decision.Reason = decision.Message
	}
	if decision.IsZero() {
		return Decision{}
	}
	return decision
}

func applyHookSpecificOutput(decision *Decision, specific map[string]any) {
	if context := firstString(specific, "additionalContext", "additional_context"); context != "" {
		decision.Context = context
	}
	if rewrite, ok := firstValue(specific, "updatedInput", "updated_input"); ok {
		decision.Rewrite = rewrite
	}
	if transformed, ok := firstValue(specific, "updatedMCPToolOutput", "transform_result"); ok {
		decision.TransformResult = transformed
	}
	if reason := firstString(specific, "permissionDecisionReason", "reason"); reason != "" {
		decision.Reason = reason
	}

	switch normalizeDecisionName(firstString(specific, "permissionDecision", "decision")) {
	case "allow", "approve":
		decision.Type = DecisionAllow
	case "deny", "block":
		decision.Type = DecisionBlock
	case "ask":
		decision.Type = DecisionNone
	}
}

func (p ParsedOutput) Redact(redactor *Redactor) ParsedOutput {
	p.PlainText = redactor.Redact(p.PlainText)
	p.JSONError = redactor.Redact(p.JSONError)
	if p.JSON != nil {
		if redacted, ok := redactor.RedactValue(p.JSON).(map[string]any); ok {
			p.JSON = redacted
		}
	}
	p.Decision = p.Decision.Redact(redactor)
	return p
}

func (d Decision) Redact(redactor *Redactor) Decision {
	d.Reason = redactor.Redact(d.Reason)
	d.Message = redactor.Redact(d.Message)
	d.Context = redactor.Redact(d.Context)
	d.Rewrite = redactor.RedactValue(d.Rewrite)
	d.TransformResult = redactor.RedactValue(d.TransformResult)
	if d.Raw != nil {
		if redacted, ok := redactor.RedactValue(d.Raw).(map[string]any); ok {
			d.Raw = redacted
		}
	}
	return d
}

func firstString(data map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := stringField(data, key); value != "" {
			return value
		}
	}
	return ""
}

func stringField(data map[string]any, key string) string {
	value, ok := data[key].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}

func firstValue(data map[string]any, keys ...string) (any, bool) {
	for _, key := range keys {
		value, ok := data[key]
		if ok && value != nil {
			return value, true
		}
	}
	return nil, false
}

func normalizeDecisionName(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
