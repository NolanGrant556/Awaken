package context

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"awaken/src/tool"
)

type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

type Message struct {
	Role       Role       `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`
	IsError    bool       `json:"is_error,omitempty"`
}

type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type Context struct {
	mu          sync.RWMutex
	messages    []Message
	maxMessages int
	maxTokens   int
	tools       []tool.ToolDefinition
}

type Option func(*Context)

func WithMaxMessages(max int) Option {
	return func(c *Context) {
		c.maxMessages = max
	}
}

func WithMaxTokens(max int) Option {
	return func(c *Context) {
		c.maxTokens = max
	}
}

func WithTools(tools []tool.ToolDefinition) Option {
	return func(c *Context) {
		c.tools = tools
	}
}

func New(opts ...Option) *Context {
	c := &Context{
		messages:    []Message{},
		maxMessages: 100,
		maxTokens:   32000,
		tools:       []tool.ToolDefinition{},
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

func (c *Context) AddUserMessage(content string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.messages = append(c.messages, Message{
		Role:    RoleUser,
		Content: content,
	})
}

func (c *Context) AddAssistantMessage(content string, toolCalls []ToolCall) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.messages = append(c.messages, Message{
		Role:      RoleAssistant,
		Content:   content,
		ToolCalls: toolCalls,
	})
}

func (c *Context) AddToolMessage(toolCallID, content string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.messages = append(c.messages, Message{
		Role:       RoleTool,
		ToolCallID: toolCallID,
		Content:    content,
	})
}

func (c *Context) AddToolResult(toolCallID, name, content string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.messages = append(c.messages, Message{
		Role:       RoleTool,
		ToolCallID: toolCallID,
		Name:       name,
		Content:    content,
	})
}

func (c *Context) AddToolResultWithError(toolCallID, name, content string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.messages = append(c.messages, Message{
		Role:       RoleTool,
		ToolCallID: toolCallID,
		Name:       name,
		Content:    content,
		IsError:    true,
	})
}

func (c *Context) GetMessages() []Message {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make([]Message, len(c.messages))
	copy(result, c.messages)
	return result
}

func (c *Context) GetTools() []tool.ToolDefinition {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.tools
}

func (c *Context) SetMessages(messages []Message) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.messages = messages
}

func (c *Context) GetLastUserMessage() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for i := len(c.messages) - 1; i >= 0; i-- {
		if c.messages[i].Role == RoleUser {
			return c.messages[i].Content
		}
	}
	return ""
}

func (c *Context) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.messages = []Message{}
}

func (c *Context) Trim() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.messages, _ = compactMessages(c.messages, c.maxMessages, c.maxTokens, 0)
}

func (c *Context) ToLLMFormat() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := map[string]interface{}{
		"messages": c.messages,
	}

	if len(c.tools) > 0 {
		result["tools"] = c.tools
	}

	return result
}

func (c *Context) ToJSON() string {
	format := c.ToLLMFormat()
	data, err := json.Marshal(format)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func GenerateToolCallID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("call_%x", b)
}

func ParseToolArguments(args string) (map[string]interface{}, error) {
	var result map[string]interface{}
	err := json.Unmarshal([]byte(args), &result)
	return result, err
}

func BuildToolArguments(params map[string]interface{}) string {
	data, err := json.Marshal(params)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func EstimateTokens(text string) int {
	return len(strings.Fields(text)) * 4 / 3
}

func (c *Context) EstimateTotalTokens() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	total, _ := estimateMessages(c.messages)
	return total
}

func (c *Context) EstimateTotalChars() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	_, total := estimateMessages(c.messages)
	return total
}

type CompactResult struct {
	OriginalMessages  int  `json:"original_messages"`
	RemainingMessages int  `json:"remaining_messages"`
	DroppedMessages   int  `json:"dropped_messages"`
	OriginalTokens    int  `json:"original_tokens"`
	RemainingTokens   int  `json:"remaining_tokens"`
	OriginalChars     int  `json:"original_chars"`
	RemainingChars    int  `json:"remaining_chars"`
	Compacted         bool `json:"compacted"`
}

func (c *Context) CompactToFit(maxMessages, maxTokens, maxChars int) CompactResult {
	c.mu.Lock()
	defer c.mu.Unlock()

	compacted, result := compactMessages(c.messages, maxMessages, maxTokens, maxChars)
	c.messages = compacted
	return result
}

type Conversation struct {
	Context   *Context
	CreatedAt time.Time
	UpdatedAt time.Time
	Metadata  map[string]interface{}
}

func NewConversation(tools []tool.ToolDefinition) *Conversation {
	return &Conversation{
		Context:   New(WithTools(tools)),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Metadata:  make(map[string]interface{}),
	}
}

func (conv *Conversation) UserInput(input string) {
	conv.Context.AddUserMessage(input)
	conv.UpdatedAt = time.Now()
}

func (conv *Conversation) AssistantOutput(content string, toolCalls []ToolCall) {
	conv.Context.AddAssistantMessage(content, toolCalls)
	conv.UpdatedAt = time.Now()
}

func (conv *Conversation) ToolResult(toolCallID, name, result string) {
	conv.Context.AddToolResult(toolCallID, name, result)
	conv.UpdatedAt = time.Now()
}

type ToolResult struct {
	ToolCallID string
	Name       string
	Content    string
	IsError    bool
}

func (conv *Conversation) ToolExchange(content string, toolCalls []ToolCall, results []ToolResult) {
	conv.AssistantOutput(content, toolCalls)
	for _, result := range results {
		if result.IsError {
			conv.Context.AddToolResultWithError(result.ToolCallID, result.Name, result.Content)
			continue
		}
		conv.Context.AddToolResult(result.ToolCallID, result.Name, result.Content)
	}
	conv.UpdatedAt = time.Now()
}

func (conv *Conversation) Reset() {
	conv.Context.Clear()
	conv.UpdatedAt = time.Now()
}

func CompactMessages(messages []Message, maxMessages, maxTokens, maxChars int) ([]Message, CompactResult) {
	compacted, result := compactMessages(messages, maxMessages, maxTokens, maxChars)
	return compacted, result
}

func compactMessages(messages []Message, maxMessages, maxTokens, maxChars int) ([]Message, CompactResult) {
	original := append([]Message(nil), messages...)
	originalTokens, originalChars := estimateMessages(original)
	result := CompactResult{
		OriginalMessages:  len(original),
		RemainingMessages: len(original),
		OriginalTokens:    originalTokens,
		RemainingTokens:   originalTokens,
		OriginalChars:     originalChars,
		RemainingChars:    originalChars,
	}

	if len(original) == 0 || !messagesExceedLimits(original, maxMessages, maxTokens, maxChars) {
		return original, result
	}

	spans := messageSpans(original)
	if len(spans) == 0 {
		return original, result
	}

	keepIdx := 0
	compacted := original
	for keepIdx < len(spans)-1 && messagesExceedLimits(compacted, maxMessages, maxTokens, maxChars) {
		keepIdx++
		compacted = append([]Message(nil), original[spans[keepIdx].start:]...)
	}

	if messagesExceedLimits(compacted, maxMessages, maxTokens, maxChars) {
		last := spans[len(spans)-1]
		compacted = append([]Message(nil), original[last.start:last.end]...)
	}

	if err := ValidateToolResultPairing(compacted); err != nil {
		return original, result
	}

	remainingTokens, remainingChars := estimateMessages(compacted)
	result.RemainingMessages = len(compacted)
	result.DroppedMessages = len(original) - len(compacted)
	result.RemainingTokens = remainingTokens
	result.RemainingChars = remainingChars
	result.Compacted = result.DroppedMessages > 0
	return compacted, result
}

type messageSpan struct {
	start int
	end   int
}

func messageSpans(messages []Message) []messageSpan {
	spans := make([]messageSpan, 0, len(messages))
	for i := 0; i < len(messages); {
		start := i
		i++

		if messages[start].Role == RoleAssistant && len(messages[start].ToolCalls) > 0 {
			pending := make(map[string]struct{}, len(messages[start].ToolCalls))
			for _, tc := range messages[start].ToolCalls {
				if tc.ID != "" {
					pending[tc.ID] = struct{}{}
				}
			}
			for i < len(messages) && len(pending) > 0 {
				if messages[i].Role != RoleTool {
					break
				}
				delete(pending, messages[i].ToolCallID)
				i++
			}
		}

		spans = append(spans, messageSpan{start: start, end: i})
	}
	return spans
}

func messagesExceedLimits(messages []Message, maxMessages, maxTokens, maxChars int) bool {
	if maxMessages > 0 && len(messages) > maxMessages {
		return true
	}
	tokens, chars := estimateMessages(messages)
	if maxTokens > 0 && tokens > maxTokens {
		return true
	}
	if maxChars > 0 && chars > maxChars {
		return true
	}
	return false
}

func estimateMessages(messages []Message) (tokens int, chars int) {
	for _, msg := range messages {
		tokens += EstimateTokens(msg.Content)
		chars += len(msg.Content)
		if msg.Name != "" {
			tokens += EstimateTokens(msg.Name)
			chars += len(msg.Name)
		}
		for _, tc := range msg.ToolCalls {
			tokens += EstimateTokens(tc.Function.Arguments)
			tokens += EstimateTokens(tc.Function.Name)
			chars += len(tc.Function.Arguments)
			chars += len(tc.Function.Name)
		}
	}
	return tokens, chars
}
