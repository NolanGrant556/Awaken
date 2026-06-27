package mcpclient

const (
	MaxDescriptionLength = 2048
	MaxStatusMessageLen  = 512
	MaxProviderNameLen   = 64
)

type TransportType string

const (
	TransportStdio TransportType = "stdio"
	TransportHTTP  TransportType = "http"
	TransportSSE   TransportType = "sse"
	TransportWS    TransportType = "ws"
)

type ConfigScope string

const (
	ScopeLocal      ConfigScope = "local"
	ScopeUser       ConfigScope = "user"
	ScopeProject    ConfigScope = "project"
	ScopeDynamic    ConfigScope = "dynamic"
	ScopeEnterprise ConfigScope = "enterprise"
	ScopeManaged    ConfigScope = "managed"
)

type OAuthConfig struct {
	ClientID              string `json:"client_id,omitempty"`
	ClientSecret          string `json:"client_secret,omitempty"`
	AuthServerMetadataURL string `json:"auth_server_metadata_url,omitempty"`
	CallbackPort          int    `json:"callback_port,omitempty"`
}

type StdioConfig struct {
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

type RemoteConfig struct {
	URL           string            `json:"url"`
	Headers       map[string]string `json:"headers,omitempty"`
	HeadersHelper string            `json:"headers_helper,omitempty"`
	OAuth         *OAuthConfig      `json:"oauth,omitempty"`
}

type ServerConfig struct {
	Name      string            `json:"name"`
	Transport TransportType     `json:"transport"`
	Scope     ConfigScope       `json:"scope,omitempty"`
	Disabled  bool              `json:"disabled,omitempty"`
	Stdio     *StdioConfig      `json:"stdio,omitempty"`
	HTTP      *RemoteConfig     `json:"http,omitempty"`
	SSE       *RemoteConfig     `json:"sse,omitempty"`
	WS        *RemoteConfig     `json:"ws,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

type ServerCapabilities struct {
	Tools             bool `json:"tools,omitempty"`
	Resources         bool `json:"resources,omitempty"`
	Prompts           bool `json:"prompts,omitempty"`
	ResourceSubscribe bool `json:"resource_subscribe,omitempty"`
}

type ConnectionStatus string

const (
	StatusConnected ConnectionStatus = "connected"
	StatusFailed    ConnectionStatus = "failed"
	StatusNeedsAuth ConnectionStatus = "needs_auth"
	StatusPending   ConnectionStatus = "pending"
	StatusDisabled  ConnectionStatus = "disabled"
)

type ErrorClass string

const (
	ErrorAuth           ErrorClass = "auth"
	ErrorSessionExpired ErrorClass = "session_expired"
	ErrorTransient      ErrorClass = "transient"
	ErrorConfig         ErrorClass = "config"
)

type ServerState struct {
	Name                 string             `json:"name"`
	Status               ConnectionStatus   `json:"status"`
	Transport            TransportType      `json:"transport,omitempty"`
	Capabilities         ServerCapabilities `json:"capabilities,omitempty"`
	ErrorClass           ErrorClass         `json:"error_class,omitempty"`
	Message              string             `json:"message,omitempty"`
	ReconnectAttempt     int                `json:"reconnect_attempt,omitempty"`
	MaxReconnectAttempts int                `json:"max_reconnect_attempts,omitempty"`
}

type ToolAnnotations struct {
	ReadOnlyHint    bool `json:"read_only_hint,omitempty"`
	DestructiveHint bool `json:"destructive_hint,omitempty"`
	OpenWorldHint   bool `json:"open_world_hint,omitempty"`
}

type RemoteTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema map[string]any  `json:"input_schema,omitempty"`
	Annotations ToolAnnotations `json:"annotations,omitempty"`
	Meta        map[string]any  `json:"meta,omitempty"`
}

type ServerResource struct {
	Server      string `json:"server,omitempty"`
	URI         string `json:"uri"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	MIMEType    string `json:"mime_type,omitempty"`
}

type ServerDescriptor struct {
	Config       ServerConfig       `json:"config"`
	State        ServerState        `json:"state,omitempty"`
	Capabilities ServerCapabilities `json:"capabilities,omitempty"`
	Instructions string             `json:"instructions,omitempty"`
	Tools        []RemoteTool       `json:"tools,omitempty"`
	Resources    []ServerResource   `json:"resources,omitempty"`
}

type DiscoveredTool struct {
	ServerName       string            `json:"server_name"`
	OriginalName     string            `json:"original_name"`
	SafeName         string            `json:"safe_name"`
	Description      string            `json:"description,omitempty"`
	ParameterNameMap map[string]string `json:"parameter_name_map,omitempty"`
	Remote           RemoteTool        `json:"remote"`
}

type DiscoveryResult struct {
	ServerName   string           `json:"server_name"`
	Tools        []DiscoveredTool `json:"tools,omitempty"`
	Resources    []ServerResource `json:"resources,omitempty"`
	Instructions string           `json:"instructions,omitempty"`
}

type StateEvent string

const (
	EventConnectStarted   StateEvent = "connect_started"
	EventConnectSucceeded StateEvent = "connect_succeeded"
	EventConnectFailed    StateEvent = "connect_failed"
	EventAuthRequired     StateEvent = "auth_required"
	EventSessionExpired   StateEvent = "session_expired"
	EventDisabled         StateEvent = "disabled"
	EventEnabled          StateEvent = "enabled"
)

func NextStatus(current ConnectionStatus, event StateEvent) ConnectionStatus {
	switch event {
	case EventDisabled:
		return StatusDisabled
	case EventEnabled:
		return StatusPending
	}
	if current == StatusDisabled {
		return StatusDisabled
	}
	switch event {
	case EventConnectStarted:
		return StatusPending
	case EventConnectSucceeded:
		return StatusConnected
	case EventConnectFailed:
		return StatusFailed
	case EventAuthRequired:
		return StatusNeedsAuth
	case EventSessionExpired:
		return StatusPending
	default:
		return current
	}
}

func InitialState(config ServerConfig) ServerState {
	status := StatusPending
	if config.Disabled {
		status = StatusDisabled
	}
	return ServerState{
		Name:      config.Name,
		Status:    status,
		Transport: config.Transport,
	}
}
