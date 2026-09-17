package agentcore

import (
	"encoding/json"
	"fmt"
	"strings"

	convcontext "awaken/src/context"
)

const toolResultPageUnloadedPrefix = "[TOOL_RESULT_PAGE_UNLOADED]"

// applyPagedRetrievalProjection drops older retrieved pages from the request
// clone. Durable conversation and SemanticJournal keep the full page; the next
// turn re-reads with the same tool arguments. This matches the near-tail media
// rule: only the latest safe group stays in the provider-visible request.
func applyPagedRetrievalProjection(messages []convcontext.Message) {
	if len(messages) == 0 {
		return
	}
	lastStart := lastSafeGroupStart(messages)
	calls := indexToolCallsByID(messages)
	for i := 0; i < lastStart; i++ {
		unloadPagedRetrievalMessage(&messages[i], calls)
	}
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

func unloadPagedRetrievalMessage(message *convcontext.Message, calls map[string]convcontext.FunctionCall) {
	if message == nil || message.Role != convcontext.RoleTool || message.IsError {
		return
	}
	if !isPagedRetrievalToolName(message.Name) {
		return
	}
	if strings.Contains(message.Content, toolResultPageUnloadedPrefix) {
		return
	}
	fn := calls[message.ToolCallID]
	stub := pagedRetrievalUnloadStub(message.Name, message.ToolCallID, fn.Arguments)
	message.Content = rewritePagedRetrievalContent(message.Content, stub)
}

func isPagedRetrievalToolName(name string) bool {
	switch strings.TrimSpace(name) {
	case "tool_result_read", "thread_history_read", "archived_history":
		return true
	default:
		return false
	}
}

func pagedRetrievalUnloadStub(toolName, toolCallID, arguments string) string {
	var b strings.Builder
	b.WriteString(toolResultPageUnloadedPrefix)
	b.WriteByte('\n')
	b.WriteString("This retrieved page was dropped from the live request to keep the context window usable. Durable storage is unchanged. Call ")
	if strings.TrimSpace(toolName) == "" {
		b.WriteString("the same retrieval tool")
	} else {
		b.WriteString(toolName)
	}
	b.WriteString(" again with the same arguments if this exact page is needed.\n")
	if id := strings.TrimSpace(toolCallID); id != "" {
		fmt.Fprintf(&b, "tool_call_id=%s\n", id)
	}
	if args := strings.TrimSpace(arguments); args != "" {
		fmt.Fprintf(&b, "arguments=%s\n", args)
	}
	return b.String()
}

func rewritePagedRetrievalContent(raw, stub string) string {
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
	details["page_unloaded"] = true
	payload["details"] = details
	encoded, err := json.Marshal(payload)
	if err != nil {
		return stub
	}
	return string(encoded)
}
