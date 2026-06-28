package hooktimeout

import (
	"fmt"
	"sort"
	"time"
)

const (
	DefaultPromptTimeout   = 30 * time.Second
	DefaultHTTPTimeout     = 30 * time.Second
	DefaultAgentTimeout    = 60 * time.Second
	DefaultSessionTimeout  = 5 * time.Second
	DefaultFunctionTimeout = 5 * time.Second
	DefaultMaxTimeout      = 120 * time.Second
)

type HookType string

const (
	HookTypePrompt   HookType = "prompt"
	HookTypeHTTP     HookType = "http"
	HookTypeAgent    HookType = "agent"
	HookTypeSession  HookType = "session"
	HookTypeFunction HookType = "function"
)

type Source string

const (
	SourceManaged Source = "managed"
	SourceProject Source = "project"
	SourceUser    Source = "user"
)

type Status string

const (
	StatusPlanned Status = "planned"
	StatusSkipped Status = "skipped"
)

// Policy describes the boundaries used while turning hook config into a plan.
type Policy struct {
	DefaultTimeouts map[HookType]time.Duration
	MaxTimeout      time.Duration
	DisableAll      bool
	ManagedOnly     bool
	RejectOverMax   bool
}

// Hook is the minimum hook metadata needed to compute a timeout plan.
type Hook struct {
	ID             string
	Name           string
	Type           HookType
	Source         Source
	TimeoutSeconds *int
	Order          int
}

// Plan contains runnable steps plus skipped decisions for explanation.
type Plan struct {
	Steps   []Step
	Skipped []Decision
}

type Step struct {
	Hook      Hook
	Source    Source
	Type      HookType
	Timeout   time.Duration
	TimeoutMS int64
	Status    Status
	Reason    string
}

type Decision struct {
	Hook   Hook
	Source Source
	Type   HookType
	Status Status
	Reason string
}

// BuildPlan converts hooks into runnable steps, keeping disabled hooks separate.
func BuildPlan(hooks []Hook, policy Policy) (Plan, error) {
	policy = normalizePolicy(policy)

	plan := Plan{
		Steps:   make([]Step, 0, len(hooks)),
		Skipped: make([]Decision, 0),
	}
	for _, hook := range hooks {
		if policy.DisableAll {
			plan.Skipped = append(plan.Skipped, skippedDecision(hook, "all hooks disabled by policy"))
			continue
		}
		if policy.ManagedOnly && hook.Source != SourceManaged {
			plan.Skipped = append(plan.Skipped, skippedDecision(hook, "skipped because policy allows managed hooks only"))
			continue
		}

		timeout, reason, err := timeoutForHook(hook, policy)
		if err != nil {
			return Plan{}, err
		}
		plan.Steps = append(plan.Steps, Step{
			Hook:      hook,
			Source:    hook.Source,
			Type:      hook.Type,
			Timeout:   timeout,
			TimeoutMS: timeout.Milliseconds(),
			Status:    StatusPlanned,
			Reason:    reason,
		})
	}

	sort.SliceStable(plan.Steps, func(i, j int) bool {
		return plan.Steps[i].Hook.Order < plan.Steps[j].Hook.Order
	})
	sort.SliceStable(plan.Skipped, func(i, j int) bool {
		return plan.Skipped[i].Hook.Order < plan.Skipped[j].Hook.Order
	})
	return plan, nil
}

func skippedDecision(hook Hook, reason string) Decision {
	return Decision{
		Hook:   hook,
		Source: hook.Source,
		Type:   hook.Type,
		Status: StatusSkipped,
		Reason: reason,
	}
}

func timeoutForHook(hook Hook, policy Policy) (time.Duration, string, error) {
	var timeout time.Duration
	var reason string
	if hook.TimeoutSeconds != nil {
		explicit, err := durationFromSeconds(*hook.TimeoutSeconds)
		if err != nil {
			return 0, "", fmt.Errorf("hook %s explicit timeout: %w", hookLabel(hook), err)
		}
		timeout = explicit
		reason = "using explicit hook timeout"
	} else {
		defaultTimeout, ok := policy.DefaultTimeouts[hook.Type]
		if !ok {
			return 0, "", fmt.Errorf("hook %s has no default timeout for type %q", hookLabel(hook), hook.Type)
		}
		if defaultTimeout <= 0 {
			return 0, "", fmt.Errorf("hook %s default timeout for type %q must be positive", hookLabel(hook), hook.Type)
		}
		timeout = defaultTimeout
		reason = fmt.Sprintf("using default %s hook timeout", hook.Type)
	}

	if timeout > policy.MaxTimeout {
		if policy.RejectOverMax {
			return 0, "", fmt.Errorf("hook %s timeout %s exceeds max %s", hookLabel(hook), timeout, policy.MaxTimeout)
		}
		return policy.MaxTimeout, fmt.Sprintf("%s; clamped to max %s", reason, policy.MaxTimeout), nil
	}
	return timeout, reason, nil
}

func durationFromSeconds(seconds int) (time.Duration, error) {
	if seconds <= 0 {
		return 0, fmt.Errorf("must be greater than 0 seconds")
	}
	const maxDurationSeconds = int64(1<<63-1) / int64(time.Second)
	if int64(seconds) > maxDurationSeconds {
		return 0, fmt.Errorf("exceeds maximum Go duration")
	}
	return time.Duration(seconds) * time.Second, nil
}

func normalizePolicy(policy Policy) Policy {
	defaults := map[HookType]time.Duration{
		HookTypePrompt:   DefaultPromptTimeout,
		HookTypeHTTP:     DefaultHTTPTimeout,
		HookTypeAgent:    DefaultAgentTimeout,
		HookTypeSession:  DefaultSessionTimeout,
		HookTypeFunction: DefaultFunctionTimeout,
	}
	for hookType, timeout := range policy.DefaultTimeouts {
		defaults[hookType] = timeout
	}
	policy.DefaultTimeouts = defaults
	if policy.MaxTimeout <= 0 {
		policy.MaxTimeout = DefaultMaxTimeout
	}
	return policy
}

func hookLabel(hook Hook) string {
	if hook.ID != "" {
		return hook.ID
	}
	if hook.Name != "" {
		return hook.Name
	}
	return fmt.Sprintf("%s/%s", hook.Source, hook.Type)
}
