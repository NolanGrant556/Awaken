package runtime

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"awaken/src/agentcore"
	convcontext "awaken/src/context"
	"awaken/src/llm"
	"awaken/src/tool"
)

func TestContextEpochKeepsStableBaselineAcrossDeltasAndRestart(t *testing.T) {
	t.Setenv(awakenHomeEnv, t.TempDir())
	store := NewContextEpochStore()
	candidate := testContextEpochCandidate()

	first, err := store.Project(context.Background(), candidate)
	if err != nil {
		t.Fatalf("create epoch: %v", err)
	}
	if first.Receipt.State != "created" || first.Receipt.BoundaryReason != "initial_baseline" {
		t.Fatalf("unexpected initial receipt: %+v", first.Receipt)
	}
	if got := contextEpochToolNames(first.Tools); strings.Join(got, ",") != "alpha,beta,mcp-zulu" {
		t.Fatalf("expected canonical builtin/category order, got %v", got)
	}
	baseline := first.SystemPrompt

	changed := candidate
	changed.ExactSystemPrompt = "BASE\nRUNTIME=2026-07-20"
	changed.Sources = []agentcore.ContextEpochSourceCandidate{
		{SourceKey: "system_prompt", Kind: agentcore.ContextContributionSystemPrompt, Content: "BASE", State: agentcore.ContextEpochSourceUpdated},
		{SourceKey: "runtime_context", Kind: agentcore.ContextContributionRuntimeContext, Content: "RUNTIME=2026-07-20", State: agentcore.ContextEpochSourceUpdated},
	}
	second, err := store.Project(context.Background(), changed)
	if err != nil {
		t.Fatalf("append runtime delta: %v", err)
	}
	if second.Receipt.EpochID != first.Receipt.EpochID || second.Receipt.State != "updated" {
		t.Fatalf("runtime delta unexpectedly rotated epoch: first=%+v second=%+v", first.Receipt, second.Receipt)
	}
	if !strings.HasPrefix(second.SystemPrompt, baseline) || !strings.Contains(second.SystemPrompt, "RUNTIME=2026-07-20") {
		t.Fatalf("baseline was not preserved with an appended delta: %q", second.SystemPrompt)
	}

	restarted := NewContextEpochStore()
	third, err := restarted.Project(context.Background(), changed)
	if err != nil {
		t.Fatalf("restart projection: %v", err)
	}
	if third.SystemPrompt != second.SystemPrompt || third.Receipt.EpochID != second.Receipt.EpochID {
		t.Fatalf("restart did not reuse exact epoch bytes\nwant=%q\n got=%q", second.SystemPrompt, third.SystemPrompt)
	}

	unavailable := changed
	unavailable.Sources = []agentcore.ContextEpochSourceCandidate{
		{SourceKey: "system_prompt", Kind: agentcore.ContextContributionSystemPrompt, Content: "BASE", State: agentcore.ContextEpochSourceUpdated},
		{SourceKey: "runtime_context", Kind: agentcore.ContextContributionRuntimeContext, State: agentcore.ContextEpochSourceUnavailable},
	}
	fourth, err := restarted.Project(context.Background(), unavailable)
	if err != nil {
		t.Fatalf("unavailable projection: %v", err)
	}
	if fourth.SystemPrompt != third.SystemPrompt {
		t.Fatalf("temporary unavailable source replaced the last admitted value")
	}

	dir := filepath.Join(activeSessionsDir(), safeTranscriptFileName(candidate.ThreadID), "context", "epochs")
	for _, path := range []string{filepath.Join(dir, "current.json"), filepath.Join(dir, first.Receipt.EpochID+".json")} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("epoch sidecar %s mode=%o, want 600", path, info.Mode().Perm())
		}
	}
}

func TestContextEpochThreeFullReplayToolTurnsKeepCanonicalPrefix(t *testing.T) {
	t.Setenv(awakenHomeEnv, t.TempDir())
	store := NewContextEpochStore()
	candidate := testContextEpochCandidate()
	cfg := llm.Config{
		APIKey: "test", ProviderID: candidate.ProviderID, ProfileID: candidate.ProfileID,
		CredentialRevision: candidate.CredentialRevision, BaseURL: candidate.NormalizedBaseURL,
		Protocol: llm.ProtocolOpenAIChat, Model: candidate.ModelID,
	}
	var messages []convcontext.Message
	var canonicalSystem json.RawMessage
	var canonicalTools json.RawMessage
	for turn := 1; turn <= 3; turn++ {
		projection, err := store.Project(context.Background(), candidate)
		if err != nil {
			t.Fatal(err)
		}
		messages = append(messages,
			convcontext.Message{Role: convcontext.RoleUser, Content: "turn"},
			convcontext.Message{Role: convcontext.RoleAssistant, ToolCalls: []convcontext.ToolCall{{
				ID: "call-" + string(rune('0'+turn)), Type: "function",
				Function: convcontext.FunctionCall{Name: "alpha", Arguments: `{}`},
			}}},
			convcontext.Message{Role: convcontext.RoleTool, ToolCallID: "call-" + string(rune('0'+turn)), Name: "alpha", Content: "ok"},
		)
		prepared, err := llm.PrepareRequest(cfg, llm.Request{
			SystemPrompt: projection.SystemPrompt, Messages: messages, Tools: projection.Tools,
			AffinityScopeID: candidate.ThreadID, Purpose: llm.RequestPurposeAgentTurn,
		}, true)
		if err != nil {
			t.Fatal(err)
		}
		var body map[string]json.RawMessage
		if err := json.Unmarshal(prepared.Body, &body); err != nil {
			t.Fatal(err)
		}
		var wireMessages []json.RawMessage
		if err := json.Unmarshal(body["messages"], &wireMessages); err != nil || len(wireMessages) == 0 {
			t.Fatalf("decode messages: %v body=%s", err, prepared.Body)
		}
		var firstRole struct {
			Role string `json:"role"`
		}
		if err := json.Unmarshal(wireMessages[0], &firstRole); err != nil || firstRole.Role != "system" {
			t.Fatalf("host instruction lost system authority: %s", wireMessages[0])
		}
		if turn == 1 {
			canonicalSystem = append([]byte(nil), wireMessages[0]...)
			canonicalTools = append([]byte(nil), body["tools"]...)
			continue
		}
		if string(canonicalSystem) != string(wireMessages[0]) || string(canonicalTools) != string(body["tools"]) {
			t.Fatalf("turn %d changed canonical provider prefix\nsystem=%s\ntools=%s", turn, wireMessages[0], body["tools"])
		}
	}
}

func TestContextEpochDynamicToolsAppendByFirstDiscoveryAndUnsafeChangesRotate(t *testing.T) {
	t.Setenv(awakenHomeEnv, t.TempDir())
	store := NewContextEpochStore()
	candidate := testContextEpochCandidate()
	first, err := store.Project(context.Background(), candidate)
	if err != nil {
		t.Fatal(err)
	}

	withNewDynamic := candidate
	withNewDynamic.Tools = append(append([]agentcore.ContextEpochToolCandidate(nil), candidate.Tools...), testEpochTool("mcp-alpha", tool.SourceMCP, "mcp-alpha-v1"))
	second, err := store.Project(context.Background(), withNewDynamic)
	if err != nil {
		t.Fatal(err)
	}
	if second.Receipt.EpochID != first.Receipt.EpochID {
		t.Fatalf("append-only dynamic tool rotated epoch: %+v", second.Receipt)
	}
	if got := contextEpochToolNames(second.Tools); strings.Join(got, ",") != "alpha,beta,mcp-zulu,mcp-alpha" {
		t.Fatalf("new dynamic tool did not retain first-discovery trailing order: %v", got)
	}

	removed := withNewDynamic
	removed.Tools = append([]agentcore.ContextEpochToolCandidate(nil), withNewDynamic.Tools[1:]...)
	third, err := store.Project(context.Background(), removed)
	if err != nil {
		t.Fatal(err)
	}
	if third.Receipt.EpochID == second.Receipt.EpochID || third.Receipt.BoundaryReason != "tool_removed" {
		t.Fatalf("tool removal did not create an explicit cold boundary: %+v", third.Receipt)
	}

	compacted := removed
	compacted.CompactBoundaryID = "compact-success-1"
	fourth, err := store.Project(context.Background(), compacted)
	if err != nil {
		t.Fatal(err)
	}
	if fourth.Receipt.EpochID == third.Receipt.EpochID || fourth.Receipt.BoundaryReason != "compact_succeeded" {
		t.Fatalf("successful compact did not rotate epoch: %+v", fourth.Receipt)
	}
}

func testContextEpochCandidate() agentcore.ContextEpochCandidate {
	return agentcore.ContextEpochCandidate{
		ThreadID: "thread-epoch", WorkspaceRoot: "/workspace", ProviderID: "kimi-coding-plan",
		ProfileID: "profile-1", CredentialRevision: 1, ModelID: "kimi-for-coding",
		ProtocolFamily: "openai_chat", EndpointContractID: "openai_chat_completions_v1",
		NormalizedBaseURL: "https://api.kimi.com/coding/v1", PromptSchemaVersion: "awaken_prompt_v1",
		ExactSystemPrompt: "BASE\nRUNTIME=2026-07-19",
		Sources: []agentcore.ContextEpochSourceCandidate{
			{SourceKey: "system_prompt", Kind: agentcore.ContextContributionSystemPrompt, Content: "BASE", State: agentcore.ContextEpochSourceUpdated},
			{SourceKey: "runtime_context", Kind: agentcore.ContextContributionRuntimeContext, Content: "RUNTIME=2026-07-19", State: agentcore.ContextEpochSourceUpdated},
		},
		Tools: []agentcore.ContextEpochToolCandidate{
			testEpochTool("mcp-zulu", tool.SourceMCP, "mcp-zulu-v1"),
			testEpochTool("beta", tool.SourceBuiltin, "builtin-beta-v1"),
			testEpochTool("alpha", tool.SourceBuiltin, "builtin-alpha-v1"),
		},
	}
}

func testEpochTool(name string, source tool.ToolSource, registrationID string) agentcore.ContextEpochToolCandidate {
	return agentcore.ContextEpochToolCandidate{
		Schema: tool.ToolSchema{Type: "function", Function: tool.ToolFunctionSchema{
			Name: name, Description: name, Parameters: tool.ToolParametersSchema{Type: "object"},
		}},
		Provenance: agentcore.ToolSchemaProvenance{Origin: source, RegistrationID: registrationID},
	}
}

func contextEpochToolNames(values []tool.ToolSchema) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, value.Function.Name)
	}
	return result
}
