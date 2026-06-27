package compactengine

type ThresholdStateName string

const (
	ThresholdStateOK       ThresholdStateName = "ok"
	ThresholdStateWarning  ThresholdStateName = "warning"
	ThresholdStateError    ThresholdStateName = "error"
	ThresholdStateBlocking ThresholdStateName = "blocking"
)

type AutoCompactDecisionReason string

const (
	AutoCompactDecisionAutoThreshold        AutoCompactDecisionReason = "auto_threshold"
	AutoCompactDecisionUnderThreshold       AutoCompactDecisionReason = "under_threshold"
	AutoCompactDecisionCircuitBreakerOpen   AutoCompactDecisionReason = "circuit_breaker_open"
	AutoCompactDecisionDisabled             AutoCompactDecisionReason = "disabled"
	AutoCompactDecisionInvalidContextWindow AutoCompactDecisionReason = "invalid_context_window"
)

type AutoCompactConfig struct {
	ContextWindowTokens       int
	MaxOutputTokensForSummary int
	AutoCompactBufferTokens   int
	WarningBufferTokens       int
	ErrorBufferTokens         int
	ManualCompactBufferTokens int
	AutoCompactEnabled        bool
	DisableAutoCompact        bool
	MaxConsecutiveFailures    int
}

func DefaultAutoCompactConfig() AutoCompactConfig {
	return AutoCompactConfig{
		MaxOutputTokensForSummary: 20000,
		AutoCompactBufferTokens:   13000,
		WarningBufferTokens:       20000,
		ErrorBufferTokens:         20000,
		ManualCompactBufferTokens: 3000,
		AutoCompactEnabled:        true,
		MaxConsecutiveFailures:    3,
	}
}

type TokenThresholdState struct {
	TokenUsage                  int
	EffectiveContextWindow      int
	AutoCompactThreshold        int
	WarningThreshold            int
	ErrorThreshold              int
	BlockingLimit               int
	PercentLeft                 int
	IsAboveWarningThreshold     bool
	IsAboveErrorThreshold       bool
	IsAboveAutoCompactThreshold bool
	IsAtBlockingLimit           bool
	State                       ThresholdStateName
}

type CircuitBreakerState struct {
	ConsecutiveFailures int
}

func (s CircuitBreakerState) RecordFailure() CircuitBreakerState {
	s.ConsecutiveFailures++
	return s
}

func (s CircuitBreakerState) RecordSuccess() CircuitBreakerState {
	s.ConsecutiveFailures = 0
	return s
}

func (s CircuitBreakerState) IsOpen(maxConsecutiveFailures int) bool {
	return maxConsecutiveFailures > 0 && s.ConsecutiveFailures >= maxConsecutiveFailures
}

type AutoCompactDecision struct {
	ShouldCompact bool
	Reason        AutoCompactDecisionReason
	Thresholds    TokenThresholdState
	CircuitOpen   bool
}

func CalculateTokenThresholdState(tokenUsage int, config AutoCompactConfig) TokenThresholdState {
	config = config.withDefaults()
	autoEnabled := config.autoCompactEnabled()
	effectiveWindow := config.ContextWindowTokens - config.MaxOutputTokensForSummary
	if effectiveWindow < 0 {
		effectiveWindow = 0
	}
	autoThreshold := clampNonNegative(effectiveWindow - config.AutoCompactBufferTokens)
	decisionThreshold := effectiveWindow
	if autoEnabled {
		decisionThreshold = autoThreshold
	}
	warningThreshold := bufferedThreshold(decisionThreshold, config.WarningBufferTokens)
	errorThreshold := bufferedThreshold(decisionThreshold, config.ErrorBufferTokens)
	blockingLimit := bufferedThreshold(effectiveWindow, config.ManualCompactBufferTokens)

	percentLeft := 0
	if decisionThreshold > 0 {
		remaining := decisionThreshold - tokenUsage
		if remaining < 0 {
			remaining = 0
		}
		percentLeft = (remaining * 100) / decisionThreshold
	}

	state := ThresholdStateOK
	isAboveWarning := tokenUsage >= warningThreshold && decisionThreshold > 0
	isAboveError := tokenUsage >= errorThreshold && decisionThreshold > 0
	isAboveAuto := autoEnabled && tokenUsage >= autoThreshold && autoThreshold > 0
	isBlocking := tokenUsage >= blockingLimit && blockingLimit > 0
	if isBlocking {
		state = ThresholdStateBlocking
	} else if isAboveError {
		state = ThresholdStateError
	} else if isAboveWarning {
		state = ThresholdStateWarning
	}

	return TokenThresholdState{
		TokenUsage:                  tokenUsage,
		EffectiveContextWindow:      effectiveWindow,
		AutoCompactThreshold:        autoThreshold,
		WarningThreshold:            warningThreshold,
		ErrorThreshold:              errorThreshold,
		BlockingLimit:               blockingLimit,
		PercentLeft:                 percentLeft,
		IsAboveWarningThreshold:     isAboveWarning,
		IsAboveErrorThreshold:       isAboveError,
		IsAboveAutoCompactThreshold: isAboveAuto,
		IsAtBlockingLimit:           isBlocking,
		State:                       state,
	}
}

func ShouldAutoCompact(tokenUsage int, config AutoCompactConfig, breaker CircuitBreakerState) AutoCompactDecision {
	config = config.withDefaults()
	thresholds := CalculateTokenThresholdState(tokenUsage, config)
	if config.ContextWindowTokens <= 0 {
		return AutoCompactDecision{Reason: AutoCompactDecisionInvalidContextWindow, Thresholds: thresholds}
	}
	if !config.autoCompactEnabled() {
		return AutoCompactDecision{Reason: AutoCompactDecisionDisabled, Thresholds: thresholds}
	}
	if breaker.IsOpen(config.MaxConsecutiveFailures) {
		return AutoCompactDecision{
			Reason:      AutoCompactDecisionCircuitBreakerOpen,
			Thresholds:  thresholds,
			CircuitOpen: true,
		}
	}
	if thresholds.IsAboveAutoCompactThreshold {
		return AutoCompactDecision{
			ShouldCompact: true,
			Reason:        AutoCompactDecisionAutoThreshold,
			Thresholds:    thresholds,
		}
	}
	return AutoCompactDecision{Reason: AutoCompactDecisionUnderThreshold, Thresholds: thresholds}
}

func (c AutoCompactConfig) withDefaults() AutoCompactConfig {
	defaults := DefaultAutoCompactConfig()
	if c.MaxOutputTokensForSummary <= 0 {
		c.MaxOutputTokensForSummary = defaults.MaxOutputTokensForSummary
	}
	if c.AutoCompactBufferTokens <= 0 {
		c.AutoCompactBufferTokens = defaults.AutoCompactBufferTokens
	}
	if c.WarningBufferTokens <= 0 {
		c.WarningBufferTokens = defaults.WarningBufferTokens
	}
	if c.ErrorBufferTokens <= 0 {
		c.ErrorBufferTokens = defaults.ErrorBufferTokens
	}
	if c.ManualCompactBufferTokens <= 0 {
		c.ManualCompactBufferTokens = defaults.ManualCompactBufferTokens
	}
	if c.MaxConsecutiveFailures <= 0 {
		c.MaxConsecutiveFailures = defaults.MaxConsecutiveFailures
	}
	if !c.DisableAutoCompact && !c.AutoCompactEnabled {
		c.AutoCompactEnabled = defaults.AutoCompactEnabled
	}
	return c
}

func (c AutoCompactConfig) autoCompactEnabled() bool {
	return c.AutoCompactEnabled && !c.DisableAutoCompact
}

func clampNonNegative(value int) int {
	if value < 0 {
		return 0
	}
	return value
}

func bufferedThreshold(limit, buffer int) int {
	if limit <= 0 {
		return 0
	}
	if buffer <= 0 || buffer >= limit {
		return limit
	}
	return limit - buffer
}
