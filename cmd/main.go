package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"awaken/src/agent"
	"awaken/src/llm"
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

func oneShotMode(cfg llm.Config, prompt string) {
	fmt.Printf("Question: %s\n", prompt)
	fmt.Println("Thinking...")

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	req := llm.Request{
		UserPrompt: prompt,
		Model:      cfg.Model,
	}

	resp, err := llm.Call(ctx, cfg, req)
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
	fmt.Println("Type 'agent' to use ReAct agent mode")
	fmt.Println("-----------------------------------")

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

		if strings.EqualFold(input, "agent") {
			agentMode(cfg)
			fmt.Println("-----------------------------------")
			continue
		}

		if input == "" {
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)

		req := llm.Request{
			UserPrompt: input,
			Model:      cfg.Model,
			Stream:     true,
		}

		fmt.Print("\nAnswer: ")
		err := llm.StreamCall(ctx, cfg, req, func(content string, isThinking bool) {
			if isThinking {
				fmt.Printf("\r[Thinking...] %s", content)
			} else {
				fmt.Print(content)
			}
		})
		cancel()

		if err != nil {
			fmt.Printf("\nError: %v\n", err)
		} else {
			fmt.Println("\n")
		}
	}
}

func agentMode(cfg llm.Config) {
	fmt.Println("ReAct Agent Mode - Type 'back' to return")

	agentCfg := agent.AgentConfig{
		MaxThoughts: 5,
		SystemPrompt: "你是一个智能助手，能够通过思考和行动来解决问题。",
		ModelConfig: agent.LLMConfig{
			APIKey:  cfg.APIKey,
			BaseURL: cfg.BaseURL,
			Model:   cfg.Model,
		},
	}

	ag := agent.NewAgent(agentCfg)
	scanner := bufio.NewScanner(os.Stdin)

	for {
		fmt.Print("agent> ")
		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(scanner.Text())

		if strings.EqualFold(input, "back") {
			return
		}

		if input == "" {
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)

		fmt.Println("Agent is thinking...")
		resp, err := ag.Run(ctx, input)
		cancel()

		if err != nil {
			fmt.Printf("Error: %v\n", err)
		} else {
			fmt.Printf("\nResult:\n%s\n", resp)
		}

		fmt.Println("\n---")
	}
}