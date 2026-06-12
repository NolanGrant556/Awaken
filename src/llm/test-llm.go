//go:build ignore

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"awaken/src/llm"
)

func main() {
	cfg := llm.LoadConfig()

	if cfg.APIKey == "" || cfg.APIKey == "YOUR-API-KEY" {
		fmt.Println("Error: LLM_API_KEY is not configured")
		os.Exit(1)
	}

	if cfg.Model == "" || cfg.Model == "YOUR-MODEL" {
		fmt.Println("Error: LLM_MODEL_ID is not configured")
		os.Exit(1)
	}

	if cfg.BaseURL == "" || cfg.BaseURL == "YOUR-URL" {
		fmt.Println("Error: LLM_BASE_URL is not configured")
		os.Exit(1)
	}

	fmt.Printf("Config loaded:\n")
	fmt.Printf("  API Key: %s\n", maskKey(cfg.APIKey))
	fmt.Printf("  Model: %s\n", cfg.Model)
	fmt.Printf("  Base URL: %s\n", cfg.BaseURL)
	fmt.Println()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req := llm.Request{
		UserPrompt: "你好",
	}

	fmt.Println("Sending request...")
	resp, err := llm.Call(ctx, cfg, req)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\nResponse:\n%s\n", resp)
}

func maskKey(key string) string {
	if len(key) <= 8 {
		return "***"
	}
	return key[:4] + "..." + key[len(key)-4:]
}
