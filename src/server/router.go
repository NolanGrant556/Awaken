package server

import (
	"io"
	"net/http"

	"awaken/src/agentcore"
	"awaken/src/protocol/agui"
	awruntime "awaken/src/runtime"
)

func NewRouter(runService *awruntime.RunService, infoProvider RuntimeInfoProvider) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", handleHealth)
	mux.HandleFunc("GET /v1/agui/demo", handleAGUIDemo)
	mux.HandleFunc("GET /v1/runtime/info", func(w http.ResponseWriter, r *http.Request) {
		handleRuntimeInfo(w, r, infoProvider)
	})
	if runService != nil {
		handlers := runHandlers{runService: runService}
		mux.HandleFunc("POST /v1/runs", handlers.handleCreateRun)
		mux.HandleFunc("GET /v1/runs/{run_id}/events", handlers.handleRunEvents)
		mux.HandleFunc("POST /v1/runs/{run_id}/cancel", handlers.handleCancelRun)
		mux.HandleFunc("POST /v1/runs/{run_id}/approvals/{approval_id}/resolve", handlers.handleResolveApproval)
	}
	return mux
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.WriteString(w, "ok")
}

func handleAGUIDemo(w http.ResponseWriter, r *http.Request) {
	stream, ok := NewSSEStream(w)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	demoEvents := []agentcore.AwakenEvent{
		{Type: agentcore.RunStartedEventType, ThreadID: "demo-thread", RunID: "demo-run"},
		{Type: agentcore.TextMessageStartedEventType, ThreadID: "demo-thread", RunID: "demo-run", MessageID: "msg-demo", Role: "assistant"},
		{Type: agentcore.TextDeltaEventType, ThreadID: "demo-thread", RunID: "demo-run", MessageID: "msg-demo", Delta: "Hello from Awaken demo."},
		{Type: agentcore.TextMessageFinishedEventType, ThreadID: "demo-thread", RunID: "demo-run", MessageID: "msg-demo"},
		{Type: agentcore.ToolCallStartedEventType, ThreadID: "demo-thread", RunID: "demo-run", MessageID: "msg-demo", ParentMessageID: "msg-demo", ToolCallID: "tool-demo", ToolName: "demo_lookup"},
		{Type: agentcore.ToolCallArgsEventType, ThreadID: "demo-thread", RunID: "demo-run", ToolCallID: "tool-demo", Arguments: "{\"query\":\"demo\"}"},
		{Type: agentcore.ToolCallFinishedEventType, ThreadID: "demo-thread", RunID: "demo-run", ToolCallID: "tool-demo"},
		{Type: agentcore.ToolResultEventType, ThreadID: "demo-thread", RunID: "demo-run", MessageID: "tool-result-demo", ToolCallID: "tool-demo", Content: "demo result", Role: "tool"},
		{Type: agentcore.RunFinishedEventType, ThreadID: "demo-thread", RunID: "demo-run", Content: "demo complete"},
	}

	for _, event := range demoEvents {
		for _, mapped := range agui.MapEvent(event) {
			if err := stream.WriteEvent(mapped); err != nil {
				return
			}
		}
	}
}
