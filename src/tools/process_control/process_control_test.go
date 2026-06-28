package processcontrol

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"awaken/src/processes"
	"awaken/src/tool"
)

func TestProcessControlStartListLogWriteAndKill(t *testing.T) {
	root := t.TempDir()
	registry := processes.NewRegistry(processes.RegistryConfig{})
	processTool := NewProcessControlTool(registry)
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, nil))

	start := executeProcessControl(t, processTool, ctx, `{"action":"start","command":"cat","background":true}`)
	if start.Session == nil || start.Session.Status != processes.StatusRunning || start.Session.Mode != processes.ModePipe {
		t.Fatalf("unexpected start response: %+v", start)
	}

	write := executeProcessControl(t, processTool, ctx, `{"action":"write","session_id":"`+start.Session.ID+`","stdin":"hello pipe\n"}`)
	if write.WrittenBytes != len("hello pipe\n") {
		t.Fatalf("unexpected write response: %+v", write)
	}

	log := waitForToolLog(t, processTool, ctx, start.Session.ID, "hello pipe")
	if log.Log == nil || !strings.Contains(log.Log.Stdout, "hello pipe") {
		t.Fatalf("expected stdout log, got %+v", log)
	}

	list := executeProcessControl(t, processTool, ctx, `{"action":"list"}`)
	if len(list.Sessions) != 1 || list.Sessions[0].ID != start.Session.ID {
		t.Fatalf("expected listed session, got %+v", list)
	}

	kill := executeProcessControl(t, processTool, ctx, `{"action":"kill","session_id":"`+start.Session.ID+`"}`)
	if kill.Session == nil || kill.Session.Status != processes.StatusKilled {
		t.Fatalf("expected killed session, got %+v", kill)
	}
}

func TestProcessControlWaitTimesOut(t *testing.T) {
	root := t.TempDir()
	registry := processes.NewRegistry(processes.RegistryConfig{})
	processTool := NewProcessControlTool(registry)
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, nil))

	start := executeProcessControl(t, processTool, ctx, `{"action":"start","command":"sleep 5","background":true}`)
	defer func() { _, _ = registry.Kill(context.Background(), start.Session.ID) }()

	wait := executeProcessControl(t, processTool, ctx, `{"action":"wait","session_id":"`+start.Session.ID+`","timeout_ms":10}`)
	if !wait.TimedOut || wait.Session == nil || wait.Session.Status != processes.StatusRunning {
		t.Fatalf("expected timed out running wait, got %+v", wait)
	}
}

func TestProcessControlForegroundStartUsesBoundedWait(t *testing.T) {
	root := t.TempDir()
	registry := processes.NewRegistry(processes.RegistryConfig{})
	processTool := NewProcessControlTool(registry)
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, nil))

	response := executeProcessControl(t, processTool, ctx, `{"action":"start","command":"printf done","background":false,"timeout_ms":1000}`)
	if response.TimedOut || response.Session == nil || response.Session.Status != processes.StatusExited {
		t.Fatalf("expected foreground start to finish, got %+v", response)
	}
}

func TestProcessControlPermissionClassifiesStartAndKill(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	processTool := NewProcessControlTool(processes.NewRegistry(processes.RegistryConfig{}))
	permissionContext := tool.NewPermissionContext(root, nil)

	decision := processTool.CheckPermission(context.Background(), `{"action":"start","command":"cat","background":true}`, permissionContext)
	if decision.Behavior != tool.PermissionAllow {
		t.Fatalf("expected simple read-only start to be allowed, got %+v", decision)
	}

	decision = processTool.CheckPermission(context.Background(), `{"action":"start","command":"rm -rf build","background":true}`, permissionContext)
	if decision.Behavior != tool.PermissionAsk || decision.ApprovalRequest.RiskTone != "danger" {
		t.Fatalf("expected dangerous start to ask with danger risk, got %+v", decision)
	}

	decision = processTool.CheckPermission(context.Background(), `{"action":"start","command":"cat /etc/hosts","background":true}`, permissionContext)
	if decision.Behavior != tool.PermissionAsk || decision.ApprovalRequest.RiskTone != "warning" {
		t.Fatalf("expected path-bearing read command to ask with warning risk, got %+v", decision)
	}

	decision = processTool.CheckPermission(context.Background(), `{"action":"kill","session_id":"proc_123"}`, permissionContext)
	if decision.Behavior != tool.PermissionAsk || decision.ApprovalRequest.RiskTone != "warning" {
		t.Fatalf("expected kill to ask with warning risk, got %+v", decision)
	}

	decision = processTool.CheckPermission(context.Background(), `{"action":"write","session_id":"proc_123","stdin":"rm -rf build\n"}`, permissionContext)
	if decision.Behavior != tool.PermissionAsk || decision.ApprovalRequest.RiskTone != "warning" {
		t.Fatalf("expected write to ask with warning risk, got %+v", decision)
	}

	decision = processTool.CheckPermission(context.Background(), `{"action":"write","stdin":"hello"}`, permissionContext)
	if decision.Behavior != tool.PermissionDeny {
		t.Fatalf("expected write without session_id to be denied, got %+v", decision)
	}

	decision = processTool.CheckPermission(context.Background(), `{"action":"start","command":"pwd","workdir":"`+outside+`"}`, permissionContext)
	if decision.Behavior != tool.PermissionDeny {
		t.Fatalf("expected outside workdir to be denied, got %+v", decision)
	}
}

func TestProcessControlRegistryStillRejectsSymlinkEscapedWorkdir(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(root, "outside")
	if err := createSymlink(outside, link); err != nil {
		t.Skipf("symlink unavailable in this environment: %v", err)
	}

	processTool := NewProcessControlTool(processes.NewRegistry(processes.RegistryConfig{}))
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, nil))
	_, err := processTool.Execute(ctx, `{"action":"start","command":"pwd","workdir":"outside"}`)
	if err == nil || !strings.Contains(err.Error(), "outside workspace") {
		t.Fatalf("expected escaped workdir error, got %v", err)
	}
}

func executeProcessControl(t *testing.T, processTool *ProcessControlTool, ctx context.Context, input string) processControlResponse {
	t.Helper()
	out, err := processTool.Execute(ctx, input)
	if err != nil {
		t.Fatalf("Execute(%s) returned error: %v", input, err)
	}
	var response processControlResponse
	if err := json.Unmarshal([]byte(out), &response); err != nil {
		t.Fatalf("failed to decode response %q: %v", out, err)
	}
	return response
}

func waitForToolLog(t *testing.T, processTool *ProcessControlTool, ctx context.Context, sessionID string, want string) processControlResponse {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		response := executeProcessControl(t, processTool, ctx, `{"action":"log","session_id":"`+sessionID+`"}`)
		if response.Log != nil && strings.Contains(response.Log.Stdout, want) {
			return response
		}
		time.Sleep(10 * time.Millisecond)
	}
	return executeProcessControl(t, processTool, ctx, `{"action":"log","session_id":"`+sessionID+`"}`)
}
