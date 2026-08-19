package agentcore

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	convcontext "awaken/src/context"
	"awaken/src/tool"
	"awaken/src/tools/read"
)

type countingMediaReadTool struct {
	executes *int
}

func (t countingMediaReadTool) Name() string { return read.ToolName }
func (t countingMediaReadTool) Description() string {
	return "Test Read that returns one image part."
}
func (t countingMediaReadTool) Parameters() []tool.ToolParameter {
	return []tool.ToolParameter{
		{Name: "file_path", Description: "Path", Type: "string", Required: true},
		{Name: "pages", Description: "Pages", Type: "string"},
	}
}
func (t countingMediaReadTool) Definition() tool.ToolDefinition {
	return tool.ToolDefinition{
		Name:        read.ToolName,
		DisplayName: "Read",
		Source:      tool.SourceBuiltin,
		Description: t.Description(),
		Annotations: tool.ToolAnnotations{ReadOnly: true},
	}
}
func (t countingMediaReadTool) Execute(context.Context, string) (string, error) {
	return "image result", nil
}
func (t countingMediaReadTool) ExecuteOutcome(context.Context, string, tool.ToolObserver) (tool.ExecutionOutcome, error) {
	if t.executes != nil {
		*t.executes++
	}
	outcome := tool.SucceededOutcome("image result")
	outcome.ModelContent = []tool.ModelContentPart{{
		Type:        "image",
		MimeType:    "image/png",
		DisplayName: "page.png",
		DataBase64:  "aW1hZ2U=",
	}}
	return outcome, nil
}

func TestEstimateReadMediaAdmissionTokens(t *testing.T) {
	if got := estimateReadMediaAdmissionTokens(`{"file_path":"notes.md"}`); got != 0 {
		t.Fatalf("text read estimate=%d", got)
	}
	if got := estimateReadMediaAdmissionTokens(`{"file_path":"chart.png"}`); got != mediaAdmissionTokensPerPart {
		t.Fatalf("image estimate=%d", got)
	}
	if got := estimateReadMediaAdmissionTokens(`{"file_path":"report.pdf"}`); got != defaultOmittedPDFAdmissionPages*mediaAdmissionTokensPerPart {
		t.Fatalf("omitted pdf pages estimate=%d", got)
	}
	if got := estimateReadMediaAdmissionTokens(`{"file_path":"report.pdf","pages":"1-10"}`); got != 10*mediaAdmissionTokensPerPart {
		t.Fatalf("explicit 1-10 estimate=%d", got)
	}
	if got := estimateReadMediaAdmissionTokens(`{"file_path":"report.pdf","pages":"1,3,7-9"}`); got != 5*mediaAdmissionTokensPerPart {
		t.Fatalf("sparse pages estimate=%d", got)
	}
}

func TestExecuteToolCallsAdmitsOnlyFittingMediaReads(t *testing.T) {
	original, err := tool.GetTool(read.ToolName)
	if err != nil {
		t.Fatalf("original Read: %v", err)
	}
	var executes int
	if err := tool.RegisterToolForce(countingMediaReadTool{executes: &executes}); err != nil {
		t.Fatalf("register fake Read: %v", err)
	}
	t.Cleanup(func() {
		if restoreErr := tool.RegisterToolForce(original); restoreErr != nil {
			t.Errorf("restore Read: %v", restoreErr)
		}
	})

	cfg := testAgentConfig("http://unused", 3)
	cfg.ModelConfig.ContextWindow = 40_000
	cfg.ModelConfig.Capabilities.SupportsVision = true
	conv := convcontext.NewConversation(nil)
	runner := NewRunner(cfg, conv, NewRequestBuilder(cfg))
	padConversationToAdmissionRemaining(t, runner, conv, 4_500)

	calls := make([]convcontext.ToolCall, 0, 14)
	for i := 0; i < 14; i++ {
		calls = append(calls, convcontext.ToolCall{
			ID:   "read_img_" + itoa(i),
			Type: "function",
			Function: convcontext.FunctionCall{
				Name:      read.ToolName,
				Arguments: `{"file_path":"chart.png"}`,
			},
		})
	}

	results := runner.executeToolCalls(context.Background(), calls, nil)
	if len(results) != 14 {
		t.Fatalf("results=%d", len(results))
	}
	if executes == 0 || executes >= 14 {
		t.Fatalf("expected a prefix of media Reads to execute, got executes=%d", executes)
	}
	for i, result := range results {
		if i < executes {
			if result.IsError || result.ErrorCode == ErrorCodeContextAdmissionExhausted {
				t.Fatalf("result %d should have executed, got %+v", i, result)
			}
			continue
		}
		if result.ErrorCode != ErrorCodeContextAdmissionExhausted {
			t.Fatalf("result %d should be admission exhausted, got %+v", i, result)
		}
	}
	if err := convcontext.ValidateToolResultPairing(append(
		[]convcontext.Message{{Role: convcontext.RoleAssistant, ToolCalls: calls}},
		toolResultsAsMessages(results)...,
	)); err != nil {
		t.Fatalf("pairing: %v", err)
	}
}

func TestExecuteToolCallsDoesNotAdmitGateTextReads(t *testing.T) {
	original, err := tool.GetTool(read.ToolName)
	if err != nil {
		t.Fatalf("original Read: %v", err)
	}
	var executes int
	if err := tool.RegisterToolForce(countingMediaReadTool{executes: &executes}); err != nil {
		t.Fatalf("register fake Read: %v", err)
	}
	t.Cleanup(func() {
		if restoreErr := tool.RegisterToolForce(original); restoreErr != nil {
			t.Errorf("restore Read: %v", restoreErr)
		}
	})

	cfg := testAgentConfig("http://unused", 3)
	cfg.ModelConfig.ContextWindow = 40_000
	conv := convcontext.NewConversation(nil)
	runner := NewRunner(cfg, conv, NewRequestBuilder(cfg))
	padConversationToAdmissionRemaining(t, runner, conv, 4_500)

	calls := []convcontext.ToolCall{
		{ID: "read_text_0", Type: "function", Function: convcontext.FunctionCall{Name: read.ToolName, Arguments: `{"file_path":"notes.md"}`}},
		{ID: "read_text_1", Type: "function", Function: convcontext.FunctionCall{Name: read.ToolName, Arguments: `{"file_path":"readme.md","offset":1,"limit":20}`}},
	}
	results := runner.executeToolCalls(context.Background(), calls, nil)
	if executes != 2 {
		t.Fatalf("text Reads should not be admission-gated, executes=%d results=%+v", executes, results)
	}
	for i, result := range results {
		if result.ErrorCode == ErrorCodeContextAdmissionExhausted {
			t.Fatalf("text result %d was admission-gated: %+v", i, result)
		}
	}
}

func padConversationToAdmissionRemaining(t *testing.T, runner *Runner, conv *convcontext.Conversation, keepTokens int) {
	t.Helper()
	for attempt := 0; attempt < 40; attempt++ {
		remaining := runner.admissionRemainingTokens(nil)
		if remaining <= keepTokens {
			if remaining <= 0 {
				t.Fatalf("padded past blocking threshold, remaining=%d", remaining)
			}
			return
		}
		need := remaining - keepTokens
		if need > 600 {
			need = 600
		}
		if need < 80 {
			need = 80
		}
		conv.UserInput(strings.Repeat("pad ", need))
	}
	t.Fatalf("could not pad conversation to remaining<=%d, remaining=%d", keepTokens, runner.admissionRemainingTokens(nil))
}

func toolResultsAsMessages(results []ToolExecutionResult) []convcontext.Message {
	out := make([]convcontext.Message, 0, len(results))
	for _, result := range results {
		out = append(out, convcontext.Message{
			Role:       convcontext.RoleTool,
			ToolCallID: result.ToolCallID,
			Name:       result.Name,
			Content:    result.Content,
			IsError:    result.IsError,
		})
	}
	return out
}

func itoa(value int) string {
	raw, _ := json.Marshal(value)
	return string(raw)
}
