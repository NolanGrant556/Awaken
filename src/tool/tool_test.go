package tool

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"testing"
)

type catalogTestTool struct {
	name       string
	paramName  string
	paramType  string
	executeErr error
}

func (t catalogTestTool) Name() string {
	return t.name
}

func (t catalogTestTool) Description() string {
	return "test tool"
}

func (t catalogTestTool) Definition() ToolDefinition {
	return ToolDefinition{
		Name:        t.name,
		DisplayName: t.name,
		Source:      SourceBuiltin,
		Description: "test tool",
		Annotations: ToolAnnotations{ReadOnly: false, Destructive: false, OpenWorld: false},
	}
}

func (t catalogTestTool) Parameters() []ToolParameter {
	if t.paramName == "" {
		return nil
	}
	return []ToolParameter{{Name: t.paramName, Type: t.paramType, Required: true}}
}

func (t catalogTestTool) Execute(ctx context.Context, input string) (string, error) {
	if t.executeErr != nil {
		return "", t.executeErr
	}
	return input, nil
}

func TestRegisterToolRejectsDuplicateName(t *testing.T) {
	candidate := catalogTestTool{name: "catalog_duplicate_tool"}
	if err := RegisterTool(candidate); err != nil {
		t.Fatalf("expected first registration to succeed, got %v", err)
	}

	if err := RegisterTool(candidate); !errors.Is(err, ErrDuplicateTool) {
		t.Fatalf("expected ErrDuplicateTool, got %v", err)
	}
}

func TestValidateToolRejectsInvalidNameAndParameterType(t *testing.T) {
	if err := ValidateTool(catalogTestTool{name: "bad name"}); !errors.Is(err, ErrInvalidTool) {
		t.Fatalf("expected invalid tool name error, got %v", err)
	}
	if err := ValidateTool(catalogTestTool{name: "valid_name", paramName: "flag", paramType: "bool"}); !errors.Is(err, ErrInvalidTool) {
		t.Fatalf("expected invalid parameter type error, got %v", err)
	}
	if err := ValidateTool(catalogTestTool{name: "valid_name", paramName: "bad param", paramType: "string"}); !errors.Is(err, ErrInvalidTool) {
		t.Fatalf("expected invalid parameter name error, got %v", err)
	}
}

func TestListToolsIsStableSorted(t *testing.T) {
	first := toolNames(ListTools())
	second := toolNames(ListTools())
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("expected stable tool order, got %v then %v", first, second)
	}
	if !sort.StringsAreSorted(first) {
		t.Fatalf("expected sorted tool names, got %v", first)
	}
}

// TestValidateToolAcceptsNamespacedMcpNames verifies the extended
// mcpSafeNamePattern accepts nested forms used by the MCP runtime.
func TestValidateToolAcceptsNamespacedMcpNames(t *testing.T) {
	cases := []string{
		"mcp__filesystem__read_file",
		"mcp__slack__post_message",
		"mcp__chrome-devtools-mcp__close_page",
		"mcp__planning__draft_outline",
		"web_search", // builtin-style still accepted
	}
	for _, name := range cases {
		if err := ValidateTool(catalogTestTool{name: name}); err != nil {
			t.Errorf("expected %q to be valid, got %v", name, err)
		}
	}
}

// TestRegisterToolForceReplacesExisting verifies the Force variant overwrites
// a prior registration without error. Used by the MCP manager when refreshing
// a server's tool list.
func TestRegisterToolForceReplacesExisting(t *testing.T) {
	first := catalogTestTool{name: "catalog_force_tool"}
	second := catalogTestTool{name: "catalog_force_tool"}

	if err := RegisterTool(first); err != nil {
		t.Fatalf("initial register failed: %v", err)
	}
	if err := RegisterToolForce(second); err != nil {
		t.Fatalf("force register failed: %v", err)
	}
	// Clean up.
	UnregisterTool("catalog_force_tool")
}

func TestRegisterToolForceIfChangedPreservesIdentityForNoOpRefresh(t *testing.T) {
	const name = "catalog_force_if_changed_tool"
	t.Cleanup(func() { UnregisterTool(name) })
	runtime := &catalogTestTool{name: name}
	if err := RegisterToolForce(runtime); err != nil {
		t.Fatalf("initial register failed: %v", err)
	}
	before, err := GetRegisteredTool(name)
	if err != nil {
		t.Fatal(err)
	}
	beforeVersion := RegistryVersion()

	changed, err := RegisterToolForceIfChanged(runtime)
	if err != nil {
		t.Fatalf("idempotent refresh failed: %v", err)
	}
	if changed {
		t.Fatal("identical runtime and definition unexpectedly replaced registration")
	}
	after, err := GetRegisteredTool(name)
	if err != nil {
		t.Fatal(err)
	}
	if after.RegistrationID != before.RegistrationID || RegistryVersion() != beforeVersion {
		t.Fatalf("no-op refresh churned registry identity: before=%s after=%s versions=%d/%d", before.RegistrationID, after.RegistrationID, beforeVersion, RegistryVersion())
	}

	replacement := &catalogTestTool{name: name}
	changed, err = RegisterToolForceIfChanged(replacement)
	if err != nil {
		t.Fatalf("runtime replacement failed: %v", err)
	}
	if !changed {
		t.Fatal("distinct runtime binding was treated as a no-op")
	}
	replaced, err := GetRegisteredTool(name)
	if err != nil {
		t.Fatal(err)
	}
	if replaced.RegistrationID == before.RegistrationID || replaced.Runtime != replacement {
		t.Fatalf("runtime replacement did not receive a new execution pin: before=%+v after=%+v", before, replaced)
	}
}

// TestUnregisterToolsByPrefix verifies bulk removal by prefix. Used by the
// MCP manager to clear all tools from a single server in one call.
func TestUnregisterToolsByPrefix(t *testing.T) {
	names := []string{
		"mcp__echo__alpha",
		"mcp__echo__beta",
		"mcp__other__gamma",
	}
	for _, n := range names {
		if err := RegisterToolForce(catalogTestTool{name: n}); err != nil {
			t.Fatalf("register %s: %v", n, err)
		}
	}
	removed := UnregisterToolsByPrefix("mcp__echo__")
	if removed != 2 {
		t.Errorf("expected 2 removals, got %d", removed)
	}
	all := ListToolNames()
	for _, n := range all {
		if n == "mcp__echo__alpha" || n == "mcp__echo__beta" {
			t.Errorf("expected echo tools removed, found %s", n)
		}
	}
	// Clean up the remaining mcp__other__gamma.
	UnregisterToolsByPrefix("mcp__other__")
}

// TestListToolNames verifies the helper returns a sorted snapshot of all
// currently registered tool names.
func TestListToolNames(t *testing.T) {
	RegisterToolForce(catalogTestTool{name: "catalog_list_a"})
	RegisterToolForce(catalogTestTool{name: "catalog_list_b"})
	defer func() {
		UnregisterTool("catalog_list_a")
		UnregisterTool("catalog_list_b")
	}()

	names := ListToolNames()
	found := map[string]bool{}
	for _, n := range names {
		found[n] = true
	}
	if !found["catalog_list_a"] || !found["catalog_list_b"] {
		t.Errorf("expected both catalog_list_* in %v", names)
	}
	if !sort.StringsAreSorted(names) {
		t.Errorf("expected sorted names, got %v", names)
	}
}

func toolNames(tools []Tool) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name())
	}
	return names
}
