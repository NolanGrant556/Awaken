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
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatalf("failed to get working directory: %v", err)
	}
	infoProvider := server.NewLLMRuntimeInfoProvider(cfg.Model, cfg.BaseURL, cfg.APIKey, cwd, string(llm.ResolveProtocol(cfg)))

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
				return agent.RunStreamWithEvents(ctx, input, nil, sink)
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
