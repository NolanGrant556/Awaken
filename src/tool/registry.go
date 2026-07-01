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

// mcpSafeNamePattern extends the builtin safe-name pattern to allow the
// double-underscore separator used by MCP-sourced tools
// (mcp__{server}__{tool}) and the skill__ prefix used by skill tools.
var mcpSafeNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_]+(?:__[a-zA-Z0-9_]+)*$`)

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

// RegisterToolForce registers a tool, replacing any existing registration
// with the same name. Used by the MCP manager when refreshing a server's
// tool list and by tests that re-create tool instances.
func RegisterToolForce(t Tool) error {
	if err := ValidateTool(t); err != nil {
		return err
	}
	mu.Lock()
	defer mu.Unlock()
	toolRegistry[t.Name()] = t
	return nil
}

func MustRegisterTool(tool Tool) {
	if err := RegisterTool(tool); err != nil {
		panic(err)
	}
}

// UnregisterTool removes a tool from the registry by name. It is a no-op
// when the tool does not exist. Used by the MCP manager when a server
// disconnects or is disabled.
func UnregisterTool(name string) {
	mu.Lock()
	defer mu.Unlock()
	delete(toolRegistry, name)
}

// UnregisterToolsByPrefix removes every tool whose name starts with the
// given prefix. Returns the count of removed tools. Used by the MCP
// manager to clear all tools from a single server in one call.
func UnregisterToolsByPrefix(prefix string) int {
	mu.Lock()
	defer mu.Unlock()
	removed := 0
	for name := range toolRegistry {
		if startsWith(name, prefix) {
			delete(toolRegistry, name)
			removed++
		}
	}
	return removed
}

// ListToolNames returns the names of all currently registered tools, sorted.
// Used by tests and by the /v1/tools endpoint.
func ListToolNames() []string {
	mu.RLock()
	defer mu.RUnlock()
	names := make([]string, 0, len(toolRegistry))
	for name := range toolRegistry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func startsWith(name, prefix string) bool {
	if len(prefix) > len(name) {
		return false
	}
	return name[:len(prefix)] == prefix
}

func ValidateTool(tool Tool) error {
	if tool == nil {
		return fmt.Errorf("%w: nil tool", ErrInvalidTool)
	}
	if !isValidToolName(tool.Name()) {
		return fmt.Errorf("%w: tool name %q must match %s", ErrInvalidTool, tool.Name(), mcpSafeNamePattern.String())
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

// isValidToolName accepts both builtin-style names and the namespaced
// mcp__{server}__{tool} / skill__{id} forms.
func isValidToolName(name string) bool {
	if name == "" {
		return false
	}
	return mcpSafeNamePattern.MatchString(name)
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
