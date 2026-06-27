package runtimefooter

import (
	"fmt"
	"math"
	"path/filepath"
	"strings"
)

const (
	FieldModel      = "model"
	FieldContextPct = "context_pct"
	FieldCWD        = "cwd"
)

var defaultFields = []string{FieldModel, FieldContextPct, FieldCWD}

// Config models display.runtime_footer plus per-platform runtime_footer overrides.
type Config struct {
	Enabled   *bool
	Fields    []string
	Platforms map[string]PlatformConfig
}

// PlatformConfig models display.platforms.<platform>.runtime_footer.
type PlatformConfig struct {
	Enabled *bool
	Fields  []string
}

// RuntimeState is the caller-provided runtime data available to the footer.
type RuntimeState struct {
	Model         string
	ContextTokens int
	ContextLength int
	CWD           string
	Home          string
}

// ResolvedConfig is the effective footer config after default/global/platform merge.
type ResolvedConfig struct {
	Enabled bool
	Fields  []string
}

// ResolveConfig applies default, global, and platform-specific footer config.
func ResolveConfig(config Config, platform string) ResolvedConfig {
	resolved := ResolvedConfig{
		Enabled: false,
		Fields:  append([]string(nil), defaultFields...),
	}

	applyOverride(&resolved, config.Enabled, config.Fields)
	if platform != "" && config.Platforms != nil {
		if platformConfig, ok := config.Platforms[platform]; ok {
			applyOverride(&resolved, platformConfig.Enabled, platformConfig.Fields)
		}
	}

	return resolved
}

// Format renders the requested fields, skipping unknown fields and missing data.
func Format(state RuntimeState, fields []string) string {
	var parts []string
	for _, field := range fields {
		switch field {
		case FieldModel:
			if model := shortModel(state.Model); model != "" {
				parts = append(parts, model)
			}
		case FieldContextPct:
			if pct, ok := contextPercent(state.ContextTokens, state.ContextLength); ok {
				parts = append(parts, fmt.Sprintf("%d%%", pct))
			}
		case FieldCWD:
			if cwd := homeRelativeCWD(state.CWD, state.Home); cwd != "" {
				parts = append(parts, cwd)
			}
		}
	}

	return strings.Join(parts, " · ")
}

// Build resolves config and formats a footer, returning an empty string when disabled.
func Build(config Config, platform string, state RuntimeState) string {
	resolved := ResolveConfig(config, platform)
	if !resolved.Enabled {
		return ""
	}
	return Format(state, resolved.Fields)
}

func applyOverride(resolved *ResolvedConfig, enabled *bool, fields []string) {
	if enabled != nil {
		resolved.Enabled = *enabled
	}
	if len(fields) > 0 {
		resolved.Fields = append([]string(nil), fields...)
	}
}

func shortModel(model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return ""
	}
	if slash := strings.LastIndex(model, "/"); slash >= 0 {
		return model[slash+1:]
	}
	return model
}

func contextPercent(tokens, length int) (int, bool) {
	if length <= 0 || tokens < 0 {
		return 0, false
	}
	pct := int(math.Round(float64(tokens) / float64(length) * 100))
	if pct < 0 {
		return 0, true
	}
	if pct > 100 {
		return 100, true
	}
	return pct, true
}

func homeRelativeCWD(cwd, home string) string {
	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		return ""
	}

	cleanCWD := filepath.Clean(cwd)
	cleanHome := filepath.Clean(strings.TrimSpace(home))
	if cleanHome == "." || cleanHome == "" {
		return cleanCWD
	}

	separator := string(filepath.Separator)
	if cleanCWD == cleanHome {
		return "~"
	}
	if strings.HasPrefix(cleanCWD, cleanHome+separator) {
		return "~" + cleanCWD[len(cleanHome):]
	}
	return cleanCWD
}
