package credentialpool

import (
	"errors"
	"strings"
	"time"
)

type Strategy string

const (
	StrategyLeastUsed          Strategy = "least_used"
	StrategyRoundRobin         Strategy = "round_robin"
	StrategyStickyUntilFailure Strategy = "sticky_until_failure"
)

type FailureReason string

const (
	FailureReasonUnknown      FailureReason = "unknown"
	FailureReasonUnauthorized FailureReason = "unauthorized"
	FailureReasonRateLimit    FailureReason = "rate_limit"
)

const DefaultUnauthorizedCooldown = 5 * time.Minute

var (
	ErrEmptyProvider      = errors.New("provider is required")
	ErrNoCredentials      = errors.New("at least one credential is required")
	ErrDuplicateID        = errors.New("credential ids must be unique")
	ErrCredentialNotFound = errors.New("credential not found")
	ErrInvalidStrategy    = errors.New("unsupported credential pool strategy")
)

type ProviderCredential struct {
	ID       string            `json:"id"`
	Provider string            `json:"provider"`
	Label    string            `json:"label,omitempty"`
	Secret   string            `json:"-"`
	BaseURL  string            `json:"base_url,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

type UsageStats struct {
	Requests             int           `json:"requests"`
	Successes            int           `json:"successes"`
	Failures             int           `json:"failures"`
	UnauthorizedFailures int           `json:"unauthorized_failures"`
	RateLimitFailures    int           `json:"rate_limit_failures"`
	LastUsedAt           time.Time     `json:"last_used_at,omitempty"`
	LastSuccessAt        time.Time     `json:"last_success_at,omitempty"`
	LastFailureAt        time.Time     `json:"last_failure_at,omitempty"`
	LastFailureReason    FailureReason `json:"last_failure_reason,omitempty"`
}

type Selector struct {
	strategy Strategy
}

func NewSelector(strategy Strategy) (Selector, error) {
	if strategy == "" {
		strategy = StrategyLeastUsed
	}
	if !strategy.Valid() {
		return Selector{}, ErrInvalidStrategy
	}
	return Selector{strategy: strategy}, nil
}

func (s Selector) Strategy() Strategy {
	return s.strategy
}

func (s Strategy) Valid() bool {
	switch s {
	case StrategyLeastUsed, StrategyRoundRobin, StrategyStickyUntilFailure:
		return true
	default:
		return false
	}
}

func FailureReasonFromHTTPStatus(status int) FailureReason {
	switch status {
	case 401, 403:
		return FailureReasonUnauthorized
	case 429:
		return FailureReasonRateLimit
	default:
		return FailureReasonUnknown
	}
}

func NormalizeFailureReason(reason FailureReason) FailureReason {
	switch FailureReason(strings.ToLower(strings.TrimSpace(string(reason)))) {
	case FailureReasonUnauthorized, "401", "auth", "authentication", "forbidden":
		return FailureReasonUnauthorized
	case FailureReasonRateLimit, "429", "rate_limited", "too_many_requests":
		return FailureReasonRateLimit
	case "":
		return FailureReasonUnknown
	default:
		return FailureReasonUnknown
	}
}

type CredentialSnapshot struct {
	ID                 string            `json:"id"`
	Provider           string            `json:"provider"`
	Label              string            `json:"label,omitempty"`
	BaseURL            string            `json:"base_url,omitempty"`
	Metadata           map[string]string `json:"metadata,omitempty"`
	Stats              UsageStats        `json:"stats"`
	DisabledUntil      time.Time         `json:"disabled_until,omitempty"`
	LastFailureReason  FailureReason     `json:"last_failure_reason,omitempty"`
	LastFailureMessage string            `json:"last_failure_message,omitempty"`
}

type PoolSnapshot struct {
	Provider         string               `json:"provider"`
	Strategy         Strategy             `json:"strategy"`
	RoundRobinCursor int                  `json:"round_robin_cursor"`
	StickyID         string               `json:"sticky_id,omitempty"`
	Credentials      []CredentialSnapshot `json:"credentials"`
}

type credentialState struct {
	credential         ProviderCredential
	stats              UsageStats
	disabledUntil      time.Time
	lastFailureReason  FailureReason
	lastFailureMessage string
}
