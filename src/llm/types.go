package llm

import (
	stdctx "context"
	"encoding/json"
	"os"
	"strings"
	"time"

	"awaken/src/context"
	"awaken/src/tool"
)

// Protocol 定义 LLM 协议类型
type Protocol string

const (
	ProtocolOpenAIChat        Protocol = "openai_chat"
	ProtocolOpenAIResponses   Protocol = "openai_responses"
	ProtocolAnthropicMessages Protocol = "anthropic_messages"
	ProtocolMiniMaxChat       Protocol = "minimax_chat"
	protocolInvalid           Protocol = "invalid"
)

// Config 是 LLM 配置
type Config struct {
	AccountScopeID        string
	RequestScheduler      *RequestScheduler
	APIKey                string
	BaseURL               string
	Model                 string
	CapabilityModelID     string
	EndpointSelector      string
	ModelSelectorKind     string
	Protocol              Protocol
	ProviderID            string
	ProfileID             string
	CredentialRevision    uint64
	Temperature           float64
	MaxTokens             int
	AuthScheme            string
	InferenceEndpoint     string
	ContextWindow         int
	SupportsPromptCache   bool
	AutoCompactTokenLimit int
	ReasoningEffort       string
}

type RequestPurpose string

const (
	RequestPurposeAgentTurn         RequestPurpose = "agent_turn"
	RequestPurposeSubagent          RequestPurpose = "subagent"
	RequestPurposeScheduledTask     RequestPurpose = "scheduled_task"
	RequestPurposeThreadTitle       RequestPurpose = "thread_title"
	RequestPurposeCompactSummary    RequestPurpose = "compact_summary"
	RequestPurposeNativeCompaction  RequestPurpose = "native_compaction"
	RequestPurposeMemoryRecall      RequestPurpose = "memory_recall"
	RequestPurposeMemoryMaintenance RequestPurpose = "memory_maintenance"
	RequestPurposeConnectionTest    RequestPurpose = "connection_test"
	RequestPurposeHookPrompt        RequestPurpose = "hook_prompt"
	RequestPurposeHookAgent         RequestPurpose = "hook_agent"
	RequestPurposeResearchSynthesis RequestPurpose = "research_synthesis"
)

// Request 是统一请求结构
type Request struct {
	RequestID           string
	SystemPrompt        string
	UserPrompt          string
	Model               string
	Temperature         float64
	TemperatureOverride *float64
	OmitTemperature     bool
	MaxTokens           int
	Stream              bool
	StreamIdleTimeout   time.Duration
	DisableReasoning    bool
	Purpose             RequestPurpose
	AffinityScopeID     string
	ReasoningPolicy     string
	Messages            []context.Message
	Tools               []tool.ToolSchema
	// NativeCompaction asks the exact OpenAI Responses transport to invoke
	// POST /responses/compact instead of generating an assistant response.
	// It is ignored by protocols and endpoints without an explicit capability.
	NativeCompaction bool
	// NativeCompactionState is opaque provider output from a previous native
	// compaction. Compatible OpenAI Responses requests prepend these items to
	// input; incompatible identities are ignored and the host checkpoint remains
	// authoritative.
	NativeCompactionState *NativeCompactionState
}

type NativeCompactionState struct {
	ProviderID         string
	ProfileID          string
	CredentialRevision uint64
	ModelID            string
	ProtocolFamily     Protocol
	EndpointContractID string
	Output             []json.RawMessage
	Digest             string
}

// PreparedRequest is the immutable, protocol-lowered HTTP request. Callers
// persist its digest before invoking SendPrepared; transports must not rebuild
// or mutate Body after this point.
type PreparedRequest struct {
	AffinityScopeID    string
	Timing             *RequestTiming
	Purpose            RequestPurpose
	ReasoningPolicy    string
	RequestID          string
	Protocol           Protocol
	Endpoint           string
	Body               []byte
	RequestShapeDigest string
	Stream             bool
	StreamIdleTimeout  time.Duration
	Cache              PreparedPromptCache
	Operation          PreparedOperation
}

type RequestTiming struct {
	PrepareMS        int64
	QueueWaitMS      int64
	TransportMS      int64
	TransportStarted time.Time
	FirstTokenMS     *int64
}

type PreparedOperation string

const (
	PreparedOperationModelResponse    PreparedOperation = "model_response"
	PreparedOperationNativeCompaction PreparedOperation = "native_compaction"
)

type PreparedPromptCache struct {
	CapabilityID     string
	EndpointContract string
	Mode             PromptCacheMode
	KeyApplied       bool
	ReasoningReplay  bool
}

// Response 是统一响应结构
type Response struct {
	Content               string
	Thinking              string
	ToolCalls             []ToolCall
	Usage                 *Usage
	IsFinished            bool
	StopReason            string
	MaxOutputReached      bool
	ProviderRequestID     string
	RateLimitObservation  *RateLimitObservation
	ProviderOpaqueState   *context.ProviderOpaqueMessageState
	NativeCompactionState *NativeCompactionState
}

// RateLimitObservation preserves the most recent provider response-header
// facts without interpreting provider-specific reset formats.
type RateLimitObservation struct {
	RequestLimit     string
	RequestRemaining string
	RequestReset     string
	TokenLimit       string
	TokenRemaining   string
	TokenReset       string
	ObservedAt       time.Time
}

// Usage 是 provider 返回的可选 token 用量信息。
type Usage struct {
	InputTokens         *int
	CachedInputTokens   *int
	UncachedInputTokens *int
	CacheWriteTokens    *int
	OutputTokens        *int
	ReasoningTokens     *int
	TotalTokens         *int
	Raw                 RawUsage
}

type RawUsage struct {
	NormalizerID             string `json:"normalizer_id"`
	PromptTokens             *int   `json:"prompt_tokens,omitempty"`
	InputTokens              *int   `json:"input_tokens,omitempty"`
	CompletionTokens         *int   `json:"completion_tokens,omitempty"`
	OutputTokens             *int   `json:"output_tokens,omitempty"`
	ReasoningTokens          *int   `json:"reasoning_tokens,omitempty"`
	TotalTokens              *int   `json:"total_tokens,omitempty"`
	CachedTokens             *int   `json:"cached_tokens,omitempty"`
	PromptCacheHitTokens     *int   `json:"prompt_cache_hit_tokens,omitempty"`
	PromptCacheMissTokens    *int   `json:"prompt_cache_miss_tokens,omitempty"`
	CacheReadInputTokens     *int   `json:"cache_read_input_tokens,omitempty"`
	CacheCreationInputTokens *int   `json:"cache_creation_input_tokens,omitempty"`
}

// ToolCall 是统一工具调用结构
type ToolCall struct {
	ID    string
	Name  string
	Input string
}

// StreamCallback 是流式回调函数
type StreamCallback func(content string, isThinking bool)

type ToolCallDelta struct {
	Index          int
	ID             string
	Name           string
	ArgumentsDelta string
}

type StreamObserver struct {
	OnContent       StreamCallback
	OnToolCallDelta func(ToolCallDelta)
	OnEvent         func(StreamEvent)
}

type StreamEventType string

const (
	StreamEventTextDelta      StreamEventType = "text_delta"
	StreamEventReasoningDelta StreamEventType = "reasoning_delta"
	StreamEventToolCallStart  StreamEventType = "tool_call_start"
	StreamEventToolCallDelta  StreamEventType = "tool_call_delta"
	StreamEventToolCallDone   StreamEventType = "tool_call_done"
	StreamEventUsage          StreamEventType = "usage"
	StreamEventCompleted      StreamEventType = "completed"
	StreamEventFailed         StreamEventType = "failed"
)

// StreamEvent is the provider-neutral stream contract. Existing callbacks
// remain supported, while new protocol adapters publish the same typed events
// so callers never need to inspect provider SSE payloads.
type StreamEvent struct {
	Type          StreamEventType
	Content       string
	IsThinking    bool
	ToolCallDelta *ToolCallDelta
	ToolCall      *ToolCall
	Usage         *Usage
	StopReason    string
	ErrorCode     string
}

// Transport 是 LLM 传输层接口
type Transport interface {
	Invoke(ctx stdctx.Context, cfg Config, req Request) (*Response, error)
	StreamInvoke(ctx stdctx.Context, cfg Config, req Request, callback StreamCallback) (*Response, error)
}

type observerTransport interface {
	StreamInvokeWithObserver(ctx stdctx.Context, cfg Config, req Request, observer StreamObserver) (*Response, error)
}

func isMaxOutputStopReason(stopReason string) bool {
	switch strings.ToLower(strings.TrimSpace(stopReason)) {
	case "length", "max_tokens", "max_output", "max_output_tokens":
		return true
	default:
		return false
	}
}

func responseHasSignal(resp *Response) bool {
	if resp == nil {
		return false
	}
	return resp.Content != "" || resp.Thinking != "" || len(resp.ToolCalls) > 0
}

// resolveProtocol 解析协议类型
func resolveProtocol(cfg Config) Protocol {
	if cfg.Protocol != "" && cfg.Protocol != "auto" {
		switch cfg.Protocol {
		case ProtocolOpenAIChat:
			return ProtocolOpenAIChat
		case ProtocolOpenAIResponses:
			return ProtocolOpenAIResponses
		case ProtocolAnthropicMessages:
			return ProtocolAnthropicMessages
		case ProtocolMiniMaxChat:
			return ProtocolMiniMaxChat
		default:
			return protocolInvalid
		}
	}

	// 检查环境变量
	protocol := os.Getenv("LLM_PROTOCOL")
	if protocol != "" && protocol != "auto" {
		switch protocol {
		case "openai_chat":
			return ProtocolOpenAIChat
		case "openai_responses":
			return ProtocolOpenAIResponses
		case "anthropic_messages":
			return ProtocolAnthropicMessages
		case "minimax_chat":
			return ProtocolMiniMaxChat
		default:
			return protocolInvalid
		}
	}
	// Empty/auto remains a legacy OpenAI Chat default. Non-empty unknown values
	// fail closed above; endpoint strings are never used to guess a protocol.
	return ProtocolOpenAIChat
}

// ResolveProtocol 导出协议解析能力，供 llm 包外部（如 cmd/awaken-core）使用。
// 内部仍复用 resolveProtocol，避免改动既有传输选择路径。
func ResolveProtocol(cfg Config) Protocol {
	return resolveProtocol(cfg)
}
