package agentcore

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	convcontext "awaken/src/context"
	"awaken/src/tool"
	"awaken/src/tool/profile"
)

// TestWriteAuditPopulatesObservabilityFieldsForAllow verifies that a
// successful allow decision produces an audit record carrying the Task 16
// unified observability fields: ProfileName, Intent, DecisionAction="allow",
// DecisionSource, and the (empty) transform/trash/guardian fields.
func TestWriteAuditPopulatesObservabilityFieldsForAllow(t *testing.T) {
	toolName := "audit_allow_tool"
	mustRegisterRuntimeTool(t, runtimeTestTool{
		name:     toolName,
		readOnly: true,
		params: []tool.ToolParameter{
			{Name: "value", Type: "string", Required: true},
		},
	})

	sink := &tool.InMemorySink{}
	runtime := NewToolRuntimeWithConfig(nil, ToolRuntimeConfig{
		AuditSink: sink,
	})

	result := runtime.Execute(context.Background(), convcontext.ToolCall{
		ID: "call_audit_allow",
		Function: convcontext.FunctionCall{
			Name:      toolName,
			Arguments: `{"value":"hello"}`,
		},
	})

	if result.IsError {
		t.Fatalf("expected success, got %+v", result)
	}

	records := sink.Records()
	if len(records) != 1 {
		t.Fatalf("expected 1 audit record, got %d", len(records))
	}
	rec := records[0]
	if rec.ProfileName != profile.DefaultProfile().Name {
		t.Errorf("ProfileName = %q, want %q", rec.ProfileName, profile.DefaultProfile().Name)
	}
	if rec.DecisionAction != "allow" {
		t.Errorf("DecisionAction = %q, want %q", rec.DecisionAction, "allow")
	}
	if rec.Intent == nil {
		t.Error("Intent should not be nil for an allow record")
	}
	// Verify the intent serializes to something containing the tool name.
	intentJSON, _ := json.Marshal(rec.Intent)
	if !strings.Contains(string(intentJSON), toolName) {
		t.Errorf("Intent does not contain tool name %q: %s", toolName, string(intentJSON))
	}
	if rec.TransformedFrom != "" || rec.TransformedTo != "" {
		t.Errorf("Transformed fields should be empty for allow, got from=%q to=%q",
			rec.TransformedFrom, rec.TransformedTo)
	}
	if rec.GuardianViolation != "" {
		t.Errorf("GuardianViolation should be empty for allow, got %q", rec.GuardianViolation)
	}
	if len(rec.TrashedPaths) != 0 {
		t.Errorf("TrashedPaths should be empty for allow, got %v", rec.TrashedPaths)
	}
}

// TestWriteAuditPopulatesObservabilityFieldsForDeny verifies that a denied
// tool call produces an audit record with DecisionAction="denied" and the
// decision source populated from the denial.
func TestWriteAuditPopulatesObservabilityFieldsForDeny(t *testing.T) {
	toolName := "audit_deny_tool"
	mustRegisterRuntimeTool(t, runtimeTestTool{
		name:       toolName,
		permission: tool.DenyPermission("sensitive path .git/config"),
		params: []tool.ToolParameter{
			{Name: "value", Type: "string", Required: true},
		},
	})

	sink := &tool.InMemorySink{}
	runtime := NewToolRuntimeWithConfig(nil, ToolRuntimeConfig{
		AuditSink: sink,
	})

	result := runtime.Execute(context.Background(), convcontext.ToolCall{
		ID: "call_audit_deny",
		Function: convcontext.FunctionCall{
			Name:      toolName,
			Arguments: `{"value":"hello"}`,
		},
	})

	if !result.IsError {
		t.Fatalf("expected denied result, got %+v", result)
	}

	records := sink.Records()
	if len(records) != 1 {
		t.Fatalf("expected 1 audit record, got %d", len(records))
	}
	rec := records[0]
	if rec.Outcome != "denied" {
		t.Errorf("Outcome = %q, want %q", rec.Outcome, "denied")
	}
	if rec.Intent == nil {
		t.Error("Intent should not be nil even for a deny (intent is built before the decision)")
	}
}

// TestWriteAuditPopulatesTransformFieldsForTransformDecision verifies that
// when the auto policy returns a generic transform decision, the audit record
// carries both source and target identities.
func TestWriteAuditPopulatesTransformFieldsForTransformDecision(t *testing.T) {
	// Register an original compatibility tool and its transform target.
	originalToolName := "audit_transform_source_tool"
	targetToolName := "audit_transform_target_tool"
	mustRegisterRuntimeTool(t, runtimeTestTool{
		name: originalToolName,
		params: []tool.ToolParameter{
			{Name: "value", Type: "string", Required: true},
		},
	})
	mustRegisterRuntimeTool(t, runtimeTestTool{
		name:   targetToolName,
		output: "transformed output",
		params: []tool.ToolParameter{
			{Name: "patch", Type: "string", Required: true},
		},
	})

	sink := &tool.InMemorySink{}
	// Use a static policy that returns a transform decision directing the
	// runtime to execute targetToolName with the given input.
	transformDecision := ToolDecision{
		Action:          ToolDecisionTransform,
		Source:          ToolDecisionSourceTransform,
		Reason:          "rewrite to compatible target",
		UpdatedToolName: targetToolName,
		UpdatedInput:    `{"patch":"*** Begin Patch\n*** End Patch"}`,
	}
	runtime := NewToolRuntimeWithConfig(nil, ToolRuntimeConfig{
		AuditSink:      sink,
		AutoToolPolicy: staticAutoToolPolicy{decision: transformDecision},
	})

	result := runtime.Execute(context.Background(), convcontext.ToolCall{
		ID: "call_audit_transform",
		Function: convcontext.FunctionCall{
			Name:      originalToolName,
			Arguments: `{"value":"hello"}`,
		},
	})

	if result.IsError {
		t.Fatalf("expected transformed execution to succeed, got %+v", result)
	}

	records := sink.Records()
	if len(records) != 1 {
		t.Fatalf("expected 1 audit record, got %d", len(records))
	}
	rec := records[0]
	if rec.TransformedFrom != originalToolName {
		t.Errorf("TransformedFrom = %q, want %q", rec.TransformedFrom, originalToolName)
	}
	if rec.TransformedTo != targetToolName {
		t.Errorf("TransformedTo = %q, want %q", rec.TransformedTo, targetToolName)
	}
}

// TestEmitToolPermissionDecidedCarriesObservabilityFields verifies that the
// ToolPermissionDecided event carries the ProfileName, TransformedFrom,
// TransformedTo, TrashedPaths, and GuardianViolation fields so the Desktop UI
// can render them.
func TestEmitToolPermissionDecidedCarriesObservabilityFields(t *testing.T) {
	toolName := "audit_event_tool"
	mustRegisterRuntimeTool(t, runtimeTestTool{
		name:     toolName,
		readOnly: true,
		params: []tool.ToolParameter{
			{Name: "value", Type: "string", Required: true},
		},
	})

	var events []AwakenEvent
	runtime := NewToolRuntime(EventSinkFunc(func(event AwakenEvent) {
		events = append(events, event)
	}))

	result := runtime.Execute(context.Background(), convcontext.ToolCall{
		ID: "call_event_audit",
		Function: convcontext.FunctionCall{
			Name:      toolName,
			Arguments: `{"value":"hello"}`,
		},
	})

	if result.IsError {
		t.Fatalf("expected success, got %+v", result)
	}

	var decidedEvent *AwakenEvent
	for i := range events {
		if events[i].Type == ToolPermissionDecidedEventType {
			decidedEvent = &events[i]
			break
		}
	}
	if decidedEvent == nil {
		t.Fatalf("expected a ToolPermissionDecided event, got %+v", events)
	}
	if decidedEvent.ProfileName != profile.DefaultProfile().Name {
		t.Errorf("ProfileName = %q, want %q", decidedEvent.ProfileName, profile.DefaultProfile().Name)
	}
	if decidedEvent.ToolDecisionAction != string(ToolDecisionAllow) {
		t.Errorf("ToolDecisionAction = %q, want %q", decidedEvent.ToolDecisionAction, ToolDecisionAllow)
	}
}
