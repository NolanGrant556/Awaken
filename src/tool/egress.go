package tool

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
)

type MessageRecipientKind string

const (
	RecipientIndividual MessageRecipientKind = "individual"
	RecipientGroup      MessageRecipientKind = "group"
	RecipientChannel    MessageRecipientKind = "channel"
	RecipientBroadcast  MessageRecipientKind = "broadcast"
	RecipientUnknown    MessageRecipientKind = "unknown"
)

type MessageRecipient struct {
	ID   string               `json:"id"`
	Kind MessageRecipientKind `json:"kind"`
}

type MessageEgressEnvelope struct {
	ProviderID        ProviderID         `json:"provider_id"`
	Channel           string             `json:"channel"`
	SendingAccount    string             `json:"sending_account"`
	Recipients        []MessageRecipient `json:"recipients"`
	RecipientScope    string             `json:"recipient_scope,omitempty"`
	MessageCount      int                `json:"message_count"`
	ExpandsRecipients bool               `json:"expands_recipients,omitempty"`
	BodyDigest        string             `json:"body_digest"`
	BodyPreview       string             `json:"body_preview,omitempty"`
	Attachments       []string           `json:"attachments,omitempty"`
	EnvelopeDigest    string             `json:"envelope_digest"`
}

type EgressAction string

const (
	EgressAllow           EgressAction = "allow"
	EgressDeny            EgressAction = "deny"
	EgressNeedsMassReview EgressAction = "needs_mass_message_review"
)

type MessageEgressDecision struct {
	Action         EgressAction `json:"action"`
	Reason         string       `json:"reason"`
	EnvelopeDigest string       `json:"envelope_digest"`
	RecipientCount int          `json:"recipient_count"`
}

type MessageEgressGateway interface {
	Decide(ctx context.Context, envelope MessageEgressEnvelope) (MessageEgressDecision, error)
}

var ErrUnobservableMessageRecipients = errors.New("message recipient semantics are not observable")

type DefaultMessageEgressGateway struct{}

func (DefaultMessageEgressGateway) Decide(ctx context.Context, envelope MessageEgressEnvelope) (MessageEgressDecision, error) {
	if err := ctx.Err(); err != nil {
		return MessageEgressDecision{}, err
	}
	if envelope.MessageCount <= 0 {
		envelope.MessageCount = 1
	}
	envelope.EnvelopeDigest = MessageEnvelopeDigest(envelope)
	decision := MessageEgressDecision{
		EnvelopeDigest: envelope.EnvelopeDigest,
		RecipientCount: len(envelope.Recipients),
	}
	if strings.TrimSpace(envelope.Channel) == "" || strings.TrimSpace(envelope.SendingAccount) == "" || len(envelope.Recipients) == 0 {
		decision.Action = EgressDeny
		decision.Reason = "channel, sending account, and recipients must be observable before sending"
		return decision, ErrUnobservableMessageRecipients
	}
	for _, recipient := range envelope.Recipients {
		if strings.TrimSpace(recipient.ID) == "" || recipient.Kind == "" || recipient.Kind == RecipientUnknown {
			decision.Action = EgressDeny
			decision.Reason = "recipient identity or kind is unknown"
			return decision, ErrUnobservableMessageRecipients
		}
	}
	mass := envelope.ExpandsRecipients || envelope.MessageCount > 1 || len(envelope.Recipients) > 1
	for _, recipient := range envelope.Recipients {
		switch recipient.Kind {
		case RecipientGroup, RecipientChannel, RecipientBroadcast:
			mass = true
		}
	}
	if mass {
		decision.Action = EgressNeedsMassReview
		decision.Reason = "message envelope targets a group, channel, multiple recipients, or multiple sends"
		return decision, nil
	}
	decision.Action = EgressAllow
	decision.Reason = "single message to one observable individual recipient"
	return decision, nil
}

func MessageEnvelopeDigest(envelope MessageEgressEnvelope) string {
	envelope.EnvelopeDigest = ""
	envelope.BodyPreview = ""
	if envelope.MessageCount <= 0 {
		envelope.MessageCount = 1
	}
	data, _ := json.Marshal(envelope)
	digest := sha256.Sum256(data)
	return "message_" + hex.EncodeToString(digest[:16])
}

// MessageRecipientIdentityDigest is the canonical, non-reversible identity
// binding used by durable review. Raw recipient identifiers must never be
// copied into approval tokens, diagnostics, or protocol events.
func MessageRecipientIdentityDigest(recipients []MessageRecipient) string {
	identities := make([]string, 0, len(recipients))
	for _, recipient := range recipients {
		identities = append(identities, strings.ToLower(strings.TrimSpace(string(recipient.Kind)))+"\x00"+strings.TrimSpace(recipient.ID))
	}
	sort.Strings(identities)
	data, _ := json.Marshal(identities)
	digest := sha256.Sum256(data)
	return "recipients_sha256:" + hex.EncodeToString(digest[:])
}

// MassMessageMetadataFromEnvelope converts the canonical egress envelope into
// review metadata without dropping the two facts that distinguish one send
// from a batch: how many messages are planned and whether execution can expand
// the recipient set after review begins.
func MassMessageMetadataFromEnvelope(envelope MessageEgressEnvelope, sourceTool string) MassMessageMetadata {
	messageCount := envelope.MessageCount
	if messageCount <= 0 {
		messageCount = 1
	}
	return MassMessageMetadata{
		Channel:                 envelope.Channel,
		RecipientScope:          envelope.RecipientScope,
		RecipientCount:          len(envelope.Recipients),
		MessageCount:            messageCount,
		ExpandsRecipients:       envelope.ExpandsRecipients,
		BodyPreview:             envelope.BodyPreview,
		Attachments:             append([]string(nil), envelope.Attachments...),
		SendingAccount:          envelope.SendingAccount,
		SourceTool:              strings.TrimSpace(sourceTool),
		EnvelopeDigest:          MessageEnvelopeDigest(envelope),
		BodyDigest:              strings.TrimSpace(envelope.BodyDigest),
		RecipientIdentityDigest: MessageRecipientIdentityDigest(envelope.Recipients),
	}
}
