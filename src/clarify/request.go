package clarify

import (
	"errors"
	"fmt"
	"strings"
)

const MaxChoices = 4

var (
	ErrQuestionRequired = errors.New("clarify question is required")
	ErrTooManyChoices   = errors.New("clarify choices exceed maximum")
	ErrEmptyAnswer      = errors.New("clarify answer is required")
	ErrInvalidChoice    = errors.New("clarify choice is invalid")
	ErrOtherNotAllowed  = errors.New("clarify other answer is not allowed")
	ErrAlreadyResolved  = errors.New("clarify request is already resolved")
	ErrUnavailable      = errors.New("clarify is not available in this execution context")
)

type Status string

const (
	StatusPending   Status = "pending"
	StatusAnswered  Status = "answered"
	StatusCancelled Status = "cancelled"
	StatusFailed    Status = "failed"
)

type AnswerKind string

const (
	AnswerKindOpen   AnswerKind = "open"
	AnswerKindChoice AnswerKind = "choice"
	AnswerKindOther  AnswerKind = "other"
)

type RequestSpec struct {
	ID         string   `json:"id,omitempty"`
	Question   string   `json:"question"`
	Choices    []string `json:"choices,omitempty"`
	AllowOther bool     `json:"allow_other,omitempty"`
}

type Request struct {
	ID         string   `json:"id,omitempty"`
	Question   string   `json:"question"`
	Choices    []string `json:"choices,omitempty"`
	AllowOther bool     `json:"allow_other,omitempty"`
	Status     Status   `json:"status"`
	Answer     *Answer  `json:"answer,omitempty"`
	Error      string   `json:"error,omitempty"`
}

type Answer struct {
	Kind        AnswerKind `json:"kind"`
	Value       string     `json:"value"`
	Choice      string     `json:"choice,omitempty"`
	ChoiceIndex int        `json:"choice_index"`
}

type Result struct {
	Question     string     `json:"question"`
	Choices      []string   `json:"choices,omitempty"`
	UserResponse string     `json:"user_response,omitempty"`
	Status       Status     `json:"status"`
	AnswerKind   AnswerKind `json:"answer_kind,omitempty"`
	Error        string     `json:"error,omitempty"`
}

func NewRequest(spec RequestSpec) (*Request, error) {
	question := strings.TrimSpace(spec.Question)
	if question == "" {
		return nil, ErrQuestionRequired
	}

	choices, err := NormalizeChoices(spec.Choices)
	if err != nil {
		return nil, err
	}

	return &Request{
		ID:         strings.TrimSpace(spec.ID),
		Question:   question,
		Choices:    choices,
		AllowOther: spec.AllowOther,
		Status:     StatusPending,
	}, nil
}

func NormalizeChoices(choices []string) ([]string, error) {
	normalized := make([]string, 0, len(choices))
	seen := map[string]struct{}{}
	for _, choice := range choices {
		trimmed := strings.TrimSpace(choice)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		normalized = append(normalized, trimmed)
	}
	if len(normalized) > MaxChoices {
		return nil, fmt.Errorf("%w: got %d, max %d", ErrTooManyChoices, len(normalized), MaxChoices)
	}
	return normalized, nil
}

func (r *Request) IsOpenEnded() bool {
	return len(r.Choices) == 0
}

func (r *Request) AnswerOpen(value string) error {
	if err := r.ensurePending(); err != nil {
		return err
	}
	if !r.IsOpenEnded() {
		return ErrInvalidChoice
	}
	return r.recordAnswer(AnswerKindOpen, value, "", -1)
}

func (r *Request) AnswerChoice(choice string) error {
	if err := r.ensurePending(); err != nil {
		return err
	}
	index, normalized, ok := r.findChoice(choice)
	if !ok {
		return ErrInvalidChoice
	}
	return r.recordAnswer(AnswerKindChoice, normalized, normalized, index)
}

func (r *Request) AnswerChoiceIndex(index int) error {
	if err := r.ensurePending(); err != nil {
		return err
	}
	if index < 0 || index >= len(r.Choices) {
		return ErrInvalidChoice
	}
	choice := r.Choices[index]
	return r.recordAnswer(AnswerKindChoice, choice, choice, index)
}

func (r *Request) AnswerOther(value string) error {
	if err := r.ensurePending(); err != nil {
		return err
	}
	if r.IsOpenEnded() {
		return r.recordAnswer(AnswerKindOpen, value, "", -1)
	}
	if index, choice, ok := r.findChoice(value); ok {
		return r.recordAnswer(AnswerKindChoice, choice, choice, index)
	}
	if !r.AllowOther {
		return ErrOtherNotAllowed
	}
	return r.recordAnswer(AnswerKindOther, value, "", -1)
}

func (r *Request) Cancel() error {
	if err := r.ensurePending(); err != nil {
		return err
	}
	r.Status = StatusCancelled
	return nil
}

func (r *Request) Fail(message string) error {
	if err := r.ensurePending(); err != nil {
		return err
	}
	message = strings.TrimSpace(message)
	if message == "" {
		message = "clarify request failed"
	}
	r.Status = StatusFailed
	r.Error = message
	return nil
}

func (r *Request) FailUnavailable() error {
	return r.Fail(ErrUnavailable.Error())
}

func (r Request) Result() Result {
	result := Result{
		Question: r.Question,
		Choices:  append([]string(nil), r.Choices...),
		Status:   r.Status,
		Error:    r.Error,
	}
	if r.Answer != nil {
		result.UserResponse = r.Answer.Value
		result.AnswerKind = r.Answer.Kind
	}
	return result
}

func (r *Request) ensurePending() error {
	if r == nil || r.Status != StatusPending {
		return ErrAlreadyResolved
	}
	return nil
}

func (r *Request) recordAnswer(kind AnswerKind, value, choice string, index int) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return ErrEmptyAnswer
	}
	r.Status = StatusAnswered
	r.Answer = &Answer{
		Kind:        kind,
		Value:       value,
		Choice:      choice,
		ChoiceIndex: index,
	}
	return nil
}

func (r *Request) findChoice(choice string) (int, string, bool) {
	normalized := strings.TrimSpace(choice)
	if normalized == "" {
		return -1, "", false
	}
	for i, candidate := range r.Choices {
		if candidate == normalized {
			return i, candidate, true
		}
	}
	return -1, "", false
}
