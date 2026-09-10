package agentcore

import (
	convcontext "awaken/src/context"
	"context"
)

var hostReadSlots = make(chan struct{}, 16)
var hostMediaReadSlots = make(chan struct{}, 2)

// Heavy media waits before taking a general read slot. Text reads can still
// progress while multiple Agents are queued for PDF/image processing.
func acquireReadResources(ctx context.Context, call convcontext.ToolCall) (func(), error) {
	switch call.Function.Name {
	case "Read", "Glob", "Grep", "WebSearch", "WebFetch":
	default:
		return func() {}, nil
	}
	heavy := call.Function.Name == "Read" && estimateReadMediaAdmissionTokens(call.Function.Arguments) > 0
	if heavy {
		select {
		case hostMediaReadSlots <- struct{}{}:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	select {
	case hostReadSlots <- struct{}{}:
		return func() {
			<-hostReadSlots
			if heavy {
				<-hostMediaReadSlots
			}
		}, nil
	case <-ctx.Done():
		if heavy {
			<-hostMediaReadSlots
		}
		return nil, ctx.Err()
	}
}
