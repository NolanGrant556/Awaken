package mcpclient

import (
	"errors"
	"time"
)

// ServerState represents the lifecycle state of a single MCP server connection.
type ServerState string

const (
	StatePending   ServerState = "pending"
	StateConnected ServerState = "connected"
	StateFailed    ServerState = "failed"
	StateNeedsAuth ServerState = "needs-auth"
	StateDisabled  ServerState = "disabled"
)

// Scope indicates the configuration source layer that declared a server.
type Scope string

const (
	ScopeUser    Scope = "user"
	ScopeProject Scope = "project"
	ScopeDynamic Scope = "dynamic"
	ScopeBuiltin Scope = "builtin"
)

// TransportKind enumerates the five MCP transport variants Awaken supports.
type TransportKind string

const (
	TransportStdio  TransportKind = "stdio"
	TransportHTTP   TransportKind = "http"
	TransportSSE    TransportKind = "sse"
	TransportWS     TransportKind = "ws"
	TransportInProc TransportKind = "in-process"
)

// ServerSnapshot is the read-only view exposed by Manager.Snapshot() and the
// /v1/mcp/servers endpoint.
type ServerSnapshot struct {
	Name            string            `json:"name"`
	State           ServerState       `json:"state"`
	Transport       TransportKind     `json:"transport"`
	Scope           Scope             `json:"scope"`
	ErrorReason     string            `json:"errorReason,omitempty"`
	LastConnectedAt *time.Time        `json:"lastConnectedAt,omitempty"`
	Capabilities    *ServerCapability `json:"capabilities,omitempty"`
}

// ServerCapability is the trimmed-down result of the MCP initialize handshake.
type ServerCapability struct {
	Name             string `json:"name,omitempty"`
	Version          string `json:"version,omitempty"`
	ProtocolVersion  string `json:"protocolVersion,omitempty"`
	SupportsTools    bool   `json:"supportsTools,omitempty"`
	SupportsPrompts  bool   `json:"supportsPrompts,omitempty"`
	SupportsResources bool  `json:"supportsResources,omitempty"`
}

// NeedsAuthError is returned when a tool call cannot proceed because the
// owning server requires authentication. The agent layer maps this to the
// needs_auth tool-progress stage rather than a generic failure.
type NeedsAuthError struct {
	ServerName string
	Reason     string
}

func (e *NeedsAuthError) Error() string {
	if e.Reason == "" {
		return "mcp server " + e.ServerName + " requires authentication"
	}
	return "mcp server " + e.ServerName + " requires authentication: " + e.Reason
}

// IsNeedsAuth reports whether err is a NeedsAuthError from this package.
func IsNeedsAuth(err error) bool {
	_, ok := err.(*NeedsAuthError)
	return ok
}

// ErrServerNotFound is returned by Manager methods when the named server
// does not exist in the manager's server map.
var ErrServerNotFound = errors.New("mcp server not found")

// ErrIllegalState is returned when an operation is requested on a server
// that is not in a state that allows it (e.g. Enable on a connected server).
var ErrIllegalState = errors.New("mcp server is in an illegal state for this operation")

// ToolNotFoundError is returned when a tool name is not currently registered.
type ToolNotFoundError struct {
	ToolName string
}

func (e *ToolNotFoundError) Error() string {
	return "tool not found: " + e.ToolName
}
