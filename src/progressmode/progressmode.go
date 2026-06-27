package progressmode

import (
	"fmt"
	"os"
	"strings"
)

const (
	EnvToolProgressMode = "HERMES_TOOL_PROGRESS_MODE"
	DefaultMode         = ModeAll
)

type Mode string

const (
	ModeOff     Mode = "off"
	ModeNew     Mode = "new"
	ModeAll     Mode = "all"
	ModeVerbose Mode = "verbose"
)

type Source string

const (
	SourcePlatform Source = "platform"
	SourceGlobal   Source = "global"
	SourceEnv      Source = "env"
	SourceDefault  Source = "default"
	SourceDisabled Source = "disabled"
)

type PlatformConfig struct {
	ToolProgress Mode
}

type Config struct {
	ToolProgress        Mode
	ToolProgressCommand bool
	Platforms           map[string]PlatformConfig
}

type ResolveRequest struct {
	Config   Config
	Platform string

	// Env overrides process environment lookup when non-nil. This keeps tests
	// deterministic while still allowing production callers to use os.Getenv.
	Env map[string]string
}

type Resolution struct {
	EffectiveMode Mode
	Source        Source
	Enabled       bool
	Reason        string
}

type CycleResult struct {
	Resolution Resolution
	Platform   string
	CanCycle   bool
	NextMode   Mode
	Reason     string
}

func Resolve(req ResolveRequest) Resolution {
	platform := normalizePlatform(req.Platform)
	if platformUnsupported(platform) {
		return resolution(
			ModeOff,
			SourceDisabled,
			fmt.Sprintf("platform %q does not support edited progress messages; tool progress disabled", platform),
		)
	}

	var invalid []string

	if platform != "" {
		if platformConfig, ok := req.Config.Platforms[platform]; ok {
			raw := strings.TrimSpace(string(platformConfig.ToolProgress))
			if raw != "" {
				if mode, ok := parseMode(platformConfig.ToolProgress); ok {
					return resolution(
						mode,
						SourcePlatform,
						fmt.Sprintf("resolved from display.platforms.%s.tool_progress", platform),
					)
				}
				invalid = append(invalid, fmt.Sprintf("invalid display.platforms.%s.tool_progress value %q", platform, raw))
			}
		}
	}

	if raw := strings.TrimSpace(string(req.Config.ToolProgress)); raw != "" {
		if mode, ok := parseMode(req.Config.ToolProgress); ok {
			return resolution(mode, SourceGlobal, "resolved from display.tool_progress")
		}
		invalid = append(invalid, fmt.Sprintf("invalid display.tool_progress value %q", raw))
	}

	if raw := strings.TrimSpace(lookupEnv(req, EnvToolProgressMode)); raw != "" {
		if mode, ok := parseMode(Mode(raw)); ok {
			return resolution(mode, SourceEnv, fmt.Sprintf("resolved from %s", EnvToolProgressMode))
		}
		invalid = append(invalid, fmt.Sprintf("invalid %s value %q", EnvToolProgressMode, raw))
	}

	reason := fmt.Sprintf("no tool progress mode configured; using default %s", DefaultMode)
	if len(invalid) > 0 {
		reason = strings.Join(invalid, "; ") + fmt.Sprintf("; using default %s", DefaultMode)
	}
	return resolution(DefaultMode, SourceDefault, reason)
}

func NextMode(current Mode) Mode {
	mode, ok := parseMode(current)
	if !ok {
		mode = DefaultMode
	}

	switch mode {
	case ModeOff:
		return ModeNew
	case ModeNew:
		return ModeAll
	case ModeAll:
		return ModeVerbose
	default:
		return ModeOff
	}
}

func Cycle(req ResolveRequest) CycleResult {
	resolved := Resolve(req)
	result := CycleResult{
		Resolution: resolved,
		Platform:   normalizePlatform(req.Platform),
		NextMode:   resolved.EffectiveMode,
		Reason:     resolved.Reason,
	}

	if !req.Config.ToolProgressCommand {
		result.Reason = "display.tool_progress_command is disabled"
		return result
	}
	if resolved.Source == SourceDisabled {
		return result
	}

	result.CanCycle = true
	result.NextMode = NextMode(resolved.EffectiveMode)
	result.Reason = fmt.Sprintf("cycled tool progress from %s to %s", resolved.EffectiveMode, result.NextMode)
	return result
}

func parseMode(raw Mode) (Mode, bool) {
	mode := Mode(strings.ToLower(strings.TrimSpace(string(raw))))
	switch mode {
	case ModeOff, ModeNew, ModeAll, ModeVerbose:
		return mode, true
	default:
		return "", false
	}
}

func lookupEnv(req ResolveRequest, key string) string {
	if req.Env != nil {
		return req.Env[key]
	}
	return os.Getenv(key)
}

func normalizePlatform(platform string) string {
	return strings.ToLower(strings.TrimSpace(platform))
}

func platformUnsupported(platform string) bool {
	switch platform {
	case "webhook":
		return true
	default:
		return false
	}
}

func resolution(mode Mode, source Source, reason string) Resolution {
	return Resolution{
		EffectiveMode: mode,
		Source:        source,
		Enabled:       mode != ModeOff,
		Reason:        reason,
	}
}
