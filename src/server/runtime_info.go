package server

import (
	"net/url"
	"strings"

	"awaken/src/llm"
	"awaken/src/mcp"
	"awaken/src/modeldisplay"
	"awaken/src/skills"
	"awaken/src/slashcommands"
)

const RuntimeVersion = "0.1.0"

// RuntimeInfo exposes non-sensitive runtime metadata.
type RuntimeInfo struct {
	RuntimeVersion             string                      `json:"runtime_version"`
	APIVersion                 string                      `json:"api_version"`
	ConfiguredModel            string                      `json:"configured_model"`
	ConfiguredModelDisplayName string                      `json:"configured_model_display_name,omitempty"`
	Protocol                   string                      `json:"protocol"`
	AuthScheme                 string                      `json:"auth_scheme"`
	HasAPIKey                  bool                        `json:"has_api_key"`
	DefaultWorkspace           string                      `json:"default_workspace"`
	BaseURLHost                string                      `json:"base_url_host"`
	ActiveProfile              string                      `json:"active_profile"`
	Profiles                   []RuntimeProfileInfo        `json:"profiles"`
	Capabilities               CapabilitiesSnapshot        `json:"capabilities"`
	SlashCommands              []slashcommands.CommandInfo `json:"slash_commands"`
}

type RuntimeProfileInfo struct {
	Name             string `json:"name"`
	Model            string `json:"model"`
	ModelDisplayName string `json:"model_display_name,omitempty"`
	Protocol         string `json:"protocol"`
	AuthScheme       string `json:"auth_scheme"`
	HasAPIKey        bool   `json:"has_api_key"`
	BaseURLHost      string `json:"base_url_host"`
	IsActive         bool   `json:"is_active"`
}

type CapabilitiesSnapshot struct {
	MCP    mcp.Snapshot    `json:"mcp"`
	Skills skills.Snapshot `json:"skills"`
}

// RuntimeInfoProvider provides runtime information for /v1/runtime/info.
type RuntimeInfoProvider interface {
	GetRuntimeInfo() RuntimeInfo
}

// StaticRuntimeInfoProvider returns fixed RuntimeInfo for tests.
type StaticRuntimeInfoProvider struct {
	Info RuntimeInfo
}

func (s *StaticRuntimeInfoProvider) GetRuntimeInfo() RuntimeInfo {
	return s.Info
}

type llmRuntimeInfoProvider struct {
	cfg              llm.Config
	defaultWorkspace string
	mcpRegistry      mcp.Registry
	skillsRegistry   skills.Registry
}

func (p *llmRuntimeInfoProvider) GetRuntimeInfo() RuntimeInfo {
	return RuntimeInfo{
		RuntimeVersion:             RuntimeVersion,
		APIVersion:                 "v1",
		ConfiguredModel:            p.cfg.Model,
		ConfiguredModelDisplayName: modeldisplay.RenderModelName(p.cfg.Model),
		Protocol:                   string(llm.ResolveProtocol(p.cfg)),
		AuthScheme:                 p.cfg.AuthScheme,
		HasAPIKey:                  p.cfg.APIKey != "",
		DefaultWorkspace:           p.defaultWorkspace,
		BaseURLHost:                extractHost(p.cfg.BaseURL),
		ActiveProfile:              p.cfg.ActiveProfile,
		Profiles:                   runtimeProfiles(p.cfg),
		Capabilities: CapabilitiesSnapshot{
			MCP:    snapshotMCP(p.mcpRegistry),
			Skills: snapshotSkills(p.skillsRegistry),
		},
		SlashCommands: slashcommands.ListCommands(),
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

// NewLLMRuntimeInfoProvider builds runtime info from LLM config.
// Responses expose only non-sensitive metadata; API keys and full base URLs are omitted.
func NewLLMRuntimeInfoProvider(cfg llm.Config, workspace string, options ...RuntimeInfoOption) RuntimeInfoProvider {
	provider := &llmRuntimeInfoProvider{
		cfg:              cfg,
		defaultWorkspace: workspace,
	}
	for _, option := range options {
		if option != nil {
			option(provider)
		}
	}
	return provider
}

func runtimeProfiles(cfg llm.Config) []RuntimeProfileInfo {
	profiles := make([]RuntimeProfileInfo, 0, len(cfg.Profiles))
	for _, profile := range cfg.Profiles {
		profiles = append(profiles, RuntimeProfileInfo{
			Name:             profile.Name,
			Model:            profile.Model,
			ModelDisplayName: modeldisplay.RenderModelName(profile.Model),
			Protocol:         string(runtimeProfileProtocol(profile)),
			AuthScheme:       profile.AuthScheme,
			HasAPIKey:        profile.HasAPIKey,
			BaseURLHost:      extractHost(profile.BaseURL),
			IsActive:         profile.Name == cfg.ActiveProfile,
		})
	}
	return profiles
}

func runtimeProfileProtocol(profile llm.Profile) llm.Protocol {
	switch profile.Protocol {
	case llm.ProtocolOpenAIChat, llm.ProtocolAnthropicMessages:
		return profile.Protocol
	}

	baseURL := strings.ToLower(profile.BaseURL)
	if strings.Contains(baseURL, "api.anthropic.com") {
		return llm.ProtocolAnthropicMessages
	}
	if strings.HasSuffix(baseURL, "/anthropic") {
		return llm.ProtocolAnthropicMessages
	}
	if strings.Contains(baseURL, "/anthropic/") {
		return llm.ProtocolAnthropicMessages
	}

	return llm.ProtocolOpenAIChat
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
