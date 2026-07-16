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
	Role              Role                    `json:"role"`
	Content           string                  `json:"content"`
	Attachments       []MessageAttachment     `json:"attachments,omitempty"`
	ToolCalls         []ToolCall              `json:"tool_calls,omitempty"`
	ToolCallID        string                  `json:"tool_call_id,omitempty"`
	Name              string                  `json:"name,omitempty"`
	IsError           bool                    `json:"is_error,omitempty"`
	CompactBoundaryID string                  `json:"compact_boundary_id,omitempty"`
	ModelContent      []tool.ModelContentPart `json:"model_content,omitempty"`
}

type MessageAttachment struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	MimeType    string `json:"mime_type"`
	Path        string `json:"path,omitempty"`
	DataBase64  string `json:"data_base64,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	Width       int    `json:"width,omitempty"`
	Height      int    `json:"height,omitempty"`
	ByteSize    int    `json:"byte_size,omitempty"`
	StorageKey  string `json:"storage_key,omitempty"`
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
	tools       []tool.ToolSchema
}

const roughImageAttachmentTokenEstimate = 2000

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

func WithTools(tools []tool.ToolSchema) Option {
	return func(c *Context) {
		c.tools = tools
	}
}

func New(opts ...Option) *Context {
	c := &Context{
		messages:    []Message{},
		maxMessages: 100,
		maxTokens:   32000,
		tools:       []tool.ToolSchema{},
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

func (c *Context) AddUserMessage(content string) {
	c.AddUserMessageWithAttachments(content, nil)
}

func (c *Context) AddUserMessageWithAttachments(content string, attachments []MessageAttachment) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.messages = append(c.messages, Message{
		Role:        RoleUser,
		Content:     content,
		Attachments: cloneAttachments(attachments),
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
	c.AddToolResultWithModelContent(toolCallID, name, content, nil, false)
}

func (c *Context) AddToolResultWithModelContent(toolCallID, name, content string, modelContent []tool.ModelContentPart, isError bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.messages = append(c.messages, Message{
		Role:         RoleTool,
		ToolCallID:   toolCallID,
		Name:         name,
		Content:      content,
		ModelContent: tool.CloneModelContent(modelContent),
		IsError:      isError,
	})
}

func (c *Context) AddToolResultWithError(toolCallID, name, content string) {
	c.AddToolResultWithModelContent(toolCallID, name, content, nil, true)
}

func (c *Context) GetMessages() []Message {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return cloneMessages(c.messages)
}

func (c *Context) GetMessagesForModel() []Message {
	c.mu.RLock()
	defer c.mu.RUnlock()

	start := 0
	for i, msg := range c.messages {
		if strings.TrimSpace(msg.CompactBoundaryID) != "" {
			start = i
		}
	}
	return cloneMessages(c.messages[start:])
}

func (c *Context) GetTools() []tool.ToolSchema {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.tools
}

func (c *Context) SetMessages(messages []Message) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.messages = cloneMessages(messages)
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

	if c.maxMessages > 0 && len(c.messages) > c.maxMessages {
		c.messages = c.messages[len(c.messages)-c.maxMessages:]
	}
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
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}
	wordEstimate := len(strings.Fields(text)) * 4 / 3
	charEstimate := len([]rune(text)) / 4
	if charEstimate == 0 {
		charEstimate = 1
	}
	if wordEstimate > charEstimate {
		return wordEstimate
	}
	return charEstimate
}

func EstimateAttachmentPayloadTokens(kind string, content string) int {
	if strings.EqualFold(strings.TrimSpace(kind), "image") {
		return roughImageAttachmentTokenEstimate
	}
	return EstimateTokens(content)
}

func EstimateMessageAttachmentTokens(attachment MessageAttachment) int {
	total := 0
	total += EstimateTokens(attachment.Kind)
	total += EstimateTokens(attachment.MimeType)
	total += EstimateTokens(attachment.Path)
	total += EstimateTokens(attachment.DisplayName)
	total += EstimateAttachmentPayloadTokens(attachment.Kind, attachment.DataBase64)
	return total
}

func EstimateModelContentTokens(part tool.ModelContentPart) int {
	total := EstimateTokens(part.Type) + EstimateTokens(part.MimeType) + EstimateTokens(part.DisplayName)
	if part.Type == "image" || part.Type == "file" {
		return total + roughImageAttachmentTokenEstimate
	}
	return total
}

func (c *Context) EstimateTotalTokens() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	total := 0
	for _, msg := range c.messages {
		total += EstimateTokens(msg.Content)
		for _, attachment := range msg.Attachments {
			total += EstimateMessageAttachmentTokens(attachment)
		}
		for _, part := range msg.ModelContent {
			total += EstimateModelContentTokens(part)
		}
		for _, tc := range msg.ToolCalls {
			total += EstimateTokens(tc.Function.Arguments)
			total += EstimateTokens(tc.Function.Name)
		}
	}
	return total
}

func cloneMessages(messages []Message) []Message {
	result := make([]Message, len(messages))
	for i, message := range messages {
		result[i] = message
		result[i].Attachments = cloneAttachments(message.Attachments)
		result[i].ModelContent = tool.CloneModelContent(message.ModelContent)
		result[i].ToolCalls = append([]ToolCall(nil), message.ToolCalls...)
	}
	return result
}

func cloneAttachments(attachments []MessageAttachment) []MessageAttachment {
	if len(attachments) == 0 {
		return nil
	}
	result := make([]MessageAttachment, len(attachments))
	copy(result, attachments)
	return result
}

type Conversation struct {
	Context   *Context
	CreatedAt time.Time
	UpdatedAt time.Time
	Metadata  map[string]interface{}
}

func NewConversation(tools []tool.ToolSchema) *Conversation {
	return &Conversation{
		Context:   New(WithTools(tools)),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Metadata:  make(map[string]interface{}),
	}
}

func (conv *Conversation) UserInput(input string) {
	conv.UserInputWithAttachments(input, nil)
}

func (conv *Conversation) UserInputWithAttachments(input string, attachments []MessageAttachment) {
	conv.Context.AddUserMessageWithAttachments(input, attachments)
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
	ToolCallID   string
	Name         string
	Content      string
	IsError      bool
	ModelContent []tool.ModelContentPart
}

func (conv *Conversation) ToolExchange(content string, toolCalls []ToolCall, results []ToolResult) {
	conv.AssistantOutput(content, toolCalls)
	for _, result := range results {
		conv.Context.AddToolResultWithModelContent(result.ToolCallID, result.Name, result.Content, result.ModelContent, result.IsError)
	}
	conv.UpdatedAt = time.Now()
}

func (conv *Conversation) Reset() {
	conv.Context.Clear()
	conv.UpdatedAt = time.Now()
}
