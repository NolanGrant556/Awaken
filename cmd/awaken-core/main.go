package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"awaken/src/acp"
	"awaken/src/acp/protocol"
	"awaken/src/agentcore"
	"awaken/src/connectors"
	"awaken/src/desktopdata"
	"awaken/src/llm"
	"awaken/src/mcpclient"
	"awaken/src/memory"
	"awaken/src/runtime"
	"awaken/src/server"
	"awaken/src/skillruntime"
	"awaken/src/skills"
	"awaken/src/tool"
	_ "awaken/src/tools"
	configureacprunner "awaken/src/tools/configure_acp_runner"
	delegateexternalagent "awaken/src/tools/delegate_external_agent"
	bashtool "awaken/src/tools/bash"
)

func main() {
	addr := os.Getenv("AWAKEN_CORE_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8787"
	}

	defaultWorkspace := resolveDefaultWorkspace()
	infoProvider := server.NewLLMRuntimeInfoProvider("", "", "", defaultWorkspace, "")
	legacyCLIReport, err := connectors.MigrateLegacyBuiltinCLIConfigs()
	if err != nil {
		log.Printf("[connectors] legacy CLI MCP migration failed: %v", err)
	} else if len(legacyCLIReport.Removed) > 0 || len(legacyCLIReport.ReviewRequired) > 0 {
		log.Printf("[connectors] legacy CLI MCP migration removed=%v review_required=%v", legacyCLIReport.Removed, legacyCLIReport.ReviewRequired)
	}

	manager := buildMCPManager(defaultWorkspace)
	connRegistry := buildConnectorRegistry(manager, defaultWorkspace, legacyCLIReport.ReviewRequired)
	skillRegistry := buildSkillRegistry(manager, defaultWorkspace)
	syncSkillsForWorkspace := func(workspaceRoot string) {
		skillRegistry.ReloadLocalSources(skills.LocalSourceDirs(workspaceRoot))
	}
	skillManager := skillruntime.NewManager(skillRegistry, skillruntime.StdLogger{})
	skillManager.RefreshTools()

	acpManager := buildACPManager(defaultWorkspace)
	delegateexternalagent.SetManager(acpManager)
	configureacprunner.SetRegistry(acpManager.Registry())

	runService := runtime.NewRunServiceWithModelProfiles(
		func(workspace runtime.WorkspaceConfig, threadID string, profile *runtime.ModelProfileConfig) *agentcore.Agent {
			modelCfg := modelConfigFromProfile(profile)
			workspaceRoot := workspace.WorkspaceRoot
			if workspaceRoot == "" {
				workspaceRoot = defaultWorkspace
			}
			recallManager, recallErr := memory.NewRecallManager(workspaceRoot, llm.Config{
				APIKey: modelCfg.APIKey, BaseURL: modelCfg.BaseURL, Model: modelCfg.Model,
				Protocol: llm.Protocol(modelCfg.Protocol), Temperature: modelCfg.Temperature,
				ProviderID: modelCfg.ProviderID, ProfileID: modelCfg.ProfileID, CredentialRevision: modelCfg.CredentialRevision,
				MaxTokens: modelCfg.MaxTokens, AuthScheme: modelCfg.AuthScheme, ContextWindow: modelCfg.ContextWindow,
			})
			if recallErr != nil {
				log.Printf("[memory] automatic recall disabled: %v", recallErr)
			}
			agentCfg := agentcore.AgentConfig{
				MaxThoughts: 0,
				EntryPoint:  "awaken-core",
				ModelConfig: modelCfg,
				SkillSummary: skills.FormatRuntimeSkillSummary(
					skillRegistry.PreviewLocalSources(skills.LocalSourceDirs(workspaceRoot)),
				),
				MemoryContextProvider: func() (string, error) {
					if recallManager != nil {
						return recallManager.Context()
					}
					return memory.BuildIndexContext(workspaceRoot)
				},
				MemoryRecallStart: func(ctx context.Context, query string, recentTools []string) {
					if recallManager != nil {
						recallManager.StartTurnWithTools(ctx, query, recentTools)
					}
				},
				MemoryRecallSetAuthority: func(authority agentcore.ModelCallAuthority) {
					if recallManager != nil {
						recallManager.SetModelInvoker(func(ctx context.Context, cfg llm.Config, req llm.Request, retryIndex int, auxiliaryFallback bool) (*llm.Response, error) {
							if strings.TrimSpace(req.AffinityScopeID) == "" {
								req.AffinityScopeID = authority.ThreadID
							}
							return agentcore.InvokeRecordedModel(ctx, authority, cfg, req, retryIndex, auxiliaryFallback)
						})
					}
				},
				MemoryRecallEnd: func() {
					if recallManager != nil {
						recallManager.EndTurn()
					}
				},
				MemoryRecallDrain: func() []agentcore.MemoryRecallReceipt {
					if recallManager == nil {
						return nil
					}
					receipts := recallManager.DrainPromotedReceipts()
					result := make([]agentcore.MemoryRecallReceipt, 0, len(receipts))
					for _, receipt := range receipts {
						result = append(result, agentcore.MemoryRecallReceipt{MemoryID: receipt.MemoryID, Scope: string(receipt.Scope), Type: string(receipt.Type), Revision: receipt.Revision, Bytes: receipt.Bytes})
					}
					return result
				},
				MemoryRecallReset: func() {
					if recallManager != nil {
						recallManager.ResetAfterCompaction()
					}
				},
				Workspace: agentcore.WorkspaceConfig{
					ScopeID:           workspace.ScopeID,
					Root:              workspace.WorkspaceRoot,
					ReadableRoots:     workspace.ReadableRoots,
					WritableRoots:     workspace.WritableRoots,
					OutputRoots:       workspace.OutputRoots,
					AttachedFiles:     workspace.AttachedFiles,
					ProtectedRoots:    workspace.ProtectedRoots,
					TemporaryWorkRoot: workspace.TemporaryWorkRoot,
				},
				ExecutionProfile:       workspace.ExecutionProfile,
				ModelContentStore:      runtime.NewToolResultStore(),
				ToolResultOffloadStore: runtime.NewToolResultStore(),
				ContextEpochStore:      runtime.NewContextEpochStore(),
			}
			return agentcore.NewAgentWithThreadID(agentCfg, threadID)
		},
		func(agent *agentcore.Agent) runtime.Executor {
			return runtime.ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
				return agent.RunStreamWithEvents(ctx, input, sink)
			})
		},
	)
	runService.SetSkillActivator(skillManager.Activator())
	skillManager.SetOnActivated(func(plan *skillruntime.SkillActivationPlan) {
		if plan == nil {
			return
		}
		log.Printf("[skills] activated %s status=%s tools=%d receipts=%d",
			plan.SkillID, plan.CompatibilityStatus, len(plan.EffectiveTools), len(plan.ResourceReceipts))
	})
	// Core supports starting without any workspace; runs still require one at
	// StartRun time. Only configure the service default when the host
	// provided a real path.
	if defaultWorkspace != "" {
		if err := runService.SetDefaultWorkspace(runtime.WorkspaceConfig{WorkspaceRoot: defaultWorkspace}); err != nil {
			log.Fatalf("failed to configure default workspace: %v", err)
		}
	}
	cleanupCtx, cancelCleanup := context.WithCancel(context.Background())
	defer cancelCleanup()
	if projectService := runService.ProjectService(); projectService != nil {
		go projectService.RunCleanupWorker(cleanupCtx)
	}

	srv := &http.Server{
		Addr: addr,
	}

	shutdownCh := make(chan struct{}, 1)
	shutdownOnce := sync.OnceFunc(func() {
		shutdownCh <- struct{}{}
	})

	srv.Handler = server.NewRouterWithWorkspaceSkillSyncAndACP(
		runService,
		infoProvider,
		manager,
		connRegistry,
		skillRegistry,
		acpManager,
		defaultWorkspace,
		syncSkillsForWorkspace,
		shutdownOnce,
	)

	go func() {
		log.Printf("awaken-core listening on http://%s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("awaken-core server error: %v", err)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	select {
	case <-sigCh:
	case <-shutdownCh:
	}

	log.Println("awaken-core shutting down")
	cancelCleanup()
	bashtool.CancelAllTasks()
	if acpManager != nil {
		acpCtx, acpCancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer acpCancel()
		if err := acpManager.CloseAll(acpCtx); err != nil {
			log.Printf("awaken-core acp close error: %v", err)
		}
	}
	if err := runService.FlushTranscripts(); err != nil {
		log.Printf("awaken-core flush transcripts error: %v", err)
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("awaken-core shutdown error: %v", err)
	}
}

func modelConfigFromProfile(profile *runtime.ModelProfileConfig) agentcore.ModelConfig {
	if profile == nil {
		return agentcore.ModelConfig{}
	}
	return agentcore.ModelConfig{
		APIKey:             profile.APIKey,
		BaseURL:            profile.BaseURL,
		Model:              profile.ModelID,
		Protocol:           profile.Protocol,
		ProviderID:         profile.ProviderID,
		ProfileID:          profile.ProfileID,
		CredentialRevision: profile.CredentialRevision,
		Temperature:        profile.Temperature,
		MaxTokens:          profile.MaxTokens,
		AuthScheme:         profile.AuthScheme,
		ContextWindow:      profile.ContextWindow,
		Capabilities: tool.ModelCapabilities{
			SupportsVision:    profile.SupportsVision,
			SupportsNativePDF: profile.SupportsNativePDF,
		},
	}
}

// buildMCPManager loads project and user MCP configs, merges them, and
// starts the manager. Failures are logged but never fatal: the runtime
// stays up even if no MCP server is configured. The manager is still returned
// with an empty server set so connectors can inject MCP configs dynamically.
func buildMCPManager(workspace string) *mcpclient.Manager {
	userCfg, err := mcpclient.LoadUserConfig()
	if err != nil {
		log.Printf("[mcp] user config load failed: %v", err)
	}
	var projectCfg mcpclient.ConfigFile
	if workspace != "" {
		projectCfg, err = mcpclient.LoadProjectConfig(workspace)
		if err != nil {
			log.Printf("[mcp] project config load failed: %v", err)
		}
	}
	merged := mcpclient.MergeScoped(
		mcpclient.ToScoped(userCfg, mcpclient.ScopeUser),
		mcpclient.ToScoped(projectCfg, mcpclient.ScopeProject),
		nil,
		mcpclient.StdLogger{},
	)
	manager := mcpclient.NewManager(mcpclient.WithLogger(mcpclient.StdLogger{}))
	manager.SetCredentialStore(mcpclient.NewPersistentCredentialStore())
	if len(merged) > 0 {
		go manager.Start(context.Background(), merged)
	}
	return manager
}

// buildConnectorRegistry loads the connector sources (builtin + local) and
// returns a connector registry backed by the MCP manager. Returns nil when
// no MCP manager is available, since connector lifecycle depends on it.
func buildConnectorRegistry(manager *mcpclient.Manager, _ string, legacyCLIReview []string) *connectors.Registry {
	if manager == nil {
		return nil
	}
	ctx := context.Background()

	localSource, err := connectors.NewLocalSource(connectors.StdLogger{})
	if err != nil {
		log.Printf("[connectors] local source load failed: %v", err)
	}

	var sources []connectors.ConnectorSource
	sources = append(sources, connectors.NewBuiltinSource())
	if localSource != nil {
		sources = append(sources, localSource)
	}
	sources = append(sources, connectors.NewNoopTeamSource())

	logger := connectors.StdLogger{}
	registry := connectors.NewRegistry(manager, logger)
	registry.SetCLIControlToken(os.Getenv("AWAKEN_DESKTOP_CONTROL_TOKEN"))
	if err := registry.LoadSources(ctx, sources); err != nil {
		log.Printf("[connectors] load sources failed: %v", err)
	}
	registry.SetLegacyCLIReviewRequired(legacyCLIReview)
	userCfg, err := mcpclient.LoadUserConfig()
	if err != nil {
		log.Printf("[connectors] user mcp config load failed: %v", err)
	} else {
		registry.RestoreBuiltinOwnership(userCfg)
	}

	log.Printf("[connectors] loaded %d connector definitions", len(registry.List()))
	return registry
}

func buildSkillRegistry(manager *mcpclient.Manager, workspace string) *skills.SkillRegistry {
	logger := mcpclient.StdLogger{}
	registry := skills.NewSkillRegistry(logger)
	if home, err := os.UserHomeDir(); err == nil {
		registry.SetDisabledStorePath(filepath.Join(home, ".awaken", "skills", "disabled-skills.json"))
	}
	registry.LoadFromSources(skills.LocalSourceDirs(workspace))

	if manager != nil {
		bridge := skills.NewBridge(logger, manager)
		go bridge.Start(context.Background(), manager, registry)
	}

	log.Printf("[skills] loaded %d skill definitions", len(registry.ListAll()))
	return registry
}

// buildACPManager loads the user-scoped runner config and starts the ACP manager.
// The manager is created even if no runners are configured so routes degrade
// gracefully. ACP runners are intentionally user-scoped only; project-level
// runner configs are not loaded or merged.
func buildACPManager(defaultWorkspace string) *acp.Manager {
	_ = defaultWorkspace
	userPath := ""
	if home := desktopdata.AwakenHomeRoot(); home != "" {
		userPath = acp.ConfigPath(home)
	}
	registry := acp.NewRegistry(userPath)
	if err := registry.Load(); err != nil {
		log.Printf("[acp] registry load failed: %v", err)
	}
	manager := acp.NewManager(registry, func(ctx context.Context, snapshot acp.RunnerSnapshot, cwd string) (acp.Client, error) {
		return protocol.NewClient(ctx, snapshot, cwd)
	})
	manager.SetPermissionAwaiter(acp.NewChanPermissionAwaiter())
	log.Printf("[acp] loaded %d runners", len(registry.All()))
	return manager
}

// resolveDefaultWorkspace returns the startup default workspace. When the
// host sets AWAKEN_DEFAULT_WORKSPACE (even to an empty value) that value is
// authoritative: an empty value means "no default workspace" and Core starts
// without one instead of guessing cwd or home. The cwd fallback only applies
// to standalone launches where the variable is not set at all.
func resolveDefaultWorkspace() string {
	if defaultWorkspace, ok := os.LookupEnv("AWAKEN_DEFAULT_WORKSPACE"); ok {
		return strings.TrimSpace(defaultWorkspace)
	}

	cwd, err := os.Getwd()
	if err != nil {
		log.Fatalf("failed to get working directory: %v", err)
	}
	return cwd
}
