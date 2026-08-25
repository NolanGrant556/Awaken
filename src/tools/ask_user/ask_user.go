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
	return "当任务信息不足、需要用户确认偏好、或必须让用户做选择时，向用户提问并等待用户回答或取消。提供清晰的问题和 2-4 个选项；runtime 会把回答或取消作为工具结果返回，然后你方可继续。示例：{\"questions\":[{\"question\":\"选择主题\",\"options\":[{\"label\":\"深色\"},{\"label\":\"浅色\"}],\"multi_select\":false}]}。"
}

func (t *AskUserTool) Parameters() []tool.ToolParameter {
	return []tool.ToolParameter{
		{
			Name:        "questions",
			Description: "1-3 个结构化问题；每个问题必须提供 2-4 个明确选项。",
			Type:        "array",
			Required:    true,
		},
	}
}

// InputSchema exposes one canonical object shape to every model/provider.
// Legacy questions_json and bare-array inputs are normalized by
// CanonicalizeLegacyArguments before runtime validation, but are intentionally
// absent here so new model calls cannot choose an ambiguous schema branch.
func (t *AskUserTool) InputSchema() tool.ToolParametersSchema {
	noExtra := false
	questions := questionArraySchema(&noExtra)
	return tool.ToolParametersSchema{
		Type:                 "object",
		AdditionalProperties: &noExtra,
		Required:             []string{"questions"},
		Properties:           map[string]tool.ToolParam{"questions": questions},
	}
}

func questionArraySchema(noExtra *bool) tool.ToolParam {
	return tool.ToolParam{
		Type: "array", Description: "1-3 个必须由用户决定的问题；每题提供 2-4 个互斥选项。", MinItems: intPtr(1), MaxItems: intPtr(3),
		Items: &tool.ToolParam{Type: "object", AdditionalProperties: noExtra, Required: []string{"question", "options"}, Properties: map[string]tool.ToolParam{
			"question":     {Type: "string", Description: "一句可直接回答的问题。"},
			"header":       {Type: "string", Description: "可选短标签。"},
			"multi_select": {Type: "boolean", Description: "是否允许多选，默认 false。"},
			"options": {Type: "array", Description: "2-4 个清晰且互斥的用户选择。", MinItems: intPtr(2), MaxItems: intPtr(4), Items: &tool.ToolParam{
				Type: "object", AdditionalProperties: noExtra, Required: []string{"label"}, Properties: map[string]tool.ToolParam{
					"label": {Type: "string", Description: "简短且互斥的选择标签。"}, "description": {Type: "string", Description: "选择该项的影响或取舍。"},
				},
			}},
		}},
	}
}

func (t *AskUserTool) Definition() tool.ToolDefinition {
	return tool.BuildBuiltinDefinition(t, tool.ToolDefinition{
		Name:        ToolName,
		DisplayName: "Ask User",
		Description: t.Description(),
		Purpose:     "在无法安全推断关键选择时暂停当前工具调用，向用户提出少量结构化问题并等待回答或取消。",
		WhenToUse: []string{
			"缺失信息会实质改变结果、权限、外部副作用或不可逆实现方向时",
			"用户必须在 2-4 个明确选项中做选择后任务才能继续时",
		},
		WhenNotToUse: []string{
			"答案可从当前 workspace、runtime state 或只读检查中获得时先检查事实",
			"可以在用户意图范围内做低风险、可逆的合理默认时不要产生不必要提问",
			"不要把普通进度更新或最终答复包装成工具调用",
		},
		OutputSchema: tool.ToolParam{Type: "object", Properties: map[string]tool.ToolParam{
			"resolution":     {Type: "string", Description: "用户处理问题的结果：answered 或 cancelled"},
			"answer_text":    {Type: "string", Description: "answered 时用户返回的原始回答文本；cancelled 时为空"},
			"question_count": {Type: "integer", Description: "本次向用户展示的问题数量"},
		}},
		SideEffects: []tool.ToolSideEffect{{Kind: tool.SideEffectNone, Description: "暂停当前工具调用等待用户回答或取消，不修改 workspace 或外部系统；问题取消不终止 run。", Reversibility: tool.ReversibilityNotApplicable}},
		Examples:    []tool.ToolExample{{Scenario: "实现方向需要用户选择", Input: map[string]any{"questions": []any{map[string]any{"question": "选择数据迁移策略", "options": []any{map[string]any{"label": "兼容迁移", "description": "保留旧格式并渐进升级"}, map[string]any{"label": "一次切换", "description": "立即要求新格式"}}}}}, Outcome: "runtime 发出用户问题并在 answered 或 cancelled resolution 前暂停当前工具调用"}},
		FailureModes: []tool.ToolFailureMode{
			{Code: "ask_user_invalid_input", Cause: "问题或选项不符合数量和结构约束", Retryable: false, Recovery: []string{"修正问题结构"}},
			{Code: "ask_user_no_awaiter", Cause: "当前 runtime 不支持等待用户回答", Retryable: false, Recovery: []string{"向用户直接说明阻塞信息"}},
			{Code: "ask_user_aborted", Cause: "run 取消、超时或等待通道被中断", Retryable: false, Recovery: []string{"尊重运行中断并停止依赖该选择的操作"}},
		},
		Annotations: tool.ToolAnnotations{
			ReadOnly:        true,
			Destructive:     false,
			CapabilityClass: tool.CapabilityLocalTransform,
			TrustedOrigin:   tool.TrustedOriginBuiltin,
		},
	})
}

func intPtr(value int) *int { return &value }

func (t *AskUserTool) Execute(ctx context.Context, input string) (string, error) {
	return "", fmt.Errorf("ask_user should be handled by the runtime awaiter, not executed directly")
}

// CanonicalizeLegacyArguments converts only historical ask_user carriers to
// the current {"questions": [...]} object. Canonical objects are returned
// unchanged so the registered schema remains authoritative for unknown fields,
// item bounds, and nested types.
func CanonicalizeLegacyArguments(input string) (string, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return input, nil
	}

	var decoded any
	if err := json.Unmarshal([]byte(trimmed), &decoded); err != nil {
		return input, nil
	}
	switch value := decoded.(type) {
	case []any:
		questions, err := ParseQuestions(trimmed)
		if err != nil {
			return "", err
		}
		return marshalCanonicalQuestions(questions)
	case map[string]any:
		_, hasQuestions := value["questions"]
		legacyValue, hasLegacy := value["questions_json"]
		if hasQuestions || !hasLegacy {
			return input, nil
		}
		legacyString, ok := legacyValue.(string)
		if !ok || strings.TrimSpace(legacyString) == "" {
			return input, nil
		}
		questions, err := ParseQuestions(legacyString)
		if err != nil {
			return "", err
		}
		return marshalCanonicalQuestions(questions)
	default:
		return input, nil
	}
}

func marshalCanonicalQuestions(questions []Question) (string, error) {
	data, err := json.Marshal(askUserInput{Questions: questions})
	if err != nil {
		return "", fmt.Errorf("encode canonical questions: %w", err)
	}
	return string(data), nil
}

func ParseQuestions(input string) ([]Question, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, fmt.Errorf("questions is required")
	}

	var questions []Question
	var legacy struct {
		QuestionsJSON string `json:"questions_json"`
	}
	if err := json.Unmarshal([]byte(input), &legacy); err == nil && strings.TrimSpace(legacy.QuestionsJSON) != "" {
		return ParseQuestions(legacy.QuestionsJSON)
	}

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

func FormatCancelledResult() string {
	return "User cancelled this question without answering. The run is still active. Continue without this clarification when possible; otherwise explain what cannot be completed, and do not infer or fabricate an answer."
}

func init() {
	tool.MustRegisterToolWithMode(&AskUserTool{}, tool.DefinitionValidationStrictBuiltin)
}
