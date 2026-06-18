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
	infoProvider := server.NewLLMRuntimeInfoProvider(cfg.Model, cfg.BaseURL, cfg.APIKey, cwd)

	runService := runtime.NewRunService(func() runtime.Executor {
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
		}

		return runtime.ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			agent := agentcore.NewAgent(agentCfg)
			return agent.RunStreamWithEvents(ctx, input, nil, sink)
		})
	})

	srv := &http.Server{
		Addr:    addr,
		Handler: server.NewRouter(runService, infoProvider),
	}

	log.Printf("awaken-core listening on http://%s", addr)
	log.Fatal(srv.ListenAndServe())
}
