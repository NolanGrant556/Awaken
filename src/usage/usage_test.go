package usage

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"awaken/src/context"
	"awaken/src/tool"
)

func TestLedgerNormalizesRecordsAndTotals(t *testing.T) {
	ledger := NewLedger()
	cost := 0.012

	record := ledger.RecordUsage(Record{
		Provider:         "openai",
		Protocol:         "openai_chat",
		Model:            "test-model",
		PromptTokens:     10,
		CompletionTokens: 5,
		EstimatedCostUSD: &cost,
	})
	ledger.RecordUsage(Record{
		PromptTokens: 3,
		TotalTokens:  3,
		RequestCount: 2,
		Estimated:    true,
		InputBytes:   12,
	})

	if record.RequestCount != 1 {
		t.Fatalf("expected default request count 1, got %d", record.RequestCount)
	}
	if record.TotalTokens != 15 {
		t.Fatalf("expected total tokens from prompt+completion, got %d", record.TotalTokens)
	}
	if record.RecordedAt.IsZero() {
		t.Fatal("expected recorded_at to be set")
	}

	records := ledger.Records()
	records[0].Provider = "mutated"
	if got := ledger.Records()[0].Provider; got != "openai" {
		t.Fatalf("expected Records to return a copy, got provider %q", got)
	}

	totals := ledger.Totals()
	if totals.PromptTokens != 13 || totals.CompletionTokens != 5 || totals.TotalTokens != 18 || totals.RequestCount != 3 {
		t.Fatalf("unexpected totals: %+v", totals)
	}
	if totals.EstimatedRecords != 1 || totals.InputBytes != 12 {
		t.Fatalf("unexpected estimated totals: %+v", totals)
	}
	if totals.EstimatedCostUSD == nil || *totals.EstimatedCostUSD != cost {
		t.Fatalf("expected cost total %v, got %+v", cost, totals.EstimatedCostUSD)
	}
}

func TestEstimateInputAndEstimatedRecordDoNotPersistPromptContent(t *testing.T) {
	secret := "secret prompt text"
	estimate := EstimateInput("system "+secret, "", []context.Message{
		{Role: context.RoleUser, Content: "hello"},
		{
			Role: context.RoleAssistant,
			ToolCalls: []context.ToolCall{
				{
					ID:   "call_1",
					Type: "function",
					Function: context.FunctionCall{
						Name:      "lookup",
						Arguments: `{"query":"` + secret + `"}`,
					},
				},
			},
		},
	}, []tool.ToolDefinition{
		{
			Type: "function",
			Function: tool.ToolFunctionSchema{
				Name:        "lookup",
				Description: "lookup values",
				Parameters: tool.ToolParametersSchema{
					Type: "object",
					Properties: map[string]tool.ToolParam{
						"query": {Type: "string"},
					},
					Required: []string{"query"},
				},
			},
		},
	})

	if estimate.InputBytes <= len(secret) || estimate.EstimatedTokens != EstimateTokensFromBytes(estimate.InputBytes) {
		t.Fatalf("unexpected estimate: %+v", estimate)
	}
	if estimate.MessageCount != 2 || estimate.ToolCount != 1 {
		t.Fatalf("unexpected estimate counts: %+v", estimate)
	}

	record := NewEstimatedRecord(Metadata{Provider: "local", Protocol: "test", Model: "m"}, estimate)
	encoded, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("failed to marshal record: %v", err)
	}
	if strings.Contains(string(encoded), secret) {
		t.Fatalf("usage record must not contain prompt content: %s", encoded)
	}
	if !record.Estimated || record.PromptTokens != estimate.EstimatedTokens || record.CompletionTokens != 0 {
		t.Fatalf("unexpected estimated record: %+v", record)
	}
}

func TestBudgetGuardZeroValuesDoNotBlock(t *testing.T) {
	err := NewBudgetGuard(BudgetLimits{}).Check(Estimate{InputBytes: 1000, EstimatedTokens: 250})
	if err != nil {
		t.Fatalf("expected zero limits not to block, got %v", err)
	}
}

func TestBudgetGuardBlocksConfiguredLimits(t *testing.T) {
	err := NewBudgetGuard(BudgetLimits{MaxInputBytes: 10}).Check(Estimate{InputBytes: 11, EstimatedTokens: 3})
	if !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("expected ErrBudgetExceeded, got %v", err)
	}
	var budgetErr *BudgetExceededError
	if !errors.As(err, &budgetErr) {
		t.Fatalf("expected BudgetExceededError, got %T", err)
	}
	if budgetErr.Limit != "max_input_bytes" || budgetErr.Actual != 11 || budgetErr.LimitMax != 10 {
		t.Fatalf("unexpected budget error: %+v", budgetErr)
	}

	err = NewBudgetGuard(BudgetLimits{MaxEstimatedTokens: 2}).Check(Estimate{InputBytes: 8, EstimatedTokens: 3})
	if !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("expected token budget error, got %v", err)
	}
	if !strings.Contains(err.Error(), "max_estimated_tokens") {
		t.Fatalf("expected token limit name in error, got %q", err.Error())
	}
}

func TestProviderFromBaseURL(t *testing.T) {
	cases := map[string]string{
		"https://api.openai.com/v1":    "openai",
		"https://api.anthropic.com/v1": "anthropic",
		"http://127.0.0.1:8787/custom": "127.0.0.1",
	}
	for rawURL, want := range cases {
		if got := ProviderFromBaseURL(rawURL); got != want {
			t.Fatalf("ProviderFromBaseURL(%q) = %q, want %q", rawURL, got, want)
		}
	}
}
