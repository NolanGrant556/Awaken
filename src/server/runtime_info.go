package server

import (
	"net/url"

	"awaken/src/mcp"
	"awaken/src/skills"
)

const RuntimeVersion = "0.1.0"

// RuntimeInfo 暴露运行时的非敏感元数据。
type RuntimeInfo struct {
	RuntimeVersion   string               `json:"runtime_version"`
	APIVersion       string               `json:"api_version"`
	ConfiguredModel  string               `json:"configured_model"`
	Protocol         string               `json:"protocol"`
	HasAPIKey        bool                 `json:"has_api_key"`
	DefaultWorkspace string               `json:"default_workspace"`
	BaseURLHost      string               `json:"base_url_host"`
	Capabilities     CapabilitiesSnapshot `json:"capabilities"`
}

type CapabilitiesSnapshot struct {
	MCP    mcp.Snapshot    `json:"mcp"`
	Skills skills.Snapshot `json:"skills"`
}

// RuntimeInfoProvider 提供运行时的信息，用于构造 /v1/runtime/info 响应。
type RuntimeInfoProvider interface {
	GetRuntimeInfo() RuntimeInfo
}

// StaticRuntimeInfoProvider 返回固定的 RuntimeInfo，适用于测试。
type StaticRuntimeInfoProvider struct {
	Info RuntimeInfo
}

func (s *StaticRuntimeInfoProvider) GetRuntimeInfo() RuntimeInfo {
	return s.Info
}

type llmRuntimeInfoProvider struct {
	configuredModel  string
	protocol         string
	hasAPIKey        bool
	defaultWorkspace string
	baseURL          string
	mcpRegistry      mcp.Registry
	skillsRegistry   skills.Registry
}

func (p *llmRuntimeInfoProvider) GetRuntimeInfo() RuntimeInfo {
	return RuntimeInfo{
		RuntimeVersion:   RuntimeVersion,
		APIVersion:       "v1",
		ConfiguredModel:  p.configuredModel,
		Protocol:         p.protocol,
		HasAPIKey:        p.hasAPIKey,
		DefaultWorkspace: p.defaultWorkspace,
		BaseURLHost:      extractHost(p.baseURL),
		Capabilities: CapabilitiesSnapshot{
			MCP:    snapshotMCP(p.mcpRegistry),
			Skills: snapshotSkills(p.skillsRegistry),
		},
	}
}

type RuntimeInfoOption func(*llmRuntimeInfoProvider)

func WithMCPRegistry(registry mcp.Registry) RuntimeInfoOption {
	return func(provider *llmRuntimeInfoProvider) {
		provider.mcpRegistry = registry
	}
}

func WithSkillsRegistry(registry skills.Registry) RuntimeInfoOption {
	return func(provider *llmRuntimeInfoProvider) {
		provider.skillsRegistry = registry
	}
}

// NewLLMRuntimeInfoProvider 根据 LLM 配置构造一个 RuntimeInfoProvider。
// 它只接收非敏感字段，避免在响应中泄漏 API key 或完整 base URL。
func NewLLMRuntimeInfoProvider(model, baseURL, apiKey, workspace, protocol string, options ...RuntimeInfoOption) RuntimeInfoProvider {
	provider := &llmRuntimeInfoProvider{
		configuredModel:  model,
		protocol:         protocol,
		hasAPIKey:        apiKey != "",
		defaultWorkspace: workspace,
		baseURL:          baseURL,
	}
	for _, option := range options {
		if option != nil {
			option(provider)
		}
	}
	return provider
}

func extractHost(baseURL string) string {
	if baseURL == "" {
		return ""
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}
	return u.Host
}

func snapshotMCP(registry mcp.Registry) mcp.Snapshot {
	if registry == nil {
		return mcp.Snapshot{Servers: []mcp.Server{}, Summary: mcp.SnapshotSummary{}}
	}
	return mcp.RedactedSnapshot(registry.Snapshot())
}

func snapshotSkills(registry skills.Registry) skills.Snapshot {
	if registry == nil {
		return skills.Snapshot{Skills: []skills.Definition{}, Summary: skills.Summary{}}
	}
	return registry.Snapshot()
}

func sanitizeRuntimeInfo(info RuntimeInfo) RuntimeInfo {
	info.Capabilities = sanitizeCapabilitiesSnapshot(info.Capabilities)
	return info
}

func sanitizeCapabilitiesSnapshot(snapshot CapabilitiesSnapshot) CapabilitiesSnapshot {
	snapshot.MCP = mcp.RedactedSnapshot(snapshot.MCP)
	return snapshot
}
