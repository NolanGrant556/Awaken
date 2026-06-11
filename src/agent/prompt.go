package agent

import (
	"fmt"
	"strings"

	"awaken/src/tool"
)

const reactPromptTemplate = `你是一个智能助手，能够通过思考和行动来解决问题。

## 可用工具

%s

## 格式要求

你需要按照以下格式进行响应：

### 思考阶段
<thought>你的思考过程，分析当前情况和下一步计划</thought>

### 行动阶段
<action>
<name>工具名称或"总结"</name>
<input>工具输入参数或最终回答</input>
</action>

## 规则说明

1. 如果问题可以直接回答，不需要调用工具，则使用"总结"作为工具名称
2. 如果需要调用工具来获取信息，则使用相应的工具名称
3. 工具调用完成后，根据返回结果继续思考或总结
4. 你的回答必须严格按照指定格式输出

## 对话历史

%s

## 当前任务
%s

开始思考和行动：`

const finalAnswerTemplate = `## 总结

根据我的分析和工具执行结果，以下是对你问题的回答：

%s`

func BuildReActPrompt(state AgentState, userInput string) string {
	toolsDesc := tool.GetToolsDescription()
	
	var history strings.Builder
	for _, msg := range state.Messages {
		history.WriteString(fmt.Sprintf("%s: %s\n", msg.Role, msg.Content))
	}
	
	for _, thought := range state.Thoughts {
		history.WriteString(fmt.Sprintf("思考 %d: %s\n", thought.Index, thought.Thought))
		if thought.Action != "" {
			history.WriteString(fmt.Sprintf("行动: %s\n", thought.Action))
			history.WriteString(fmt.Sprintf("输入: %s\n", thought.ActionInput))
			if thought.Observation != "" {
				history.WriteString(fmt.Sprintf("观察: %s\n", thought.Observation))
			}
		}
	}
	
	return fmt.Sprintf(reactPromptTemplate, toolsDesc, history.String(), userInput)
}

func BuildFinalAnswer(response string) string {
	return fmt.Sprintf(finalAnswerTemplate, response)
}

type ParsedAction struct {
	ActionType string
	ToolName   string
	Input      string
}

func ParseAction(response string) (ParsedAction, error) {
	result := ParsedAction{}
	
	thoughtStart := strings.Index(response, "<thought>")
	thoughtEnd := strings.Index(response, "</thought>")
	if thoughtStart != -1 && thoughtEnd != -1 {
		result.ActionType = "thought"
	}
	
	actionStart := strings.Index(response, "<action>")
	actionEnd := strings.Index(response, "</action>")
	if actionStart == -1 || actionEnd == -1 {
		return result, fmt.Errorf("invalid action format")
	}
	
	actionContent := response[actionStart+8 : actionEnd]
	
	nameStart := strings.Index(actionContent, "<name>")
	nameEnd := strings.Index(actionContent, "</name>")
	if nameStart == -1 || nameEnd == -1 {
		return result, fmt.Errorf("missing action name")
	}
	result.ToolName = strings.TrimSpace(actionContent[nameStart+6 : nameEnd])
	
	inputStart := strings.Index(actionContent, "<input>")
	inputEnd := strings.Index(actionContent, "</input>")
	if inputStart == -1 || inputEnd == -1 {
		return result, fmt.Errorf("missing action input")
	}
	result.Input = strings.TrimSpace(actionContent[inputStart+7 : inputEnd])
	
	result.ActionType = "action"
	return result, nil
}