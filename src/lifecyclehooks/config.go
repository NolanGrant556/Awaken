package lifecyclehooks

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Event string

const (
	EventBeforeRun  Event = "before_run"
	EventAfterRun   Event = "after_run"
	EventBeforeTool Event = "before_tool"
	EventAfterTool  Event = "after_tool"
)

type Config struct {
	Hooks map[Event][]CommandConfig `json:"hooks,omitempty"`
}

type CommandConfig struct {
	Command   string   `json:"command"`
	Args      []string `json:"args,omitempty"`
	TimeoutMS int      `json:"timeout_ms,omitempty"`
	Enabled   *bool    `json:"enabled,omitempty"`
}

func LoadConfig() Config {
	configPath := "config.toml"
	if configDir := strings.TrimSpace(os.Getenv("AWAKEN_CONFIG_DIR")); configDir != "" {
		configPath = filepath.Join(configDir, "config.toml")
	}
	return LoadConfigFile(configPath)
}

func LoadConfigFile(path string) Config {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}
	}
	return ParseConfigFile(string(data))
}

func ParseConfigFile(content string) Config {
	var cfg Config
	var (
		inHookSection bool
		currentEvent  Event
		current       CommandConfig
	)

	flush := func() {
		if !inHookSection || strings.TrimSpace(current.Command) == "" {
			return
		}
		if cfg.Hooks == nil {
			cfg.Hooks = make(map[Event][]CommandConfig)
		}
		current.Command = strings.TrimSpace(current.Command)
		cfg.Hooks[currentEvent] = append(cfg.Hooks[currentEvent], current)
	}

	for _, rawLine := range strings.Split(content, "\n") {
		line := strings.TrimSpace(stripInlineComment(rawLine))
		if line == "" {
			continue
		}
		if section, ok := parseSection(line); ok {
			flush()
			event, isHookSection := parseHookSection(section)
			inHookSection = isHookSection
			currentEvent = event
			current = CommandConfig{}
			continue
		}
		if !inHookSection {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		assignHookValue(&current, strings.TrimSpace(key), strings.TrimSpace(value))
	}
	flush()

	return cfg
}

func (c Config) CommandsFor(event Event) []CommandConfig {
	if len(c.Hooks) == 0 {
		return nil
	}
	return append([]CommandConfig(nil), c.Hooks[event]...)
}

func (c Config) HasHooks() bool {
	for event := range c.Hooks {
		for _, hook := range c.Hooks[event] {
			if hook.isRunnable() {
				return true
			}
		}
	}
	return false
}

func (h CommandConfig) isRunnable() bool {
	return strings.TrimSpace(h.Command) != "" && h.enabled()
}

func (h CommandConfig) enabled() bool {
	if h.Enabled == nil {
		return true
	}
	return *h.Enabled
}

func parseSection(line string) (string, bool) {
	if strings.HasPrefix(line, "[[") && strings.HasSuffix(line, "]]") {
		return strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "[["), "]]")), true
	}
	if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
		return strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "["), "]")), true
	}
	return "", false
}

func parseHookSection(section string) (Event, bool) {
	section = strings.TrimSpace(section)
	for _, prefix := range []string{"runtime.hooks.", "hooks."} {
		if strings.HasPrefix(section, prefix) {
			event := Event(strings.TrimSpace(strings.TrimPrefix(section, prefix)))
			if isSupportedEvent(event) {
				return event, true
			}
		}
	}
	return "", false
}

func isSupportedEvent(event Event) bool {
	switch event {
	case EventBeforeRun, EventAfterRun, EventBeforeTool, EventAfterTool:
		return true
	default:
		return false
	}
}

func assignHookValue(cfg *CommandConfig, key, rawValue string) {
	switch key {
	case "command":
		cfg.Command = trimScalarValue(rawValue)
	case "args":
		cfg.Args = parseStringArray(rawValue)
	case "timeout_ms":
		cfg.TimeoutMS = parsePositiveInt(rawValue)
	case "enabled":
		if parsed, err := strconv.ParseBool(trimScalarValue(rawValue)); err == nil {
			cfg.Enabled = &parsed
		}
	}
}

func parseStringArray(value string) []string {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "[") || !strings.HasSuffix(value, "]") {
		if scalar := trimScalarValue(value); scalar != "" {
			return []string{scalar}
		}
		return nil
	}

	value = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(value, "["), "]"))
	if value == "" {
		return nil
	}

	var out []string
	var token strings.Builder
	inSingleQuote := false
	inDoubleQuote := false
	escaped := false

	flush := func() {
		item := trimScalarValue(token.String())
		if item != "" {
			out = append(out, item)
		}
		token.Reset()
	}

	for _, r := range value {
		switch {
		case escaped:
			token.WriteRune(r)
			escaped = false
		case r == '\\' && inDoubleQuote:
			token.WriteRune(r)
			escaped = true
		case r == '\'' && !inDoubleQuote:
			inSingleQuote = !inSingleQuote
			token.WriteRune(r)
		case r == '"' && !inSingleQuote:
			inDoubleQuote = !inDoubleQuote
			token.WriteRune(r)
		case r == ',' && !inSingleQuote && !inDoubleQuote:
			flush()
		default:
			token.WriteRune(r)
		}
	}
	flush()

	return out
}

func parsePositiveInt(value string) int {
	parsed, err := strconv.Atoi(trimScalarValue(value))
	if err != nil || parsed <= 0 {
		return 0
	}
	return parsed
}

func trimScalarValue(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 {
		if unquoted, err := strconv.Unquote(value); err == nil {
			return unquoted
		}
		if strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'") {
			return strings.TrimSuffix(strings.TrimPrefix(value, "'"), "'")
		}
	}
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
