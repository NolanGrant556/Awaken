package server

import (
	"io"
	"net/http"

	"awaken/src/agentcore"
	"awaken/src/connectors"
	"awaken/src/mcpclient"
	"awaken/src/protocol/agui"
	awruntime "awaken/src/runtime"
	"awaken/src/skills"
)

func NewRouter(runService *awruntime.RunService, infoProvider RuntimeInfoProvider) http.Handler {
	return newRouterInternal(routerConfig{runService: runService, infoProvider: infoProvider})
}

// NewRouterWithMCP builds a router that also exposes the /v1/mcp/* surface
// when a non-nil manager is supplied. Existing callers continue to use
// NewRouter; the MCP-aware variant is consumed by cmd/awaken-core once it
// constructs the manager.
func NewRouterWithMCP(runService *awruntime.RunService, infoProvider RuntimeInfoProvider, manager *mcpclient.Manager) http.Handler {
	return newRouterInternal(routerConfig{runService: runService, infoProvider: infoProvider, manager: manager})
}

// NewRouterWithMCPAndConnectors builds a router that also exposes the
// /v1/connectors/* surface when a non-nil connector registry is supplied.
// /v1/skills/* routes are always registered (returning 503 when no skill
// registry is configured).
func NewRouterWithMCPAndConnectors(runService *awruntime.RunService, infoProvider RuntimeInfoProvider, manager *mcpclient.Manager, connectorRegistry *connectors.Registry) http.Handler {
	return newRouterInternal(routerConfig{runService: runService, infoProvider: infoProvider, manager: manager, connectorRegistry: connectorRegistry})
}

// NewRouterWithMCPConnectorsAndSkills builds a fully-wired router with MCP,
// connector, and skill surfaces. Nil registries gracefully degrade to 503.
func NewRouterWithMCPConnectorsAndSkills(runService *awruntime.RunService, infoProvider RuntimeInfoProvider, manager *mcpclient.Manager, connectorRegistry *connectors.Registry, skillRegistry *skills.SkillRegistry) http.Handler {
	return newRouterInternal(routerConfig{runService: runService, infoProvider: infoProvider, manager: manager, connectorRegistry: connectorRegistry, skillRegistry: skillRegistry})
}

// NewRouterWithWorkspace builds a fully-wired router and enables project-scoped
// extension writes such as MCP config and skill installation.
func NewRouterWithWorkspace(runService *awruntime.RunService, infoProvider RuntimeInfoProvider, manager *mcpclient.Manager, connectorRegistry *connectors.Registry, skillRegistry *skills.SkillRegistry, workspaceRoot string, shutdown ...func()) http.Handler {
	cfg := routerConfig{
		runService:        runService,
		infoProvider:      infoProvider,
		manager:           manager,
		connectorRegistry: connectorRegistry,
		skillRegistry:     skillRegistry,
		workspaceRoot:     workspaceRoot,
	}
	if len(shutdown) > 0 {
		cfg.shutdown = shutdown[0]
	}
	return newRouterInternal(cfg)
}

// NewRouterWithWorkspaceAndSkillSync additionally lets the runtime synchronize
// project-scoped skill tools to the workspace that is about to run.
func NewRouterWithWorkspaceAndSkillSync(runService *awruntime.RunService, infoProvider RuntimeInfoProvider, manager *mcpclient.Manager, connectorRegistry *connectors.Registry, skillRegistry *skills.SkillRegistry, workspaceRoot string, syncSkillsForWorkspace func(string), shutdown ...func()) http.Handler {
	cfg := routerConfig{
		runService:             runService,
		infoProvider:           infoProvider,
		manager:                manager,
		connectorRegistry:      connectorRegistry,
		skillRegistry:          skillRegistry,
		workspaceRoot:          workspaceRoot,
		syncSkillsForWorkspace: syncSkillsForWorkspace,
	}
	if len(shutdown) > 0 {
		cfg.shutdown = shutdown[0]
	}
	return newRouterInternal(cfg)
}

type routerConfig struct {
	runService             *awruntime.RunService
	infoProvider           RuntimeInfoProvider
	manager                *mcpclient.Manager
	connectorRegistry      *connectors.Registry
	skillRegistry          *skills.SkillRegistry
	workspaceRoot          string
	syncSkillsForWorkspace func(string)
	shutdown               func()
}

func newRouterInternal(cfg routerConfig) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", handleHealth)
	mux.HandleFunc("GET /v1/agui/demo", handleAGUIDemo)
	mux.HandleFunc("GET /v1/runtime/info", func(w http.ResponseWriter, r *http.Request) {
		handleRuntimeInfo(w, r, cfg.infoProvider)
	})
	mux.HandleFunc("GET /v1/model-providers", handleModelProviders)
	if cfg.runService != nil {
		handlers := runHandlers{
			runService:             cfg.runService,
			skillRegistry:          cfg.skillRegistry,
			syncSkillsForWorkspace: cfg.syncSkillsForWorkspace,
		}
		mux.HandleFunc("GET /v1/threads", handlers.handleListThreads)
		mux.HandleFunc("PATCH /v1/threads/{thread_id}", handlers.handleRenameThread)
		mux.HandleFunc("DELETE /v1/threads/{thread_id}", handlers.handleDeleteThread)
		mux.HandleFunc("GET /v1/threads/{thread_id}/transcript", handlers.handleGetThreadTranscript)
		mux.HandleFunc("POST /v1/runs", handlers.handleCreateRun)
		mux.HandleFunc("GET /v1/runs/{run_id}", handlers.handleGetRun)
		mux.HandleFunc("GET /v1/runs/{run_id}/events", handlers.handleRunEvents)
		mux.HandleFunc("POST /v1/runs/{run_id}/cancel", handlers.handleCancelRun)
		mux.HandleFunc("POST /v1/runs/{run_id}/mass-message-reviews/{review_id}/resolve", handlers.handleResolveMassMessageReview)
		mux.HandleFunc("POST /v1/runs/{run_id}/user-questions/{question_id}/answer", handlers.handleAnswerUserQuestion)
		mux.HandleFunc("GET /v1/runs/{run_id}/audit", handlers.handleGetRunAudit)
		if cfg.shutdown != nil {
			mux.HandleFunc("POST /v1/shutdown", func(w http.ResponseWriter, r *http.Request) {
				if err := cfg.runService.FlushTranscripts(); err != nil {
					writeErrorJSON(w, http.StatusInternalServerError, "PERSISTENCE_ERROR", err.Error())
					return
				}
				writeJSON(w, http.StatusOK, map[string]string{"status": "shutting down"})
				go cfg.shutdown()
			})
		}
	}
	mcpH := mcpHandlers{manager: cfg.manager, workspaceRoot: cfg.workspaceRoot}
	mux.HandleFunc("GET /v1/mcp/tools", mcpH.handleListTools)
	mux.HandleFunc("GET /v1/tools", handleListTools)
	if cfg.manager != nil {
		mux.HandleFunc("GET /v1/mcp/servers", mcpH.handleListServers)
		mux.HandleFunc("POST /v1/mcp/servers", mcpH.handleConfigureServers)
		mux.HandleFunc("GET /v1/mcp/servers/{name}/config", mcpH.handleGetServerConfig)
		mux.HandleFunc("PATCH /v1/mcp/servers/{name}/config", mcpH.handleUpdateServerConfig)
		mux.HandleFunc("POST /v1/mcp/servers/{name}/auth", mcpH.handleServerAuth)
		mux.HandleFunc("POST /v1/mcp/servers/{name}/disable", mcpH.handleServerDisable)
		mux.HandleFunc("POST /v1/mcp/servers/{name}/enable", mcpH.handleServerEnable)
		mux.HandleFunc("DELETE /v1/mcp/servers/{name}", mcpH.handleDeleteServer)
	}
	connH := connectorHandlers{registry: cfg.connectorRegistry}
	if cfg.connectorRegistry != nil {
		mux.HandleFunc("GET /v1/connectors", connH.handleListConnectors)
		mux.HandleFunc("POST /v1/connectors/{id}/connect", connH.handleConnect)
		mux.HandleFunc("POST /v1/connectors/{id}/disconnect", connH.handleDisconnect)
	}
	skillH := skillHandlers{registry: cfg.skillRegistry, workspaceRoot: cfg.workspaceRoot}
	mux.HandleFunc("GET /v1/skills", skillH.handleListSkills)
	mux.HandleFunc("POST /v1/skills/install", skillH.handleInstallSkill)
	// POST /v1/skills/{id}/execute reserved for follow-up
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
