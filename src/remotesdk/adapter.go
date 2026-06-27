package remotesdk

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func DecodeMessage(data []byte) (SDKMessage, error) {
	var msg SDKMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return SDKMessage{}, err
	}
	msg.Raw = append(msg.Raw[:0], data...)
	return msg, nil
}

func ConvertJSONMessage(data []byte, opts ConvertOptions) ConversionResult {
	msg, err := DecodeMessage(data)
	if err != nil {
		return ConversionResult{
			Kind:   ConversionUnsupported,
			Reason: fmt.Sprintf("invalid_json: %v", err),
		}
	}
	return ConvertMessage(msg, opts)
}

func ConvertMessage(msg SDKMessage, opts ConvertOptions) ConversionResult {
	base := conversionBase(msg)

	switch msg.Type {
	case SDKMessageAssistant:
		event := baseEvent(msg, opts, EventAssistantMessage, SeverityInfo)
		event.Role = firstNonEmpty(msg.Message.Role, "assistant")
		event.Content = msg.Message.Content.PlainText()
		event.ToolUses = extractToolUses(msg.Message.Content)
		event.Error = strings.TrimSpace(msg.Error)
		if event.Error != "" {
			event.Severity = SeverityError
		}
		return eventResult(base, event)

	case SDKMessageUser:
		isToolResult := IsToolResultUserMessage(msg)
		if opts.ConvertToolResults && isToolResult {
			event := baseEvent(msg, opts, EventUserMessage, SeverityInfo)
			event.Role = firstNonEmpty(msg.Message.Role, "user")
			event.Content = msg.Message.Content.PlainText()
			event.ToolResults = extractToolResults(msg.Message.Content)
			return eventResult(base, event)
		}
		if opts.ConvertUserTextMessages && !isToolResult && hasDisplayableUserContent(msg.Message.Content) {
			event := baseEvent(msg, opts, EventUserMessage, SeverityInfo)
			event.Role = firstNonEmpty(msg.Message.Role, "user")
			event.Content = msg.Message.Content.PlainText()
			return eventResult(base, event)
		}
		reason := "user_message_echo_ignored"
		if isToolResult {
			reason = "tool_result_conversion_disabled"
		}
		return ignoredResult(base, reason)

	case SDKMessageStreamEvent:
		event := baseEvent(msg, opts, EventStreamEvent, SeverityInfo)
		event.StreamEventType = msg.Event.Type
		event.Delta = msg.Event.DeltaText()
		event.RawStreamEvent = cloneRaw(msg.Event.Raw)
		if msg.Event.ContentBlock.Type == "tool_use" {
			event.ToolUseID = msg.Event.ContentBlock.ID
			event.ToolName = msg.Event.ContentBlock.Name
		}
		return eventResult(base, event)

	case SDKMessageResult:
		if msg.Subtype == "success" {
			return ignoredResult(base, "success_result_ignored")
		}
		event := baseEvent(msg, opts, EventResult, SeverityForResult(msg.Subtype, msg.IsError))
		event.ResultSubtype = msg.Subtype
		event.Content = resultContent(msg)
		event.Error = strings.Join(msg.Errors, "\n")
		return eventResult(base, event)

	case SDKMessageSystem:
		return convertSystemMessage(msg, opts, base)

	case SDKMessageToolProgress:
		event := baseEvent(msg, opts, EventToolProgress, SeverityInfo)
		event.ToolUseID = msg.ToolUseID
		event.ToolName = msg.ToolName
		event.ElapsedSeconds = msg.ElapsedSeconds
		event.TaskID = msg.TaskID
		event.Content = toolProgressContent(msg)
		return eventResult(base, event)

	case SDKMessageAuthStatus, SDKMessageToolUseSummary, SDKMessageRateLimitEvent:
		return ignoredResult(base, "sdk_only_message_ignored")

	case "":
		return unsupportedResult(base, "missing_message_type")

	default:
		return unsupportedResult(base, "unsupported_message_type")
	}
}

func IsSessionEndMessage(msg SDKMessage) bool {
	return msg.Type == SDKMessageResult
}

func IsSuccessResult(msg SDKMessage) bool {
	return msg.Type == SDKMessageResult && msg.Subtype == "success"
}

func ResultText(msg SDKMessage) (string, bool) {
	if IsSuccessResult(msg) {
		return msg.Result, true
	}
	return "", false
}

func IsToolResultUserMessage(msg SDKMessage) bool {
	return msg.Type == SDKMessageUser && msg.Message.Content.HasToolResult()
}

func SeverityForResult(subtype string, isError bool) Severity {
	switch strings.TrimSpace(subtype) {
	case "success":
		return SeverityInfo
	case "error_max_turns", "error_max_budget_usd":
		return SeverityWarning
	case "error_during_execution", "error_max_structured_output_retries":
		return SeverityError
	default:
		if isError || strings.HasPrefix(subtype, "error_") {
			return SeverityError
		}
		return SeverityInfo
	}
}

func SeverityForStatus(status string) Severity {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "", "compacting", "running", "started", "initializing", "waiting":
		return SeverityInfo
	case "blocked", "rate_limited", "warning":
		return SeverityWarning
	case "failed", "error", "cancelled":
		return SeverityError
	default:
		return SeverityInfo
	}
}

func convertSystemMessage(msg SDKMessage, opts ConvertOptions, base ConversionResult) ConversionResult {
	switch SDKSystemSubtype(msg.Subtype) {
	case SDKSystemInit:
		event := baseEvent(msg, opts, EventSystemInit, SeverityInfo)
		event.Model = msg.Model
		event.CWD = msg.CWD
		event.Tools = append([]string(nil), msg.Tools...)
		if msg.Model != "" {
			event.Content = fmt.Sprintf("Remote session initialized (model: %s)", msg.Model)
		} else {
			event.Content = "Remote session initialized"
		}
		return eventResult(base, event)

	case SDKSystemStatus:
		if msg.Status == nil || strings.TrimSpace(*msg.Status) == "" {
			return ignoredResult(base, "empty_status_ignored")
		}
		status := strings.TrimSpace(*msg.Status)
		event := baseEvent(msg, opts, EventStatus, SeverityForStatus(status))
		event.Status = status
		if status == "compacting" {
			event.Content = "Compacting conversation"
		} else {
			event.Content = fmt.Sprintf("Status: %s", status)
		}
		return eventResult(base, event)

	case SDKSystemCompactBoundary:
		event := baseEvent(msg, opts, EventCompactBoundary, SeverityInfo)
		event.Content = "Conversation compacted"
		metadata := msg.CompactMetadata
		event.CompactMetadata = &metadata
		return eventResult(base, event)

	default:
		if strings.TrimSpace(msg.Subtype) == "" {
			return unsupportedResult(base, "missing_system_subtype")
		}
		return ignoredResult(base, "unsupported_system_subtype_ignored")
	}
}

func conversionBase(msg SDKMessage) ConversionResult {
	return ConversionResult{
		MessageType: msg.Type,
		Subtype:     msg.Subtype,
		UUID:        msg.UUID,
		SessionID:   msg.SessionID,
	}
}

func baseEvent(msg SDKMessage, opts ConvertOptions, eventType AwakenRemoteEventType, severity Severity) AwakenRemoteEvent {
	return AwakenRemoteEvent{
		Type:            eventType,
		Severity:        severity,
		Timestamp:       messageTimestamp(msg.Timestamp, opts),
		UUID:            msg.UUID,
		SessionID:       msg.SessionID,
		ParentToolUseID: stringFromPtr(msg.ParentToolUseID),
		ToolUseID:       msg.ToolUseID,
		ToolName:        msg.ToolName,
		RawMessage:      cloneRaw(msg.Raw),
	}
}

func eventResult(base ConversionResult, event AwakenRemoteEvent) ConversionResult {
	base.Kind = ConversionEvent
	base.Event = &event
	return base
}

func ignoredResult(base ConversionResult, reason string) ConversionResult {
	base.Kind = ConversionIgnored
	base.Reason = reason
	return base
}

func unsupportedResult(base ConversionResult, reason string) ConversionResult {
	base.Kind = ConversionUnsupported
	base.Reason = reason
	return base
}

func messageTimestamp(value string, opts ConvertOptions) time.Time {
	trimmed := strings.TrimSpace(value)
	if trimmed != "" {
		if ts, err := time.Parse(time.RFC3339Nano, trimmed); err == nil {
			return ts.UTC()
		}
	}
	return fallbackTimestamp(opts)
}

func fallbackTimestamp(opts ConvertOptions) time.Time {
	if !opts.FallbackTimestamp.IsZero() {
		return opts.FallbackTimestamp.UTC()
	}
	return time.Now().UTC()
}

func extractToolUses(content SDKContent) []ToolUseBlock {
	var out []ToolUseBlock
	for _, block := range content.Blocks {
		if block.Type != "tool_use" {
			continue
		}
		out = append(out, ToolUseBlock{
			ID:    block.ID,
			Name:  block.Name,
			Input: cloneMap(block.Input),
		})
	}
	return out
}

func extractToolResults(content SDKContent) []ToolResultBlock {
	var out []ToolResultBlock
	for _, block := range content.Blocks {
		if block.Type != "tool_result" {
			continue
		}
		out = append(out, ToolResultBlock{
			ToolUseID: block.ToolUseID,
			Content:   block.ContentText(),
			IsError:   block.IsError,
		})
	}
	return out
}

func hasDisplayableUserContent(content SDKContent) bool {
	if content.Text != nil {
		return true
	}
	for _, block := range content.Blocks {
		if block.Type != "tool_result" {
			return true
		}
	}
	return false
}

func resultContent(msg SDKMessage) string {
	if msg.Subtype == "success" {
		return msg.Result
	}
	if len(msg.Errors) > 0 {
		return strings.Join(msg.Errors, "\n")
	}
	return firstNonEmpty(msg.Result, "Remote session ended with an error")
}

func toolProgressContent(msg SDKMessage) string {
	toolName := firstNonEmpty(msg.ToolName, "tool")
	if msg.ElapsedSeconds > 0 {
		return fmt.Sprintf("Tool %s running for %.0fs", toolName, msg.ElapsedSeconds)
	}
	return fmt.Sprintf("Tool %s is running", toolName)
}

func stringFromPtr(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func cloneRaw(raw json.RawMessage) json.RawMessage {
	if raw == nil {
		return nil
	}
	return append(json.RawMessage(nil), raw...)
}

func cloneMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
