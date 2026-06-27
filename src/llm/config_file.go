package llm

import (
	"os"
	"strconv"
	"strings"
)

type fileConfig struct {
	LLM   llmFileConfig
	Model modelFileConfig
}

type llmFileConfig struct {
	ActiveProfile string
	BaseURL       string
	ModelID       string
	Protocol      string
	AuthScheme    string
	Profiles      []profileFileConfig
}

type profileFileConfig struct {
	ID         string
	Name       string
	BaseURL    string
	ModelID    string
	Protocol   string
	AuthScheme string
	APIKeyEnv  string
}

type modelFileConfig struct {
	Temperature           float64
	MaxOutputTokens       int
	ContextWindow         int
	AutoCompactTokenLimit int
}

func loadConfigFile(path string) fileConfig {
	data, err := os.ReadFile(path)
	if err != nil {
		return fileConfig{}
	}
	return parseConfigFile(string(data))
}

func parseConfigFile(content string) fileConfig {
	var cfg fileConfig
	section := ""
	for _, rawLine := range strings.Split(content, "\n") {
		line := strings.TrimSpace(stripInlineComment(rawLine))
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[[") && strings.HasSuffix(line, "]]") {
			section = strings.TrimSpace(strings.Trim(line, "[]"))
			switch section {
			case "llm.profiles":
				cfg.LLM.Profiles = append(cfg.LLM.Profiles, profileFileConfig{})
			}
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(strings.Trim(line, "[]"))
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = trimConfigValue(value)

		switch section {
		case "llm":
			assignLLMConfigValue(&cfg.LLM, key, value)
		case "llm.profiles":
			assignProfileConfigValue(&cfg.LLM, key, value)
		case "model":
			assignModelConfigValue(&cfg.Model, key, value)
		}
	}
	return cfg
}

func assignLLMConfigValue(cfg *llmFileConfig, key, value string) {
	switch key {
	case "active_profile":
		cfg.ActiveProfile = value
	case "base_url":
		cfg.BaseURL = value
	case "model_id":
		cfg.ModelID = value
	case "protocol":
		cfg.Protocol = value
	case "auth_scheme":
		cfg.AuthScheme = value
	}
}

func assignProfileConfigValue(cfg *llmFileConfig, key, value string) {
	if len(cfg.Profiles) == 0 {
		cfg.Profiles = append(cfg.Profiles, profileFileConfig{})
	}
	profile := &cfg.Profiles[len(cfg.Profiles)-1]
	switch key {
	case "id":
		profile.ID = value
	case "name":
		profile.Name = value
	case "base_url":
		profile.BaseURL = value
	case "model_id":
		profile.ModelID = value
	case "protocol":
		profile.Protocol = value
	case "auth_scheme":
		profile.AuthScheme = value
	case "api_key_env":
		profile.APIKeyEnv = value
	}
}

func assignModelConfigValue(cfg *modelFileConfig, key, value string) {
	switch key {
	case "temperature":
		if parsed, err := strconv.ParseFloat(value, 64); err == nil {
			cfg.Temperature = parsed
		}
	case "max_output_tokens":
		cfg.MaxOutputTokens = parsePositiveInt(value)
	case "context_window":
		cfg.ContextWindow = parsePositiveInt(value)
	case "auto_compact_token_limit":
		cfg.AutoCompactTokenLimit = parsePositiveInt(value)
	}
}

func parsePositiveInt(value string) int {
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 0
	}
	return parsed
}

func trimConfigValue(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, `"`)
	value = strings.TrimSuffix(value, `"`)
	value = strings.TrimPrefix(value, `'`)
	value = strings.TrimSuffix(value, `'`)
	return value
}

func stripInlineComment(line string) string {
	inSingleQuote := false
	inDoubleQuote := false
	for i, r := range line {
		switch r {
		case '\'':
			if !inDoubleQuote {
				inSingleQuote = !inSingleQuote
			}
		case '"':
			if !inSingleQuote {
				inDoubleQuote = !inDoubleQuote
			}
		case '#':
			if !inSingleQuote && !inDoubleQuote {
				return line[:i]
			}
		}
	}
	return line
}
