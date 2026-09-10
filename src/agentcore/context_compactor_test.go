package agentcore

import (
	stdcontext "context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	convcontext "awaken/src/context"
	"awaken/src/llm"
	"awaken/src/tool"
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
	toolCanary := "critical canary tool conclusion: checksum alpha-42"
	largeToolResult := strings.Join([]string{
		"tool output",
		"status=ok",
		"path=README.md",
		strings.Repeat("x", 3000),
		toolCanary,
		"error_count=0",
	}, "\n")
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
					if !strings.Contains(message.Content, toolCanary) ||
						!strings.Contains(message.Content, "README.md") ||
						!strings.Contains(message.Content, "status=ok") ||
						!strings.Contains(message.Content, "error_count=0") {
						t.Fatalf("expected structural digest to preserve key facts, got %q", message.Content)
					}
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
	if result.ToolResultsPrunedCount != 1 {
		t.Fatalf("expected explicit pruned count alias, got %+v", result)
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

func TestCompactConversationPinsLatestAssistantProgressOutsideModelSummary(t *testing.T) {
	messages := []convcontext.Message{
		{Role: convcontext.RoleUser, Content: "process 24 files"},
		{Role: convcontext.RoleAssistant, Content: "21/24 complete; continue the final three files."},
		{Role: convcontext.RoleUser, Content: "keep going after compact"},
	}
	result, err := CompactConversationWithRequest(stdcontext.Background(), CompactRequest{
		Mode:         CompactModeManual,
		Messages:     messages,
		Envelope:     ContextEnvelope{Messages: messages, Model: "test-model"},
		TargetTokens: 1,
		summaryInvoker: func(_ stdcontext.Context, _ llm.Config, req llm.Request) (string, error) {
			instruction := req.Messages[len(req.Messages)-1].Content
			if !strings.Contains(instruction, "Latest older assistant progress marker that MUST be preserved") ||
				!strings.Contains(instruction, "21/24 complete") {
				t.Fatalf("summary request did not pin latest progress: %s", instruction)
			}
			return renderCompactSummarySections(map[string][]string{
				"Active Task":       {"- Process the requested files."},
				"Completed Actions": {"- Earlier files were processed."},
				"Remaining Work":    {"- Continue from the protected tail."},
				"Critical Context":  {"- Preserve the exact progress marker."},
			}), nil
		},
	})
	if err != nil {
		t.Fatalf("CompactConversationWithRequest returned error: %v", err)
	}
	if !strings.Contains(result.SummaryMessage.Content, "Latest compacted assistant progress: 21/24 complete; continue the final three files.") {
		t.Fatalf("host-owned compact header lost latest progress: %s", result.SummaryMessage.Content)
	}
}

func TestCompactConversationSummaryFailureAddsFallbackMarker(t *testing.T) {
	fallbackFact := "fallback canary fact: preserve customer tier platinum"
	messages := []convcontext.Message{
		{Role: convcontext.RoleUser, Content: fallbackFact},
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
	if result.FallbackMarker != result.Fallback.Marker {
		t.Fatalf("expected fallback marker alias to be populated, got %+v", result)
	}
	if !strings.Contains(result.SummaryMessage.Content, fallbackFact) {
		t.Fatalf("expected fallback summary to preserve deterministic facts, got %q", result.SummaryMessage.Content)
	}
	if strings.Contains(result.SummaryMessage.Content, "Critical Context\n- "+result.Fallback.Marker) {
		t.Fatalf("expected fallback marker to stay out of handoff sections, got %q", result.SummaryMessage.Content)
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

func TestCompactConversationLowInformationSemanticSummaryUsesFallback(t *testing.T) {
	userAsk := "deeply research this project and find task design defects"
	messages := []convcontext.Message{
		{Role: convcontext.RoleUser, Content: userAsk},
		{Role: convcontext.RoleAssistant, Content: "I will inspect the repository."},
		{Role: convcontext.RoleUser, Content: "latest tail question"},
	}
	sawLatestUserInInstruction := false

	result, err := CompactConversationWithRequest(stdcontext.Background(), CompactRequest{
		Mode:         CompactModeManual,
		Messages:     messages,
		Envelope:     ContextEnvelope{Messages: messages, Model: "test-model"},
		TargetTokens: 1,
		summaryInvoker: func(_ stdcontext.Context, _ llm.Config, req llm.Request) (string, error) {
			if len(req.Messages) == 0 {
				t.Fatalf("expected compact summary request messages")
			}
			instruction := req.Messages[len(req.Messages)-1].Content
			if strings.Contains(instruction, "Latest older user request that MUST be preserved") &&
				strings.Contains(instruction, userAsk) {
				sawLatestUserInInstruction = true
			}
			return renderCompactSummarySections(map[string][]string{
				"Active Task":               {"- None."},
				"Goal":                      {"- None."},
				"Constraints & Preferences": {"- None."},
				"Completed Actions":         {"- None."},
				"Active State":              {"- None."},
				"In Progress":               {"- None."},
				"Blocked":                   {"- None."},
				"Key Decisions":             {"- None."},
				"Resolved Questions":        {"- None."},
				"Pending User Asks":         {"- None."},
				"Relevant Files":            {"- None."},
				"Remaining Work":            {"- None."},
				"Critical Context":          {"- None."},
			}), nil
		},
	})
	if err != nil {
		t.Fatalf("CompactConversationWithRequest returned error: %v", err)
	}
	if !sawLatestUserInInstruction {
		t.Fatalf("expected summary instruction to explicitly pin the latest older user request")
	}
	if !result.Fallback.Used {
		t.Fatalf("expected low-information semantic summary to use fallback, got %+v", result)
	}
	if result.Fallback.Marker != "[COMPACT_FALLBACK:low_information_summary]" {
		t.Fatalf("expected low-information fallback marker, got %+v", result.Fallback)
	}
	if result.SummaryRawChars == 0 ||
		result.SummaryParsedSections != len(compactSummarySections) ||
		result.SummaryMeaningfulSections != 0 ||
		result.SummaryQualityIssue == "" ||
		!result.SummarySourceHadUser ||
		!result.SummaryLatestUserPinned {
		t.Fatalf("expected all-None summary diagnostics to be populated, got %+v", result)
	}
	if !strings.Contains(result.SummaryMessage.Content, userAsk) {
		t.Fatalf("expected deterministic fallback to preserve user ask, got %q", result.SummaryMessage.Content)
	}
	if strings.Contains(result.SummaryMessage.Content, "Active Task\n- None.") &&
		strings.Contains(result.SummaryMessage.Content, "Pending User Asks\n- None.") &&
		strings.Contains(result.SummaryMessage.Content, "Critical Context\n- None.") {
		t.Fatalf("expected low-information fallback to avoid all-empty handoff, got %q", result.SummaryMessage.Content)
	}
}

func TestCompactConversationMalformedSemanticSummaryKeepsNarrative(t *testing.T) {
	userAsk := "audit compact parser behavior"
	prose := "I cannot determine anything useful from this request, but the earlier audit of compact parser behavior must continue."
	messages := []convcontext.Message{
		{Role: convcontext.RoleUser, Content: userAsk},
		{Role: convcontext.RoleAssistant, Content: "I will inspect compact parsing."},
		{Role: convcontext.RoleUser, Content: "latest tail question"},
	}

	result, err := CompactConversationWithRequest(stdcontext.Background(), CompactRequest{
		Mode:         CompactModeManual,
		Messages:     messages,
		Envelope:     ContextEnvelope{Messages: messages, Model: "test-model"},
		TargetTokens: 1,
		summaryInvoker: func(_ stdcontext.Context, _ llm.Config, _ llm.Request) (string, error) {
			return prose, nil
		},
	})
	if err != nil {
		t.Fatalf("CompactConversationWithRequest returned error: %v", err)
	}
	if result.Fallback.Used {
		t.Fatalf("expected unstructured prose to install without fallback, got %+v", result)
	}
	if result.SummaryRawChars == 0 ||
		result.SummaryParsedSections != 0 ||
		result.SummaryMeaningfulSections != 0 ||
		result.SummaryQualityIssue == "" ||
		!result.SummarySourceHadUser ||
		!result.SummaryLatestUserPinned {
		t.Fatalf("expected parser diagnostics to distinguish malformed raw summary, got %+v", result)
	}
	if !strings.Contains(result.SummaryMessage.Content, prose) {
		t.Fatalf("expected installed summary to keep the raw narrative, got %q", result.SummaryMessage.Content)
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
	var sawDigestOnRetry bool
	result, err := CompactConversationWithRequest(stdcontext.Background(), CompactRequest{
		Mode:         CompactModeManual,
		Messages:     messages,
		Envelope:     ContextEnvelope{Messages: messages, Model: "test-model"},
		TargetTokens: 1,
		summaryInvoker: func(_ stdcontext.Context, _ llm.Config, req llm.Request) (string, error) {
			attemptSizes = append(attemptSizes, len(req.Messages)-1)
			if len(attemptSizes) > 1 {
				joined := ""
				for _, message := range req.Messages {
					joined += message.Content + "\n"
				}
				if strings.Contains(joined, "[PRUNED_SAFE_GROUP_DIGEST") {
					sawDigestOnRetry = true
				}
			}
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
	if want := []int{9, 8, 7}; !reflect.DeepEqual(attemptSizes, want) {
		t.Fatalf("expected retry pruning pattern %v, got %v", want, attemptSizes)
	}
	if !sawDigestOnRetry {
		t.Fatalf("expected retry requests to include deterministic pruned group digest")
	}
	if result.PrunedSafeGroupCount != 4 || result.PrunedMessageCount != 4 {
		t.Fatalf("expected pruned counts to reflect two retry drops, got %+v", result)
	}
	if result.PrunedDigestMessageCount != 2 || result.SummaryRetryCount != 2 {
		t.Fatalf("expected digest and retry metadata, got %+v", result)
	}
	if result.Fallback.Used {
		t.Fatalf("expected successful summary after retries, got fallback %+v", result.Fallback)
	}
}

func TestCompactConversationUsesDedicatedSummaryBudget(t *testing.T) {
	messages := []convcontext.Message{
		{Role: convcontext.RoleUser, Content: "old fact"},
		{Role: convcontext.RoleAssistant, Content: "old answer"},
		{Role: convcontext.RoleUser, Content: "latest tail question"},
	}

	result, err := CompactConversationWithRequest(stdcontext.Background(), CompactRequest{
		Mode:         CompactModeManual,
		Messages:     messages,
		Envelope:     ContextEnvelope{Messages: messages, Model: "test-model", MaxTokens: 256},
		TargetTokens: 1,
		summaryInvoker: func(_ stdcontext.Context, _ llm.Config, req llm.Request) (string, error) {
			if req.MaxTokens != compactSummaryDefaultMaxTokens {
				t.Fatalf("expected compact summary budget %d, got %d", compactSummaryDefaultMaxTokens, req.MaxTokens)
			}
			return renderCompactSummarySections(map[string][]string{
				"Active Task":               {"- old fact"},
				"Goal":                      {"- Preserve facts."},
				"Constraints & Preferences": {"- None."},
				"Completed Actions":         {"- None."},
				"Active State":              {"- None."},
				"In Progress":               {"- None."},
				"Blocked":                   {"- None."},
				"Key Decisions":             {"- None."},
				"Resolved Questions":        {"- None."},
				"Pending User Asks":         {"- None."},
				"Relevant Files":            {"- None."},
				"Remaining Work":            {"- Continue."},
				"Critical Context":          {"- old fact"},
			}), nil
		},
	})
	if err != nil {
		t.Fatalf("CompactConversationWithRequest returned error: %v", err)
	}
	if result.SummaryMaxTokens != compactSummaryDefaultMaxTokens {
		t.Fatalf("expected result to record summary budget, got %+v", result)
	}
	if !strings.Contains(result.SummaryMessage.Content, "Compact summary max tokens: 4096.") {
		t.Fatalf("expected summary header to record budget, got %q", result.SummaryMessage.Content)
	}
}

func TestCompactConversationSummaryRequestStreamsWithReasoningDisabled(t *testing.T) {
	messages := []convcontext.Message{
		{Role: convcontext.RoleUser, Content: "old fact"},
		{Role: convcontext.RoleAssistant, Content: "old answer"},
		{Role: convcontext.RoleUser, Content: "latest tail question"},
	}

	result, err := CompactConversationWithRequest(stdcontext.Background(), CompactRequest{
		Mode:         CompactModeManual,
		Messages:     messages,
		Envelope:     ContextEnvelope{Messages: messages, Model: "kimi-for-coding"},
		TargetTokens: 1,
		summaryInvoker: func(_ stdcontext.Context, _ llm.Config, req llm.Request) (string, error) {
			if !req.Stream {
				t.Fatalf("expected compact summary request to use streaming")
			}
			if req.StreamIdleTimeout != compactSummaryStreamIdleTimeout {
				t.Fatalf("expected compact summary stream idle timeout %s, got %s", compactSummaryStreamIdleTimeout, req.StreamIdleTimeout)
			}
			if !req.DisableReasoning {
				t.Fatalf("expected compact summary request to disable reasoning")
			}
			return renderCompactSummarySections(map[string][]string{
				"Active Task":               {"- old fact"},
				"Goal":                      {"- Preserve facts."},
				"Constraints & Preferences": {"- None."},
				"Completed Actions":         {"- None."},
				"Active State":              {"- None."},
				"In Progress":               {"- None."},
				"Blocked":                   {"- None."},
				"Key Decisions":             {"- None."},
				"Resolved Questions":        {"- None."},
				"Pending User Asks":         {"- None."},
				"Relevant Files":            {"- None."},
				"Remaining Work":            {"- Continue."},
				"Critical Context":          {"- old fact"},
			}), nil
		},
	})
	if err != nil {
		t.Fatalf("CompactConversationWithRequest returned error: %v", err)
	}
	if result.Fallback.Used {
		t.Fatalf("expected streaming summary request to succeed without fallback, got %+v", result.Fallback)
	}
}

func TestCompactConversationDefaultSummaryInvokerPreservesKimiK27Thinking(t *testing.T) {
	messages := []convcontext.Message{
		{Role: convcontext.RoleUser, Content: "old fact"},
		{Role: convcontext.RoleAssistant, Content: "old answer"},
		{Role: convcontext.RoleUser, Content: "latest tail question"},
	}
	var gotStream bool
	var gotThinkingType string
	var gotTemperature float64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("expected /chat/completions, got %s", r.URL.Path)
		}
		var body struct {
			Stream      bool    `json:"stream"`
			Temperature float64 `json:"temperature"`
			Thinking    struct {
				Type string `json:"type"`
			} `json:"thinking"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode compact summary request: %v", err)
		}
		gotStream = body.Stream
		gotThinkingType = body.Thinking.Type
		gotTemperature = body.Temperature
		w.Header().Set("Content-Type", "text/event-stream")
		payload, err := json.Marshal(map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"delta": map[string]string{
						"content": renderCompactSummarySections(map[string][]string{
							"Active Task":               {"- old fact"},
							"Goal":                      {"- Preserve facts."},
							"Constraints & Preferences": {"- None."},
							"Completed Actions":         {"- None."},
							"Active State":              {"- None."},
							"In Progress":               {"- None."},
							"Blocked":                   {"- None."},
							"Key Decisions":             {"- None."},
							"Resolved Questions":        {"- None."},
							"Pending User Asks":         {"- None."},
							"Relevant Files":            {"- None."},
							"Remaining Work":            {"- Continue."},
							"Critical Context":          {"- old fact"},
						}),
					},
				},
			},
		})
		if err != nil {
			t.Fatalf("failed to marshal stream payload: %v", err)
		}
		_, _ = w.Write([]byte("data: " + string(payload) + "\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	result, err := CompactConversationWithRequest(stdcontext.Background(), CompactRequest{
		Mode:     CompactModeManual,
		Messages: messages,
		Envelope: ContextEnvelope{
			Messages: messages,
			Model:    "kimi-for-coding",
			CompactModelConfig: ModelConfig{
				APIKey:   "test-key",
				BaseURL:  server.URL,
				Model:    "kimi-for-coding",
				Protocol: string(llm.ProtocolOpenAIChat),
			},
		},
		TargetTokens: 1,
	})
	if err != nil {
		t.Fatalf("CompactConversationWithRequest returned error: %v", err)
	}
	if !gotStream {
		t.Fatalf("expected default compact summary invoker to use streaming")
	}
	if gotThinkingType != "" {
		t.Fatalf("expected Kimi K2.7 compact summary to omit thinking.disabled, got %q", gotThinkingType)
	}
	if gotTemperature != 1 {
		t.Fatalf("expected Kimi K2.7 compact summary temperature 1, got %v", gotTemperature)
	}
	if result.Fallback.Used {
		t.Fatalf("expected streamed compact summary to avoid fallback, got %+v", result.Fallback)
	}
	if !strings.Contains(result.SummaryMessage.Content, "old fact") {
		t.Fatalf("expected streamed summary to be stored, got %q", result.SummaryMessage.Content)
	}
}

func TestCompactConversationDefaultSummaryInvokerRetriesProviderRequiredTemperature(t *testing.T) {
	messages := []convcontext.Message{
		{Role: convcontext.RoleUser, Content: "old fact"},
		{Role: convcontext.RoleAssistant, Content: "old answer"},
		{Role: convcontext.RoleUser, Content: "latest tail question"},
	}
	attempts := 0
	temperatures := make([]float64, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("expected /chat/completions, got %s", r.URL.Path)
		}
		var body struct {
			Temperature float64 `json:"temperature"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode compact summary request: %v", err)
		}
		attempts++
		temperatures = append(temperatures, body.Temperature)
		if attempts == 1 {
			http.Error(w, "invalid temperature: only 0.6 is allowed for this model", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		payload, err := json.Marshal(map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"delta": map[string]string{
						"content": renderCompactSummarySections(map[string][]string{
							"Active Task":               {"- old fact"},
							"Goal":                      {"- Preserve facts."},
							"Constraints & Preferences": {"- None."},
							"Completed Actions":         {"- None."},
							"Active State":              {"- None."},
							"In Progress":               {"- None."},
							"Blocked":                   {"- None."},
							"Key Decisions":             {"- None."},
							"Resolved Questions":        {"- None."},
							"Pending User Asks":         {"- None."},
							"Relevant Files":            {"- None."},
							"Remaining Work":            {"- Continue."},
							"Critical Context":          {"- old fact"},
						}),
					},
				},
			},
		})
		if err != nil {
			t.Fatalf("failed to marshal stream payload: %v", err)
		}
		_, _ = w.Write([]byte("data: " + string(payload) + "\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	result, err := CompactConversationWithRequest(stdcontext.Background(), CompactRequest{
		Mode:     CompactModeManual,
		Messages: messages,
		Envelope: ContextEnvelope{
			Messages:    messages,
			Model:       "custom-model",
			Temperature: 1.0,
			CompactModelConfig: ModelConfig{
				APIKey:      "test-key",
				BaseURL:     server.URL,
				Model:       "custom-model",
				Protocol:    string(llm.ProtocolOpenAIChat),
				Temperature: 1.0,
			},
		},
		TargetTokens: 1,
	})
	if err != nil {
		t.Fatalf("CompactConversationWithRequest returned error: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("expected provider-required temperature retry, got attempts=%d temperatures=%v", attempts, temperatures)
	}
	if len(temperatures) != 2 || temperatures[0] != 1.0 || temperatures[1] != 0.6 {
		t.Fatalf("expected retry to switch temperature from 1.0 to 0.6, got %v", temperatures)
	}
	if result.Fallback.Used {
		t.Fatalf("expected retry summary to avoid fallback, got %+v", result.Fallback)
	}
}

func TestCompactConversationPreservesCanaryUserAndToolFacts(t *testing.T) {
	userFact := "legacy user fact: preferred deployment window is Friday 18:00"
	userPreference := "legacy preference: keep answers in Chinese"
	toolFact := "critical canary tool conclusion: invoice total is 4242 USD"
	pendingAsk := "pending user ask: produce the final rollout checklist"
	messages := []convcontext.Message{
		{Role: convcontext.RoleUser, Content: userFact},
		{Role: convcontext.RoleAssistant, Content: "Noted."},
		{Role: convcontext.RoleUser, Content: userPreference},
		{
			Role:    convcontext.RoleAssistant,
			Content: "calling tool",
			ToolCalls: []convcontext.ToolCall{{
				ID: "call-canary",
				Function: convcontext.FunctionCall{
					Name:      "read_report",
					Arguments: `{"path":"reports/invoice.json"}`,
				},
			}},
		},
		{Role: convcontext.RoleTool, ToolCallID: "call-canary", Name: "read_report", Content: strings.Repeat("padding\n", 400) + toolFact + "\npath=reports/invoice.json"},
		{Role: convcontext.RoleUser, Content: pendingAsk},
	}

	result, err := CompactConversationWithRequest(stdcontext.Background(), CompactRequest{
		Mode:         CompactModeManual,
		Messages:     messages,
		Envelope:     ContextEnvelope{Messages: messages, Model: "test-model"},
		TargetTokens: 1,
		summaryInvoker: func(_ stdcontext.Context, _ llm.Config, req llm.Request) (string, error) {
			joined := ""
			for _, message := range req.Messages {
				joined += message.Content + "\n"
			}
			for _, want := range []string{userFact, userPreference, toolFact, "reports/invoice.json"} {
				if !strings.Contains(joined, want) {
					t.Fatalf("expected summary request to preserve %q, got %q", want, joined)
				}
			}
			return renderCompactSummarySections(map[string][]string{
				"Active Task":               {"- " + pendingAsk},
				"Goal":                      {"- Preserve low-distortion handoff continuity."},
				"Constraints & Preferences": {"- " + userPreference},
				"Completed Actions":         {"- read_report executed."},
				"Active State":              {"- Ready to produce rollout checklist."},
				"In Progress":               {"- None."},
				"Blocked":                   {"- None."},
				"Key Decisions":             {"- None."},
				"Resolved Questions":        {"- None."},
				"Pending User Asks":         {"- " + pendingAsk},
				"Relevant Files":            {"- reports/invoice.json"},
				"Remaining Work":            {"- Produce rollout checklist."},
				"Critical Context":          {"- " + userFact, "- " + toolFact},
			}), nil
		},
	})
	if err != nil {
		t.Fatalf("CompactConversationWithRequest returned error: %v", err)
	}
	for _, want := range []string{userFact, userPreference, toolFact, pendingAsk, "reports/invoice.json"} {
		if !strings.Contains(result.SummaryMessage.Content, want) {
			t.Fatalf("expected compact summary to preserve %q, got %q", want, result.SummaryMessage.Content)
		}
	}
}

func TestCompactSummaryRequestStripsMediaPayloads(t *testing.T) {
	messages := []convcontext.Message{
		{Role: convcontext.RoleUser, Content: "read the deck"},
		{
			Role: convcontext.RoleAssistant,
			ToolCalls: []convcontext.ToolCall{{
				ID: "call-1", Function: convcontext.FunctionCall{Name: "Read", Arguments: `{"file_path":"deck.pdf"}`},
			}},
		},
		{
			Role: convcontext.RoleTool, ToolCallID: "call-1", Name: "Read",
			Content: "page image " + strings.Repeat("x", 400),
			ModelContent: []tool.ModelContentPart{{
				Type: "image", MimeType: "image/png", DisplayName: "deck.pdf", Page: 1,
				DataBase64: "aW1hZ2U=", StorageKey: "thread/tool-results/call/part-001.png", ByteSize: 8,
			}},
		},
		{Role: convcontext.RoleUser, Content: "latest tail question"},
	}

	result, err := CompactConversationWithRequest(stdcontext.Background(), CompactRequest{
		Mode:         CompactModeManual,
		Messages:     messages,
		Envelope:     ContextEnvelope{Messages: messages, Model: "test-model"},
		TargetTokens: 1,
		summaryInvoker: func(_ stdcontext.Context, _ llm.Config, req llm.Request) (string, error) {
			for _, message := range req.Messages {
				for _, part := range message.ModelContent {
					if strings.TrimSpace(part.DataBase64) != "" {
						t.Fatalf("summary request leaked media payload: %+v", part)
					}
				}
				for _, attachment := range message.Attachments {
					if strings.TrimSpace(attachment.DataBase64) != "" {
						t.Fatalf("summary request leaked attachment payload: %+v", attachment)
					}
				}
			}
			return renderCompactSummarySections(map[string][]string{
				"Active Task":               {"- Summarize the deck."},
				"Goal":                      {"- Keep images out of the summary request."},
				"Constraints & Preferences": {"- None."},
				"Completed Actions":         {"- Read deck.pdf."},
				"Active State":              {"- None."},
				"In Progress":               {"- None."},
				"Blocked":                   {"- None."},
				"Key Decisions":             {"- None."},
				"Resolved Questions":        {"- None."},
				"Pending User Asks":         {"- None."},
				"Relevant Files":            {"- deck.pdf"},
				"Remaining Work":            {"- Continue from the protected tail."},
				"Critical Context":          {"- Older page images were stubbed."},
			}), nil
		},
	})
	if err != nil {
		t.Fatalf("CompactConversationWithRequest returned error: %v", err)
	}
	if result.Fallback.Used {
		t.Fatalf("did not expect fallback, got %+v", result.Fallback)
	}
}

func TestCompactOversizedTailAbsorbsClosedMediaGroup(t *testing.T) {
	messages := []convcontext.Message{
		{Role: convcontext.RoleUser, Content: "read these pages"},
	}
	calls := make([]convcontext.ToolCall, 0, 14)
	for i := 1; i <= 14; i++ {
		calls = append(calls, convcontext.ToolCall{
			ID:       "call-" + strings.Repeat("x", 0) + itoaCompact(i),
			Function: convcontext.FunctionCall{Name: "Read", Arguments: `{"file_path":"deck.pdf","pages":"1-10"}`},
		})
	}
	messages = append(messages, convcontext.Message{Role: convcontext.RoleAssistant, ToolCalls: calls})
	for i := 1; i <= 14; i++ {
		parts := make([]tool.ModelContentPart, 0, 10)
		for page := 1; page <= 10; page++ {
			parts = append(parts, tool.ModelContentPart{
				Type: "image", MimeType: "image/png", DisplayName: "deck.pdf", Page: page,
				DataBase64: "aW1hZ2U=", StorageKey: "thread/tool-results/call/part.png", ByteSize: 8,
			})
		}
		messages = append(messages, convcontext.Message{
			Role: convcontext.RoleTool, ToolCallID: "call-" + itoaCompact(i), Name: "Read",
			Content: "pages", ModelContent: parts,
		})
	}

	result, err := CompactConversationWithRequest(stdcontext.Background(), CompactRequest{
		Mode:     CompactModeManual,
		Messages: messages,
		Envelope: ContextEnvelope{
			Messages: messages, Model: "test-model",
			CompactModelConfig: ModelConfig{ContextWindow: 40000},
		},
		TargetTokens: 1,
		summaryInvoker: func(_ stdcontext.Context, _ llm.Config, req llm.Request) (string, error) {
			for _, message := range req.Messages {
				for _, part := range message.ModelContent {
					if strings.TrimSpace(part.DataBase64) != "" {
						t.Fatalf("summary request still carried page images: %+v", part)
					}
				}
			}
			return renderCompactSummarySections(map[string][]string{
				"Active Task":               {"- Read these pages."},
				"Goal":                      {"- Absorb the oversized media tail."},
				"Constraints & Preferences": {"- None."},
				"Completed Actions":         {"- Read deck.pdf."},
				"Active State":              {"- None."},
				"In Progress":               {"- None."},
				"Blocked":                   {"- None."},
				"Key Decisions":             {"- None."},
				"Resolved Questions":        {"- None."},
				"Pending User Asks":         {"- None."},
				"Relevant Files":            {"- deck.pdf"},
				"Remaining Work":            {"- Continue from the latest user request."},
				"Critical Context":          {"- 140 page images moved into the prefix."},
			}), nil
		},
	})
	if err != nil {
		t.Fatalf("CompactConversationWithRequest returned error: %v", err)
	}
	if len(result.TailMessages) != 1 || result.TailMessages[0].Content != "read these pages" {
		t.Fatalf("expected absorbed tail to keep only the latest user, got %+v", result.TailMessages)
	}
	for _, message := range result.TailMessages {
		for _, part := range message.ModelContent {
			if strings.TrimSpace(part.DataBase64) != "" {
				t.Fatalf("compact tail still carried DataBase64: %+v", part)
			}
		}
	}
}

func TestCompactOversizedUnclosedPairingIsRejected(t *testing.T) {
	parts := make([]tool.ModelContentPart, 0, 40)
	for page := 1; page <= 40; page++ {
		parts = append(parts, tool.ModelContentPart{
			Type: "image", MimeType: "image/png", DisplayName: "deck.pdf", Page: page,
			DataBase64: "aW1hZ2U=", StorageKey: "thread/tool-results/call/part.png", ByteSize: 8,
		})
	}
	messages := []convcontext.Message{
		{Role: convcontext.RoleUser, Content: "read these pages"},
		{
			Role: convcontext.RoleAssistant,
			ToolCalls: []convcontext.ToolCall{
				{ID: "call-1", Function: convcontext.FunctionCall{Name: "Read"}},
				{ID: "call-2", Function: convcontext.FunctionCall{Name: "Read"}},
			},
		},
		{Role: convcontext.RoleTool, ToolCallID: "call-1", Name: "Read", Content: "pages", ModelContent: parts},
	}
	_, err := CompactConversationWithRequest(stdcontext.Background(), CompactRequest{
		Mode:     CompactModeManual,
		Messages: messages,
		Envelope: ContextEnvelope{
			Messages: messages, Model: "test-model",
			CompactModelConfig: ModelConfig{ContextWindow: 40000},
		},
		TargetTokens: 1,
		summaryInvoker: func(stdcontext.Context, llm.Config, llm.Request) (string, error) {
			return "should not run", nil
		},
	})
	if !errors.Is(err, errOversizedProtectedTail) {
		t.Fatalf("expected oversized protected tail, got %v", err)
	}
}

func TestCompactTailLimitHonorsGlobalAllocatedTailBudget(t *testing.T) {
	tail := []convcontext.Message{{
		Role: convcontext.RoleTool, Name: "Bash", ToolCallID: "call-closed",
		Content: strings.Repeat("office text ", 2000),
	}}
	over, limit := compactTailExceedsBudget(CompactRequest{
		TargetTokens: 100,
		Envelope:     ContextEnvelope{CompactModelConfig: ModelConfig{ContextWindow: 100000}},
	}, tail)
	if !over || limit != 100 {
		t.Fatalf("tail allocator was overridden by the larger whole-request target: over=%v limit=%d", over, limit)
	}
}

func TestCompactConversationDropsProviderOpaqueReplayState(t *testing.T) {
	state := &convcontext.ProviderOpaqueMessageState{
		ProviderID: "openai", ProfileID: "profile", CredentialRevision: 1,
		ModelID: "model", ProtocolFamily: "openai_chat", EndpointContractID: "official",
		Content: strings.Repeat("private-reasoning", 100),
	}
	messages := []convcontext.Message{
		{Role: convcontext.RoleUser, Content: strings.Repeat("old task context ", 1200)},
		{Role: convcontext.RoleAssistant, Content: "old answer", ProviderOpaqueState: state},
		{Role: convcontext.RoleUser, Content: "latest request"},
		{Role: convcontext.RoleAssistant, Content: "latest answer", ProviderOpaqueState: state},
	}
	result, err := CompactConversationWithRequest(stdcontext.Background(), CompactRequest{
		Mode: CompactModeAuto, Messages: messages, Envelope: ContextEnvelope{Messages: messages}, TargetTokens: 400,
		summaryInvoker: func(stdcontext.Context, llm.Config, llm.Request) (string, error) {
			return renderCompactSummarySections(map[string][]string{
				"Active Task": {"- Keep the current task."}, "Goal": {"- Continue safely."},
				"Constraints & Preferences": {"- None."}, "Completed Actions": {"- None."},
				"Active State": {"- None."}, "In Progress": {"- None."}, "Blocked": {"- None."},
				"Key Decisions": {"- None."}, "Resolved Questions": {"- None."},
				"Pending User Asks": {"- latest request"}, "Relevant Files": {"- None."},
				"Remaining Work": {"- Answer the latest request."}, "Critical Context": {"- Opaque reasoning is not portable."},
			}), nil
		},
	})
	if err != nil {
		t.Fatalf("compact conversation: %v", err)
	}
	for index, message := range result.Messages {
		if message.ProviderOpaqueState != nil {
			t.Fatalf("message %d retained provider opaque replay state", index)
		}
	}
}

func itoaCompact(value int) string {
	if value == 0 {
		return "0"
	}
	digits := make([]byte, 0, 4)
	for value > 0 {
		digits = append([]byte{byte('0' + value%10)}, digits...)
		value /= 10
	}
	return string(digits)
}
