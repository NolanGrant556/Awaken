package networkroute

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	SystemSnapshotEnvironment   = "AWAKEN_SYSTEM_PROXY_SNAPSHOT"
	ProxyPolicyEnvironment      = "AWAKEN_PROXY_POLICY"
	snapshotSchemaVersion       = 1
	defaultProbeTimeout         = 250 * time.Millisecond
	defaultProbeTTL             = 2 * time.Second
	defaultProxyFailureCooldown = 30 * time.Second
)

type Policy string

const (
	PolicyAuto   Policy = "auto"
	PolicyProxy  Policy = "proxy"
	PolicyDirect Policy = "direct"
)

var ErrProxyRequired = errors.New("network proxy is required but unavailable")

type Route struct {
	Proxies []string `json:"proxies"`
	Direct  bool     `json:"direct"`
}

type SystemSnapshot struct {
	SchemaVersion int    `json:"schema_version"`
	Generation    int64  `json:"generation"`
	Policy        Policy `json:"policy"`
	HTTP          Route  `json:"http"`
	HTTPS         Route  `json:"https"`
	ObservedAt    string `json:"observed_at,omitempty"`
}

type endpointProbe func(context.Context, string) bool

type DialContextFunc func(context.Context, string, string) (net.Conn, error)

type probeObservation struct {
	reachable  bool
	checkedAt  time.Time
	retryAfter time.Time
}

type environmentConfig struct {
	httpProxy  *url.URL
	httpsProxy *url.URL
	allProxy   *url.URL
	noProxy    []string
}

type Manager struct {
	mu                   sync.RWMutex
	environment          environmentConfig
	system               SystemSnapshot
	policy               Policy
	policyExplicit       bool
	probe                endpointProbe
	now                  func() time.Time
	probeTTL             time.Duration
	proxyFailureCooldown time.Duration
	probeMu              sync.Mutex
	probeCache           map[string]probeObservation
}

type Option func(*Manager)

func WithEndpointProbe(probe func(context.Context, string) bool) Option {
	return func(manager *Manager) {
		if probe != nil {
			manager.probe = probe
		}
	}
}

func WithProbeTTL(ttl time.Duration) Option {
	return func(manager *Manager) {
		if ttl >= 0 {
			manager.probeTTL = ttl
		}
	}
}

func WithNow(now func() time.Time) Option {
	return func(manager *Manager) {
		if now != nil {
			manager.now = now
		}
	}
}

func WithProxyFailureCooldown(cooldown time.Duration) Option {
	return func(manager *Manager) {
		if cooldown >= 0 {
			manager.proxyFailureCooldown = cooldown
		}
	}
}

func NewFromEnvironment(entries []string, options ...Option) *Manager {
	values := environmentValues(entries)
	policy, explicit := parsePolicy(values[ProxyPolicyEnvironment])
	manager := &Manager{
		environment:          parseEnvironmentConfig(values),
		policy:               policy,
		policyExplicit:       explicit,
		probe:                dialEndpoint,
		now:                  time.Now,
		probeTTL:             defaultProbeTTL,
		proxyFailureCooldown: defaultProxyFailureCooldown,
		probeCache:           make(map[string]probeObservation),
		system: SystemSnapshot{
			SchemaVersion: snapshotSchemaVersion,
			Policy:        policy,
			HTTP:          Route{Proxies: []string{}, Direct: true},
			HTTPS:         Route{Proxies: []string{}, Direct: true},
		},
	}
	for _, option := range options {
		option(manager)
	}
	if raw := strings.TrimSpace(values[SystemSnapshotEnvironment]); raw != "" {
		if snapshot, err := ParseSystemSnapshot([]byte(raw)); err == nil {
			_, _ = manager.UpdateSystemSnapshot(snapshot)
		}
	}
	return manager
}

func parsePolicy(raw string) (Policy, bool) {
	switch Policy(strings.ToLower(strings.TrimSpace(raw))) {
	case PolicyProxy:
		return PolicyProxy, true
	case PolicyDirect:
		return PolicyDirect, true
	case PolicyAuto:
		return PolicyAuto, strings.TrimSpace(raw) != ""
	default:
		return PolicyAuto, false
	}
}

func (manager *Manager) UpdateSystemSnapshot(snapshot SystemSnapshot) (SystemSnapshot, error) {
	normalized, err := normalizeSnapshot(snapshot)
	if err != nil {
		return SystemSnapshot{}, err
	}
	manager.mu.Lock()
	if !manager.policyExplicit {
		manager.policy = normalized.Policy
	}
	normalized.Policy = manager.policy
	manager.system = normalized
	manager.mu.Unlock()
	return normalized, nil
}

func (manager *Manager) Snapshot() SystemSnapshot {
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	return cloneSnapshot(manager.system)
}

func (manager *Manager) ProxyForRequest(request *http.Request) (*url.URL, error) {
	if request == nil || request.URL == nil {
		return nil, nil
	}
	manager.mu.RLock()
	policy := manager.policy
	environment := manager.environment
	snapshot := cloneSnapshot(manager.system)
	manager.mu.RUnlock()

	if policy == PolicyDirect || shouldBypass(request.URL, environment.noProxy) {
		return nil, nil
	}

	if candidate := environment.proxyForScheme(request.URL.Scheme); candidate != nil {
		if policy != PolicyAuto || !isLoopbackProxy(candidate) || manager.endpointReachable(request.Context(), candidate) {
			return candidate, nil
		}
	}

	route := snapshot.HTTP
	if strings.EqualFold(request.URL.Scheme, "https") {
		route = snapshot.HTTPS
	}
	for _, raw := range route.Proxies {
		candidate, err := parseProxyURL(raw, false)
		if err != nil {
			continue
		}
		if policy == PolicyAuto && isLoopbackProxy(candidate) && !manager.endpointReachable(request.Context(), candidate) {
			continue
		}
		return candidate, nil
	}
	if policy == PolicyAuto {
		return nil, nil
	}
	return nil, ErrProxyRequired
}

func (manager *Manager) EnvironmentForChild(source []string) ([]string, error) {
	filtered := make([]string, 0, len(source)+8)
	for _, entry := range source {
		key, _, ok := strings.Cut(entry, "=")
		if !ok || isProxyEnvironmentKey(key) {
			continue
		}
		filtered = append(filtered, entry)
	}

	httpProxy, httpErr := manager.proxyForSyntheticURL("http://awaken-network-route.invalid")
	httpsProxy, httpsErr := manager.proxyForSyntheticURL("https://awaken-network-route.invalid")
	if httpErr != nil {
		return nil, httpErr
	}
	if httpsErr != nil {
		return nil, httpsErr
	}
	if httpProxy != nil {
		filtered = append(filtered, "HTTP_PROXY="+httpProxy.String(), "http_proxy="+httpProxy.String())
	}
	if httpsProxy != nil {
		filtered = append(filtered, "HTTPS_PROXY="+httpsProxy.String(), "https_proxy="+httpsProxy.String())
	}
	if httpProxy != nil && httpsProxy != nil && httpProxy.String() == httpsProxy.String() {
		filtered = append(filtered, "ALL_PROXY="+httpProxy.String(), "all_proxy="+httpProxy.String())
	}

	values := environmentValues(source)
	noProxy := mergeNoProxy(values["NO_PROXY"], values["no_proxy"])
	filtered = append(filtered, "NO_PROXY="+noProxy, "no_proxy="+noProxy)
	return filtered, nil
}

func (manager *Manager) proxyForSyntheticURL(raw string) (*url.URL, error) {
	request, err := http.NewRequest(http.MethodGet, raw, nil)
	if err != nil {
		return nil, err
	}
	return manager.ProxyForRequest(request)
}

func (manager *Manager) endpointReachable(ctx context.Context, proxyURL *url.URL) bool {
	address := proxyAddress(proxyURL)
	if address == "" {
		return false
	}
	now := manager.now()
	manager.probeMu.Lock()
	if observation, ok := manager.probeCache[address]; ok {
		if !observation.reachable && now.Before(observation.retryAfter) {
			manager.probeMu.Unlock()
			return false
		}
		if now.Sub(observation.checkedAt) <= manager.probeTTL {
			manager.probeMu.Unlock()
			return observation.reachable
		}
	}
	manager.probeMu.Unlock()

	reachable := manager.probe(ctx, address)
	manager.probeMu.Lock()
	manager.probeCache[address] = probeObservation{reachable: reachable, checkedAt: now}
	manager.probeMu.Unlock()
	return reachable
}

// ReportDialFailure invalidates a cached loopback observation for the failed
// address. A safe retry can then re-probe immediately instead of waiting for
// the short probe TTL to expire.
func (manager *Manager) ReportDialFailure(address string) {
	manager.probeMu.Lock()
	delete(manager.probeCache, address)
	manager.probeMu.Unlock()
}

// ReportProxyFailure marks the loopback proxy selected for request unhealthy
// for a bounded cooldown. A TCP listener can remain reachable while its HTTP
// CONNECT/TLS path is broken; auto policy may use direct routing during the
// cooldown. Proxy-only and non-loopback enterprise routes never silently fall
// back to direct.
func (manager *Manager) ReportProxyFailure(request *http.Request) bool {
	if request == nil || request.URL == nil {
		return false
	}
	manager.mu.RLock()
	policy := manager.policy
	manager.mu.RUnlock()
	if policy != PolicyAuto {
		return false
	}
	proxyURL, err := manager.ProxyForRequest(request)
	if err != nil || proxyURL == nil || !isLoopbackProxy(proxyURL) {
		return false
	}
	address := proxyAddress(proxyURL)
	if address == "" {
		return false
	}
	now := manager.now()
	manager.probeMu.Lock()
	manager.probeCache[address] = probeObservation{
		reachable:  false,
		checkedAt:  now,
		retryAfter: now.Add(manager.proxyFailureCooldown),
	}
	manager.probeMu.Unlock()
	return true
}

func dialEndpoint(ctx context.Context, address string) bool {
	dialer := net.Dialer{Timeout: defaultProbeTimeout}
	connection, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return false
	}
	_ = connection.Close()
	return true
}

func parseEnvironmentConfig(values map[string]string) environmentConfig {
	config := environmentConfig{
		httpProxy:  firstProxy(values, "HTTP_PROXY", "http_proxy"),
		httpsProxy: firstProxy(values, "HTTPS_PROXY", "https_proxy"),
		allProxy:   firstProxy(values, "ALL_PROXY", "all_proxy"),
	}
	rawNoProxy := firstNonEmpty(values["NO_PROXY"], values["no_proxy"])
	if rawNoProxy != "" {
		config.noProxy = strings.Split(rawNoProxy, ",")
	}
	return config
}

func (config environmentConfig) proxyForScheme(scheme string) *url.URL {
	if strings.EqualFold(scheme, "https") && config.httpsProxy != nil {
		return cloneURL(config.httpsProxy)
	}
	if strings.EqualFold(scheme, "http") && config.httpProxy != nil {
		return cloneURL(config.httpProxy)
	}
	return cloneURL(config.allProxy)
}

func firstProxy(values map[string]string, names ...string) *url.URL {
	for _, name := range names {
		if parsed, err := parseProxyURL(values[name], true); err == nil && parsed != nil {
			return parsed
		}
	}
	return nil
}

func parseProxyURL(raw string, allowUserInfo bool) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "" {
		return nil, errors.New("invalid proxy URL")
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https", "socks5", "socks5h":
	default:
		return nil, errors.New("unsupported proxy scheme")
	}
	if !allowUserInfo && parsed.User != nil {
		return nil, errors.New("system proxy must not contain credentials")
	}
	if parsed.Path != "" && parsed.Path != "/" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("proxy URL must not contain path, query, or fragment")
	}
	if parsed.Port() != "" {
		port, err := strconv.Atoi(parsed.Port())
		if err != nil || port < 1 || port > 65535 {
			return nil, errors.New("invalid proxy port")
		}
	}
	parsed.Path = ""
	return parsed, nil
}

func normalizeSnapshot(snapshot SystemSnapshot) (SystemSnapshot, error) {
	if snapshot.SchemaVersion != snapshotSchemaVersion {
		return SystemSnapshot{}, errors.New("unsupported network route snapshot version")
	}
	policy, _ := parsePolicy(string(snapshot.Policy))
	snapshot.Policy = policy
	var err error
	if snapshot.HTTP, err = normalizeRoute(snapshot.HTTP); err != nil {
		return SystemSnapshot{}, err
	}
	if snapshot.HTTPS, err = normalizeRoute(snapshot.HTTPS); err != nil {
		return SystemSnapshot{}, err
	}
	return snapshot, nil
}

func normalizeRoute(route Route) (Route, error) {
	if len(route.Proxies) > 8 {
		return Route{}, errors.New("too many proxy directives")
	}
	normalized := Route{Proxies: make([]string, 0, len(route.Proxies)), Direct: route.Direct}
	seen := make(map[string]struct{})
	for _, raw := range route.Proxies {
		parsed, err := parseProxyURL(raw, false)
		if err != nil || parsed == nil {
			return Route{}, errors.New("invalid system proxy directive")
		}
		value := parsed.String()
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		normalized.Proxies = append(normalized.Proxies, value)
	}
	if len(normalized.Proxies) == 0 && !normalized.Direct {
		normalized.Direct = true
	}
	return normalized, nil
}

func shouldBypass(target *url.URL, entries []string) bool {
	host := strings.TrimSuffix(strings.ToLower(target.Hostname()), ".")
	if host == "" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() || host == "localhost" {
		return true
	}
	port := target.Port()
	if port == "" {
		if strings.EqualFold(target.Scheme, "https") {
			port = "443"
		} else {
			port = "80"
		}
	}
	for _, raw := range entries {
		entry := strings.TrimSpace(strings.ToLower(raw))
		if entry == "" {
			continue
		}
		if entry == "*" {
			return true
		}
		if _, network, err := net.ParseCIDR(entry); err == nil {
			if ip := net.ParseIP(host); ip != nil && network.Contains(ip) {
				return true
			}
			continue
		}
		entryHost, entryPort := splitNoProxyEntry(entry)
		if entryPort != "" && entryPort != port {
			continue
		}
		entryHost = strings.TrimPrefix(entryHost, "*.")
		entryHost = strings.TrimPrefix(entryHost, ".")
		if host == entryHost || strings.HasSuffix(host, "."+entryHost) {
			return true
		}
	}
	return false
}

func splitNoProxyEntry(entry string) (string, string) {
	if host, port, err := net.SplitHostPort(entry); err == nil {
		return strings.Trim(host, "[]"), port
	}
	if strings.Count(entry, ":") == 1 {
		parts := strings.SplitN(entry, ":", 2)
		if _, err := strconv.Atoi(parts[1]); err == nil {
			return parts[0], parts[1]
		}
	}
	return strings.Trim(entry, "[]"), ""
}

func isLoopbackProxy(proxyURL *url.URL) bool {
	host := strings.TrimSuffix(strings.ToLower(proxyURL.Hostname()), ".")
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func proxyAddress(proxyURL *url.URL) string {
	if proxyURL == nil {
		return ""
	}
	port := proxyURL.Port()
	if port == "" {
		switch strings.ToLower(proxyURL.Scheme) {
		case "http":
			port = "80"
		case "https":
			port = "443"
		default:
			return ""
		}
	}
	return net.JoinHostPort(proxyURL.Hostname(), port)
}

func environmentValues(entries []string) map[string]string {
	values := make(map[string]string, len(entries))
	for _, entry := range entries {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			values[key] = value
		}
	}
	return values
}

func isProxyEnvironmentKey(key string) bool {
	switch strings.ToUpper(strings.TrimSpace(key)) {
	case "HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY":
		return true
	default:
		return false
	}
}

func mergeNoProxy(values ...string) string {
	seen := make(map[string]struct{})
	ordered := make([]string, 0, 8)
	for _, value := range append(values, "localhost,127.0.0.1,::1") {
		for _, item := range strings.Split(value, ",") {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			key := strings.ToLower(item)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			ordered = append(ordered, item)
		}
	}
	return strings.Join(ordered, ",")
}

func cloneURL(value *url.URL) *url.URL {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneSnapshot(snapshot SystemSnapshot) SystemSnapshot {
	snapshot.HTTP.Proxies = append([]string{}, snapshot.HTTP.Proxies...)
	snapshot.HTTPS.Proxies = append([]string{}, snapshot.HTTPS.Proxies...)
	return snapshot
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

var defaultManager atomic.Pointer[Manager]

func init() {
	defaultManager.Store(NewFromEnvironment(os.Environ()))
}

func SetDefaultManager(manager *Manager) {
	if manager != nil {
		defaultManager.Store(manager)
	}
}

func DefaultManager() *Manager {
	return defaultManager.Load()
}

func DefaultProxyForRequest(request *http.Request) (*url.URL, error) {
	return DefaultManager().ProxyForRequest(request)
}

func ReportDefaultProxyFailure(request *http.Request) bool {
	return DefaultManager().ReportProxyFailure(request)
}

func DefaultEnvironmentForChild(source []string) ([]string, error) {
	return DefaultManager().EnvironmentForChild(source)
}

func WithRouteFailureFeedback(dial DialContextFunc) DialContextFunc {
	if dial == nil {
		dialer := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
		dial = dialer.DialContext
	}
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		connection, err := dial(ctx, network, address)
		if err != nil {
			DefaultManager().ReportDialFailure(address)
		}
		return connection, err
	}
}

func InstallDefaultHTTPTransport(manager *Manager) {
	SetDefaultManager(manager)
	if transport, ok := http.DefaultTransport.(*http.Transport); ok {
		clone := transport.Clone()
		clone.Proxy = DefaultProxyForRequest
		clone.DialContext = WithRouteFailureFeedback(clone.DialContext)
		http.DefaultTransport = clone
	}
}
