package researchruntime

import (
	"context"
	"strings"
	"sync"
	"testing"
)

type stubChildren struct {
	mu        sync.Mutex
	responses []string
	errs      []error
	calls     []ChildSpec
}

func (s *stubChildren) Run(_ context.Context, spec ChildSpec) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, spec)
	var err error
	if len(s.errs) > 0 {
		err = s.errs[0]
		s.errs = s.errs[1:]
	}
	if len(s.responses) == 0 {
		return "", err
	}
	next := s.responses[0]
	s.responses = s.responses[1:]
	return next, err
}

type memJournal struct{ kinds []string }

func (m *memJournal) Append(_ context.Context, kind string, _ map[string]any) error {
	m.kinds = append(m.kinds, kind)
	return nil
}

type memPublisher struct{ markdown string }

func (m *memPublisher) Publish(_ context.Context, markdown string) (string, string, error) {
	m.markdown = markdown
	return "output/research-run.md", "art_1", nil
}

type stubSynth struct{ raw string }

func (s *stubSynth) Synthesize(_ context.Context, _ string) (string, error) {
	return s.raw, nil
}

func TestParseIntentRequiresQuery(t *testing.T) {
	tokens := []ComposerToken{{Type: "workflow", Payload: WorkflowPayload{WorkflowID: WorkflowID}}}
	if _, err := ParseIntent(tokens, "   "); err == nil {
		t.Fatal("expected query required")
	}
	intent, err := ParseIntent(tokens, "本仓 WebSearch 渠道是什么")
	if err != nil || intent.Query == "" || intent.Breadth != 4 {
		t.Fatalf("%+v %v", intent, err)
	}
	if intent, err := ParseIntent(nil, "x"); err != nil || intent != nil {
		t.Fatalf("no token should yield nil intent")
	}
}

func TestOrchestratorProducesCitedPartialReport(t *testing.T) {
	children := &stubChildren{responses: []string{
		`{"questions":[{"id":"q1","text":"What search providers exist?"}]}`,
		`{"claims":[{"id":"claim-0","question_id":"q1","statement":"WebSearch uses Exa by default.","confidence":"high","evidence":[{"kind":"workspace","title":"tools doc","locator":"docs/04_工具权限与审批/工具定义与暴露.md:123","source_type":"primary"}]}],"uncertainties":[]}`,
		`{"verdicts":[{"claim_id":"claim-0","verdict":"partial","reason":"single workspace source"}]}`,
	}}
	publisher := &memPublisher{}
	journal := &memJournal{}
	orch := &Orchestrator{
		Children: children, PublishFn: publisher, Journal: journal,
		Synth: &stubSynth{raw: `{"answer":"Exa is the default WebSearch channel.","claim_ids":["claim-0"],"limitations":["needs live web confirmation"]}`},
	}
	result, err := orch.Run(context.Background(), Intent{Query: "本仓 WebSearch 渠道是什么", Breadth: 2, SourceHint: SourceWorkspace})
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != OutcomePartial {
		t.Fatalf("outcome=%s", result.Outcome)
	}
	if !strings.Contains(result.Markdown, "[S1]") || !strings.Contains(result.Markdown, "工具定义与暴露.md") {
		t.Fatalf("markdown=%s", result.Markdown)
	}
	if len(children.calls) != 3 {
		t.Fatalf("report must not spawn a child, calls=%d", len(children.calls))
	}
	if children.calls[0].Host == nil || containsString(children.calls[0].Host.ToolAllowlist, "WebSearch") {
		t.Fatalf("workspace hint must omit web tools: %+v", children.calls[0].Host)
	}
	if strings.Contains(result.Markdown, "[S99]") {
		t.Fatal("invented citation")
	}
	if publisher.markdown == "" || result.ArtifactID != "art_1" {
		t.Fatal("expected publish")
	}
	if len(journal.kinds) == 0 || journal.kinds[0] != "research_accepted" {
		t.Fatalf("journal=%v", journal.kinds)
	}
}

func TestVerifierRunErrorKeepsShardPartial(t *testing.T) {
	children := &stubChildren{
		responses: []string{
			`{"questions":[{"id":"q1","text":"q"}]}`,
			`{"claims":[{"id":"claim-0","question_id":"q1","statement":"WebSearch uses Exa.","confidence":"high","evidence":[{"kind":"workspace","title":"web_search.go","locator":"src/tools/web_search/web_search.go:22","source_type":"primary"}]}],"uncertainties":[]}`,
		},
		errs: []error{nil, nil, context.Canceled},
	}
	publisher := &memPublisher{}
	orch := &Orchestrator{Children: children, PublishFn: publisher}
	result, err := orch.Run(context.Background(), Intent{Query: "q", Breadth: 2, SourceHint: SourceWorkspace})
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != OutcomePartial {
		t.Fatalf("outcome=%s coverage=%v", result.Outcome, result.Coverage)
	}
	if len(result.Claims) == 0 || publisher.markdown == "" {
		t.Fatal("verifier LLM failure must still publish a partial report")
	}
	if !strings.Contains(result.Markdown, "web_search.go") {
		t.Fatalf("markdown=%s", result.Markdown)
	}
}

func TestEmptyClaimsStillPublishesPartial(t *testing.T) {
	children := &stubChildren{responses: []string{
		`{"questions":[{"id":"q1","text":"q"}]}`,
		`{"claims":[],"uncertainties":["nothing found"]}`,
	}}
	publisher := &memPublisher{}
	orch := &Orchestrator{Children: children, PublishFn: publisher}
	result, err := orch.Run(context.Background(), Intent{Query: "q", Breadth: 2, SourceHint: SourceWorkspace})
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != OutcomePartial {
		t.Fatalf("outcome=%s coverage=%v", result.Outcome, result.Coverage)
	}
	if publisher.markdown == "" || !strings.Contains(result.Markdown, "No verified claims were produced.") {
		t.Fatalf("markdown=%s", result.Markdown)
	}
}

func TestVerifierIDMismatchKeepsShardPartial(t *testing.T) {
	children := &stubChildren{responses: []string{
		`{"questions":[{"id":"q1","text":"q"}]}`,
		`{"claims":[{"id":"claim-0","question_id":"q1","statement":"A","confidence":"low","evidence":[{"kind":"web","title":"t","locator":"https://example.com","source_type":"secondary"}]}],"uncertainties":[]}`,
		`{"verdicts":[{"claim_id":"claim-other","verdict":"verified","reason":"bad"}]}`,
	}}
	orch := &Orchestrator{Children: children}
	result, err := orch.Run(context.Background(), Intent{Query: "q", Breadth: 2})
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != OutcomePartial {
		t.Fatalf("outcome=%s coverage=%v", result.Outcome, result.Coverage)
	}
	if len(result.Claims) == 0 {
		t.Fatal("mismatch must keep claims as partial, not drop the report")
	}
}

func TestStopProducesNoPublish(t *testing.T) {
	cancelled := false
	children := &stubChildren{responses: []string{
		`{"questions":[{"id":"q1","text":"q"}]}`,
	}}
	orch := &Orchestrator{Children: children, Cancelled: func() bool { return cancelled }, PublishFn: &memPublisher{}}
	cancelled = true
	result, err := orch.Run(context.Background(), Intent{Query: "q"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != OutcomeCancelled || result.ArtifactID != "" {
		t.Fatalf("%+v", result)
	}
}

func TestWebHintFailsWhenSearchDisabled(t *testing.T) {
	t.Setenv("AWAKEN_WEB_SEARCH_PROVIDERS", "")
	publisher := &memPublisher{}
	orch := &Orchestrator{Children: &stubChildren{}, PublishFn: publisher}
	result, err := orch.Run(context.Background(), Intent{Query: "Tavily keyless?", SourceHint: SourceWeb})
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != OutcomeFailed {
		t.Fatalf("outcome=%s coverage=%v", result.Outcome, result.Coverage)
	}
	if publisher.markdown != "" {
		t.Fatal("must not publish empty report")
	}
}

func TestHostDowngradesVerifiedWithoutIndependentSources(t *testing.T) {
	claims := applyHostVerificationRules([]Claim{{
		ID: "claim-0", VerificationStatus: "verified", EvidenceIDs: []string{"ev-1"},
	}}, []Evidence{{
		ID: "ev-1", Kind: "web", SourceType: "secondary", IndependenceKey: "web:example.com",
	}})
	if claims[0].VerificationStatus != "partial" {
		t.Fatalf("status=%s", claims[0].VerificationStatus)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
