package clarify

import (
	"errors"
	"reflect"
	"testing"
)

func TestNewRequestRequiresQuestion(t *testing.T) {
	_, err := NewRequest(RequestSpec{Question: "   "})
	if !errors.Is(err, ErrQuestionRequired) {
		t.Fatalf("expected ErrQuestionRequired, got %v", err)
	}
}

func TestNewRequestTrimsAndDeduplicatesChoices(t *testing.T) {
	req, err := NewRequest(RequestSpec{
		Question: "  Which path?  ",
		Choices:  []string{"  Fast  ", "Safe", "Fast", "", "  Safe  "},
	})
	if err != nil {
		t.Fatalf("expected request, got %v", err)
	}
	if req.Question != "Which path?" {
		t.Fatalf("expected trimmed question, got %q", req.Question)
	}
	want := []string{"Fast", "Safe"}
	if !reflect.DeepEqual(req.Choices, want) {
		t.Fatalf("expected choices %v, got %v", want, req.Choices)
	}
	if req.Status != StatusPending {
		t.Fatalf("expected pending status, got %q", req.Status)
	}
}

func TestNewRequestRejectsMoreThanFourChoices(t *testing.T) {
	_, err := NewRequest(RequestSpec{
		Question: "Pick one?",
		Choices:  []string{"A", "B", "C", "D", "E"},
	})
	if !errors.Is(err, ErrTooManyChoices) {
		t.Fatalf("expected ErrTooManyChoices, got %v", err)
	}
}

func TestBlankChoicesBecomeOpenEnded(t *testing.T) {
	req, err := NewRequest(RequestSpec{
		Question: "What should we do?",
		Choices:  []string{" ", "\t"},
	})
	if err != nil {
		t.Fatalf("expected request, got %v", err)
	}
	if !req.IsOpenEnded() {
		t.Fatalf("expected open-ended request, got choices %v", req.Choices)
	}
}

func TestAnswerOpenRecordsTrimmedFreeformAnswer(t *testing.T) {
	req := mustRequest(t, RequestSpec{Question: "What do you need?"})
	if err := req.AnswerOpen("  more context  "); err != nil {
		t.Fatalf("expected open answer, got %v", err)
	}
	if req.Status != StatusAnswered {
		t.Fatalf("expected answered status, got %q", req.Status)
	}
	if req.Answer == nil || req.Answer.Kind != AnswerKindOpen || req.Answer.Value != "more context" || req.Answer.ChoiceIndex != -1 {
		t.Fatalf("unexpected answer: %+v", req.Answer)
	}

	result := req.Result()
	if result.Question != "What do you need?" || result.UserResponse != "more context" || result.AnswerKind != AnswerKindOpen {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestAnswerOpenRejectsChoiceQuestion(t *testing.T) {
	req := mustRequest(t, RequestSpec{
		Question: "Which path?",
		Choices:  []string{"Fast", "Safe"},
	})
	err := req.AnswerOpen("anything")
	if !errors.Is(err, ErrInvalidChoice) {
		t.Fatalf("expected ErrInvalidChoice, got %v", err)
	}
	if req.Status != StatusPending {
		t.Fatalf("expected request to remain pending, got %q", req.Status)
	}
}

func TestAnswerChoiceRecordsChoiceByLabelAndIndex(t *testing.T) {
	req := mustRequest(t, RequestSpec{
		Question: "Which path?",
		Choices:  []string{"Fast", "Safe"},
	})
	if err := req.AnswerChoice(" Safe "); err != nil {
		t.Fatalf("expected choice answer, got %v", err)
	}
	if req.Answer == nil || req.Answer.Kind != AnswerKindChoice || req.Answer.Value != "Safe" || req.Answer.Choice != "Safe" || req.Answer.ChoiceIndex != 1 {
		t.Fatalf("unexpected answer: %+v", req.Answer)
	}
}

func TestAnswerChoiceIndexRecordsChoice(t *testing.T) {
	req := mustRequest(t, RequestSpec{
		Question: "Which path?",
		Choices:  []string{"Fast", "Safe"},
	})
	if err := req.AnswerChoiceIndex(0); err != nil {
		t.Fatalf("expected indexed choice answer, got %v", err)
	}
	if req.Answer == nil || req.Answer.Value != "Fast" || req.Answer.ChoiceIndex != 0 {
		t.Fatalf("unexpected answer: %+v", req.Answer)
	}
}

func TestAnswerChoiceRejectsMissingChoice(t *testing.T) {
	req := mustRequest(t, RequestSpec{
		Question: "Which path?",
		Choices:  []string{"Fast", "Safe"},
	})
	if err := req.AnswerChoice("Other"); !errors.Is(err, ErrInvalidChoice) {
		t.Fatalf("expected ErrInvalidChoice, got %v", err)
	}
	if err := req.AnswerChoiceIndex(2); !errors.Is(err, ErrInvalidChoice) {
		t.Fatalf("expected ErrInvalidChoice for index, got %v", err)
	}
}

func TestAnswerOtherRequiresOptInForChoiceQuestions(t *testing.T) {
	req := mustRequest(t, RequestSpec{
		Question: "Which path?",
		Choices:  []string{"Fast", "Safe"},
	})
	err := req.AnswerOther("Something else")
	if !errors.Is(err, ErrOtherNotAllowed) {
		t.Fatalf("expected ErrOtherNotAllowed, got %v", err)
	}
	if req.Status != StatusPending {
		t.Fatalf("expected request to remain pending, got %q", req.Status)
	}
}

func TestAnswerOtherRecordsFreeformWhenAllowed(t *testing.T) {
	req := mustRequest(t, RequestSpec{
		Question:   "Which path?",
		Choices:    []string{"Fast", "Safe"},
		AllowOther: true,
	})
	if err := req.AnswerOther("  decide later  "); err != nil {
		t.Fatalf("expected other answer, got %v", err)
	}
	if req.Answer == nil || req.Answer.Kind != AnswerKindOther || req.Answer.Value != "decide later" || req.Answer.ChoiceIndex != -1 {
		t.Fatalf("unexpected answer: %+v", req.Answer)
	}
}

func TestAnswerOtherNormalizesMatchingChoice(t *testing.T) {
	req := mustRequest(t, RequestSpec{
		Question:   "Which path?",
		Choices:    []string{"Fast", "Safe"},
		AllowOther: true,
	})
	if err := req.AnswerOther("  Safe  "); err != nil {
		t.Fatalf("expected normalized choice answer, got %v", err)
	}
	if req.Answer == nil || req.Answer.Kind != AnswerKindChoice || req.Answer.Value != "Safe" || req.Answer.ChoiceIndex != 1 {
		t.Fatalf("unexpected answer: %+v", req.Answer)
	}
}

func TestAnswerOtherOnOpenQuestionRecordsOpenAnswer(t *testing.T) {
	req := mustRequest(t, RequestSpec{Question: "Anything else?"})
	if err := req.AnswerOther("  yes  "); err != nil {
		t.Fatalf("expected open answer through other path, got %v", err)
	}
	if req.Answer == nil || req.Answer.Kind != AnswerKindOpen || req.Answer.Value != "yes" {
		t.Fatalf("unexpected answer: %+v", req.Answer)
	}
}

func TestAnswersRejectEmptyText(t *testing.T) {
	req := mustRequest(t, RequestSpec{Question: "Anything else?"})
	if err := req.AnswerOpen("   "); !errors.Is(err, ErrEmptyAnswer) {
		t.Fatalf("expected ErrEmptyAnswer, got %v", err)
	}
	if req.Status != StatusPending {
		t.Fatalf("expected request to remain pending, got %q", req.Status)
	}
}

func TestCancelAndDuplicateTransitions(t *testing.T) {
	req := mustRequest(t, RequestSpec{Question: "Continue?"})
	if err := req.Cancel(); err != nil {
		t.Fatalf("expected cancel, got %v", err)
	}
	if req.Status != StatusCancelled {
		t.Fatalf("expected cancelled status, got %q", req.Status)
	}
	if err := req.AnswerOpen("yes"); !errors.Is(err, ErrAlreadyResolved) {
		t.Fatalf("expected ErrAlreadyResolved after cancel, got %v", err)
	}
	if err := req.Cancel(); !errors.Is(err, ErrAlreadyResolved) {
		t.Fatalf("expected ErrAlreadyResolved on duplicate cancel, got %v", err)
	}
}

func TestDuplicateAnswerIsRejected(t *testing.T) {
	req := mustRequest(t, RequestSpec{Question: "Continue?"})
	if err := req.AnswerOpen("yes"); err != nil {
		t.Fatalf("expected first answer, got %v", err)
	}
	if err := req.AnswerOpen("no"); !errors.Is(err, ErrAlreadyResolved) {
		t.Fatalf("expected ErrAlreadyResolved on duplicate answer, got %v", err)
	}
}

func TestFailTransitionsAndDefaultMessage(t *testing.T) {
	req := mustRequest(t, RequestSpec{Question: "Continue?"})
	if err := req.Fail("  callback unavailable  "); err != nil {
		t.Fatalf("expected fail, got %v", err)
	}
	if req.Status != StatusFailed || req.Error != "callback unavailable" {
		t.Fatalf("unexpected failed request: %+v", req)
	}
	if err := req.AnswerOpen("yes"); !errors.Is(err, ErrAlreadyResolved) {
		t.Fatalf("expected ErrAlreadyResolved after fail, got %v", err)
	}

	req = mustRequest(t, RequestSpec{Question: "Continue?"})
	if err := req.Fail(" "); err != nil {
		t.Fatalf("expected fail with default message, got %v", err)
	}
	if req.Error != "clarify request failed" {
		t.Fatalf("expected default failure message, got %q", req.Error)
	}
}

func TestFailUnavailableRecordsClearExecutionContextError(t *testing.T) {
	req := mustRequest(t, RequestSpec{Question: "Continue?"})
	if err := req.FailUnavailable(); err != nil {
		t.Fatalf("expected unavailable failure, got %v", err)
	}
	if req.Status != StatusFailed || req.Error != ErrUnavailable.Error() {
		t.Fatalf("unexpected unavailable failure: %+v", req)
	}
	result := req.Result()
	if result.Status != StatusFailed || result.Error != ErrUnavailable.Error() {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func mustRequest(t *testing.T, spec RequestSpec) *Request {
	t.Helper()
	req, err := NewRequest(spec)
	if err != nil {
		t.Fatalf("failed to construct request: %v", err)
	}
	return req
}
