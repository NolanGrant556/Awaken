package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"awaken/src/agentcore"
	"awaken/src/connectors"
	"awaken/src/mcpclient"
	"awaken/src/runtime"
	"awaken/src/server"
	"awaken/src/skillruntime"
	"awaken/src/skills"
	"awaken/src/tool"
	_ "awaken/src/tools"
	bashtool "awaken/src/tools/bash"
)

func main() {
	addr := os.Getenv("AWAKEN_CORE_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8787"
	}

	defaultWorkspace := resolveDefaultWorkspace()
	infoProvider := server.NewLLMRuntimeInfoProvider("", "", "", defaultWorkspace, "")

	manager := buildMCPManager(defaultWorkspace)
	connRegistry := buildConnectorRegistry(manager, defaultWorkspace)
	skillRegistry := buildSkillRegistry(manager, defaultWorkspace)
	syncSkillsForWorkspace := func(workspaceRoot string) {
		skillRegistry.ReloadLocalSources(skills.LocalSourceDirs(workspaceRoot))
	}
	skillManager := skillruntime.NewManager(skillRegistry, skillruntime.StdLogger{})
	skillManager.RefreshTools()

	runService := runtime.NewRunServiceWithModelProfiles(
		func(workspace runtime.WorkspaceConfig, threadID string, profile *runtime.ModelProfileConfig) *agentcore.Agent {
			modelCfg := modelConfigFromProfile(profile)
			workspaceRoot := workspace.WorkspaceRoot
			if workspaceRoot == "" {
				workspaceRoot = defaultWorkspace
			}
			agentCfg := agentcore.AgentConfig{
				MaxThoughts: 0,
				EntryPoint:  "awaken-core",
				ModelConfig: modelCfg,
				SkillSummary: skills.FormatRuntimeSkillSummary(
					skillRegistry.PreviewLocalSources(skills.LocalSourceDirs(workspaceRoot)),
				),
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
				ExecutionProfile:  workspace.ExecutionProfile,
				ModelContentStore: runtime.NewToolResultStore(),
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

	srv.Handler = server.NewRouterWithWorkspaceAndSkillSync(
		runService,
		infoProvider,
		manager,
		connRegistry,
		skillRegistry,
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
		APIKey:        profile.APIKey,
		BaseURL:       profile.BaseURL,
		Model:         profile.ModelID,
		Protocol:      profile.Protocol,
		Temperature:   profile.Temperature,
		MaxTokens:     profile.MaxTokens,
		AuthScheme:    profile.AuthScheme,
		ContextWindow: profile.ContextWindow,
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
func buildConnectorRegistry(manager *mcpclient.Manager, _ string) *connectors.Registry {
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
	if err := registry.LoadSources(ctx, sources); err != nil {
		log.Printf("[connectors] load sources failed: %v", err)
	}
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
	registry.LoadFromSources(skills.LocalSourceDirs(workspace))

	if manager != nil {
		bridge := skills.NewBridge(logger, manager)
		go bridge.Start(context.Background(), manager, registry)
	}

	log.Printf("[skills] loaded %d skill definitions", len(registry.ListAll()))
	return registry
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
