package agentcore

import (
	stdcontext "context"
	"errors"
	"reflect"
	"strings"
	"testing"

	convcontext "awaken/src/context"
	"awaken/src/llm"
)

func TestCompactConversationWithRequestCanaryFactEntersSummary(t *testing.T) {
	canary := "legacy canary fact: migrate the billing export after qa signoff"
	messages := []convcontext.Message{
		{Role: convcontext.RoleUser, Content: canary},
		{Role: convcontext.RoleAssistant, Content: "Acknowledged."},
		{Role: convcontext.RoleUser, Content: "latest tail question"},
	}

	result, err := CompactConversationWithRequest(stdcontext.Background(), CompactRequest{
		Mode:         CompactModeManual,
		Messages:     messages,
		Envelope:     ContextEnvelope{Messages: messages, Model: "test-model"},
		TargetTokens: 1,
		summaryInvoker: func(_ stdcontext.Context, _ llm.Config, req llm.Request) (string, error) {
			found := false
			for _, message := range req.Messages {
				if strings.Contains(message.Content, canary) {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("expected compact summary request to contain canary fact, got %+v", req.Messages)
			}
			return renderCompactSummarySections(map[string][]string{
				"Active Task":               {"- " + canary},
				"Goal":                      {"- Preserve the canary fact."},
				"Relevant Files":            {"- None."},
				"Remaining Work":            {"- Continue from the protected tail."},
				"Critical Context":          {"- Canary retained."},
				"Completed Actions":         {"- None."},
				"Active State":              {"- None."},
				"In Progress":               {"- None."},
				"Blocked":                   {"- None."},
				"Key Decisions":             {"- None."},
				"Resolved Questions":        {"- None."},
				"Pending User Asks":         {"- None."},
				"Constraints & Preferences": {"- None."},
			}), nil
		},
	})
	if err != nil {
		t.Fatalf("CompactConversationWithRequest returned error: %v", err)
	}
	if !strings.Contains(result.SummaryMessage.Content, canary) {
		t.Fatalf("expected compact summary to contain canary fact, got %q", result.SummaryMessage.Content)
	}
	if len(result.TailMessages) != 1 || result.TailMessages[0].Content != "latest tail question" {
		t.Fatalf("expected protected tail to keep latest raw message, got %+v", result.TailMessages)
	}
}

func TestCompactConversationWithRequestErrorsWhenManualCompactHasOnlyOneSafeGroup(t *testing.T) {
	messages := []convcontext.Message{
		{Role: convcontext.RoleUser, Content: "huge single message " + strings.Repeat("x", 5000)},
	}

	result, err := CompactConversationWithRequest(stdcontext.Background(), CompactRequest{
		Mode:         CompactModeManual,
		Messages:     messages,
		Envelope:     ContextEnvelope{Messages: messages, Model: "test-model"},
		TargetTokens: 1,
	})
	if !errors.Is(err, errNotEnoughCompactableHistory) {
		t.Fatalf("expected not-enough-compactable-history error, got result=%+v err=%v", result, err)
	}
}

func TestCompactConversationStructuralPassKeepsToolPairing(t *testing.T) {
	largeToolResult := "tool output " + strings.Repeat("x", 3000)
	messages := []convcontext.Message{
		{Role: convcontext.RoleUser, Content: "inspect repository"},
		{
			Role:    convcontext.RoleAssistant,
			Content: "calling tool",
			ToolCalls: []convcontext.ToolCall{{
				ID: "call-1",
				Function: convcontext.FunctionCall{
					Name:      "file_read",
					Arguments: `{"path":"README.md"}`,
				},
			}},
		},
		{Role: convcontext.RoleTool, ToolCallID: "call-1", Name: "file_read", Content: largeToolResult},
		{Role: convcontext.RoleUser, Content: "latest tail question"},
	}

	result, err := CompactConversationWithRequest(stdcontext.Background(), CompactRequest{
		Mode:         CompactModeManual,
		Messages:     messages,
		Envelope:     ContextEnvelope{Messages: messages, Model: "test-model"},
		TargetTokens: 1,
		summaryInvoker: func(_ stdcontext.Context, _ llm.Config, req llm.Request) (string, error) {
			if err := convcontext.ValidateToolResultPairing(req.Messages); err != nil {
				t.Fatalf("expected structurally compacted summary request to keep pairing, got %v", err)
			}
			foundCompactedTool := false
			for _, message := range req.Messages {
				if message.Role == convcontext.RoleTool && strings.Contains(message.Content, "[STRUCTURAL_COMPACTION]") {
					foundCompactedTool = true
				}
				if strings.Contains(message.Content, largeToolResult) {
					t.Fatalf("expected large tool result to be structurally compacted before summary request")
				}
			}
			if !foundCompactedTool {
				t.Fatalf("expected structurally compacted tool result in summary request, got %+v", req.Messages)
			}
			return renderCompactSummarySections(map[string][]string{
				"Active Task":               {"- Inspect repository."},
				"Goal":                      {"- Keep tool history concise."},
				"Constraints & Preferences": {"- Preserve tool pairing."},
				"Completed Actions":         {"- file_read executed."},
				"Active State":              {"- None."},
				"In Progress":               {"- None."},
				"Blocked":                   {"- None."},
				"Key Decisions":             {"- None."},
				"Resolved Questions":        {"- None."},
				"Pending User Asks":         {"- None."},
				"Relevant Files":            {"- README.md"},
				"Remaining Work":            {"- Continue from protected tail."},
				"Critical Context":          {"- Tool result was structurally compacted."},
			}), nil
		},
	})
	if err != nil {
		t.Fatalf("CompactConversationWithRequest returned error: %v", err)
	}
	if err := convcontext.ValidateToolResultPairing(result.Messages); err != nil {
		t.Fatalf("expected compact result to keep tool pairing, got %v", err)
	}
	if result.ToolResultsPruned != 1 {
		t.Fatalf("expected one structurally pruned tool result, got %+v", result)
	}
	if !strings.Contains(result.SummaryMessage.Content, "Structural tool results pruned: 1.") {
		t.Fatalf("expected summary metadata to mention structural prune count, got %q", result.SummaryMessage.Content)
	}
}

func TestCompactConversationStructuralPassCountsMultipleToolResults(t *testing.T) {
	largeToolResultA := "tool output a " + strings.Repeat("x", 3000)
	largeToolResultB := "tool output b " + strings.Repeat("y", 3000)
	messages := []convcontext.Message{
		{Role: convcontext.RoleUser, Content: "inspect repository"},
		{
			Role:    convcontext.RoleAssistant,
			Content: "calling tool a",
			ToolCalls: []convcontext.ToolCall{{
				ID: "call-1",
				Function: convcontext.FunctionCall{
					Name:      "file_read",
					Arguments: `{"path":"README.md"}`,
				},
			}},
		},
		{Role: convcontext.RoleTool, ToolCallID: "call-1", Name: "file_read", Content: largeToolResultA},
		{
			Role:    convcontext.RoleAssistant,
			Content: "calling tool b",
			ToolCalls: []convcontext.ToolCall{{
				ID: "call-2",
				Function: convcontext.FunctionCall{
					Name:      "file_read",
					Arguments: `{"path":"docs/plan.md"}`,
				},
			}},
		},
		{Role: convcontext.RoleTool, ToolCallID: "call-2", Name: "file_read", Content: largeToolResultB},
		{Role: convcontext.RoleUser, Content: "latest tail question"},
	}

	result, err := CompactConversationWithRequest(stdcontext.Background(), CompactRequest{
		Mode:         CompactModeManual,
		Messages:     messages,
		Envelope:     ContextEnvelope{Messages: messages, Model: "test-model"},
		TargetTokens: 1,
		summaryInvoker: func(_ stdcontext.Context, _ llm.Config, req llm.Request) (string, error) {
			compactedCount := 0
			for _, message := range req.Messages {
				if message.Role == convcontext.RoleTool && strings.Contains(message.Content, "[STRUCTURAL_COMPACTION]") {
					compactedCount++
				}
			}
			if compactedCount != 2 {
				t.Fatalf("expected two structurally compacted tool results in summary request, got %d %+v", compactedCount, req.Messages)
			}
			return renderCompactSummarySections(map[string][]string{
				"Active Task":               {"- Inspect repository."},
				"Goal":                      {"- Keep tool history concise."},
				"Constraints & Preferences": {"- Preserve tool pairing."},
				"Completed Actions":         {"- file_read executed twice."},
				"Active State":              {"- None."},
				"In Progress":               {"- None."},
				"Blocked":                   {"- None."},
				"Key Decisions":             {"- None."},
				"Resolved Questions":        {"- None."},
				"Pending User Asks":         {"- None."},
				"Relevant Files":            {"- README.md, docs/plan.md"},
				"Remaining Work":            {"- Continue from protected tail."},
				"Critical Context":          {"- Two tool results were structurally compacted."},
			}), nil
		},
	})
	if err != nil {
		t.Fatalf("CompactConversationWithRequest returned error: %v", err)
	}
	if result.ToolResultsPruned != 2 {
		t.Fatalf("expected two structurally pruned tool results, got %+v", result)
	}
	if !strings.Contains(result.SummaryMessage.Content, "Structural tool results pruned: 2.") {
		t.Fatalf("expected summary metadata to mention structural prune count, got %q", result.SummaryMessage.Content)
	}
}

func TestCompactConversationSummaryFailureAddsFallbackMarker(t *testing.T) {
	messages := []convcontext.Message{
		{Role: convcontext.RoleUser, Content: "older context"},
		{Role: convcontext.RoleAssistant, Content: "older answer"},
		{Role: convcontext.RoleUser, Content: "latest tail question"},
	}

	result, err := CompactConversationWithRequest(stdcontext.Background(), CompactRequest{
		Mode:         CompactModeManual,
		Messages:     messages,
		Envelope:     ContextEnvelope{Messages: messages, Model: "test-model"},
		TargetTokens: 1,
		summaryInvoker: func(stdcontext.Context, llm.Config, llm.Request) (string, error) {
			return "", errors.New("provider unavailable")
		},
	})
	if err != nil {
		t.Fatalf("CompactConversationWithRequest returned error: %v", err)
	}
	if !result.Fallback.Used {
		t.Fatalf("expected fallback metadata, got %+v", result)
	}
	if !strings.Contains(result.SummaryMessage.Content, result.Fallback.Marker) {
		t.Fatalf("expected fallback marker in summary message, got %q", result.SummaryMessage.Content)
	}
	for _, section := range compactSummarySections {
		if !strings.Contains(result.SummaryMessage.Content, section) {
			t.Fatalf("expected fallback summary to include section %q, got %q", section, result.SummaryMessage.Content)
		}
	}
}

func TestCompactConversationEmptySummaryResponseUsesFallback(t *testing.T) {
	messages := []convcontext.Message{
		{Role: convcontext.RoleUser, Content: "older context"},
		{Role: convcontext.RoleAssistant, Content: "older answer"},
		{Role: convcontext.RoleUser, Content: "latest tail question"},
	}

	result, err := CompactConversationWithRequest(stdcontext.Background(), CompactRequest{
		Mode:         CompactModeManual,
		Messages:     messages,
		Envelope:     ContextEnvelope{Messages: messages, Model: "test-model"},
		TargetTokens: 1,
		summaryInvoker: func(stdcontext.Context, llm.Config, llm.Request) (string, error) {
			return "", nil
		},
	})
	if err != nil {
		t.Fatalf("CompactConversationWithRequest returned error: %v", err)
	}
	if !result.Fallback.Used {
		t.Fatalf("expected fallback metadata for empty summary response, got %+v", result)
	}
	if result.Fallback.Marker != "[COMPACT_FALLBACK:empty_summary_response]" {
		t.Fatalf("expected empty-summary fallback marker, got %+v", result.Fallback)
	}
	if !strings.Contains(result.SummaryMessage.Content, result.Fallback.Marker) {
		t.Fatalf("expected fallback marker in summary message, got %q", result.SummaryMessage.Content)
	}
	if strings.Contains(result.SummaryMessage.Content, "Active Task\n- None.") &&
		strings.Contains(result.SummaryMessage.Content, "Critical Context\n- None.") {
		t.Fatalf("expected explicit fallback summary, not all-None normalized success, got %q", result.SummaryMessage.Content)
	}
}

func TestCompactConversationPromptTooLongRetriesPruneOldestGroups(t *testing.T) {
	messages := make([]convcontext.Message, 0, 10)
	for i := 0; i < 10; i++ {
		messages = append(messages, convcontext.Message{
			Role:    convcontext.RoleUser,
			Content: "older group " + strings.Repeat(string(rune('a'+i)), 16),
		})
	}

	var attemptSizes []int
	result, err := CompactConversationWithRequest(stdcontext.Background(), CompactRequest{
		Mode:         CompactModeManual,
		Messages:     messages,
		Envelope:     ContextEnvelope{Messages: messages, Model: "test-model"},
		TargetTokens: 1,
		summaryInvoker: func(_ stdcontext.Context, _ llm.Config, req llm.Request) (string, error) {
			attemptSizes = append(attemptSizes, len(req.Messages)-1)
			if len(attemptSizes) < 3 {
				return "", errors.New("maximum context length exceeded")
			}
			return renderCompactSummarySections(map[string][]string{
				"Active Task":               {"- Compact older groups."},
				"Goal":                      {"- Succeed after pruning."},
				"Constraints & Preferences": {"- None."},
				"Completed Actions":         {"- Two retry prunes applied."},
				"Active State":              {"- None."},
				"In Progress":               {"- None."},
				"Blocked":                   {"- None."},
				"Key Decisions":             {"- Retry at most twice."},
				"Resolved Questions":        {"- None."},
				"Pending User Asks":         {"- None."},
				"Relevant Files":            {"- None."},
				"Remaining Work":            {"- Continue from protected tail."},
				"Critical Context":          {"- Retry succeeded."},
			}), nil
		},
	})
	if err != nil {
		t.Fatalf("CompactConversationWithRequest returned error: %v", err)
	}
	if want := []int{9, 7, 5}; !reflect.DeepEqual(attemptSizes, want) {
		t.Fatalf("expected retry pruning pattern %v, got %v", want, attemptSizes)
	}
	if result.PrunedSafeGroupCount != 4 || result.PrunedMessageCount != 4 {
		t.Fatalf("expected pruned counts to reflect two retry drops, got %+v", result)
	}
	if result.Fallback.Used {
		t.Fatalf("expected successful summary after retries, got fallback %+v", result.Fallback)
	}
}
