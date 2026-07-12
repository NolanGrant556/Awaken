package tool

import (
	"context"
	"errors"
	"testing"
)

func TestMassMessageApprovalGrantIsExactAndOneShot(t *testing.T) {
	binding := MassMessageApprovalBinding{
		RunID: "run-1", ToolCallID: "call-1", ToolName: "mcp__mail__send",
		ProviderID: "connector:mail:send", EnvelopeDigest: "message-1",
		BodyDigest: "sha256:body-1", RecipientIdentityDigest: "recipients_sha256:1",
	}
	ctx, revoke, err := IssueMassMessageApprovalGrant(context.Background(), binding)
	if err != nil {
		t.Fatalf("issue grant: %v", err)
	}
	defer revoke()
	mutated := binding
	mutated.BodyDigest = "sha256:mutated"
	if err := ConsumeMassMessageApprovalGrant(ctx, mutated); !errors.Is(err, ErrMassMessageApprovalMismatch) {
		t.Fatalf("mutation should mismatch, got %v", err)
	}
	if err := ConsumeMassMessageApprovalGrant(ctx, binding); !errors.Is(err, ErrMassMessageApprovalInvalid) {
		t.Fatalf("mismatch must burn token, got %v", err)
	}

	ctx, revoke, err = IssueMassMessageApprovalGrant(context.Background(), binding)
	if err != nil {
		t.Fatalf("issue exact grant: %v", err)
	}
	defer revoke()
	if err := ConsumeMassMessageApprovalGrant(ctx, binding); err != nil {
		t.Fatalf("consume exact grant: %v", err)
	}
	if err := ConsumeMassMessageApprovalGrant(ctx, binding); !errors.Is(err, ErrMassMessageApprovalInvalid) {
		t.Fatalf("replay should fail, got %v", err)
	}
}

func TestMessageRecipientIdentityDigestIsCanonicalAndOpaque(t *testing.T) {
	a := []MessageRecipient{{ID: "person@example.com", Kind: RecipientIndividual}, {ID: "group-1", Kind: RecipientGroup}}
	b := []MessageRecipient{{ID: "group-1", Kind: RecipientGroup}, {ID: "person@example.com", Kind: RecipientIndividual}}
	if got, want := MessageRecipientIdentityDigest(a), MessageRecipientIdentityDigest(b); got != want {
		t.Fatalf("recipient order changed canonical digest: %q != %q", got, want)
	}
	digest := MessageRecipientIdentityDigest(a)
	if digest == "" || digest == a[0].ID || digest == a[1].ID {
		t.Fatalf("recipient identity digest is not opaque: %q", digest)
	}
}
