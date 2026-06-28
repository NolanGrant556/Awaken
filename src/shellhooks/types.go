package shellhooks

import "time"

// Event names are Awaken's stable, snake_case hook lifecycle points.
type Event string

const (
	EventPreToolCall  Event = "pre_tool_call"
	EventPostToolCall Event = "post_tool_call"
	EventPreLLMCall   Event = "pre_llm_call"
	EventPostLLMCall  Event = "post_llm_call"
	EventSessionStart Event = "session_start"
	EventSessionEnd   Event = "session_end"
	EventPreCompact   Event = "pre_compact"
	EventPostCompact  Event = "post_compact"
	EventNotification Event = "notification"
	EventFileChanged  Event = "file_changed"
	EventCWDChanged   Event = "cwd_changed"
)

var validEvents = map[Event]struct{}{
	EventPreToolCall:  {},
	EventPostToolCall: {},
	EventPreLLMCall:   {},
	EventPostLLMCall:  {},
	EventSessionStart: {},
	EventSessionEnd:   {},
	EventPreCompact:   {},
	EventPostCompact:  {},
	EventNotification: {},
	EventFileChanged:  {},
	EventCWDChanged:   {},
}

func IsValidEvent(event Event) bool {
	_, ok := validEvents[event]
	return ok
}

type Spec struct {
	Event   Event         `json:"event"`
	Matcher string        `json:"matcher,omitempty"`
	Command string        `json:"command,omitempty"`
	Argv    []string      `json:"argv,omitempty"`
	Timeout time.Duration `json:"timeout,omitempty"`
	CWD     string        `json:"cwd,omitempty"`
	Env     []string      `json:"env,omitempty"`
}

type Input struct {
	HookEventName    Event          `json:"hook_event_name"`
	SessionID        string         `json:"session_id,omitempty"`
	CWD              string         `json:"cwd,omitempty"`
	ToolName         string         `json:"tool_name,omitempty"`
	ToolInput        any            `json:"tool_input,omitempty"`
	ToolResult       any            `json:"tool_result,omitempty"`
	Model            string         `json:"model,omitempty"`
	LLMInput         any            `json:"llm_input,omitempty"`
	LLMOutput        any            `json:"llm_output,omitempty"`
	NotificationType string         `json:"notification_type,omitempty"`
	FilePath         string         `json:"file_path,omitempty"`
	FileEvent        string         `json:"file_event,omitempty"`
	OldCWD           string         `json:"old_cwd,omitempty"`
	NewCWD           string         `json:"new_cwd,omitempty"`
	CompactTrigger   string         `json:"compact_trigger,omitempty"`
	SessionReason    string         `json:"session_reason,omitempty"`
	Extra            map[string]any `json:"extra,omitempty"`

	// MatchQuery overrides event-specific matching when callers already know
	// the exact value a hook matcher should evaluate.
	MatchQuery string `json:"-"`
}

type DecisionType string

const (
	DecisionNone            DecisionType = ""
	DecisionAllow           DecisionType = "allow"
	DecisionBlock           DecisionType = "block"
	DecisionRewrite         DecisionType = "rewrite"
	DecisionContext         DecisionType = "context"
	DecisionTransformResult DecisionType = "transform_result"
)

type Decision struct {
	Type            DecisionType   `json:"type,omitempty"`
	Reason          string         `json:"reason,omitempty"`
	Message         string         `json:"message,omitempty"`
	Context         string         `json:"context,omitempty"`
	Rewrite         any            `json:"rewrite,omitempty"`
	TransformResult any            `json:"transform_result,omitempty"`
	Raw             map[string]any `json:"raw,omitempty"`
}

func (d Decision) IsZero() bool {
	return d.Type == DecisionNone &&
		d.Reason == "" &&
		d.Message == "" &&
		d.Context == "" &&
		d.Rewrite == nil &&
		d.TransformResult == nil
}

type ParsedOutput struct {
	JSON      map[string]any `json:"json,omitempty"`
	PlainText string         `json:"plain_text,omitempty"`
	Decision  Decision       `json:"decision,omitempty"`
	JSONError string         `json:"json_error,omitempty"`
}

type Status string

const (
	StatusSuccess Status = "success"
	StatusBlocked Status = "blocked"
	StatusSkipped Status = "skipped"
	StatusTimeout Status = "timeout"
	StatusError   Status = "error"
)

type Result struct {
	Status        Status        `json:"status"`
	ApprovalState ApprovalState `json:"approval_state,omitempty"`
	ExitCode      int           `json:"exit_code"`
	TimedOut      bool          `json:"timed_out"`
	Duration      time.Duration `json:"duration"`
	Stdout        string        `json:"stdout,omitempty"`
	Stderr        string        `json:"stderr,omitempty"`
	InertOutput   string        `json:"inert_output,omitempty"`
	Decision      Decision      `json:"decision,omitempty"`
	Parsed        ParsedOutput  `json:"parsed,omitempty"`
	Error         string        `json:"error,omitempty"`
}

type RunRequest struct {
	Argv  []string
	Stdin []byte
	CWD   string
	Env   []string
}

type RunOutput struct {
	ExitCode int
	Stdout   string
	Stderr   string
	Duration time.Duration
	TimedOut bool
}
