package permissionproduct

import (
	"testing"
	"time"
)

type fakeClock struct {
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	return c.now
}

func (c *fakeClock) Advance(duration time.Duration) {
	c.now = c.now.Add(duration)
}

func newTestService() (*Service, *fakeClock) {
	clock := &fakeClock{now: time.Date(2026, 6, 28, 10, 0, 0, 0, time.UTC)}
	return NewService(WithClock(clock.Now), WithDefaultTTL(30*time.Minute)), clock
}

func baseRequest() PermissionRequest {
	return PermissionRequest{
		ID:        "request-1",
		SessionID: "session-1",
		ProjectID: "project-1",
		Mode:      PermissionModeAsk,
		Tool:      "shell",
		Action:    "execute",
		Path:      "npm test",
		RiskLevel: RiskLevelHigh,
		Agent: AgentAttribution{
			ID:   "agent-1",
			Type: "builder",
			Name: "Builder",
		},
	}
}

func TestSubmitRequestAddsPendingQueue(t *testing.T) {
	service, _ := newTestService()

	decision := service.SubmitRequest(baseRequest())
	if decision.Outcome != DecisionOutcomePending {
		t.Fatalf("expected pending decision, got %q", decision.Outcome)
	}
	if decision.Agent.ID != "agent-1" {
		t.Fatalf("expected agent attribution to be preserved")
	}

	pending := service.ListPending()
	if len(pending) != 1 {
		t.Fatalf("expected one pending request, got %d", len(pending))
	}
	if pending[0].ID != "request-1" {
		t.Fatalf("unexpected pending request id: %q", pending[0].ID)
	}
}

func TestResolveOnceDoesNotRememberRule(t *testing.T) {
	service, _ := newTestService()
	request := baseRequest()

	service.SubmitRequest(request)
	decision, ok := service.Resolve(request.ID, Resolution{
		Outcome:  DecisionOutcomeAllow,
		Remember: DecisionPersistenceOnce,
	})
	if !ok {
		t.Fatalf("expected resolve to succeed")
	}
	if decision.Outcome != DecisionOutcomeAllow || decision.Persistence != DecisionPersistenceOnce {
		t.Fatalf("unexpected resolution: %#v", decision)
	}
	if len(service.ListPending()) != 0 {
		t.Fatalf("expected pending queue to be empty after resolve")
	}
	if _, ok := service.ApplyRules(request); ok {
		t.Fatalf("did not expect once resolution to create a remembered rule")
	}

	next := request
	next.ID = "request-2"
	nextDecision := service.SubmitRequest(next)
	if nextDecision.Outcome != DecisionOutcomePending {
		t.Fatalf("expected same request to ask again after once resolution, got %q", nextDecision.Outcome)
	}
}

func TestRememberAlwaysAllowAndDeny(t *testing.T) {
	service, _ := newTestService()
	request := baseRequest()

	service.SubmitRequest(request)
	allowDecision, ok := service.Resolve(request.ID, Resolution{
		Outcome:  DecisionOutcomeAllow,
		Remember: DecisionPersistenceAlways,
		Scope:    RuleScopeSession,
	})
	if !ok {
		t.Fatalf("expected allow resolution to succeed")
	}
	if allowDecision.RuleID == "" {
		t.Fatalf("expected always allow to create a rule")
	}

	allowed := request
	allowed.ID = "request-2"
	allowedDecision := service.SubmitRequest(allowed)
	if allowedDecision.Outcome != DecisionOutcomeAllow || allowedDecision.Source != DecisionSourceRule {
		t.Fatalf("expected remembered allow rule, got %#v", allowedDecision)
	}

	denyRequest := request
	denyRequest.ID = "request-3"
	denyRequest.Path = "rm -rf dist"
	service.SubmitRequest(denyRequest)
	denyDecision, ok := service.Resolve(denyRequest.ID, Resolution{
		Outcome:  DecisionOutcomeDeny,
		Remember: DecisionPersistenceAlways,
		Scope:    RuleScopeSession,
	})
	if !ok {
		t.Fatalf("expected deny resolution to succeed")
	}
	if denyDecision.RuleID == "" {
		t.Fatalf("expected always deny to create a rule")
	}

	denied := denyRequest
	denied.ID = "request-4"
	deniedDecision := service.SubmitRequest(denied)
	if deniedDecision.Outcome != DecisionOutcomeDeny || deniedDecision.Source != DecisionSourceRule {
		t.Fatalf("expected remembered deny rule, got %#v", deniedDecision)
	}
}

func TestEvaluateMode(t *testing.T) {
	service, _ := newTestService()
	request := baseRequest()

	request.Mode = PermissionModeBypass
	request.RiskLevel = RiskLevelCritical
	if decision := service.EvaluateMode(request); decision.Outcome != DecisionOutcomeAllow {
		t.Fatalf("expected bypass to allow, got %q", decision.Outcome)
	}

	request.Mode = PermissionModeDontAsk
	if decision := service.EvaluateMode(request); decision.Outcome != DecisionOutcomeDeny {
		t.Fatalf("expected dont_ask to deny, got %q", decision.Outcome)
	}

	request.Mode = PermissionModeAuto
	request.RiskLevel = RiskLevelLow
	if decision := service.EvaluateMode(request); decision.Outcome != DecisionOutcomeAllow {
		t.Fatalf("expected auto low-risk request to allow, got %q", decision.Outcome)
	}
	request.RiskLevel = RiskLevelHigh
	if decision := service.EvaluateMode(request); decision.Outcome != DecisionOutcomeDeny {
		t.Fatalf("expected auto high-risk request to deny, got %q", decision.Outcome)
	}

	request.Mode = PermissionModeAcceptEdits
	request.Action = "edit"
	request.RiskLevel = RiskLevelMedium
	if decision := service.EvaluateMode(request); decision.Outcome != DecisionOutcomeAllow {
		t.Fatalf("expected accept_edits edit to allow, got %q", decision.Outcome)
	}
	request.Action = "execute"
	if decision := service.EvaluateMode(request); decision.Outcome != DecisionOutcomeAsk {
		t.Fatalf("expected accept_edits execute to ask, got %q", decision.Outcome)
	}

	request.Mode = PermissionModePlan
	if decision := service.EvaluateMode(request); decision.Outcome != DecisionOutcomeAsk {
		t.Fatalf("expected plan mode to ask, got %q", decision.Outcome)
	}
}

func TestAgentScopedRule(t *testing.T) {
	service, _ := newTestService()
	request := baseRequest()
	request.Action = "edit"
	request.Path = "src/app.go"
	request.RiskLevel = RiskLevelMedium

	service.RememberRule(Rule{
		Scope:     RuleScopeAgent,
		Behavior:  DecisionOutcomeAllow,
		Key:       request.Key(),
		Agent:     AgentAttribution{ID: "agent-1"},
		RiskLevel: RiskLevelMedium,
	})

	agentDecision := service.SubmitRequest(request)
	if agentDecision.Outcome != DecisionOutcomeAllow {
		t.Fatalf("expected matching agent to use rule, got %q", agentDecision.Outcome)
	}

	otherAgent := request
	otherAgent.ID = "request-2"
	otherAgent.Agent.ID = "agent-2"
	otherDecision := service.SubmitRequest(otherAgent)
	if otherDecision.Outcome != DecisionOutcomePending {
		t.Fatalf("expected other agent to need approval, got %q", otherDecision.Outcome)
	}
}

func TestExpiration(t *testing.T) {
	service, clock := newTestService()
	request := baseRequest()
	request.TTL = time.Minute

	service.SubmitRequest(request)
	clock.Advance(2 * time.Minute)

	if pending := service.ListPending(); len(pending) != 0 {
		t.Fatalf("expected pending request to expire, got %d pending", len(pending))
	}
	history := service.History()
	last := history[len(history)-1]
	if last.Source != DecisionSourceExpiration || last.Outcome != DecisionOutcomeDeny {
		t.Fatalf("expected expiration denial, got %#v", last)
	}

	expiringRule := Rule{
		Scope:     RuleScopeSession,
		Behavior:  DecisionOutcomeAllow,
		SessionID: request.SessionID,
		Key:       request.Key(),
		RiskLevel: request.RiskLevel,
		TTL:       time.Minute,
	}
	service.RememberRule(expiringRule)
	beforeExpiry := request
	beforeExpiry.ID = "request-2"
	if decision := service.SubmitRequest(beforeExpiry); decision.Outcome != DecisionOutcomeAllow {
		t.Fatalf("expected unexpired rule to allow, got %q", decision.Outcome)
	}

	clock.Advance(2 * time.Minute)
	afterExpiry := request
	afterExpiry.ID = "request-3"
	if decision := service.SubmitRequest(afterExpiry); decision.Outcome != DecisionOutcomePending {
		t.Fatalf("expected expired rule to stop matching, got %q", decision.Outcome)
	}
}

func TestHistoryOrdering(t *testing.T) {
	service, clock := newTestService()
	request := baseRequest()

	pending := service.SubmitRequest(request)
	clock.Advance(time.Minute)
	resolved, ok := service.Resolve(request.ID, Resolution{Outcome: DecisionOutcomeAllow})
	if !ok {
		t.Fatalf("expected resolve to succeed")
	}
	clock.Advance(time.Minute)
	autoRequest := request
	autoRequest.ID = "request-2"
	autoRequest.Mode = PermissionModeAuto
	autoRequest.RiskLevel = RiskLevelLow
	auto := service.SubmitRequest(autoRequest)

	history := service.History()
	if len(history) != 3 {
		t.Fatalf("expected three history entries, got %d", len(history))
	}
	if history[0].ID != pending.ID || history[1].ID != resolved.ID || history[2].ID != auto.ID {
		t.Fatalf("history order did not follow decision order: %#v", history)
	}
	if !(history[0].Sequence < history[1].Sequence && history[1].Sequence < history[2].Sequence) {
		t.Fatalf("expected increasing history sequence: %#v", history)
	}
	if !(history[0].CreatedAt.Before(history[1].CreatedAt) && history[1].CreatedAt.Before(history[2].CreatedAt)) {
		t.Fatalf("expected increasing decision timestamps: %#v", history)
	}
}

func TestDuplicateRequestReturnsExistingPending(t *testing.T) {
	service, _ := newTestService()
	request := baseRequest()

	first := service.SubmitRequest(request)
	duplicate := request
	duplicate.ID = "different-request-id"
	second := service.SubmitRequest(duplicate)

	if first.ID != second.ID {
		t.Fatalf("expected duplicate to return same pending decision id, got %q and %q", first.ID, second.ID)
	}
	if first.RequestID != second.RequestID {
		t.Fatalf("expected duplicate to keep original request id, got %q and %q", first.RequestID, second.RequestID)
	}
	if pending := service.ListPending(); len(pending) != 1 {
		t.Fatalf("expected one pending request after duplicate submit, got %d", len(pending))
	}
	if history := service.History(); len(history) != 1 {
		t.Fatalf("expected duplicate submit not to append history, got %d entries", len(history))
	}
}
