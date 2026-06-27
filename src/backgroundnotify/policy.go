package backgroundnotify

import (
	"fmt"
	"strings"
)

type Mode string

const (
	ModeAll    Mode = "all"
	ModeResult Mode = "result"
	ModeError  Mode = "error"
	ModeOff    Mode = "off"
)

type EventType string

const (
	EventTypeRunningUpdate EventType = "running_update"
	EventTypeFinalResult   EventType = "final_result"
)

type Source string

const (
	SourceDefault Source = "default"
	SourceConfig  Source = "config"
	SourceEnv     Source = "env"
)

type ProcessResult struct {
	ExitCode int
}

type Event struct {
	Type   EventType
	Result ProcessResult
}

type Resolution struct {
	Mode    Mode
	Source  Source
	Raw     string
	Warning string
	Reason  string
}

type Decision struct {
	Notify    bool
	Mode      Mode
	EventType EventType
	Reason    string
}

func NormalizeMode(raw string) (Mode, bool) {
	switch Mode(strings.ToLower(strings.TrimSpace(raw))) {
	case ModeAll:
		return ModeAll, true
	case ModeResult:
		return ModeResult, true
	case ModeError:
		return ModeError, true
	case ModeOff:
		return ModeOff, true
	default:
		return "", false
	}
}

func Resolve(configValue any, envValue string) Resolution {
	if strings.TrimSpace(envValue) != "" {
		return resolveRaw(SourceEnv, envValue)
	}

	switch value := configValue.(type) {
	case nil:
		return defaultResolution("empty notification mode defaults to all")
	case bool:
		if !value {
			return Resolution{
				Mode:   ModeOff,
				Source: SourceConfig,
				Raw:    "false",
				Reason: "config boolean false disables watcher messages",
			}
		}
		return invalidResolution(SourceConfig, "true")
	case string:
		if strings.TrimSpace(value) == "" {
			return defaultResolution("empty notification mode defaults to all")
		}
		return resolveRaw(SourceConfig, value)
	default:
		raw := strings.TrimSpace(fmt.Sprint(value))
		if raw == "" {
			return defaultResolution("empty notification mode defaults to all")
		}
		return resolveRaw(SourceConfig, raw)
	}
}

func ShouldNotify(mode Mode, event Event) Decision {
	if _, ok := NormalizeMode(string(mode)); !ok {
		mode = ModeAll
	}

	decision := Decision{
		Mode:      mode,
		EventType: event.Type,
	}

	switch event.Type {
	case EventTypeRunningUpdate:
		if mode == ModeAll {
			decision.Notify = true
			decision.Reason = "all mode sends running output updates"
			return decision
		}
		decision.Reason = string(mode) + " mode suppresses running output updates"
		return decision
	case EventTypeFinalResult:
		switch mode {
		case ModeAll:
			decision.Notify = true
			decision.Reason = "all mode sends final results"
		case ModeResult:
			decision.Notify = true
			decision.Reason = "result mode sends final results"
		case ModeError:
			decision.Notify = event.Result.ExitCode != 0
			if decision.Notify {
				decision.Reason = "error mode sends failed final results"
			} else {
				decision.Reason = "error mode suppresses successful final results"
			}
		case ModeOff:
			decision.Reason = "off mode disables watcher messages"
		}
		return decision
	default:
		decision.Reason = "unknown event type"
		return decision
	}
}

func resolveRaw(source Source, raw string) Resolution {
	mode, ok := NormalizeMode(raw)
	if !ok {
		return invalidResolution(source, raw)
	}
	return Resolution{
		Mode:   mode,
		Source: source,
		Raw:    strings.TrimSpace(raw),
		Reason: string(source) + " notification mode resolved",
	}
}

func defaultResolution(reason string) Resolution {
	return Resolution{
		Mode:   ModeAll,
		Source: SourceDefault,
		Reason: reason,
	}
}

func invalidResolution(source Source, raw string) Resolution {
	return Resolution{
		Mode:    ModeAll,
		Source:  source,
		Raw:     strings.TrimSpace(raw),
		Warning: "unknown background notification mode; defaulting to all",
		Reason:  "invalid notification mode defaults to all",
	}
}
