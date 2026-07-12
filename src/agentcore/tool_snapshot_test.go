package agentcore

import (
	"context"
	"strings"
	"testing"

	convcontext "awaken/src/context"
	"awaken/src/tool"
)

type snapshotTestTool struct {
	name                string
	version             string
	readiness           tool.ProviderReadiness
	calls               *int
	observedEnvironment *string
}

type providerSwitchTestTool struct {
	name       string
	providerID tool.ProviderID
	calls      *int
	outcome    tool.ExecutionOutcome
}

func (t *providerSwitchTestTool) Name() string        { return t.name }
func (t *providerSwitchTestTool) Description() string { return "provider switch side-effect test" }
func (t *providerSwitchTestTool) Parameters() []tool.ToolParameter {
	return []tool.ToolParameter{{Name: "path", Type: "string", Required: true}}
}
func (t *providerSwitchTestTool) Definition() tool.ToolDefinition {
	return tool.ToolDefinition{
		Name: t.name, DisplayName: t.name, Source: tool.SourcePlugin,
		Description: t.Description(), ProviderID: t.providerID,
		SideEffects:  []tool.ToolSideEffect{{Kind: tool.SideEffectUpdate, Description: "May update the shared document target.", Reversibility: tool.ReversibilityConditional}},
		Annotations:  tool.ToolAnnotations{ReadOnly: false, CapabilityClass: tool.CapabilityArtifactWrite, TrustedOrigin: tool.TrustedOriginBuiltin},
		Capabilities: []tool.Capability{tool.CapabilityDocumentCreate},
		Policy:       tool.PolicyAlwaysAllow,
	}
}
func (t *providerSwitchTestTool) ProviderManifest() tool.ProviderManifest {
	return tool.ProviderManifest{
		ProviderID: t.providerID, ToolName: t.name, Source: tool.SourcePlugin,
		Readiness: tool.ProviderReady, Capabilities: []tool.Capability{tool.CapabilityDocumentCreate},
	}
}
func (t *providerSwitchTestTool) Execute(ctx context.Context, input string) (string, error) {
	outcome, err := t.ExecuteOutcome(ctx, input, nil)
	return outcome.Content, err
}
func (t *providerSwitchTestTool) ExecuteOutcome(context.Context, string, tool.ToolObserver) (tool.ExecutionOutcome, error) {
	if t.calls != nil {
		*t.calls++
	}
	return t.outcome, nil
}

func (t *snapshotTestTool) Name() string        { return t.name }
func (t *snapshotTestTool) Description() string { return "snapshot " + t.version }
func (t *snapshotTestTool) Parameters() []tool.ToolParameter {
	return []tool.ToolParameter{{Name: "value", Type: "string"}}
}
func (t *snapshotTestTool) Definition() tool.ToolDefinition {
	return tool.ToolDefinition{
		Name: t.name, DisplayName: t.name, Source: tool.SourcePlugin,
		Description: "snapshot " + t.version, Purpose: "verify pinned run snapshot " + t.version,
		ProviderID:  tool.ProviderID("test:" + t.name + ":" + t.version),
		SideEffects: []tool.ToolSideEffect{{Kind: tool.SideEffectRead, Description: "test read", Reversibility: tool.ReversibilityNotApplicable}},
		Annotations: tool.ToolAnnotations{ReadOnly: true, CapabilityClass: tool.CapabilityArtifactRead, TrustedOrigin: tool.TrustedOriginBuiltin},
		Policy:      tool.PolicyAlwaysAllow,
	}
}
func (t *snapshotTestTool) ProviderManifest() tool.ProviderManifest {
	return tool.ProviderManifest{
		ProviderID: tool.ProviderID("test:" + t.name + ":" + t.version),
		ToolName:   t.name, Source: tool.SourcePlugin, Readiness: t.readiness,
		Capabilities: []tool.Capability{tool.CapabilityFileRead},
	}
}
func (t *snapshotTestTool) Execute(ctx context.Context, _ string) (string, error) {
	if t.calls != nil {
		*t.calls++
	}
	if t.observedEnvironment != nil {
		if environment, ok := tool.ExecutionEnvironmentSnapshotFromContext(ctx); ok {
			*t.observedEnvironment = environment.SnapshotID
		}
	}
	return t.version, nil
}

func TestRunToolSnapshotPinsDefinitionAndExecutable(t *testing.T) {
	name := "snapshot_pin_tool"
	oldCalls, newCalls := 0, 0
	observedEnvironment := ""
	oldTool := &snapshotTestTool{name: name, version: "v1", readiness: tool.ProviderReady, calls: &oldCalls, observedEnvironment: &observedEnvironment}
	if err := tool.RegisterToolForce(oldTool); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { tool.UnregisterTool(name) })

	environment := tool.ProbeExecutionEnvironment(t.Context(), tool.ExecutionEnvironmentProbeOptions{
		Executables: []string{}, PackageManagers: []string{}, PATH: "/usr/bin", PATHSource: "test",
	})
	pinned := tool.NewRunToolAvailabilitySnapshot(environment)
	newTool := &snapshotTestTool{name: name, version: "v2", readiness: tool.ProviderReady, calls: &newCalls}
	if err := tool.RegisterToolForce(newTool); err != nil {
		t.Fatal(err)
	}

	request := NewRequestBuilder(AgentConfig{
		ToolAvailabilitySnapshot: &pinned,
		ToolExposure:             tool.ToolExposurePolicy{Mode: tool.ExposureAll, ExtraHidden: allRegisteredToolNamesExcept(name)},
	}).Build(convcontext.NewConversation(nil).Context, nil)
	if len(request.Tools) != 1 || !strings.Contains(request.Tools[0].Function.Description, "v1") {
		t.Fatalf("request drifted from pinned definition: %+v", request.Tools)
	}

	result := NewToolRuntimeWithConfig(nil, ToolRuntimeConfig{
		ToolAvailabilitySnapshot:     &pinned,
		ExecutionEnvironmentSnapshot: &environment,
	}).Execute(t.Context(), convcontext.ToolCall{ID: "call_pin", Function: convcontext.FunctionCall{Name: name, Arguments: `{}`}})
	if result.IsError || result.Content != "v1" || oldCalls != 1 || newCalls != 0 {
		t.Fatalf("execution did not use pinned runtime: result=%+v old=%d new=%d", result, oldCalls, newCalls)
	}
	if observedEnvironment != environment.SnapshotID {
		t.Fatalf("execution re-probed or lost the pinned environment: got %q want %q", observedEnvironment, environment.SnapshotID)
	}
	if result.Outcome.Diagnostic["tool_snapshot_id"] != pinned.SnapshotID || result.Outcome.Diagnostic["environment_snapshot_id"] != environment.SnapshotID {
		t.Fatalf("snapshot identities missing from outcome: %+v", result.Outcome.Diagnostic)
	}
	if result.Outcome.Diagnostic["provider_id"] != tool.ProviderID("test:"+name+":v1") || result.Outcome.Diagnostic["provider_readiness"] != tool.ProviderReady {
		t.Fatalf("selected provider facts missing from outcome: %+v", result.Outcome.Diagnostic)
	}
}

func TestToolRuntimeClonesConfiguredRunSnapshots(t *testing.T) {
	name := "snapshot_runtime_clone_tool"
	observedEnvironment := ""
	runtimeTool := &snapshotTestTool{name: name, version: "pinned", readiness: tool.ProviderReady, observedEnvironment: &observedEnvironment}
	if err := tool.RegisterToolForce(runtimeTool); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { tool.UnregisterTool(name) })
	environment := tool.ExecutionEnvironmentSnapshot{
		SnapshotID: "env_runtime_clone", OS: "test", Architecture: "test", Confidence: "observed",
		PATH: []string{"/safe/bin"}, Executables: []tool.ExecutableObservation{{Name: "safe", Path: "/safe/bin/safe", Provenance: "test"}},
	}
	snapshot := tool.NewRunToolAvailabilitySnapshot(environment)
	expectedToolSnapshotID, expectedEnvironmentID := snapshot.SnapshotID, environment.SnapshotID
	runtime := NewToolRuntimeWithConfig(nil, ToolRuntimeConfig{
		ToolAvailabilitySnapshot: &snapshot, ExecutionEnvironmentSnapshot: &environment,
	})

	// Mutate every caller-owned top-level/slice handle after construction.
	snapshot.SnapshotID = "tools_tampered"
	environment.SnapshotID = "env_tampered"
	environment.PATH[0] = "/attacker/bin"
	environment.Executables[0].Path = "/attacker/bin/safe"

	result := runtime.Execute(t.Context(), convcontext.ToolCall{ID: "clone", Function: convcontext.FunctionCall{Name: name, Arguments: `{}`}})
	if result.IsError || result.ToolSnapshotID != expectedToolSnapshotID || result.EnvironmentSnapshotID != expectedEnvironmentID || observedEnvironment != expectedEnvironmentID {
		t.Fatalf("caller mutation rewrote runtime snapshot: result=%+v observed_env=%q", result, observedEnvironment)
	}
}

func TestPinnedProviderReadinessReturnsStructuredRecoveryWithoutExecution(t *testing.T) {
	name := "snapshot_needs_auth_tool"
	calls := 0
	runtime := &snapshotTestTool{name: name, version: "auth", readiness: tool.ProviderNeedsAuth, calls: &calls}
	if err := tool.RegisterToolForce(runtime); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { tool.UnregisterTool(name) })
	environment := tool.ProbeExecutionEnvironment(t.Context(), tool.ExecutionEnvironmentProbeOptions{
		Executables: []string{}, PackageManagers: []string{"npm"}, PATH: "/usr/bin", PATHSource: "test",
		LookPath: func(value string) (string, error) {
			if value == "npm" {
				return "/usr/bin/npm", nil
			}
			return "", tool.ErrToolNotFound
		},
	})
	snapshot := tool.NewRunToolAvailabilitySnapshot(environment)
	result := NewToolRuntimeWithConfig(nil, ToolRuntimeConfig{
		ToolAvailabilitySnapshot:     &snapshot,
		ExecutionEnvironmentSnapshot: &environment,
	}).Execute(t.Context(), convcontext.ToolCall{ID: "call_auth", Function: convcontext.FunctionCall{Name: name, Arguments: `{}`}})
	if result.Outcome.Status != tool.OutcomeNeedsAuth || result.Outcome.ErrorCode != tool.ErrorCodeNeedsAuth {
		t.Fatalf("outcome=%+v", result.Outcome)
	}
	if calls != 0 {
		t.Fatalf("unready provider executed %d time(s)", calls)
	}
	if result.Outcome.Details["provider_id"] == "" || len(result.Outcome.NextActions) == 0 {
		t.Fatalf("missing structured recovery facts: %+v", result.Outcome)
	}
}

func TestPartialSideEffectNeverAutoSwitchesToAlternativeProvider(t *testing.T) {
	providerACalls, providerBCalls := 0, 0
	providerA := &providerSwitchTestTool{
		name: "provider_switch_a", providerID: "test:provider-switch:a", calls: &providerACalls,
		outcome: tool.ExecutionOutcome{
			Content: "provider A failed after a partial update", Status: tool.OutcomeFailed,
			TransportStatus: tool.TransportCompleted, SemanticStatus: tool.SemanticFailure,
			ProcessStatus: tool.ProcessNotApplicable, ErrorCode: "provider_partial_failure",
			ErrorCategory: tool.ErrorCategoryDomain, Retryable: false,
			Operation: &tool.OperationDescriptor{Type: "document.create", Target: "report.docx", SideEffectState: tool.OutcomeSideEffectPartial},
			Changes:   []tool.ResourceChange{{Kind: "file", Action: "updated", Resource: "report.docx", Certainty: tool.ChangeObserved, Reversibility: tool.ChangeConditional}},
		},
	}
	providerB := &providerSwitchTestTool{
		name: "provider_switch_b", providerID: "test:provider-switch:b", calls: &providerBCalls,
		outcome: tool.ExecutionOutcome{
			Content: "provider B completed", Status: tool.OutcomeSucceeded,
			TransportStatus: tool.TransportCompleted, SemanticStatus: tool.SemanticSuccess,
			ProcessStatus: tool.ProcessNotApplicable, Retryable: false,
			Operation: &tool.OperationDescriptor{Type: "document.create", Target: "report.docx", SideEffectState: tool.OutcomeSideEffectApplied},
		},
	}
	for _, candidate := range []tool.Tool{providerA, providerB} {
		if err := tool.RegisterToolForce(candidate); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { tool.UnregisterTool(candidate.Name()) })
	}

	environment := tool.ProbeExecutionEnvironment(t.Context(), tool.ExecutionEnvironmentProbeOptions{
		Executables: []string{}, PackageManagers: []string{}, PATH: "/usr/bin", PATHSource: "test",
	})
	snapshot := tool.NewRunToolAvailabilitySnapshot(environment)
	runtime := NewToolRuntimeWithConfig(nil, ToolRuntimeConfig{
		ToolAvailabilitySnapshot: &snapshot, ExecutionEnvironmentSnapshot: &environment,
	})
	arguments := `{"path":"report.docx"}`
	failed := runtime.Execute(t.Context(), convcontext.ToolCall{ID: "provider_a", Function: convcontext.FunctionCall{Name: providerA.Name(), Arguments: arguments}})
	if failed.Outcome.Status != tool.OutcomeFailed || failed.Outcome.Operation == nil || failed.Outcome.Operation.SideEffectState != tool.OutcomeSideEffectPartial {
		t.Fatalf("provider A partial truth was lost: %+v", failed.Outcome)
	}
	if providerACalls != 1 || providerBCalls != 0 {
		t.Fatalf("runtime silently switched providers after partial side effect: A=%d B=%d", providerACalls, providerBCalls)
	}

	succeeded := runtime.Execute(t.Context(), convcontext.ToolCall{ID: "provider_b", Function: convcontext.FunctionCall{Name: providerB.Name(), Arguments: arguments}})
	if succeeded.IsError || providerACalls != 1 || providerBCalls != 1 {
		t.Fatalf("an explicit new provider call did not remain a distinct execution: result=%+v A=%d B=%d", succeeded, providerACalls, providerBCalls)
	}
}

func allRegisteredToolNamesExcept(keep string) []string {
	names := tool.ListToolNames()
	result := make([]string, 0, len(names)-1)
	for _, name := range names {
		if name != keep {
			result = append(result, name)
		}
	}
	return result
}
