package server

import (
	"io"
	"net/http"

	"awaken/src/acp"
	"awaken/src/agentcore"
	"awaken/src/connectors"
	"awaken/src/mcpclient"
	"awaken/src/memory"
	"awaken/src/modelprofile"
	"awaken/src/protocol/agui"
	awruntime "awaken/src/runtime"
	"awaken/src/scheduledtask"
	"awaken/src/skills"
	"awaken/src/tool/trash"
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
func NewRouterWithWorkspaceAndSkillSync(runService *awruntime.RunService, infoProvider RuntimeInfoProvider, manager *mcpclient.Manager, connectorRegistry *connectors.Registry, skillRegistry *skills.SkillRegistry, workspaceRoot string, syncSkillsForWorkspace func(awruntime.WorkspaceConfig), shutdown ...func()) http.Handler {
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

// NewRouterWithWorkspaceSkillSyncAndACP builds a fully-wired router that also
// exposes ACP runner/session/permission routes when acpManager is non-nil.
func NewRouterWithWorkspaceSkillSyncAndACP(runService *awruntime.RunService, infoProvider RuntimeInfoProvider, manager *mcpclient.Manager, connectorRegistry *connectors.Registry, skillRegistry *skills.SkillRegistry, acpManager *acp.Manager, workspaceRoot string, syncSkillsForWorkspace func(awruntime.WorkspaceConfig), shutdown ...func()) http.Handler {
	return NewRouterWithControl(runService, infoProvider, manager, connectorRegistry, skillRegistry, acpManager, workspaceRoot, syncSkillsForWorkspace, nil, shutdown...)
}

// NewRouterWithControl builds a fully-wired router that also exposes bridge-only
// control routes protected by a Bearer control token.
func NewRouterWithControl(runService *awruntime.RunService, infoProvider RuntimeInfoProvider, manager *mcpclient.Manager, connectorRegistry *connectors.Registry, skillRegistry *skills.SkillRegistry, acpManager *acp.Manager, workspaceRoot string, syncSkillsForWorkspace func(awruntime.WorkspaceConfig), control *ControlConfig, shutdown ...func()) http.Handler {
	cfg := routerConfig{
		runService:             runService,
		infoProvider:           infoProvider,
		manager:                manager,
		connectorRegistry:      connectorRegistry,
		skillRegistry:          skillRegistry,
		workspaceRoot:          workspaceRoot,
		syncSkillsForWorkspace: syncSkillsForWorkspace,
		acpManager:             acpManager,
		control:                control,
	}
	if len(shutdown) > 0 {
		cfg.shutdown = shutdown[0]
	}
	return newRouterInternal(cfg)
}

// NewRouterWithScheduledTasks builds a fully-wired router that also exposes
// the /v1/scheduled-tasks/* surface. A nil service keeps the routes
// registered but answering 503 (the runtime started without a store).
func NewRouterWithScheduledTasks(runService *awruntime.RunService, infoProvider RuntimeInfoProvider, manager *mcpclient.Manager, connectorRegistry *connectors.Registry, skillRegistry *skills.SkillRegistry, acpManager *acp.Manager, workspaceRoot string, syncSkillsForWorkspace func(awruntime.WorkspaceConfig), control *ControlConfig, scheduledTasks *scheduledtask.Service, shutdown ...func()) http.Handler {
	cfg := routerConfig{
		runService:             runService,
		infoProvider:           infoProvider,
		manager:                manager,
		connectorRegistry:      connectorRegistry,
		skillRegistry:          skillRegistry,
		workspaceRoot:          workspaceRoot,
		syncSkillsForWorkspace: syncSkillsForWorkspace,
		acpManager:             acpManager,
		control:                control,
		scheduledTasks:         scheduledTasks,
	}
	if len(shutdown) > 0 {
		cfg.shutdown = shutdown[0]
	}
	return newRouterInternal(cfg)
}

// ControlConfig groups the runtime identity and lease manager used for bridge-only
// control routes.
type ControlConfig struct {
	Identity          *awruntime.RuntimeIdentity
	LeaseManager      *awruntime.LeaseManager
	ACPSessionManager *awruntime.ACPSessionManager
	ModelProfileStore ACPModelProfileStore
}

type ACPModelProfileStore interface {
	ListReady() (modelprofile.ProfileCatalog, error)
	Resolve(profileID string) (modelprofile.ResolvedProfile, error)
}

type routerConfig struct {
	runService             *awruntime.RunService
	infoProvider           RuntimeInfoProvider
	manager                *mcpclient.Manager
	connectorRegistry      *connectors.Registry
	skillRegistry          *skills.SkillRegistry
	skillTrashAdapter      trash.TrashAdapter
	workspaceRoot          string
	syncSkillsForWorkspace func(awruntime.WorkspaceConfig)
	shutdown               func()
	acpManager             *acp.Manager
	control                *ControlConfig
	scheduledTasks         *scheduledtask.Service
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
			acpManager:             cfg.acpManager,
		}
		mux.HandleFunc("GET /v1/threads", handlers.handleListThreads)
		mux.HandleFunc("PATCH /v1/threads/{thread_id}", handlers.handleRenameThread)
		mux.HandleFunc("DELETE /v1/threads/{thread_id}", handlers.handleDeleteThread)
		mux.HandleFunc("GET /v1/threads/{thread_id}/transcript", handlers.handleGetThreadTranscript)
		mux.HandleFunc("POST /v1/threads/{thread_id}/context-snapshot", handlers.handleRefreshThreadContextSnapshot)
		mux.HandleFunc("POST /v1/runs", handlers.handleCreateRun)
		mux.HandleFunc("GET /v1/runs/{run_id}", handlers.handleGetRun)
		mux.HandleFunc("GET /v1/runs/{run_id}/events", handlers.handleRunEvents)
		mux.HandleFunc("POST /v1/runs/{run_id}/cancel", handlers.handleCancelRun)
		mux.HandleFunc("GET /v1/runs/{run_id}/subagents", handlers.handleListSubagents)
		mux.HandleFunc("GET /v1/runs/{run_id}/subagents/{task_id}", handlers.handleGetSubagent)
		mux.HandleFunc("POST /v1/runs/{run_id}/subagents/{task_id}/messages", handlers.handleMessageSubagent)
		mux.HandleFunc("POST /v1/runs/{run_id}/subagents/{task_id}/cancel", handlers.handleCancelSubagent)
		mux.HandleFunc("POST /v1/runs/{run_id}/user-questions/{question_id}/answer", handlers.handleAnswerUserQuestion)
		mux.HandleFunc("POST /v1/runs/{run_id}/external-interactions/{interaction_id}/resolve", handlers.handleResolveExternalInteraction)
		mux.HandleFunc("GET /v1/runs/{run_id}/audit", handlers.handleGetRunAudit)
		if projectService := cfg.runService.ProjectService(); projectService != nil {
			projects := projectHandlers{service: projectService}
			mux.HandleFunc("GET /v1/projects", projects.handleListProjects)
			mux.HandleFunc("POST /v1/projects", projects.handleUpsertProject)
			mux.HandleFunc("PUT /v1/projects/order", projects.handleReorderProjects)
			mux.HandleFunc("DELETE /v1/projects/{project_id}", projects.handleDetachProject)
			mux.HandleFunc("DELETE /v1/projects/{project_id}/threads", projects.handleDeleteProjectThreads)
			mux.HandleFunc("POST /v1/projects/{project_id}/folders", projects.handleAttachProjectFolder)
			mux.HandleFunc("DELETE /v1/projects/{project_id}/folders", projects.handleDetachProjectFolder)
			mux.HandleFunc("PUT /v1/projects/{project_id}/primary-folder", projects.handleSetPrimaryProjectFolder)
			mux.HandleFunc("POST /v1/projects/{project_id}/folders/trash", projects.handlePrepareTrash)
			mux.HandleFunc("GET /v1/project-operations", projects.handleListPendingTrashOperations)
			mux.HandleFunc("GET /v1/project-operations/{operation_id}", projects.handleGetOperation)
			mux.HandleFunc("POST /v1/project-operations/{operation_id}/cancel", projects.handleCancelOperation)
			mux.HandleFunc("POST /v1/project-operations/{operation_id}/claim-trash", projects.handleClaimTrashOperation)
			mux.HandleFunc("POST /v1/project-operations/{operation_id}/attest-trash", projects.handleAttestTrashOperation)
			mux.HandleFunc("POST /v1/project-operations/{operation_id}/stage-trash", projects.handleStageTrashOperation)
			mux.HandleFunc("POST /v1/project-operations/{operation_id}/begin-trash", projects.handleBeginTrashInvocation)
			mux.HandleFunc("POST /v1/project-operations/{operation_id}/complete-trash", projects.handleCompleteTrashOperation)
			mux.HandleFunc("POST /v1/project-operations/{operation_id}/fail-trash", projects.handleFailTrashOperation)
		}
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
	memoryH := memoryHandlers{defaultWorkspace: cfg.workspaceRoot}
	if cfg.runService != nil {
		memoryH.projects = cfg.runService.ProjectService()
	}
	mux.HandleFunc("GET /v1/memories", memoryH.handleList)
	mux.HandleFunc("GET /v1/memories/{memory_id}", memoryH.handleGet)
	mux.HandleFunc("DELETE /v1/memories/{memory_id}", memoryH.handleDelete)
	if settingsStore, err := memory.NewSettingsStore(); err == nil {
		settingsH := memorySettingsHandlers{store: settingsStore}
		mux.HandleFunc("GET /v1/memory/settings", settingsH.handleGet)
		mux.HandleFunc("PATCH /v1/memory/settings", settingsH.handleUpdate)
	}
	dreamH := memoryDreamHandlers{memoryHandlers: memoryH, runner: memory.NewDreamRunner()}
	mux.HandleFunc("POST /v1/memory/dream", dreamH.handleStart)
	mux.HandleFunc("GET /v1/memory/dream", dreamH.handleList)
	mux.HandleFunc("GET /v1/memory/dream/{task_id}", dreamH.handleGet)
	mux.HandleFunc("POST /v1/memory/dream/{task_id}/cancel", dreamH.handleCancel)
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
		mux.HandleFunc("PUT /v1/connectors/{id}/cli-readiness", connH.handleCLIReadiness)
	}
	skillH := skillHandlers{registry: cfg.skillRegistry, workspaceRoot: cfg.workspaceRoot, trashAdapter: cfg.skillTrashAdapter}
	mux.HandleFunc("GET /v1/skills", skillH.handleListSkills)
	mux.HandleFunc("POST /v1/skills/preview", skillH.handlePreviewSkill)
	mux.HandleFunc("POST /v1/skills/install", skillH.handleInstallSkill)
	mux.HandleFunc("DELETE /v1/skills/{id}", skillH.handleDeleteSkill)
	mux.HandleFunc("POST /v1/skills/{id}/enable", skillH.handleEnableSkill)
	mux.HandleFunc("POST /v1/skills/{id}/disable", skillH.handleDisableSkill)
	// Scheduled-task routes are always registered; a nil service answers 503.
	// Go 1.22 mux precedence lets the literal /events beat /{task_id}.
	schedH := newScheduledTaskHandlers(cfg.scheduledTasks)
	mux.HandleFunc("GET /v1/scheduled-tasks", schedH.handleList)
	mux.HandleFunc("POST /v1/scheduled-tasks", schedH.handleCreate)
	mux.HandleFunc("GET /v1/scheduled-tasks/events", schedH.handleEvents)
	mux.HandleFunc("GET /v1/scheduled-tasks/{task_id}", schedH.handleGet)
	mux.HandleFunc("PATCH /v1/scheduled-tasks/{task_id}", schedH.handlePatch)
	mux.HandleFunc("DELETE /v1/scheduled-tasks/{task_id}", schedH.handleDelete)
	mux.HandleFunc("POST /v1/scheduled-tasks/{task_id}/pause", func(w http.ResponseWriter, r *http.Request) {
		schedH.handleSetEnabled(w, r, false)
	})
	mux.HandleFunc("POST /v1/scheduled-tasks/{task_id}/resume", func(w http.ResponseWriter, r *http.Request) {
		schedH.handleSetEnabled(w, r, true)
	})
	mux.HandleFunc("POST /v1/scheduled-tasks/{task_id}/run", schedH.handleRun)
	mux.HandleFunc("POST /v1/scheduled-tasks/{task_id}/cancel", schedH.handleCancel)
	mux.HandleFunc("GET /v1/scheduled-tasks/{task_id}/executions", schedH.handleListExecutions)
	if cfg.acpManager != nil {
		acpH := acpHandlers{manager: cfg.acpManager, runService: cfg.runService}
		mux.HandleFunc("GET /v1/acp/runners", acpH.handleListRunners)
		mux.HandleFunc("POST /v1/acp/runners", acpH.handleCreateRunner)
		mux.HandleFunc("PATCH /v1/acp/runners/{runner_id}", acpH.handleUpdateRunner)
		mux.HandleFunc("DELETE /v1/acp/runners/{runner_id}", acpH.handleDeleteRunner)
		mux.HandleFunc("POST /v1/acp/runners/{runner_id}/probe", acpH.handleProbeRunner)
		mux.HandleFunc("GET /v1/acp/sessions", acpH.handleListSessions)
		mux.HandleFunc("POST /v1/acp/sessions/{runner_id}/close", acpH.handleCloseSession)
		mux.HandleFunc("POST /v1/runs/{run_id}/acp-permissions/{interaction_id}/resolve", acpH.handleResolvePermission)
	}
	if cfg.control != nil && cfg.control.Identity != nil {
		ctrl := controlHandlers{
			identity:     cfg.control.Identity,
			leaseManager: cfg.control.LeaseManager,
			runService:   cfg.runService,
		}
		controlMux := http.NewServeMux()
		controlMux.HandleFunc("GET /v1/control/runtime/identity", ctrl.handleRuntimeIdentity)
		controlMux.HandleFunc("POST /v1/control/leases", ctrl.handleCreateLease)
		controlMux.HandleFunc("POST /v1/control/leases/{lease_id}/heartbeat", ctrl.handleHeartbeatLease)
		controlMux.HandleFunc("DELETE /v1/control/leases/{lease_id}", ctrl.handleReleaseLease)
		acpSessionCtrl := acpSessionControlHandlers{
			runService:        cfg.runService,
			acpSessionManager: cfg.control.ACPSessionManager,
		}
		controlMux.HandleFunc("POST /v1/control/acp/sessions", acpSessionCtrl.handleCreateACPSession)
		controlMux.HandleFunc("GET /v1/control/acp/sessions/{session_id}", acpSessionCtrl.handleGetACPSession)
		controlMux.HandleFunc("POST /v1/control/acp/sessions/{session_id}/close", acpSessionCtrl.handleCloseACPSession)
		acpProfileCtrl := acpModelProfileControlHandlers{store: cfg.control.ModelProfileStore}
		controlMux.HandleFunc("GET /v1/control/acp/model-profiles", acpProfileCtrl.handleList)
		if cfg.runService != nil && cfg.control.ACPSessionManager != nil {
			acpRunCtrl := acpRunControlHandlers{
				runService:        cfg.runService,
				acpSessionManager: cfg.control.ACPSessionManager,
				acpManager:        cfg.acpManager,
				identity:          cfg.control.Identity,
				modelProfileStore: cfg.control.ModelProfileStore,
			}
			controlMux.HandleFunc("POST /v1/control/acp/sessions/{session_id}/runs", acpRunCtrl.handleCreateACPRun)
			controlMux.HandleFunc("DELETE /v1/control/acp/sessions/{session_id}/current-run/{run_id}", acpRunCtrl.handleClearCurrentRun)
			controlMux.HandleFunc("GET /v1/control/acp/sessions/{session_id}/runs/{run_id}/events", acpRunCtrl.handleACPRunEvents)
			controlMux.HandleFunc("POST /v1/control/acp/sessions/{session_id}/runs/{run_id}/cancel", acpRunCtrl.handleCancelACPRun)
			if cfg.acpManager != nil {
				controlMux.HandleFunc("POST /v1/control/acp/sessions/{session_id}/runs/{run_id}/permissions/{interaction_id}/resolve", acpRunCtrl.handleResolveACPPermission)
			}
		}
		auth := ControlTokenMiddleware(cfg.control.Identity.ControlToken)
		mux.Handle("/v1/control/", auth(controlMux))
	}
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
