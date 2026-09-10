package agentcore

import (
	"context"
	"sync"

	convcontext "awaken/src/context"
	"awaken/src/hookruntime"
	"awaken/src/tool"
)

const maxParallelReadTools = 4

// Only this bounded built-in read set may run together. Hooks can rewrite
// arguments or add effects, so a batch with tool Hooks keeps the serial path.
func (r *Runner) canExecuteParallelReads(calls []convcontext.ToolCall) bool {
	if len(calls) < 2 || r.toolRuntime == nil {
		return false
	}
	if snapshot := r.config.HookAvailabilitySnapshot; snapshot != nil {
		for _, event := range []hookruntime.Event{hookruntime.EventPreToolUse, hookruntime.EventPostToolUse, hookruntime.EventPostToolUseFailure} {
			if len(snapshot.HooksByEvent[event]) > 0 {
				return false
			}
		}
	}
	for _, call := range calls {
		switch call.Function.Name {
		case "Read", "Glob", "Grep", "WebSearch", "WebFetch":
		default:
			return false
		}
		var definition tool.ToolDefinition
		if r.toolRuntime.toolSnapshot != nil {
			entry, err := r.toolRuntime.toolSnapshot.Lookup(call.Function.Name)
			if err != nil {
				return false
			}
			definition = entry.Definition
		} else {
			entry, err := tool.GetRegisteredTool(call.Function.Name)
			if err != nil {
				return false
			}
			definition = entry.Definition
		}
		if definition.Source != tool.SourceBuiltin || !definition.Annotations.ReadOnly {
			return false
		}
	}
	return true
}

type parallelReadEvents struct {
	mu     sync.Mutex
	events []AwakenEvent
}

func (b *parallelReadEvents) Emit(event AwakenEvent) {
	b.mu.Lock()
	b.events = append(b.events, event)
	b.mu.Unlock()
}

func (r *Runner) executeParallelReads(ctx context.Context, calls []convcontext.ToolCall, events *runEventEmitter) []ToolExecutionResult {
	results := make([]ToolExecutionResult, 0, len(calls))
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	for start := 0; start < len(calls); start += maxParallelReadTools {
		batch := calls[start:min(len(calls), start+maxParallelReadTools)]
		outcomes := make([]ToolExecutionResult, len(batch))
		buffers := make([]parallelReadEvents, len(batch))
		execute := make([]bool, len(batch))
		started := make([]bool, len(batch))
		remaining, reserved := 0, 0
		for _, call := range batch {
			reserved += estimateReadMediaAdmissionTokens(call.Function.Arguments)
		}
		if reserved > 0 {
			remaining = r.admissionRemainingTokens(results)
		}
		admissionExhausted := batchHasAdmissionExhaustion(results)
		for index, call := range batch {
			prepared := r.prepareToolCall(runCtx, call, events)
			started[index] = prepared.started
			before := prepared.before
			if prepared.result != nil {
				outcomes[index] = *prepared.result
				if prepared.stopReason != "" {
					cancel()
					if events != nil {
						events.setPendingTerminalReason(prepared.stopReason)
					}
				}
				continue
			}
			estimated := estimateReadMediaAdmissionTokens(call.Function.Arguments)
			if admissionExhausted || (estimated > 0 && estimated > remaining) {
				outcomes[index] = syntheticAdmissionExhaustedResult(call, remaining, estimated)
				admissionExhausted = true
				continue
			}
			// Reserve before any worker starts; concurrent media Reads cannot
			// each spend the same remaining window. Outcomes commit in order.
			remaining -= estimated
			if r.failureSupervisor != nil {
				r.failureSupervisor.NoteRealExecution(before.CallSignature)
			}
			execute[index] = true
		}
		var group sync.WaitGroup
		for index, call := range batch {
			if !execute[index] {
				continue
			}
			group.Add(1)
			go func(index int, call convcontext.ToolCall) {
				defer group.Done()
				if runCtx.Err() != nil {
					outcomes[index] = syntheticCancelledToolResult(call)
					return
				}
				runtime := *r.toolRuntime
				runtime.sink = &buffers[index]
				id := r.toolExecutionIdentity(call, events)
				outcomes[index] = runtime.Execute(tool.ContextWithToolExecutionIdentity(runCtx, id), call)
			}(index, call)
		}
		group.Wait()
		for index, call := range batch {
			result := outcomes[index]
			var after AfterOutcomeDecision
			if execute[index] {
				var err error
				result, after, err = r.finishToolOutcome(runCtx, call, result, events)
				if err != nil {
					cancel()
					if events != nil {
						events.setPendingTerminalReason(TerminalReasonPersistenceError)
					}
					results = append(results, result)
					continue
				}
			}
			if started[index] {
				var buffer *artifactBufferingSink
				if events != nil {
					buffer = &artifactBufferingSink{inner: events.scopedSink()}
					for _, event := range buffers[index].events {
						buffer.Emit(event)
					}
				}
				if err := r.commitToolOutcome(runCtx, call, result, after, buffer, events); err != nil {
					cancel()
					if events != nil {
						events.setPendingTerminalReason(TerminalReasonPersistenceError)
					}
				}
			}
			results = append(results, result)
		}
	}
	return results
}
