package tool

import "sort"

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

func DefinitionFromTool(candidate Tool) ToolDefinition {
	return ToolDefinition{
		Type: "function",
		Function: ToolFunctionSchema{
			Name:        candidate.Name(),
			Description: candidate.Description(),
			Parameters:  ParametersSchemaFromTool(candidate),
		},
	}
}

func DefinitionsFromTools(candidates []Tool) []ToolDefinition {
	if len(candidates) == 0 {
		return nil
	}

	definitions := make([]ToolDefinition, 0, len(candidates))
	for _, candidate := range candidates {
		definitions = append(definitions, DefinitionFromTool(candidate))
	}

	sort.Slice(definitions, func(i, j int) bool {
		return definitions[i].Function.Name < definitions[j].Function.Name
	})
	return definitions
}

func ParametersSchemaFromTool(candidate Tool) ToolParametersSchema {
	if provider, ok := candidate.(ParameterSchemaProvider); ok {
		return provider.ParameterSchema()
	}

	params := candidate.Parameters()
	properties := make(map[string]ToolParam, len(params))
	required := make([]string, 0)

	for _, param := range params {
		properties[param.Name] = ToolParam{
			Type:        param.Type,
			Description: param.Description,
		}
		if param.Required {
			required = append(required, param.Name)
		}
	}

	return ToolParametersSchema{
		Type:       "object",
		Properties: properties,
		Required:   required,
	}
}
