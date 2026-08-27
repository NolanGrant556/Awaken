package agentcore_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"awaken/src/agentcore"
	convcontext "awaken/src/context"
	"awaken/src/tool"
	writetool "awaken/src/tools/write"
)

func TestToolRuntimeWritesInsideManagedSessionWorkspace(t *testing.T) {
	home := t.TempDir()
	awakenHome := filepath.Join(home, ".awaken")
	sessionsRoot := filepath.Join(awakenHome, "sessions")
	workspace := filepath.Join(sessionsRoot, "thread_runtime_write")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}

	permissionContext := tool.NewTaskScopePermissionContext(
		"scope_runtime_write",
		workspace,
		[]string{workspace},
		[]string{workspace},
		[]string{filepath.Join(workspace, "output")},
		nil,
		[]string{awakenHome},
		filepath.Join(t.TempDir(), "scope_runtime_write"),
	)
	permissionContext.ProtectedRootExceptions = []string{sessionsRoot}
	permissionContext = permissionContext.Normalize()

	target := filepath.Join(workspace, "translation-content.md")
	arguments, err := json.Marshal(map[string]string{"file_path": target, "content": "translated content\n"})
	if err != nil {
		t.Fatal(err)
	}
	runtime := agentcore.NewToolRuntimeWithConfig(nil, agentcore.ToolRuntimeConfig{
		PermissionContext: permissionContext,
		ToolSessionState:  tool.NewToolSessionState(workspace),
	})
	result := runtime.Execute(context.Background(), convcontext.ToolCall{
		ID: "write-managed-session",
		Function: convcontext.FunctionCall{
			Name:      writetool.ToolName,
			Arguments: string(arguments),
		},
	})
	if result.IsError || result.Outcome.Status != tool.OutcomeSucceeded {
		t.Fatalf("Write in a managed session workspace failed: %+v", result)
	}
	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "translated content\n" {
		t.Fatalf("unexpected file content %q", content)
	}
}

func TestToolRuntimeWritesExplicitAbsolutePathOutsideWorkspaceRoots(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	permissionContext := tool.NewTaskScopePermissionContext(
		"scope_external_write",
		workspace,
		[]string{workspace},
		[]string{workspace},
		[]string{workspace},
		nil,
		nil,
		filepath.Join(workspace, ".tmp"),
	)
	target := filepath.Join(outside, "translation-content.md")
	arguments, err := json.Marshal(map[string]string{"file_path": target, "content": "external content\n"})
	if err != nil {
		t.Fatal(err)
	}
	runtime := agentcore.NewToolRuntimeWithConfig(nil, agentcore.ToolRuntimeConfig{
		PermissionContext: permissionContext,
		ToolSessionState:  tool.NewToolSessionState(workspace),
	})
	result := runtime.Execute(context.Background(), convcontext.ToolCall{
		ID: "write-explicit-external-path",
		Function: convcontext.FunctionCall{
			Name:      writetool.ToolName,
			Arguments: string(arguments),
		},
	})
	if result.IsError || result.Outcome.Status != tool.OutcomeSucceeded {
		t.Fatalf("Write to an explicit absolute path outside workspace roots failed: %+v", result)
	}
	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "external content\n" {
		t.Fatalf("unexpected file content %q", content)
	}
}

func TestToolRuntimeWritesPluginRepositoryExceptionButNotPluginState(t *testing.T) {
	home := t.TempDir()
	awakenHome := filepath.Join(home, ".awaken")
	pluginsRoot := filepath.Join(awakenHome, "plugins")
	packageRoot := filepath.Join(pluginsRoot, "packages", "acme")
	workspace := filepath.Join(home, "workspace")
	if err := os.MkdirAll(packageRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(awakenHome, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}

	permissionContext := tool.NewTaskScopePermissionContext(
		"scope_plugin_write",
		workspace,
		[]string{workspace},
		[]string{workspace},
		[]string{workspace},
		nil,
		[]string{awakenHome},
		filepath.Join(t.TempDir(), "scope_plugin_write"),
	)
	permissionContext.ProtectedRootExceptions = []string{pluginsRoot}
	permissionContext = permissionContext.Normalize()
	runtime := agentcore.NewToolRuntimeWithConfig(nil, agentcore.ToolRuntimeConfig{
		PermissionContext: permissionContext,
		ToolSessionState:  tool.NewToolSessionState(workspace),
	})

	packageFile := filepath.Join(packageRoot, "notes.txt")
	writeArgs, err := json.Marshal(map[string]string{"file_path": packageFile, "content": "authorized plugin edit\n"})
	if err != nil {
		t.Fatal(err)
	}
	writeResult := runtime.Execute(context.Background(), convcontext.ToolCall{
		ID:       "write-plugin-package",
		Function: convcontext.FunctionCall{Name: writetool.ToolName, Arguments: string(writeArgs)},
	})
	if writeResult.IsError || writeResult.Outcome.Status != tool.OutcomeSucceeded {
		t.Fatalf("Write inside the Plugin repository failed: %+v", writeResult)
	}
	if content, err := os.ReadFile(packageFile); err != nil || string(content) != "authorized plugin edit\n" {
		t.Fatalf("Plugin package write content=%q err=%v", content, err)
	}

	for index, target := range []string{
		filepath.Join(awakenHome, "config", "plugins.json"),
		filepath.Join(packageRoot, ".env"),
	} {
		args, marshalErr := json.Marshal(map[string]string{"file_path": target, "content": "must not be written\n"})
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		result := runtime.Execute(context.Background(), convcontext.ToolCall{
			ID:       fmt.Sprintf("deny-plugin-sensitive-%d", index),
			Function: convcontext.FunctionCall{Name: writetool.ToolName, Arguments: string(args)},
		})
		if !result.IsError || result.Outcome.Status != tool.OutcomeDenied {
			t.Fatalf("sensitive Plugin-related target %q was not denied: %+v", target, result)
		}
		if _, statErr := os.Lstat(target); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("denied target %q changed: %v", target, statErr)
		}
	}
}
