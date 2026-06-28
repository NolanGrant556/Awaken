package agentcore

import (
	"path/filepath"
	"strings"
	"testing"

	convcontext "awaken/src/context"
	"awaken/src/llm"
	"awaken/src/mcp"
	"awaken/src/tool"
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
	if req.Cache.SystemPromptCacheControl.Type != "" {
		t.Fatalf("expected cache metadata to be empty by default, got %+v", req.Cache)
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

func TestRequestBuilderIncludesExposedMCPTools(t *testing.T) {
	registry, err := mcp.NewStaticRegistry([]mcp.Server{{
		Config: mcp.ServerConfig{
			ID:        "docs",
			Name:      "Docs",
			Transport: mcp.TransportHTTP,
			Enabled:   true,
			URL:       "https://example.invalid/mcp",
		},
		State: mcp.ServerRuntime{
			Mode:   mcp.RuntimeModeStatic,
			Status: mcp.ServerStatusReady,
		},
		Tools: []mcp.ToolDescriptor{{
			Name:          "search",
			Description:   "Search docs",
			ExposeToModel: true,
			InputSchema: tool.ToolParametersSchema{
				Type: "object",
				Properties: map[string]tool.ToolParam{
					"query": {Type: "string", Description: "Search query"},
				},
				Required: []string{"query"},
			},
		}},
	}})
	if err != nil {
		t.Fatalf("failed to build MCP registry: %v", err)
	}

	req := NewRequestBuilder(AgentConfig{MCPRegistry: registry}).Build(convcontext.New(), nil)
	for _, definition := range req.Tools {
		if definition.Function.Name == "mcp__docs__search" {
			if !strings.Contains(req.SystemPrompt, "mcp__docs__search") {
				t.Fatalf("expected system prompt summary to mention MCP tool, got %s", req.SystemPrompt)
			}
			return
		}
	}

	t.Fatalf("expected MCP tool definition in %+v", req.Tools)
}

func TestRequestBuilderAddsSystemPromptCacheMetadataWhenEnabled(t *testing.T) {
	builder := NewRequestBuilder(AgentConfig{
		SystemPrompt: "stable system prompt",
		PromptCache: PromptCacheConfig{
			SystemPrompt: true,
		},
	})

	req := builder.Build(convcontext.New(), []tool.ToolDefinition{})
	if req.Cache.SystemPromptCacheControl.Type != llm.CacheControlTypeEphemeral {
		t.Fatalf("expected ephemeral system prompt cache metadata, got %+v", req.Cache)
	}
}

func TestRequestBuilderHonorsSystemPromptCacheMinimumSize(t *testing.T) {
	builder := NewRequestBuilder(AgentConfig{
		SystemPrompt: "short",
		PromptCache: PromptCacheConfig{
			SystemPrompt:         true,
			MinSystemPromptBytes: 10000,
		},
	})

	req := builder.Build(convcontext.New(), []tool.ToolDefinition{})
	if req.Cache.SystemPromptCacheControl.Type != "" {
		t.Fatalf("expected cache metadata to stay empty below threshold, got %+v", req.Cache)
	}
}
