package tool

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

var (
	toolRegistry = make(map[string]Tool)
	mu           sync.RWMutex
	ErrToolNotFound = errors.New("tool not found")
)

func RegisterTool(tool Tool) {
	mu.Lock()
	defer mu.Unlock()
	toolRegistry[tool.Name()] = tool
}

func GetTool(name string) (Tool, error) {
	mu.RLock()
	defer mu.RUnlock()
	tool, exists := toolRegistry[name]
	if !exists {
		return nil, ErrToolNotFound
	}
	return tool, nil
}

func ListTools() []Tool {
	mu.RLock()
	defer mu.RUnlock()
	tools := make([]Tool, 0, len(toolRegistry))
	for _, tool := range toolRegistry {
		tools = append(tools, tool)
	}
	return tools
}

func GetToolsDescription() string {
	tools := ListTools()
	if len(tools) == 0 {
		return "No tools available."
	}
	
	var desc string
	for _, tool := range tools {
		desc += fmt.Sprintf("- %s: %s\n", tool.Name(), tool.Description())
		for _, param := range tool.Parameters() {
			required := ""
			if param.Required {
				required = " (required)"
			}
			desc += fmt.Sprintf("  - %s (%s)%s: %s\n", param.Name, param.Type, required, param.Description)
		}
	}
	return desc
}

func ExecuteTool(ctx context.Context, toolName string, input string) (string, error) {
	tool, err := GetTool(toolName)
	if err != nil {
		return "", err
	}
	return tool.Execute(ctx, input)
}