package permissionproduct

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

const defaultPendingTTL = 30 * time.Minute

type Service struct {
	mu         sync.Mutex
	mode       PermissionMode
	defaultTTL time.Duration
	clock      func() time.Time
	pending    PendingQueue
	rules      []Rule
	history    []PermissionDecision
	sequence   int64
	idSequence int64
}

type ServiceOption func(*Service)

func NewService(options ...ServiceOption) *Service {
	service := &Service{
		mode:       PermissionModeAsk,
		defaultTTL: defaultPendingTTL,
		clock:      time.Now,
		pending:    NewPendingQueue(),
	}
	for _, option := range options {
		option(service)
	}
	if service.clock == nil {
		service.clock = time.Now
	}
	service.mode = normalizeMode(service.mode)
	return service
}

func WithMode(mode PermissionMode) ServiceOption {
	return func(service *Service) {
		service.mode = mode
	}
}

func WithDefaultTTL(ttl time.Duration) ServiceOption {
	return func(service *Service) {
		service.defaultTTL = ttl
	}
}

func WithClock(clock func() time.Time) ServiceOption {
	return func(service *Service) {
		service.clock = clock
	}
}

func (s *Service) SubmitRequest(request PermissionRequest) PermissionDecision {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	s.expirePendingLocked(now)
	s.pruneRulesLocked(now)

	request = s.normalizeRequestLocked(request, now)
	if decision, ok := s.applyRulesLocked(request, now); ok {
		s.recordLocked(&decision)
		return decision
	}

	decision := s.evaluateModeLocked(request, now)
	if decision.Outcome != DecisionOutcomeAsk {
		s.recordLocked(&decision)
		return decision
	}

	decision.Outcome = DecisionOutcomePending
	decision.Source = DecisionSourceMode
	decision.Reason = "awaiting user permission"
	decision.ExpiresAt = request.ExpiresAt

	existing, duplicate := s.pending.Add(request, decision, now)
	if duplicate {
		return existing
	}
	s.recordLocked(&decision)
	return decision
}

func (s *Service) ListPending() []PermissionRequest {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	s.expirePendingLocked(now)
	return cloneRequests(s.pending.List(now))
}

func (s *Service) Resolve(requestID string, resolution Resolution) (PermissionDecision, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	s.expirePendingLocked(now)
	s.pruneRulesLocked(now)

	outcome := normalizeResolveOutcome(resolution.Outcome)
	if outcome != DecisionOutcomeAllow && outcome != DecisionOutcomeDeny {
		return PermissionDecision{}, false
	}

	request, ok := s.pending.Pop(strings.TrimSpace(requestID))
	if !ok {
		return PermissionDecision{}, false
	}
	if request.IsExpired(now) {
		s.recordExpirationLocked(request, now)
		return PermissionDecision{}, false
	}

	persistence := normalizePersistence(resolution.Remember)
	scope := normalizeScope(resolution.Scope)
	decision := s.newDecisionLocked(request, outcome, DecisionSourceUser, now)
	decision.Persistence = persistence
	decision.Scope = scope
	decision.Reason = strings.TrimSpace(resolution.Reason)
	if decision.Reason == "" {
		decision.Reason = "resolved by user"
	}

	if persistence == DecisionPersistenceAlways {
		rule := s.ruleFromResolutionLocked(request, resolution, outcome, scope, decision.ID, now)
		rule = s.rememberRuleLocked(rule, now)
		decision.RuleID = rule.ID
	}

	s.recordLocked(&decision)
	return decision, true
}

func (s *Service) RememberRule(rule Rule) Rule {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	s.pruneRulesLocked(now)
	return s.rememberRuleLocked(rule, now)
}

func (s *Service) ApplyRules(request PermissionRequest) (PermissionDecision, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	s.pruneRulesLocked(now)
	request = s.normalizeRequestLocked(request, now)
	return s.applyRulesLocked(request, now)
}

func (s *Service) EvaluateMode(request PermissionRequest) PermissionDecision {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	request = s.normalizeRequestLocked(request, now)
	return s.evaluateModeLocked(request, now)
}

func (s *Service) History() []PermissionDecision {
	s.mu.Lock()
	defer s.mu.Unlock()

	history := make([]PermissionDecision, len(s.history))
	copy(history, s.history)
	return history
}

func (s *Service) applyRulesLocked(request PermissionRequest, now time.Time) (PermissionDecision, bool) {
	for _, rule := range s.rules {
		if rule.Behavior != DecisionOutcomeDeny || !rule.Matches(request, now) {
			continue
		}
		decision := s.newDecisionLocked(request, DecisionOutcomeDeny, DecisionSourceRule, now)
		decision.Scope = rule.Scope
		decision.RuleID = rule.ID
		decision.Reason = ruleReason(rule, "denied by remembered rule")
		return decision, true
	}
	for _, rule := range s.rules {
		if rule.Behavior != DecisionOutcomeAllow || !rule.Matches(request, now) {
			continue
		}
		decision := s.newDecisionLocked(request, DecisionOutcomeAllow, DecisionSourceRule, now)
		decision.Scope = rule.Scope
		decision.RuleID = rule.ID
		decision.Reason = ruleReason(rule, "allowed by remembered rule")
		return decision, true
	}
	return PermissionDecision{}, false
}

func (s *Service) evaluateModeLocked(request PermissionRequest, now time.Time) PermissionDecision {
	switch request.Mode {
	case PermissionModeBypass:
		decision := s.newDecisionLocked(request, DecisionOutcomeAllow, DecisionSourceMode, now)
		decision.Reason = "bypass mode allows the request"
		return decision
	case PermissionModeDontAsk:
		decision := s.newDecisionLocked(request, DecisionOutcomeDeny, DecisionSourceMode, now)
		decision.Reason = "dont_ask mode denies requests that need permission"
		return decision
	case PermissionModeAuto:
		if riskAtMost(request.RiskLevel, RiskLevelMedium) {
			decision := s.newDecisionLocked(request, DecisionOutcomeAllow, DecisionSourceMode, now)
			decision.Reason = "auto mode allows low and medium risk requests"
			return decision
		}
		decision := s.newDecisionLocked(request, DecisionOutcomeDeny, DecisionSourceMode, now)
		decision.Reason = "auto mode blocks high risk requests"
		return decision
	case PermissionModeAcceptEdits:
		if isAcceptEditsAction(request.Action) && riskAtMost(request.RiskLevel, RiskLevelMedium) {
			decision := s.newDecisionLocked(request, DecisionOutcomeAllow, DecisionSourceMode, now)
			decision.Reason = "accept_edits mode allows low and medium risk edits"
			return decision
		}
		decision := s.newDecisionLocked(request, DecisionOutcomeAsk, DecisionSourceMode, now)
		decision.Reason = "accept_edits mode asks for non-edit or high risk requests"
		return decision
	case PermissionModePlan:
		decision := s.newDecisionLocked(request, DecisionOutcomeAsk, DecisionSourceMode, now)
		decision.Reason = "plan mode requires approval before execution"
		return decision
	default:
		decision := s.newDecisionLocked(request, DecisionOutcomeAsk, DecisionSourceMode, now)
		decision.Reason = "ask mode requires user permission"
		return decision
	}
}

func (s *Service) normalizeRequestLocked(request PermissionRequest, now time.Time) PermissionRequest {
	request.ID = strings.TrimSpace(request.ID)
	if request.ID == "" {
		request.ID = s.nextIDLocked("req")
	}
	request.SessionID = strings.TrimSpace(request.SessionID)
	request.ProjectID = strings.TrimSpace(request.ProjectID)
	if strings.TrimSpace(string(request.Mode)) == "" {
		request.Mode = s.mode
	} else {
		request.Mode = normalizeMode(request.Mode)
	}
	request.Tool = strings.TrimSpace(request.Tool)
	request.Action = strings.ToLower(strings.TrimSpace(request.Action))
	request.Path = strings.TrimSpace(request.Path)
	request.Description = strings.TrimSpace(request.Description)
	request.RiskLevel = normalizeRiskLevel(request.RiskLevel)
	request.Agent = request.Agent.Normalize()
	request.Metadata = copyMetadata(request.Metadata)
	if request.CreatedAt.IsZero() {
		request.CreatedAt = now
	}
	if request.ExpiresAt.IsZero() {
		ttl := request.TTL
		if ttl == 0 {
			ttl = s.defaultTTL
		}
		if ttl > 0 {
			request.ExpiresAt = request.CreatedAt.Add(ttl)
		}
	}
	return request
}

func (s *Service) rememberRuleLocked(rule Rule, now time.Time) Rule {
	rule.ID = strings.TrimSpace(rule.ID)
	if rule.ID == "" {
		rule.ID = s.nextIDLocked("rule")
	}
	rule.Scope = normalizeScope(rule.Scope)
	rule.Behavior = normalizeRuleBehavior(rule.Behavior)
	rule.SessionID = strings.TrimSpace(rule.SessionID)
	rule.ProjectID = strings.TrimSpace(rule.ProjectID)
	rule.Key = rule.Key.Normalize()
	rule.Agent = rule.Agent.Normalize()
	rule.RiskLevel = normalizeRiskLevel(rule.RiskLevel)
	rule.Reason = strings.TrimSpace(rule.Reason)
	rule.Metadata = copyMetadata(rule.Metadata)
	if rule.CreatedAt.IsZero() {
		rule.CreatedAt = now
	}
	if rule.ExpiresAt.IsZero() && rule.TTL > 0 {
		rule.ExpiresAt = rule.CreatedAt.Add(rule.TTL)
	}

	for index, existing := range s.rules {
		if sameRuleIdentity(existing, rule) {
			s.rules[index] = rule
			return rule
		}
	}
	s.rules = append(s.rules, rule)
	return rule
}

func (s *Service) ruleFromResolutionLocked(request PermissionRequest, resolution Resolution, outcome DecisionOutcome, scope RuleScope, decisionID string, now time.Time) Rule {
	return Rule{
		Scope:     scope,
		Behavior:  outcome,
		SessionID: request.SessionID,
		ProjectID: request.ProjectID,
		Key:       request.Key(),
		Agent:     request.Agent,
		RiskLevel: request.RiskLevel,
		TTL:       resolution.RuleTTL,
		CreatedAt: now,
		ExpiresAt: resolution.ExpiresAt,
		Reason:    fmt.Sprintf("remembered from %s", decisionID),
	}
}

func (s *Service) newDecisionLocked(request PermissionRequest, outcome DecisionOutcome, source DecisionSource, now time.Time) PermissionDecision {
	return PermissionDecision{
		ID:          s.nextIDLocked("decision"),
		RequestID:   request.ID,
		Outcome:     outcome,
		Persistence: DecisionPersistenceOnce,
		Source:      source,
		Mode:        request.Mode,
		Key:         request.Key(),
		Agent:       request.Agent,
		RiskLevel:   request.RiskLevel,
		CreatedAt:   now,
	}
}

func (s *Service) expirePendingLocked(now time.Time) {
	expired := s.pending.PruneExpired(now)
	for _, request := range expired {
		s.recordExpirationLocked(request, now)
	}
}

func (s *Service) recordExpirationLocked(request PermissionRequest, now time.Time) {
	decision := s.newDecisionLocked(request, DecisionOutcomeDeny, DecisionSourceExpiration, now)
	decision.Reason = "permission request expired"
	decision.ExpiresAt = request.ExpiresAt
	s.recordLocked(&decision)
}

func (s *Service) pruneRulesLocked(now time.Time) {
	if len(s.rules) == 0 {
		return
	}
	active := s.rules[:0]
	for _, rule := range s.rules {
		if !rule.IsExpired(now) {
			active = append(active, rule)
		}
	}
	s.rules = active
}

func (s *Service) recordLocked(decision *PermissionDecision) {
	s.sequence++
	decision.Sequence = s.sequence
	s.history = append(s.history, *decision)
}

func (s *Service) nextIDLocked(prefix string) string {
	s.idSequence++
	return fmt.Sprintf("%s-%06d", prefix, s.idSequence)
}

func (s *Service) now() time.Time {
	if s.clock == nil {
		return time.Now()
	}
	return s.clock()
}

func sameRuleIdentity(left Rule, right Rule) bool {
	left = comparableRule(left)
	right = comparableRule(right)
	return left.Scope == right.Scope &&
		left.Behavior == right.Behavior &&
		left.SessionID == right.SessionID &&
		left.ProjectID == right.ProjectID &&
		left.Key == right.Key &&
		left.Agent.ID == right.Agent.ID &&
		left.Agent.Type == right.Agent.Type
}

func comparableRule(rule Rule) Rule {
	rule.Scope = normalizeScope(rule.Scope)
	rule.Behavior = normalizeRuleBehavior(rule.Behavior)
	rule.Key = rule.Key.Normalize()
	rule.Agent = rule.Agent.Normalize()
	rule.SessionID = strings.TrimSpace(rule.SessionID)
	rule.ProjectID = strings.TrimSpace(rule.ProjectID)
	return rule
}

func ruleReason(rule Rule, fallback string) string {
	if rule.Reason != "" {
		return rule.Reason
	}
	return fallback
}

func normalizeResolveOutcome(outcome DecisionOutcome) DecisionOutcome {
	switch DecisionOutcome(strings.ToLower(strings.TrimSpace(string(outcome)))) {
	case DecisionOutcomeAllow:
		return DecisionOutcomeAllow
	case DecisionOutcomeDeny:
		return DecisionOutcomeDeny
	default:
		return ""
	}
}

func isAcceptEditsAction(action string) bool {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "edit", "write", "create", "delete", "patch", "apply_patch":
		return true
	default:
		return false
	}
}

func cloneRequests(requests []PermissionRequest) []PermissionRequest {
	cloned := make([]PermissionRequest, len(requests))
	for index, request := range requests {
		request.Metadata = copyMetadata(request.Metadata)
		cloned[index] = request
	}
	return cloned
}
