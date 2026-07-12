package tool

import (
	"context"
	"errors"
	"testing"
)

func TestLocalUserExecutionBrokerAllowsHostOperationsWithoutFabricatedEnforcement(t *testing.T) {
	broker := NewLocalUserExecutionBroker()
	for _, operation := range []BrokerOperation{
		BrokerOperationInspect, BrokerOperationMutation, BrokerOperationDelete,
		BrokerOperationNetwork, BrokerOperationUnknown,
	} {
		authorization, err := broker.Authorize(context.Background(), ExecutionBrokerRequest{
			ProviderID: "builtin:shell", Capability: CapabilityShellExecute, Operation: operation,
			Command: "host command", WorkspaceRoot: "/workspace", WorkingDirectory: "/outside-user-dir",
		})
		if err != nil || authorization.Action != BrokerAllow || authorization.RequiresAppliedSandbox || authorization.EnforcementMode != "current_login_user_direct" {
			t.Fatalf("operation %q did not receive honest direct authorization: authorization=%+v err=%v", operation, authorization, err)
		}
		sideEffects := OutcomeSideEffectUnknown
		if operation == BrokerOperationInspect {
			sideEffects = OutcomeSideEffectNone
		}
		receipt, err := broker.Complete(context.Background(), authorization, BrokerExecutionResult{
			TransportStatus: TransportCompleted, ProcessStatus: ProcessExited, SideEffectState: sideEffects,
		})
		if err != nil || receipt.BrokerID != broker.ID() || receipt.SideEffectState != sideEffects || len(receipt.WorkspaceChanges) != 0 {
			t.Fatalf("operation %q fabricated broker facts: receipt=%+v err=%v", operation, receipt, err)
		}
		if sideEffects == OutcomeSideEffectUnknown && receipt.CommitStatus != "side_effects_unobserved" {
			t.Fatalf("operation %q must preserve unknown effects: %+v", operation, receipt)
		}
	}
	for _, request := range []ExecutionBrokerRequest{
		{ProviderID: "builtin:dependency_install", Capability: CapabilityDependencyInstall, Operation: BrokerOperationDependencyInstall},
		{ProviderID: "builtin:git_remote", Capability: CapabilityGitRemoteRead, Operation: BrokerOperationGitRemote},
		{ProviderID: "builtin:shell", Capability: CapabilityShellExecute, Operation: BrokerOperationArtifactPublish},
	} {
		authorization, err := broker.Authorize(context.Background(), request)
		if !errors.Is(err, ErrExecutionBrokerDenied) || authorization.Action != BrokerDeny {
			t.Fatalf("typed reserved operation escaped through direct Shell broker: request=%+v authorization=%+v err=%v", request, authorization, err)
		}
	}
}

func TestRestrictedExecutionBrokerAllowsOnlyBoundedInspection(t *testing.T) {
	broker := NewRestrictedExecutionBroker()
	authorization, err := broker.Authorize(context.Background(), ExecutionBrokerRequest{
		ProviderID: "builtin:shell", Capability: CapabilityShellExecute, Operation: BrokerOperationInspect,
		Command: "git status", WorkspaceRoot: "/workspace", WorkingDirectory: "/workspace",
	})
	if err != nil || authorization.Action != BrokerAllow || authorization.AuthorizationID == "" {
		t.Fatalf("expected inspect authorization, authorization=%+v err=%v", authorization, err)
	}
	receipt, err := broker.Complete(context.Background(), authorization, BrokerExecutionResult{
		TransportStatus: TransportCompleted, ProcessStatus: ProcessExited, SideEffectState: OutcomeSideEffectNone,
	})
	if err != nil || receipt.BrokerID != broker.ID() || receipt.AuthorizationID != authorization.AuthorizationID {
		t.Fatalf("missing broker completion receipt: receipt=%+v err=%v", receipt, err)
	}

	for _, operation := range []BrokerOperation{BrokerOperationMutation, BrokerOperationNetwork, BrokerOperationUnknown} {
		authorization, err := broker.Authorize(context.Background(), ExecutionBrokerRequest{Operation: operation})
		if !errors.Is(err, ErrExecutionBrokerDenied) || authorization.Action != BrokerDeny {
			t.Fatalf("operation %q must fail closed: authorization=%+v err=%v", operation, authorization, err)
		}
	}
}

func TestRestrictedExecutionBrokerTransformsDeleteToTrash(t *testing.T) {
	broker := NewRestrictedExecutionBroker()
	authorization, err := broker.Authorize(context.Background(), ExecutionBrokerRequest{
		Operation: BrokerOperationDelete, ReferencedPaths: []string{"/workspace/a", "/workspace/b"},
	})
	if !errors.Is(err, ErrExecutionTransformRequired) || authorization.Action != BrokerTransform || authorization.Transform == nil {
		t.Fatalf("expected typed delete transform, authorization=%+v err=%v", authorization, err)
	}
	if authorization.Transform.ToolName != "trash_file" || len(authorization.Transform.Paths) != 2 {
		t.Fatalf("unexpected transform: %+v", authorization.Transform)
	}
}

func TestExecutionBrokerContextDistinguishesAbsentFromExplicitNil(t *testing.T) {
	if broker, configured := ExecutionBrokerFromContext(context.Background()); configured || broker != nil {
		t.Fatalf("unexpected implicit broker: %T configured=%v", broker, configured)
	}
	ctx := ContextWithExecutionBroker(context.Background(), nil)
	if broker, configured := ExecutionBrokerFromContext(ctx); !configured || broker != nil {
		t.Fatalf("explicit nil broker must be observable: %T configured=%v", broker, configured)
	}
}
