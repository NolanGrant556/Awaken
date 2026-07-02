package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"awaken/src/agentcore"
	"awaken/src/llm"
	"awaken/src/runtime"
	"awaken/src/server"
	_ "awaken/src/tools"
)

func main() {
	addr := os.Getenv("AWAKEN_CORE_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8787"
	}

	cfg := llm.LoadConfig()
	defaultWorkspace := resolveDefaultWorkspace()
	infoProvider := server.NewLLMRuntimeInfoProvider(cfg.Model, cfg.BaseURL, cfg.APIKey, defaultWorkspace, string(llm.ResolveProtocol(cfg)))

	runService := runtime.NewRunServiceWithModelProfiles(
		func(workspace runtime.WorkspaceConfig, threadID string, profile *runtime.ModelProfileConfig) *agentcore.Agent {
			modelCfg := modelConfigFromLLMConfig(cfg)
			if profile != nil {
				modelCfg = modelConfigFromProfile(profile)
			}
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

	srv := &http.Server{
		Addr:    addr,
		Handler: server.NewRouter(runService, infoProvider),
	}

	log.Printf("awaken-core listening on http://%s", addr)
	log.Fatal(srv.ListenAndServe())
}

func modelConfigFromLLMConfig(cfg llm.Config) agentcore.ModelConfig {
	return agentcore.ModelConfig{
		APIKey:                cfg.APIKey,
		BaseURL:               cfg.BaseURL,
		Model:                 cfg.Model,
		Protocol:              string(llm.ResolveProtocol(cfg)),
		Temperature:           cfg.Temperature,
		MaxTokens:             cfg.MaxTokens,
		AuthScheme:            cfg.AuthScheme,
		ContextWindow:         cfg.ContextWindow,
		AutoCompactTokenLimit: cfg.AutoCompactTokenLimit,
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
	}
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
