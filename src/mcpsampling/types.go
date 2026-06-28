package mcpsampling

import (
	"context"
	"time"
)

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

type ContentType string

const (
	ContentText       ContentType = "text"
	ContentImage      ContentType = "image"
	ContentToolUse    ContentType = "tool_use"
	ContentToolResult ContentType = "tool_result"
)

type StopReason string

const (
	StopReasonEndTurn   StopReason = "endTurn"
	StopReasonMaxTokens StopReason = "maxTokens"
	StopReasonToolUse   StopReason = "toolUse"
)

type Config struct {
	ServerName       string
	Model            string
	DefaultModel     string
	MaxRPM           int
	Timeout          time.Duration
	MaxTokensCap     int
	MaxToolRounds    int
	DisableToolLoops bool
	AllowedModels    []string
	Now              func() time.Time
}

type Request struct {
	SystemPrompt     string
	Messages         []SamplingMessage
	Tools            []Tool
	ModelPreferences ModelPreferences
	MaxTokens        int
	Temperature      *float64
}

type ModelPreferences struct {
	Hints []ModelHint
}

type ModelHint struct {
	Name string
}

type SamplingMessage struct {
	Role    Role
	Content []ContentBlock
}

type ContentBlock struct {
	Type      ContentType
	Text      string
	Data      string
	MIMEType  string
	ID        string
	Name      string
	Input     any
	ToolUseID string
	Content   []ContentBlock
	IsError   bool
}

type Tool struct {
	Name        string
	Description string
	InputSchema map[string]any
}

type LLMClient interface {
	CreateMessage(ctx context.Context, req LLMRequest) (LLMResponse, error)
}

type LLMRequest struct {
	Model       string
	Messages    []LLMMessage
	Tools       []LLMTool
	MaxTokens   int
	Temperature *float64
}

type LLMMessage struct {
	Role       Role
	Content    any
	ToolCalls  []LLMToolCall
	ToolCallID string
	Name       string
}

type LLMContentPart struct {
	Type     string       `json:"type"`
	Text     string       `json:"text,omitempty"`
	ImageURL *LLMImageURL `json:"image_url,omitempty"`
}

type LLMImageURL struct {
	URL string `json:"url"`
}

type LLMTool struct {
	Type     string          `json:"type"`
	Function LLMFunctionTool `json:"function"`
}

type LLMFunctionTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type LLMToolCall struct {
	ID       string          `json:"id"`
	Type     string          `json:"type"`
	Function LLMFunctionCall `json:"function"`
}

type LLMFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type LLMResponse struct {
	Model   string
	Choices []LLMChoice
	Usage   LLMUsage
}

type LLMChoice struct {
	Message      LLMMessage
	FinishReason string
}

type LLMUsage struct {
	TotalTokens int
}

type Result struct {
	Role       Role
	Content    []ContentBlock
	Model      string
	StopReason StopReason
}

type ErrorData struct {
	Code    int
	Message string
}

type Metrics struct {
	Requests     int64
	Errors       int64
	TokensUsed   int64
	ToolUseCount int64
	RateLimited  int64
}

const (
	ErrorCodeRateLimited      = -32001
	ErrorCodeModelNotAllowed  = -32002
	ErrorCodeInvalidRequest   = -32003
	ErrorCodeLLMFailure       = -32004
	ErrorCodeTimeout          = -32005
	ErrorCodeEmptyResponse    = -32006
	ErrorCodeToolLoopExceeded = -32007
)
