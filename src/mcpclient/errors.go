package mcpclient

import (
	"errors"
	"strings"
)

var (
	ErrAuthenticationRequired = errors.New("mcp authentication required")
	ErrSessionExpired         = errors.New("mcp session expired")
	ErrTransient              = errors.New("mcp transient error")
	ErrInvalidConfig          = errors.New("mcp invalid config")
)

type ErrorInfo struct {
	Class   ErrorClass       `json:"class"`
	Status  ConnectionStatus `json:"status"`
	Message string           `json:"message"`
}

func ClassifyError(err error) ErrorClass {
	if err == nil {
		return ""
	}
	message := strings.ToLower(err.Error())
	switch {
	case errors.Is(err, ErrAuthenticationRequired),
		strings.Contains(message, "401"),
		strings.Contains(message, "unauthorized"),
		strings.Contains(message, "unauthenticated"),
		strings.Contains(message, "requires re-authorization"),
		strings.Contains(message, "token expired"),
		strings.Contains(message, "oauth"):
		return ErrorAuth
	case errors.Is(err, ErrSessionExpired),
		strings.Contains(message, `"code":-32001`),
		strings.Contains(message, `"code": -32001`),
		strings.Contains(message, "session not found"),
		strings.Contains(message, "session expired"):
		return ErrorSessionExpired
	case errors.Is(err, ErrInvalidConfig),
		strings.Contains(message, "invalid config"),
		strings.Contains(message, "unsupported transport"),
		strings.Contains(message, "missing command"),
		strings.Contains(message, "command cannot be empty"),
		strings.Contains(message, "invalid url"),
		strings.Contains(message, "not configured"):
		return ErrorConfig
	case errors.Is(err, ErrTransient),
		strings.Contains(message, "timeout"),
		strings.Contains(message, "temporary"),
		strings.Contains(message, "connection refused"),
		strings.Contains(message, "connection reset"),
		strings.Contains(message, "connection closed"),
		strings.Contains(message, "eof"),
		strings.Contains(message, "502"),
		strings.Contains(message, "503"),
		strings.Contains(message, "504"),
		strings.Contains(message, "network"),
		strings.Contains(message, "unavailable"):
		return ErrorTransient
	default:
		return ErrorTransient
	}
}

func StatusForErrorClass(class ErrorClass) ConnectionStatus {
	switch class {
	case ErrorAuth:
		return StatusNeedsAuth
	case ErrorSessionExpired:
		return StatusPending
	case ErrorConfig:
		return StatusFailed
	case ErrorTransient:
		return StatusFailed
	default:
		return StatusFailed
	}
}

func ErrorInfoFor(err error, config ServerConfig) ErrorInfo {
	if err == nil {
		return ErrorInfo{}
	}
	class := ClassifyError(err)
	return ErrorInfo{
		Class:   class,
		Status:  StatusForErrorClass(class),
		Message: capStatusMessage(RedactConfigSecrets(err.Error(), config)),
	}
}

func StateFromError(name string, config ServerConfig, err error) ServerState {
	info := ErrorInfoFor(err, config)
	return ServerState{
		Name:       name,
		Status:     info.Status,
		Transport:  config.Transport,
		ErrorClass: info.Class,
		Message:    info.Message,
	}
}
