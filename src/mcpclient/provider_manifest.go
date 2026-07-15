package mcpclient

import (
	"encoding/json"
	"fmt"
	"strings"

	"awaken/src/tool"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// AwakenProviderMetaKey is the optional versioned MCP _meta extension used for
// catalog metadata and host-mediated Artifact publication. It is never a
// prerequisite for calling an MCP tool.
const AwakenProviderMetaKey = "io.awaken.tool/provider/v1"

// ServerProviderManifest records stable provider identity supplied by a
// Connector when it creates an MCP server configuration. Capabilities remain
// per-tool and are not inherited from this server-level identity.
type ServerProviderManifest struct {
	ProviderID   tool.ProviderID              `json:"providerId"`
	DisplayName  string                       `json:"displayName,omitempty"`
	Source       tool.ToolSource              `json:"source,omitempty"`
	Dependencies []tool.DependencyRequirement `json:"dependencies,omitempty"`
}

// MCPToolProviderMetadata is the optional Awaken extension carried in
// mcp.Tool._meta. Capabilities and dependencies improve catalog diagnostics;
// ArtifactOutput opts into host publication of returned bytes.
type MCPToolProviderMetadata struct {
	ProviderID     tool.ProviderID              `json:"provider_id,omitempty"`
	Capabilities   []tool.Capability            `json:"capabilities,omitempty"`
	Dependencies   []tool.DependencyRequirement `json:"dependencies,omitempty"`
	ArtifactOutput *MCPArtifactOutputContract   `json:"artifact_output,omitempty"`
}

// MCPArtifactOutputContract declares the only MCP result shape from which the
// Awaken host may publish a local deliverable. The provider returns bounded
// bytes, not a provider-local path: paths are meaningful only after the host
// validates the exact bytes and commits them through its workspace broker.
// Every configured field must also be declared as a string by OutputSchema.
type MCPArtifactOutputContract struct {
	BytesBase64Field  string `json:"bytes_base64_field"`
	SHA256Field       string `json:"sha256_field"`
	RelativePathField string `json:"relative_path_field"`
	ContentTypeField  string `json:"content_type_field"`
	TitleField        string `json:"title_field,omitempty"`
	MaxBytes          int64  `json:"max_bytes,omitempty"`
}

type adapterProviderContext struct {
	serverManifest ServerProviderManifest
	connectorID    string
	state          ServerState
	errorReason    string
}

func (m *Manager) adapterProviderContext(serverName string) adapterProviderContext {
	if m == nil {
		return adapterProviderContext{}
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	server, ok := m.servers[serverName]
	if !ok || server == nil {
		return adapterProviderContext{}
	}
	ctx := adapterProviderContext{
		connectorID: strings.TrimSpace(server.sourceConnectorID),
		state:       server.state,
		errorReason: strings.TrimSpace(server.errorReason),
	}
	if server.config.Provider != nil {
		ctx.serverManifest = cloneServerProviderManifest(*server.config.Provider)
	}
	return ctx
}

func cloneServerProviderManifest(value ServerProviderManifest) ServerProviderManifest {
	value.Dependencies = append([]tool.DependencyRequirement(nil), value.Dependencies...)
	return value
}

func decodeMCPToolProviderMetadata(value *mcp.Tool) (MCPToolProviderMetadata, []string, bool) {
	if value == nil || value.Meta == nil {
		return MCPToolProviderMetadata{}, nil, false
	}
	raw, exists := value.Meta[AwakenProviderMetaKey]
	if !exists {
		return MCPToolProviderMetadata{}, nil, false
	}
	if raw == nil {
		return MCPToolProviderMetadata{}, []string{fmt.Sprintf("invalid %s metadata: value must be a non-null object", AwakenProviderMetaKey)}, false
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return MCPToolProviderMetadata{}, []string{fmt.Sprintf("invalid %s metadata: %v", AwakenProviderMetaKey, err)}, false
	}
	var metadata MCPToolProviderMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return MCPToolProviderMetadata{}, []string{fmt.Sprintf("invalid %s metadata: %v", AwakenProviderMetaKey, err)}, false
	}
	metadata.Capabilities = tool.NormalizeCapabilities(metadata.Capabilities)
	metadata.Dependencies = normalizeDependencies(metadata.Dependencies)
	return metadata, nil, true
}

func normalizeDependencies(values []tool.DependencyRequirement) []tool.DependencyRequirement {
	if len(values) == 0 {
		return nil
	}
	result := make([]tool.DependencyRequirement, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value.Name = strings.TrimSpace(value.Name)
		value.ProbeID = strings.TrimSpace(value.ProbeID)
		if value.Name == "" {
			continue
		}
		key := string(value.Kind) + "\x00" + value.Name + "\x00" + value.VersionConstraint
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	return result
}

func mergeDependencies(groups ...[]tool.DependencyRequirement) []tool.DependencyRequirement {
	var values []tool.DependencyRequirement
	for _, group := range groups {
		values = append(values, group...)
	}
	return normalizeDependencies(values)
}

func mergeCapabilities(groups ...[]tool.Capability) []tool.Capability {
	var values []tool.Capability
	for _, group := range groups {
		values = append(values, group...)
	}
	return tool.NormalizeCapabilities(values)
}

func inferMCPProviderCapabilities(toolName string, capabilityClass tool.CapabilityClass) []tool.Capability {
	name := strings.ToLower(strings.TrimSpace(toolName))
	var capabilities []tool.Capability

	if isMessageToolName(name) {
		capabilities = append(capabilities, tool.CapabilityMessageSend)
	}
	if isDeleteToolName(name) {
		capabilities = append(capabilities, tool.CapabilityFileDelete)
	}
	switch capabilityClass {
	case tool.CapabilityBrowserRead, tool.CapabilityBrowserInteract:
		capabilities = append(capabilities, tool.CapabilityBrowserInteractV2)
	}
	return tool.NormalizeCapabilities(capabilities)
}

func isMessageToolName(name string) bool {
	return containsAny(name,
		"send_message", "send-message", "post_message", "post-message",
		"send_email", "send-email", "send_mail", "send-mail", "send_dm", "send-dm",
		"broadcast_message", "broadcast-message", "message_send", "message-send",
	)
}

func isDeleteToolName(name string) bool {
	return strings.HasPrefix(name, "delete_") || strings.HasPrefix(name, "delete-") ||
		strings.HasPrefix(name, "remove_") || strings.HasPrefix(name, "remove-") ||
		strings.HasSuffix(name, "_delete") || strings.HasSuffix(name, "-delete")
}
