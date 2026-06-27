package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"awaken/src/agentcore"
	"awaken/src/llm"
	"awaken/src/memory"
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
	memoryStore, err := memory.NewDefaultStore()
	if err != nil {
		log.Printf("memory store disabled: %v", err)
	}

	runService := runtime.NewRunService(
		func(workspace runtime.WorkspaceConfig, threadID string) *agentcore.Agent {
			agentCfg := agentcore.AgentConfig{
				MaxThoughts: 5,
				EntryPoint:  "awaken-core",
				ModelConfig: agentcore.ModelConfig{
					APIKey:      cfg.APIKey,
					BaseURL:     cfg.BaseURL,
					Model:       cfg.Model,
					Temperature: cfg.Temperature,
					MaxTokens:   cfg.MaxTokens,
					AuthScheme:  cfg.AuthScheme,
				},
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
		Handler: server.NewRouterWithMemory(runService, infoProvider, memoryStore),
	}

	log.Printf("awaken-core listening on http://%s", addr)
	log.Fatal(srv.ListenAndServe())
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
