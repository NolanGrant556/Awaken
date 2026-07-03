package llm

import (
	stdctx "context"
	"os"
	"strings"

	"awaken/src/context"
	"awaken/src/tool"
)

// Protocol 定义 LLM 协议类型
type Protocol string

const (
	ProtocolOpenAIChat        Protocol = "openai_chat"
	ProtocolAnthropicMessages Protocol = "anthropic_messages"
)

// Config 是 LLM 配置
type Config struct {
	APIKey                string
	BaseURL               string
	Model                 string
	Protocol              Protocol
	Temperature           float64
	MaxTokens             int
	AuthScheme            string
	ContextWindow         int
	AutoCompactTokenLimit int
}

// Request 是统一请求结构
type Request struct {
	SystemPrompt string
	UserPrompt   string
	Model        string
	Temperature  float64
	MaxTokens    int
	Stream       bool
	Messages     []context.Message
	Tools        []tool.ToolDefinition
}

// Response 是统一响应结构
type Response struct {
	Content          string
	Thinking         string
	ToolCalls        []ToolCall
	Usage            *Usage
	IsFinished       bool
	StopReason       string
	MaxOutputReached bool
}

// Usage 是 provider 返回的可选 token 用量信息。
type Usage struct {
	InputTokens  int
	OutputTokens int
	TotalTokens  int
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
		case ProtocolAnthropicMessages:
			return ProtocolAnthropicMessages
		}
	}

	// 检查环境变量
	protocol := os.Getenv("LLM_PROTOCOL")
	if protocol != "" && protocol != "auto" {
		switch protocol {
		case "openai_chat":
			return ProtocolOpenAIChat
		case "anthropic_messages":
			return ProtocolAnthropicMessages
		}
	}

	// 自动判断
	baseURL := strings.ToLower(cfg.BaseURL)
	if strings.Contains(baseURL, "api.anthropic.com") {
		return ProtocolAnthropicMessages
	}
	if strings.HasSuffix(baseURL, "/anthropic") {
		return ProtocolAnthropicMessages
	}
	if strings.Contains(baseURL, "/anthropic/") {
		return ProtocolAnthropicMessages
	}

	return ProtocolOpenAIChat
}

// ResolveProtocol 导出协议解析能力，供 llm 包外部（如 cmd/awaken-core）使用。
// 内部仍复用 resolveProtocol，避免改动既有传输选择路径。
func ResolveProtocol(cfg Config) Protocol {
	return resolveProtocol(cfg)
}
