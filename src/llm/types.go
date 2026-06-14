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
	APIKey      string
	BaseURL     string
	Model       string
	Protocol    Protocol
	Temperature float64
	MaxTokens   int
	AuthScheme  string
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
	Content    string
	ToolCalls  []ToolCall
	IsFinished bool
}

// ToolCall 是统一工具调用结构
type ToolCall struct {
	ID    string
	Name  string
	Input string
}

// StreamCallback 是流式回调函数
type StreamCallback func(content string, isThinking bool)

// Transport 是 LLM 传输层接口
type Transport interface {
	Invoke(ctx stdctx.Context, cfg Config, req Request) (*Response, error)
	StreamInvoke(ctx stdctx.Context, cfg Config, req Request, callback StreamCallback) (*Response, error)
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
