package agentcore

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	convcontext "awaken/src/context"
	"awaken/src/tool"
)

type orchestratorTestTool struct {
	name           string
	readOnly       bool
	delay          time.Duration
	release        <-chan struct{}
	permission     tool.PermissionDecision
	probe          *orchestratorProbe
	cancel         context.CancelFunc
	failIfExecuted bool
}

func (t orchestratorTestTool) Name() string {
	return t.name
}

func (t orchestratorTestTool) Description() string {
	return "orchestrator test tool"
}

func (t orchestratorTestTool) Parameters() []tool.ToolParameter {
	return []tool.ToolParameter{
		{Name: "value", Description: "value", Type: "string", Required: false},
	}
}

func (t orchestratorTestTool) IsReadOnly() bool {
	return t.readOnly
}

func (t orchestratorTestTool) CheckPermission(ctx context.Context, input string, permissionContext tool.PermissionContext) tool.PermissionDecision {
	if t.permission.Behavior != "" {
		return t.permission
	}
	return tool.AllowPermission()
}

func (t orchestratorTestTool) Execute(ctx context.Context, input string) (string, error) {
	if t.failIfExecuted {
		panic(t.name + " should not be executed")
	}
	done := func() {}
	if t.probe != nil {
		done = t.probe.started(t.name)
	}
	defer done()

	if t.cancel != nil {
		t.cancel()
		if err := ctx.Err(); err != nil {
			return "", err
		}
	}
	if t.release != nil {
		select {
		case <-t.release:
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	if t.delay > 0 {
		select {
		case <-time.After(t.delay):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	return "ran:" + t.name, nil
}

type orchestratorProbe struct {
	mu            sync.Mutex
	active        int
	maxActive     int
	order         []string
	activeTwoOnce sync.Once
	activeTwo     chan struct{}
}

func newOrchestratorProbe() *orchestratorProbe {
	return &orchestratorProbe{
		activeTwo: make(chan struct{}),
	}
}

func (p *orchestratorProbe) started(name string) func() {
	p.mu.Lock()
	p.active++
	if p.active > p.maxActive {
		p.maxActive = p.active
	}
	p.order = append(p.order, name)
	if p.active >= 2 {
		p.activeTwoOnce.Do(func() {
			close(p.activeTwo)
		})
	}
	p.mu.Unlock()

	return func() {
		p.mu.Lock()
		p.active--
		p.mu.Unlock()
	}
}

func (p *orchestratorProbe) maxConcurrent() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.maxActive
}

func (p *orchestratorProbe) executionOrder() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.order...)
}

type orchestratorApprovalSink struct {
	probe *orchestratorProbe
	delay time.Duration
}

func (s orchestratorApprovalSink) Emit(AwakenEvent) {}

func (s orchestratorApprovalSink) AwaitApproval(ctx context.Context, request ApprovalRequest) (ApprovalDecision, error) {
	done := func() {}
	if s.probe != nil {
		done = s.probe.started(request.ToolName)
	}
	defer done()

	if s.delay > 0 {
		select {
		case <-time.After(s.delay):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	return ApprovalDecisionApproved, nil
}

func TestToolOrchestratorDefaultSequentialBehavior(t *testing.T) {
	probe := newOrchestratorProbe()
	firstTool := "orchestrator_default_first"
	secondTool := "orchestrator_default_second"
	mustRegisterRuntimeTool(t, orchestratorTestTool{name: firstTool, readOnly: true, delay: 20 * time.Millisecond, probe: probe})
	mustRegisterRuntimeTool(t, orchestratorTestTool{name: secondTool, readOnly: true, delay: 20 * time.Millisecond, probe: probe})

	results := NewToolOrchestrator(NewToolRuntime(nil), ToolOrchestratorConfig{}).Execute(context.Background(), []convcontext.ToolCall{
		{ID: "call_default_1", Function: convcontext.FunctionCall{Name: firstTool, Arguments: `{}`}},
		{ID: "call_default_2", Function: convcontext.FunctionCall{Name: secondTool, Arguments: `{}`}},
	}, nil)

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].ToolCallID != "call_default_1" || results[1].ToolCallID != "call_default_2" {
		t.Fatalf("expected result order to match tool call order, got %+v", results)
	}
	if probe.maxConcurrent() != 1 {
		t.Fatalf("expected default strategy to execute sequentially, max concurrent=%d", probe.maxConcurrent())
	}
	if got := probe.executionOrder(); !reflect.DeepEqual(got, []string{firstTool, secondTool}) {
		t.Fatalf("expected sequential execution order, got %+v", got)
	}
}

func TestToolOrchestratorConcurrentReadOnlyToolsOverlapAndPreserveOrder(t *testing.T) {
	probe := newOrchestratorProbe()
	release := make(chan struct{})
	firstTool := "orchestrator_readonly_first"
	secondTool := "orchestrator_readonly_second"
	mustRegisterRuntimeTool(t, orchestratorTestTool{name: firstTool, readOnly: true, release: release, probe: probe})
	mustRegisterRuntimeTool(t, orchestratorTestTool{name: secondTool, readOnly: true, release: release, probe: probe})

	orchestrator := NewToolOrchestrator(NewToolRuntime(nil), ToolOrchestratorConfig{
		Strategy: ToolExecutionStrategyConcurrent,
	})
	resultCh := make(chan []ToolExecutionResult, 1)
	startedAt := time.Now()
	go func() {
		resultCh <- orchestrator.Execute(context.Background(), []convcontext.ToolCall{
			{ID: "call_readonly_1", Function: convcontext.FunctionCall{Name: firstTool, Arguments: `{}`}},
			{ID: "call_readonly_2", Function: convcontext.FunctionCall{Name: secondTool, Arguments: `{}`}},
		}, nil)
	}()

	select {
	case <-probe.activeTwo:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("expected read-only tools to overlap under concurrent strategy")
	}
	close(release)

	var results []ToolExecutionResult
	select {
	case results = <-resultCh:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for concurrent tool results")
	}

	if time.Since(startedAt) > time.Second {
		t.Fatalf("expected concurrent execution to complete promptly")
	}
	if probe.maxConcurrent() < 2 {
		t.Fatalf("expected overlapping execution, max concurrent=%d", probe.maxConcurrent())
	}
	if len(results) != 2 || results[0].ToolCallID != "call_readonly_1" || results[1].ToolCallID != "call_readonly_2" {
		t.Fatalf("expected result order to match input order, got %+v", results)
	}
	if results[0].Name != firstTool || results[1].Name != secondTool {
		t.Fatalf("expected result names to match input order, got %+v", results)
	}
}

func TestToolOrchestratorConcurrentStrategyKeepsMutationToolsSequential(t *testing.T) {
	probe := newOrchestratorProbe()
	firstTool := "orchestrator_mutation_first"
	secondTool := "orchestrator_mutation_second"
	mustRegisterRuntimeTool(t, orchestratorTestTool{name: firstTool, readOnly: false, delay: 20 * time.Millisecond, probe: probe})
	mustRegisterRuntimeTool(t, orchestratorTestTool{name: secondTool, readOnly: false, delay: 20 * time.Millisecond, probe: probe})

	results := NewToolOrchestrator(NewToolRuntime(nil), ToolOrchestratorConfig{
		Strategy: ToolExecutionStrategyConcurrent,
	}).Execute(context.Background(), []convcontext.ToolCall{
		{ID: "call_mutation_1", Function: convcontext.FunctionCall{Name: firstTool, Arguments: `{}`}},
		{ID: "call_mutation_2", Function: convcontext.FunctionCall{Name: secondTool, Arguments: `{}`}},
	}, nil)

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if probe.maxConcurrent() != 1 {
		t.Fatalf("expected mutation tools to remain sequential, max concurrent=%d", probe.maxConcurrent())
	}
	if got := probe.executionOrder(); !reflect.DeepEqual(got, []string{firstTool, secondTool}) {
		t.Fatalf("expected mutation tools to preserve order, got %+v", got)
	}
}

func TestToolOrchestratorConcurrentStrategyKeepsApprovalToolsSequential(t *testing.T) {
	probe := newOrchestratorProbe()
	firstTool := "orchestrator_approval_first"
	secondTool := "orchestrator_approval_second"
	approval := tool.AskPermission("approval required", tool.ApprovalRequest{})
	mustRegisterRuntimeTool(t, orchestratorTestTool{name: firstTool, readOnly: true, permission: approval})
	mustRegisterRuntimeTool(t, orchestratorTestTool{name: secondTool, readOnly: true, permission: approval})

	runtime := NewToolRuntime(orchestratorApprovalSink{probe: probe, delay: 20 * time.Millisecond})
	results := NewToolOrchestrator(runtime, ToolOrchestratorConfig{
		Strategy: ToolExecutionStrategyConcurrent,
	}).Execute(context.Background(), []convcontext.ToolCall{
		{ID: "call_approval_1", Function: convcontext.FunctionCall{Name: firstTool, Arguments: `{}`}},
		{ID: "call_approval_2", Function: convcontext.FunctionCall{Name: secondTool, Arguments: `{}`}},
	}, nil)

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].IsError || results[1].IsError {
		t.Fatalf("expected approval tools to execute after approval, got %+v", results)
	}
	if probe.maxConcurrent() != 1 {
		t.Fatalf("expected approval tools to remain sequential, max concurrent approvals=%d", probe.maxConcurrent())
	}
}

func TestToolOrchestratorCancelFillsResultsForUnstartedCalls(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	firstTool := "orchestrator_cancel_first"
	secondTool := "orchestrator_cancel_second"
	mustRegisterRuntimeTool(t, orchestratorTestTool{name: firstTool, readOnly: false, cancel: cancel})
	mustRegisterRuntimeTool(t, orchestratorTestTool{name: secondTool, readOnly: true, failIfExecuted: true})

	results := NewToolOrchestrator(NewToolRuntime(nil), ToolOrchestratorConfig{
		Strategy: ToolExecutionStrategyConcurrent,
	}).Execute(ctx, []convcontext.ToolCall{
		{ID: "call_cancel_1", Function: convcontext.FunctionCall{Name: firstTool, Arguments: `{}`}},
		{ID: "call_cancel_2", Function: convcontext.FunctionCall{Name: secondTool, Arguments: `{}`}},
	}, nil)

	if len(results) != 2 {
		t.Fatalf("expected result count to match tool calls, got %d", len(results))
	}
	if results[0].ToolCallID != "call_cancel_1" || results[1].ToolCallID != "call_cancel_2" {
		t.Fatalf("expected tool call ids to be preserved, got %+v", results)
	}
	if !results[0].IsError || !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatalf("expected first result to observe cancellation, got result=%+v ctx=%v", results[0], ctx.Err())
	}
	if !results[1].IsError || results[1].Content != "工具执行已被取消" {
		t.Fatalf("expected synthetic cancelled result for unstarted call, got %+v", results[1])
	}
}
