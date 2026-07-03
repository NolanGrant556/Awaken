package agentcore

import (
	"path/filepath"
	"strings"
	"testing"

	convcontext "awaken/src/context"
	"awaken/src/tool"
)

func TestRequestBuilderBuildsCompleteRequest(t *testing.T) {
	ctx := convcontext.New()
	ctx.AddUserMessage("hello")

	tools := []tool.ToolSchema{
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
			Model:         "test-model",
			Temperature:   0.2,
			MaxTokens:     777,
			ContextWindow: 400000,
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
	if req.ContextWindowState.EstimatedTokens == 0 {
		t.Fatalf("expected context window estimate on request")
	}
	if req.ContextWindowState.ModelWindow != 400000 {
		t.Fatalf("expected configured context window, got %+v", req.ContextWindowState)
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

func TestRequestBuilderBuildsContextEnvelope(t *testing.T) {
	ctx := convcontext.New()
	ctx.AddUserMessage("hello")
	workspaceRoot := filepath.Join(t.TempDir(), "workspace")
	writableRoot := filepath.Join(workspaceRoot, "safe")
	tools := []tool.ToolSchema{
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
		SystemPrompt: "static {{.RuntimeInfo}}",
		EntryPoint:   "test-entry",
		ModelConfig: ModelConfig{
			Model:       "test-model",
			Temperature: 0.2,
			MaxTokens:   777,
		},
		Workspace: WorkspaceConfig{
			Root:          workspaceRoot,
			WritableRoots: []string{writableRoot},
		},
	})

	envelope := builder.BuildContextEnvelope(ctx, tools)
	if len(envelope.SystemBlocks) != 1 || envelope.SystemBlocks[0].Content != "static {{.RuntimeInfo}}" {
		t.Fatalf("unexpected system blocks: %+v", envelope.SystemBlocks)
	}
	if len(envelope.SystemContextItems) != 1 || envelope.SystemContextItems[0].Type != "runtime_snapshot" {
		t.Fatalf("expected runtime snapshot system context item, got %+v", envelope.SystemContextItems)
	}
	if len(envelope.Messages) != 1 || envelope.Messages[0].Content != "hello" {
		t.Fatalf("unexpected envelope messages: %+v", envelope.Messages)
	}
	if len(envelope.ToolSchemas) != 1 || envelope.ToolSchemas[0].Function.Name != "explicit_tool" {
		t.Fatalf("unexpected tool schemas: %+v", envelope.ToolSchemas)
	}
	if envelope.RuntimeSnapshot.ModelName != "test-model" || envelope.RuntimeSnapshot.EntryPoint != "test-entry" {
		t.Fatalf("unexpected runtime snapshot: %+v", envelope.RuntimeSnapshot)
	}
	if envelope.RuntimeSnapshot.WorkspaceRoot != workspaceRoot {
		t.Fatalf("expected workspace root %q, got %q", workspaceRoot, envelope.RuntimeSnapshot.WorkspaceRoot)
	}
	if len(envelope.RuntimeSnapshot.WritableRoots) != 1 || envelope.RuntimeSnapshot.WritableRoots[0] != writableRoot {
		t.Fatalf("unexpected writable roots: %+v", envelope.RuntimeSnapshot.WritableRoots)
	}
	if !contextEnvelopeHasDelta(envelope, ContextDeltaRuntimeSnapshot) || !contextEnvelopeHasDelta(envelope, ContextDeltaToolPool) {
		t.Fatalf("expected runtime snapshot and tool pool deltas, got %+v", envelope.Deltas)
	}
	if len(envelope.CacheKeys) == 0 {
		t.Fatalf("expected cache keys")
	}
	if envelope.ContextWindowState.EstimatedTokens == 0 {
		t.Fatalf("expected context window estimate")
	}
}

func TestRequestBuilderBuildsLLMRequestFromEnvelope(t *testing.T) {
	ctx := convcontext.New()
	ctx.AddUserMessage("hello")
	tools := []tool.ToolSchema{
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
		SystemPrompt: "base {{.RuntimeInfo}}",
		EntryPoint:   "test-entry",
		ModelConfig: ModelConfig{
			Model:       "test-model",
			Temperature: 0.2,
			MaxTokens:   777,
		},
	})

	envelope := builder.BuildContextEnvelope(ctx, tools)
	req := builder.BuildLLMRequestFromEnvelope(envelope)
	if !strings.Contains(req.SystemPrompt, "base") ||
		!strings.Contains(req.SystemPrompt, "test-entry") ||
		!strings.Contains(req.SystemPrompt, "test-model") ||
		!strings.Contains(req.SystemPrompt, "explicit_tool") {
		t.Fatalf("expected request system prompt to include static prompt, runtime, and tool summary, got %s", req.SystemPrompt)
	}
	if len(req.Messages) != 1 || req.Messages[0].Content != "hello" {
		t.Fatalf("unexpected request messages: %+v", req.Messages)
	}
	if len(req.Tools) != 1 || req.Tools[0].Function.Name != "explicit_tool" {
		t.Fatalf("unexpected request tools: %+v", req.Tools)
	}
	if req.Model != "test-model" || req.Temperature != 0.2 || req.MaxTokens != 777 {
		t.Fatalf("unexpected model config: %+v", req)
	}
}

func contextEnvelopeHasDelta(envelope ContextEnvelope, deltaType ContextDeltaType) bool {
	for _, delta := range envelope.Deltas {
		if delta.Type == deltaType {
			return true
		}
	}
	return false
}
