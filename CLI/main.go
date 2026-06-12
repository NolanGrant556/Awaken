package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	agentcore "awaken/src/agentcore"
	llm "awaken/src/llm"
	_ "awaken/src/tool"
)

func main() {
	flag.Parse()

	cfg := llm.LoadConfig()

	if cfg.APIKey == "" || cfg.APIKey == "YOUR-API-KEY" {
		fmt.Println("Error: LLM_API_KEY is not configured in .env")
		os.Exit(1)
	}

	args := flag.Args()
	if len(args) == 0 {
		interactiveMode(cfg)
	} else {
		oneShotMode(cfg, strings.Join(args, " "))
	}
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
		},
	}
	return agentcore.NewAgent(agentCfg)
}

func oneShotMode(cfg llm.Config, prompt string) {
	fmt.Printf("Question: %s\n", prompt)
	fmt.Println("Thinking...")

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	ag := createAgent(cfg)
	resp, err := ag.Run(ctx, prompt)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\nAnswer:\n%s\n", resp)
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
		_, err := ag.RunStream(ctx, input, func(content string) {
			fmt.Print(content)
		})
		cancel()

		if err != nil {
			fmt.Printf("\nError: %v\n", err)
		} else {
			fmt.Println()
		}
	}
}
