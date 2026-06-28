package webfetch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"awaken/src/tool"
)

const (
	defaultMaxBytes = 64 * 1024
	maxBytesLimit   = 256 * 1024
	defaultTimeout  = 10 * time.Second
	maxRedirects    = 5
)

type WebFetchTool struct {
	client            *http.Client
	allowPrivateHosts bool
}

type webFetchParams struct {
	URL      string `json:"url"`
	MaxBytes int    `json:"max_bytes"`
}

type webFetchResult struct {
	Status      int    `json:"status"`
	ContentType string `json:"content_type"`
	FinalURL    string `json:"final_url"`
	BodyExcerpt string `json:"body_excerpt"`
	Truncated   bool   `json:"truncated"`
}

func (t *WebFetchTool) Name() string {
	return "web_fetch"
}

func (t *WebFetchTool) Description() string {
	return "安全读取 http/https URL，返回状态码、内容类型、最终 URL 和受限正文摘录；会拒绝本机、内网和非 Web scheme"
}

func (t *WebFetchTool) Parameters() []tool.ToolParameter {
	return []tool.ToolParameter{
		{Name: "url", Description: "要读取的 http 或 https URL", Type: "string", Required: true},
		{Name: "max_bytes", Description: "最多读取多少响应字节，默认 65536，最大 262144", Type: "integer", Required: false},
	}
}

func (t *WebFetchTool) IsReadOnly() bool {
	return true
}

func (t *WebFetchTool) Execute(ctx context.Context, input string) (string, error) {
	params, err := parseParams(input)
	if err != nil {
		return "", err
	}

	target, err := validateTargetURL(params.URL, t.allowPrivateHosts)
	if err != nil {
		return "", err
	}

	maxBytes := normalizeMaxBytes(params.MaxBytes)
	client := t.httpClient()
	reqCtx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, target.String(), nil)
	if err != nil {
		return "", fmt.Errorf("创建请求失败")
	}
	req.Header.Set("User-Agent", "Awaken-WebFetch/1.0")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml,text/plain,application/json;q=0.9,*/*;q=0.8")

	resp, err := client.Do(req)
	if err != nil {
		return "", conservativeFetchError(err)
	}
	defer resp.Body.Close()

	body, truncated, err := readBodyExcerpt(resp.Body, maxBytes)
	if err != nil {
		return "", fmt.Errorf("读取响应失败")
	}

	result := webFetchResult{
		Status:      resp.StatusCode,
		ContentType: resp.Header.Get("Content-Type"),
		FinalURL:    sanitizeURLForOutput(resp.Request.URL),
		BodyExcerpt: redactSensitiveText(body),
		Truncated:   truncated,
	}

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("序列化响应失败")
	}
	return string(data), nil
}

func (t *WebFetchTool) httpClient() *http.Client {
	if t.client != nil {
		return t.client
	}
	return newHTTPClient(t.allowPrivateHosts)
}

func newHTTPClient(allowPrivateHosts bool) *http.Client {
	dialer := &net.Dialer{Timeout: defaultTimeout, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, fmt.Errorf("invalid target address")
			}
			ip, err := resolveAllowedIP(ctx, host, allowPrivateHosts)
			if err != nil {
				return nil, err
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		},
	}

	client := &http.Client{
		Timeout:   defaultTimeout,
		Transport: transport,
	}
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= maxRedirects {
			return fmt.Errorf("too many redirects")
		}
		_, err := validateTargetURL(req.URL.String(), allowPrivateHosts)
		return err
	}
	return client
}

func parseParams(input string) (webFetchParams, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return webFetchParams{}, fmt.Errorf("缺少必需参数: url")
	}
	if !strings.HasPrefix(input, "{") {
		return webFetchParams{URL: input}, nil
	}

	var params webFetchParams
	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return webFetchParams{}, fmt.Errorf("参数格式错误，需要 JSON 对象")
	}
	if strings.TrimSpace(params.URL) == "" {
		return webFetchParams{}, fmt.Errorf("缺少必需参数: url")
	}
	return params, nil
}

func validateTargetURL(rawURL string, allowPrivateHosts bool) (*url.URL, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, fmt.Errorf("缺少必需参数: url")
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("URL 格式无效")
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return nil, fmt.Errorf("不支持的 URL scheme: %s", firstNonEmpty(scheme, "<empty>"))
	}
	if parsed.Host == "" || strings.TrimSpace(parsed.Hostname()) == "" {
		return nil, fmt.Errorf("URL 缺少 host")
	}
	if parsed.User != nil {
		return nil, fmt.Errorf("URL 不能包含用户名或密码")
	}
	if err := validateHost(parsed.Hostname(), allowPrivateHosts); err != nil {
		return nil, err
	}
	return parsed, nil
}

func validateHost(host string, allowPrivateHosts bool) error {
	if allowPrivateHosts {
		return nil
	}
	host = strings.Trim(strings.ToLower(strings.TrimSpace(host)), ".")
	if host == "" {
		return fmt.Errorf("URL 缺少 host")
	}
	if ip, ok := parseIPLiteral(host); ok {
		if isBlockedIP(ip) {
			return fmt.Errorf("拒绝访问本机、内网或 link-local 地址")
		}
		return nil
	}
	if looksNumericIPv4Host(host) {
		return fmt.Errorf("拒绝访问可疑的数字 IP host")
	}
	if isObviousPrivateHost(host) {
		return fmt.Errorf("拒绝访问明显的本机或内网 host")
	}
	return nil
}

func resolveAllowedIP(ctx context.Context, host string, allowPrivateHosts bool) (netip.Addr, error) {
	host = strings.Trim(strings.ToLower(strings.TrimSpace(host)), ".")
	if ip, ok := parseIPLiteral(host); ok {
		if !allowPrivateHosts && isBlockedIP(ip) {
			return netip.Addr{}, fmt.Errorf("拒绝访问本机、内网或 link-local 地址")
		}
		return ip, nil
	}
	if !allowPrivateHosts {
		if looksNumericIPv4Host(host) || isObviousPrivateHost(host) {
			return netip.Addr{}, fmt.Errorf("拒绝访问明显的本机或内网 host")
		}
	}

	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil || len(addrs) == 0 {
		return netip.Addr{}, fmt.Errorf("无法解析 URL host")
	}

	var selected netip.Addr
	for _, addr := range addrs {
		ip, ok := parseIPLiteral(addr.IP.String())
		if !ok {
			continue
		}
		if !allowPrivateHosts && isBlockedIP(ip) {
			return netip.Addr{}, fmt.Errorf("拒绝访问解析到本机、内网或 link-local 的 host")
		}
		if !selected.IsValid() {
			selected = ip
		}
	}
	if !selected.IsValid() {
		return netip.Addr{}, fmt.Errorf("无法解析 URL host")
	}
	return selected, nil
}

func parseIPLiteral(host string) (netip.Addr, bool) {
	ip, err := netip.ParseAddr(strings.Trim(host, "[]"))
	if err != nil {
		return netip.Addr{}, false
	}
	return ip.Unmap(), true
}

func isBlockedIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	return ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified() ||
		ip.IsMulticast()
}

func looksNumericIPv4Host(host string) bool {
	if host == "" {
		return false
	}
	for _, r := range host {
		if (r < '0' || r > '9') && r != '.' {
			return false
		}
	}
	return strings.Contains(host, ".")
}

func isObviousPrivateHost(host string) bool {
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	if !strings.Contains(host, ".") {
		return true
	}
	privateSuffixes := []string{
		".local",
		".localdomain",
		".internal",
		".intranet",
		".lan",
		".home",
		".corp",
	}
	for _, suffix := range privateSuffixes {
		if strings.HasSuffix(host, suffix) {
			return true
		}
	}
	return false
}

func normalizeMaxBytes(value int) int {
	if value <= 0 {
		return defaultMaxBytes
	}
	if value > maxBytesLimit {
		return maxBytesLimit
	}
	return value
}

func readBodyExcerpt(reader io.Reader, maxBytes int) (string, bool, error) {
	limited, err := io.ReadAll(io.LimitReader(reader, int64(maxBytes)+1))
	if err != nil {
		return "", false, err
	}
	truncated := len(limited) > maxBytes
	if truncated {
		limited = limited[:maxBytes]
	}
	return takeBytesAtRuneBoundary(limited), truncated, nil
}

func takeBytesAtRuneBoundary(data []byte) string {
	for len(data) > 0 && !utf8.Valid(data) {
		data = data[:len(data)-1]
	}
	return string(data)
}

func sanitizeURLForOutput(u *url.URL) string {
	if u == nil {
		return ""
	}
	safe := *u
	safe.User = nil
	query := safe.Query()
	for key, values := range query {
		if isSensitiveParamName(key) {
			query[key] = redactValues(values)
		}
	}
	safe.RawQuery = query.Encode()
	return safe.String()
}

func isSensitiveParamName(key string) bool {
	normalized := strings.ToLower(strings.NewReplacer("-", "", "_", "", ".", "").Replace(key))
	return strings.Contains(normalized, "token") ||
		strings.Contains(normalized, "secret") ||
		strings.Contains(normalized, "password") ||
		strings.Contains(normalized, "authorization") ||
		normalized == "apikey" ||
		normalized == "auth" ||
		normalized == "signature" ||
		normalized == "sig"
}

func redactValues(values []string) []string {
	redacted := make([]string, len(values))
	for i := range values {
		redacted[i] = "[REDACTED]"
	}
	return redacted
}

var sensitiveTextPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(authorization\s*[:=]\s*)(bearer\s+)?[^\s&<>"']+`),
	regexp.MustCompile(`(?i)((?:api[_-]?key|access[_-]?token|refresh[_-]?token|id[_-]?token|auth[_-]?token|token|secret|password)\s*[:=]\s*)[^\s&<>"']+`),
}

func redactSensitiveText(text string) string {
	for _, pattern := range sensitiveTextPatterns {
		text = pattern.ReplaceAllString(text, "${1}[REDACTED]")
	}
	return text
}

func conservativeFetchError(err error) error {
	if err == nil {
		return nil
	}
	if errorsIsPrivateHost(err) {
		return fmt.Errorf("请求被拒绝: 目标 URL 不安全")
	}
	return fmt.Errorf("请求失败: 无法获取该 URL")
}

func errorsIsPrivateHost(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	return strings.Contains(message, "内网") ||
		strings.Contains(message, "link-local") ||
		strings.Contains(message, "本机")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func init() {
	tool.MustRegisterTool(&WebFetchTool{})
}
