package webuibridge

import "time"

type ClientScope string

const (
	ScopeRuntimeRead     ClientScope = "runtime:read"
	ScopeRuntimeWrite    ClientScope = "runtime:write"
	ScopeEventsSubscribe ClientScope = "events:subscribe"
)

type ConnectionEventType string

const (
	EventPairingCreated      ConnectionEventType = "pairing.created"
	EventPairingExchanged    ConnectionEventType = "pairing.exchanged"
	EventClientRegistered    ConnectionEventType = "client.registered"
	EventClientRevoked       ConnectionEventType = "client.revoked"
	EventProxyPlanBuilt      ConnectionEventType = "proxy.plan_built"
	EventWebSocketPlanBuilt  ConnectionEventType = "websocket.plan_built"
	EventSubscriptionCreated ConnectionEventType = "subscription.created"
	EventSubscriptionClosed  ConnectionEventType = "subscription.closed"
)

type BridgeConfig struct {
	RuntimeBaseURL    string           `json:"runtimeBaseUrl,omitempty"`
	PublicBaseURL     string           `json:"publicBaseUrl,omitempty"`
	DefaultScopes     []ClientScope    `json:"defaultScopes,omitempty"`
	HeartbeatInterval time.Duration    `json:"heartbeatInterval,omitempty"`
	Security          SecurityPolicy   `json:"security"`
	Now               func() time.Time `json:"-"`
}

type SecurityPolicy struct {
	PairingCodeTTL time.Duration `json:"pairingCodeTtl"`
	AllowedOrigins []string      `json:"allowedOrigins,omitempty"`
	AllowedHosts   []string      `json:"allowedHosts,omitempty"`
	RouteAllowlist []RouteRule   `json:"routeAllowlist,omitempty"`
	WebSocketPaths []string      `json:"webSocketPaths,omitempty"`
}

type RouteRule struct {
	Name           string        `json:"name,omitempty"`
	Method         string        `json:"method"`
	Path           string        `json:"path,omitempty"`
	PathPattern    string        `json:"pathPattern,omitempty"`
	PathPrefix     string        `json:"pathPrefix,omitempty"`
	RequiredScopes []ClientScope `json:"requiredScopes,omitempty"`
}

type AccessToken struct {
	Digest     string     `json:"digest"`
	Preview    string     `json:"preview"`
	IssuedAt   time.Time  `json:"issuedAt"`
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"`
}

type PairingCode struct {
	Code       string        `json:"code,omitempty"`
	Digest     string        `json:"digest"`
	ClientName string        `json:"clientName,omitempty"`
	Scopes     []ClientScope `json:"scopes,omitempty"`
	CreatedAt  time.Time     `json:"createdAt"`
	ExpiresAt  time.Time     `json:"expiresAt"`
	UsedAt     *time.Time    `json:"usedAt,omitempty"`
}

type ClientSession struct {
	ID         string        `json:"id"`
	Name       string        `json:"name"`
	UserAgent  string        `json:"userAgent,omitempty"`
	Origin     string        `json:"origin,omitempty"`
	Host       string        `json:"host,omitempty"`
	Scopes     []ClientScope `json:"scopes,omitempty"`
	Token      AccessToken   `json:"token"`
	CreatedAt  time.Time     `json:"createdAt"`
	LastSeenAt time.Time     `json:"lastSeenAt"`
	RevokedAt  *time.Time    `json:"revokedAt,omitempty"`
}

type Subscription struct {
	ID        string                 `json:"id"`
	ClientID  string                 `json:"clientId"`
	Topics    []string               `json:"topics,omitempty"`
	AfterSeq  uint64                 `json:"afterSeq,omitempty"`
	CreatedAt time.Time              `json:"createdAt"`
	ClosedAt  *time.Time             `json:"closedAt,omitempty"`
	Active    bool                   `json:"active"`
	History   []ConnectionEvent      `json:"history,omitempty"`
	Events    <-chan ConnectionEvent `json:"-"`
}

type RouteProxyPlan struct {
	ClientID       string        `json:"clientId"`
	Method         string        `json:"method"`
	Path           string        `json:"path"`
	TargetPath     string        `json:"targetPath"`
	RouteName      string        `json:"routeName,omitempty"`
	RequiredScopes []ClientScope `json:"requiredScopes,omitempty"`
	TokenDigest    string        `json:"tokenDigest"`
	CreatedAt      time.Time     `json:"createdAt"`
}

type WebSocketPlan struct {
	ClientID          string        `json:"clientId"`
	Path              string        `json:"path"`
	Origin            string        `json:"origin,omitempty"`
	Host              string        `json:"host,omitempty"`
	Topics            []string      `json:"topics,omitempty"`
	AfterSeq          uint64        `json:"afterSeq,omitempty"`
	RequiredScopes    []ClientScope `json:"requiredScopes,omitempty"`
	TokenDigest       string        `json:"tokenDigest"`
	HeartbeatInterval time.Duration `json:"heartbeatInterval"`
	CreatedAt         time.Time     `json:"createdAt"`
}

type ConnectionEvent struct {
	Seq            uint64              `json:"seq"`
	Type           ConnectionEventType `json:"type"`
	Topic          string              `json:"topic,omitempty"`
	ClientID       string              `json:"clientId,omitempty"`
	SubscriptionID string              `json:"subscriptionId,omitempty"`
	Path           string              `json:"path,omitempty"`
	Message        string              `json:"message,omitempty"`
	Metadata       map[string]string   `json:"metadata,omitempty"`
	Timestamp      time.Time           `json:"timestamp"`
}

type PairingCodeRequest struct {
	ClientName string        `json:"clientName,omitempty"`
	Scopes     []ClientScope `json:"scopes,omitempty"`
}

type PairingCodeExchange struct {
	Code       string        `json:"code"`
	ClientID   string        `json:"clientId,omitempty"`
	ClientName string        `json:"clientName,omitempty"`
	Token      string        `json:"-"`
	Origin     string        `json:"origin,omitempty"`
	Host       string        `json:"host,omitempty"`
	UserAgent  string        `json:"userAgent,omitempty"`
	Scopes     []ClientScope `json:"scopes,omitempty"`
}

type ClientRegistration struct {
	ClientID   string        `json:"clientId,omitempty"`
	ClientName string        `json:"clientName,omitempty"`
	Token      string        `json:"-"`
	Origin     string        `json:"origin,omitempty"`
	Host       string        `json:"host,omitempty"`
	UserAgent  string        `json:"userAgent,omitempty"`
	Scopes     []ClientScope `json:"scopes,omitempty"`
}

type ProxyPlanRequest struct {
	ClientID string `json:"clientId"`
	Method   string `json:"method"`
	Path     string `json:"path"`
	Origin   string `json:"origin,omitempty"`
	Host     string `json:"host,omitempty"`
}

type WebSocketPlanRequest struct {
	ClientID string   `json:"clientId"`
	Path     string   `json:"path"`
	Origin   string   `json:"origin,omitempty"`
	Host     string   `json:"host,omitempty"`
	Topics   []string `json:"topics,omitempty"`
	AfterSeq uint64   `json:"afterSeq,omitempty"`
}

type SubscriptionRequest struct {
	ClientID string   `json:"clientId"`
	Topics   []string `json:"topics,omitempty"`
	AfterSeq uint64   `json:"afterSeq,omitempty"`
}
