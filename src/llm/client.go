package llm

import (
	"bufio"
	stdctx "context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	ctx "awaken/src/context"
	"awaken/src/providererrors"
	"awaken/src/usage"
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
// 当设置了 AWAKEN_CONFIG_DIR 环境变量时，从该目录读取配置文件；
// 否则从当前工作目录读取（向后兼容）。
func LoadConfig() Config {
	configDir := os.Getenv("AWAKEN_CONFIG_DIR")
	envPath := ".env"
	configPath := "config.toml"
	if configDir != "" {
		envPath = filepath.Join(configDir, ".env")
		configPath = filepath.Join(configDir, "config.toml")
	}
	loadEnvFile(envPath)
	fileCfg := loadConfigFile(configPath)

	profiles := resolveProfiles(fileCfg)
	requestedActiveProfile := firstNonEmpty(os.Getenv("LLM_ACTIVE_PROFILE"), fileCfg.LLM.ActiveProfile)
	activeProfile := resolveActiveProfileName(profiles, requestedActiveProfile)
	selectedProfile := profileByName(profiles, activeProfile)

	apiKey := resolveProfileAPIKey(selectedProfile)
	baseURL := firstNonEmpty(selectedProfile.BaseURL, fileCfg.LLM.BaseURL, os.Getenv("LLM_BASE_URL"))
	model := firstNonEmpty(selectedProfile.Model, fileCfg.LLM.ModelID, os.Getenv("LLM_MODEL_ID"))
	protocol := firstNonEmpty(string(selectedProfile.Protocol), fileCfg.LLM.Protocol, os.Getenv("LLM_PROTOCOL"))
	authScheme := firstNonEmpty(selectedProfile.AuthScheme, fileCfg.LLM.AuthScheme, os.Getenv("LLM_AUTH_SCHEME"))
	temperature := fileCfg.Model.Temperature
	maxTokens := fileCfg.Model.MaxOutputTokens

	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}

	protocolValue := Protocol(protocol)
	resolvedProfiles := materializeProfiles(
		profiles,
		activeProfile,
		baseURL,
		model,
		protocolValue,
		authScheme,
	)

	return Config{
		APIKey:                apiKey,
		BaseURL:               normalizeBaseURL(baseURL),
		Model:                 model,
		Protocol:              protocolValue,
		Temperature:           temperature,
		MaxTokens:             maxTokens,
		ContextWindow:         fileCfg.Model.ContextWindow,
		AutoCompactTokenLimit: fileCfg.Model.AutoCompactTokenLimit,
		AuthScheme:            authScheme,
		ActiveProfile:         activeProfile,
		Profiles:              resolvedProfiles,
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

func loadEnvFile(path string) {
	file, err := os.Open(path)
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

		// An empty value in .env (e.g. `LLM_API_KEY=`) must not clobber a
		// non-empty value already present in the process environment (such as a
		// real key injected by the parent process). Skip in that case.
		if value == "" && os.Getenv(key) != "" {
			continue
		}

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
	body := readErrorBody(resp.Body)
	classified := providererrors.Classify(providererrors.ClassificationInput{
		StatusCode: resp.StatusCode,
		Body:       string(body),
	})
	return &RequestError{
		StatusCode:     resp.StatusCode,
		Kind:           errorKindFromProviderReason(classified.Reason, resp.StatusCode, string(body)),
		ProviderReason: string(classified.Reason),
		RetryableHint:  classified.Retryable,
		Message:        classified.Message,
	}
}

func readErrorBody(body io.Reader) []byte {
	if body == nil {
		return nil
	}
	data, _ := io.ReadAll(io.LimitReader(body, 64*1024))
	io.Copy(io.Discard, body)
	return data
}

// transportError 返回脱敏的 transport 错误，不暴露完整 URL 或 url.Error 细节。
func transportError(err error) error {
	return &RequestError{
		Kind:    ErrorKindTransport,
		Message: "transport error",
	}
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
	resp, err := withLLMRetry(ctx, cfg, req, func() (*Response, error) {
		return transport.Invoke(ctx, cfg, req)
	})
	if err != nil {
		return nil, err
	}
	return attachUsageMetadata(cfg, req, resp), nil
}

// StreamCall 流式调用（兼容旧 API）
func StreamCall(ctx stdctx.Context, cfg Config, req Request, callback StreamCallback) error {
	_, err := StreamInvoke(ctx, cfg, req, callback)
	return err
}

// StreamInvoke 流式调用，返回完整响应
func StreamInvoke(ctx stdctx.Context, cfg Config, req Request, callback StreamCallback) (*Response, error) {
	return StreamInvokeWithObserver(ctx, cfg, req, StreamObserver{OnContent: callback})
}

func StreamInvokeWithObserver(ctx stdctx.Context, cfg Config, req Request, observer StreamObserver) (*Response, error) {
	if cfg.APIKey == "" {
		return nil, ErrMissingAPIKey
	}

	model := resolveModel(cfg, req)
	if model == "" {
		return nil, ErrMissingModel
	}

	transport := getTransport(cfg)
	resp, err := withLLMRetry(ctx, cfg, req, func() (*Response, error) {
		if observerTransport, ok := transport.(observerTransport); ok {
			return observerTransport.StreamInvokeWithObserver(ctx, cfg, req, observer)
		}
		return transport.StreamInvoke(ctx, cfg, req, observer.OnContent)
	})
	if err != nil {
		return nil, err
	}
	return attachUsageMetadata(cfg, req, resp), nil
}

func attachUsageMetadata(cfg Config, req Request, resp *Response) *Response {
	if resp == nil {
		return nil
	}

	metadata := usage.Metadata{
		Provider: usage.ProviderFromBaseURL(cfg.BaseURL),
		Protocol: string(resolveProtocol(cfg)),
		Model:    resolveModel(cfg, req),
	}

	if resp.Usage == nil {
		estimate := usage.EstimateInput(req.SystemPrompt, req.UserPrompt, req.Messages, req.Tools)
		record := usage.NewEstimatedRecord(metadata, estimate)
		resp.Usage = &record
		return resp
	}

	record := usage.NormalizeRecord(*resp.Usage)
	if record.Provider == "" {
		record.Provider = metadata.Provider
	}
	if record.Protocol == "" {
		record.Protocol = metadata.Protocol
	}
	if record.Model == "" {
		record.Model = metadata.Model
	}
	resp.Usage = &record
	return resp
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
