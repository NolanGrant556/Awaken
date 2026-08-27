package researchruntime

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"awaken/src/subagentruntime"
)

type Phase string

const (
	PhaseAccepted    Phase = "accepted"
	PhasePlanning    Phase = "planning"
	PhaseResearching Phase = "researching"
	PhaseVerifying   Phase = "verifying"
	PhaseReporting   Phase = "reporting"
	PhaseTerminal    Phase = "terminal"
)

type TerminalOutcome string

const (
	OutcomeVerified    TerminalOutcome = "verified"
	OutcomePartial     TerminalOutcome = "partial"
	OutcomeFailed      TerminalOutcome = "failed"
	OutcomeCancelled   TerminalOutcome = "cancelled"
	OutcomeInterrupted TerminalOutcome = "interrupted"
)

type ChildSpec struct {
	Description string
	Prompt      string
	AgentType   string
	Host        *subagentruntime.HostSpawnOptions
}

type ChildClient interface {
	Run(ctx context.Context, spec ChildSpec) (string, error)
}

type PlanWriter interface {
	Apply(ctx context.Context, items []PlanItem) error
}

type PlanItem struct {
	Step   string
	Status string
}

type Publisher interface {
	Publish(ctx context.Context, markdown string) (path string, artifactID string, err error)
}

type Synthesizer interface {
	Synthesize(ctx context.Context, prompt string) (string, error)
}

type Journal interface {
	Append(ctx context.Context, kind string, payload map[string]any) error
}

type Orchestrator struct {
	Children  ChildClient
	Plan      PlanWriter
	PublishFn Publisher
	Synth     Synthesizer
	Journal   Journal
	Cancelled func() bool
}

type Result struct {
	Outcome    TerminalOutcome
	Answer     string
	Markdown   string
	ReportPath string
	ArtifactID string
	Coverage   []string
	Claims     []Claim
}

func (o *Orchestrator) Run(ctx context.Context, intent Intent) (Result, error) {
	intent.Breadth = NormalizeBreadth(intent.Breadth)
	intent.SourceHint = NormalizeSourceHint(string(intent.SourceHint))
	if o.Cancelled != nil && o.Cancelled() {
		return Result{Outcome: OutcomeCancelled}, nil
	}
	if intent.SourceHint == SourceWeb && !SearchChannelsEnabled() {
		coverage := []string{"search channels are disabled; enable Settings > Search"}
		_ = o.append(ctx, "research_accepted", map[string]any{"query": intent.Query, "breadth": intent.Breadth})
		_ = o.append(ctx, "research_terminal", map[string]any{"outcome": string(OutcomeFailed)})
		return Result{Outcome: OutcomeFailed, Coverage: coverage}, nil
	}
	if err := o.append(ctx, "research_accepted", map[string]any{"query": intent.Query, "breadth": intent.Breadth}); err != nil {
		return Result{}, err
	}
	if err := o.setPlan(ctx, "in_progress", "pending", "pending", "pending"); err != nil {
		return Result{}, err
	}

	questions, coverage := o.plan(ctx, intent)
	if intent.SourceHint == SourceBoth && !SearchChannelsEnabled() {
		coverage = append(coverage, "search channels unavailable; continuing workspace-only")
	}
	if o.stopped(ctx) {
		return Result{Outcome: OutcomeCancelled, Coverage: coverage}, nil
	}
	if err := o.setPlan(ctx, "completed", "in_progress", "pending", "pending"); err != nil {
		return Result{}, err
	}

	claims, evidence, coverage := o.research(ctx, intent, questions, coverage)
	if o.stopped(ctx) {
		return Result{Outcome: OutcomeCancelled, Coverage: coverage, Claims: claims}, nil
	}
	if err := o.setPlan(ctx, "completed", "completed", "in_progress", "pending"); err != nil {
		return Result{}, err
	}

	claims, coverage = o.verify(ctx, intent, claims, evidence, coverage)
	if o.stopped(ctx) {
		return Result{Outcome: OutcomeCancelled, Coverage: coverage, Claims: claims}, nil
	}
	if err := o.setPlan(ctx, "completed", "completed", "completed", "in_progress"); err != nil {
		return Result{}, err
	}

	result := o.report(ctx, intent, claims, evidence, coverage)
	if result.Outcome == OutcomeCancelled || o.stopped(ctx) {
		return Result{Outcome: OutcomeCancelled, Coverage: uniqueStrings(append(coverage, result.Coverage...)), Claims: result.Claims}, nil
	}
	if o.PublishFn != nil && strings.TrimSpace(result.Markdown) != "" && result.Outcome != OutcomeFailed {
		path, artifactID, err := o.PublishFn.Publish(ctx, result.Markdown)
		if err != nil {
			result.Outcome = OutcomeFailed
			result.Coverage = append(result.Coverage, "report publish failed")
			return result, err
		}
		result.ReportPath = path
		result.ArtifactID = artifactID
	}
	_ = o.setPlan(ctx, "completed", "completed", "completed", "completed")
	_ = o.append(ctx, "research_terminal", map[string]any{"outcome": string(result.Outcome)})
	return result, nil
}

func (o *Orchestrator) plan(ctx context.Context, intent Intent) ([]Question, []string) {
	raw, err := o.Children.Run(ctx, ChildSpec{
		Description: "研究规划", Prompt: plannerPrompt(intent), AgentType: subagentruntime.AgentTypePlan,
		Host: spawnOptions(intent),
	})
	coverage := []string{}
	if err != nil {
		coverage = append(coverage, "planner failed; using the original query")
		return []Question{{ID: "q1", Text: intent.Query}}, coverage
	}
	var parsed PlannerOutput
	if err := parseJSON(raw, &parsed); err != nil || len(parsed.Questions) == 0 {
		coverage = append(coverage, "planner output was invalid; using the original query")
		return []Question{{ID: "q1", Text: intent.Query}}, coverage
	}
	if len(parsed.Questions) > intent.Breadth {
		parsed.Questions = parsed.Questions[:intent.Breadth]
	}
	for i := range parsed.Questions {
		if parsed.Questions[i].ID == "" {
			parsed.Questions[i].ID = fmt.Sprintf("q%d", i+1)
		}
	}
	_ = o.append(ctx, "research_plan", map[string]any{"question_count": len(parsed.Questions)})
	return parsed.Questions, coverage
}

func (o *Orchestrator) research(ctx context.Context, intent Intent, questions []Question, coverage []string) ([]Claim, []Evidence, []string) {
	type waveResult struct {
		parsed ResearcherOutput
		err    error
		failed string
	}
	results := make([]waveResult, len(questions))
	var wg sync.WaitGroup
	for i, question := range questions {
		if o.stopped(ctx) {
			break
		}
		wg.Add(1)
		go func(index int, question Question) {
			defer wg.Done()
			raw, err := o.Children.Run(ctx, ChildSpec{
				Description: "调研 " + question.ID, Prompt: researcherPrompt(intent, question), AgentType: subagentruntime.AgentTypeExplore,
				Host: spawnOptions(intent),
			})
			if err != nil {
				results[index] = waveResult{failed: "question " + question.ID + " failed"}
				return
			}
			var parsed ResearcherOutput
			if err := parseJSON(raw, &parsed); err != nil {
				results[index] = waveResult{failed: "question " + question.ID + " returned invalid claims"}
				return
			}
			results[index] = waveResult{parsed: parsed}
		}(i, question)
	}
	wg.Wait()

	var claims []Claim
	var evidence []Evidence
	evSeq := 0
	claimSeq := 0
	for i, question := range questions {
		item := results[i]
		if item.failed != "" {
			coverage = append(coverage, item.failed)
			continue
		}
		if item.err != nil {
			coverage = append(coverage, "question "+question.ID+" failed")
			continue
		}
		kept := 0
		for _, rawClaim := range item.parsed.Claims {
			if kept >= MaxClaimsPerQuestion || len(claims) >= MaxClaimsTotal {
				coverage = append(coverage, "claim cap reached")
				break
			}
			id := strings.TrimSpace(rawClaim.ID)
			if id == "" {
				id = fmt.Sprintf("claim-%d", claimSeq)
			}
			claimSeq++
			claim := Claim{ID: id, QuestionID: question.ID, Statement: strings.TrimSpace(rawClaim.Statement), Confidence: rawClaim.Confidence, VerificationStatus: "partial"}
			for _, rawEv := range rawClaim.Evidence {
				if len(evidence) >= MaxEvidenceTotal {
					break
				}
				evID := fmt.Sprintf("ev-%d", evSeq)
				evSeq++
				evidence = append(evidence, Evidence{
					ID: evID, Kind: rawEv.Kind, Title: rawEv.Title, Locator: rawEv.Locator,
					SourceType: rawEv.SourceType, IndependenceKey: independenceKey(rawEv), Digest: digestOf(rawEv.Locator),
				})
				claim.EvidenceIDs = append(claim.EvidenceIDs, evID)
			}
			if strings.TrimSpace(claim.Statement) == "" {
				continue
			}
			claims = append(claims, claim)
			kept++
		}
		coverage = append(coverage, item.parsed.Uncertainties...)
	}
	_ = o.append(ctx, "research_claim_batch", map[string]any{"claim_count": len(claims), "evidence_count": len(evidence)})
	return claims, evidence, coverage
}

func (o *Orchestrator) verify(ctx context.Context, intent Intent, claims []Claim, evidence []Evidence, coverage []string) ([]Claim, []string) {
	if len(claims) == 0 {
		return claims, coverage
	}
	byID := map[string][]Evidence{}
	for _, ev := range evidence {
		byID[ev.ID] = append(byID[ev.ID], ev)
	}
	shards := shardClaims(claims, 2)
	for _, shard := range shards {
		if o.stopped(ctx) {
			return claims, coverage
		}
		expected := map[string]bool{}
		prompt := ""
		for _, claim := range shard {
			expected[claim.ID] = true
			var evs []Evidence
			for _, id := range claim.EvidenceIDs {
				evs = append(evs, byID[id]...)
			}
			prompt += verifierPrompt(claim, flattenEvidence(evs)) + "\n"
		}
		raw, err := o.Children.Run(ctx, ChildSpec{
			Description: "核验", Prompt: prompt, AgentType: subagentruntime.AgentTypeReview,
			Host: spawnOptions(intent),
		})
		if err != nil {
			coverage = append(coverage, "verifier shard failed")
			for i := range claims {
				if expected[claims[i].ID] {
					claims[i].VerificationStatus = "rejected"
				}
			}
			continue
		}
		var parsed VerifierOutput
		if err := parseJSON(raw, &parsed); err != nil {
			coverage = append(coverage, "verifier returned invalid JSON")
			for i := range claims {
				if expected[claims[i].ID] {
					claims[i].VerificationStatus = "rejected"
				}
			}
			continue
		}
		seen := map[string]string{}
		valid := true
		for _, verdict := range parsed.Verdicts {
			if !expected[verdict.ClaimID] || seen[verdict.ClaimID] != "" {
				valid = false
				break
			}
			seen[verdict.ClaimID] = strings.ToLower(strings.TrimSpace(verdict.Verdict))
		}
		if !valid || len(seen) != len(expected) {
			coverage = append(coverage, "verifier claim-id mismatch; shard excluded")
			for i := range claims {
				if expected[claims[i].ID] {
					claims[i].VerificationStatus = "rejected"
				}
			}
			continue
		}
		for i := range claims {
			if status, ok := seen[claims[i].ID]; ok {
				switch status {
				case "verified", "partial", "rejected":
					claims[i].VerificationStatus = status
				default:
					claims[i].VerificationStatus = "rejected"
				}
			}
		}
	}
	claims = applyHostVerificationRules(claims, evidence)
	_ = o.append(ctx, "research_verification", map[string]any{"claim_count": len(claims)})
	return claims, coverage
}

func (o *Orchestrator) report(ctx context.Context, intent Intent, claims []Claim, evidence []Evidence, coverage []string) Result {
	if o.stopped(ctx) {
		return Result{Outcome: OutcomeCancelled, Coverage: coverage, Claims: claims}
	}
	usable := make([]Claim, 0, len(claims))
	verified := 0
	for _, claim := range claims {
		if claim.VerificationStatus == "rejected" || strings.TrimSpace(claim.Statement) == "" {
			continue
		}
		usable = append(usable, claim)
		if claim.VerificationStatus == "verified" {
			verified++
		}
	}
	if len(usable) == 0 {
		return Result{Outcome: OutcomeFailed, Coverage: append(coverage, "no usable claims"), Claims: claims}
	}
	answer := ""
	if o.Synth != nil {
		raw, err := o.Synth.Synthesize(ctx, synthPrompt(intent, usable))
		if err == nil {
			var parsed SynthOutput
			if parseJSON(raw, &parsed) == nil {
				allowed := map[string]bool{}
				for _, claim := range usable {
					allowed[claim.ID] = true
				}
				validIDs := true
				for _, id := range parsed.ClaimIDs {
					if id != "" && !allowed[id] {
						validIDs = false
						break
					}
				}
				if validIDs {
					answer = strings.TrimSpace(parsed.Answer)
					coverage = append(coverage, parsed.Limitations...)
				}
			}
		}
	}
	if answer == "" {
		var lines []string
		for _, claim := range usable {
			lines = append(lines, "- "+claim.Statement)
		}
		answer = strings.Join(lines, "\n")
		coverage = append(coverage, "synthesis fell back to the verified claim list")
	}
	if o.stopped(ctx) {
		return Result{Outcome: OutcomeCancelled, Coverage: coverage, Claims: usable}
	}
	outcome := OutcomePartial
	if verified == len(usable) && coverageEmpty(coverage) {
		outcome = OutcomeVerified
	}
	markdown := renderReport(intent, string(outcome), answer, usable, evidence, uniqueStrings(coverage))
	_ = o.append(ctx, "research_report", map[string]any{"outcome": string(outcome)})
	return Result{Outcome: outcome, Answer: answer, Markdown: markdown, Coverage: uniqueStrings(coverage), Claims: usable}
}

func spawnOptions(intent Intent) *subagentruntime.HostSpawnOptions {
	hint := intent.SourceHint
	if hint == SourceBoth && !SearchChannelsEnabled() {
		hint = SourceWorkspace
	}
	return ChildHostSpawnOptions(hint)
}

func coverageEmpty(notes []string) bool {
	for _, note := range notes {
		if strings.TrimSpace(note) != "" {
			return false
		}
	}
	return true
}

func shardClaims(claims []Claim, shards int) [][]Claim {
	if len(claims) == 0 {
		return nil
	}
	if shards < 1 {
		shards = 1
	}
	if len(claims) < shards {
		shards = 1
	}
	out := make([][]Claim, shards)
	for i, claim := range claims {
		out[i%shards] = append(out[i%shards], claim)
	}
	var compact [][]Claim
	for _, shard := range out {
		if len(shard) > 0 {
			compact = append(compact, shard)
		}
	}
	return compact
}

func flattenEvidence(values []Evidence) []Evidence {
	return values
}

func (o *Orchestrator) setPlan(ctx context.Context, plan, research, verify, report string) error {
	if o.Plan == nil {
		return nil
	}
	return o.Plan.Apply(ctx, []PlanItem{
		{Step: "计划", Status: plan},
		{Step: "调研", Status: research},
		{Step: "核验", Status: verify},
		{Step: "报告", Status: report},
	})
}

func (o *Orchestrator) append(ctx context.Context, kind string, payload map[string]any) error {
	if o.Journal == nil {
		return nil
	}
	return o.Journal.Append(ctx, kind, payload)
}

func (o *Orchestrator) stopped(ctx context.Context) bool {
	if ctx.Err() != nil {
		return true
	}
	return o.Cancelled != nil && o.Cancelled()
}
