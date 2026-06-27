package gatewaymeta

import (
	"sync"
	"time"
)

type ChatType string

const (
	ChatTypeDM      ChatType = "dm"
	ChatTypeGroup   ChatType = "group"
	ChatTypeChannel ChatType = "channel"
	ChatTypeThread  ChatType = "thread"
)

type CostStatus string

const (
	CostStatusUnknown   CostStatus = "unknown"
	CostStatusEstimated CostStatus = "estimated"
	CostStatusFinal     CostStatus = "final"
)

type SessionSource struct {
	Platform     string            `json:"platform,omitempty"`
	ChatID       string            `json:"chat_id,omitempty"`
	ChatName     string            `json:"chat_name,omitempty"`
	ChatType     ChatType          `json:"chat_type,omitempty"`
	UserID       string            `json:"user_id,omitempty"`
	UserName     string            `json:"user_name,omitempty"`
	ThreadID     string            `json:"thread_id,omitempty"`
	MessageID    string            `json:"message_id,omitempty"`
	RoutingHints map[string]string `json:"routing_hints,omitempty"`
}

type TokenUsage struct {
	InputTokens      int `json:"input_tokens"`
	OutputTokens     int `json:"output_tokens"`
	CacheReadTokens  int `json:"cache_read_tokens"`
	CacheWriteTokens int `json:"cache_write_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type CostState struct {
	EstimatedUSD float64    `json:"estimated_usd"`
	Status       CostStatus `json:"status"`
}

type AutoResetState struct {
	WasAutoReset     bool       `json:"was_auto_reset"`
	Reason           string     `json:"reason,omitempty"`
	ResetHadActivity bool       `json:"reset_had_activity,omitempty"`
	MarkedAt         *time.Time `json:"marked_at,omitempty"`
}

type ResumeState struct {
	Pending  bool       `json:"pending"`
	Reason   string     `json:"reason,omitempty"`
	MarkedAt *time.Time `json:"marked_at,omitempty"`
}

type Entry struct {
	mu sync.RWMutex

	sessionKey       string
	sessionID        string
	createdAt        time.Time
	updatedAt        time.Time
	source           SessionSource
	displayName      string
	platform         string
	chatType         ChatType
	usage            TokenUsage
	cost             CostState
	lastPromptTokens int
	autoReset        AutoResetState
	suspended        bool
	resume           ResumeState
}

type Snapshot struct {
	SessionKey       string         `json:"session_key"`
	SessionID        string         `json:"session_id"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	Source           SessionSource  `json:"source"`
	DisplayName      string         `json:"display_name,omitempty"`
	Platform         string         `json:"platform,omitempty"`
	ChatType         ChatType       `json:"chat_type,omitempty"`
	Usage            TokenUsage     `json:"usage"`
	Cost             CostState      `json:"cost"`
	LastPromptTokens int            `json:"last_prompt_tokens"`
	AutoReset        AutoResetState `json:"auto_reset"`
	Suspended        bool           `json:"suspended"`
	Resume           ResumeState    `json:"resume"`
}

func NewEntry(sessionKey, sessionID string, source SessionSource, createdAt time.Time) *Entry {
	now := normalizeTime(createdAt)
	source = normalizeSource(source)

	return &Entry{
		sessionKey:  sessionKey,
		sessionID:   sessionID,
		createdAt:   now,
		updatedAt:   now,
		source:      source,
		displayName: defaultDisplayName(source),
		platform:    source.Platform,
		chatType:    source.ChatType,
		cost: CostState{
			Status: CostStatusUnknown,
		},
	}
}

func (e *Entry) Touch(at time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.updatedAt = normalizeTime(at)
}

func (e *Entry) UpdateActivity(at time.Time, lastPromptTokens int) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.updatedAt = normalizeTime(at)
	e.lastPromptTokens = lastPromptTokens
}

func (e *Entry) AddUsage(usage TokenUsage, cost CostState, at time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.usage.InputTokens += usage.InputTokens
	e.usage.OutputTokens += usage.OutputTokens
	e.usage.CacheReadTokens += usage.CacheReadTokens
	e.usage.CacheWriteTokens += usage.CacheWriteTokens
	e.usage.TotalTokens += usage.totalDelta()
	e.cost.EstimatedUSD += cost.EstimatedUSD
	if cost.Status != "" {
		e.cost.Status = cost.Status
	}
	if e.cost.Status == "" {
		e.cost.Status = CostStatusUnknown
	}
	e.updatedAt = normalizeTime(at)
}

func (e *Entry) MarkAutoReset(reason string, resetHadActivity bool, at time.Time) {
	now := normalizeTime(at)

	e.mu.Lock()
	defer e.mu.Unlock()

	e.autoReset = AutoResetState{
		WasAutoReset:     true,
		Reason:           reason,
		ResetHadActivity: resetHadActivity,
		MarkedAt:         timePtr(now),
	}
	e.updatedAt = now
}

func (e *Entry) Suspend(at time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.suspended = true
	e.resume = ResumeState{}
	e.updatedAt = normalizeTime(at)
}

func (e *Entry) MarkResumePending(reason string, at time.Time) bool {
	now := normalizeTime(at)

	e.mu.Lock()
	defer e.mu.Unlock()

	if e.suspended {
		return false
	}
	e.resume = ResumeState{
		Pending:  true,
		Reason:   reason,
		MarkedAt: timePtr(now),
	}
	e.updatedAt = now
	return true
}

func (e *Entry) ClearResumePending(at time.Time) bool {
	now := normalizeTime(at)

	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.resume.Pending {
		return false
	}
	e.resume = ResumeState{}
	e.updatedAt = now
	return true
}

func (e *Entry) ToSnapshot() Snapshot {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return Snapshot{
		SessionKey:       e.sessionKey,
		SessionID:        e.sessionID,
		CreatedAt:        e.createdAt,
		UpdatedAt:        e.updatedAt,
		Source:           e.source.clone(),
		DisplayName:      e.displayName,
		Platform:         e.platform,
		ChatType:         e.chatType,
		Usage:            e.usage,
		Cost:             e.cost,
		LastPromptTokens: e.lastPromptTokens,
		AutoReset:        e.autoReset.clone(),
		Suspended:        e.suspended,
		Resume:           e.resume.clone(),
	}
}

func (e *Entry) Clone() *Entry {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return &Entry{
		sessionKey:       e.sessionKey,
		sessionID:        e.sessionID,
		createdAt:        e.createdAt,
		updatedAt:        e.updatedAt,
		source:           e.source.clone(),
		displayName:      e.displayName,
		platform:         e.platform,
		chatType:         e.chatType,
		usage:            e.usage,
		cost:             e.cost,
		lastPromptTokens: e.lastPromptTokens,
		autoReset:        e.autoReset.clone(),
		suspended:        e.suspended,
		resume:           e.resume.clone(),
	}
}

func (s SessionSource) clone() SessionSource {
	copied := s
	if s.RoutingHints != nil {
		copied.RoutingHints = make(map[string]string, len(s.RoutingHints))
		for key, value := range s.RoutingHints {
			copied.RoutingHints[key] = value
		}
	}
	return copied
}

func normalizeSource(source SessionSource) SessionSource {
	source = source.clone()
	if source.ChatType == "" {
		source.ChatType = ChatTypeDM
	}
	return source
}

func defaultDisplayName(source SessionSource) string {
	switch {
	case source.ChatName != "":
		return source.ChatName
	case source.UserName != "":
		return source.UserName
	case source.ChatID != "":
		return source.ChatID
	case source.UserID != "":
		return source.UserID
	default:
		return source.Platform
	}
}

func (u TokenUsage) totalDelta() int {
	if u.TotalTokens > 0 {
		return u.TotalTokens
	}
	return u.InputTokens + u.OutputTokens + u.CacheReadTokens + u.CacheWriteTokens
}

func (s AutoResetState) clone() AutoResetState {
	s.MarkedAt = cloneTimePtr(s.MarkedAt)
	return s
}

func (s ResumeState) clone() ResumeState {
	s.MarkedAt = cloneTimePtr(s.MarkedAt)
	return s
}

func normalizeTime(at time.Time) time.Time {
	if at.IsZero() {
		return time.Now().UTC()
	}
	return at.UTC()
}

func timePtr(at time.Time) *time.Time {
	copied := at
	return &copied
}

func cloneTimePtr(at *time.Time) *time.Time {
	if at == nil {
		return nil
	}
	return timePtr(*at)
}
