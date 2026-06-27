package toolorder

import (
	"reflect"
	"testing"
)

func TestAssembleKeepsBuiltinsAsSortedPrefix(t *testing.T) {
	result := Assemble(
		[]Tool{
			{Name: "write_file", Source: SourceBuiltin},
			{Name: "read_file", Source: SourceBuiltin},
		},
		[]Tool{
			{Name: "aaa_mcp_tool", Source: SourceMCP},
			{Name: "zzz_mcp_tool", Source: SourceMCP},
		},
	)

	assertNames(t, result.Tools, []string{"read_file", "write_file", "aaa_mcp_tool", "zzz_mcp_tool"})
	if result.CacheBreakpointIndex != 2 {
		t.Fatalf("expected cache breakpoint after builtin prefix, got %d", result.CacheBreakpointIndex)
	}
}

func TestAssembleDoesNotInsertMCPToolsIntoBuiltinPrefix(t *testing.T) {
	result := Assemble(
		[]Tool{
			{Name: "shell", Source: SourceBuiltin},
			{Name: "view", Source: SourceBuiltin},
		},
		[]Tool{
			{Name: "apply_patch", Source: SourceMCP},
		},
	)

	assertSources(t, result.Tools, []Source{SourceBuiltin, SourceBuiltin, SourceMCP})
	assertNames(t, result.Tools, []string{"shell", "view", "apply_patch"})
}

func TestAssembleSortsEachPartitionByName(t *testing.T) {
	result := Assemble(
		[]Tool{
			{Name: "z_builtin", Source: SourceBuiltin},
			{Name: "a_builtin", Source: SourceBuiltin},
			{Name: "m_builtin", Source: SourceBuiltin},
		},
		[]Tool{
			{Name: "z_external", Source: SourceMCP},
			{Name: "a_external", Source: SourcePlugin},
			{Name: "m_external", Source: SourceSession},
		},
	)

	assertNames(t, result.Tools, []string{
		"a_builtin",
		"m_builtin",
		"z_builtin",
		"a_external",
		"m_external",
		"z_external",
	})
}

func TestAssemblePrefersBuiltinsForDuplicateNames(t *testing.T) {
	result := Assemble(
		[]Tool{
			{Name: "read", Source: SourceBuiltin},
		},
		[]Tool{
			{Name: "read", Source: SourceMCP},
			{Name: "write", Source: SourceMCP},
		},
	)

	assertSources(t, result.Tools, []Source{SourceBuiltin, SourceMCP})
	assertNames(t, result.DroppedDuplicates, []string{"read"})
	assertSources(t, result.DroppedDuplicates, []Source{SourceMCP})
}

func TestAssembleKeepsFirstExternalDuplicateAfterStableSort(t *testing.T) {
	result := Assemble(nil, []Tool{
		{Name: "shared", Source: SourcePlugin},
		{Name: "alpha", Source: SourceMCP},
		{Name: "shared", Source: SourceSession},
	})

	assertNames(t, result.Tools, []string{"alpha", "shared"})
	assertSources(t, result.Tools, []Source{SourceMCP, SourcePlugin})
	assertNames(t, result.DroppedDuplicates, []string{"shared"})
	assertSources(t, result.DroppedDuplicates, []Source{SourceSession})
}

func TestAssembleReportsCacheBreakpointAfterKeptBuiltins(t *testing.T) {
	result := Assemble(
		[]Tool{
			{Name: "shell", Source: SourceBuiltin},
			{Name: "edit", Source: SourceBuiltin},
			{Name: "shell", Source: SourceBuiltin},
		},
		[]Tool{
			{Name: "alpha", Source: SourceMCP},
			{Name: "shell", Source: SourceMCP},
		},
	)

	assertNames(t, result.Tools, []string{"edit", "shell", "alpha"})
	if result.CacheBreakpointIndex != 2 {
		t.Fatalf("expected cache breakpoint after kept builtins, got %d", result.CacheBreakpointIndex)
	}
	assertNames(t, result.DroppedDuplicates, []string{"shell", "shell"})
	assertSources(t, result.DroppedDuplicates, []Source{SourceBuiltin, SourceMCP})
}

func TestAssembleDoesNotMutateInputs(t *testing.T) {
	builtins := []Tool{
		{Name: "z_builtin", Source: SourceBuiltin},
		{Name: "a_builtin", Source: SourceBuiltin},
	}
	externals := []Tool{
		{Name: "z_external", Source: SourceMCP},
		{Name: "a_external", Source: SourceMCP},
	}
	wantBuiltins := append([]Tool(nil), builtins...)
	wantExternals := append([]Tool(nil), externals...)

	result := Assemble(builtins, externals)

	if !reflect.DeepEqual(builtins, wantBuiltins) {
		t.Fatalf("expected builtins input to remain unchanged, got %#v", builtins)
	}
	if !reflect.DeepEqual(externals, wantExternals) {
		t.Fatalf("expected externals input to remain unchanged, got %#v", externals)
	}

	builtins[0].Name = "changed_builtin"
	externals[0].Name = "changed_external"

	assertNames(t, result.Tools, []string{"a_builtin", "z_builtin", "a_external", "z_external"})
}

func assertNames(t *testing.T, tools []Tool, want []string) {
	t.Helper()

	got := make([]string, 0, len(tools))
	for _, tool := range tools {
		got = append(got, tool.Name)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected names %v, got %v", want, got)
	}
}

func assertSources(t *testing.T, tools []Tool, want []Source) {
	t.Helper()

	got := make([]Source, 0, len(tools))
	for _, tool := range tools {
		got = append(got, tool.Source)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected sources %v, got %v", want, got)
	}
}
