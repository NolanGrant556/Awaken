package texttoolparser

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseNoToolCall(t *testing.T) {
	text := "plain assistant text"

	result := Parse("hermes", text)

	if result.Found {
		t.Fatalf("expected no tool call, got %+v", result)
	}
	if result.Content != text {
		t.Fatalf("expected original content %q, got %q", text, result.Content)
	}
	if len(result.ToolCalls) != 0 {
		t.Fatalf("expected no tool calls, got %+v", result.ToolCalls)
	}
}

func TestParseUnknownParserFallback(t *testing.T) {
	text := `<tool_call>{"name":"lookup","arguments":{"query":"awaken"}}</tool_call>`

	result := Parse("missing", text)

	if result.Found {
		t.Fatalf("expected unknown parser fallback, got %+v", result)
	}
	if result.Content != text {
		t.Fatalf("expected original content %q, got %q", text, result.Content)
	}
}

func TestParseHermesClosed(t *testing.T) {
	text := `I will search.
<tool_call>{"name":"lookup","arguments":{"query":"awaken","limit":2}}</tool_call>`

	result := Parse("hermes", text)

	assertFound(t, result)
	if result.Content != "I will search." {
		t.Fatalf("unexpected content: %q", result.Content)
	}
	want := []ToolCall{{
		ID:        "call_texttool_0",
		Name:      "lookup",
		Arguments: `{"query":"awaken","limit":2}`,
	}}
	if !reflect.DeepEqual(result.ToolCalls, want) {
		t.Fatalf("unexpected tool calls:\nwant %+v\n got %+v", want, result.ToolCalls)
	}
}

func TestParseHermesUnclosed(t *testing.T) {
	text := `Ready<tool_call>{"name":"read_file","arguments":{"path":"README.md"}}`

	result := Parse("hermes", text)

	assertFound(t, result)
	if result.Content != "Ready" {
		t.Fatalf("unexpected content: %q", result.Content)
	}
	if result.ToolCalls[0].ID != "call_texttool_0" || result.ToolCalls[0].Name != "read_file" {
		t.Fatalf("unexpected tool call: %+v", result.ToolCalls[0])
	}
	if result.ToolCalls[0].Arguments != `{"path":"README.md"}` {
		t.Fatalf("unexpected arguments: %q", result.ToolCalls[0].Arguments)
	}
}

func TestParseHermesInvalidJSONFallback(t *testing.T) {
	text := `<tool_call>{"name":"lookup","arguments":}</tool_call>`

	result := Parse("hermes", text)

	if result.Found {
		t.Fatalf("expected invalid JSON fallback, got %+v", result)
	}
	if result.Content != text {
		t.Fatalf("expected original content %q, got %q", text, result.Content)
	}
}

func TestParseHermesMissingNameIgnored(t *testing.T) {
	text := `<tool_call>{"arguments":{"query":"awaken"}}</tool_call>`

	result := Parse("hermes", text)

	if result.Found {
		t.Fatalf("expected missing name to be ignored, got %+v", result)
	}
	if result.Content != text {
		t.Fatalf("expected original content %q, got %q", text, result.Content)
	}
}

func TestParseQwenAlias(t *testing.T) {
	text := `<tool_call>{"name":"shell","arguments":{"cmd":"pwd"}}</tool_call>`

	result := Parse("qwen", text)

	assertFound(t, result)
	if len(result.ToolCalls) != 1 || result.ToolCalls[0].Name != "shell" {
		t.Fatalf("unexpected qwen alias result: %+v", result)
	}
	if result.ToolCalls[0].Arguments != `{"cmd":"pwd"}` {
		t.Fatalf("unexpected arguments: %q", result.ToolCalls[0].Arguments)
	}
}

func TestParseHermesMissingArgumentsDefaultsToEmptyObject(t *testing.T) {
	text := `<tool_call>{"name":"lookup"}</tool_call>`

	result := Parse("hermes", text)

	assertFound(t, result)
	if result.ToolCalls[0].Arguments != `{}` {
		t.Fatalf("expected missing arguments to default to empty object, got %q", result.ToolCalls[0].Arguments)
	}
}

func TestParseKimiFunctionNameExtraction(t *testing.T) {
	text := `Plan<|tool_calls_section_begin|><|tool_call_begin|>functions.search_files:0<|tool_call_argument_begin|>{"query":"parser"}<|tool_call_end|><|tool_calls_section_end|>Done`

	result := Parse("kimi_k2", text)

	assertFound(t, result)
	if result.Content != "PlanDone" {
		t.Fatalf("unexpected content: %q", result.Content)
	}
	want := []ToolCall{{
		ID:        "functions.search_files:0",
		Name:      "search_files",
		Arguments: `{"query":"parser"}`,
	}}
	if !reflect.DeepEqual(result.ToolCalls, want) {
		t.Fatalf("unexpected tool calls:\nwant %+v\n got %+v", want, result.ToolCalls)
	}
}

func TestParseKimiSingularSectionToken(t *testing.T) {
	text := `Before<|tool_call_section_begin|><|tool_call_begin|>read_file:0<|tool_call_argument_begin|>{ "path" : "go.mod" }<|tool_call_end|><|tool_call_section_end|>After`

	result := Parse("kimi-k2", text)

	assertFound(t, result)
	if result.Content != "BeforeAfter" {
		t.Fatalf("unexpected content: %q", result.Content)
	}
	if result.ToolCalls[0].ID != "read_file:0" || result.ToolCalls[0].Name != "read_file" {
		t.Fatalf("unexpected tool call: %+v", result.ToolCalls[0])
	}
	if result.ToolCalls[0].Arguments != `{"path":"go.mod"}` {
		t.Fatalf("unexpected compact arguments: %q", result.ToolCalls[0].Arguments)
	}
}

func TestParseKimiMultipleToolCalls(t *testing.T) {
	text := `Plan<|tool_calls_section_begin|><|tool_call_begin|>functions.search_files:0<|tool_call_argument_begin|>{"query":"parser"}<|tool_call_end|><|tool_call_begin|>read_file:1<|tool_call_argument_begin|>{"path":"README.md"}<|tool_call_end|><|tool_calls_section_end|>Done`

	result := Parse("kimi", text)

	assertFound(t, result)
	if result.Content != "PlanDone" {
		t.Fatalf("unexpected content: %q", result.Content)
	}
	want := []ToolCall{
		{ID: "functions.search_files:0", Name: "search_files", Arguments: `{"query":"parser"}`},
		{ID: "read_file:1", Name: "read_file", Arguments: `{"path":"README.md"}`},
	}
	if !reflect.DeepEqual(result.ToolCalls, want) {
		t.Fatalf("unexpected tool calls:\nwant %+v\n got %+v", want, result.ToolCalls)
	}
}

func TestParseHermesMultipleToolCalls(t *testing.T) {
	text := `A<tool_call>{"name":"read_file","arguments":{"path":"a.go"}}</tool_call>B<tool_call>{"name":"shell","arguments":{"cmd":"go test"}}</tool_call>C`

	result := Parse("hermes", text)

	assertFound(t, result)
	if result.Content != "ABC" {
		t.Fatalf("unexpected stripped content: %q", result.Content)
	}
	want := []ToolCall{
		{ID: "call_texttool_0", Name: "read_file", Arguments: `{"path":"a.go"}`},
		{ID: "call_texttool_1", Name: "shell", Arguments: `{"cmd":"go test"}`},
	}
	if !reflect.DeepEqual(result.ToolCalls, want) {
		t.Fatalf("unexpected tool calls:\nwant %+v\n got %+v", want, result.ToolCalls)
	}
}

func TestParseHermesContentStrip(t *testing.T) {
	text := `Intro<tool_call>{"name":"lookup","arguments":{}}</tool_call>Outro`

	result := Parse("hermes", text)

	assertFound(t, result)
	if result.Content != "IntroOutro" {
		t.Fatalf("unexpected stripped content: %q", result.Content)
	}
	if strings.Contains(result.Content, "tool_call") {
		t.Fatalf("expected tool_call markup to be stripped, got %q", result.Content)
	}
}

func assertFound(t *testing.T, result Result) {
	t.Helper()
	if !result.Found {
		t.Fatalf("expected tool calls to be found, got %+v", result)
	}
	if len(result.ToolCalls) == 0 {
		t.Fatalf("expected at least one tool call, got %+v", result)
	}
}
