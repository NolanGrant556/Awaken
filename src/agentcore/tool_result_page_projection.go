package agentcore

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	convcontext "awaken/src/context"
)

const (
	priorToolResultUnloadedPrefix = "[PRIOR_TOOL_RESULT_UNLOADED]"
	toolResultPageUnloadedPrefix  = "[TOOL_RESULT_PAGE_UNLOADED]"
	priorToolResultKeepRunes      = 512
	priorToolResultArgsKeepRunes  = 500
)

// applyPriorToolResultProjection drops older tool bodies from the request
// clone. Durable conversation and SemanticJournal keep the full text. Only the
// near-tail safe group stays provider-visible, matching media projection.
// Earlier results become a short pointer so the next turn can re-run the tool
// or read thread_history_read instead of stacking every prepare/execute JSON
// until compact's 10% floor is already unreachable.
func applyPriorToolResultProjection(messages []convcontext.Message) {
	if len(messages) == 0 {
		return
	}
	lastStart := lastSafeGroupStart(messages)
	calls := indexToolCallsByID(messages)
	for i := 0; i < lastStart; i++ {
		unloadPriorToolResultMessage(&messages[i], calls)
	}
}

func applyPagedRetrievalProjection(messages []convcontext.Message) {
	applyPriorToolResultProjection(messages)
}

func indexToolCallsByID(messages []convcontext.Message) map[string]convcontext.FunctionCall {
	calls := make(map[string]convcontext.FunctionCall)
	for _, message := range messages {
		for _, call := range message.ToolCalls {
			if strings.TrimSpace(call.ID) == "" {
				continue
			}
			calls[call.ID] = call.Function
		}
	}
	return calls
}

func unloadPriorToolResultMessage(message *convcontext.Message, calls map[string]convcontext.FunctionCall) {
	if message == nil || message.Role != convcontext.RoleTool {
		return
	}
	if message.IsError && utf8.RuneCountInString(message.Content) <= priorToolResultKeepRunes {
		return
	}
	if alreadyUnloadedToolResult(message.Content) {
		return
	}
	if utf8.RuneCountInString(message.Content) <= priorToolResultKeepRunes {
		return
	}
	fn := calls[message.ToolCallID]
	name := strings.TrimSpace(message.Name)
	if name == "" {
		name = strings.TrimSpace(fn.Name)
	}
	stub := priorToolResultUnloadStub(name, message.ToolCallID, fn.Arguments)
	message.Content = rewriteUnloadedToolResultContent(message.Content, stub)
}

func alreadyUnloadedToolResult(content string) bool {
	return strings.Contains(content, priorToolResultUnloadedPrefix) ||
		strings.Contains(content, toolResultPageUnloadedPrefix) ||
		strings.Contains(content, "[TOOL_RESULT_OFFLOADED]") ||
		strings.Contains(content, mediaStubPrefix)
}

func priorToolResultUnloadStub(toolName, toolCallID, arguments string) string {
	var b strings.Builder
	b.WriteString(priorToolResultUnloadedPrefix)
	b.WriteByte('\n')
	b.WriteString("This earlier tool result was dropped from the live request to keep the context window usable. Durable journal is unchanged. Re-run the tool for current state, or use thread_history_read for the exact previous text.\n")
	if name := strings.TrimSpace(toolName); name != "" {
		fmt.Fprintf(&b, "tool_name=%s\n", name)
	}
	if id := strings.TrimSpace(toolCallID); id != "" {
		fmt.Fprintf(&b, "tool_call_id=%s\n", id)
	}
	if args := boundRunes(strings.TrimSpace(arguments), priorToolResultArgsKeepRunes); args != "" {
		fmt.Fprintf(&b, "arguments=%s\n", args)
	}
	return b.String()
}

func boundRunes(value string, limit int) string {
	if limit <= 0 || utf8.RuneCountInString(value) <= limit {
		return value
	}
	return string([]rune(value)[:limit]) + "…"
}

func rewriteUnloadedToolResultContent(raw, stub string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || !strings.HasPrefix(trimmed, "{") {
		return stub
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return stub
	}
	if payload["schema_version"] == nil && payload["content"] == nil {
		return stub
	}
	payload["content"] = stub
	payload["truncated"] = true
	details, _ := payload["details"].(map[string]any)
	if details == nil {
		details = map[string]any{}
	}
	details["prior_result_unloaded"] = true
	payload["details"] = details
	encoded, err := json.Marshal(payload)
	if err != nil {
		return stub
	}
	return string(encoded)
}

func rewritePagedRetrievalContent(raw, stub string) string {
	return rewriteUnloadedToolResultContent(raw, stub)
}
