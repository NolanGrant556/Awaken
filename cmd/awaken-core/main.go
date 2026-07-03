package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"awaken/src/agentcore"
	"awaken/src/connectors"
	"awaken/src/mcpclient"
	"awaken/src/runtime"
	"awaken/src/server"
	"awaken/src/skillruntime"
	"awaken/src/skills"
	_ "awaken/src/tools"
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

	runService := runtime.NewRunServiceWithModelProfiles(
		func(workspace runtime.WorkspaceConfig, threadID string, profile *runtime.ModelProfileConfig) *agentcore.Agent {
			modelCfg := modelConfigFromProfile(profile)
			agentCfg := agentcore.AgentConfig{
				MaxThoughts: 0,
				EntryPoint:  "awaken-core",
				ModelConfig: modelCfg,
				Workspace: agentcore.WorkspaceConfig{
					Root:          workspace.WorkspaceRoot,
					WritableRoots: workspace.WritableRoots,
				},
			}
			return agentcore.NewAgentWithThreadID(agentCfg, threadID)
		},
		func(agent *agentcore.Agent) runtime.Executor {
			return runtime.ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
				return agent.RunStreamWithEvents(ctx, input, sink)
			})
		},
	)
	if err := runService.SetDefaultWorkspace(runtime.WorkspaceConfig{WorkspaceRoot: defaultWorkspace}); err != nil {
		log.Fatalf("failed to configure default workspace: %v", err)
	}

	srv := &http.Server{
		Addr:    addr,
		Handler: server.NewRouterWithMCPConnectorsAndSkills(runService, infoProvider, manager, connRegistry, skillRegistry),
	}

	log.Printf("awaken-core listening on http://%s", addr)
	log.Fatal(srv.ListenAndServe())
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
	}
}

// buildMCPManager loads project and user MCP configs, merges them, and
// starts the manager. Failures are logged but never fatal: the runtime
// stays up even if no MCP server is configured.
func buildMCPManager(workspace string) *mcpclient.Manager {
	userCfg, err := mcpclient.LoadUserConfig()
	if err != nil {
		log.Printf("[mcp] user config load failed: %v", err)
	}
	projectCfg, err := mcpclient.LoadProjectConfig(workspace)
	if err != nil {
		log.Printf("[mcp] project config load failed: %v", err)
	}
	merged := mcpclient.MergeScoped(
		mcpclient.ToScoped(userCfg, mcpclient.ScopeUser),
		mcpclient.ToScoped(projectCfg, mcpclient.ScopeProject),
		nil,
		mcpclient.StdLogger{},
	)
	if len(merged) == 0 {
		return nil
	}
	manager := mcpclient.NewManager(mcpclient.WithLogger(mcpclient.StdLogger{}))
	go manager.Start(context.Background(), merged)
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

	log.Printf("[connectors] loaded %d connector definitions", len(registry.List()))
	return registry
}

func buildSkillRegistry(manager *mcpclient.Manager, workspace string) *skills.SkillRegistry {
	logger := mcpclient.StdLogger{}
	registry := skills.NewSkillRegistry(logger)
	dirs := map[skills.SourceOrigin]string{
		skills.SourceProject: filepath.Join(workspace, ".awaken", "skills"),
	}
	if home, err := os.UserHomeDir(); err == nil {
		dirs[skills.SourceUser] = filepath.Join(home, ".awaken", "skills")
	}
	registry.LoadFromSources(dirs, skills.LoadBundledSkills())

	skillManager := skillruntime.NewManager(registry, skillruntime.StdLogger{})
	skillManager.RefreshTools()

	if manager != nil {
		bridge := skills.NewBridge(logger, manager)
		go bridge.Start(context.Background(), manager, registry)
	}

	log.Printf("[skills] loaded %d skill definitions", len(registry.ListAll()))
	return registry
}

func resolveDefaultWorkspace() string {
	defaultWorkspace := os.Getenv("AWAKEN_DEFAULT_WORKSPACE")
	if defaultWorkspace != "" {
		return defaultWorkspace
	}

	cwd, err := os.Getwd()
	if err != nil {
		log.Fatalf("failed to get working directory: %v", err)
	}
	return cwd
}
