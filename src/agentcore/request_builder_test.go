package agentcore

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	convcontext "awaken/src/context"
	"awaken/src/tool"
	_ "awaken/src/tools/todo_write"
)

func TestRequestBuilderBuildsCompleteRequest(t *testing.T) {
	ctx := convcontext.New()
	ctx.AddUserMessage("hello")

	tools := []tool.ToolDefinition{
		{
			Type: "function",
			Function: tool.ToolFunctionSchema{
				Name:        "explicit_tool",
				Description: "explicit tool",
				Parameters: tool.ToolParametersSchema{
					Type:       "object",
					Properties: map[string]tool.ToolParam{},
				},
			},
		},
	}

	builder := NewRequestBuilder(AgentConfig{
		EntryPoint: "test-entry",
		ModelConfig: ModelConfig{
			Model:       "test-model",
			Temperature: 0.2,
			MaxTokens:   777,
		},
	})

	req := builder.Build(ctx, tools)
	if req.SystemPrompt == "" {
		t.Fatal("expected system prompt")
	}
	if len(req.Messages) != 1 || req.Messages[0].Content != "hello" {
		t.Fatalf("unexpected messages: %+v", req.Messages)
	}
	if len(req.Tools) != 1 || req.Tools[0].Function.Name != "explicit_tool" {
		t.Fatalf("unexpected tools: %+v", req.Tools)
	}
	if req.Model != "test-model" || req.Temperature != 0.2 || req.MaxTokens != 777 {
		t.Fatalf("unexpected model config: %+v", req)
	}
	if !strings.Contains(req.SystemPrompt, "test-model") || !strings.Contains(req.SystemPrompt, "test-entry") {
		t.Fatalf("expected runtime info in system prompt: %s", req.SystemPrompt)
	}
}

func TestRequestBuilderUsesRegisteredTools(t *testing.T) {
	toolName := "request_builder_registered_tool"
	if err := tool.RegisterTool(runnerTestTool{name: toolName}); err != nil {
		t.Fatalf("failed to register test tool %s: %v", toolName, err)
	}

	builder := NewRequestBuilder(AgentConfig{})
	req := builder.Build(convcontext.New(), nil)

	for _, definition := range req.Tools {
		if definition.Function.Name == toolName {
			if !strings.Contains(req.SystemPrompt, toolName) {
				t.Fatalf("expected system prompt tool summary to mention %s", toolName)
			}
			return
		}
	}
	t.Fatalf("expected registered tool %s in request tools", toolName)
}

func TestRequestBuilderUsesRegisteredToolParameterSchema(t *testing.T) {
	candidate, err := tool.GetTool("todo_write")
	if err != nil {
		t.Fatalf("expected todo_write to be registered: %v", err)
	}

	schema := NewRequestBuilder(AgentConfig{}).buildToolParameters(candidate)
	items, ok := schema.Properties["items"]
	if !ok {
		t.Fatalf("expected todo_write schema to expose items property: %+v", schema)
	}
	if items.Type != "array" || items.Items == nil {
		t.Fatalf("expected items to be an array with item schema, got %+v", items)
	}
	status := items.Items.Properties["status"]
	wantStatuses := []string{"pending", "in_progress", "completed", "cancelled"}
	if !reflect.DeepEqual(status.Enum, wantStatuses) {
		t.Fatalf("expected status enum %+v, got %+v", wantStatuses, status.Enum)
	}
}

func TestRequestBuilderRefreshesRuntimeInfo(t *testing.T) {
	builder := NewRequestBuilder(AgentConfig{
		EntryPoint:  "first",
		ModelConfig: ModelConfig{Model: "first-model"},
	})

	first := builder.Build(convcontext.New(), nil)
	builder.config.EntryPoint = "second"
	builder.config.ModelConfig.Model = "second-model"
	second := builder.Build(convcontext.New(), nil)

	if first.SystemPrompt == second.SystemPrompt {
		t.Fatal("expected runtime prompt to change after config update")
	}
	if !strings.Contains(second.SystemPrompt, "second") || !strings.Contains(second.SystemPrompt, "second-model") {
		t.Fatalf("expected updated runtime info, got %s", second.SystemPrompt)
	}
}

func TestRequestBuilderUsesConfiguredWorkspaceRoot(t *testing.T) {
	workspaceRoot := filepath.Join(t.TempDir(), "workspace")
	builder := NewRequestBuilder(AgentConfig{
		Workspace: WorkspaceConfig{Root: workspaceRoot},
	})

	req := builder.Build(convcontext.New(), nil)
	if !strings.Contains(req.SystemPrompt, workspaceRoot) {
		t.Fatalf("expected system prompt to include workspace root %q, got %s", workspaceRoot, req.SystemPrompt)
	}
}
