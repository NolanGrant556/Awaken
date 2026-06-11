package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
)

var (
	ErrMissingAPIKey = errors.New("LLM_API_KEY is not set in environment")
	ErrMissingModel  = errors.New("model is not specified and no default model configured")
	ErrRequestFailed = errors.New("LLM request failed")
	ErrEmptyResponse = errors.New("received empty response from LLM")
)

type Config struct {
	APIKey  string
	BaseURL string
	Model   string
}

type Request struct {
	SystemPrompt string
	UserPrompt   string
	Model        string
	Temperature  float64
	MaxTokens    int
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model       string         `json:"model"`
	Messages    []chatMessage  `json:"messages"`
	Temperature float64        `json:"temperature,omitempty"`
	MaxTokens   int            `json:"max_tokens,omitempty"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicRequest struct {
	Model       string              `json:"model"`
	Messages    []anthropicMessage  `json:"messages"`
	Temperature float64             `json:"temperature,omitempty"`
	MaxTokens   int                 `json:"max_tokens,omitempty"`
}

type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

func LoadConfig() Config {
	loadEnvFile()

	apiKey := os.Getenv("LLM_API_KEY")
	baseURL := os.Getenv("LLM_BASE_URL")
	model := os.Getenv("LLM_MODEL_ID")

	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}

	return Config{
		APIKey:  apiKey,
		BaseURL: strings.TrimSuffix(baseURL, "/"),
		Model:   model,
	}
}

func loadEnvFile() {
	file, err := os.Open(".env")
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") || !strings.Contains(line, "=") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		value = strings.TrimPrefix(value, `"`)
		value = strings.TrimSuffix(value, `"`)

		os.Setenv(key, value)
	}
}

func Call(ctx context.Context, cfg Config, req Request) (string, error) {
	if cfg.APIKey == "" {
		return "", ErrMissingAPIKey
	}

	model := req.Model
	if model == "" {
		model = cfg.Model
	}
	if model == "" {
		return "", ErrMissingModel
	}

	temperature := req.Temperature
	if temperature == 0 {
		temperature = 0.7
	}

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 1024
	}

	isAnthropic := strings.Contains(cfg.BaseURL, "/anthropic")

	var url string
	var body []byte
	var err error

	if isAnthropic {
		url = cfg.BaseURL + "/v1/messages"

		messages := []anthropicMessage{}
		if req.SystemPrompt != "" {
			messages = append(messages, anthropicMessage{Role: "system", Content: req.SystemPrompt})
		}
		messages = append(messages, anthropicMessage{Role: "user", Content: req.UserPrompt})

		anthropicReq := anthropicRequest{
			Model:       model,
			Messages:    messages,
			Temperature: temperature,
			MaxTokens:   maxTokens,
		}

		body, err = json.Marshal(anthropicReq)
		if err != nil {
			return "", fmt.Errorf("failed to marshal request: %w", err)
		}
	} else {
		url = cfg.BaseURL + "/chat/completions"

		messages := []chatMessage{}
		if req.SystemPrompt != "" {
			messages = append(messages, chatMessage{Role: "system", Content: req.SystemPrompt})
		}
		messages = append(messages, chatMessage{Role: "user", Content: req.UserPrompt})

		chatReq := chatRequest{
			Model:       model,
			Messages:    messages,
			Temperature: temperature,
			MaxTokens:   maxTokens,
		}

		body, err = json.Marshal(chatReq)
		if err != nil {
			return "", fmt.Errorf("failed to marshal request: %w", err)
		}
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrRequestFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%w: status %d", ErrRequestFailed, resp.StatusCode)
	}

	if isAnthropic {
		var anthropicResp anthropicResponse
		if err := json.NewDecoder(resp.Body).Decode(&anthropicResp); err != nil {
			return "", fmt.Errorf("failed to decode response: %w", err)
		}

		for _, c := range anthropicResp.Content {
			if c.Type == "text" && c.Text != "" {
				return c.Text, nil
			}
		}

		return "", ErrEmptyResponse
	} else {
		var chatResp chatResponse
		if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
			return "", fmt.Errorf("failed to decode response: %w", err)
		}

		if len(chatResp.Choices) == 0 || chatResp.Choices[0].Message.Content == "" {
			return "", ErrEmptyResponse
		}

		return chatResp.Choices[0].Message.Content, nil
	}
}
