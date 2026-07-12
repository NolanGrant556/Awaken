package tool

import (
	"context"
	"errors"
	"testing"
)

func observableEnvelope() MessageEgressEnvelope {
	return MessageEgressEnvelope{
		ProviderID: "connector:mail", Channel: "email", SendingAccount: "sender@example.com",
		Recipients:     []MessageRecipient{{ID: "person@example.com", Kind: RecipientIndividual}},
		RecipientScope: "explicit individual", MessageCount: 1, BodyDigest: "sha256:body", BodyPreview: "hello",
	}
}

func TestMessageEgressGatewayAllowsOnlyOneObservableIndividualSend(t *testing.T) {
	gateway := DefaultMessageEgressGateway{}
	envelope := observableEnvelope()
	decision, err := gateway.Decide(context.Background(), envelope)
	if err != nil || decision.Action != EgressAllow || decision.EnvelopeDigest == "" {
		t.Fatalf("expected single-send allow, decision=%+v err=%v", decision, err)
	}

	tests := []struct {
		name   string
		mutate func(*MessageEgressEnvelope)
	}{
		{name: "multiple recipients", mutate: func(value *MessageEgressEnvelope) {
			value.Recipients = append(value.Recipients, MessageRecipient{ID: "second@example.com", Kind: RecipientIndividual})
		}},
		{name: "group", mutate: func(value *MessageEgressEnvelope) { value.Recipients[0].Kind = RecipientGroup }},
		{name: "multiple messages", mutate: func(value *MessageEgressEnvelope) { value.MessageCount = 2 }},
		{name: "recipient expansion", mutate: func(value *MessageEgressEnvelope) { value.ExpandsRecipients = true }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := observableEnvelope()
			test.mutate(&candidate)
			decision, err := gateway.Decide(context.Background(), candidate)
			if err != nil || decision.Action != EgressNeedsMassReview {
				t.Fatalf("expected mass review, decision=%+v err=%v", decision, err)
			}
		})
	}
}

func TestMessageEgressGatewayFailsClosedForUnknownRecipients(t *testing.T) {
	envelope := observableEnvelope()
	envelope.Recipients[0].Kind = RecipientUnknown
	decision, err := (DefaultMessageEgressGateway{}).Decide(context.Background(), envelope)
	if !errors.Is(err, ErrUnobservableMessageRecipients) || decision.Action != EgressDeny {
		t.Fatalf("expected unknown-recipient denial, decision=%+v err=%v", decision, err)
	}
}

func TestMassMessageMetadataFromEnvelopePreservesBatchFacts(t *testing.T) {
	envelope := observableEnvelope()
	envelope.MessageCount = 12
	envelope.ExpandsRecipients = true
	envelope.Attachments = []string{"brief.pdf"}
	metadata := MassMessageMetadataFromEnvelope(envelope, "mail_send")
	if metadata.MessageCount != 12 || !metadata.ExpandsRecipients || metadata.RecipientCount != 1 || metadata.SourceTool != "mail_send" {
		t.Fatalf("batch facts were dropped: %+v", metadata)
	}
	envelope.Attachments[0] = "mutated.pdf"
	if metadata.Attachments[0] != "brief.pdf" {
		t.Fatalf("metadata attachments alias caller memory: %+v", metadata.Attachments)
	}
}

func TestMessageEnvelopeDigestBindsRecipientsAndBatchShapeButNotPreview(t *testing.T) {
	base := observableEnvelope()
	first := MessageEnvelopeDigest(base)
	base.BodyPreview = "different preview of same body"
	if got := MessageEnvelopeDigest(base); got != first {
		t.Fatalf("preview-only change must not invalidate reviewed body digest: %q != %q", got, first)
	}
	base.MessageCount = 2
	if got := MessageEnvelopeDigest(base); got == first {
		t.Fatal("message count change must invalidate envelope digest")
	}
}
