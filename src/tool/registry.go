package tool

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"sync"
)

var (
	toolRegistry     = make(map[string]Tool)
	mu               sync.RWMutex
	ErrToolNotFound  = errors.New("tool not found")
	ErrDuplicateTool = errors.New("tool already registered")
	ErrInvalidTool   = errors.New("invalid tool")
)

var safeNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

func RegisterTool(tool Tool) error {
	if err := ValidateTool(tool); err != nil {
		return err
	}

	mu.Lock()
	defer mu.Unlock()
	if _, exists := toolRegistry[tool.Name()]; exists {
		return fmt.Errorf("%w: %s", ErrDuplicateTool, tool.Name())
	}
	toolRegistry[tool.Name()] = tool
	return nil
}

func MustRegisterTool(tool Tool) {
	if err := RegisterTool(tool); err != nil {
		panic(err)
	}
}

func ValidateTool(tool Tool) error {
	if tool == nil {
		return fmt.Errorf("%w: nil tool", ErrInvalidTool)
	}
	if !safeNamePattern.MatchString(tool.Name()) {
		return fmt.Errorf("%w: tool name %q must match %s", ErrInvalidTool, tool.Name(), safeNamePattern.String())
	}
	for _, param := range tool.Parameters() {
		if !safeNamePattern.MatchString(param.Name) {
			return fmt.Errorf("%w: parameter name %q must match %s", ErrInvalidTool, param.Name, safeNamePattern.String())
		}
		if !isJSONSchemaType(param.Type) {
			return fmt.Errorf("%w: parameter %q has invalid JSON Schema type %q", ErrInvalidTool, param.Name, param.Type)
		}
	}
	return nil
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
	sort.Slice(tools, func(i, j int) bool {
		return tools[i].Name() < tools[j].Name()
	})
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

func isJSONSchemaType(value string) bool {
	switch value {
	case "string", "number", "integer", "boolean", "object", "array":
		return true
	default:
		return false
	}
}
