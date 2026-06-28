package toolorder

import "sort"

type Source string

const (
	SourceBuiltin Source = "builtin"
	SourceMCP     Source = "mcp"
	SourcePlugin  Source = "plugin"
	SourceSession Source = "session"
)

type Tool struct {
	Name   string
	Source Source
}

type Pool struct {
	Builtins  []Tool
	Externals []Tool
}

type OrderResult struct {
	Tools                []Tool
	DroppedDuplicates    []Tool
	CacheBreakpointIndex int
}

func Assemble(builtins, externals []Tool) OrderResult {
	return Order(Pool{
		Builtins:  builtins,
		Externals: externals,
	})
}

func Order(pool Pool) OrderResult {
	builtins := copyTools(pool.Builtins)
	externals := copyTools(pool.Externals)

	sortByName(builtins)
	sortByName(externals)

	tools := make([]Tool, 0, len(builtins)+len(externals))
	dropped := make([]Tool, 0)
	seen := make(map[string]struct{}, len(builtins)+len(externals))

	for _, tool := range builtins {
		if _, ok := seen[tool.Name]; ok {
			dropped = append(dropped, tool)
			continue
		}
		seen[tool.Name] = struct{}{}
		tools = append(tools, tool)
	}

	cacheBreakpointIndex := len(tools)

	for _, tool := range externals {
		if _, ok := seen[tool.Name]; ok {
			dropped = append(dropped, tool)
			continue
		}
		seen[tool.Name] = struct{}{}
		tools = append(tools, tool)
	}

	return OrderResult{
		Tools:                tools,
		DroppedDuplicates:    dropped,
		CacheBreakpointIndex: cacheBreakpointIndex,
	}
}

func copyTools(tools []Tool) []Tool {
	copied := make([]Tool, len(tools))
	copy(copied, tools)
	return copied
}

func sortByName(tools []Tool) {
	sort.SliceStable(tools, func(i, j int) bool {
		return tools[i].Name < tools[j].Name
	})
}
