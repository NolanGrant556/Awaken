package webuibridge

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/url"
	"path"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	defaultPairingCodeTTL   = 5 * time.Minute
	defaultHeartbeat        = 30 * time.Second
	subscriptionBufferSize  = 256
	pairingCodeRandomBytes  = 4
	generatedIDRandomBytes  = 8
	defaultRuntimeBaseURL   = "http://127.0.0.1"
	defaultWebSocketPath    = "/ws"
	tokenDigestAlgorithmTag = "sha256:"
)

var (
	ErrInvalidPairingCode = errors.New("invalid pairing code")
	ErrPairingCodeExpired = errors.New("pairing code expired")
	ErrPairingCodeUsed    = errors.New("pairing code already used")
	ErrInvalidClient      = errors.New("invalid client")
	ErrClientRevoked      = errors.New("client revoked")
	ErrScopeDenied        = errors.New("client scope denied")
	ErrOriginDenied       = errors.New("origin denied")
	ErrHostDenied         = errors.New("host denied")
	ErrRouteDenied        = errors.New("route denied")
	ErrInvalidToken       = errors.New("invalid token")
	ErrSubscriptionClosed = errors.New("subscription closed")
)

type Service struct {
	mu            sync.Mutex
	config        BridgeConfig
	pairings      map[string]PairingCode
	clients       map[string]ClientSession
	events        []ConnectionEvent
	nextSeq       uint64
	subscriptions map[string]*subscriptionRecord
}

type subscriptionRecord struct {
	sub    Subscription
	events chan ConnectionEvent
}

func NewService(config BridgeConfig) *Service {
	normalized := normalizeConfig(config)
	return &Service{
		config:        normalized,
		pairings:      make(map[string]PairingCode),
		clients:       make(map[string]ClientSession),
		subscriptions: make(map[string]*subscriptionRecord),
	}
}

func DefaultSecurityPolicy() SecurityPolicy {
	return SecurityPolicy{
		PairingCodeTTL: defaultPairingCodeTTL,
		AllowedOrigins: []string{
			"http://127.0.0.1:*",
			"http://localhost:*",
			"http://[::1]:*",
		},
		AllowedHosts: []string{
			"127.0.0.1",
			"localhost",
			"::1",
			"[::1]",
		},
		RouteAllowlist: DefaultRuntimeRouteAllowlist(),
		WebSocketPaths: []string{
			defaultWebSocketPath,
		},
	}
}

func DefaultRuntimeRouteAllowlist() []RouteRule {
	return []RouteRule{
		{Name: "health", Method: "GET", Path: "/health", RequiredScopes: []ClientScope{ScopeRuntimeRead}},
		{Name: "runtime-info", Method: "GET", Path: "/v1/runtime/info", RequiredScopes: []ClientScope{ScopeRuntimeRead}},
		{Name: "agui-demo", Method: "GET", Path: "/v1/agui/demo", RequiredScopes: []ClientScope{ScopeRuntimeRead}},
		{Name: "list-threads", Method: "GET", Path: "/v1/threads", RequiredScopes: []ClientScope{ScopeRuntimeRead}},
		{Name: "search-threads", Method: "GET", Path: "/v1/threads/search", RequiredScopes: []ClientScope{ScopeRuntimeRead}},
		{Name: "thread-transcript", Method: "GET", PathPattern: "/v1/threads/{thread_id}/transcript", RequiredScopes: []ClientScope{ScopeRuntimeRead}},
		{Name: "thread-export", Method: "GET", PathPattern: "/v1/threads/{thread_id}/export", RequiredScopes: []ClientScope{ScopeRuntimeRead}},
		{Name: "thread-share-bundle", Method: "POST", PathPattern: "/v1/threads/{thread_id}/share-bundle", RequiredScopes: []ClientScope{ScopeRuntimeRead}},
		{Name: "thread-clear-plan", Method: "POST", PathPattern: "/v1/threads/{thread_id}/clear-plan", RequiredScopes: []ClientScope{ScopeRuntimeRead}},
		{Name: "create-run", Method: "POST", Path: "/v1/runs", RequiredScopes: []ClientScope{ScopeRuntimeWrite}},
		{Name: "get-run", Method: "GET", PathPattern: "/v1/runs/{run_id}", RequiredScopes: []ClientScope{ScopeRuntimeRead}},
		{Name: "run-events", Method: "GET", PathPattern: "/v1/runs/{run_id}/events", RequiredScopes: []ClientScope{ScopeRuntimeRead}},
		{Name: "cancel-run", Method: "POST", PathPattern: "/v1/runs/{run_id}/cancel", RequiredScopes: []ClientScope{ScopeRuntimeWrite}},
		{Name: "resolve-approval", Method: "POST", PathPattern: "/v1/runs/{run_id}/approvals/{approval_id}/resolve", RequiredScopes: []ClientScope{ScopeRuntimeWrite}},
	}
}

func (s *Service) CreatePairingCode(req PairingCodeRequest) (PairingCode, error) {
	now := s.now()
	code, err := randomHex(pairingCodeRandomBytes)
	if err != nil {
		return PairingCode{}, err
	}
	code = strings.ToUpper(code)
	scopes := normalizeScopes(req.Scopes, s.config.DefaultScopes)
	pairing := PairingCode{
		Code:       code,
		Digest:     DigestToken(normalizePairingCode(code)),
		ClientName: strings.TrimSpace(req.ClientName),
		Scopes:     scopes,
		CreatedAt:  now,
		ExpiresAt:  now.Add(s.config.Security.PairingCodeTTL),
	}

	stored := pairing
	stored.Code = ""

	s.mu.Lock()
	s.pairings[pairing.Digest] = stored
	s.mu.Unlock()

	return pairing, nil
}

func (s *Service) ExchangePairingCode(req PairingCodeExchange) (ClientSession, error) {
	now := s.now()
	digest := DigestToken(normalizePairingCode(req.Code))

	s.mu.Lock()
	pairing, ok := s.pairings[digest]
	if !ok {
		s.mu.Unlock()
		return ClientSession{}, ErrInvalidPairingCode
	}
	if pairing.UsedAt != nil {
		s.mu.Unlock()
		return ClientSession{}, ErrPairingCodeUsed
	}
	if !now.Before(pairing.ExpiresAt) {
		delete(s.pairings, digest)
		s.mu.Unlock()
		return ClientSession{}, ErrPairingCodeExpired
	}
	if err := s.config.Security.validateOriginHost(req.Origin, req.Host); err != nil {
		s.mu.Unlock()
		return ClientSession{}, err
	}

	requestedScopes := normalizeScopes(req.Scopes, pairing.Scopes)
	if !hasScopes(pairing.Scopes, requestedScopes) {
		s.mu.Unlock()
		return ClientSession{}, ErrScopeDenied
	}

	reg := ClientRegistration{
		ClientID:   req.ClientID,
		ClientName: firstNonEmpty(req.ClientName, pairing.ClientName, "paired client"),
		Token:      req.Token,
		Origin:     req.Origin,
		Host:       req.Host,
		UserAgent:  req.UserAgent,
		Scopes:     requestedScopes,
	}
	client, err := s.registerClientLocked(reg, now)
	if err != nil {
		s.mu.Unlock()
		return ClientSession{}, err
	}

	usedAt := now
	pairing.UsedAt = &usedAt
	s.pairings[digest] = pairing
	s.mu.Unlock()

	return client, nil
}

func (s *Service) RegisterClient(req ClientRegistration) (ClientSession, error) {
	now := s.now()

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.config.Security.validateOriginHost(req.Origin, req.Host); err != nil {
		return ClientSession{}, err
	}
	return s.registerClientLocked(req, now)
}

func (s *Service) RevokeClient(clientID string) (ClientSession, error) {
	now := s.now()

	s.mu.Lock()
	defer s.mu.Unlock()
	client, ok := s.clients[strings.TrimSpace(clientID)]
	if !ok {
		return ClientSession{}, ErrInvalidClient
	}
	if client.RevokedAt == nil {
		revokedAt := now
		client.RevokedAt = &revokedAt
		client.LastSeenAt = now
		s.clients[client.ID] = client
	}

	for id, rec := range s.subscriptions {
		if rec.sub.ClientID != client.ID || !rec.sub.Active {
			continue
		}
		s.closeSubscriptionLocked(id, now)
	}

	return cloneClient(client), nil
}

func (s *Service) ListClients() []ClientSession {
	s.mu.Lock()
	defer s.mu.Unlock()

	clients := make([]ClientSession, 0, len(s.clients))
	for _, client := range s.clients {
		clients = append(clients, cloneClient(client))
	}
	sort.Slice(clients, func(i, j int) bool {
		if clients[i].CreatedAt.Equal(clients[j].CreatedAt) {
			return clients[i].ID < clients[j].ID
		}
		return clients[i].CreatedAt.Before(clients[j].CreatedAt)
	})
	return clients
}

func (s *Service) BuildProxyPlan(req ProxyPlanRequest) (RouteProxyPlan, error) {
	now := s.now()
	method := strings.ToUpper(strings.TrimSpace(req.Method))
	if method == "" {
		return RouteProxyPlan{}, ErrRouteDenied
	}
	requestPath, targetPath, err := normalizeRequestPath(req.Path)
	if err != nil {
		return RouteProxyPlan{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	client, err := s.authorizeClientLocked(req.ClientID, req.Token, req.Origin, req.Host, now)
	if err != nil {
		return RouteProxyPlan{}, err
	}
	rule, ok := s.config.Security.matchRoute(method, requestPath)
	if !ok {
		return RouteProxyPlan{}, ErrRouteDenied
	}
	if !hasScopes(client.Scopes, rule.RequiredScopes) {
		return RouteProxyPlan{}, ErrScopeDenied
	}

	return RouteProxyPlan{
		ClientID:       client.ID,
		Method:         method,
		Path:           requestPath,
		TargetPath:     targetPath,
		RouteName:      rule.Name,
		RequiredScopes: copyScopes(rule.RequiredScopes),
		TokenDigest:    client.Token.Digest,
		CreatedAt:      now,
	}, nil
}

func (s *Service) BuildWebSocketPlan(req WebSocketPlanRequest) (WebSocketPlan, error) {
	now := s.now()
	requestPath, _, err := normalizeRequestPath(req.Path)
	if err != nil {
		return WebSocketPlan{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	client, err := s.authorizeClientLocked(req.ClientID, req.Token, req.Origin, req.Host, now)
	if err != nil {
		return WebSocketPlan{}, err
	}
	if !s.config.Security.allowsWebSocketPath(requestPath) {
		return WebSocketPlan{}, ErrRouteDenied
	}
	requiredScopes := []ClientScope{ScopeEventsSubscribe}
	if !hasScopes(client.Scopes, requiredScopes) {
		return WebSocketPlan{}, ErrScopeDenied
	}

	return WebSocketPlan{
		ClientID:          client.ID,
		Path:              requestPath,
		Origin:            strings.TrimSpace(req.Origin),
		Host:              strings.TrimSpace(req.Host),
		Topics:            copyStrings(req.Topics),
		AfterSeq:          req.AfterSeq,
		RequiredScopes:    requiredScopes,
		TokenDigest:       client.Token.Digest,
		HeartbeatInterval: s.config.HeartbeatInterval,
		CreatedAt:         now,
	}, nil
}

func (s *Service) Subscribe(req SubscriptionRequest) (Subscription, error) {
	now := s.now()

	s.mu.Lock()
	defer s.mu.Unlock()
	client, err := s.authorizeClientLocked(req.ClientID, req.Token, req.Origin, req.Host, now)
	if err != nil {
		return Subscription{}, err
	}
	if !hasScopes(client.Scopes, []ClientScope{ScopeEventsSubscribe}) {
		return Subscription{}, ErrScopeDenied
	}

	id, err := randomID("sub")
	if err != nil {
		return Subscription{}, err
	}
	topics := normalizeTopics(req.Topics)
	history := s.historyLocked(req.AfterSeq, topics)
	events := make(chan ConnectionEvent, subscriptionBufferSize)
	sub := Subscription{
		ID:        id,
		ClientID:  client.ID,
		Topics:    topics,
		AfterSeq:  req.AfterSeq,
		CreatedAt: now,
		Active:    true,
		History:   history,
		Events:    events,
	}
	s.subscriptions[id] = &subscriptionRecord{sub: sub, events: events}
	return cloneSubscription(sub), nil
}

func (s *Service) Unsubscribe(subscriptionID string) (Subscription, error) {
	now := s.now()

	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closeSubscriptionLocked(strings.TrimSpace(subscriptionID), now)
}

func (s *Service) RecordEvent(event ConnectionEvent) ConnectionEvent {
	now := s.now()

	s.mu.Lock()
	defer s.mu.Unlock()

	s.nextSeq++
	event.Seq = s.nextSeq
	if event.Timestamp.IsZero() {
		event.Timestamp = now
	} else {
		event.Timestamp = event.Timestamp.UTC()
	}
	event.Metadata = copyStringMap(event.Metadata)
	s.events = append(s.events, event)

	for id, rec := range s.subscriptions {
		if !rec.sub.Active || !topicMatches(rec.sub.Topics, event.Topic) {
			continue
		}
		select {
		case rec.events <- event:
		default:
			_, _ = s.closeSubscriptionLocked(id, now)
		}
	}

	return event
}

func (s *Service) registerClientLocked(req ClientRegistration, now time.Time) (ClientSession, error) {
	token := strings.TrimSpace(req.Token)
	if token == "" {
		return ClientSession{}, ErrInvalidToken
	}
	clientID := strings.TrimSpace(req.ClientID)
	if clientID == "" {
		var err error
		clientID, err = randomID("client")
		if err != nil {
			return ClientSession{}, err
		}
	}
	if _, exists := s.clients[clientID]; exists {
		return ClientSession{}, ErrInvalidClient
	}

	scopes := normalizeScopes(req.Scopes, s.config.DefaultScopes)
	client := ClientSession{
		ID:         clientID,
		Name:       firstNonEmpty(req.ClientName, "web client"),
		UserAgent:  strings.TrimSpace(req.UserAgent),
		Origin:     strings.TrimSpace(req.Origin),
		Host:       strings.TrimSpace(req.Host),
		Scopes:     scopes,
		Token:      NewAccessToken(token, now),
		CreatedAt:  now,
		LastSeenAt: now,
	}
	s.clients[client.ID] = client
	return cloneClient(client), nil
}

func (s *Service) requireClientLocked(clientID string, now time.Time) (ClientSession, error) {
	client, ok := s.clients[strings.TrimSpace(clientID)]
	if !ok {
		return ClientSession{}, ErrInvalidClient
	}
	if client.RevokedAt != nil {
		return ClientSession{}, ErrClientRevoked
	}
	client.LastSeenAt = now
	s.clients[client.ID] = client
	return client, nil
}

func (s *Service) authorizeClientLocked(clientID, token, origin, host string, now time.Time) (ClientSession, error) {
	client, ok := s.clients[strings.TrimSpace(clientID)]
	if !ok {
		return ClientSession{}, ErrInvalidClient
	}
	if client.RevokedAt != nil {
		return ClientSession{}, ErrClientRevoked
	}
	if !client.Token.Matches(token) {
		return ClientSession{}, ErrInvalidToken
	}
	if err := s.config.Security.validateOriginHost(origin, host); err != nil {
		return ClientSession{}, err
	}
	if err := validateClientBinding(client, origin, host); err != nil {
		return ClientSession{}, err
	}

	client.LastSeenAt = now
	lastUsedAt := now
	client.Token.LastUsedAt = &lastUsedAt
	s.clients[client.ID] = client
	return client, nil
}

func (s *Service) closeSubscriptionLocked(subscriptionID string, now time.Time) (Subscription, error) {
	rec, ok := s.subscriptions[subscriptionID]
	if !ok {
		return Subscription{}, ErrSubscriptionClosed
	}
	if rec.sub.Active {
		closedAt := now
		rec.sub.ClosedAt = &closedAt
		rec.sub.Active = false
		close(rec.events)
	}
	delete(s.subscriptions, subscriptionID)
	return cloneSubscription(rec.sub), nil
}

func (s *Service) historyLocked(afterSeq uint64, topics []string) []ConnectionEvent {
	history := make([]ConnectionEvent, 0)
	for _, event := range s.events {
		if event.Seq <= afterSeq || !topicMatches(topics, event.Topic) {
			continue
		}
		history = append(history, cloneEvent(event))
	}
	return history
}

func (s *Service) now() time.Time {
	return s.config.Now().UTC()
}

func NewAccessToken(raw string, issuedAt time.Time) AccessToken {
	digest := DigestToken(raw)
	return AccessToken{
		Digest:   digest,
		Preview:  previewDigest(digest),
		IssuedAt: issuedAt.UTC(),
	}
}

func (t AccessToken) Matches(raw string) bool {
	return subtle.ConstantTimeCompare([]byte(t.Digest), []byte(DigestToken(raw))) == 1
}

func DigestToken(raw string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(raw)))
	return tokenDigestAlgorithmTag + hex.EncodeToString(sum[:])
}

func normalizeConfig(config BridgeConfig) BridgeConfig {
	defaults := DefaultSecurityPolicy()
	security := config.Security
	if security.PairingCodeTTL <= 0 {
		security.PairingCodeTTL = defaults.PairingCodeTTL
	}
	if security.AllowedOrigins == nil {
		security.AllowedOrigins = defaults.AllowedOrigins
	}
	if security.AllowedHosts == nil {
		security.AllowedHosts = defaults.AllowedHosts
	}
	if security.RouteAllowlist == nil {
		security.RouteAllowlist = defaults.RouteAllowlist
	}
	if security.WebSocketPaths == nil {
		security.WebSocketPaths = defaults.WebSocketPaths
	}

	config.Security = cloneSecurityPolicy(security)
	if strings.TrimSpace(config.RuntimeBaseURL) == "" {
		config.RuntimeBaseURL = defaultRuntimeBaseURL
	}
	if config.DefaultScopes == nil {
		config.DefaultScopes = []ClientScope{ScopeRuntimeRead, ScopeEventsSubscribe}
	} else {
		config.DefaultScopes = normalizeScopes(config.DefaultScopes, nil)
	}
	if config.HeartbeatInterval <= 0 {
		config.HeartbeatInterval = defaultHeartbeat
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	return config
}

func (p SecurityPolicy) validateOriginHost(origin, host string) error {
	if !p.allowsOrigin(origin) {
		return ErrOriginDenied
	}
	if !p.allowsHost(host) {
		return ErrHostDenied
	}
	return nil
}

func (p SecurityPolicy) allowsOrigin(origin string) bool {
	origin = strings.TrimSpace(origin)
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return false
	}
	normalized := parsed.Scheme + "://" + parsed.Host
	for _, allowed := range p.AllowedOrigins {
		if wildcardMatch(strings.TrimSpace(allowed), normalized) {
			return true
		}
	}
	return false
}

func (p SecurityPolicy) allowsHost(host string) bool {
	host = strings.TrimSpace(host)
	if host == "" {
		return true
	}
	hostOnly := host
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		hostOnly = parsedHost
	}
	hostOnly = strings.Trim(hostOnly, "[]")
	for _, allowed := range p.AllowedHosts {
		allowed = strings.TrimSpace(allowed)
		if wildcardMatch(allowed, host) || wildcardMatch(strings.Trim(allowed, "[]"), hostOnly) {
			return true
		}
	}
	return false
}

func (p SecurityPolicy) matchRoute(method, requestPath string) (RouteRule, bool) {
	for _, rule := range p.RouteAllowlist {
		if strings.ToUpper(strings.TrimSpace(rule.Method)) != method {
			continue
		}
		if rule.Path != "" && requestPath == rule.Path {
			return cloneRouteRule(rule), true
		}
		if rule.PathPattern != "" && matchPathPattern(rule.PathPattern, requestPath) {
			return cloneRouteRule(rule), true
		}
		if rule.PathPrefix != "" && strings.HasPrefix(requestPath, rule.PathPrefix) {
			return cloneRouteRule(rule), true
		}
	}
	return RouteRule{}, false
}

func (p SecurityPolicy) allowsWebSocketPath(requestPath string) bool {
	for _, allowed := range p.WebSocketPaths {
		if requestPath == strings.TrimSpace(allowed) {
			return true
		}
	}
	return false
}

func normalizeRequestPath(rawPath string) (requestPath string, targetPath string, err error) {
	rawPath = strings.TrimSpace(rawPath)
	if rawPath == "" {
		return "", "", ErrRouteDenied
	}
	parsed, err := url.ParseRequestURI(rawPath)
	if err != nil || parsed.IsAbs() || parsed.Host != "" || !strings.HasPrefix(parsed.Path, "/") {
		return "", "", ErrRouteDenied
	}
	decodedPath, err := url.PathUnescape(parsed.EscapedPath())
	if err != nil {
		return "", "", ErrRouteDenied
	}
	for _, segment := range strings.Split(decodedPath, "/") {
		if segment == ".." {
			return "", "", ErrRouteDenied
		}
	}
	cleanPath := path.Clean(decodedPath)
	if cleanPath == "." {
		cleanPath = "/"
	}
	if !strings.HasPrefix(cleanPath, "/") {
		cleanPath = "/" + cleanPath
	}
	targetPath = cleanPath
	if parsed.RawQuery != "" {
		targetPath += "?" + parsed.RawQuery
	}
	return cleanPath, targetPath, nil
}

func matchPathPattern(pattern, requestPath string) bool {
	patternParts := strings.Split(strings.Trim(pattern, "/"), "/")
	pathParts := strings.Split(strings.Trim(requestPath, "/"), "/")
	if len(patternParts) != len(pathParts) {
		return false
	}
	for i, patternPart := range patternParts {
		if strings.HasPrefix(patternPart, "{") && strings.HasSuffix(patternPart, "}") {
			if pathParts[i] == "" {
				return false
			}
			continue
		}
		if patternPart != pathParts[i] {
			return false
		}
	}
	return true
}

func wildcardMatch(pattern, value string) bool {
	if pattern == "" {
		return false
	}
	if pattern == "*" {
		return true
	}
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(value, strings.TrimSuffix(pattern, "*"))
	}
	return pattern == value
}

func validateClientBinding(client ClientSession, origin, host string) error {
	if client.Origin != "" && strings.TrimSpace(origin) != client.Origin {
		return ErrOriginDenied
	}
	if client.Host != "" && strings.TrimSpace(host) != client.Host {
		return ErrHostDenied
	}
	return nil
}

func hasScopes(clientScopes, requiredScopes []ClientScope) bool {
	if len(requiredScopes) == 0 {
		return true
	}
	available := make(map[ClientScope]bool, len(clientScopes))
	for _, scope := range clientScopes {
		available[scope] = true
	}
	for _, scope := range requiredScopes {
		if !available[scope] {
			return false
		}
	}
	return true
}

func normalizeScopes(scopes, fallback []ClientScope) []ClientScope {
	if len(scopes) == 0 {
		return copyScopes(fallback)
	}
	seen := make(map[ClientScope]bool, len(scopes))
	normalized := make([]ClientScope, 0, len(scopes))
	for _, scope := range scopes {
		scope = ClientScope(strings.TrimSpace(string(scope)))
		if scope == "" || seen[scope] {
			continue
		}
		seen[scope] = true
		normalized = append(normalized, scope)
	}
	return normalized
}

func normalizeTopics(topics []string) []string {
	seen := make(map[string]bool, len(topics))
	normalized := make([]string, 0, len(topics))
	for _, topic := range topics {
		topic = strings.TrimSpace(topic)
		if topic == "" || seen[topic] {
			continue
		}
		seen[topic] = true
		normalized = append(normalized, topic)
	}
	return normalized
}

func topicMatches(topics []string, eventTopic string) bool {
	if len(topics) == 0 {
		return true
	}
	for _, topic := range topics {
		if topic == eventTopic {
			return true
		}
	}
	return false
}

func normalizePairingCode(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

func randomID(prefix string) (string, error) {
	value, err := randomHex(generatedIDRandomBytes)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s_%s", prefix, value), nil
}

func randomHex(bytes int) (string, error) {
	buf := make([]byte, bytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func previewDigest(digest string) string {
	digest = strings.TrimSpace(digest)
	if len(digest) <= len(tokenDigestAlgorithmTag)+12 {
		return "redacted"
	}
	return digest[:len(tokenDigestAlgorithmTag)+12] + "..."
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func cloneSecurityPolicy(policy SecurityPolicy) SecurityPolicy {
	return SecurityPolicy{
		PairingCodeTTL: policy.PairingCodeTTL,
		AllowedOrigins: copyStrings(policy.AllowedOrigins),
		AllowedHosts:   copyStrings(policy.AllowedHosts),
		RouteAllowlist: cloneRouteRules(policy.RouteAllowlist),
		WebSocketPaths: copyStrings(policy.WebSocketPaths),
	}
}

func cloneRouteRules(rules []RouteRule) []RouteRule {
	cloned := make([]RouteRule, len(rules))
	for i, rule := range rules {
		cloned[i] = cloneRouteRule(rule)
	}
	return cloned
}

func cloneRouteRule(rule RouteRule) RouteRule {
	rule.RequiredScopes = copyScopes(rule.RequiredScopes)
	return rule
}

func cloneClient(client ClientSession) ClientSession {
	client.Scopes = copyScopes(client.Scopes)
	return client
}

func cloneSubscription(sub Subscription) Subscription {
	sub.Topics = copyStrings(sub.Topics)
	sub.History = cloneEvents(sub.History)
	return sub
}

func cloneEvents(events []ConnectionEvent) []ConnectionEvent {
	cloned := make([]ConnectionEvent, len(events))
	for i, event := range events {
		cloned[i] = cloneEvent(event)
	}
	return cloned
}

func cloneEvent(event ConnectionEvent) ConnectionEvent {
	event.Metadata = copyStringMap(event.Metadata)
	return event
}

func copyScopes(scopes []ClientScope) []ClientScope {
	if len(scopes) == 0 {
		return nil
	}
	cloned := make([]ClientScope, len(scopes))
	copy(cloned, scopes)
	return cloned
}

func copyStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	cloned := make([]string, len(values))
	copy(cloned, values)
	return cloned
}

func copyStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}
