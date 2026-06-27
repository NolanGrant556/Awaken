package agentcore

import (
	stdcontext "context"
	"sync"

	convcontext "awaken/src/context"
	"awaken/src/tool"
)

type ToolExecutionStrategy string

const (
	ToolExecutionStrategySequential ToolExecutionStrategy = "sequential"
	ToolExecutionStrategyConcurrent ToolExecutionStrategy = "concurrent"
)

type ToolOrchestratorConfig struct {
	Strategy ToolExecutionStrategy
}

type ToolOrchestrator struct {
	runtime *ToolRuntime
	config  ToolOrchestratorConfig
}

type toolOrchestratorEvents interface {
	emitToolCall(convcontext.ToolCall)
	emitToolProgress(convcontext.ToolCall, string)
	emitToolResult(ToolExecutionResult)
	markMutation()
}

func NewToolOrchestrator(runtime *ToolRuntime, config ToolOrchestratorConfig) *ToolOrchestrator {
	if runtime == nil {
		runtime = NewToolRuntime(nil)
	}
	config.Strategy = normalizeToolExecutionStrategy(config.Strategy)
	return &ToolOrchestrator{
		runtime: runtime,
		config:  config,
	}
}

func toolOrchestratorConfigFromAgentConfig(config AgentConfig) ToolOrchestratorConfig {
	return ToolOrchestratorConfig{
		Strategy: config.ToolExecution.Strategy,
	}
}

func normalizeToolExecutionStrategy(strategy ToolExecutionStrategy) ToolExecutionStrategy {
	if strategy == ToolExecutionStrategyConcurrent {
		return ToolExecutionStrategyConcurrent
	}
	return ToolExecutionStrategySequential
}

func (o *ToolOrchestrator) Execute(
	ctx stdcontext.Context,
	calls []convcontext.ToolCall,
	events toolOrchestratorEvents,
) []ToolExecutionResult {
	if ctx == nil {
		ctx = stdcontext.Background()
	}
	if len(calls) == 0 {
		return nil
	}

	results := make([]ToolExecutionResult, len(calls))
	for i := 0; i < len(calls); {
		if ctx.Err() != nil {
			fillCancelledToolResults(results, calls, i, len(calls))
			break
		}

		if o.canExecuteConcurrently(ctx, calls[i]) {
			end := i + 1
			for end < len(calls) && ctx.Err() == nil && o.canExecuteConcurrently(ctx, calls[end]) {
				end++
			}
			if end-i > 1 {
				o.executeConcurrentRun(ctx, calls, results, i, end, events)
			} else {
				results[i] = o.executeOne(ctx, calls[i], events)
			}
			i = end
			continue
		}

		results[i] = o.executeOne(ctx, calls[i], events)
		i++
	}
	return results
}

func (o *ToolOrchestrator) executeOne(
	ctx stdcontext.Context,
	call convcontext.ToolCall,
	events toolOrchestratorEvents,
) ToolExecutionResult {
	if ctx.Err() != nil {
		return syntheticCancelledToolResult(call)
	}
	o.emitToolStart(call, events)
	result := o.runtime.Execute(ctx, call)
	o.emitToolFinished(call, result, events)
	return result
}

func (o *ToolOrchestrator) executeConcurrentRun(
	ctx stdcontext.Context,
	calls []convcontext.ToolCall,
	results []ToolExecutionResult,
	start int,
	end int,
	events toolOrchestratorEvents,
) {
	var wg sync.WaitGroup
	started := make([]bool, end-start)

	for i := start; i < end; i++ {
		call := calls[i]
		if ctx.Err() != nil {
			results[i] = syntheticCancelledToolResult(call)
			continue
		}

		o.emitToolStart(call, events)
		started[i-start] = true
		wg.Add(1)
		go func(index int, toolCall convcontext.ToolCall) {
			defer wg.Done()
			results[index] = o.runtime.Execute(ctx, toolCall)
		}(i, call)
	}

	wg.Wait()

	for i := start; i < end; i++ {
		if !started[i-start] {
			continue
		}
		o.emitToolFinished(calls[i], results[i], events)
	}
}

func (o *ToolOrchestrator) canExecuteConcurrently(ctx stdcontext.Context, call convcontext.ToolCall) bool {
	if o.config.Strategy != ToolExecutionStrategyConcurrent || ctx.Err() != nil {
		return false
	}

	selectedTool, err := tool.GetTool(call.Function.Name)
	if err != nil {
		return false
	}
	readOnlyTool, ok := selectedTool.(tool.ReadOnlyTool)
	if !ok || !readOnlyTool.IsReadOnly() {
		return false
	}

	input, err := normalizeToolInput(call.Function.Arguments, selectedTool.Parameters())
	if err != nil {
		return false
	}
	toolCtx := tool.ContextWithPermissionContext(ctx, o.runtime.permissionContext)
	permission := o.runtime.decideToolPermission(toolCtx, selectedTool, input)
	return permission.Behavior == tool.PermissionAllow
}

func (o *ToolOrchestrator) emitToolStart(call convcontext.ToolCall, events toolOrchestratorEvents) {
	if events == nil {
		return
	}
	events.emitToolCall(call)
	events.emitToolProgress(call, "Executing tool")
}

func (o *ToolOrchestrator) emitToolFinished(
	call convcontext.ToolCall,
	result ToolExecutionResult,
	events toolOrchestratorEvents,
) {
	if events == nil {
		return
	}
	if call.Function.Name == "apply_patch" && !result.IsError {
		events.markMutation()
	}
	events.emitToolProgress(call, toolProgressForResult(result))
	events.emitToolResult(result)
}

func fillCancelledToolResults(
	results []ToolExecutionResult,
	calls []convcontext.ToolCall,
	start int,
	end int,
) {
	for i := start; i < end; i++ {
		results[i] = syntheticCancelledToolResult(calls[i])
	}
}
