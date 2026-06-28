package mcpmanagement

import "time"

const RedactedValue = "[REDACTED]"

type Transport string

const (
	TransportStdio          Transport = "stdio"
	TransportHTTP           Transport = "http"
	TransportSSE            Transport = "sse"
	TransportStreamableHTTP Transport = "streamable_http"
)

type TransportConfig struct {
	Type    Transport         `json:"type"`
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

type ToolInfo struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema any            `json:"input_schema,omitempty"`
	Meta        map[string]any `json:"_meta,omitempty"`
}

type ConnectionState string

const (
	ConnectionUnknown   ConnectionState = "unknown"
	ConnectionDisabled  ConnectionState = "disabled"
	ConnectionPending   ConnectionState = "pending"
	ConnectionTesting   ConnectionState = "testing"
	ConnectionConnected ConnectionState = "connected"
	ConnectionFailed    ConnectionState = "failed"
	ConnectionNeedsAuth ConnectionState = "needs_auth"
)

type ConnectionStatus struct {
	State         ConnectionState `json:"state"`
	Message       string          `json:"message,omitempty"`
	TestedAt      time.Time       `json:"tested_at,omitempty"`
	LastConnected time.Time       `json:"last_connected,omitempty"`
	Tools         []ToolInfo      `json:"tools,omitempty"`
}

type OAuthStatus string

const (
	OAuthUnknown     OAuthStatus = "unknown"
	OAuthNotRequired OAuthStatus = "not_required"
	OAuthRequired    OAuthStatus = "required"
	OAuthAuthorizing OAuthStatus = "authorizing"
	OAuthAuthorized  OAuthStatus = "authorized"
	OAuthError       OAuthStatus = "error"
)

type OAuthState struct {
	Status    OAuthStatus `json:"status"`
	AuthURL   string      `json:"auth_url,omitempty"`
	Message   string      `json:"message,omitempty"`
	UpdatedAt time.Time   `json:"updated_at,omitempty"`
}

type MCPServer struct {
	ID           string           `json:"id"`
	Name         string           `json:"name"`
	Description  string           `json:"description,omitempty"`
	Enabled      bool             `json:"enabled"`
	Builtin      bool             `json:"builtin,omitempty"`
	Transport    TransportConfig  `json:"transport"`
	Tools        []ToolInfo       `json:"tools,omitempty"`
	Connection   ConnectionStatus `json:"connection,omitempty"`
	OAuth        OAuthState       `json:"oauth,omitempty"`
	CreatedAt    time.Time        `json:"created_at,omitempty"`
	UpdatedAt    time.Time        `json:"updated_at,omitempty"`
	OriginalJSON string           `json:"original_json,omitempty"`
}

type ImportSource string

const (
	ImportSourceExternal   ImportSource = "external"
	ImportSourceClaudeCode ImportSource = "claude_code"
	ImportSourceCodex      ImportSource = "codex"
	ImportSourceAionUI     ImportSource = "aionui"
)

type ImportCandidate struct {
	Source     ImportSource `json:"source"`
	Server     MCPServer    `json:"server"`
	Importable bool         `json:"importable"`
	SkipReason string       `json:"skip_reason,omitempty"`
}

type ImportPlan struct {
	Source          ImportSource      `json:"source"`
	Candidates      []ImportCandidate `json:"candidates"`
	ImportableCount int               `json:"importable_count"`
	SkippedCount    int               `json:"skipped_count"`
}

type ConnectionTestPlan struct {
	ServerID   string            `json:"server_id"`
	ServerName string            `json:"server_name"`
	Transport  Transport         `json:"transport"`
	Command    string            `json:"command,omitempty"`
	Args       []string          `json:"args,omitempty"`
	Env        map[string]string `json:"env,omitempty"`
	URL        string            `json:"url,omitempty"`
	Headers    map[string]string `json:"headers,omitempty"`
	Checks     []string          `json:"checks"`
}

type ServerSnapshot struct {
	GeneratedAt time.Time   `json:"generated_at"`
	Servers     []MCPServer `json:"servers"`
}
