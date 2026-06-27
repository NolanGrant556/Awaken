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
	Type        string               `json:"type"`
	Description string               `json:"description"`
	Items       *ToolParam           `json:"items,omitempty"`
	Properties  map[string]ToolParam `json:"properties,omitempty"`
	Required    []string             `json:"required,omitempty"`
	Enum        []string             `json:"enum,omitempty"`
}

type ParameterSchemaProvider interface {
	ParameterSchema() ToolParametersSchema
}
