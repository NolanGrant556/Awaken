package usage

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"awaken/src/context"
	"awaken/src/tool"
)

var ErrBudgetExceeded = errors.New("usage budget exceeded")

type Metadata struct {
	Provider string `json:"provider,omitempty"`
	Protocol string `json:"protocol,omitempty"`
	Model    string `json:"model,omitempty"`
}

type Record struct {
	Provider         string    `json:"provider,omitempty"`
	Protocol         string    `json:"protocol,omitempty"`
	Model            string    `json:"model,omitempty"`
	PromptTokens     int       `json:"prompt_tokens,omitempty"`
	CompletionTokens int       `json:"completion_tokens,omitempty"`
	TotalTokens      int       `json:"total_tokens,omitempty"`
	RequestCount     int       `json:"request_count,omitempty"`
	InputBytes       int       `json:"input_bytes,omitempty"`
	Estimated        bool      `json:"estimated,omitempty"`
	EstimatedCostUSD *float64  `json:"estimated_cost_usd,omitempty"`
	RecordedAt       time.Time `json:"recorded_at,omitempty"`
}

type Totals struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	RequestCount     int
	InputBytes       int
	EstimatedRecords int
	EstimatedCostUSD *float64
}

type Recorder interface {
	RecordUsage(record Record) Record
}

type Ledger struct {
	mu      sync.Mutex
	records []Record
}

func NewLedger() *Ledger {
	return &Ledger{}
}

func (l *Ledger) RecordUsage(record Record) Record {
	normalized := NormalizeRecord(record)

	l.mu.Lock()
	defer l.mu.Unlock()
	l.records = append(l.records, normalized)
	return normalized
}

func (l *Ledger) Records() []Record {
	l.mu.Lock()
	defer l.mu.Unlock()

	result := make([]Record, len(l.records))
	copy(result, l.records)
	return result
}

func (l *Ledger) Totals() Totals {
	l.mu.Lock()
	defer l.mu.Unlock()

	var totals Totals
	var cost float64
	var hasCost bool
	for _, record := range l.records {
		totals.PromptTokens += record.PromptTokens
		totals.CompletionTokens += record.CompletionTokens
		totals.TotalTokens += record.TotalTokens
		totals.RequestCount += record.RequestCount
		totals.InputBytes += record.InputBytes
		if record.Estimated {
			totals.EstimatedRecords++
		}
		if record.EstimatedCostUSD != nil {
			cost += *record.EstimatedCostUSD
			hasCost = true
		}
	}
	if hasCost {
		totals.EstimatedCostUSD = &cost
	}
	return totals
}

func NormalizeRecord(record Record) Record {
	if record.RequestCount <= 0 {
		record.RequestCount = 1
	}
	if record.TotalTokens == 0 {
		record.TotalTokens = record.PromptTokens + record.CompletionTokens
	}
	if record.RecordedAt.IsZero() {
		record.RecordedAt = time.Now().UTC()
	}
	return record
}

type Estimate struct {
	InputBytes      int `json:"input_bytes"`
	EstimatedTokens int `json:"estimated_tokens"`
	MessageCount    int `json:"message_count"`
	ToolCount       int `json:"tool_count"`
}

func EstimateInput(systemPrompt, userPrompt string, messages []context.Message, tools []tool.ToolDefinition) Estimate {
	inputBytes := len(systemPrompt)
	messageCount := len(messages)
	if len(messages) == 0 && userPrompt != "" {
		inputBytes += len(userPrompt)
		messageCount = 1
	}

	for _, msg := range messages {
		inputBytes += len(msg.Content)
		inputBytes += len(msg.ToolCallID)
		inputBytes += len(msg.Name)
		for _, call := range msg.ToolCalls {
			inputBytes += len(call.ID)
			inputBytes += len(call.Type)
			inputBytes += len(call.Function.Name)
			inputBytes += len(call.Function.Arguments)
		}
	}

	for _, definition := range tools {
		inputBytes += len(definition.Type)
		inputBytes += len(definition.Function.Name)
		inputBytes += len(definition.Function.Description)
		if encoded, err := json.Marshal(definition.Function.Parameters); err == nil {
			inputBytes += len(encoded)
		}
	}

	return Estimate{
		InputBytes:      inputBytes,
		EstimatedTokens: EstimateTokensFromBytes(inputBytes),
		MessageCount:    messageCount,
		ToolCount:       len(tools),
	}
}

func EstimateTokensFromBytes(inputBytes int) int {
	if inputBytes <= 0 {
		return 0
	}
	return (inputBytes + 3) / 4
}

func NewEstimatedRecord(metadata Metadata, estimate Estimate) Record {
	return NormalizeRecord(Record{
		Provider:     metadata.Provider,
		Protocol:     metadata.Protocol,
		Model:        metadata.Model,
		PromptTokens: estimate.EstimatedTokens,
		TotalTokens:  estimate.EstimatedTokens,
		RequestCount: 1,
		InputBytes:   estimate.InputBytes,
		Estimated:    true,
	})
}

type BudgetLimits struct {
	MaxInputBytes      int
	MaxEstimatedTokens int
}

func (limits BudgetLimits) Enabled() bool {
	return limits.MaxInputBytes > 0 || limits.MaxEstimatedTokens > 0
}

type BudgetGuard struct {
	limits BudgetLimits
}

func NewBudgetGuard(limits BudgetLimits) BudgetGuard {
	return BudgetGuard{limits: limits}
}

func (g BudgetGuard) Check(estimate Estimate) error {
	if g.limits.MaxInputBytes > 0 && estimate.InputBytes > g.limits.MaxInputBytes {
		return &BudgetExceededError{
			Limit:    "max_input_bytes",
			LimitMax: g.limits.MaxInputBytes,
			Actual:   estimate.InputBytes,
			Estimate: estimate,
		}
	}
	if g.limits.MaxEstimatedTokens > 0 && estimate.EstimatedTokens > g.limits.MaxEstimatedTokens {
		return &BudgetExceededError{
			Limit:    "max_estimated_tokens",
			LimitMax: g.limits.MaxEstimatedTokens,
			Actual:   estimate.EstimatedTokens,
			Estimate: estimate,
		}
	}
	return nil
}

type BudgetExceededError struct {
	Limit    string
	LimitMax int
	Actual   int
	Estimate Estimate
}

func (e *BudgetExceededError) Error() string {
	if e == nil {
		return ErrBudgetExceeded.Error()
	}
	return fmt.Sprintf("%s: %s actual=%d max=%d", ErrBudgetExceeded, e.Limit, e.Actual, e.LimitMax)
}

func (e *BudgetExceededError) Is(target error) bool {
	return target == ErrBudgetExceeded
}

func ProviderFromBaseURL(baseURL string) string {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}
	host := strings.ToLower(parsed.Hostname())
	switch host {
	case "api.openai.com":
		return "openai"
	case "api.anthropic.com":
		return "anthropic"
	default:
		return host
	}
}
