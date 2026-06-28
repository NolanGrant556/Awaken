package memoryextract

import "context"

type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
	RoleSystem    Role = "system"
)

type TranscriptMessage struct {
	ID        string
	Role      Role
	Content   string
	Hidden    bool
	ToolCalls []ToolCall
}

func (m TranscriptMessage) ModelVisible() bool {
	return !m.Hidden && (m.Role == RoleUser || m.Role == RoleAssistant)
}

type ToolCall struct {
	Name         string
	FilePath     string
	WritesMemory bool
}

type CursorState struct {
	LastMessageID string
	Fingerprints  []string
}

type Category string

const (
	CategoryPreference Category = "preference"
	CategoryProject    Category = "project_fact"
	CategoryCommand    Category = "command"
	CategoryPath       Category = "path"
)

type SourceRef struct {
	MessageID string
	Role      Role
	Start     int
	End       int
}

type CandidateMemory struct {
	ID         string
	Text       string
	Category   Category
	Confidence float64
	SourceRefs []SourceRef
}

type ExtractionInput struct {
	Messages []TranscriptMessage
	State    CursorState
}

type Extractor interface {
	Extract(context.Context, ExtractionInput) ([]CandidateMemory, error)
}

type Sink interface {
	WriteMemories(context.Context, []CandidateMemory) ([]WriteReceipt, error)
}

type WriteReceipt struct {
	CandidateID string
	Location    string
}

type SecretScanner interface {
	Scan(string) []SecretMatch
}

type SecretMatch struct {
	RuleID string
	Label  string
}

type SkipReason string

const (
	SkipNone              SkipReason = ""
	SkipNoVisibleMessages SkipReason = "no_visible_messages"
	SkipManualMemoryWrite SkipReason = "manual_memory_write"
)

type FilteredCandidate struct {
	Candidate     CandidateMemory
	Reason        string
	SecretMatches []SecretMatch
}

type Result struct {
	State               CursorState
	CursorFound         bool
	VisibleMessageCount int
	Candidates          []CandidateMemory
	Filtered            []FilteredCandidate
	Receipts            []WriteReceipt
	SkippedReason       SkipReason
}

func (r Result) Skipped() bool {
	return r.SkippedReason != SkipNone
}
