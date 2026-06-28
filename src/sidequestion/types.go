package sidequestion

import "time"

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

type Message struct {
	Role            Role     `json:"role"`
	Content         string   `json:"content"`
	StopReason      *string  `json:"stop_reason,omitempty"`
	CompactBoundary bool     `json:"compact_boundary,omitempty"`
	ToolCallID      string   `json:"tool_call_id,omitempty"`
	ToolCalls       []string `json:"tool_calls,omitempty"`
}

type CacheSafeParams struct {
	SystemPrompt        string    `json:"system_prompt"`
	UserContext         string    `json:"user_context"`
	SystemContext       string    `json:"system_context"`
	ToolUseContext      string    `json:"tool_use_context,omitempty"`
	ForkContextMessages []Message `json:"fork_context_messages,omitempty"`
}

type SideQuestionContext struct {
	Messages                []Message
	LastCacheSafeParams     *CacheSafeParams
	RebuiltCacheSafeParams  CacheSafeParams
	CurrentToolUseContext   string
	LastCacheSafeCapturedAt time.Time
}

type SideQuestionRequest struct {
	Question string
	Context  SideQuestionContext
}

type CacheSource string

const (
	CacheSourceLastSafe CacheSource = "last_cache_safe"
	CacheSourceRebuilt  CacheSource = "rebuilt"
)

type ForkPlan struct {
	Question                    string
	CacheSafeParams             CacheSafeParams
	CacheSource                 CacheSource
	CacheHitExpected            bool
	StrippedInProgressAssistant bool
	CompactBoundaryIndex        int
	OriginalMessageCount        int
	ForkMessageCount            int
	UsageIncrement              UsageCounters
}

type TaskType string

const (
	TaskTypeText   TaskType = "text"
	TaskTypeVision TaskType = "vision"
)

type ProviderCandidate struct {
	Name           string
	Provider       string
	Model          string
	Available      bool
	SupportsText   bool
	SupportsVision bool
}

type ProviderRoute struct {
	TaskType      TaskType
	Preferred     string
	Auto          bool
	Candidates    []ProviderCandidate
	CurrentIndex  int
	FailureRecord []ProviderFailure
}

type ProviderFailure struct {
	Provider string
	Model    string
	Class    ProviderErrorClass
	Message  string
}

type ProviderErrorClass string

const (
	ProviderErrorNone            ProviderErrorClass = ""
	ProviderErrorCreditExhausted ProviderErrorClass = "credit_exhausted"
	ProviderErrorRateLimited     ProviderErrorClass = "rate_limited"
	ProviderErrorConnection      ProviderErrorClass = "connection"
	ProviderErrorAuth            ProviderErrorClass = "auth"
	ProviderErrorFatal           ProviderErrorClass = "fatal"
)

type ProviderError struct {
	StatusCode int
	Class      ProviderErrorClass
	Message    string
}

func (e ProviderError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	if e.Class != "" {
		return string(e.Class)
	}
	return "provider error"
}

type UsageCounters struct {
	SideQuestionCountIncrement int `json:"side_question_count_increment"`
	PromptTokens               int `json:"prompt_tokens,omitempty"`
	CompletionTokens           int `json:"completion_tokens,omitempty"`
	TotalTokens                int `json:"total_tokens,omitempty"`
}

type RunStatus string

const (
	RunStatusCompleted RunStatus = "completed"
	RunStatusFailed    RunStatus = "failed"
	RunStatusAborted   RunStatus = "aborted"
)

type RunOutcome struct {
	Response string
	Err      error
	Aborted  bool
	Usage    UsageCounters
	Provider ProviderCandidate
}

type RunResult struct {
	Status   RunStatus
	Response string
	Error    string
	Usage    UsageCounters
	Provider ProviderCandidate
}
