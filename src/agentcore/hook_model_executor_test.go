package agentcore

import (
	"testing"

	"awaken/src/tool/profile"
)

func TestHookInvocationIdentityMarksRecordsWithoutAgentID(t *testing.T) {
	recorder := &forkCaptureRecorder{}
	emitter := newRunEventEmitter("thread", EventSinkFunc(func(AwakenEvent) {}))
	emitter.runID, emitter.recorder, emitter.hookInvocationID = "run", recorder, "hook-invocation"
	emitter.assistantText.WriteString(`{"ok":true}`)
	emitter.textOpen, emitter.textMessageID = true, "message"
	emitter.finishTextIfOpen()
	records := recorder.snapshot()
	if len(records) != 1 || records[0].Payload["hook_invocation_id"] != "hook-invocation" {
		t.Fatalf("Hook-internal assistant record was not isolated: %+v", records)
	}
}

func TestHookAgentReadOnlyProfileNeverWidensParent(t *testing.T) {
	for _, parent := range []profile.ExecutionProfile{
		profile.WorkspaceReadOnly(),
		profile.WorkspaceWrite(),
		profile.OpenWorldRestricted(),
		profile.DangerFullAccess(),
	} {
		child, err := hookAgentReadOnlyProfile(parent)
		if err != nil {
			t.Fatalf("parent=%s: %v", parent.Name, err)
		}
		if child.PermissionProfile != profile.PermissionReadOnly || child.NetworkPolicy != profile.NetworkNone || child.EnvPolicy != profile.EnvMinimal {
			t.Fatalf("parent=%s child=%+v", parent.Name, child)
		}
		for _, permission := range child.SandboxPermissions {
			if permission != "read" {
				t.Fatalf("parent=%s leaked permission %q", parent.Name, permission)
			}
		}
	}
}
