package agentcore

import (
	"context"
	"strings"
	"testing"

	convcontext "awaken/src/context"
	"awaken/src/subagentruntime"
	"awaken/src/tool"
	"awaken/src/tool/profile"
)

func TestChildQuestionReturnsToParentWithoutOpeningUserGate(t *testing.T) {
	sink := &userQuestionTestSink{answer: "must not consume this answer"}
	runtime := NewToolRuntimeWithConfig(sink, ToolRuntimeConfig{SubagentOrigin: &profile.SubagentOrigin{AgentID: "child"}})
	result := runtime.Execute(context.Background(), convcontext.ToolCall{ID: "child-question", Function: convcontext.FunctionCall{
		Name: "ask_user", Arguments: `{"questions":[{"question":"Which file?","options":[{"label":"A"},{"label":"B"}]}]}`,
	}})
	if !result.IsError || result.ErrorCode != "ask_user_parent_required" || !strings.Contains(result.Content, "父 Agent") {
		t.Fatalf("missing parent handoff: %+v", result)
	}
	for _, event := range sink.events {
		if event.Type == UserQuestionRequestedType {
			t.Fatal("child opened a direct user question")
		}
	}
}

func TestAllChildRolesExcludeDirectUserQuestions(t *testing.T) {
	for _, role := range []subagentruntime.Role{
		{Type: "general"},
		{Type: "plugin", AllowedTools: []string{"Read", "ask_user"}},
	} {
		child := AgentConfig{ToolExposure: tool.ToolExposurePolicy{Mode: tool.ExposureAll}}
		applyResolvedChildRole(&child, role)
		found := false
		for _, name := range child.ToolExposure.ExtraHidden {
			found = found || name == "ask_user"
		}
		if !found {
			t.Fatalf("%s exposed a direct user question", role.Type)
		}
	}
}
