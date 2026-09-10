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
	Run(ctx context.Context, spec ChildSpec) (ChildResult, error)
}

type ChildResult struct {
	AgentID   string
	TaskID    string
	AttemptID string
	Output    string
	Receipts  []subagentruntime.HostToolReceipt
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

type PrefetchedSource struct {
	URL       string
	Content   string
	ErrorCode string
	Receipt   *subagentruntime.HostToolReceipt
}

type SourcePrefetcher interface {
	Fetch(ctx context.Context, url string) (PrefetchedSource, error)
}

type Journal interface {
	Append(ctx context.Context, kind string, payload map[string]any) error
}

type Orchestrator struct {
	Children  ChildClient
	Plan      PlanWriter
	PublishFn Publisher
	Synth     Synthesizer
	Prefetch  SourcePrefetcher
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
		if err := o.append(ctx, "research_accepted", map[string]any{"query": intent.Query, "breadth": intent.Breadth}); err != nil {
			return Result{}, err
		}
		if err := o.append(ctx, "research_terminal", map[string]any{"outcome": string(OutcomeFailed)}); err != nil {
			return Result{}, err
		}
		return Result{Outcome: OutcomeFailed, Coverage: coverage}, nil
	}
	if err := o.append(ctx, "research_accepted", map[string]any{"query": intent.Query, "breadth": intent.Breadth}); err != nil {
		return Result{}, err
	}
	if err := o.setPlan(ctx, "in_progress", "pending", "pending", "pending"); err != nil {
		return Result{}, err
	}
	prefetched, prefetchCoverage, err := o.prefetchExplicitSources(ctx, intent)
	if err != nil {
		return Result{}, err
	}

	questions, coverage, err := o.plan(ctx, intent)
	if err != nil {
		return Result{}, err
	}
	coverage = append(coverage, prefetchCoverage...)
	if len(prefetched) > 0 && len(questions) > 0 {
		questions[0].Text = "Inspect the exact user-provided URL without changing or dropping its query string, summarize its content, and assess source trust: " + prefetched[0].URL
	}
	if intent.SourceHint == SourceBoth && !SearchChannelsEnabled() {
		coverage = append(coverage, "search channels unavailable; continuing workspace-only")
	}
	if o.stopped(ctx) {
		return Result{Outcome: OutcomeCancelled, Coverage: coverage}, nil
	}
	if err := o.setPlan(ctx, "completed", "in_progress", "pending", "pending"); err != nil {
		return Result{}, err
	}

	claims, evidence, coverage, err := o.research(ctx, intent, questions, prefetched, coverage)
	if err != nil {
		return Result{}, err
	}
	if o.stopped(ctx) {
		return Result{Outcome: OutcomeCancelled, Coverage: coverage, Claims: claims}, nil
	}
	if err := o.setPlan(ctx, "completed", "completed", "in_progress", "pending"); err != nil {
		return Result{}, err
	}

	claims, coverage, err = o.verify(ctx, intent, claims, evidence, coverage)
	if err != nil {
		return Result{}, err
	}
	if o.stopped(ctx) {
		return Result{Outcome: OutcomeCancelled, Coverage: coverage, Claims: claims}, nil
	}
	if err := o.setPlan(ctx, "completed", "completed", "completed", "in_progress"); err != nil {
		return Result{}, err
	}

	result, err := o.report(ctx, intent, claims, evidence, coverage)
	if err != nil {
		return Result{}, err
	}
	if result.Outcome == OutcomeCancelled || o.stopped(ctx) {
		return Result{Outcome: OutcomeCancelled, Coverage: uniqueStrings(append(coverage, result.Coverage...)), Claims: result.Claims}, nil
	}
	if o.PublishFn != nil && strings.TrimSpace(result.Markdown) != "" && result.Outcome != OutcomeFailed {
		path, artifactID, err := o.PublishFn.Publish(ctx, result.Markdown)
		if err != nil {
			result.Outcome = OutcomeFailed
			result.Coverage = append(result.Coverage, "report publish failed")
			if appendErr := o.append(ctx, "research_terminal", map[string]any{"outcome": string(result.Outcome)}); appendErr != nil {
				return result, appendErr
			}
			return result, err
		}
		result.ReportPath = path
		result.ArtifactID = artifactID
	}
	if result.Outcome != OutcomeFailed {
		if err := o.setPlan(ctx, "completed", "completed", "completed", "completed"); err != nil {
			return result, err
		}
	}
	if err := o.append(ctx, "research_terminal", map[string]any{"outcome": string(result.Outcome)}); err != nil {
		return result, err
	}
	return result, nil
}

func (o *Orchestrator) prefetchExplicitSources(ctx context.Context, intent Intent) ([]PrefetchedSource, []string, error) {
	if o.Prefetch == nil || intent.SourceHint == SourceWorkspace {
		return nil, nil, nil
	}
	urls := ExplicitHTTPSURLs(intent.Query)
	results := make([]PrefetchedSource, 0, len(urls))
	var coverage []string
	for index, url := range urls {
		if o.stopped(ctx) {
			return results, coverage, nil
		}
		if index >= 4 {
			coverage = append(coverage, "explicit URL exceeds this research run's four-source prefetch limit: "+url)
			results = append(results, PrefetchedSource{URL: url, ErrorCode: "explicit_source_limit"})
			continue
		}
		result, err := o.Prefetch.Fetch(ctx, url)
		if err != nil {
			return results, coverage, err
		}
		results = append(results, result)
		if strings.TrimSpace(result.ErrorCode) != "" {
			coverage = append(coverage, "exact user URL fetch failed ("+result.ErrorCode+"): "+url)
		}
	}
	return results, coverage, nil
}

func (o *Orchestrator) plan(ctx context.Context, intent Intent) ([]Question, []string, error) {
	child, err := o.Children.Run(ctx, ChildSpec{
		Description: "研究规划", Prompt: plannerPrompt(intent), AgentType: subagentruntime.AgentTypePlan,
		Host: spawnOptions(intent),
	})
	coverage := []string{}
	if err != nil {
		coverage = append(coverage, "planner failed; using the original query")
		return []Question{{ID: "q1", Text: intent.Query}}, coverage, nil
	}
	var parsed PlannerOutput
	if err := parseJSON(child.Output, &parsed); err != nil || len(parsed.Questions) == 0 {
		coverage = append(coverage, "planner output was invalid; using the original query")
		return []Question{{ID: "q1", Text: intent.Query}}, coverage, nil
	}
	if len(parsed.Questions) > intent.Breadth {
		parsed.Questions = parsed.Questions[:intent.Breadth]
	}
	for i := range parsed.Questions {
		if parsed.Questions[i].ID == "" {
			parsed.Questions[i].ID = fmt.Sprintf("q%d", i+1)
		}
	}
	if err := o.append(ctx, "research_plan", map[string]any{"question_count": len(parsed.Questions)}); err != nil {
		return nil, coverage, err
	}
	return parsed.Questions, coverage, nil
}

func (o *Orchestrator) research(ctx context.Context, intent Intent, questions []Question, prefetched []PrefetchedSource, coverage []string) ([]Claim, []Evidence, []string, error) {
	type waveResult struct {
		parsed   ResearcherOutput
		receipts []subagentruntime.HostToolReceipt
		err      error
		failed   string
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
			prompt := researcherPrompt(intent, question)
			var seedReceipts []subagentruntime.HostToolReceipt
			for sourceIndex, source := range prefetched {
				if sourceIndex%len(questions) != index || source.ErrorCode == "explicit_source_limit" {
					continue
				}
				prompt += prefetchedSourcePrompt(source)
				if source.Receipt != nil {
					seedReceipts = append(seedReceipts, *source.Receipt)
				}
			}
			child, err := o.Children.Run(ctx, ChildSpec{
				Description: "调研 " + question.ID, Prompt: prompt, AgentType: subagentruntime.AgentTypeExplore,
				Host: spawnOptions(intent),
			})
			if err != nil {
				results[index] = waveResult{failed: "question " + question.ID + " failed"}
				return
			}
			var parsed ResearcherOutput
			if err := parseJSON(child.Output, &parsed); err != nil {
				results[index] = waveResult{failed: "question " + question.ID + " returned invalid claims"}
				return
			}
			results[index] = waveResult{parsed: parsed, receipts: append(seedReceipts, child.Receipts...)}
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
			id := fmt.Sprintf("claim-%d", claimSeq)
			claimSeq++
			claim := Claim{ID: id, QuestionID: question.ID, Statement: strings.TrimSpace(rawClaim.Statement), Confidence: rawClaim.Confidence, VerificationStatus: "partial"}
			for _, rawEv := range rawClaim.Evidence {
				if len(evidence) >= MaxEvidenceTotal {
					break
				}
				grounded, ok := groundEvidence(rawEv, item.receipts)
				if !ok {
					coverage = append(coverage, "question "+question.ID+" returned evidence without a successful Read/WebFetch receipt")
					continue
				}
				evID := fmt.Sprintf("ev-%d", evSeq)
				evSeq++
				grounded.ID = evID
				evidence = append(evidence, grounded)
				claim.EvidenceIDs = append(claim.EvidenceIDs, evID)
			}
			if strings.TrimSpace(claim.Statement) == "" {
				continue
			}
			claims = append(claims, claim)
			kept++
		}
		coverage = append(coverage, compactCoverageNotes(item.parsed.Uncertainties)...)
	}
	if err := o.append(ctx, "research_claim_batch", map[string]any{"claim_count": len(claims), "evidence_count": len(evidence)}); err != nil {
		return claims, evidence, coverage, err
	}
	for _, source := range prefetched {
		if source.ErrorCode != "" || source.Receipt == nil {
			continue
		}
		locator, _ := source.Receipt.Details["final_url"].(string)
		covered := false
		for _, item := range evidence {
			if item.Locator == source.URL || (locator != "" && item.Locator == locator) {
				covered = true
				break
			}
		}
		if !covered {
			coverage = append(coverage, "explicit URL was fetched but no grounded claim covered it: "+source.URL)
		}
	}
	return claims, evidence, coverage, nil
}

func receiptsForChild(child ChildResult) []subagentruntime.HostToolReceipt {
	if child.AgentID == "" {
		return child.Receipts
	} // legacy/test adapter without execution identities
	var receipts []subagentruntime.HostToolReceipt
	for _, receipt := range child.Receipts {
		if receipt.AgentID == child.AgentID && receipt.TaskID == child.TaskID && receipt.AttemptID == child.AttemptID {
			receipts = append(receipts, receipt)
		}
	}
	return receipts
}

func (o *Orchestrator) verify(ctx context.Context, intent Intent, claims []Claim, evidence []Evidence, coverage []string) ([]Claim, []string, error) {
	if len(claims) == 0 {
		return claims, coverage, nil
	}
	byID := map[string][]Evidence{}
	for _, ev := range evidence {
		byID[ev.ID] = append(byID[ev.ID], ev)
	}
	shards := shardClaims(claims, 2)
	type verificationResult struct {
		child    ChildResult
		err      error
		expected map[string]bool
	}
	results := make([]verificationResult, len(shards))
	var wait sync.WaitGroup
	for index, shard := range shards {
		if o.stopped(ctx) {
			return claims, coverage, nil
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
		wait.Add(1)
		go func(index int, prompt string, expected map[string]bool) {
			defer wait.Done()
			if o.stopped(ctx) {
				results[index] = verificationResult{err: context.Canceled, expected: expected}
				return
			}
			child, err := o.Children.Run(ctx, ChildSpec{Description: "核验", Prompt: prompt, AgentType: subagentruntime.AgentTypeReview, Host: spawnOptions(intent)})
			results[index] = verificationResult{child: child, err: err, expected: expected}
		}(index, prompt, expected)
	}
	wait.Wait()
	if o.stopped(ctx) {
		return claims, coverage, nil
	}
	// Merge in stable shard order so scheduling cannot renumber claims or
	// citations. Every shard keeps its own independently obtained receipts.
	for _, result := range results {
		child, err, expected := result.child, result.err, result.expected
		if err != nil {
			coverage = keepShardPartial(claims, expected, coverage, "verifier shard failed; shard kept as partial")
			continue
		}
		var parsed VerifierOutput
		if err := parseJSON(child.Output, &parsed); err != nil {
			coverage = keepShardPartial(claims, expected, coverage, "verifier returned invalid JSON; shard kept as partial")
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
			coverage = keepShardPartial(claims, expected, coverage, "verifier claim-id mismatch; shard kept as partial")
			continue
		}
		for i := range claims {
			if status, ok := seen[claims[i].ID]; ok {
				if !receiptsSupportClaim(claims[i], evidence, receiptsForChild(child)) {
					claims[i].VerificationStatus = "partial"
					coverage = append(coverage, "verifier did not re-read evidence for "+claims[i].ID+"; claim kept as partial")
					continue
				}
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
	if err := o.append(ctx, "research_verification", map[string]any{"claim_count": len(claims)}); err != nil {
		return claims, coverage, err
	}
	return claims, coverage, nil
}

func (o *Orchestrator) report(ctx context.Context, intent Intent, claims []Claim, evidence []Evidence, coverage []string) (Result, error) {
	if o.stopped(ctx) {
		return Result{Outcome: OutcomeCancelled, Coverage: coverage, Claims: claims}, nil
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
		coverage = append(coverage, "no usable claims")
		answer := "No verified claims were produced."
		markdown := renderReport(intent, string(OutcomePartial), answer, nil, evidence, uniqueStrings(coverage))
		if err := o.append(ctx, "research_report", map[string]any{"outcome": string(OutcomePartial)}); err != nil {
			return Result{}, err
		}
		return Result{Outcome: OutcomePartial, Answer: answer, Markdown: markdown, Coverage: uniqueStrings(coverage), Claims: claims}, nil
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
		return Result{Outcome: OutcomeCancelled, Coverage: coverage, Claims: usable}, nil
	}
	outcome := OutcomePartial
	if verified == len(usable) && coverageEmpty(coverage) {
		outcome = OutcomeVerified
	}
	markdown := renderReport(intent, string(outcome), answer, usable, evidence, uniqueStrings(coverage))
	if err := o.append(ctx, "research_report", map[string]any{"outcome": string(outcome)}); err != nil {
		return Result{}, err
	}
	return Result{Outcome: outcome, Answer: answer, Markdown: markdown, Coverage: uniqueStrings(coverage), Claims: usable}, nil
}

func keepShardPartial(claims []Claim, expected map[string]bool, coverage []string, note string) []string {
	coverage = append(coverage, note)
	for i := range claims {
		if expected[claims[i].ID] && claims[i].VerificationStatus == "verified" {
			claims[i].VerificationStatus = "partial"
		}
	}
	return coverage
}

func compactCoverageNotes(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		note := strings.TrimSpace(value)
		if note == "" {
			continue
		}
		runes := []rune(note)
		if len(runes) > 180 {
			note = string(runes[:180]) + "…"
		}
		out = append(out, note)
	}
	return out
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
