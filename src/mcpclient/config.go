package mcpclient

import (
	"fmt"
	"net/url"
	"strings"
)

func ValidateServerConfig(config ServerConfig) error {
	if strings.TrimSpace(config.Name) == "" {
		return fmt.Errorf("%w: missing server name", ErrInvalidConfig)
	}
	switch config.Transport {
	case TransportStdio:
		if config.Stdio == nil {
			return fmt.Errorf("%w: stdio config not configured", ErrInvalidConfig)
		}
		if strings.TrimSpace(config.Stdio.Command) == "" {
			return fmt.Errorf("%w: missing command", ErrInvalidConfig)
		}
	case TransportHTTP:
		return validateRemoteConfig(config.HTTP, "http", "http", "https")
	case TransportSSE:
		return validateRemoteConfig(config.SSE, "sse", "http", "https")
	case TransportWS:
		return validateRemoteConfig(config.WS, "ws", "ws", "wss")
	default:
		return fmt.Errorf("%w: unsupported transport %q", ErrInvalidConfig, config.Transport)
	}
	return nil
}

func remoteConfigForTransport(config ServerConfig) *RemoteConfig {
	switch config.Transport {
	case TransportHTTP:
		return config.HTTP
	case TransportSSE:
		return config.SSE
	case TransportWS:
		return config.WS
	default:
		return nil
	}
}

func validateRemoteConfig(remote *RemoteConfig, label string, allowedSchemes ...string) error {
	if remote == nil {
		return fmt.Errorf("%w: %s config not configured", ErrInvalidConfig, label)
	}
	if strings.TrimSpace(remote.URL) == "" {
		return fmt.Errorf("%w: %s url not configured", ErrInvalidConfig, label)
	}
	parsed, err := url.Parse(remote.URL)
	if err != nil {
		return fmt.Errorf("%w: invalid url %s", ErrInvalidConfig, SafeEndpoint(remote.URL))
	}
	for _, scheme := range allowedSchemes {
		if parsed.Scheme == scheme {
			return nil
		}
	}
	return fmt.Errorf("%w: invalid url scheme for %s endpoint %s", ErrInvalidConfig, label, SafeEndpoint(remote.URL))
}
