package teamcore

import "time"

type Role string

const (
	RoleLeader   Role = "leader"
	RoleTeammate Role = "teammate"
)

type WorkspaceMode string

const (
	WorkspaceModeShared   WorkspaceMode = "shared"
	WorkspaceModeIsolated WorkspaceMode = "isolated"
)

type SessionMode string

const (
	SessionModePlan   SessionMode = "plan"
	SessionModeManual SessionMode = "manual"
	SessionModeAuto   SessionMode = "auto"
)

type TeamRunStatus string

const (
	TeamRunStatusAccepted  TeamRunStatus = "accepted"
	TeamRunStatusRunning   TeamRunStatus = "running"
	TeamRunStatusCompleted TeamRunStatus = "completed"
	TeamRunStatusCancelled TeamRunStatus = "cancelled"
	TeamRunStatusFailed    TeamRunStatus = "failed"
)

type MemberRunStatus string

const (
	MemberRunStatusAccepted  MemberRunStatus = "accepted"
	MemberRunStatusRunning   MemberRunStatus = "running"
	MemberRunStatusCompleted MemberRunStatus = "completed"
	MemberRunStatusCancelled MemberRunStatus = "cancelled"
	MemberRunStatusFailed    MemberRunStatus = "failed"
)

type TeamEventType string

const (
	TeamEventCreated       TeamEventType = "team.created"
	TeamEventUpdated       TeamEventType = "team.updated"
	TeamEventDeleted       TeamEventType = "team.deleted"
	TeamEventMemberAdded   TeamEventType = "team.member_added"
	TeamEventMemberRemoved TeamEventType = "team.member_removed"
	TeamEventMemberRenamed TeamEventType = "team.member_renamed"
	TeamEventPinned        TeamEventType = "team.pinned"
	TeamEventArchived      TeamEventType = "team.archived"
	TeamEventRunAccepted   TeamEventType = "team.run_accepted"
	TeamEventMemberRunPlan TeamEventType = "team.member_run_planned"
	TeamEventMessageSent   TeamEventType = "team.message_sent"
)

type Slot struct {
	ID   string `json:"id"`
	Role Role   `json:"role"`
}

type RuntimeBinding struct {
	Backend        string            `json:"backend"`
	AgentID        string            `json:"agent_id,omitempty"`
	AgentType      string            `json:"agent_type,omitempty"`
	Model          string            `json:"model,omitempty"`
	ConversationID string            `json:"conversation_id,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

type Member struct {
	Slot        Slot           `json:"slot"`
	Name        string         `json:"name"`
	DisplayName string         `json:"display_name,omitempty"`
	Runtime     RuntimeBinding `json:"runtime"`
	JoinedAt    time.Time      `json:"joined_at,omitempty"`
}

type SharedWorkspace struct {
	Root          string        `json:"root"`
	WritableRoots []string      `json:"writable_roots,omitempty"`
	Mode          WorkspaceMode `json:"mode"`
}

type Team struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	Description  string          `json:"description,omitempty"`
	LeaderSlotID string          `json:"leader_slot_id"`
	Workspace    SharedWorkspace `json:"workspace"`
	SessionMode  SessionMode     `json:"session_mode"`
	Members      []Member        `json:"members"`
	Pinned       bool            `json:"pinned,omitempty"`
	Archived     bool            `json:"archived,omitempty"`
	CreatedAt    time.Time       `json:"created_at,omitempty"`
	UpdatedAt    time.Time       `json:"updated_at,omitempty"`
}

type SlotWork struct {
	SlotID             string `json:"slot_id"`
	Role               Role   `json:"role"`
	PendingWakeCount   int    `json:"pending_wake_count"`
	StartingChildCount int    `json:"starting_child_count"`
}

type TeamRun struct {
	ID                 string        `json:"id"`
	TeamID             string        `json:"team_id"`
	LeaderPrompt       string        `json:"leader_prompt"`
	TargetSlotID       string        `json:"target_slot_id"`
	TargetRole         Role          `json:"target_role"`
	Status             TeamRunStatus `json:"status"`
	ActiveChildCount   int           `json:"active_child_count"`
	PendingWakeCount   int           `json:"pending_wake_count"`
	StartingChildCount int           `json:"starting_child_count"`
	SlotWork           []SlotWork    `json:"slot_work,omitempty"`
	CreatedAt          time.Time     `json:"created_at"`
}

type MemberRun struct {
	ID           string          `json:"id"`
	TeamRunID    string          `json:"team_run_id"`
	TeamID       string          `json:"team_id"`
	SlotID       string          `json:"slot_id"`
	Role         Role            `json:"role"`
	MemberName   string          `json:"member_name"`
	Input        string          `json:"input"`
	Workspace    SharedWorkspace `json:"workspace"`
	Runtime      RuntimeBinding  `json:"runtime"`
	ParentSlotID string          `json:"parent_slot_id,omitempty"`
	ParentRunID  string          `json:"parent_run_id,omitempty"`
	Status       MemberRunStatus `json:"status"`
	SessionMode  SessionMode     `json:"session_mode"`
	PlannedAt    time.Time       `json:"planned_at"`
}

type RunPlan struct {
	TeamRun      TeamRun     `json:"team_run"`
	LeaderRun    MemberRun   `json:"leader_run"`
	TeammateRuns []MemberRun `json:"teammate_runs,omitempty"`
	Events       []TeamEvent `json:"events,omitempty"`
}

type MailboxMessage struct {
	ID         string    `json:"id"`
	TeamID     string    `json:"team_id"`
	FromSlotID string    `json:"from_slot_id"`
	ToSlotID   string    `json:"to_slot_id"`
	FromName   string    `json:"from_name,omitempty"`
	ToName     string    `json:"to_name,omitempty"`
	Summary    string    `json:"summary,omitempty"`
	Content    string    `json:"content"`
	Broadcast  bool      `json:"broadcast,omitempty"`
	Seq        uint64    `json:"seq"`
	CreatedAt  time.Time `json:"created_at"`
}

type MailboxMessageDraft struct {
	Summary string `json:"summary,omitempty"`
	Content string `json:"content"`
}

type TeamEvent struct {
	Type      TeamEventType     `json:"type"`
	TeamID    string            `json:"team_id"`
	TeamRunID string            `json:"team_run_id,omitempty"`
	SlotID    string            `json:"slot_id,omitempty"`
	Role      Role              `json:"role,omitempty"`
	Status    string            `json:"status,omitempty"`
	MessageID string            `json:"message_id,omitempty"`
	At        time.Time         `json:"at"`
	Payload   map[string]string `json:"payload,omitempty"`
}
