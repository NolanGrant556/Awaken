package agent

import (
	"context"
	"fmt"
	"strings"

	"awaken/src/llm"
	"awaken/src/tool"
)

type Agent struct {
	config AgentConfig
	state  AgentState
}

func NewAgent(config AgentConfig) *Agent {
	return &Agent{
		config: config,
		state: AgentState{
			Messages: []Message{},
			Thoughts: []Thought{},
		},
	}
}

func (a *Agent) Reset() {
	a.state = AgentState{
		Messages: []Message{},
		Thoughts: []Thought{},
	}
}

func (a *Agent) GetState() AgentState {
	return a.state
}

func (a *Agent) Run(ctx context.Context, userInput string) (string, error) {
	a.state.Messages = append(a.state.Messages, Message{
		Role:    "user",
		Content: userInput,
	})

	for i := 0; i < a.config.MaxThoughts; i++ {
		prompt := BuildReActPrompt(a.state, userInput)
		
		llmReq := llm.Request{
			SystemPrompt: a.config.SystemPrompt,
			UserPrompt:   prompt,
		}
		
		llmCfg := llm.Config{
			APIKey:  a.config.ModelConfig.APIKey,
			BaseURL: a.config.ModelConfig.BaseURL,
			Model:   a.config.ModelConfig.Model,
		}
		
		response, err := llm.Call(ctx, llmCfg, llmReq)
		if err != nil {
			return "", fmt.Errorf("LLM call failed: %w", err)
		}
		
		parsed, err := ParseAction(response)
		if err != nil {
			return "", fmt.Errorf("failed to parse action: %w", err)
		}
		
		thoughtIndex := len(a.state.Thoughts) + 1
		
		if strings.EqualFold(parsed.ToolName, "总结") {
			a.state.Thoughts = append(a.state.Thoughts, Thought{
				Index:       thoughtIndex,
				Thought:     "任务已完成，准备总结回答",
				Action:      "总结",
				ActionInput: parsed.Input,
			})
			
			a.state.Messages = append(a.state.Messages, Message{
				Role:    "assistant",
				Content: BuildFinalAnswer(parsed.Input),
			})
			
			return parsed.Input, nil
		}
		
		observation, err := tool.ExecuteTool(ctx, parsed.ToolName, parsed.Input)
		if err != nil {
			observation = fmt.Sprintf("工具执行失败: %v", err)
		}
		
		a.state.Thoughts = append(a.state.Thoughts, Thought{
			Index:       thoughtIndex,
			Thought:     extractThought(response),
			Action:      parsed.ToolName,
			ActionInput: parsed.Input,
			Observation: observation,
		})
	}
	
	return "", fmt.Errorf("达到最大思考次数 %d，未能完成任务", a.config.MaxThoughts)
}

func extractThought(response string) string {
	thoughtStart := strings.Index(response, "<thought>")
	thoughtEnd := strings.Index(response, "</thought>")
	if thoughtStart != -1 && thoughtEnd != -1 {
		return strings.TrimSpace(response[thoughtStart+9 : thoughtEnd])
	}
	return "分析当前情况并决定下一步行动"
}