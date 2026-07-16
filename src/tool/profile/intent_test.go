package profile_test

import (
	"encoding/json"
	"strings"
	"testing"

	convcontext "awaken/src/context"
	"awaken/src/tool"
	"awaken/src/tool/profile"
	bashtool "awaken/src/tools/bash"
	edittool "awaken/src/tools/edit"
	readtool "awaken/src/tools/read"
	writetool "awaken/src/tools/write"
)

func mustMarshalArgs(t *testing.T, args map[string]any) string {
	t.Helper()
	data, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestInferSevenToolKinds(t *testing.T) {
	tests := []struct {
		name        string
		annotations tool.ToolAnnotations
		want        profile.ToolKind
	}{
		{"Read", tool.ToolAnnotations{ReadOnly: true}, profile.ToolKindFileOperation},
		{"Edit", tool.ToolAnnotations{}, profile.ToolKindFileOperation},
		{"Write", tool.ToolAnnotations{}, profile.ToolKindFileOperation},
		{"Glob", tool.ToolAnnotations{ReadOnly: true}, profile.ToolKindFileOperation},
		{"Grep", tool.ToolAnnotations{ReadOnly: true}, profile.ToolKindFileOperation},
		{"WebSearch", tool.ToolAnnotations{ReadOnly: true, OpenWorld: true}, profile.ToolKindFileOperation},
		{"Bash", tool.ToolAnnotations{OpenWorld: true}, profile.ToolKindShellCommand},
	}
	for _, test := range tests {
		if got := profile.InferToolKind(test.name, test.annotations); got != test.want {
			t.Errorf("InferToolKind(%s)=%s want %s", test.name, got, test.want)
		}
	}
}

func TestBuildExecutionIntentForFileTools(t *testing.T) {
	workspace := t.TempDir()
	permission := tool.NewPermissionContext(workspace, []string{workspace})
	tests := []struct {
		name       string
		arguments  map[string]any
		runtime    tool.Tool
		wantAction profile.RequestedAction
		wantPath   string
	}{
		{"Read", map[string]any{"file_path": "notes/a.md"}, &readtool.Tool{}, profile.ActionRead, "notes/a.md"},
		{"Edit", map[string]any{"file_path": "notes/a.md", "old_string": "a", "new_string": "b"}, &edittool.Tool{}, profile.ActionWrite, "notes/a.md"},
		{"Write", map[string]any{"file_path": "notes/b.md", "content": "hello"}, &writetool.Tool{}, profile.ActionWrite, "notes/b.md"},
	}
	for _, test := range tests {
		call := convcontext.ToolCall{ID: "call_" + test.name, Type: "function", Function: convcontext.FunctionCall{Name: test.name, Arguments: mustMarshalArgs(t, test.arguments)}}
		intent := profile.BuildExecutionIntent(call, test.runtime, permission, nil)
		if intent.ToolName != test.name || intent.RequestedAction != test.wantAction || intent.ToolKind != profile.ToolKindFileOperation {
			t.Fatalf("%s intent=%+v", test.name, intent)
		}
		if len(intent.Paths) != 1 || filepathSlash(intent.Paths[0]) != test.wantPath {
			t.Fatalf("%s paths=%v want %s", test.name, intent.Paths, test.wantPath)
		}
	}
}

func TestBuildExecutionIntentForBash(t *testing.T) {
	workspace := t.TempDir()
	permission := tool.NewPermissionContext(workspace, []string{workspace})
	call := convcontext.ToolCall{ID: "call_bash", Type: "function", Function: convcontext.FunctionCall{
		Name: "Bash", Arguments: mustMarshalArgs(t, map[string]any{"command": "echo hello > output.txt"}),
	}}
	intent := profile.BuildExecutionIntent(call, &bashtool.Tool{}, permission, nil)
	if intent.ToolKind != profile.ToolKindShellCommand || intent.RequestedAction != profile.ActionExecute || intent.NetworkScope != profile.NetworkScopeOpen {
		t.Fatalf("Bash intent=%+v", intent)
	}
	if len(intent.Paths) != 1 || filepathSlash(intent.Paths[0]) != "output.txt" {
		t.Fatalf("Bash redirect paths=%v", intent.Paths)
	}
}

func TestExecutionIntentStringIncludesCoreFacts(t *testing.T) {
	intent := profile.ExecutionIntent{ToolName: "Edit", ToolKind: profile.ToolKindFileOperation, RequestedAction: profile.ActionWrite, Paths: []string{"a.txt", "b.txt"}}
	text := intent.String()
	for _, want := range []string{"Edit", "file_operation", "write", "a.txt", "b.txt"} {
		if !strings.Contains(text, want) {
			t.Fatalf("intent string %q missing %q", text, want)
		}
	}
}

func filepathSlash(value string) string { return strings.ReplaceAll(value, `\\`, "/") }
