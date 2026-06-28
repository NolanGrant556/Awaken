package mcp

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"awaken/src/tool"
)

type Transport string

const (
	TransportStdio Transport = "stdio"
	TransportHTTP  Transport = "http"
	TransportSSE   Transport = "sse"
)

const RedactedHeaderValue = "[redacted]"

type RuntimeMode string

const (
	RuntimeModeStatic     RuntimeMode = "static"
	RuntimeModeTestDouble RuntimeMode = "test-double"
)

type ServerStatus string

const (
	ServerStatusDisabled ServerStatus = "disabled"
	ServerStatusReady    ServerStatus = "ready"
	ServerStatusError    ServerStatus = "error"
)

type ServerConfig struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Transport Transport         `json:"transport"`
	Enabled   bool              `json:"enabled"`
	Command   string            `json:"command,omitempty"`
	Args      []string          `json:"args,omitempty"`
	URL       string            `json:"url,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
}

type ServerRuntime struct {
	Mode      RuntimeMode  `json:"mode"`
	Status    ServerStatus `json:"status"`
	Message   string       `json:"message,omitempty"`
	LastError string       `json:"last_error,omitempty"`
}

type ToolAnnotations struct {
	ReadOnly    bool `json:"read_only"`
	Destructive bool `json:"destructive"`
	OpenWorld   bool `json:"open_world"`
}

type ToolDescriptor struct {
	Name          string                    `json:"name"`
	Description   string                    `json:"description"`
	InputSchema   tool.ToolParametersSchema `json:"input_schema"`
	Annotations   ToolAnnotations           `json:"annotations"`
	ExposeToModel bool                      `json:"expose_to_model"`
}

type Server struct {
	Config ServerConfig     `json:"config"`
	State  ServerRuntime    `json:"state"`
	Tools  []ToolDescriptor `json:"tools"`
}

type SnapshotSummary struct {
	ServerCount      int `json:"server_count"`
	ReadyServerCount int `json:"ready_server_count"`
	ToolCount        int `json:"tool_count"`
	ExposedToolCount int `json:"exposed_tool_count"`
}

type Snapshot struct {
	Servers []Server        `json:"servers"`
	Summary SnapshotSummary `json:"summary"`
}

type Registry interface {
	Snapshot() Snapshot
	ToolDefinitions() []tool.ToolDefinition
}

type StaticRegistry struct {
	servers []Server
}

func NewStaticRegistry(servers []Server) (*StaticRegistry, error) {
	normalized := make([]Server, 0, len(servers))
	seenServerIDs := make(map[string]struct{}, len(servers))

	for _, server := range servers {
		if err := validateServer(server); err != nil {
			return nil, err
		}

		if _, exists := seenServerIDs[server.Config.ID]; exists {
			return nil, fmt.Errorf("duplicate MCP server id %q", server.Config.ID)
		}
		seenServerIDs[server.Config.ID] = struct{}{}

		server.Tools = sortedTools(server.Tools)
		normalized = append(normalized, server)
	}

	sort.Slice(normalized, func(i, j int) bool {
		return normalized[i].Config.ID < normalized[j].Config.ID
	})
	return &StaticRegistry{servers: normalized}, nil
}

func (r *StaticRegistry) Snapshot() Snapshot {
	if r == nil {
		return Snapshot{Servers: []Server{}, Summary: SnapshotSummary{}}
	}

	servers := make([]Server, 0, len(r.servers))
	summary := SnapshotSummary{}

	for _, server := range r.servers {
		servers = append(servers, cloneServer(server))
		summary.ServerCount++
		if server.Config.Enabled && server.State.Status == ServerStatusReady {
			summary.ReadyServerCount++
		}
		summary.ToolCount += len(server.Tools)
		for _, candidate := range server.Tools {
			if shouldExposeTool(server, candidate) {
				summary.ExposedToolCount++
			}
		}
	}

	return Snapshot{
		Servers: servers,
		Summary: summary,
	}
}

func RedactedSnapshot(snapshot Snapshot) Snapshot {
	servers := make([]Server, 0, len(snapshot.Servers))
	for _, server := range snapshot.Servers {
		servers = append(servers, cloneServer(server))
	}
	return Snapshot{
		Servers: servers,
		Summary: snapshot.Summary,
	}
}

func (r *StaticRegistry) ToolDefinitions() []tool.ToolDefinition {
	if r == nil {
		return nil
	}

	definitions := make([]tool.ToolDefinition, 0)
	for _, server := range r.servers {
		for _, candidate := range server.Tools {
			if !shouldExposeTool(server, candidate) {
				continue
			}
			definitions = append(definitions, adaptToolDefinition(server, candidate))
		}
	}

	sort.Slice(definitions, func(i, j int) bool {
		return definitions[i].Function.Name < definitions[j].Function.Name
	})
	return definitions
}

func adaptToolDefinition(server Server, candidate ToolDescriptor) tool.ToolDefinition {
	description := strings.TrimSpace(candidate.Description)
	if description == "" {
		description = fmt.Sprintf("MCP tool %q from server %q", candidate.Name, server.Config.Name)
	} else {
		description = fmt.Sprintf("%s [MCP server: %s]", description, server.Config.Name)
	}

	return tool.ToolDefinition{
		Type: "function",
		Function: tool.ToolFunctionSchema{
			Name:        BuildToolName(server.Config.ID, candidate.Name),
			Description: description,
			Parameters:  normalizeInputSchema(candidate.InputSchema),
		},
	}
}

func BuildToolName(serverID string, toolName string) string {
	return "mcp__" + normalizeNameComponent(serverID) + "__" + normalizeNameComponent(toolName)
}

func validateServer(server Server) error {
	if strings.TrimSpace(server.Config.ID) == "" {
		return fmt.Errorf("MCP server id is required")
	}
	if strings.TrimSpace(server.Config.Name) == "" {
		return fmt.Errorf("MCP server %q name is required", server.Config.ID)
	}

	switch server.Config.Transport {
	case TransportStdio:
		if strings.TrimSpace(server.Config.Command) == "" {
			return fmt.Errorf("MCP server %q stdio transport requires command", server.Config.ID)
		}
	case TransportHTTP, TransportSSE:
		if strings.TrimSpace(server.Config.URL) == "" {
			return fmt.Errorf("MCP server %q %s transport requires url", server.Config.ID, server.Config.Transport)
		}
	default:
		return fmt.Errorf("MCP server %q has unsupported transport %q", server.Config.ID, server.Config.Transport)
	}

	switch server.State.Mode {
	case RuntimeModeStatic, RuntimeModeTestDouble:
	default:
		return fmt.Errorf("MCP server %q has unsupported runtime mode %q", server.Config.ID, server.State.Mode)
	}

	switch server.State.Status {
	case ServerStatusDisabled, ServerStatusReady, ServerStatusError:
	default:
		return fmt.Errorf("MCP server %q has unsupported status %q", server.Config.ID, server.State.Status)
	}

	seenToolNames := make(map[string]struct{}, len(server.Tools))
	for _, candidate := range server.Tools {
		if strings.TrimSpace(candidate.Name) == "" {
			return fmt.Errorf("MCP server %q has a tool with empty name", server.Config.ID)
		}
		if _, exists := seenToolNames[candidate.Name]; exists {
			return fmt.Errorf("MCP server %q has duplicate tool %q", server.Config.ID, candidate.Name)
		}
		seenToolNames[candidate.Name] = struct{}{}

		if err := validateInputSchema(candidate.InputSchema); err != nil {
			return fmt.Errorf("MCP server %q tool %q: %w", server.Config.ID, candidate.Name, err)
		}
	}

	return nil
}

func validateInputSchema(schema tool.ToolParametersSchema) error {
	if schema.Type == "" {
		schema.Type = "object"
	}
	if schema.Type != "object" {
		return fmt.Errorf("input schema type must be object, got %q", schema.Type)
	}
	for name, param := range schema.Properties {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("input schema has empty property name")
		}
		if !isJSONSchemaType(param.Type) {
			return fmt.Errorf("property %q has unsupported type %q", name, param.Type)
		}
	}
	return nil
}

func normalizeInputSchema(schema tool.ToolParametersSchema) tool.ToolParametersSchema {
	normalized := tool.ToolParametersSchema{
		Type:       schema.Type,
		Properties: make(map[string]tool.ToolParam, len(schema.Properties)),
		Required:   append([]string(nil), schema.Required...),
	}
	if normalized.Type == "" {
		normalized.Type = "object"
	}
	for name, param := range schema.Properties {
		normalized.Properties[name] = param
	}
	sort.Strings(normalized.Required)
	return normalized
}

func shouldExposeTool(server Server, candidate ToolDescriptor) bool {
	return server.Config.Enabled &&
		server.State.Status == ServerStatusReady &&
		candidate.ExposeToModel
}

func cloneServer(server Server) Server {
	cloned := server
	cloned.Config.Args = append([]string(nil), server.Config.Args...)
	if len(server.Config.Headers) > 0 {
		cloned.Config.Headers = make(map[string]string, len(server.Config.Headers))
		for key := range server.Config.Headers {
			cloned.Config.Headers[key] = RedactedHeaderValue
		}
	}
	cloned.Tools = make([]ToolDescriptor, 0, len(server.Tools))
	for _, candidate := range server.Tools {
		copied := candidate
		copied.InputSchema = normalizeInputSchema(candidate.InputSchema)
		cloned.Tools = append(cloned.Tools, copied)
	}
	return cloned
}

func sortedTools(candidates []ToolDescriptor) []ToolDescriptor {
	sorted := append([]ToolDescriptor(nil), candidates...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Name < sorted[j].Name
	})
	return sorted
}

var invalidToolNameChars = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

func normalizeNameComponent(value string) string {
	normalized := invalidToolNameChars.ReplaceAllString(strings.TrimSpace(value), "_")
	normalized = strings.Trim(normalized, "_")
	if normalized == "" {
		return "unnamed"
	}
	return normalized
}

func isJSONSchemaType(value string) bool {
	switch value {
	case "string", "number", "integer", "boolean", "object", "array":
		return true
	default:
		return false
	}
}
