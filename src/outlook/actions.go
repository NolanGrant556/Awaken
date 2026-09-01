package outlook

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

type DraftUpdate struct {
	Subject *string
	Body    *string
	To      []Recipient
	CC      []Recipient
	BCC     []Recipient
}

func (c *Client) UpdateDraft(ctx context.Context, mailbox, messageID string, input DraftUpdate) (Message, error) {
	root, err := mailboxRoot(mailbox)
	if err != nil {
		return Message{}, err
	}
	messageID, err = requiredIdentifier("messageId", messageID)
	if err != nil {
		return Message{}, err
	}
	payload := map[string]any{}
	if input.Subject != nil {
		subject := strings.TrimSpace(*input.Subject)
		if len(subject) > 998 {
			return Message{}, fmt.Errorf("subject exceeds 998 characters")
		}
		payload["subject"] = subject
	}
	if input.Body != nil {
		if len(*input.Body) > 100_000 {
			return Message{}, fmt.Errorf("body exceeds 100000 characters")
		}
		payload["body"] = map[string]string{"contentType": "Text", "content": *input.Body}
	}
	for key, recipients := range map[string][]Recipient{
		"toRecipients":  input.To,
		"ccRecipients":  input.CC,
		"bccRecipients": input.BCC,
	} {
		if recipients == nil {
			continue
		}
		value, payloadErr := recipientPayload(recipients)
		if payloadErr != nil {
			return Message{}, payloadErr
		}
		payload[key] = value
	}
	if len(payload) == 0 {
		return Message{}, fmt.Errorf("at least one draft field is required")
	}
	var message Message
	if err := c.do(ctx, http.MethodPatch, root+"/messages/"+url.PathEscape(messageID), payload, &message); err != nil {
		return Message{}, err
	}
	return message, nil
}

func (c *Client) SendDraft(ctx context.Context, mailbox, messageID string) error {
	root, err := mailboxRoot(mailbox)
	if err != nil {
		return err
	}
	messageID, err = requiredIdentifier("messageId", messageID)
	if err != nil {
		return err
	}
	return c.do(ctx, http.MethodPost, root+"/messages/"+url.PathEscape(messageID)+"/send", nil, nil)
}

func (c *Client) CreateReplyDraft(ctx context.Context, mailbox, messageID, comment string, replyAll bool, cc []Recipient, attachmentPaths []string) (Message, error) {
	root, err := mailboxRoot(mailbox)
	if err != nil {
		return Message{}, err
	}
	messageID, err = requiredIdentifier("messageId", messageID)
	if err != nil {
		return Message{}, err
	}
	if len(comment) > 100_000 {
		return Message{}, fmt.Errorf("comment exceeds 100000 characters")
	}
	action := "createReply"
	if replyAll {
		action = "createReplyAll"
	}
	var message Message
	if err := c.do(ctx, http.MethodPost, root+"/messages/"+url.PathEscape(messageID)+"/"+action, map[string]string{"comment": comment}, &message); err != nil {
		return Message{}, err
	}
	if len(cc) > 0 {
		merged := mergeRecipients(emailPartiesToRecipients(message.CCRecipients), cc)
		updated, updateErr := c.UpdateDraft(ctx, mailbox, message.ID, DraftUpdate{CC: merged})
		if updateErr != nil {
			return Message{}, fmt.Errorf("reply draft %s was created but CC recipients could not be added: %w", message.ID, updateErr)
		}
		message = updated
	}
	if err := c.AddAttachments(ctx, mailbox, message.ID, attachmentPaths); err != nil {
		return Message{}, fmt.Errorf("reply draft %s was created but attachments could not be added: %w", message.ID, err)
	}
	return message, nil
}

func emailPartiesToRecipients(parties []EmailParty) []Recipient {
	result := make([]Recipient, 0, len(parties))
	for _, party := range parties {
		result = append(result, party.EmailAddress)
	}
	return result
}

func mergeRecipients(groups ...[]Recipient) []Recipient {
	result := make([]Recipient, 0)
	seen := make(map[string]struct{})
	for _, group := range groups {
		for _, recipient := range group {
			key := strings.ToLower(strings.TrimSpace(recipient.Address))
			if key == "" {
				continue
			}
			if _, duplicate := seen[key]; duplicate {
				continue
			}
			seen[key] = struct{}{}
			result = append(result, recipient)
		}
	}
	return result
}

func (c *Client) ForwardMessage(ctx context.Context, mailbox, messageID, comment string, to []Recipient) error {
	root, err := mailboxRoot(mailbox)
	if err != nil {
		return err
	}
	messageID, err = requiredIdentifier("messageId", messageID)
	if err != nil {
		return err
	}
	if len(comment) > 100_000 {
		return fmt.Errorf("comment exceeds 100000 characters")
	}
	recipients, err := recipientPayload(to)
	if err != nil {
		return err
	}
	if len(recipients) == 0 {
		return fmt.Errorf("at least one recipient is required")
	}
	payload := map[string]any{"comment": comment, "toRecipients": recipients}
	return c.do(ctx, http.MethodPost, root+"/messages/"+url.PathEscape(messageID)+"/forward", payload, nil)
}

func (c *Client) CreateForwardDraft(ctx context.Context, mailbox, messageID, comment string, to []Recipient, attachmentPaths []string) (Message, error) {
	root, err := mailboxRoot(mailbox)
	if err != nil {
		return Message{}, err
	}
	messageID, err = requiredIdentifier("messageId", messageID)
	if err != nil {
		return Message{}, err
	}
	if len(comment) > 100_000 {
		return Message{}, fmt.Errorf("comment exceeds 100000 characters")
	}
	recipients, err := recipientPayload(to)
	if err != nil {
		return Message{}, err
	}
	if len(recipients) == 0 {
		return Message{}, fmt.Errorf("at least one recipient is required")
	}
	payload := map[string]any{"comment": comment, "toRecipients": recipients}
	var message Message
	if err := c.do(ctx, http.MethodPost, root+"/messages/"+url.PathEscape(messageID)+"/createForward", payload, &message); err != nil {
		return Message{}, err
	}
	if err := c.AddAttachments(ctx, mailbox, message.ID, attachmentPaths); err != nil {
		return Message{}, fmt.Errorf("forward draft %s was created but attachments could not be added: %w", message.ID, err)
	}
	return message, nil
}
