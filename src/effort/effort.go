package effort

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

type Level string

const (
	LevelLow    Level = "low"
	LevelMedium Level = "medium"
	LevelHigh   Level = "high"
	LevelMax    Level = "max"
	LevelAuto   Level = "auto"
)

const (
	maxIntValue = int64(1<<(strconv.IntSize-1) - 1)
	minIntValue = -maxIntValue - 1
)

type Value struct {
	level   Level
	numeric *int
}

type Provider string

const (
	ProviderUnknown    Provider = ""
	ProviderFirstParty Provider = "first_party"
	ProviderAnthropic  Provider = "anthropic"
	ProviderOpenAI     Provider = "openai"
	ProviderLMStudio   Provider = "lmstudio"
	ProviderThirdParty Provider = "third_party"
)

type Source string

const (
	SourceNone         Source = ""
	SourceEnvironment  Source = "environment"
	SourceSession      Source = "session"
	SourceApplication  Source = "application"
	SourceModelDefault Source = "model_default"
)

var (
	ErrInvalidValue       = errors.New("invalid effort value")
	ErrNumericRequiresAnt = errors.New("numeric effort is ant-only")
)

func LevelValue(level Level) (Value, error) {
	if !IsLevel(level) {
		return Value{}, fmt.Errorf("%w: %q", ErrInvalidValue, level)
	}
	return Value{level: level}, nil
}

func NumericValue(n int) Value {
	return Value{numeric: &n}
}

func IsLevel(level Level) bool {
	switch level {
	case LevelLow, LevelMedium, LevelHigh, LevelMax:
		return true
	default:
		return false
	}
}

func (v Value) IsNumeric() bool {
	return v.numeric != nil
}

func (v Value) Numeric() (int, bool) {
	if v.numeric == nil {
		return 0, false
	}
	return *v.numeric, true
}

func (v Value) Level() (Level, bool) {
	if v.numeric != nil || !IsLevel(v.level) {
		return "", false
	}
	return v.level, true
}

func (v Value) String() string {
	if n, ok := v.Numeric(); ok {
		return strconv.Itoa(n)
	}
	if level, ok := v.Level(); ok {
		return string(level)
	}
	return ""
}

func (v Value) DisplayLevel(isAnt bool) Level {
	if level, ok := v.Level(); ok {
		return level
	}
	n, ok := v.Numeric()
	if !ok || !isAnt {
		return LevelHigh
	}
	if n <= 50 {
		return LevelLow
	}
	if n <= 85 {
		return LevelMedium
	}
	if n <= 100 {
		return LevelHigh
	}
	return LevelMax
}

func ParseValue(raw any) (Value, error) {
	switch value := raw.(type) {
	case nil:
		return Value{}, fmt.Errorf("%w: empty", ErrInvalidValue)
	case Value:
		if value.IsNumeric() {
			return value, nil
		}
		if _, ok := value.Level(); ok {
			return value, nil
		}
		return Value{}, fmt.Errorf("%w: %v", ErrInvalidValue, raw)
	case Level:
		return LevelValue(value)
	case string:
		return parseString(value)
	case int:
		return NumericValue(value), nil
	case int8:
		return NumericValue(int(value)), nil
	case int16:
		return NumericValue(int(value)), nil
	case int32:
		return NumericValue(int(value)), nil
	case int64:
		if value > maxIntValue || value < minIntValue {
			return Value{}, fmt.Errorf("%w: numeric value out of range", ErrInvalidValue)
		}
		return NumericValue(int(value)), nil
	case uint:
		if uint64(value) > uint64(maxIntValue) {
			return Value{}, fmt.Errorf("%w: numeric value out of range", ErrInvalidValue)
		}
		return NumericValue(int(value)), nil
	case uint8:
		return NumericValue(int(value)), nil
	case uint16:
		return NumericValue(int(value)), nil
	case uint32:
		if uint64(value) > uint64(maxIntValue) {
			return Value{}, fmt.Errorf("%w: numeric value out of range", ErrInvalidValue)
		}
		return NumericValue(int(value)), nil
	case uint64:
		if value > uint64(maxIntValue) {
			return Value{}, fmt.Errorf("%w: numeric value out of range", ErrInvalidValue)
		}
		return NumericValue(int(value)), nil
	case float32:
		return parseFloat(float64(value))
	case float64:
		return parseFloat(value)
	default:
		return Value{}, fmt.Errorf("%w: %T", ErrInvalidValue, raw)
	}
}

func parseString(raw string) (Value, error) {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	if normalized == "" {
		return Value{}, fmt.Errorf("%w: empty", ErrInvalidValue)
	}
	level := Level(normalized)
	if IsLevel(level) {
		return Value{level: level}, nil
	}
	n, err := strconv.Atoi(normalized)
	if err == nil {
		return NumericValue(n), nil
	}
	return Value{}, fmt.Errorf("%w: %q", ErrInvalidValue, raw)
}

func parseFloat(raw float64) (Value, error) {
	if math.IsNaN(raw) || math.IsInf(raw, 0) || math.Trunc(raw) != raw {
		return Value{}, fmt.Errorf("%w: numeric value must be an integer", ErrInvalidValue)
	}
	if raw > float64(maxIntValue) || raw < float64(minIntValue) {
		return Value{}, fmt.Errorf("%w: numeric value out of range", ErrInvalidValue)
	}
	return NumericValue(int(raw)), nil
}

func EnvOverride(raw string) (*Value, bool, error) {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	if normalized == "" {
		return nil, false, nil
	}
	if normalized == "unset" || normalized == string(LevelAuto) {
		return nil, true, nil
	}
	value, err := ParseValue(normalized)
	if err != nil {
		return nil, false, err
	}
	return &value, false, nil
}

type ModelCapabilities struct {
	SupportsEffort       *bool
	SupportsMaxEffort    *bool
	SupportsReasoning    *bool
	DefaultEffort        *Value
	LMStudioAllowed      []string
	SupportsAdaptive     *bool
	SupportsXHigh        *bool
	ProviderIsThirdParty bool
}

type ResolutionInput struct {
	Provider     Provider
	Model        string
	EnvRaw       string
	SessionValue *Value
	AppValue     *Value
	ModelDefault *Value
	Capabilities ModelCapabilities
	IsAnt        bool
}

type ResolvedEffort struct {
	Value             *Value
	Source            Source
	Suppressed        bool
	Omit              bool
	Reason            string
	DisplayLevel      Level
	SupportsEffort    bool
	SupportsMaxEffort bool
	Downgraded        bool
}

type CommandResult struct {
	Message     string
	Value       *Value
	Persistable *Level
	Cleared     bool
	Err         error
}

func SupportsEffort(provider Provider, model string, caps ModelCapabilities) bool {
	if caps.SupportsEffort != nil {
		return *caps.SupportsEffort
	}
	if caps.ProviderIsThirdParty || provider == ProviderThirdParty || provider == ProviderUnknown {
		return false
	}
	switch provider {
	case ProviderLMStudio:
		if caps.SupportsReasoning != nil {
			return *caps.SupportsReasoning
		}
		return len(caps.LMStudioAllowed) > 0
	case ProviderOpenAI:
		return openAIModelSupportsReasoningEffort(model)
	case ProviderFirstParty, ProviderAnthropic:
		return anthropicModelSupportsEffort(model)
	default:
		return false
	}
}

func SupportsMaxEffort(provider Provider, model string, caps ModelCapabilities, isAnt bool) bool {
	if caps.SupportsMaxEffort != nil {
		return *caps.SupportsMaxEffort
	}
	switch provider {
	case ProviderFirstParty, ProviderAnthropic:
		m := strings.ToLower(model)
		if strings.Contains(m, "opus-4-6") {
			return true
		}
		if strings.Contains(m, "4-7") || strings.Contains(m, "4.7") {
			return strings.Contains(m, "opus") || strings.Contains(m, "sonnet")
		}
		return isAnt && isKnownAnthropicModel(m)
	case ProviderLMStudio:
		allowed := normalizedLMStudioAllowed(caps.LMStudioAllowed)
		_, ok := allowed["xhigh"]
		return ok
	default:
		return false
	}
}

func ResolveAppliedEffort(input ResolutionInput) (ResolvedEffort, error) {
	supportsEffort := SupportsEffort(input.Provider, input.Model, input.Capabilities)
	supportsMax := SupportsMaxEffort(input.Provider, input.Model, input.Capabilities, input.IsAnt)
	base := ResolvedEffort{
		SupportsEffort:    supportsEffort,
		SupportsMaxEffort: supportsMax,
		DisplayLevel:      LevelHigh,
	}

	envValue, envSuppress, err := EnvOverride(input.EnvRaw)
	if err != nil {
		return base, err
	}
	if envSuppress {
		base.Suppressed = true
		base.Omit = true
		base.Source = SourceEnvironment
		base.Reason = "environment requested auto/unset"
		return base, nil
	}

	value, source := pickEffortValue(envValue, input)
	if value == nil {
		base.Omit = true
		base.Reason = "no effort value resolved"
		return base, nil
	}
	if value.IsNumeric() && !input.IsAnt {
		return base, ErrNumericRequiresAnt
	}
	if !supportsEffort {
		base.Omit = true
		base.Source = source
		base.Reason = "model/provider does not support effort"
		base.DisplayLevel = value.DisplayLevel(input.IsAnt)
		return base, nil
	}

	applied := *value
	if level, ok := applied.Level(); ok && level == LevelMax && !supportsMax {
		downgraded, _ := LevelValue(LevelHigh)
		applied = downgraded
		base.Downgraded = true
	}

	base.Value = &applied
	base.Source = source
	base.DisplayLevel = applied.DisplayLevel(input.IsAnt)
	return base, nil
}

func ToPersistable(value Value, isAnt bool) (Level, bool) {
	level, ok := value.Level()
	if !ok {
		return "", false
	}
	switch level {
	case LevelLow, LevelMedium, LevelHigh:
		return level, true
	case LevelMax:
		if isAnt {
			return level, true
		}
	}
	return "", false
}

func ExecuteCommand(args string, isAnt bool) CommandResult {
	normalized := strings.ToLower(strings.TrimSpace(args))
	if normalized == "" || normalized == "current" || normalized == "status" {
		return CommandResult{Message: "current effort requested"}
	}
	if normalized == string(LevelAuto) || normalized == "unset" {
		return CommandResult{
			Message: "Effort level set to auto",
			Cleared: true,
		}
	}
	value, err := ParseValue(normalized)
	if err != nil {
		return CommandResult{
			Message: "Invalid argument: " + args + ". Valid options are: low, medium, high, max, auto",
			Err:     err,
		}
	}
	if value.IsNumeric() && !isAnt {
		return CommandResult{
			Message: "Invalid argument: numeric effort is ant-only",
			Err:     ErrNumericRequiresAnt,
		}
	}
	var persistable *Level
	if level, ok := ToPersistable(value, isAnt); ok {
		persistable = &level
	}
	return CommandResult{
		Message:     "Set effort level to " + value.String(),
		Value:       &value,
		Persistable: persistable,
	}
}

func pickEffortValue(envValue *Value, input ResolutionInput) (*Value, Source) {
	if envValue != nil {
		return envValue, SourceEnvironment
	}
	if input.SessionValue != nil {
		return input.SessionValue, SourceSession
	}
	if input.AppValue != nil {
		return input.AppValue, SourceApplication
	}
	if input.ModelDefault != nil {
		return input.ModelDefault, SourceModelDefault
	}
	if input.Capabilities.DefaultEffort != nil {
		return input.Capabilities.DefaultEffort, SourceModelDefault
	}
	return nil, SourceNone
}

func anthropicModelSupportsEffort(model string) bool {
	m := strings.ToLower(model)
	if strings.Contains(m, "4-6") || strings.Contains(m, "4.6") ||
		strings.Contains(m, "4-7") || strings.Contains(m, "4.7") {
		return strings.Contains(m, "opus") || strings.Contains(m, "sonnet")
	}
	if isKnownAnthropicModel(m) {
		return false
	}
	return true
}

func isKnownAnthropicModel(model string) bool {
	m := strings.ToLower(model)
	return strings.Contains(m, "haiku") || strings.Contains(m, "sonnet") || strings.Contains(m, "opus")
}

func openAIModelSupportsReasoningEffort(model string) bool {
	m := strings.ToLower(strings.TrimSpace(model))
	return strings.HasPrefix(m, "o1") ||
		strings.HasPrefix(m, "o3") ||
		strings.HasPrefix(m, "o4") ||
		strings.HasPrefix(m, "gpt-5") ||
		strings.HasPrefix(m, "gpt-oss")
}
