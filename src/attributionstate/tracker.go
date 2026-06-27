package attributionstate

import "strings"

const (
	MessageTypeUser      = "user"
	MessageTypeAssistant = "assistant"

	BlockTypeText       = "text"
	BlockTypeImage      = "image"
	BlockTypeDocument   = "document"
	BlockTypeToolUse    = "tool_use"
	BlockTypeToolResult = "tool_result"
)

type FileState struct {
	ContentHash       string
	ContributionChars int
	MTimeUnix         int64
}

type Snapshot struct {
	MessageID             string
	FileStates            map[string]FileState
	PromptCount           int
	PermissionPromptCount int
	EscapeCount           int
}

type Tracker struct {
	fileStates            map[string]FileState
	snapshots             []Snapshot
	snapshotSequence      int
	promptCount           int
	permissionPromptCount int
	escapeCount           int
}

type Message struct {
	Type          string
	IsSidechain   bool
	Content       string
	ContentBlocks []ContentBlock
}

type ContentBlock struct {
	Type         string
	Text         string
	Name         string
	Input        ToolInput
	MemoryAccess bool
}

type ToolInput struct {
	Path         string
	FilePath     string
	Paths        []string
	MemoryAccess bool
}

func NewTracker() *Tracker {
	return &Tracker{fileStates: map[string]FileState{}}
}

func (t *Tracker) TrackFile(path string, state FileState) {
	t.ensureFileStates()
	t.fileStates[path] = state
}

func (t *Tracker) UpdatePromptStats(promptCount, permissionPromptCount, escapeCount int) {
	t.promptCount = promptCount
	t.permissionPromptCount = permissionPromptCount
	t.escapeCount = escapeCount
}

func (t *Tracker) MakeSnapshot(messageID string) Snapshot {
	snapshot := t.Snapshot()
	snapshot.MessageID = messageID

	t.snapshots = append(t.snapshots, cloneSnapshot(snapshot))
	t.snapshotSequence++

	return cloneSnapshot(snapshot)
}

func (t *Tracker) Snapshot() Snapshot {
	return Snapshot{
		FileStates:            cloneFileStates(t.fileStates),
		PromptCount:           t.promptCount,
		PermissionPromptCount: t.permissionPromptCount,
		EscapeCount:           t.escapeCount,
	}
}

func (t *Tracker) Latest() (Snapshot, bool) {
	if len(t.snapshots) == 0 {
		return Snapshot{}, false
	}
	return cloneSnapshot(t.snapshots[len(t.snapshots)-1]), true
}

func (t *Tracker) SnapshotSequence() int {
	return t.snapshotSequence
}

func CountUserPrompts(messages []Message) int {
	count := 0
	for _, message := range messages {
		if message.Type != MessageTypeUser || message.IsSidechain {
			continue
		}
		if hasUserPromptContent(message) {
			count++
		}
	}
	return count
}

func CountMemoryFileAccess(messages []Message) int {
	count := 0
	for _, message := range messages {
		if message.Type != MessageTypeAssistant || message.IsSidechain {
			continue
		}
		for _, block := range message.ContentBlocks {
			if block.Type != BlockTypeToolUse || !isMemoryAccessTool(block.Name) {
				continue
			}
			if block.MemoryAccess || block.Input.MemoryAccess || inputHasMemoryPath(block.Input) {
				count++
			}
		}
	}
	return count
}

func (t *Tracker) ensureFileStates() {
	if t.fileStates == nil {
		t.fileStates = map[string]FileState{}
	}
}

func cloneSnapshot(snapshot Snapshot) Snapshot {
	snapshot.FileStates = cloneFileStates(snapshot.FileStates)
	return snapshot
}

func cloneFileStates(states map[string]FileState) map[string]FileState {
	clone := make(map[string]FileState, len(states))
	for path, state := range states {
		clone[path] = state
	}
	return clone
}

func hasUserPromptContent(message Message) bool {
	if strings.TrimSpace(message.Content) != "" && !isTerminalOutput(message.Content) {
		return true
	}

	for _, block := range message.ContentBlocks {
		switch block.Type {
		case BlockTypeText:
			if strings.TrimSpace(block.Text) != "" && !isTerminalOutput(block.Text) {
				return true
			}
		case BlockTypeImage, BlockTypeDocument:
			return true
		}
	}

	return false
}

func isTerminalOutput(content string) bool {
	lower := strings.ToLower(content)
	for _, tag := range []string{"bash-output", "bash-input"} {
		if strings.Contains(lower, "<"+tag) {
			return true
		}
	}
	return false
}

func isMemoryAccessTool(name string) bool {
	_, ok := memoryAccessTools[strings.ToLower(strings.TrimSpace(name))]
	return ok
}

func inputHasMemoryPath(input ToolInput) bool {
	if isMemoryPath(input.Path) || isMemoryPath(input.FilePath) {
		return true
	}
	for _, path := range input.Paths {
		if isMemoryPath(path) {
			return true
		}
	}
	return false
}

func isMemoryPath(path string) bool {
	lower := strings.ToLower(path)
	return strings.Contains(lower, ".codex/memories") || strings.Contains(lower, "memory.md")
}

var memoryAccessTools = map[string]struct{}{
	"apply_patch": {},
	"edit":        {},
	"file_edit":   {},
	"file_read":   {},
	"file_write":  {},
	"glob":        {},
	"grep":        {},
	"grep_search": {},
	"list_files":  {},
	"multi_edit":  {},
	"read":        {},
	"read_file":   {},
	"write":       {},
	"write_file":  {},
}
