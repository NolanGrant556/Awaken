package llm

import (
	"bufio"
	stdctx "context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	ctx "awaken/src/context"
	"awaken/src/networkroute"
)

const (
	maxProviderErrorBodyBytes     = 4096
	defaultProviderRequestTimeout = 120 * time.Second
	defaultStreamHeaderTimeout    = 120 * time.Second
	defaultStreamIdleTimeout      = 90 * time.Second
)

type providerSecretPattern struct {
	replacement string
	regex       *regexp.Regexp
}

var providerErrorSecretPatterns = []providerSecretPattern{
	{
		regex:       regexp.MustCompile(`(?i)(authorization\s*[:=]\s*"?)(bearer\s+)?([^"',}\s]+)`),
		replacement: `$1$2[redacted]`,
	},
	{
		regex:       regexp.MustCompile(`(?i)(bearer\s+)(sk-[A-Za-z0-9_-]{8,}|ak-[A-Za-z0-9_-]{8,}|[A-Za-z0-9._-]{20,})`),
		replacement: `$1[redacted]`,
	},
	{
		regex:       regexp.MustCompile(`(?i)((?:api[_-]?key|x-api-key|access[_-]?token|refresh[_-]?token|session[_-]?token|id[_-]?token|token|secret)\s*[:=]\s*"?)([^"',}\s]+)`),
		replacement: `$1[redacted]`,
	},
	{
		regex:       regexp.MustCompile(`(?:sk|ak)-[A-Za-z0-9_-]{8,}`),
		replacement: `[redacted]`,
	},
}

var providerRequiredTemperaturePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)temperature.*only\s+([0-9]+(?:\.[0-9]+)?)\s+is\s+allowed`),
	regexp.MustCompile(`(?i)temperature.*(?:must\s+be|should\s+be|fixed\s+(?:value\s+)?(?:to|is)|requires?)\s+([0-9]+(?:\.[0-9]+)?)`),
}

var (
	httpClient = &http.Client{
		Timeout:   defaultProviderRequestTimeout,
		Transport: newProviderTransport(0),
	}
	streamHTTPClient = &http.Client{
		Transport: newProviderTransport(defaultStreamHeaderTimeout),
	}
)

func newProviderTransport(responseHeaderTimeout time.Duration) *http.Transport {
	return &http.Transport{
		Proxy:                 smartProxy,
		DialContext:           networkroute.WithRouteFailureFeedback((&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext),
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		MaxIdleConnsPerHost:   10,
		ResponseHeaderTimeout: responseHeaderTimeout,
	}
}

type streamIdleWatchdog struct {
	timer    *time.Timer
	timeout  time.Duration
	timedOut atomic.Bool
}

func newStreamIdleWatchdog(cancel stdctx.CancelFunc, timeout time.Duration) *streamIdleWatchdog {
	if timeout <= 0 {
		return &streamIdleWatchdog{}
	}
	watchdog := &streamIdleWatchdog{timeout: timeout}
	watchdog.timer = time.AfterFunc(timeout, func() {
		watchdog.timedOut.Store(true)
		cancel()
	})
	return watchdog
}

func (w *streamIdleWatchdog) Reset() {
	if w == nil || w.timer == nil || w.timedOut.Load() {
		return
	}
	w.timer.Reset(w.timeout)
}

func (w *streamIdleWatchdog) Stop() {
	if w == nil || w.timer == nil {
		return
	}
	w.timer.Stop()
}

func (w *streamIdleWatchdog) TimedOut() bool {
	return w != nil && w.timedOut.Load()
}

func (w *streamIdleWatchdog) Timeout() time.Duration {
	if w == nil {
		return 0
	}
	return w.timeout
}

func streamIdleTimeout() time.Duration {
	return durationFromEnvMS("AWAKEN_LLM_STREAM_IDLE_TIMEOUT_MS", defaultStreamIdleTimeout)
}

func streamIdleTimeoutForRequest(req Request) time.Duration {
	if req.StreamIdleTimeout != 0 {
		return req.StreamIdleTimeout
	}
	return streamIdleTimeout()
}

func streamIdleTimeoutForPrepared(req PreparedRequest) time.Duration {
	if req.StreamIdleTimeout != 0 {
		return req.StreamIdleTimeout
	}
	return streamIdleTimeout()
}

func durationFromEnvMS(name string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	ms, err := strconv.Atoi(raw)
	if err != nil || ms < 0 {
		return fallback
	}
	if ms == 0 {
		return 0
	}
	return time.Duration(ms) * time.Millisecond
}

func streamIdleTimeoutError(timeout time.Duration) error {
	if timeout <= 0 {
		return fmt.Errorf("%w: stream idle timeout while reading body", ErrRequestFailed)
	}
	return fmt.Errorf("%w: stream idle timeout after %s while reading body", ErrRequestFailed, timeout)
}

// smartProxy delegates to the process-wide route authority. In auto mode an
// unavailable loopback proxy is bypassed before request bytes are written;
// proxy-only mode remains fail-closed.
func smartProxy(req *http.Request) (*url.URL, error) {
	return networkroute.DefaultProxyForRequest(req)
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

	apiKey := os.Getenv("LLM_API_KEY")
	baseURL := firstNonEmpty(fileCfg.LLM.BaseURL, os.Getenv("LLM_BASE_URL"))
	model := firstNonEmpty(fileCfg.LLM.ModelID, os.Getenv("LLM_MODEL_ID"))
	protocol := firstNonEmpty(fileCfg.LLM.Protocol, os.Getenv("LLM_PROTOCOL"))
	authScheme := firstNonEmpty(fileCfg.LLM.AuthScheme, os.Getenv("LLM_AUTH_SCHEME"))
	temperature := fileCfg.Model.Temperature
	maxTokens := fileCfg.Model.MaxOutputTokens
	contextWindow := fileCfg.Model.ContextWindow
	autoCompactTokenLimit := fileCfg.Model.AutoCompactTokenLimit

	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}

	return Config{
		APIKey:                apiKey,
		BaseURL:               strings.TrimSuffix(baseURL, "/"),
		Model:                 model,
		Protocol:              Protocol(protocol),
		Temperature:           temperature,
		MaxTokens:             maxTokens,
		AuthScheme:            authScheme,
		ContextWindow:         contextWindow,
		AutoCompactTokenLimit: autoCompactTokenLimit,
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

func resolveTemperature(cfg Config, req Request) *float64 {
	if req.TemperatureOverride != nil {
		return req.TemperatureOverride
	}
	if req.OmitTemperature {
		return nil
	}
	if req.Temperature != 0 {
		return float64Pointer(req.Temperature)
	}
	if cfg.Temperature != 0 {
		return float64Pointer(cfg.Temperature)
	}
	return nil
}

func float64Pointer(value float64) *float64 {
	return &value
}

func RequiredTemperatureFromError(err error) (float64, bool) {
	if err == nil {
		return 0, false
	}
	text := err.Error()
	for _, pattern := range providerRequiredTemperaturePatterns {
		match := pattern.FindStringSubmatch(text)
		if len(match) < 2 {
			continue
		}
		value, parseErr := strconv.ParseFloat(match[1], 64)
		if parseErr == nil {
			return value, true
		}
	}
	return 0, false
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
	if protocol == ProtocolAnthropicMessages {
		httpReq.Header.Set("anthropic-version", "2023-06-01")
	}

	scheme := strings.ToLower(cfg.AuthScheme)
	if scheme == "" {
		scheme = "auto"
	}

	switch scheme {
	case "x-api-key":
		httpReq.Header.Set("x-api-key", cfg.APIKey)
		return
	case "api-key":
		httpReq.Header.Set("api-key", cfg.APIKey)
		return
	case "x-goog-api-key":
		httpReq.Header.Set("x-goog-api-key", cfg.APIKey)
		return
	case "none":
		return
	case "bearer":
		httpReq.Header.Set("Authorization", "Bearer "+cfg.APIKey)
		return
	default: // auto
		if protocol == ProtocolAnthropicMessages {
			if u, err := url.Parse(cfg.BaseURL); err == nil && u.Hostname() == "api.anthropic.com" {
				httpReq.Header.Set("x-api-key", cfg.APIKey)
				return
			}
		}
	}
	httpReq.Header.Set("Authorization", "Bearer "+cfg.APIKey)
}

func providerRequestID(resp *http.Response) string {
	if resp == nil {
		return ""
	}
	for _, name := range []string{"x-request-id", "x-ds-request-id", "request-id", "x-requestid"} {
		if value := strings.TrimSpace(resp.Header.Get(name)); value != "" {
			return value
		}
	}
	return ""
}

func providerRateLimitObservation(resp *http.Response) *RateLimitObservation {
	if resp == nil {
		return nil
	}
	header := func(names ...string) string {
		for _, name := range names {
			if value := strings.TrimSpace(resp.Header.Get(name)); value != "" {
				return value
			}
		}
		return ""
	}
	observation := &RateLimitObservation{
		RequestLimit:     header("x-ratelimit-limit-requests", "anthropic-ratelimit-requests-limit"),
		RequestRemaining: header("x-ratelimit-remaining-requests", "anthropic-ratelimit-requests-remaining"),
		RequestReset:     header("x-ratelimit-reset-requests", "anthropic-ratelimit-requests-reset"),
		TokenLimit:       header("x-ratelimit-limit-tokens", "anthropic-ratelimit-tokens-limit"),
		TokenRemaining:   header("x-ratelimit-remaining-tokens", "anthropic-ratelimit-tokens-remaining"),
		TokenReset:       header("x-ratelimit-reset-tokens", "anthropic-ratelimit-tokens-reset"),
		ObservedAt:       time.Now().UTC(),
	}
	if observation.RequestLimit == "" && observation.RequestRemaining == "" && observation.RequestReset == "" &&
		observation.TokenLimit == "" && observation.TokenRemaining == "" && observation.TokenReset == "" {
		return nil
	}
	return observation
}

func requestFailedError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxProviderErrorBodyBytes+1))
	detail := sanitizeProviderErrorDetail(body)
	return &RequestError{
		StatusCode: resp.StatusCode,
		RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After"), time.Now()),
		Detail:     detail,
	}
}

func parseRetryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		if seconds <= 0 {
			return 0
		}
		return time.Duration(seconds) * time.Second
	}
	when, err := http.ParseTime(value)
	if err != nil {
		return 0
	}
	delay := when.Sub(now)
	if delay <= 0 {
		return 0
	}
	return delay
}

func sanitizeProviderErrorDetail(body []byte) string {
	truncated := len(body) > maxProviderErrorBodyBytes
	if truncated {
		body = body[:maxProviderErrorBodyBytes]
	}

	detail := strings.TrimSpace(string(body))
	if detail == "" {
		return ""
	}
	detail = extractProviderErrorMessage(detail)
	if detail == "" {
		return ""
	}
	detail = strings.ReplaceAll(detail, "\r", " ")
	detail = strings.ReplaceAll(detail, "\n", " ")
	detail = redactProviderErrorSecrets(detail)
	if truncated {
		detail += "..."
	}
	return detail
}

func extractProviderErrorMessage(detail string) string {
	var payload interface{}
	if err := json.Unmarshal([]byte(detail), &payload); err != nil {
		return detail
	}
	return findProviderErrorMessage(payload)
}

func findProviderErrorMessage(value interface{}) string {
	switch typed := value.(type) {
	case map[string]interface{}:
		if errValue, ok := typed["error"]; ok {
			if message := findProviderErrorMessage(errValue); message != "" {
				return message
			}
		}
		for _, key := range []string{"message", "error_description"} {
			if message, ok := typed[key].(string); ok {
				return strings.TrimSpace(message)
			}
		}
	case string:
		return strings.TrimSpace(typed)
	}
	return ""
}

func redactProviderErrorSecrets(detail string) string {
	for _, pattern := range providerErrorSecretPatterns {
		detail = pattern.regex.ReplaceAllString(detail, pattern.replacement)
	}
	return detail
}

// transportError 返回脱敏的 transport 错误，不暴露完整 URL 或 url.Error 细节。
func transportError(err error) error {
	return &TransportError{}
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
	case ProtocolOpenAIResponses:
		return newOpenAIResponsesTransport()
	case ProtocolOpenAIChat, ProtocolMiniMaxChat:
		return newOpenAITransport()
	default:
		return &unsupportedTransport{protocol: cfg.Protocol}
	}
}

type unsupportedTransport struct{ protocol Protocol }

func (t *unsupportedTransport) protocolError() error {
	return fmt.Errorf("%w: %q", ErrUnsupportedProtocol, t.protocol)
}
func (t *unsupportedTransport) Invoke(stdctx.Context, Config, Request) (*Response, error) {
	return nil, t.protocolError()
}
func (t *unsupportedTransport) StreamInvoke(stdctx.Context, Config, Request, StreamCallback) (*Response, error) {
	return nil, t.protocolError()
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
	if apiKeyRequired(cfg) && cfg.APIKey == "" {
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
	return StreamInvokeWithObserver(ctx, cfg, req, StreamObserver{OnContent: callback})
}

func StreamInvokeWithObserver(ctx stdctx.Context, cfg Config, req Request, observer StreamObserver) (*Response, error) {
	if apiKeyRequired(cfg) && cfg.APIKey == "" {
		return nil, ErrMissingAPIKey
	}

	model := resolveModel(cfg, req)
	if model == "" {
		return nil, ErrMissingModel
	}

	transport := getTransport(cfg)
	if observerTransport, ok := transport.(observerTransport); ok {
		return observerTransport.StreamInvokeWithObserver(ctx, cfg, req, observer)
	}
	return transport.StreamInvoke(ctx, cfg, req, observer.OnContent)
}

func apiKeyRequired(cfg Config) bool {
	return !strings.EqualFold(strings.TrimSpace(cfg.AuthScheme), "none")
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
