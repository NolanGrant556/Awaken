package pluginruntime

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRegisterPluginListsCapabilities(t *testing.T) {
	registry := NewRegistry()
	loaded, err := registry.RegisterPlugin(PluginManifest{
		Name:        "demo",
		Description: "demo plugin",
		Source:      SourceBundled,
	}, func(ctx *RegistrationContext) error {
		if err := ctx.RegisterHookWithDescription(HookPreToolCall, "policy gate", func(context.Context, HookInvocation) (HookResult, error) {
			return HookResult{}, nil
		}); err != nil {
			return err
		}
		if err := ctx.RegisterSlashCommand(CommandRegistration{Name: "/inspect", Description: "Inspect state"}); err != nil {
			return err
		}
		if err := ctx.RegisterCLICommand(CommandRegistration{Name: "plugins", Description: "Manage plugins"}); err != nil {
			return err
		}
		return ctx.RegisterTool(ToolRegistration{Name: "demo_tool", Description: "Demo tool"})
	})
	if err != nil {
		t.Fatalf("RegisterPlugin returned error: %v", err)
	}
	if !loaded.Enabled {
		t.Fatalf("expected plugin enabled, got %+v", loaded)
	}

	capabilities := registry.ListEnabledCapabilities()
	got := map[CapabilityKind]map[string]bool{}
	for _, capability := range capabilities {
		if capability.Plugin.Key != "demo" || capability.Plugin.Name != "demo" {
			t.Fatalf("capability missing plugin ref: %+v", capability)
		}
		if got[capability.Kind] == nil {
			got[capability.Kind] = map[string]bool{}
		}
		got[capability.Kind][capability.Name] = true
	}

	for _, want := range []struct {
		kind CapabilityKind
		name string
	}{
		{CapabilityHook, string(HookPreToolCall)},
		{CapabilitySlashCommand, "inspect"},
		{CapabilityCLICommand, "plugins"},
		{CapabilityTool, "demo_tool"},
	} {
		if !got[want.kind][want.name] {
			t.Fatalf("missing capability %s/%s in %+v", want.kind, want.name, capabilities)
		}
	}
}

func TestCollisionUsesSourcePrecedence(t *testing.T) {
	registry := NewRegistry()
	if _, err := registry.RegisterPlugin(PluginManifest{Name: "shared", Key: "tools/shared", Source: SourceBundled}, func(ctx *RegistrationContext) error {
		return ctx.RegisterTool(ToolRegistration{Name: "bundled_tool"})
	}); err != nil {
		t.Fatalf("register bundled plugin: %v", err)
	}
	if _, err := registry.RegisterPlugin(PluginManifest{Name: "shared", Key: "tools/shared", Source: SourceProject}, func(ctx *RegistrationContext) error {
		return ctx.RegisterTool(ToolRegistration{Name: "project_tool"})
	}); err != nil {
		t.Fatalf("register project plugin: %v", err)
	}
	if _, err := registry.RegisterPlugin(PluginManifest{Name: "shared", Key: "tools/shared", Source: SourceUser}, func(ctx *RegistrationContext) error {
		return ctx.RegisterTool(ToolRegistration{Name: "late_user_tool"})
	}); err != nil {
		t.Fatalf("lower-priority duplicate should be shadowed without error: %v", err)
	}

	plugin, ok := registry.LoadedPlugin("tools/shared")
	if !ok {
		t.Fatal("expected active plugin")
	}
	if plugin.Manifest.Source != SourceProject {
		t.Fatalf("expected project plugin to win, got %+v", plugin.Manifest)
	}
	if _, ok := registry.Tool("project_tool"); !ok {
		t.Fatal("expected project tool to be active")
	}
	if _, ok := registry.Tool("bundled_tool"); ok {
		t.Fatal("bundled tool should have been replaced")
	}
	if _, ok := registry.Tool("late_user_tool"); ok {
		t.Fatal("lower-priority user tool should be shadowed")
	}
	if len(registry.ListShadowedPlugins()) != 2 {
		t.Fatalf("expected two shadowed plugin records, got %+v", registry.ListShadowedPlugins())
	}
}

func TestDisabledPluginDoesNotRegisterCapabilities(t *testing.T) {
	registry := NewRegistry(WithDisabledPlugins("blocked"))
	called := false
	loaded, err := registry.RegisterPlugin(PluginManifest{Name: "blocked", Source: SourceUser}, func(ctx *RegistrationContext) error {
		called = true
		return ctx.RegisterTool(ToolRegistration{Name: "should_not_register"})
	})
	if err != nil {
		t.Fatalf("RegisterPlugin returned error: %v", err)
	}
	if called {
		t.Fatal("disabled plugin registration callback should not run")
	}
	if loaded.Enabled || !strings.Contains(loaded.Error, "disabled") {
		t.Fatalf("expected disabled loaded plugin, got %+v", loaded)
	}
	if len(registry.ListEnabledCapabilities()) != 0 {
		t.Fatalf("disabled plugin should not expose capabilities: %+v", registry.ListEnabledCapabilities())
	}
}

func TestEnabledAllowlistGatesPlugins(t *testing.T) {
	registry := NewRegistry(WithEnabledPlugins("allowed"))

	blockedCalled := false
	blocked, err := registry.RegisterPlugin(PluginManifest{Name: "blocked", Source: SourceUser}, func(ctx *RegistrationContext) error {
		blockedCalled = true
		return ctx.RegisterTool(ToolRegistration{Name: "blocked_tool"})
	})
	if err != nil {
		t.Fatalf("register blocked plugin: %v", err)
	}
	if blockedCalled || blocked.Enabled {
		t.Fatalf("plugin outside allowlist should not register, called=%v loaded=%+v", blockedCalled, blocked)
	}

	allowedCalled := false
	allowed, err := registry.RegisterPlugin(PluginManifest{Name: "allowed", Source: SourceUser}, func(ctx *RegistrationContext) error {
		allowedCalled = true
		return ctx.RegisterTool(ToolRegistration{Name: "allowed_tool"})
	})
	if err != nil {
		t.Fatalf("register allowed plugin: %v", err)
	}
	if !allowedCalled || !allowed.Enabled {
		t.Fatalf("allowlisted plugin should register, called=%v loaded=%+v", allowedCalled, allowed)
	}
	if _, ok := registry.Tool("allowed_tool"); !ok {
		t.Fatal("expected allowlisted tool to be active")
	}
}

func TestCommandRegistrationNormalizesAndLooksUpCommands(t *testing.T) {
	registry := NewRegistry()
	_, err := registry.RegisterPlugin(PluginManifest{Name: "commands", Source: SourceUser}, func(ctx *RegistrationContext) error {
		if err := ctx.RegisterSlashCommand(CommandRegistration{Name: "/Do Thing", Description: "Do it"}); err != nil {
			return err
		}
		return ctx.RegisterCLICommand(CommandRegistration{Name: "Audit", Description: "Audit it"})
	})
	if err != nil {
		t.Fatalf("RegisterPlugin returned error: %v", err)
	}

	slash, ok := registry.SlashCommand("/do-thing")
	if !ok || slash.Name != "do-thing" || slash.Description != "Do it" {
		t.Fatalf("unexpected slash command lookup: %+v ok=%v", slash, ok)
	}
	cli, ok := registry.CLICommand("audit")
	if !ok || cli.Name != "audit" || cli.Description != "Audit it" {
		t.Fatalf("unexpected CLI command lookup: %+v ok=%v", cli, ok)
	}
}

func TestPreToolCallBlock(t *testing.T) {
	registry := NewRegistry()
	_, err := registry.RegisterPlugin(PluginManifest{Name: "policy", Source: SourceProject}, func(ctx *RegistrationContext) error {
		return ctx.RegisterHook(HookPreToolCall, func(context.Context, HookInvocation) (HookResult, error) {
			return BlockToolCall("shell blocked"), nil
		})
	})
	if err != nil {
		t.Fatalf("RegisterPlugin returned error: %v", err)
	}

	decision, report := registry.DispatchPreToolCall(context.Background(), ToolCallEvent{Name: "shell"})
	if report.HasErrors() {
		t.Fatalf("unexpected hook errors: %+v", report.Errors)
	}
	if decision.Allowed() || decision.Message != "shell blocked" || decision.PluginKey != "policy" {
		t.Fatalf("expected blocking decision with plugin attribution, got %+v", decision)
	}
}

func TestPostToolCallDoesNotMutateResult(t *testing.T) {
	registry := NewRegistry()
	_, err := registry.RegisterPlugin(PluginManifest{Name: "observer", Source: SourceUser}, func(ctx *RegistrationContext) error {
		return ctx.RegisterHook(HookPostToolCall, func(context.Context, HookInvocation) (HookResult, error) {
			return ReplaceToolResult("observer tried to replace result"), nil
		})
	})
	if err != nil {
		t.Fatalf("RegisterPlugin returned error: %v", err)
	}

	got, report := registry.DispatchToolResult(context.Background(), ToolResultEvent{
		Call:   ToolCallEvent{Name: "demo_tool"},
		Result: "original",
	})
	if report.HasErrors() {
		t.Fatalf("unexpected hook errors: %+v", report.Errors)
	}
	if got != "original" {
		t.Fatalf("post_tool_call should be observational, got %q", got)
	}
}

func TestTransformToolResultRunsAfterPostToolCall(t *testing.T) {
	registry := NewRegistry()
	var observed []string
	_, err := registry.RegisterPlugin(PluginManifest{Name: "transformer", Source: SourceUser}, func(ctx *RegistrationContext) error {
		if err := ctx.RegisterHook(HookPostToolCall, func(_ context.Context, invocation HookInvocation) (HookResult, error) {
			observed = append(observed, "post:"+invocation.ToolResult.Result)
			return ReplaceToolResult("ignored"), nil
		}); err != nil {
			return err
		}
		return ctx.RegisterHook(HookTransformToolResult, func(_ context.Context, invocation HookInvocation) (HookResult, error) {
			observed = append(observed, "transform:"+invocation.ToolResult.Result)
			return ReplaceToolResult(invocation.ToolResult.Result + ":rewritten"), nil
		})
	})
	if err != nil {
		t.Fatalf("RegisterPlugin returned error: %v", err)
	}

	got, report := registry.DispatchToolResult(context.Background(), ToolResultEvent{
		Call:   ToolCallEvent{Name: "demo_tool"},
		Result: "raw",
	})
	if report.HasErrors() {
		t.Fatalf("unexpected hook errors: %+v", report.Errors)
	}
	if got != "raw:rewritten" {
		t.Fatalf("expected transformed result, got %q", got)
	}
	wantObserved := []string{"post:raw", "transform:raw"}
	if strings.Join(observed, ",") != strings.Join(wantObserved, ",") {
		t.Fatalf("unexpected hook order/inputs: got %v want %v", observed, wantObserved)
	}
}

func TestTransformTerminalOutputRunsInOrder(t *testing.T) {
	registry := NewRegistry()
	_, err := registry.RegisterPlugin(PluginManifest{Name: "terminal", Source: SourceUser}, func(ctx *RegistrationContext) error {
		if err := ctx.RegisterHook(HookTransformTerminalOutput, func(_ context.Context, invocation HookInvocation) (HookResult, error) {
			return ReplaceTerminalOutput("[" + invocation.TerminalOutput.Output + "]"), nil
		}); err != nil {
			return err
		}
		return ctx.RegisterHook(HookTransformTerminalOutput, func(_ context.Context, invocation HookInvocation) (HookResult, error) {
			return ReplaceTerminalOutput(strings.ToUpper(invocation.TerminalOutput.Output)), nil
		})
	})
	if err != nil {
		t.Fatalf("RegisterPlugin returned error: %v", err)
	}

	got, report := registry.DispatchTransformTerminalOutput(context.Background(), TerminalOutputEvent{Output: "hello"})
	if report.HasErrors() {
		t.Fatalf("unexpected hook errors: %+v", report.Errors)
	}
	if got != "[HELLO]" {
		t.Fatalf("expected ordered terminal transforms, got %q", got)
	}
}

func TestHookErrorReportingContinuesDispatch(t *testing.T) {
	registry := NewRegistry()
	_, err := registry.RegisterPlugin(PluginManifest{Name: "faulty", Source: SourceUser}, func(ctx *RegistrationContext) error {
		if err := ctx.RegisterHook(HookTransformToolResult, func(context.Context, HookInvocation) (HookResult, error) {
			return HookResult{}, errors.New("boom")
		}); err != nil {
			return err
		}
		return ctx.RegisterHook(HookTransformToolResult, func(context.Context, HookInvocation) (HookResult, error) {
			return ReplaceToolResult("recovered"), nil
		})
	})
	if err != nil {
		t.Fatalf("RegisterPlugin returned error: %v", err)
	}

	got, report := registry.DispatchTransformToolResult(context.Background(), ToolResultEvent{Result: "original"})
	if got != "recovered" {
		t.Fatalf("expected later hook to run after error, got %q", got)
	}
	if len(report.Errors) != 1 {
		t.Fatalf("expected one hook error, got %+v", report.Errors)
	}
	hookErr := report.Errors[0]
	if hookErr.PluginKey != "faulty" || hookErr.Event != HookTransformToolResult || !strings.Contains(hookErr.Message, "boom") {
		t.Fatalf("unexpected hook error details: %+v", hookErr)
	}
}

func TestLLMContextAndSessionLifecycleHooks(t *testing.T) {
	registry := NewRegistry()
	sessionStarts := 0
	_, err := registry.RegisterPlugin(PluginManifest{Name: "contextual", Source: SourceUser}, func(ctx *RegistrationContext) error {
		if err := ctx.RegisterHook(HookPreLLMCall, func(context.Context, HookInvocation) (HookResult, error) {
			return InjectContext("pre context"), nil
		}); err != nil {
			return err
		}
		if err := ctx.RegisterHook(HookPostLLMCall, func(context.Context, HookInvocation) (HookResult, error) {
			return InjectContext("post context"), nil
		}); err != nil {
			return err
		}
		return ctx.RegisterHook(HookOnSessionStart, func(context.Context, HookInvocation) (HookResult, error) {
			sessionStarts++
			return HookResult{}, nil
		})
	})
	if err != nil {
		t.Fatalf("RegisterPlugin returned error: %v", err)
	}

	pre, preReport := registry.DispatchPreLLMCall(context.Background(), LLMCallEvent{SessionID: "s1"})
	if preReport.HasErrors() || len(pre) != 1 || pre[0].Content != "pre context" || pre[0].PluginKey != "contextual" || pre[0].Role != "user" {
		t.Fatalf("unexpected pre LLM injections/report: %+v %+v", pre, preReport)
	}
	post, postReport := registry.DispatchPostLLMCall(context.Background(), LLMCallEvent{SessionID: "s1"}, LLMResponseEvent{Content: "ok"})
	if postReport.HasErrors() || len(post) != 1 || post[0].Content != "post context" {
		t.Fatalf("unexpected post LLM injections/report: %+v %+v", post, postReport)
	}
	sessionReport := registry.DispatchSessionLifecycle(context.Background(), HookOnSessionStart, SessionEvent{SessionID: "s1"})
	if sessionReport.HasErrors() || sessionStarts != 1 {
		t.Fatalf("unexpected session lifecycle result: starts=%d report=%+v", sessionStarts, sessionReport)
	}
}
