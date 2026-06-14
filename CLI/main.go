package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	agentcore "awaken/src/agentcore"
	llm "awaken/src/llm"
	_ "awaken/src/tools"
)

func main() {
	cfg := llm.LoadConfig()

	if cfg.APIKey == "" || cfg.APIKey == "YOUR-API-KEY" {
		fmt.Println("Error: LLM_API_KEY is not configured in .env")
		os.Exit(1)
	}

	interactiveMode(cfg)
}

func createAgent(cfg llm.Config) *agentcore.Agent {
	agentCfg := agentcore.AgentConfig{
		MaxThoughts: 5,
		EntryPoint:  "CLI",
		ModelConfig: agentcore.ModelConfig{
			APIKey:      cfg.APIKey,
			BaseURL:     cfg.BaseURL,
			Model:       cfg.Model,
			Temperature: cfg.Temperature,
			MaxTokens:   cfg.MaxTokens,
			AuthScheme:  cfg.AuthScheme,
		},
	}
	return agentcore.NewAgent(agentCfg)
}

func interactiveMode(cfg llm.Config) {
	fmt.Println("Awaken CLI - Interactive Mode")
	fmt.Println("Type 'exit' or 'quit' to exit")
	fmt.Println("Type 'clear' to clear screen")
	fmt.Println("Type 'reset' to reset conversation")
	fmt.Println("-----------------------------------")

	ag := createAgent(cfg)

	scanner := bufio.NewScanner(os.Stdin)

	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(scanner.Text())

		if strings.EqualFold(input, "exit") || strings.EqualFold(input, "quit") {
			fmt.Println("Goodbye!")
			return
		}

		if strings.EqualFold(input, "clear") {
			fmt.Print("\033[H\033[2J")
			continue
		}

		if strings.EqualFold(input, "reset") {
			ag.Reset()
			fmt.Println("Conversation reset.")
			continue
		}

		if input == "" {
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)

		fmt.Print("\nAnswer: ")
		_, err := ag.RunStream(ctx, input, newStreamPrinter())
		cancel()

		if err != nil {
			fmt.Printf("\nError: %v\n", err)
		} else {
			fmt.Println()
		}
	}
}

func newStreamPrinter() func(content string, isThinking bool) {
	thinkingActive := false
	return func(content string, isThinking bool) {
		if isThinking {
			if !thinkingActive {
				fmt.Print("\n[Thinking]\n")
				thinkingActive = true
			}
			fmt.Print(content)
			return
		}

		if thinkingActive {
			fmt.Print("\n\n")
			thinkingActive = false
		}
		fmt.Print(content)
	}
}
