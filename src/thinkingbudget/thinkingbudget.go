package thinkingbudget

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

type Mode string

const (
	ModeDisabled Mode = "disabled"
	ModeEnabled  Mode = "enabled"
	ModeAdaptive Mode = "adaptive"
)

type Level string

const (
	LevelLow    Level = "low"
	LevelMedium Level = "medium"
	LevelHigh   Level = "high"
	LevelMax    Level = "max"
	LevelXHigh  Level = "xhigh"
)

type Config struct {
	Mode            Mode
	BudgetTokens    int
	IncludeThoughts bool
	Level           Level
}

type Options struct {
	Mode            Mode
	BudgetTokens    int
	IncludeThoughts bool
	Level           Level
}

var ultrathinkPattern = regexp.MustCompile(`(?i)\bultrathink\b`)

// Normalize accepts either Options or map[string]any input and returns a
// canonical thinking config.
func Normalize(input any) (Config, error) {
	switch value := input.(type) {
	case nil:
		return Config{Mode: ModeDisabled}, nil
	case Options:
		return normalizeOptions(value)
	case *Options:
		if value == nil {
			return Config{Mode: ModeDisabled}, nil
		}
		return normalizeOptions(*value)
	case map[string]any:
		return normalizeMap(value)
	default:
		return Config{}, fmt.Errorf("unsupported thinking config input %T", input)
	}
}

func normalizeOptions(options Options) (Config, error) {
	if options.BudgetTokens < 0 {
		return Config{}, fmt.Errorf("thinking budget must be non-negative")
	}

	mode, err := normalizeMode(options.Mode)
	if err != nil {
		return Config{}, err
	}
	level, err := normalizeLevel(options.Level)
	if err != nil {
		return Config{}, err
	}

	return Config{
		Mode:            inferMode(mode, options.BudgetTokens, level),
		BudgetTokens:    options.BudgetTokens,
		IncludeThoughts: options.IncludeThoughts,
		Level:           level,
	}, nil
}

func normalizeMap(input map[string]any) (Config, error) {
	var cfg Config

	if raw, ok := firstValue(input, "mode", "type", "thinkingMode", "thinking_mode"); ok {
		mode, err := modeFromAny(raw)
		if err != nil {
			return Config{}, err
		}
		cfg.Mode = mode
	}

	if raw, ok := firstValue(input, "thinkingBudget", "thinking_budget"); ok {
		budget, err := nonNegativeInt(raw, "thinking budget")
		if err != nil {
			return Config{}, err
		}
		cfg.BudgetTokens = budget
	}

	if raw, ok := firstValue(input, "includeThoughts", "include_thoughts"); ok {
		include, err := boolFromAny(raw, "include thoughts")
		if err != nil {
			return Config{}, err
		}
		cfg.IncludeThoughts = include
	}

	if raw, ok := firstValue(input, "thinkingLevel", "thinking_level"); ok {
		level, err := levelFromAny(raw)
		if err != nil {
			return Config{}, err
		}
		cfg.Level = level
	}

	cfg.Mode = inferMode(cfg.Mode, cfg.BudgetTokens, cfg.Level)
	return cfg, nil
}

func firstValue(input map[string]any, keys ...string) (any, bool) {
	for _, key := range keys {
		value, ok := input[key]
		if ok {
			return value, true
		}
	}
	return nil, false
}

func modeFromAny(value any) (Mode, error) {
	switch typed := value.(type) {
	case string:
		return normalizeMode(Mode(typed))
	case Mode:
		return normalizeMode(typed)
	default:
		return "", fmt.Errorf("thinking mode must be a string")
	}
}

func normalizeMode(mode Mode) (Mode, error) {
	switch normalized := Mode(strings.ToLower(strings.TrimSpace(string(mode)))); normalized {
	case "":
		return "", nil
	case ModeDisabled, ModeEnabled, ModeAdaptive:
		return normalized, nil
	default:
		return "", fmt.Errorf("unknown thinking mode %q", mode)
	}
}

func levelFromAny(value any) (Level, error) {
	switch typed := value.(type) {
	case string:
		return normalizeLevel(Level(typed))
	case Level:
		return normalizeLevel(typed)
	default:
		return "", fmt.Errorf("thinking level must be a string")
	}
}

func normalizeLevel(level Level) (Level, error) {
	switch normalized := Level(strings.ToLower(strings.TrimSpace(string(level)))); normalized {
	case "":
		return "", nil
	case LevelLow, LevelMedium, LevelHigh, LevelMax, LevelXHigh:
		return normalized, nil
	default:
		return "", fmt.Errorf("unknown thinking level %q", level)
	}
}

func boolFromAny(value any, name string) (bool, error) {
	typed, ok := value.(bool)
	if !ok {
		return false, fmt.Errorf("%s must be a bool", name)
	}
	return typed, nil
}

func nonNegativeInt(value any, name string) (int, error) {
	result, ok := intFromAny(value)
	if !ok {
		return 0, fmt.Errorf("%s must be numeric", name)
	}
	if result < 0 {
		return 0, fmt.Errorf("%s must be non-negative", name)
	}
	return result, nil
}

func intFromAny(value any) (int, bool) {
	switch typed := value.(type) {
	case bool:
		return 0, false
	case int:
		return typed, true
	case int8:
		return int(typed), true
	case int16:
		return int(typed), true
	case int32:
		return int(typed), true
	case int64:
		if typed > int64(maxInt()) || typed < int64(minInt()) {
			return 0, false
		}
		return int(typed), true
	case uint:
		return uint64ToInt(uint64(typed))
	case uint8:
		return int(typed), true
	case uint16:
		return int(typed), true
	case uint32:
		return uint64ToInt(uint64(typed))
	case uint64:
		return uint64ToInt(typed)
	case uintptr:
		return uint64ToInt(uint64(typed))
	case float32:
		return float64ToInt(float64(typed))
	case float64:
		return float64ToInt(typed)
	case json.Number:
		return jsonNumberToInt(typed)
	default:
		return 0, false
	}
}

func jsonNumberToInt(value json.Number) (int, bool) {
	if parsed, err := value.Int64(); err == nil {
		return intFromAny(parsed)
	}
	parsed, err := strconv.ParseFloat(value.String(), 64)
	if err != nil {
		return 0, false
	}
	return float64ToInt(parsed)
}

func uint64ToInt(value uint64) (int, bool) {
	if value > uint64(maxInt()) {
		return 0, false
	}
	return int(value), true
}

func float64ToInt(value float64) (int, bool) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, false
	}
	if value > float64(maxInt()) || value < float64(minInt()) {
		return 0, false
	}
	return int(value), true
}

func maxInt() int {
	return int(^uint(0) >> 1)
}

func minInt() int {
	return -maxInt() - 1
}

func inferMode(mode Mode, budgetTokens int, level Level) Mode {
	if mode != "" {
		return mode
	}
	if level != "" {
		return ModeAdaptive
	}
	if budgetTokens > 0 {
		return ModeEnabled
	}
	return ModeDisabled
}

// HasUltrathinkKeyword reports whether text contains "ultrathink" as a word.
func HasUltrathinkKeyword(text string) bool {
	return ultrathinkPattern.MatchString(text)
}

// SupportsThinking reports conservative support for model-side thinking.
func SupportsThinking(model, provider string) bool {
	canonical := canonicalModel(model)
	if canonical == "" || strings.Contains(canonical, "claude-3-") {
		return false
	}
	if strings.Contains(canonical, "sonnet-4") || strings.Contains(canonical, "opus-4") {
		return true
	}
	if isFirstPartyProvider(provider) {
		return true
	}
	return strings.Contains(canonical, "claude-4")
}

// SupportsAdaptive reports conservative support for adaptive thinking.
func SupportsAdaptive(model, provider string) bool {
	canonical := canonicalModel(model)
	if canonical == "" || strings.Contains(canonical, "claude-3-") {
		return false
	}
	if claudeMinorAtLeast(canonical, 6) {
		return true
	}
	if strings.Contains(canonical, "opus") ||
		strings.Contains(canonical, "sonnet") ||
		strings.Contains(canonical, "haiku") {
		return false
	}
	return isFirstPartyProvider(provider)
}

func canonicalModel(model string) string {
	model = strings.TrimSpace(strings.ToLower(model))
	model = strings.NewReplacer("_", "-", ".", "-").Replace(model)
	for strings.Contains(model, "--") {
		model = strings.ReplaceAll(model, "--", "-")
	}
	return model
}

func isFirstPartyProvider(provider string) bool {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "firstparty", "first-party", "first_party", "foundry":
		return true
	default:
		return false
	}
}

func claudeMinorAtLeast(model string, minMinor int) bool {
	if !strings.Contains(model, "claude") &&
		!strings.Contains(model, "opus") &&
		!strings.Contains(model, "sonnet") &&
		!strings.Contains(model, "haiku") {
		return false
	}

	parts := strings.Split(model, "-")
	for i, part := range parts {
		if part != "4" || i+1 >= len(parts) {
			continue
		}
		next := parts[i+1]
		if len(next) > 2 {
			continue
		}
		minor, err := strconv.Atoi(next)
		if err == nil && minor >= minMinor {
			return true
		}
	}
	return false
}

// ResolveMaxTokens returns the positive requested token cap, or the positive
// model default, clamped to contextRemaining when a context limit is known.
func ResolveMaxTokens(requested, modelDefault, contextRemaining int) (int, error) {
	resolved := requested
	if resolved <= 0 {
		resolved = modelDefault
	}
	if resolved <= 0 {
		return 0, fmt.Errorf("max tokens must resolve to a positive value")
	}
	if contextRemaining > 0 && resolved > contextRemaining {
		resolved = contextRemaining
	}
	if resolved <= 0 {
		return 0, fmt.Errorf("max tokens must resolve to a positive value")
	}
	return resolved, nil
}
