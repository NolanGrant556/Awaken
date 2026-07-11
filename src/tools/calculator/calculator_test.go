package calculator

import (
	"context"
	"testing"

	"awaken/src/tool"
)

func TestExecuteOutcomeReturnsStructuredCalculation(t *testing.T) {
	outcome, err := (&CalculatorTool{}).ExecuteOutcome(context.Background(), "1+2*3", nil)
	if err != nil {
		t.Fatalf("ExecuteOutcome transport error: %v", err)
	}
	if outcome.Status != tool.OutcomeSucceeded || outcome.Content != "7" {
		t.Fatalf("unexpected calculation outcome: %+v", outcome)
	}
	if outcome.Diagnostic["expression"] != "1+2*3" || outcome.Diagnostic["result"] != "7" {
		t.Fatalf("missing calculation diagnostics: %+v", outcome.Diagnostic)
	}
}

func TestEvaluateExpressionUsesConventionalPrecedence(t *testing.T) {
	tests := map[string]float64{
		"sqrt(81)+2^3": 17,
		"sqrt(81+19)":  10,
		"2^3^2":        512,
		"(1+2)*3":      9,
		"-2^2":         -4,
	}
	for expression, want := range tests {
		got, err := evaluateExpression(expression)
		if err != nil {
			t.Fatalf("evaluateExpression(%q): %v", expression, err)
		}
		if got != want {
			t.Fatalf("evaluateExpression(%q)=%v, want %v", expression, got, want)
		}
	}
}

func TestExecuteOutcomeReturnsTypedDomainFailure(t *testing.T) {
	outcome, err := (&CalculatorTool{}).ExecuteOutcome(context.Background(), "1/0", nil)
	if err != nil {
		t.Fatalf("domain failure must be an outcome, got transport error: %v", err)
	}
	if outcome.ErrorCode != "calculator_invalid_expression" || outcome.ErrorCategory != tool.ErrorCategoryDomain || outcome.Retryable {
		t.Fatalf("unexpected failure outcome: %+v", outcome)
	}
	if len(outcome.RecoveryOptions) == 0 {
		t.Fatalf("expected actionable recovery: %+v", outcome)
	}
}

func TestLegacyExecutePreservesErrorContract(t *testing.T) {
	if _, err := (&CalculatorTool{}).Execute(context.Background(), "1/0"); err == nil {
		t.Fatal("legacy Execute must still return an error for invalid expressions")
	}
}
