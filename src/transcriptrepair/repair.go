package transcriptrepair

import (
	"fmt"
	"strings"
)

type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

type BlockType string

const (
	BlockText       BlockType = "text"
	BlockToolUse    BlockType = "tool_use"
	BlockToolResult BlockType = "tool_result"
)

const (
	SyntheticToolResultPlaceholder = "[Synthetic error tool_result inserted because the original tool_result was missing after transcript repair.]"
	OrphanToolResultPlaceholder    = "[Orphaned tool_result removed after transcript repair.]"
	AssistantContentPlaceholder    = "[Tool use removed after transcript repair.]"
)

type Message struct {
	Role    Role           `json:"role"`
	Content []ContentBlock `json:"content"`
}

type ContentBlock struct {
	Type      BlockType `json:"type"`
	Text      string    `json:"text,omitempty"`
	ID        string    `json:"id,omitempty"`
	Name      string    `json:"name,omitempty"`
	Input     any       `json:"input,omitempty"`
	ToolUseID string    `json:"tool_use_id,omitempty"`
	Content   any       `json:"content,omitempty"`
	IsError   bool      `json:"is_error,omitempty"`
}

type Options struct {
	Strict bool
}

type RepairOptions = Options

type Report struct {
	Strict                     bool
	InsertedSynthetic          int
	RemovedOrphan              int
	RemovedDuplicateToolUse    int
	RemovedDuplicateToolResult int
	Events                     []RepairEvent
}

type RepairReport = Report

type RepairEvent struct {
	Kind         string
	MessageIndex int
	ToolUseID    string
	Role         Role
	Detail       string
}

func (r Report) HasRepairs() bool {
	return r.InsertedSynthetic > 0 ||
		r.RemovedOrphan > 0 ||
		r.RemovedDuplicateToolUse > 0 ||
		r.RemovedDuplicateToolResult > 0 ||
		len(r.Events) > 0
}

type StrictPairingError struct {
	Report Report
}

func (e *StrictPairingError) Error() string {
	return fmt.Sprintf(
		"transcriptrepair: tool_use/tool_result mismatch detected in strict mode; refusing repair (%s)",
		e.Report.summary(),
	)
}

func EnsureToolResultPairing(messages []Message, opts Options) ([]Message, Report, error) {
	report := Report{Strict: opts.Strict}
	original := cloneMessages(messages)
	repaired := make([]Message, 0, len(messages))
	seenToolUseIDs := map[string]struct{}{}

	for i := 0; i < len(messages); i++ {
		msg := cloneMessage(messages[i])

		switch msg.Role {
		case RoleAssistant:
			assistant, expectedIDs := repairAssistant(i, msg, seenToolUseIDs, &report)
			repaired = append(repaired, assistant)

			if i+1 < len(messages) && messages[i+1].Role == RoleUser && (len(expectedIDs) > 0 || hasToolResult(messages[i+1])) {
				user := repairUser(i+1, cloneMessage(messages[i+1]), expectedIDs, &report)
				repaired = append(repaired, user)
				i++
				continue
			}

			if len(expectedIDs) > 0 {
				repaired = append(repaired, syntheticUserMessage(i, expectedIDs, &report))
			}

		case RoleUser:
			if hasToolResult(msg) {
				msg = repairUser(i, msg, nil, &report)
			}
			repaired = append(repaired, msg)

		default:
			repaired = append(repaired, msg)
		}
	}

	if opts.Strict && report.HasRepairs() {
		return original, report, &StrictPairingError{Report: report}
	}

	return repaired, report, nil
}

func repairAssistant(index int, msg Message, seenToolUseIDs map[string]struct{}, report *Report) (Message, []string) {
	content := make([]ContentBlock, 0, len(msg.Content))
	expectedIDs := make([]string, 0)
	changed := false

	for _, block := range msg.Content {
		if block.Type != BlockToolUse {
			content = append(content, block)
			continue
		}

		if _, ok := seenToolUseIDs[block.ID]; ok {
			report.RemovedDuplicateToolUse++
			report.addEvent("removed_duplicate_tool_use", index, RoleAssistant, block.ID, "duplicate tool_use id removed")
			changed = true
			continue
		}

		seenToolUseIDs[block.ID] = struct{}{}
		expectedIDs = append(expectedIDs, block.ID)
		content = append(content, block)
	}

	if changed && len(content) == 0 {
		content = append(content, ContentBlock{
			Type: BlockText,
			Text: AssistantContentPlaceholder,
		})
		report.addEvent("inserted_assistant_placeholder", index, RoleAssistant, "", "assistant content would otherwise be empty")
	}

	msg.Content = content
	return msg, expectedIDs
}

func repairUser(index int, msg Message, expectedIDs []string, report *Report) Message {
	expected := make(map[string]struct{}, len(expectedIDs))
	for _, id := range expectedIDs {
		expected[id] = struct{}{}
	}

	seenResults := map[string]struct{}{}
	content := make([]ContentBlock, 0, len(msg.Content))

	for _, block := range msg.Content {
		if block.Type != BlockToolResult {
			content = append(content, block)
			continue
		}

		id := block.ToolUseID
		if _, ok := expected[id]; !ok {
			report.RemovedOrphan++
			report.addEvent("removed_orphan_tool_result", index, RoleUser, id, "tool_result had no adjacent assistant tool_use")
			continue
		}

		if _, ok := seenResults[id]; ok {
			report.RemovedDuplicateToolResult++
			report.addEvent("removed_duplicate_tool_result", index, RoleUser, id, "duplicate tool_result id removed")
			continue
		}

		seenResults[id] = struct{}{}
		content = append(content, block)
	}

	missing := make([]string, 0)
	for _, id := range expectedIDs {
		if _, ok := seenResults[id]; !ok {
			missing = append(missing, id)
		}
	}

	if len(missing) > 0 {
		synthetic := syntheticBlocks(index, missing, report)
		content = append(synthetic, content...)
	}

	if len(content) == 0 {
		content = append(content, ContentBlock{
			Type: BlockText,
			Text: OrphanToolResultPlaceholder,
		})
		report.addEvent("inserted_user_placeholder", index, RoleUser, "", "user content would otherwise be empty")
	}

	msg.Content = content
	return msg
}

func syntheticUserMessage(afterIndex int, ids []string, report *Report) Message {
	return Message{
		Role:    RoleUser,
		Content: syntheticBlocks(afterIndex, ids, report),
	}
}

func syntheticBlocks(index int, ids []string, report *Report) []ContentBlock {
	blocks := make([]ContentBlock, 0, len(ids))
	for _, id := range ids {
		report.InsertedSynthetic++
		report.addEvent("inserted_synthetic_tool_result", index, RoleUser, id, "missing tool_result repaired with synthetic error")
		blocks = append(blocks, ContentBlock{
			Type:      BlockToolResult,
			ToolUseID: id,
			Content:   SyntheticToolResultPlaceholder,
			IsError:   true,
		})
	}
	return blocks
}

func hasToolResult(msg Message) bool {
	for _, block := range msg.Content {
		if block.Type == BlockToolResult {
			return true
		}
	}
	return false
}

func cloneMessages(messages []Message) []Message {
	out := make([]Message, len(messages))
	for i := range messages {
		out[i] = cloneMessage(messages[i])
	}
	return out
}

func cloneMessage(msg Message) Message {
	if msg.Content == nil {
		return msg
	}
	msg.Content = append([]ContentBlock(nil), msg.Content...)
	return msg
}

func (r *Report) addEvent(kind string, index int, role Role, toolUseID, detail string) {
	r.Events = append(r.Events, RepairEvent{
		Kind:         kind,
		MessageIndex: index,
		ToolUseID:    toolUseID,
		Role:         role,
		Detail:       detail,
	})
}

func (r Report) summary() string {
	parts := []string{}
	if r.InsertedSynthetic > 0 {
		parts = append(parts, fmt.Sprintf("insertedSynthetic=%d", r.InsertedSynthetic))
	}
	if r.RemovedOrphan > 0 {
		parts = append(parts, fmt.Sprintf("removedOrphan=%d", r.RemovedOrphan))
	}
	if r.RemovedDuplicateToolUse > 0 {
		parts = append(parts, fmt.Sprintf("removedDuplicateToolUse=%d", r.RemovedDuplicateToolUse))
	}
	if r.RemovedDuplicateToolResult > 0 {
		parts = append(parts, fmt.Sprintf("removedDuplicateToolResult=%d", r.RemovedDuplicateToolResult))
	}
	if len(parts) == 0 {
		return "no mismatches"
	}
	return strings.Join(parts, ", ")
}
