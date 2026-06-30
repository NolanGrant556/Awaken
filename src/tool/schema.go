package tool

type ToolDefinition struct {
	Type     string             `json:"type"`
	Function ToolFunctionSchema `json:"function"`
}

type ToolFunctionSchema struct {
	Name        string               `json:"name"`
	Description string               `json:"description"`
	Parameters  ToolParametersSchema `json:"parameters"`
}

type ToolParametersSchema struct {
	Type       string               `json:"type"`
	Properties map[string]ToolParam `json:"properties"`
	Required   []string             `json:"required"`
}

type ToolParam struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}
