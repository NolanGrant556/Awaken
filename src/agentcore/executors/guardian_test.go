package executors

import (
	"context"
	"strings"
	"testing"

	"awaken/src/tool"
	"awaken/src/tool/profile"
)

// readOnlyTestTool is a minimal tool that declares itself read-only via
// the ReadOnlyTool interface. It is used to verify that GuardianExecutor
// allows read-only tools.
type readOnlyTestTool struct {
	name string
}

func (t *readOnlyTestTool) Name() string { return t.name }
func (t *readOnlyTestTool) Description() string {
	return "read-only test tool"
}
func (t *readOnlyTestTool) Parameters() []tool.ToolParameter { return nil }
func (t *readOnlyTestTool) Definition() tool.ToolDefinition {
	return tool.ToolDefinition{
		Name:        t.name,
		DisplayName: t.name,
		Source:      tool.SourceBuiltin,
		Description: "read-only test tool",
		Annotations: tool.ToolAnnotations{ReadOnly: true},
	}
}
func (t *readOnlyTestTool) IsReadOnly() bool { return true }
func (t *readOnlyTestTool) Execute(ctx context.Context, input string) (string, error) {
	return "read-only output: " + input, nil
}

// writeTestTool is a minimal tool that does NOT declare itself read-only.
// It is used to verify that GuardianExecutor denies non-read-only tools.
type writeTestTool struct {
	name string
}

func (t *writeTestTool) Name() string { return t.name }
func (t *writeTestTool) Description() string {
	return "write test tool"
}
func (t *writeTestTool) Parameters() []tool.ToolParameter { return nil }
func (t *writeTestTool) Definition() tool.ToolDefinition {
	return tool.ToolDefinition{
		Name:        t.name,
		DisplayName: t.name,
		Source:      tool.SourceBuiltin,
		Description: "write test tool",
		Annotations: tool.ToolAnnotations{ReadOnly: false},
	}
}
func (t *writeTestTool) Execute(ctx context.Context, input string) (string, error) {
	return "write output: " + input, nil
}

// TestGuardianExecutorDeniesNonReadOnlyTool verifies that the
// GuardianExecutor denies a tool that does not implement ReadOnlyTool or
// reports IsReadOnly() == false.
func TestGuardianExecutorDeniesNonReadOnlyTool(t *testing.T) {
	executor := NewGuardianExecutor()
	intent := profile.ExecutionIntent{
		ToolName: "write_tool",
		ToolKind: profile.ToolKindFileOperation,
	}
	nonReadOnly := &writeTestTool{name: "write_tool"}
	output, err := executor.Execute(context.Background(), intent, profile.GuardianReadOnly(), nonReadOnly, "test", nil)
	if err == nil {
		t.Fatalf("expected error for non-read-only tool, got nil")
	}
	if !output.IsError {
		t.Fatalf("expected IsError=true, got false")
	}
	if output.ErrorCode != "guardian_non_read_only" {
		t.Fatalf("expected ErrorCode=guardian_non_read_only, got %q", output.ErrorCode)
	}
	if !strings.Contains(output.Content, "not read-only") {
		t.Fatalf("expected denial message to mention 'not read-only', got %q", output.Content)
	}
}

// TestGuardianExecutorAllowsReadOnlyTool verifies that the GuardianExecutor
// allows and delegates to a tool that implements ReadOnlyTool with
// IsReadOnly() == true.
func TestGuardianExecutorAllowsReadOnlyTool(t *testing.T) {
	executor := NewGuardianExecutor()
	intent := profile.ExecutionIntent{
		ToolName: "read_tool",
		ToolKind: profile.ToolKindFileOperation,
	}
	readOnly := &readOnlyTestTool{name: "read_tool"}
	output, err := executor.Execute(context.Background(), intent, profile.GuardianReadOnly(), readOnly, "test_input", nil)
	if err != nil {
		t.Fatalf("unexpected error for read-only tool: %v", err)
	}
	if output.IsError {
		t.Fatalf("expected IsError=false, got true (ErrorCode=%q Content=%q)", output.ErrorCode, output.Content)
	}
	expected := "read-only output: test_input"
	if output.Content != expected {
		t.Fatalf("expected content %q, got %q", expected, output.Content)
	}
}
