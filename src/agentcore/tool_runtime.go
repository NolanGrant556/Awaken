package agentcore

import (
	stdcontext "context"
	"encoding/json"
	"fmt"
	"strings"

	convcontext "awaken/src/context"
	"awaken/src/internal/outputlimit"
	"awaken/src/tool"
)

type ToolRuntime struct{}

type ToolExecutionResult struct {
	ToolCallID string
	Name       string
	Content    string
	IsError    bool
	Truncated  bool
}

func NewToolRuntime() *ToolRuntime {
	return &ToolRuntime{}
}

func (rt *ToolRuntime) Execute(ctx stdcontext.Context, call convcontext.ToolCall) ToolExecutionResult {
	id := call.ID
	if id == "" {
		id = convcontext.GenerateToolCallID()
	}

	result := ToolExecutionResult{
		ToolCallID: id,
		Name:       call.Function.Name,
	}

	selectedTool, err := tool.GetTool(call.Function.Name)
	if err != nil {
		result.Content = fmt.Sprintf("工具执行失败: %v: %s", err, call.Function.Name)
		result.IsError = true
		return finalizeToolResult(result)
	}

	input, err := normalizeToolInput(call.Function.Arguments, selectedTool.Parameters())
	if err != nil {
		result.Content = fmt.Sprintf("工具参数错误: %v", err)
		result.IsError = true
		return finalizeToolResult(result)
	}

	output, err := selectedTool.Execute(ctx, input)
	if err != nil {
		result.Content = fmt.Sprintf("工具执行失败: %v", err)
		result.IsError = true
		return finalizeToolResult(result)
	}

	result.Content = output
	return finalizeToolResult(result)
}

func (rt *ToolRuntime) ExecuteToolCalls(ctx stdcontext.Context, calls []convcontext.ToolCall) []ToolExecutionResult {
	results := make([]ToolExecutionResult, 0, len(calls))
	for _, call := range calls {
		results = append(results, rt.Execute(ctx, call))
	}
	return results
}

func ExecuteToolCalls(ctx stdcontext.Context, calls []convcontext.ToolCall) []ToolExecutionResult {
	return NewToolRuntime().ExecuteToolCalls(ctx, calls)
}

func finalizeToolResult(result ToolExecutionResult) ToolExecutionResult {
	limited := outputlimit.Limit(result.Content, outputlimit.DefaultToolResultMaxChars)
	result.Content = limited.Content
	result.Truncated = limited.Truncated
	return result
}

func normalizeToolInput(arguments string, params []tool.ToolParameter) (string, error) {
	trimmed := strings.TrimSpace(arguments)
	if len(params) == 0 {
		if trimmed == "" {
			return "", nil
		}
		if !json.Valid([]byte(trimmed)) {
			return "", fmt.Errorf("arguments must be valid JSON")
		}
		return trimmed, nil
	}
	if trimmed == "" {
		trimmed = "{}"
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal([]byte(trimmed), &decoded); err != nil {
		return "", fmt.Errorf("arguments must be a JSON object: %w", err)
	}
	if decoded == nil {
		return "", fmt.Errorf("arguments must be a JSON object")
	}

	for _, param := range params {
		if param.Required {
			if _, ok := decoded[param.Name]; !ok {
				return "", fmt.Errorf("missing required parameter %q", param.Name)
			}
		}
	}

	if len(params) == 1 {
		value, ok := decoded[params[0].Name]
		if !ok {
			return "", nil
		}
		if str, ok := value.(string); ok {
			return str, nil
		}
		data, err := json.Marshal(value)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}

	data, err := json.Marshal(decoded)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func argumentsForXMLFallback(toolName, input string) string {
	selectedTool, err := tool.GetTool(toolName)
	if err != nil {
		return input
	}

	params := selectedTool.Parameters()
	paramName := singleRequiredStringParam(params)
	if paramName == "" {
		return input
	}

	data, err := json.Marshal(map[string]interface{}{
		paramName: input,
	})
	if err != nil {
		return input
	}
	return string(data)
}

func singleRequiredStringParam(params []tool.ToolParameter) string {
	requiredName := ""
	for _, param := range params {
		if !param.Required {
			continue
		}
		if param.Type != "string" {
			return ""
		}
		if requiredName != "" {
			return ""
		}
		requiredName = param.Name
	}
	return requiredName
}
