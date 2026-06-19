package llm

import (
	"bufio"
	stdctx "context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	ctx "awaken/src/context"
)

var (
	httpClient = &http.Client{
		Timeout: 120 * time.Second,
		Transport: &http.Transport{
			Proxy:                 smartProxy,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			MaxIdleConnsPerHost:   10,
		},
	}
)

// smartProxy 先尝试代理，失败则直连
func smartProxy(req *http.Request) (*url.URL, error) {
	proxyURL, err := http.ProxyFromEnvironment(req)
	if err != nil {
		return nil, nil // 出错时直连
	}
	if proxyURL != nil {
		return proxyURL, nil // 使用代理
	}
	return nil, nil // 无代理配置，直连
}

// LoadConfig 从 .env 和 config.toml 加载配置。
// .env 主要承载 LLM_API_KEY；config.toml 承载非敏感模型参数。
func LoadConfig() Config {
	loadEnvFile()
	fileCfg := loadConfigFile("config.toml")

	apiKey := os.Getenv("LLM_API_KEY")
	baseURL := firstNonEmpty(fileCfg.LLM.BaseURL, os.Getenv("LLM_BASE_URL"))
	model := firstNonEmpty(fileCfg.LLM.ModelID, os.Getenv("LLM_MODEL_ID"))
	protocol := firstNonEmpty(fileCfg.LLM.Protocol, os.Getenv("LLM_PROTOCOL"))
	authScheme := firstNonEmpty(fileCfg.LLM.AuthScheme, os.Getenv("LLM_AUTH_SCHEME"))
	temperature := fileCfg.Model.Temperature
	maxTokens := fileCfg.Model.MaxOutputTokens

	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}

	return Config{
		APIKey:      apiKey,
		BaseURL:     strings.TrimSuffix(baseURL, "/"),
		Model:       model,
		Protocol:    Protocol(protocol),
		Temperature: temperature,
		MaxTokens:   maxTokens,
		AuthScheme:  authScheme,
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
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

// --- 公共辅助函数 ---

func resolveModel(cfg Config, req Request) string {
	model := req.Model
	if model == "" {
		model = cfg.Model
	}
	return model
}

func resolveTemperature(cfg Config, req Request) float64 {
	temperature := req.Temperature
	if temperature == 0 {
		temperature = cfg.Temperature
		if temperature == 0 {
			temperature = 0.7
		}
	}
	return temperature
}

func resolveMaxTokens(cfg Config, req Request) int {
	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = cfg.MaxTokens
		if maxTokens == 0 {
			maxTokens = 1024
		}
	}
	return maxTokens
}

func applyAuthHeaders(httpReq *http.Request, cfg Config, protocol Protocol) {
	httpReq.Header.Set("Content-Type", "application/json")

	scheme := strings.ToLower(cfg.AuthScheme)
	if scheme == "" {
		scheme = "auto"
	}

	useAPIKey := false
	switch scheme {
	case "x-api-key":
		useAPIKey = true
	case "bearer":
		useAPIKey = false
	default: // auto
		if protocol == ProtocolAnthropicMessages {
			if u, err := url.Parse(cfg.BaseURL); err == nil && u.Hostname() == "api.anthropic.com" {
				useAPIKey = true
			}
		}
	}

	if useAPIKey {
		httpReq.Header.Set("x-api-key", cfg.APIKey)
		httpReq.Header.Set("anthropic-version", "2023-06-01")
	} else {
		httpReq.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	}
}

func requestFailedError(resp *http.Response) error {
	// Drain body to allow connection reuse, but do not include body in error
	// message to avoid leaking provider-internal details or secrets.
	io.Copy(io.Discard, resp.Body)
	return fmt.Errorf("%w: status %d", ErrRequestFailed, resp.StatusCode)
}

// transportError 返回脱敏的 transport 错误，不暴露完整 URL 或 url.Error 细节。
func transportError(err error) error {
	return fmt.Errorf("%w: transport error", ErrRequestFailed)
}

// requestBuildError 返回脱敏的请求构造错误，不暴露完整 URL（例如 malformed base URL）。
func requestBuildError(err error) error {
	return fmt.Errorf("%w: request build error", ErrRequestFailed)
}

func getTransport(cfg Config) Transport {
	protocol := resolveProtocol(cfg)
	switch protocol {
	case ProtocolAnthropicMessages:
		return newAnthropicTransport()
	default:
		return newOpenAITransport()
	}
}

// --- 兼容 wrapper ---

// Call 同步调用（兼容旧 API）
func Call(ctx stdctx.Context, cfg Config, req Request) (string, error) {
	if req.Stream {
		var result strings.Builder
		callback := func(content string, isThinking bool) {
			result.WriteString(content)
		}
		err := StreamCall(ctx, cfg, req, callback)
		return result.String(), err
	}

	resp, err := Invoke(ctx, cfg, req)
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

// Invoke 同步调用，返回完整响应
func Invoke(ctx stdctx.Context, cfg Config, req Request) (*Response, error) {
	if cfg.APIKey == "" {
		return nil, ErrMissingAPIKey
	}

	model := resolveModel(cfg, req)
	if model == "" {
		return nil, ErrMissingModel
	}

	transport := getTransport(cfg)
	return transport.Invoke(ctx, cfg, req)
}

// StreamCall 流式调用（兼容旧 API）
func StreamCall(ctx stdctx.Context, cfg Config, req Request, callback StreamCallback) error {
	_, err := StreamInvoke(ctx, cfg, req, callback)
	return err
}

// StreamInvoke 流式调用，返回完整响应
func StreamInvoke(ctx stdctx.Context, cfg Config, req Request, callback StreamCallback) (*Response, error) {
	if cfg.APIKey == "" {
		return nil, ErrMissingAPIKey
	}

	model := resolveModel(cfg, req)
	if model == "" {
		return nil, ErrMissingModel
	}

	transport := getTransport(cfg)
	return transport.StreamInvoke(ctx, cfg, req, callback)
}

// CallWithContext 使用 Conversation Context 调用
func CallWithContext(c stdctx.Context, cfg Config, convCtx *ctx.Context, req Request) (*Response, error) {
	req.Messages = convCtx.GetMessages()
	req.Tools = convCtx.GetTools()
	return Invoke(c, cfg, req)
}

// CallWithContextStream 使用 Conversation Context 流式调用
func CallWithContextStream(c stdctx.Context, cfg Config, convCtx *ctx.Context, req Request, callback StreamCallback) error {
	req.Messages = convCtx.GetMessages()
	req.Tools = convCtx.GetTools()
	return StreamCall(c, cfg, req, callback)
}
