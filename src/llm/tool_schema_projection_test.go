package llm

import (
	"encoding/json"
	"reflect"
	"testing"

	"awaken/src/tool"
)

func TestProjectOpenAICompatibleToolParametersRemovesRootUnionOnly(t *testing.T) {
	additional := false
	schema := tool.ToolParametersSchema{
		Type:        "object",
		Description: "exactly one input",
		Properties: map[string]tool.ToolParam{
			"path": {Type: "string"},
			"value": {
				OneOf: []tool.ToolParam{{Type: "string"}, {Type: "number"}},
			},
		},
		AdditionalProperties: &additional,
		OneOf: []tool.ToolParam{
			{Type: "object", Properties: map[string]tool.ToolParam{"path": {Type: "string"}}, Required: []string{"path"}},
			{Type: "object", Properties: map[string]tool.ToolParam{"paths": {Type: "array", Items: &tool.ToolParam{Type: "string"}}}, Required: []string{"paths"}},
		},
	}
	original := tool.SchemaAsMap(schema)

	projected := projectOpenAICompatibleToolParameters(schema)

	if projected["type"] != "object" {
		t.Fatalf("expected object root, got %+v", projected)
	}
	if _, exists := projected["oneOf"]; exists {
		t.Fatalf("expected root oneOf to be removed, got %+v", projected)
	}
	properties, ok := projected["properties"].(map[string]any)
	if !ok || properties["path"] == nil || properties["paths"] == nil {
		t.Fatalf("expected root and branch properties to be retained, got %+v", projected)
	}
	value, ok := properties["value"].(map[string]any)
	if !ok || value["oneOf"] == nil {
		t.Fatalf("expected nested oneOf to remain canonical on the wire, got %+v", properties["value"])
	}
	if !reflect.DeepEqual(tool.SchemaAsMap(schema), original) {
		t.Fatalf("projection mutated canonical schema: before=%+v after=%+v", original, tool.SchemaAsMap(schema))
	}
}

func TestOpenAIAdaptersUseCompatibleToolParameterProjection(t *testing.T) {
	schema := tool.ToolSchema{
		Type: "function",
		Function: tool.ToolFunctionSchema{
			Name: "trash_file",
			Parameters: tool.ToolParametersSchema{
				Type:       "object",
				Properties: map[string]tool.ToolParam{"path": {Type: "string"}},
				OneOf:      []tool.ToolParam{{Type: "object", Required: []string{"path"}}},
			},
		},
	}

	chatTools := convertToolsToOpenAI([]tool.ToolSchema{schema})
	function := chatTools[0]["function"].(map[string]interface{})
	chatParameters := function["parameters"].(map[string]any)
	if _, exists := chatParameters["oneOf"]; exists {
		t.Fatalf("chat adapter leaked root oneOf: %+v", chatParameters)
	}

	responsesTools := convertToolsToOpenAIResponses([]tool.ToolSchema{schema})
	if _, exists := responsesTools[0].Parameters["oneOf"]; exists {
		t.Fatalf("responses adapter leaked root oneOf: %+v", responsesTools[0].Parameters)
	}
}

func TestOpenAIRequestsOmitUnspecifiedTemperature(t *testing.T) {
	tests := []struct {
		name    string
		prepare func() (PreparedRequest, error)
	}{
		{
			name: "chat",
			prepare: func() (PreparedRequest, error) {
				return prepareOpenAIRequest(Config{BaseURL: "https://example.test/v1", Model: "custom-model", Protocol: ProtocolOpenAIChat}, Request{UserPrompt: "hi"}, false)
			},
		},
		{
			name: "responses",
			prepare: func() (PreparedRequest, error) {
				return prepareOpenAIResponsesRequest(Config{BaseURL: "https://example.test/v1", Model: "custom-model", Protocol: ProtocolOpenAIResponses}, Request{UserPrompt: "hi"}, false)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			prepared, err := test.prepare()
			if err != nil {
				t.Fatalf("prepare request: %v", err)
			}
			var body map[string]any
			if err := json.Unmarshal(prepared.Body, &body); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			if value, exists := body["temperature"]; exists {
				t.Fatalf("expected unspecified temperature to be omitted, got %v in %+v", value, body)
			}
		})
	}
}
