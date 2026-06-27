package server

import (
	"net/url"

	"awaken/src/llm"
)

const RuntimeVersion = "0.1.0"

// RuntimeInfo 暴露运行时的非敏感元数据。
type RuntimeInfo struct {
	RuntimeVersion   string               `json:"runtime_version"`
	APIVersion       string               `json:"api_version"`
	ConfiguredModel  string               `json:"configured_model"`
	Protocol         string               `json:"protocol"`
	AuthScheme       string               `json:"auth_scheme"`
	HasAPIKey        bool                 `json:"has_api_key"`
	DefaultWorkspace string               `json:"default_workspace"`
	BaseURLHost      string               `json:"base_url_host"`
	ActiveProfile    string               `json:"active_profile"`
	Profiles         []RuntimeProfileInfo `json:"profiles"`
}

type RuntimeProfileInfo struct {
	Name        string `json:"name"`
	Model       string `json:"model"`
	Protocol    string `json:"protocol"`
	AuthScheme  string `json:"auth_scheme"`
	HasAPIKey   bool   `json:"has_api_key"`
	BaseURLHost string `json:"base_url_host"`
	IsActive    bool   `json:"is_active"`
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
	cfg              llm.Config
	defaultWorkspace string
}

func (p *llmRuntimeInfoProvider) GetRuntimeInfo() RuntimeInfo {
	return RuntimeInfo{
		RuntimeVersion:   RuntimeVersion,
		APIVersion:       "v1",
		ConfiguredModel:  p.cfg.Model,
		Protocol:         string(llm.ResolveProtocol(p.cfg)),
		AuthScheme:       p.cfg.AuthScheme,
		HasAPIKey:        p.cfg.APIKey != "",
		DefaultWorkspace: p.defaultWorkspace,
		BaseURLHost:      extractHost(p.cfg.BaseURL),
		ActiveProfile:    p.cfg.ActiveProfile,
		Profiles:         runtimeProfiles(p.cfg),
	}
}

// NewLLMRuntimeInfoProvider 根据 LLM 配置构造一个 RuntimeInfoProvider。
// 响应只暴露非敏感字段，避免泄漏 API key 或完整 base URL。
func NewLLMRuntimeInfoProvider(cfg llm.Config, workspace string) RuntimeInfoProvider {
	return &llmRuntimeInfoProvider{
		cfg:              cfg,
		defaultWorkspace: workspace,
	}
}

func runtimeProfiles(cfg llm.Config) []RuntimeProfileInfo {
	profiles := make([]RuntimeProfileInfo, 0, len(cfg.Profiles))
	for _, profile := range cfg.Profiles {
		protocol := profile.Protocol
		if protocol == "" {
			protocol = llm.ResolveProtocol(llm.Config{
				BaseURL:  profile.BaseURL,
				Protocol: profile.Protocol,
			})
		}
		profiles = append(profiles, RuntimeProfileInfo{
			Name:        profile.Name,
			Model:       profile.Model,
			Protocol:    string(protocol),
			AuthScheme:  profile.AuthScheme,
			HasAPIKey:   profile.HasAPIKey,
			BaseURLHost: extractHost(profile.BaseURL),
			IsActive:    profile.Name == cfg.ActiveProfile,
		})
	}
	return profiles
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
