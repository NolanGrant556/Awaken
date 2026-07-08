package askuser

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"awaken/src/tool"
)

const ToolName = "ask_user"

type QuestionOption struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

type Question struct {
	Question    string           `json:"question"`
	Header      string           `json:"header,omitempty"`
	Options     []QuestionOption `json:"options"`
	MultiSelect bool             `json:"multi_select,omitempty"`
}

type askUserInput struct {
	Questions []Question `json:"questions"`
}

type AskUserTool struct{}

func (t *AskUserTool) Name() string {
	return ToolName
}

func (t *AskUserTool) Description() string {
	return "当任务信息不足、需要用户确认偏好、或必须让用户做选择时，向用户提问并等待回答。提供清晰的问题和 2-4 个选项；用户回答后你方可继续。示例：{\"questions\":[{\"question\":\"选择主题\",\"options\":[{\"label\":\"深色\"},{\"label\":\"浅色\"}],\"multi_select\":false}]}。"
}

func (t *AskUserTool) Parameters() []tool.ToolParameter {
	return []tool.ToolParameter{
		{
			Name: "questions_json",
			Description: "问题列表的 JSON 字符串。支持对象包装 {\"questions\": [...]} 或直接数组 [...]。" +
				"示例：{\"questions\":[{\"question\":\"选择主题\",\"options\":[{\"label\":\"深色\"},{\"label\":\"浅色\"}],\"multi_select\":false}]}。" +
				"每个问题须有 2-4 个选项，多选时设置 multi_select 为 true。",
			Type:     "string",
			Required: true,
		},
	}
}

func (t *AskUserTool) Definition() tool.ToolDefinition {
	return tool.ToolDefinition{
		Name:        ToolName,
		DisplayName: "Ask User",
		Source:      tool.SourceBuiltin,
		Description: "向用户提问并等待回答",
		Annotations: tool.ToolAnnotations{
			ReadOnly:        true,
			Destructive:     false,
			CapabilityClass: tool.CapabilityLocalTransform,
			TrustedOrigin:   tool.TrustedOriginBuiltin,
		},
	}
}

func (t *AskUserTool) Execute(ctx context.Context, input string) (string, error) {
	return "", fmt.Errorf("ask_user should be handled by the runtime awaiter, not executed directly")
}

func ParseQuestions(input string) ([]Question, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, fmt.Errorf("questions_json is required")
	}

	var questions []Question

	// Try object wrapper first: {"questions": [...]}
	var wrapped askUserInput
	if err := json.Unmarshal([]byte(input), &wrapped); err == nil && len(wrapped.Questions) > 0 {
		questions = wrapped.Questions
	} else {
		// Fall back to bare array: [...]
		if err := json.Unmarshal([]byte(input), &questions); err != nil {
			return nil, fmt.Errorf("invalid questions_json: %w", err)
		}
	}

	if len(questions) == 0 {
		return nil, fmt.Errorf("at least one question is required")
	}

	for i, q := range questions {
		q.Question = strings.TrimSpace(q.Question)
		if q.Question == "" {
			return nil, fmt.Errorf("question %d is empty", i)
		}
		if len(q.Options) < 2 {
			return nil, fmt.Errorf("question %d must have at least 2 options", i)
		}
		if len(q.Options) > 4 {
			return nil, fmt.Errorf("question %d must have at most 4 options", i)
		}
		for j, opt := range q.Options {
			opt.Label = strings.TrimSpace(opt.Label)
			if opt.Label == "" {
				return nil, fmt.Errorf("question %d option %d label is empty", i, j)
			}
			q.Options[j] = opt
		}
		questions[i] = q
	}

	return questions, nil
}

func FormatAnswerResult(questions []Question, answers map[string]string) string {
	parts := make([]string, 0, len(answers))
	for _, q := range questions {
		answer, ok := answers[q.Question]
		if !ok {
			continue
		}
		parts = append(parts, fmt.Sprintf("%q=%q", q.Question, answer))
	}
	return "User has answered your questions: " + strings.Join(parts, ", ")
}

func FormatAnswerTextResult(questions []Question, answer string) string {
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return "User has answered your questions."
	}
	if len(questions) == 1 {
		question := strings.TrimSpace(questions[0].Question)
		prefix := question + ":"
		if strings.HasPrefix(answer, prefix) {
			trimmedAnswer := strings.TrimSpace(strings.TrimPrefix(answer, prefix))
			if trimmedAnswer != "" {
				answer = trimmedAnswer
			}
		}
		if !strings.Contains(answer, "\n") {
			return FormatAnswerResult(questions, map[string]string{question: answer})
		}
	}
	return "User has answered your questions:\n" + answer
}

func init() {
	tool.MustRegisterTool(&AskUserTool{})
}
