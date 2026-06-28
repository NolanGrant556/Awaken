package teamcore

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

type Mailbox struct {
	mu       sync.RWMutex
	messages map[string][]MailboxMessage
	seq      uint64
	now      func() time.Time
}

func NewMailbox() *Mailbox {
	return &Mailbox{
		messages: make(map[string][]MailboxMessage),
		now:      func() time.Time { return time.Now().UTC() },
	}
}

func (m *Mailbox) SendToMember(team Team, fromSlotID, toSlotID string, draft MailboxMessageDraft) (MailboxMessage, error) {
	normalized, from, to, draft, err := normalizeMailboxSend(team, fromSlotID, toSlotID, draft)
	if err != nil {
		return MailboxMessage{}, err
	}

	message := MailboxMessage{
		ID:         newID("message"),
		TeamID:     normalized.ID,
		FromSlotID: from.Slot.ID,
		ToSlotID:   to.Slot.ID,
		FromName:   from.Name,
		ToName:     to.Name,
		Summary:    draft.Summary,
		Content:    draft.Content,
		CreatedAt:  m.clock(),
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.seq++
	message.Seq = m.seq
	key := mailboxKey(message.TeamID, message.ToSlotID)
	m.messages[key] = append(m.messages[key], message)
	return message, nil
}

func (m *Mailbox) Broadcast(team Team, fromSlotID string, draft MailboxMessageDraft) ([]MailboxMessage, error) {
	normalized, err := NormalizeTeam(team)
	if err != nil {
		return nil, err
	}
	if normalized.ID == "" {
		return nil, fmt.Errorf("%w: team id is required", ErrInvalidMessage)
	}
	fromSlotID = strings.TrimSpace(fromSlotID)
	from, ok := findMember(normalized, fromSlotID)
	if !ok {
		return nil, fmt.Errorf("%w: sender slot %q not found", ErrMemberNotFound, fromSlotID)
	}
	draft, err = normalizeMessageDraft(draft)
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	now := m.clock()
	messages := make([]MailboxMessage, 0, len(normalized.Members)-1)
	for _, recipient := range normalized.Members {
		if recipient.Slot.ID == from.Slot.ID {
			continue
		}
		m.seq++
		message := MailboxMessage{
			ID:         newID("message"),
			TeamID:     normalized.ID,
			FromSlotID: from.Slot.ID,
			ToSlotID:   recipient.Slot.ID,
			FromName:   from.Name,
			ToName:     recipient.Name,
			Summary:    draft.Summary,
			Content:    draft.Content,
			Broadcast:  true,
			Seq:        m.seq,
			CreatedAt:  now,
		}
		key := mailboxKey(message.TeamID, message.ToSlotID)
		m.messages[key] = append(m.messages[key], message)
		messages = append(messages, message)
	}
	return messages, nil
}

func (m *Mailbox) ListMessages(teamID, slotID string) []MailboxMessage {
	teamID = strings.TrimSpace(teamID)
	slotID = strings.TrimSpace(slotID)
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]MailboxMessage(nil), m.messages[mailboxKey(teamID, slotID)]...)
}

func normalizeMailboxSend(team Team, fromSlotID, toSlotID string, draft MailboxMessageDraft) (Team, Member, Member, MailboxMessageDraft, error) {
	normalized, err := NormalizeTeam(team)
	if err != nil {
		return Team{}, Member{}, Member{}, MailboxMessageDraft{}, err
	}
	if normalized.ID == "" {
		return Team{}, Member{}, Member{}, MailboxMessageDraft{}, fmt.Errorf("%w: team id is required", ErrInvalidMessage)
	}
	fromSlotID = strings.TrimSpace(fromSlotID)
	toSlotID = strings.TrimSpace(toSlotID)
	from, ok := findMember(normalized, fromSlotID)
	if !ok {
		return Team{}, Member{}, Member{}, MailboxMessageDraft{}, fmt.Errorf("%w: sender slot %q not found", ErrMemberNotFound, fromSlotID)
	}
	to, ok := findMember(normalized, toSlotID)
	if !ok {
		return Team{}, Member{}, Member{}, MailboxMessageDraft{}, fmt.Errorf("%w: recipient slot %q not found", ErrMemberNotFound, toSlotID)
	}
	draft, err = normalizeMessageDraft(draft)
	if err != nil {
		return Team{}, Member{}, Member{}, MailboxMessageDraft{}, err
	}
	return normalized, from, to, draft, nil
}

func normalizeMessageDraft(draft MailboxMessageDraft) (MailboxMessageDraft, error) {
	draft.Summary = strings.TrimSpace(draft.Summary)
	draft.Content = strings.TrimSpace(draft.Content)
	if draft.Content == "" {
		return MailboxMessageDraft{}, fmt.Errorf("%w: content is required", ErrInvalidMessage)
	}
	return draft, nil
}

func mailboxKey(teamID, slotID string) string {
	return teamID + "\x00" + slotID
}

func (m *Mailbox) clock() time.Time {
	if m == nil || m.now == nil {
		return time.Now().UTC()
	}
	return m.now().UTC()
}
