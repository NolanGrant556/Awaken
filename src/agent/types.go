package agent

import "context"

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Thought struct {
	Index       int    `json:"index"`
	Thought     string `json:"thought"`
	Action      string `json:"action,omitempty"`
	ActionInput string `json:"action_input,omitempty"`
	Observation string `json:"observation,omitempty"`
}

type AgentConfig struct {
	MaxThoughts     int
	ModelConfig     LLMConfig
	SystemPrompt    string
}

type LLMConfig struct {
	APIKey  string
	BaseURL string
	Model   string
}

type AgentState struct {
	Messages []Message
	Thoughts []Thought
}

type Tool interface {
	Name() string
	Description() string
	Parameters() []ToolParameter
	Execute(ctx context.Context, input string) (string, error)
}

type ToolParameter struct {
	Name        string
	Description string
	Type        string
	Required    bool
}

type ActionResult struct {
	Success     bool
	Output      string
	Error       string
}