package llm

import (
	"os"
	"strings"
)

const (
	DefaultProfileName = "default"
	defaultAPIKeyEnv   = "LLM_API_KEY"
)

func resolveProfiles(fileCfg fileConfig) []Profile {
	profiles := make([]Profile, 0, len(fileCfg.LLM.Profiles))
	for _, raw := range fileCfg.LLM.Profiles {
		name := firstNonEmpty(raw.Name, raw.ID)
		if name == "" {
			continue
		}
		profile := Profile{
			Name:       name,
			BaseURL:    normalizeBaseURL(firstNonEmpty(raw.BaseURL, fileCfg.LLM.BaseURL)),
			Model:      firstNonEmpty(raw.ModelID, fileCfg.LLM.ModelID),
			Protocol:   Protocol(firstNonEmpty(raw.Protocol, fileCfg.LLM.Protocol)),
			AuthScheme: firstNonEmpty(raw.AuthScheme, fileCfg.LLM.AuthScheme),
			APIKeyEnv:  firstNonEmpty(raw.APIKeyEnv),
		}
		profile.HasAPIKey = profileHasAPIKey(profile)
		profiles = append(profiles, profile)
	}
	return profiles
}

func resolveActiveProfileName(profiles []Profile, requested string) string {
	requested = strings.TrimSpace(requested)
	if requested != "" {
		for _, profile := range profiles {
			if profile.Name == requested {
				return profile.Name
			}
		}
		if len(profiles) == 0 {
			return requested
		}
	}
	if len(profiles) > 0 {
		return profiles[0].Name
	}
	return DefaultProfileName
}

func profileByName(profiles []Profile, name string) Profile {
	for _, profile := range profiles {
		if profile.Name == name {
			return profile
		}
	}
	return Profile{}
}

func materializeProfiles(
	profiles []Profile,
	activeProfile string,
	baseURL string,
	model string,
	protocol Protocol,
	authScheme string,
) []Profile {
	if len(profiles) == 0 {
		profile := Profile{
			Name:       firstNonEmpty(activeProfile, DefaultProfileName),
			BaseURL:    normalizeBaseURL(baseURL),
			Model:      model,
			Protocol:   protocol,
			AuthScheme: authScheme,
			APIKeyEnv:  defaultAPIKeyEnv,
		}
		profile.HasAPIKey = profileHasAPIKey(profile)
		return []Profile{profile}
	}

	resolved := make([]Profile, 0, len(profiles))
	for _, profile := range profiles {
		next := profile
		if next.Name == activeProfile {
			next.BaseURL = normalizeBaseURL(baseURL)
			next.Model = model
			next.Protocol = protocol
			next.AuthScheme = authScheme
			next.HasAPIKey = profileHasAPIKey(next)
		}
		resolved = append(resolved, next)
	}
	return resolved
}

func resolveProfileAPIKey(profile Profile) string {
	envName := strings.TrimSpace(profile.APIKeyEnv)
	if envName == "" {
		return os.Getenv(defaultAPIKeyEnv)
	}
	return os.Getenv(envName)
}

func profileHasAPIKey(profile Profile) bool {
	return resolveProfileAPIKey(profile) != ""
}

func normalizeBaseURL(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	return strings.TrimSuffix(trimmed, "/")
}
