package plugins

import (
	"testing"

	"awaken/src/plugins/ir"
)

type setupReadinessFixture map[string]SetupStatus

func (f setupReadinessFixture) SetupStatus(_ string, requirement ir.SetupRequirement) (SetupStatus, string) {
	if status := f[requirement.ID]; status != "" {
		return status, "fixture"
	}
	return SetupReady, ""
}

type countingSetupReadiness struct {
	status SetupStatus
	calls  int
}

func (f *countingSetupReadiness) SetupStatus(_ string, _ ir.SetupRequirement) (SetupStatus, string) {
	f.calls++
	return f.status, ""
}

func TestComponentStatesResolveEachSetupRequirementOnce(t *testing.T) {
	provider := &countingSetupReadiness{status: SetupReady}
	store := &Store{setupReadiness: provider}
	requirement := ir.SetupRequirement{
		ID: "shared-account", Kind: ir.SetupHostConnector, Required: true,
		ComponentIDs: []string{"skill/one", "skill/two"},
	}
	components := []ir.Component{
		{LogicalID: "skill/one", Kind: ir.KindSkill, Mapping: ir.MappingNative, Validation: "valid"},
		{LogicalID: "skill/two", Kind: ir.KindSkill, Mapping: ir.MappingNative, Validation: "valid"},
	}
	states, _, _, _ := store.componentStates("fixture", components, []ir.SetupRequirement{requirement}, Record{Enabled: true})
	if len(states) != 2 || !states[0].Switchable || !states[1].Switchable {
		t.Fatalf("states=%+v", states)
	}
	if provider.calls != 1 {
		t.Fatalf("setup readiness calls=%d, want 1", provider.calls)
	}
}

func TestSetupDependenciesFailClosedWithoutBlockingIndependentComponents(t *testing.T) {
	mcp := ir.Component{LogicalID: "mcp_server/mail", Kind: ir.KindMCPServer, Mapping: ir.MappingAdapted, Validation: "valid", MCP: &ir.MCPSpec{ShortName: "mail", Transport: "streamable-http"}}
	hook := ir.Component{LogicalID: "hook/notify", Kind: ir.KindHook, Mapping: ir.MappingAdapted, Validation: "valid", Hook: &ir.HookSpec{HandlerType: "http"}}
	skill := ir.Component{LogicalID: "skill/help", Kind: ir.KindSkill, Mapping: ir.MappingNative, Validation: "valid", Skill: &ir.SkillSpec{Path: "skills/help"}}
	field := &ir.VariableSpec{Key: "API_KEY", Type: "string", Required: true, Secret: true}
	setup := []ir.SetupRequirement{
		{ID: "api-key", Kind: ir.SetupSecret, Required: true, Field: field, ComponentIDs: []string{mcp.LogicalID}},
		{ID: "notify-account", Kind: ir.SetupOAuthPKCE, Required: true, ComponentIDs: []string{hook.LogicalID}},
	}
	store := &Store{setupReadiness: setupReadinessFixture{"notify-account": SetupNeedsAuth}}
	states, configuration, authentication, _ := store.componentStates("mail-plugin", []ir.Component{mcp, hook, skill}, setup, Record{Enabled: true})
	byID := map[string]ComponentState{}
	for _, state := range states {
		byID[state.Component.LogicalID] = state
	}
	if byID[mcp.LogicalID].Configuration != "needs_configuration" || byID[mcp.LogicalID].Switchable {
		t.Fatalf("missing secret did not block MCP: %+v", byID[mcp.LogicalID])
	}
	if byID[hook.LogicalID].Authentication != "needs_auth" || byID[hook.LogicalID].Switchable {
		t.Fatalf("missing OAuth did not block Hook: %+v", byID[hook.LogicalID])
	}
	if !byID[skill.LogicalID].Switchable {
		t.Fatalf("independent Skill should remain usable: %+v", byID[skill.LogicalID])
	}
	if configuration != "needs_configuration" || authentication != "needs_auth" {
		t.Fatalf("unexpected aggregate states: configuration=%s authentication=%s", configuration, authentication)
	}
}

func TestAppSetupAlwaysRemainsRecognizedUnsupported(t *testing.T) {
	app := ir.Component{LogicalID: "app/outlook", Kind: ir.KindApp, Mapping: ir.MappingRecognizedUnsupported, Validation: "valid", App: &ir.AppSpec{ConnectorID: "connector_openai_outlook"}}
	requirement := ir.SetupRequirement{ID: "app:outlook", Kind: ir.SetupAppConnector, Required: true, ComponentIDs: []string{app.LogicalID}, ConnectorID: app.App.ConnectorID}
	store := &Store{setupReadiness: setupReadinessFixture{requirement.ID: SetupReady}}
	states, _, authentication, compatibility := store.componentStates("outlook", []ir.Component{app}, []ir.SetupRequirement{requirement}, Record{Enabled: true})
	if len(states) != 1 || states[0].Authentication != "unsupported" || states[0].Switchable {
		t.Fatalf("App connector became runnable: %+v", states)
	}
	if authentication != "unsupported" || compatibility != "partial" {
		t.Fatalf("unexpected aggregate states: auth=%s compatibility=%s", authentication, compatibility)
	}
}

func TestPendingAdminApprovalIsDistinctAndRetryable(t *testing.T) {
	requirement := ir.SetupRequirement{ID: "account", Kind: ir.SetupOAuthPKCE, Required: true}
	items := []SetupItem{{Requirement: requirement, Status: SetupPendingAdminApproval, Actions: setupActions(SetupPendingAdminApproval, requirement.Kind)}}
	summary := SummarizeSetup(items)
	if summary.NeedsAdminApproval != 1 || summary.InProgress != 0 || summary.NeedsAuth != 0 {
		t.Fatalf("summary=%+v", summary)
	}
	if len(items[0].Actions) != 1 || items[0].Actions[0] != SetupActionRetry {
		t.Fatalf("actions=%v", items[0].Actions)
	}
	component := ir.Component{LogicalID: "mcp", Kind: ir.KindMCPServer, Validation: "valid", Mapping: ir.MappingNative}
	requirement.ComponentIDs = []string{component.LogicalID}
	store := &Store{setupReadiness: setupReadinessFixture{"account": SetupPendingAdminApproval}}
	states, _, authentication, _ := store.componentStates("mail", []ir.Component{component}, []ir.SetupRequirement{requirement}, Record{Enabled: true})
	if authentication != string(SetupPendingAdminApproval) || states[0].Switchable {
		t.Fatalf("authentication=%s state=%+v", authentication, states[0])
	}
}

func TestHostConnectorMCPIsInspectableButNotIndependentlySwitchable(t *testing.T) {
	component := ir.Component{
		LogicalID: "mcp_server/.awaken/setup.json#host_connector:outlook", Kind: ir.KindMCPServer,
		Mapping: ir.MappingAdapted, Validation: "valid",
		MCP: &ir.MCPSpec{ShortName: "awaken-outlook", HostConnectorID: "awaken.microsoft.outlook", Transport: "in-process"},
	}
	requirement := ir.SetupRequirement{
		ID: "outlook", Kind: ir.SetupHostConnector, Required: true,
		ConnectorID: "awaken.microsoft.outlook", ComponentIDs: []string{component.LogicalID},
	}
	store := &Store{setupReadiness: setupReadinessFixture{"outlook": SetupReady}}
	states, _, _, _ := store.componentStates("outlook", []ir.Component{component}, []ir.SetupRequirement{requirement}, Record{Enabled: true})
	if len(states) != 1 || states[0].Authentication != "ready" || states[0].Switchable {
		t.Fatalf("host connector MCP state = %+v", states)
	}
}
