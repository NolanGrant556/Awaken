package mcpmanagement

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

var (
	ErrInvalidServer    = errors.New("invalid mcp server")
	ErrInvalidTransport = errors.New("invalid mcp transport")
)

func ValidateServer(server MCPServer) error {
	_, err := NormalizeServer(server)
	return err
}

func NormalizeServer(server MCPServer) (MCPServer, error) {
	server.ID = strings.TrimSpace(server.ID)
	server.Name = strings.TrimSpace(server.Name)
	server.Description = strings.TrimSpace(server.Description)
	server.OriginalJSON = strings.TrimSpace(server.OriginalJSON)

	if server.ID == "" {
		return MCPServer{}, fmt.Errorf("%w: id is required", ErrInvalidServer)
	}
	if server.Name == "" {
		return MCPServer{}, fmt.Errorf("%w: name is required", ErrInvalidServer)
	}

	transport, err := normalizeTransport(server.Transport)
	if err != nil {
		return MCPServer{}, err
	}
	server.Transport = transport
	server.Tools = dedupeTools(server.Tools)
	server.Connection.Tools = dedupeTools(server.Connection.Tools)

	if server.Connection.State == "" {
		server.Connection.State = ConnectionUnknown
	}
	if server.OAuth.Status == "" {
		server.OAuth.Status = OAuthUnknown
	}

	return server, nil
}

func normalizeTransport(transport TransportConfig) (TransportConfig, error) {
	transport.Type = normalizeTransportType(transport.Type)
	switch transport.Type {
	case "":
		return TransportConfig{}, fmt.Errorf("%w: type is required", ErrInvalidTransport)
	case TransportStdio:
		return normalizeStdioTransport(transport)
	case TransportHTTP, TransportSSE, TransportStreamableHTTP:
		return normalizeURLTransport(transport)
	default:
		return TransportConfig{}, fmt.Errorf("%w: unsupported type %q", ErrInvalidTransport, transport.Type)
	}
}

func normalizeTransportType(transport Transport) Transport {
	value := strings.ToLower(strings.TrimSpace(string(transport)))
	value = strings.ReplaceAll(value, "-", "_")
	return Transport(value)
}

func normalizeStdioTransport(transport TransportConfig) (TransportConfig, error) {
	command := strings.TrimSpace(transport.Command)
	if command == "" {
		return TransportConfig{}, fmt.Errorf("%w: stdio command is required", ErrInvalidTransport)
	}
	if strings.TrimSpace(transport.URL) != "" || len(transport.Headers) > 0 {
		return TransportConfig{}, fmt.Errorf("%w: stdio transport cannot define url or headers", ErrInvalidTransport)
	}

	args := append([]string(nil), transport.Args...)
	env, err := normalizeStringMap(transport.Env)
	if err != nil {
		return TransportConfig{}, fmt.Errorf("%w: %v", ErrInvalidTransport, err)
	}

	return TransportConfig{
		Type:    TransportStdio,
		Command: command,
		Args:    args,
		Env:     env,
	}, nil
}

func normalizeURLTransport(transport TransportConfig) (TransportConfig, error) {
	rawURL := strings.TrimSpace(transport.URL)
	if rawURL == "" {
		return TransportConfig{}, fmt.Errorf("%w: %s url is required", ErrInvalidTransport, transport.Type)
	}
	if strings.TrimSpace(transport.Command) != "" || len(transport.Args) > 0 || len(transport.Env) > 0 {
		return TransportConfig{}, fmt.Errorf("%w: %s transport cannot define command, args, or env", ErrInvalidTransport, transport.Type)
	}

	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return TransportConfig{}, fmt.Errorf("%w: %s url is invalid", ErrInvalidTransport, transport.Type)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return TransportConfig{}, fmt.Errorf("%w: %s url must use http or https", ErrInvalidTransport, transport.Type)
	}

	headers, err := normalizeStringMap(transport.Headers)
	if err != nil {
		return TransportConfig{}, fmt.Errorf("%w: %v", ErrInvalidTransport, err)
	}

	return TransportConfig{
		Type:    transport.Type,
		URL:     rawURL,
		Headers: headers,
	}, nil
}

func normalizeStringMap(values map[string]string) (map[string]string, error) {
	if len(values) == 0 {
		return nil, nil
	}

	normalized := make(map[string]string, len(values))
	for key, value := range values {
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, errors.New("map key is required")
		}
		normalized[key] = value
	}
	return normalized, nil
}

func dedupeTools(tools []ToolInfo) []ToolInfo {
	if len(tools) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(tools))
	deduped := make([]ToolInfo, 0, len(tools))
	for _, tool := range tools {
		tool.Name = strings.TrimSpace(tool.Name)
		if tool.Name == "" {
			continue
		}
		tool.Description = strings.TrimSpace(tool.Description)
		key := strings.ToLower(tool.Name)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		deduped = append(deduped, cloneToolInfo(tool))
	}
	return deduped
}

func redactStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}

	redacted := make(map[string]string, len(values))
	for key, value := range values {
		if isSensitiveKey(key) && value != "" {
			value = RedactedValue
		}
		redacted[key] = value
	}
	return redacted
}

func sanitizeServer(server MCPServer) MCPServer {
	server = cloneServer(server)
	server.Transport.Env = redactStringMap(server.Transport.Env)
	server.Transport.Headers = redactStringMap(server.Transport.Headers)
	if server.OriginalJSON != "" {
		server.OriginalJSON = RedactedValue
	}
	return server
}

func isSensitiveKey(key string) bool {
	key = strings.ToLower(key)
	sensitiveParts := []string{
		"authorization",
		"auth",
		"api-key",
		"apikey",
		"access-token",
		"token",
		"secret",
		"password",
		"credential",
		"cookie",
	}
	for _, part := range sensitiveParts {
		if strings.Contains(key, part) {
			return true
		}
	}
	return false
}
