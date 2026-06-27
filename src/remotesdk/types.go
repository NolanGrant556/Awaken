package remotesdk

import (
	"encoding/json"
	"strings"
	"time"
)

type SDKMessageType string

const (
	SDKMessageAssistant      SDKMessageType = "assistant"
	SDKMessageUser           SDKMessageType = "user"
	SDKMessageStreamEvent    SDKMessageType = "stream_event"
	SDKMessageResult         SDKMessageType = "result"
	SDKMessageSystem         SDKMessageType = "system"
	SDKMessageToolProgress   SDKMessageType = "tool_progress"
	SDKMessageAuthStatus     SDKMessageType = "auth_status"
	SDKMessageToolUseSummary SDKMessageType = "tool_use_summary"
	SDKMessageRateLimitEvent SDKMessageType = "rate_limit_event"
)

type SDKSystemSubtype string

const (
	SDKSystemInit            SDKSystemSubtype = "init"
	SDKSystemStatus          SDKSystemSubtype = "status"
	SDKSystemCompactBoundary SDKSystemSubtype = "compact_boundary"
)

type Severity string

const (
	SeverityInfo    Severity = "info"
	SeverityWarning Severity = "warning"
	SeverityError   Severity = "error"
)

type AwakenRemoteEventType string

const (
	EventAssistantMessage  AwakenRemoteEventType = "assistant_message"
	EventUserMessage       AwakenRemoteEventType = "user_message"
	EventStreamEvent       AwakenRemoteEventType = "stream_event"
	EventResult            AwakenRemoteEventType = "result"
	EventSystemInit        AwakenRemoteEventType = "system_init"
	EventStatus            AwakenRemoteEventType = "status"
	EventToolProgress      AwakenRemoteEventType = "tool_progress"
	EventCompactBoundary   AwakenRemoteEventType = "compact_boundary"
	EventPermissionRequest AwakenRemoteEventType = "permission_request"
	EventPermissionResult  AwakenRemoteEventType = "permission_result"
)

type ConversionKind string

const (
	ConversionEvent       ConversionKind = "event"
	ConversionIgnored     ConversionKind = "ignored"
	ConversionUnsupported ConversionKind = "unsupported"
)

type ConvertOptions struct {
	ConvertToolResults      bool
	ConvertUserTextMessages bool
	FallbackTimestamp       time.Time
}

type ConversionResult struct {
	Kind        ConversionKind
	Event       *AwakenRemoteEvent
	Reason      string
	MessageType SDKMessageType
	Subtype     string
	UUID        string
	SessionID   string
}

type AwakenRemoteEvent struct {
	Type               AwakenRemoteEventType
	Severity           Severity
	Timestamp          time.Time
	UUID               string
	SessionID          string
	ParentToolUseID    string
	Role               string
	Content            string
	Delta              string
	Error              string
	Status             string
	ResultSubtype      string
	Model              string
	CWD                string
	Tools              []string
	ToolUseID          string
	ToolName           string
	ElapsedSeconds     float64
	TaskID             string
	StreamEventType    string
	ToolUses           []ToolUseBlock
	ToolResults        []ToolResultBlock
	CompactMetadata    *CompactMetadata
	PermissionRequest  *PermissionRequestRecord
	PermissionResponse *PermissionResponseRecord
	RawMessage         json.RawMessage
	RawStreamEvent     json.RawMessage
}

type SDKMessage struct {
	Type              SDKMessageType         `json:"type"`
	Subtype           string                 `json:"subtype,omitempty"`
	UUID              string                 `json:"uuid,omitempty"`
	SessionID         string                 `json:"session_id,omitempty"`
	Timestamp         string                 `json:"timestamp,omitempty"`
	ParentToolUseID   *string                `json:"parent_tool_use_id,omitempty"`
	Message           SDKConversationMessage `json:"message,omitempty"`
	Event             SDKStreamEvent         `json:"event,omitempty"`
	Error             string                 `json:"error,omitempty"`
	Result            string                 `json:"result,omitempty"`
	Errors            []string               `json:"errors,omitempty"`
	IsError           bool                   `json:"is_error,omitempty"`
	Status            *string                `json:"status,omitempty"`
	Model             string                 `json:"model,omitempty"`
	CWD               string                 `json:"cwd,omitempty"`
	Tools             []string               `json:"tools,omitempty"`
	PermissionMode    string                 `json:"permissionMode,omitempty"`
	CompactMetadata   CompactMetadata        `json:"compact_metadata,omitempty"`
	ToolUseID         string                 `json:"tool_use_id,omitempty"`
	ToolName          string                 `json:"tool_name,omitempty"`
	ElapsedSeconds    float64                `json:"elapsed_time_seconds,omitempty"`
	TaskID            string                 `json:"task_id,omitempty"`
	ToolUseResult     any                    `json:"tool_use_result,omitempty"`
	PermissionDenials []PermissionDenial     `json:"permission_denials,omitempty"`
	StructuredOutput  any                    `json:"structured_output,omitempty"`
	Raw               json.RawMessage        `json:"-"`
}

type SDKConversationMessage struct {
	ID           string     `json:"id,omitempty"`
	Type         string     `json:"type,omitempty"`
	Role         string     `json:"role,omitempty"`
	Content      SDKContent `json:"content,omitempty"`
	Model        string     `json:"model,omitempty"`
	StopReason   string     `json:"stop_reason,omitempty"`
	StopSequence string     `json:"stop_sequence,omitempty"`
}

type SDKContent struct {
	Text   *string
	Blocks []ContentBlock
	Raw    json.RawMessage
}

func TextContent(text string) SDKContent {
	return SDKContent{Text: &text}
}

func BlockContent(blocks ...ContentBlock) SDKContent {
	return SDKContent{Blocks: blocks}
}

func (c *SDKContent) UnmarshalJSON(data []byte) error {
	c.Raw = append(c.Raw[:0], data...)
	c.Text = nil
	c.Blocks = nil

	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		c.Text = &text
		return nil
	}

	var blocks []ContentBlock
	if err := json.Unmarshal(data, &blocks); err == nil {
		c.Blocks = blocks
		return nil
	}

	return nil
}

func (c SDKContent) MarshalJSON() ([]byte, error) {
	if c.Text != nil {
		return json.Marshal(*c.Text)
	}
	if c.Blocks != nil {
		return json.Marshal(c.Blocks)
	}
	if c.Raw != nil {
		return c.Raw, nil
	}
	return []byte("null"), nil
}

func (c SDKContent) IsZero() bool {
	return c.Text == nil && len(c.Blocks) == 0 && len(c.Raw) == 0
}

func (c SDKContent) PlainText() string {
	if c.Text != nil {
		return *c.Text
	}
	if len(c.Blocks) == 0 {
		return ""
	}

	parts := make([]string, 0, len(c.Blocks))
	for _, block := range c.Blocks {
		switch block.Type {
		case "text":
			if strings.TrimSpace(block.Text) != "" {
				parts = append(parts, block.Text)
			}
		case "tool_result":
			if text := block.ContentText(); strings.TrimSpace(text) != "" {
				parts = append(parts, text)
			}
		}
	}
	return strings.Join(parts, "\n")
}

func (c SDKContent) HasToolResult() bool {
	for _, block := range c.Blocks {
		if block.Type == "tool_result" {
			return true
		}
	}
	return false
}

type ContentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     map[string]any  `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
	Raw       json.RawMessage `json:"-"`
}

func (b *ContentBlock) UnmarshalJSON(data []byte) error {
	type alias ContentBlock
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*b = ContentBlock(decoded)
	b.Raw = append(b.Raw[:0], data...)
	return nil
}

func (b ContentBlock) ContentText() string {
	if len(b.Content) == 0 {
		return ""
	}
	var text string
	if err := json.Unmarshal(b.Content, &text); err == nil {
		return text
	}
	var blocks []ContentBlock
	if err := json.Unmarshal(b.Content, &blocks); err == nil {
		return BlockContent(blocks...).PlainText()
	}
	return string(b.Content)
}

type ToolUseBlock struct {
	ID    string
	Name  string
	Input map[string]any
}

type ToolResultBlock struct {
	ToolUseID string
	Content   string
	IsError   bool
}

type SDKStreamEvent struct {
	Type         string          `json:"type,omitempty"`
	Index        int             `json:"index,omitempty"`
	Delta        SDKStreamDelta  `json:"delta,omitempty"`
	ContentBlock ContentBlock    `json:"content_block,omitempty"`
	Raw          json.RawMessage `json:"-"`
}

func (e *SDKStreamEvent) UnmarshalJSON(data []byte) error {
	type alias SDKStreamEvent
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*e = SDKStreamEvent(decoded)
	e.Raw = append(e.Raw[:0], data...)
	return nil
}

func (e SDKStreamEvent) DeltaText() string {
	switch {
	case e.Delta.Text != "":
		return e.Delta.Text
	case e.Delta.Thinking != "":
		return e.Delta.Thinking
	case e.Delta.PartialJSON != "":
		return e.Delta.PartialJSON
	default:
		return ""
	}
}

type SDKStreamDelta struct {
	Type        string `json:"type,omitempty"`
	Text        string `json:"text,omitempty"`
	Thinking    string `json:"thinking,omitempty"`
	PartialJSON string `json:"partial_json,omitempty"`
}

type CompactMetadata struct {
	Trigger          string            `json:"trigger,omitempty"`
	PreTokens        int               `json:"pre_tokens,omitempty"`
	PreservedSegment *PreservedSegment `json:"preserved_segment,omitempty"`
}

type PreservedSegment struct {
	HeadUUID   string `json:"head_uuid,omitempty"`
	AnchorUUID string `json:"anchor_uuid,omitempty"`
	TailUUID   string `json:"tail_uuid,omitempty"`
}

type PermissionDenial struct {
	ToolName  string         `json:"tool_name,omitempty"`
	ToolUseID string         `json:"tool_use_id,omitempty"`
	ToolInput map[string]any `json:"tool_input,omitempty"`
}
