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
	if first.Receipt.CreationBoundaryReason != "initial_baseline" {
		t.Fatalf("initial epoch lost its durable creation boundary: %+v", first.Receipt)
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
	if third.Receipt.CreationBoundaryReason != "initial_baseline" {
		t.Fatalf("restart lost the stable epoch creation boundary: %+v", third.Receipt)
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

func TestContextEpochAppendOnlyDeltaReasonCarriesChangedSegments(t *testing.T) {
	t.Setenv(awakenHomeEnv, t.TempDir())
	store := NewContextEpochStore()
	candidate := testContextEpochCandidate()
	candidate.Sources = []agentcore.ContextEpochSourceCandidate{
		{SourceKey: "system_prompt", Kind: agentcore.ContextContributionSystemPrompt, Content: "BASE", State: agentcore.ContextEpochSourceUpdated},
		{SourceKey: "runtime_context", Kind: agentcore.ContextContributionRuntimeContext, Content: "RUNTIME=v1", State: agentcore.ContextEpochSourceUpdated},
	}
	if _, err := store.Project(context.Background(), candidate); err != nil {
		t.Fatalf("initial projection: %v", err)
	}

	// Append a runtime_context delta and a new skills source. Neither rotates
	// the epoch; both must show up in the BoundaryReason so a cache miss can
	// be diagnosed to the specific trailing segments that changed.
	changed := candidate
	changed.Sources = []agentcore.ContextEpochSourceCandidate{
		{SourceKey: "system_prompt", Kind: agentcore.ContextContributionSystemPrompt, Content: "BASE", State: agentcore.ContextEpochSourceUpdated},
		{SourceKey: "runtime_context", Kind: agentcore.ContextContributionRuntimeContext, Content: "RUNTIME=v2", State: agentcore.ContextEpochSourceUpdated},
		{SourceKey: "skills", Kind: agentcore.ContextContributionSkills, Content: "skill catalog v2", State: agentcore.ContextEpochSourceUpdated},
	}
	second, err := store.Project(context.Background(), changed)
	if err != nil {
		t.Fatalf("append delta projection: %v", err)
	}
	if second.Receipt.State != "updated" {
		t.Fatalf("expected append-only update state, got %+v", second.Receipt)
	}
	if !strings.HasPrefix(second.Receipt.BoundaryReason, "append_only_host_delta:") {
		t.Fatalf("expected reason to carry changed segment labels, got %q", second.Receipt.BoundaryReason)
	}
	for _, segment := range []string{"runtime_context", "skills"} {
		if !strings.Contains(second.Receipt.BoundaryReason, segment) {
			t.Fatalf("expected reason to mention %q, got %q", segment, second.Receipt.BoundaryReason)
		}
	}
}

func TestContextEpochLateAddedSourcesPersistEmptyBaselineDigestAndSurviveRestart(t *testing.T) {
	t.Setenv(awakenHomeEnv, t.TempDir())
	store := NewContextEpochStore()
	candidate := testContextEpochCandidate()
	if _, err := store.Project(context.Background(), candidate); err != nil {
		t.Fatalf("initial projection: %v", err)
	}

	lateSources := []agentcore.ContextEpochSourceCandidate{
		{SourceKey: "skill_activation_excel", Kind: agentcore.ContextContributionSkills, Content: "excel skill", State: agentcore.ContextEpochSourceUpdated},
		{SourceKey: "current_run_plan", Kind: agentcore.ContextContributionOther, Content: "plan", State: agentcore.ContextEpochSourceUpdated},
		{SourceKey: "hook_context_before_model_0", Kind: agentcore.ContextContributionOther, Content: "hook context", State: agentcore.ContextEpochSourceUpdated},
		{SourceKey: "runtime_recovery", Kind: agentcore.ContextContributionOther, Content: "recovery", State: agentcore.ContextEpochSourceUpdated},
		{SourceKey: "composer_skill_selection", Kind: agentcore.ContextContributionSkills, Content: "selected skill", State: agentcore.ContextEpochSourceUpdated},
	}
	changed := candidate
	changed.Sources = append(append([]agentcore.ContextEpochSourceCandidate(nil), candidate.Sources...), lateSources...)
	second, err := store.Project(context.Background(), changed)
	if err != nil {
		t.Fatalf("append late sources: %v", err)
	}
	if second.Receipt.State != "updated" {
		t.Fatalf("late sources did not update the epoch: %+v", second.Receipt)
	}

	raw := readRawCurrentContextEpoch(t, candidate.ThreadID)
	emptyDigest := agentcore.ContextEpochSourceDigest("")
	for _, late := range lateSources {
		var found *contextEpochSourceSnapshot
		for index := range raw.SourceSnapshots {
			if raw.SourceSnapshots[index].SourceKey == late.SourceKey {
				found = &raw.SourceSnapshots[index]
				break
			}
		}
		if found == nil {
			t.Fatalf("late source %q was not persisted", late.SourceKey)
		}
		if found.BaselineValue != "" || found.BaselineDigest != emptyDigest {
			t.Fatalf("late source %q baseline=(%q,%q), want explicit empty baseline", late.SourceKey, found.BaselineValue, found.BaselineDigest)
		}
		if found.Value != late.Content || found.ValueDigest != agentcore.ContextEpochSourceDigest(late.Content) {
			t.Fatalf("late source %q current value/digest mismatch: %+v", late.SourceKey, *found)
		}
	}

	restarted := NewContextEpochStore()
	third, err := restarted.Project(context.Background(), changed)
	if err != nil {
		t.Fatalf("restart projection after late sources: %v", err)
	}
	if third.Receipt.EpochID != second.Receipt.EpochID || third.SystemPrompt != second.SystemPrompt {
		t.Fatalf("restart did not preserve the late-source epoch: second=%+v third=%+v", second.Receipt, third.Receipt)
	}
}

func TestContextEpochNormalizesOnlyKnownMissingLateSourceBaselineDigest(t *testing.T) {
	t.Setenv(awakenHomeEnv, t.TempDir())
	store := NewContextEpochStore()
	candidate := testContextEpochCandidate()
	if _, err := store.Project(context.Background(), candidate); err != nil {
		t.Fatalf("initial projection: %v", err)
	}
	changed := candidate
	changed.Sources = append(append([]agentcore.ContextEpochSourceCandidate(nil), candidate.Sources...), agentcore.ContextEpochSourceCandidate{
		SourceKey: "skill_activation_excel", Kind: agentcore.ContextContributionSkills,
		Content: "excel skill", State: agentcore.ContextEpochSourceUpdated,
	})
	if _, err := store.Project(context.Background(), changed); err != nil {
		t.Fatalf("append late source: %v", err)
	}

	raw := readRawCurrentContextEpoch(t, candidate.ThreadID)
	for index := range raw.SourceSnapshots {
		if raw.SourceSnapshots[index].SourceKey == "skill_activation_excel" {
			raw.SourceSnapshots[index].BaselineDigest = ""
			raw.SourceSnapshots[index].BaselineValue = ""
		}
	}
	writeRawContextEpoch(t, candidate.ThreadID, raw)

	restarted := NewContextEpochStore()
	if _, err := restarted.Project(context.Background(), changed); err != nil {
		t.Fatalf("known late-source writer shape was not normalized: %v", err)
	}
	normalized := readRawCurrentContextEpoch(t, candidate.ThreadID)
	for _, source := range normalized.SourceSnapshots {
		if source.SourceKey == "skill_activation_excel" && source.BaselineDigest != agentcore.ContextEpochSourceDigest("") {
			t.Fatalf("known late-source baseline digest was not durably normalized: %+v", source)
		}
	}
}

func TestContextEpochRejectsLateSourceNormalizationWhenDeltaDoesNotMatch(t *testing.T) {
	t.Setenv(awakenHomeEnv, t.TempDir())
	store := NewContextEpochStore()
	candidate := testContextEpochCandidate()
	if _, err := store.Project(context.Background(), candidate); err != nil {
		t.Fatalf("initial projection: %v", err)
	}
	changed := candidate
	changed.Sources = append(append([]agentcore.ContextEpochSourceCandidate(nil), candidate.Sources...), agentcore.ContextEpochSourceCandidate{
		SourceKey: "skill_activation_excel", Kind: agentcore.ContextContributionSkills,
		Content: "excel skill", State: agentcore.ContextEpochSourceUpdated,
	})
	if _, err := store.Project(context.Background(), changed); err != nil {
		t.Fatalf("append late source: %v", err)
	}

	raw := readRawCurrentContextEpoch(t, candidate.ThreadID)
	for index := range raw.SourceSnapshots {
		if raw.SourceSnapshots[index].SourceKey == "skill_activation_excel" {
			raw.SourceSnapshots[index].BaselineDigest = ""
		}
	}
	for index := range raw.Deltas {
		if raw.Deltas[index].SourceKey == "skill_activation_excel" {
			raw.Deltas[index].Content += "\ntampered"
		}
	}
	writeRawContextEpoch(t, candidate.ThreadID, raw)
	before, err := os.ReadFile(currentContextEpochPath(t, candidate.ThreadID))
	if err != nil {
		t.Fatal(err)
	}

	_, err = NewContextEpochStore().Project(context.Background(), changed)
	if err == nil || !strings.Contains(err.Error(), "context epoch source digest mismatch") {
		t.Fatalf("tampered late-source delta must remain fail-stop, got %v", err)
	}
	after, readErr := os.ReadFile(currentContextEpochPath(t, candidate.ThreadID))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(after) != string(before) {
		t.Fatal("failed normalization rewrote the source sidecar")
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
	if third.Receipt.CreationBoundaryReason != "tool_removed" {
		t.Fatalf("tool removal boundary was not retained on the new epoch: %+v", third.Receipt)
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

func TestContextEpochIgnoresHostOnlyToolRegistrationIdentity(t *testing.T) {
	t.Setenv(awakenHomeEnv, t.TempDir())
	store := NewContextEpochStore()
	candidate := testContextEpochCandidate()
	first, err := store.Project(context.Background(), candidate)
	if err != nil {
		t.Fatal(err)
	}

	changed := candidate
	changed.Tools = append([]agentcore.ContextEpochToolCandidate(nil), candidate.Tools...)
	for index := range changed.Tools {
		changed.Tools[index].Provenance.RegistrationID = "reconnected-registration-" + changed.Tools[index].Schema.Function.Name
		if agentcore.ContextEpochToolDigest(changed.Tools[index]) != agentcore.ContextEpochToolDigest(candidate.Tools[index]) {
			t.Fatalf("registration-only change affected tool digest for %q", changed.Tools[index].Schema.Function.Name)
		}
	}
	second, err := store.Project(context.Background(), changed)
	if err != nil {
		t.Fatal(err)
	}
	if second.Receipt.EpochID != first.Receipt.EpochID || second.Receipt.State != "unchanged" || second.Receipt.BoundaryReason != "" {
		t.Fatalf("registration-only reconnect changed provider epoch: first=%+v second=%+v", first.Receipt, second.Receipt)
	}
	if got := contextEpochToolNames(second.Tools); strings.Join(got, ",") != "alpha,beta,mcp-zulu" {
		t.Fatalf("registration-only reconnect changed canonical tool order: %v", got)
	}
}

func TestContextEpochReasoningPolicyChangeIsExplicitColdBoundary(t *testing.T) {
	t.Setenv(awakenHomeEnv, t.TempDir())
	store := NewContextEpochStore()
	candidate := testContextEpochCandidate()
	first, err := store.Project(context.Background(), candidate)
	if err != nil {
		t.Fatal(err)
	}

	changed := candidate
	changed.ReasoningPolicy = "max"
	second, err := store.Project(context.Background(), changed)
	if err != nil {
		t.Fatal(err)
	}
	if second.Receipt.EpochID == first.Receipt.EpochID || second.Receipt.State != "rotated" || second.Receipt.BoundaryReason != "reasoning_policy_changed" {
		t.Fatalf("reasoning policy change did not create explicit cold boundary: first=%+v second=%+v", first.Receipt, second.Receipt)
	}
	if second.Receipt.CreationBoundaryReason != "reasoning_policy_changed" {
		t.Fatalf("reasoning cold boundary was not retained: %+v", second.Receipt)
	}
}

func TestContextEpochV1SidecarMigratesThroughSafeColdBoundary(t *testing.T) {
	t.Setenv(awakenHomeEnv, t.TempDir())
	store := NewContextEpochStore()
	candidate := testContextEpochCandidate()
	first, err := store.Project(context.Background(), candidate)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(activeSessionsDir(), safeTranscriptFileName(candidate.ThreadID), "context", "epochs")
	epoch, err := readCurrentContextEpoch(dir)
	if err != nil {
		t.Fatal(err)
	}
	epoch.SchemaVersion = 1
	encodedEpoch, err := json.MarshalIndent(epoch, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, epoch.EpochID+".json"), append(encodedEpoch, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	encodedPointer, err := json.MarshalIndent(contextEpochPointer{SchemaVersion: 1, EpochID: epoch.EpochID}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "current.json"), append(encodedPointer, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}

	migrated, err := store.Project(context.Background(), candidate)
	if err != nil {
		t.Fatalf("v1 migration failed: %v", err)
	}
	if migrated.Receipt.EpochID == first.Receipt.EpochID || migrated.Receipt.BoundaryReason != "prompt_schema_incompatible" {
		t.Fatalf("v1 epoch was not safely rotated: first=%+v migrated=%+v", first.Receipt, migrated.Receipt)
	}
	current, err := readCurrentContextEpoch(dir)
	if err != nil {
		t.Fatal(err)
	}
	if current.SchemaVersion != agentcore.ContextEpochSchemaVersion || current.ReasoningPolicy != candidate.ReasoningPolicy {
		t.Fatalf("migration did not persist current schema/identity: %+v", current)
	}
}

func testContextEpochCandidate() agentcore.ContextEpochCandidate {
	return agentcore.ContextEpochCandidate{
		ThreadID: "thread-epoch", WorkspaceRoot: "/workspace", ProviderID: "kimi-coding-plan",
		ProfileID: "profile-1", CredentialRevision: 1, ModelID: "kimi-for-coding",
		ProtocolFamily: "openai_chat", EndpointContractID: "openai_chat_completions_v1",
		NormalizedBaseURL: "https://api.kimi.com/coding/v1", ReasoningPolicy: "high", PromptSchemaVersion: "awaken_prompt_v1",
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

func readRawCurrentContextEpoch(t *testing.T, threadID string) contextEpochSidecar {
	t.Helper()
	dir := filepath.Join(activeSessionsDir(), safeTranscriptFileName(threadID), "context", "epochs")
	var pointer contextEpochPointer
	pointerBytes, err := os.ReadFile(filepath.Join(dir, "current.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(pointerBytes, &pointer); err != nil {
		t.Fatal(err)
	}
	var epoch contextEpochSidecar
	epochBytes, err := os.ReadFile(filepath.Join(dir, pointer.EpochID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(epochBytes, &epoch); err != nil {
		t.Fatal(err)
	}
	return epoch
}

func writeRawContextEpoch(t *testing.T, threadID string, epoch contextEpochSidecar) {
	t.Helper()
	encoded, err := json.MarshalIndent(epoch, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(currentContextEpochPath(t, threadID), append(encoded, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

func currentContextEpochPath(t *testing.T, threadID string) string {
	t.Helper()
	dir := filepath.Join(activeSessionsDir(), safeTranscriptFileName(threadID), "context", "epochs")
	var pointer contextEpochPointer
	pointerBytes, err := os.ReadFile(filepath.Join(dir, "current.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(pointerBytes, &pointer); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(dir, pointer.EpochID+".json")
}
