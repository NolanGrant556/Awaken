package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
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
	Stream       bool
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
	Stream      bool           `json:"stream,omitempty"`
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
	Stream      bool                `json:"stream,omitempty"`
}

type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

type StreamCallback func(content string, isThinking bool)

type streamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
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

func newHTTPClient() *http.Client {
	proxyURL := os.Getenv("HTTP_PROXY")
	if proxyURL == "" {
		proxyURL = os.Getenv("http_proxy")
	}

	if proxyURL != "" {
		parsedURL, err := url.Parse(proxyURL)
		if err == nil {
			host := parsedURL.Host
			if host != "" {
				// 尝试连接代理，超时 2 秒
				conn, err := net.DialTimeout("tcp", host, 2*time.Second)
				if err == nil {
					conn.Close()
					// 代理可用，使用代理
					return &http.Client{
						Transport: &http.Transport{
							Proxy: http.ProxyURL(parsedURL),
						},
						Timeout: 120 * time.Second,
					}
				}
			}
		}
	}

	// 代理不可用或未配置，使用直连
	return &http.Client{
		Transport: &http.Transport{
			Proxy: nil,
		},
		Timeout: 120 * time.Second,
	}
}

func Call(ctx context.Context, cfg Config, req Request) (string, error) {
	if req.Stream {
		var result strings.Builder
		callback := func(content string, isThinking bool) {
			result.WriteString(content)
		}
		err := StreamCall(ctx, cfg, req, callback)
		return result.String(), err
	}

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

	resp, err := newHTTPClient().Do(httpReq)
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

func StreamCall(ctx context.Context, cfg Config, req Request, callback StreamCallback) error {
	if cfg.APIKey == "" {
		return ErrMissingAPIKey
	}

	model := req.Model
	if model == "" {
		model = cfg.Model
	}
	if model == "" {
		return ErrMissingModel
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
			Stream:      true,
		}

		body, err = json.Marshal(anthropicReq)
		if err != nil {
			return fmt.Errorf("failed to marshal request: %w", err)
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
			Stream:      true,
		}

		body, err = json.Marshal(chatReq)
		if err != nil {
			return fmt.Errorf("failed to marshal request: %w", err)
		}
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	resp, err := newHTTPClient().Do(httpReq)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrRequestFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: status %d", ErrRequestFailed, resp.StatusCode)
	}

	scanner := bufio.NewScanner(resp.Body)

	var currentEvent string
	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimSpace(line)

		if line == "" {
			continue
		}

		// SSE 事件行：event: xxx
		if strings.HasPrefix(line, "event:") {
			currentEvent = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}

		// SSE 数据行：data: xxx
		if strings.HasPrefix(line, "data:") {
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))

			if data == "" || data == "[DONE]" {
				continue
			}

			// 通用 JSON 解析，提取所有可能的文本内容
			parseAndEmit(data, currentEvent, callback)
			continue
		}

		// 非 SSE 格式（OpenAI 格式直接就是 data: 行）
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				break
			}
			parseAndEmit(data, "", callback)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed to read stream: %w", err)
	}

	return nil
}

func parseAndEmit(data string, event string, callback StreamCallback) {
	var generic map[string]interface{}
	if err := json.Unmarshal([]byte(data), &generic); err != nil {
		return
	}

	// Anthropic SSE 格式检测
	if event != "" || generic["type"] == "content_block_delta" {
		// content_block_delta 类型
		if delta, ok := generic["delta"].(map[string]interface{}); ok {
			if deltaType, ok := delta["type"].(string); ok {
				switch deltaType {
				case "thinking_delta":
					if thinking, ok := delta["thinking"].(string); ok && thinking != "" {
						callback(thinking, true)
					}
				case "text_delta":
					if text, ok := delta["text"].(string); ok && text != "" {
						callback(text, false)
					}
				}
			}
		}

		// message_stop / content_block_stop 等结束事件
		if generic["type"] == "message_stop" || generic["type"] == "content_block_stop" {
			return
		}
		return
	}

	// OpenAI 格式检测
	if choices, ok := generic["choices"].([]interface{}); ok {
		for _, choice := range choices {
			if c, ok := choice.(map[string]interface{}); ok {
				if delta, ok := c["delta"].(map[string]interface{}); ok {
					if content, ok := delta["content"].(string); ok && content != "" {
						callback(content, false)
					}
				}
			}
		}
	}
}
