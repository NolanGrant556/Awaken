package runtime

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"awaken/src/agentcore"
)

type massReviewRecorder struct {
	mu       sync.Mutex
	records  []agentcore.SemanticRecord
	failKind agentcore.SemanticKind
	appended chan agentcore.SemanticKind
}

func newMassReviewRecorder() *massReviewRecorder {
	return &massReviewRecorder{appended: make(chan agentcore.SemanticKind, 8)}
}

func (r *massReviewRecorder) Append(_ context.Context, record agentcore.SemanticRecord) error {
	r.mu.Lock()
	fail := r.failKind == record.Kind
	if !fail {
		r.records = append(r.records, record)
	}
	r.mu.Unlock()
	if fail {
		return errors.New("forced persistence failure")
	}
	r.appended <- record.Kind
	return nil
}

func (r *massReviewRecorder) AppendBatch(ctx context.Context, records []agentcore.SemanticRecord) error {
	for _, record := range records {
		if err := r.Append(ctx, record); err != nil {
			return err
		}
	}
	return nil
}

func (r *massReviewRecorder) Flush(context.Context) error { return nil }

func (r *massReviewRecorder) setFailKind(kind agentcore.SemanticKind) {
	r.mu.Lock()
	r.failKind = kind
	r.mu.Unlock()
}

func (r *massReviewRecorder) snapshot() []agentcore.SemanticRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]agentcore.SemanticRecord(nil), r.records...)
}

var testMassReviewDigests = MassMessageReviewDigests{
	EnvelopeDigest:          "message_envelope_1",
	BodyDigest:              "sha256:body_1",
	RecipientIdentityDigest: "recipients_sha256:identity_1",
}

func TestMassMessageReviewIsDurableBeforeApprovalAndRejectsMismatchAndReplay(t *testing.T) {
	recorder := newMassReviewRecorder()
	session := NewSession("run_review", "thread_review", "send", nil, nil, func() {})
	session.SetSemanticRecorder(recorder)

	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	decisionCh := make(chan agentcore.ReviewDecision, 1)
	errCh := make(chan error, 1)
	go func() {
		decision, err := session.AwaitMassMessageReview(ctx, agentcore.AwakenEvent{
			ReviewID:                      "review_1",
			ToolCallID:                    "call_1",
			ToolName:                      "connector_send",
			MassMessageChannel:            "email",
			MassMessageRecipientScope:     "segment:customers",
			MassMessageRecipientCount:     42,
			MassMessageMessageCount:       42,
			MassMessageExpandsRecipients:  true,
			MassMessageBodyPreview:        "hello",
			MassMessageSourceTool:         "connector_send",
			ReviewEnvelopeDigest:          testMassReviewDigests.EnvelopeDigest,
			ReviewBodyDigest:              testMassReviewDigests.BodyDigest,
			ReviewRecipientIdentityDigest: testMassReviewDigests.RecipientIdentityDigest,
		})
		if err != nil {
			errCh <- err
			return
		}
		decisionCh <- decision
	}()

	select {
	case kind := <-recorder.appended:
		if kind != agentcore.SemanticKindMassMessageReviewRequested {
			t.Fatalf("first durable record kind=%q", kind)
		}
	case <-ctx.Done():
		t.Fatal("review request was not persisted")
	}

	mutated := testMassReviewDigests
	mutated.BodyDigest = "sha256:mutated"
	if err := session.ResolveMassMessageReview("review_1", agentcore.ReviewDecisionApproved, mutated); !errors.Is(err, ErrMassMessageReviewDigestMismatch) {
		t.Fatalf("mutated approval must be rejected, got %v", err)
	}
	select {
	case decision := <-decisionCh:
		t.Fatalf("digest mismatch released execution: %q", decision)
	case <-time.After(20 * time.Millisecond):
	}

	if err := session.ResolveMassMessageReview("review_1", agentcore.ReviewDecisionApproved, testMassReviewDigests); err != nil {
		t.Fatalf("resolve review: %v", err)
	}
	select {
	case decision := <-decisionCh:
		if decision != agentcore.ReviewDecisionApproved {
			t.Fatalf("decision=%q", decision)
		}
	case err := <-errCh:
		t.Fatalf("await review: %v", err)
	case <-ctx.Done():
		t.Fatal("approved review did not release the waiting execution")
	}
	if err := session.ResolveMassMessageReview("review_1", agentcore.ReviewDecisionApproved, testMassReviewDigests); !errors.Is(err, ErrMassMessageReviewAlreadyResolved) {
		t.Fatalf("identical repeated resolution must be rejected as replay: %v", err)
	}

	records := recorder.snapshot()
	if len(records) != 2 {
		t.Fatalf("records=%+v", records)
	}
	if records[0].Kind != agentcore.SemanticKindMassMessageReviewRequested || records[1].Kind != agentcore.SemanticKindMassMessageReviewResolved {
		t.Fatalf("unexpected durable order: %+v", records)
	}
	if records[1].Payload["decision"] != string(agentcore.ReviewDecisionApproved) {
		t.Fatalf("resolved payload=%+v", records[1].Payload)
	}
	if records[0].Payload["message_count"] != 42 || records[0].Payload["expands_recipients"] != true {
		t.Fatalf("batch expansion facts missing: %+v", records[0].Payload)
	}
	if records[0].Payload["envelope_digest"] == "" || records[1].Payload["envelope_digest"] == "" {
		t.Fatalf("missing envelope digest: %+v", records)
	}
	if records[0].Payload["envelope_digest"] != records[1].Payload["envelope_digest"] {
		t.Fatalf("review resolution changed the outbound envelope identity: %+v", records)
	}
	for _, key := range []string{"body_digest", "recipient_identity_digest"} {
		if records[0].Payload[key] == "" || records[0].Payload[key] != records[1].Payload[key] {
			t.Fatalf("review resolution changed canonical %s: %+v", key, records)
		}
	}
}

func TestMassMessageReviewPersistenceFailureDoesNotReleaseExecution(t *testing.T) {
	recorder := newMassReviewRecorder()
	recorder.setFailKind(agentcore.SemanticKindMassMessageReviewResolved)
	session := NewSession("run_review_fail", "thread_review_fail", "send", nil, nil, func() {})
	session.SetSemanticRecorder(recorder)

	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	decisionCh := make(chan agentcore.ReviewDecision, 1)
	errCh := make(chan error, 1)
	go func() {
		decision, err := session.AwaitMassMessageReview(ctx, agentcore.AwakenEvent{
			ReviewID: "review_fail", ToolCallID: "call_fail",
			ReviewEnvelopeDigest:          testMassReviewDigests.EnvelopeDigest,
			ReviewBodyDigest:              testMassReviewDigests.BodyDigest,
			ReviewRecipientIdentityDigest: testMassReviewDigests.RecipientIdentityDigest,
		})
		if err != nil {
			errCh <- err
			return
		}
		decisionCh <- decision
	}()

	select {
	case <-recorder.appended:
	case <-ctx.Done():
		t.Fatal("review request was not persisted")
	}
	if err := session.ResolveMassMessageReview("review_fail", agentcore.ReviewDecisionApproved, testMassReviewDigests); err == nil {
		t.Fatal("expected resolution persistence failure")
	}
	select {
	case decision := <-decisionCh:
		t.Fatalf("undurable decision released execution: %q", decision)
	case err := <-errCh:
		t.Fatalf("await ended before retry: %v", err)
	case <-time.After(30 * time.Millisecond):
	}

	recorder.setFailKind("")
	if err := session.ResolveMassMessageReview("review_fail", agentcore.ReviewDecisionApproved, testMassReviewDigests); err != nil {
		t.Fatalf("retry durable resolution: %v", err)
	}
	select {
	case decision := <-decisionCh:
		if decision != agentcore.ReviewDecisionApproved {
			t.Fatalf("decision=%q", decision)
		}
	case err := <-errCh:
		t.Fatalf("await review: %v", err)
	case <-ctx.Done():
		t.Fatal("durable retry did not release execution")
	}
}
