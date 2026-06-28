package permissionproduct

import (
	"strings"
	"time"
)

type PermissionMode string

const (
	PermissionModePlan        PermissionMode = "plan"
	PermissionModeAsk         PermissionMode = "ask"
	PermissionModeAuto        PermissionMode = "auto"
	PermissionModeAcceptEdits PermissionMode = "accept_edits"
	PermissionModeBypass      PermissionMode = "bypass"
	PermissionModeDontAsk     PermissionMode = "dont_ask"
)

type RiskLevel string

const (
	RiskLevelUnknown  RiskLevel = "unknown"
	RiskLevelLow      RiskLevel = "low"
	RiskLevelMedium   RiskLevel = "medium"
	RiskLevelHigh     RiskLevel = "high"
	RiskLevelCritical RiskLevel = "critical"
)

type RuleScope string

const (
	RuleScopeSession RuleScope = "session"
	RuleScopeProject RuleScope = "project"
	RuleScopeGlobal  RuleScope = "global"
	RuleScopeAgent   RuleScope = "agent"
)

type DecisionOutcome string

const (
	DecisionOutcomeAllow   DecisionOutcome = "allow"
	DecisionOutcomeDeny    DecisionOutcome = "deny"
	DecisionOutcomeAsk     DecisionOutcome = "ask"
	DecisionOutcomePending DecisionOutcome = "pending"
)

type DecisionPersistence string

const (
	DecisionPersistenceOnce   DecisionPersistence = "once"
	DecisionPersistenceAlways DecisionPersistence = "always"
)

type DecisionSource string

const (
	DecisionSourceMode       DecisionSource = "mode"
	DecisionSourceRule       DecisionSource = "rule"
	DecisionSourceUser       DecisionSource = "user"
	DecisionSourceExpiration DecisionSource = "expiration"
)

type PermissionKey struct {
	Tool   string
	Action string
	Path   string
}

type AgentAttribution struct {
	ID         string
	Type       string
	Name       string
	ParentID   string
	Background bool
}

type PermissionRequest struct {
	ID          string
	SessionID   string
	ProjectID   string
	Mode        PermissionMode
	Tool        string
	Action      string
	Path        string
	Description string
	RiskLevel   RiskLevel
	Agent       AgentAttribution
	TTL         time.Duration
	CreatedAt   time.Time
	ExpiresAt   time.Time
	Metadata    map[string]string
}

type PermissionDecision struct {
	ID          string
	Sequence    int64
	RequestID   string
	Outcome     DecisionOutcome
	Persistence DecisionPersistence
	Source      DecisionSource
	Mode        PermissionMode
	Scope       RuleScope
	RuleID      string
	Key         PermissionKey
	Agent       AgentAttribution
	RiskLevel   RiskLevel
	Reason      string
	CreatedAt   time.Time
	ExpiresAt   time.Time
}

type Rule struct {
	ID        string
	Scope     RuleScope
	Behavior  DecisionOutcome
	SessionID string
	ProjectID string
	Key       PermissionKey
	Agent     AgentAttribution
	RiskLevel RiskLevel
	TTL       time.Duration
	CreatedAt time.Time
	ExpiresAt time.Time
	Reason    string
	Metadata  map[string]string
}

type Resolution struct {
	Outcome   DecisionOutcome
	Remember  DecisionPersistence
	Scope     RuleScope
	Reason    string
	RuleTTL   time.Duration
	ExpiresAt time.Time
}

func (r PermissionRequest) Key() PermissionKey {
	return PermissionKey{
		Tool:   r.Tool,
		Action: r.Action,
		Path:   r.Path,
	}.Normalize()
}

func (r PermissionRequest) IsExpired(now time.Time) bool {
	return !r.ExpiresAt.IsZero() && !now.Before(r.ExpiresAt)
}

func (r Rule) IsExpired(now time.Time) bool {
	return !r.ExpiresAt.IsZero() && !now.Before(r.ExpiresAt)
}

func (r Rule) Matches(request PermissionRequest, now time.Time) bool {
	if r.IsExpired(now) {
		return false
	}
	if r.Behavior != DecisionOutcomeAllow && r.Behavior != DecisionOutcomeDeny {
		return false
	}
	if !r.Key.Matches(request.Key()) {
		return false
	}
	switch r.Scope {
	case RuleScopeSession:
		if r.SessionID == "" || r.SessionID != strings.TrimSpace(request.SessionID) {
			return false
		}
	case RuleScopeProject:
		if r.ProjectID == "" || r.ProjectID != strings.TrimSpace(request.ProjectID) {
			return false
		}
	case RuleScopeAgent:
		if !r.Agent.matches(request.Agent) {
			return false
		}
	case RuleScopeGlobal:
	default:
		return false
	}
	if r.Behavior == DecisionOutcomeAllow && !riskAllowedByRule(r.RiskLevel, request.RiskLevel) {
		return false
	}
	return true
}

func (k PermissionKey) Normalize() PermissionKey {
	return PermissionKey{
		Tool:   strings.TrimSpace(k.Tool),
		Action: strings.ToLower(strings.TrimSpace(k.Action)),
		Path:   strings.TrimSpace(k.Path),
	}
}

func (k PermissionKey) Matches(request PermissionKey) bool {
	k = k.Normalize()
	request = request.Normalize()
	if k.Tool != "" && k.Tool != request.Tool {
		return false
	}
	if k.Action != "" && k.Action != request.Action {
		return false
	}
	if k.Path != "" && k.Path != request.Path {
		return false
	}
	return true
}

func (k PermissionKey) dedupeString() string {
	k = k.Normalize()
	return k.Tool + "\x00" + k.Action + "\x00" + k.Path
}

func (a AgentAttribution) Normalize() AgentAttribution {
	return AgentAttribution{
		ID:         strings.TrimSpace(a.ID),
		Type:       strings.TrimSpace(a.Type),
		Name:       strings.TrimSpace(a.Name),
		ParentID:   strings.TrimSpace(a.ParentID),
		Background: a.Background,
	}
}

func (a AgentAttribution) matches(request AgentAttribution) bool {
	a = a.Normalize()
	request = request.Normalize()
	if a.ID != "" {
		return a.ID == request.ID
	}
	if a.Type != "" {
		return a.Type == request.Type
	}
	return false
}

func normalizeRiskLevel(risk RiskLevel) RiskLevel {
	switch RiskLevel(strings.ToLower(strings.TrimSpace(string(risk)))) {
	case RiskLevelLow:
		return RiskLevelLow
	case RiskLevelMedium:
		return RiskLevelMedium
	case RiskLevelHigh:
		return RiskLevelHigh
	case RiskLevelCritical:
		return RiskLevelCritical
	default:
		return RiskLevelUnknown
	}
}

func riskRank(risk RiskLevel) int {
	switch normalizeRiskLevel(risk) {
	case RiskLevelLow:
		return 1
	case RiskLevelMedium:
		return 2
	case RiskLevelHigh:
		return 3
	case RiskLevelCritical:
		return 4
	default:
		return 3
	}
}

func riskAtMost(risk RiskLevel, max RiskLevel) bool {
	return riskRank(risk) <= riskRank(max)
}

func riskAllowedByRule(ruleRisk RiskLevel, requestRisk RiskLevel) bool {
	if ruleRisk == "" {
		return true
	}
	return riskAtMost(requestRisk, ruleRisk)
}

func normalizeMode(mode PermissionMode) PermissionMode {
	switch PermissionMode(strings.ToLower(strings.TrimSpace(string(mode)))) {
	case PermissionModePlan:
		return PermissionModePlan
	case PermissionModeAuto:
		return PermissionModeAuto
	case PermissionModeAcceptEdits:
		return PermissionModeAcceptEdits
	case PermissionModeBypass:
		return PermissionModeBypass
	case PermissionModeDontAsk:
		return PermissionModeDontAsk
	default:
		return PermissionModeAsk
	}
}

func normalizeScope(scope RuleScope) RuleScope {
	switch RuleScope(strings.ToLower(strings.TrimSpace(string(scope)))) {
	case RuleScopeProject:
		return RuleScopeProject
	case RuleScopeGlobal:
		return RuleScopeGlobal
	case RuleScopeAgent:
		return RuleScopeAgent
	default:
		return RuleScopeSession
	}
}

func normalizePersistence(persistence DecisionPersistence) DecisionPersistence {
	switch DecisionPersistence(strings.ToLower(strings.TrimSpace(string(persistence)))) {
	case DecisionPersistenceAlways:
		return DecisionPersistenceAlways
	default:
		return DecisionPersistenceOnce
	}
}

func normalizeRuleBehavior(outcome DecisionOutcome) DecisionOutcome {
	switch DecisionOutcome(strings.ToLower(strings.TrimSpace(string(outcome)))) {
	case DecisionOutcomeDeny:
		return DecisionOutcomeDeny
	default:
		return DecisionOutcomeAllow
	}
}

func copyMetadata(metadata map[string]string) map[string]string {
	if len(metadata) == 0 {
		return nil
	}
	copied := make(map[string]string, len(metadata))
	for key, value := range metadata {
		copied[key] = value
	}
	return copied
}
