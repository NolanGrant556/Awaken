package convrecovery

import (
	"path/filepath"
	"strings"
)

var defaultPermissionModes = []string{
	"acceptEdits",
	"bypassPermissions",
	"default",
	"dontAsk",
	"plan",
	"auto",
}

var defaultTerminalToolNames = []string{
	"SendUserMessage",
	"Brief",
	"SendUserFile",
}

func Recover(messages []Message, opts Options) Result {
	recovered := cloneMessages(messages)
	recovered = migrateLegacyAttachments(recovered, opts.CWD)
	clearInvalidPermissionModes(recovered, permissionModeSet(opts.ValidPermissionModes))
	recovered = filterUnresolvedToolPairing(recovered)
	recovered = filterOrphanedThinkingOnlyAssistantMessages(recovered)
	recovered = filterWhitespaceOnlyAssistantMessages(recovered)

	state := detectTurnInterruption(recovered, terminalToolNameSet(opts.TerminalToolNames))
	continuationAdded := false
	if state == TurnInterruptionInterruptedTurn {
		recovered = append(recovered, Message{
			Type:          MessageTypeUser,
			IsMeta:        true,
			ContentBlocks: []Block{{Type: BlockTypeText, Text: ContinuationPrompt}},
		})
		continuationAdded = true
	}

	sentinelAdded := false
	if idx := lastSentinelRelevantIndex(recovered); idx >= 0 && recovered[idx].Type == MessageTypeUser {
		recovered = insertMessage(recovered, idx+1, Message{
			Type:          MessageTypeAssistant,
			ContentBlocks: []Block{{Type: BlockTypeText, Text: NoResponseRequested}},
		})
		sentinelAdded = true
	}

	return Result{
		Messages:              recovered,
		TurnInterruptionState: state,
		ContinuationAdded:     continuationAdded,
		SentinelAdded:         sentinelAdded,
	}
}

func cloneMessages(messages []Message) []Message {
	cloned := make([]Message, len(messages))
	for i, message := range messages {
		cloned[i] = message
		if message.ContentBlocks != nil {
			cloned[i].ContentBlocks = append([]Block(nil), message.ContentBlocks...)
		}
	}
	return cloned
}

func migrateLegacyAttachments(messages []Message, cwd string) []Message {
	for i := range messages {
		if messages[i].Type != MessageTypeAttachment {
			continue
		}

		switch messages[i].AttachmentType {
		case "new_file":
			messages[i].AttachmentType = "file"
			if messages[i].DisplayPath == "" {
				messages[i].DisplayPath = relativeDisplayPath(cwd, firstNonEmpty(messages[i].AttachmentFilename, messages[i].AttachmentPath))
			}
		case "new_directory":
			messages[i].AttachmentType = "directory"
			if messages[i].DisplayPath == "" {
				messages[i].DisplayPath = relativeDisplayPath(cwd, firstNonEmpty(messages[i].AttachmentPath, messages[i].AttachmentFilename))
			}
		default:
			if messages[i].DisplayPath == "" {
				messages[i].DisplayPath = relativeDisplayPath(cwd, firstNonEmpty(messages[i].AttachmentPath, messages[i].AttachmentFilename))
			}
		}
	}
	return messages
}

func relativeDisplayPath(cwd, path string) string {
	if path == "" {
		return ""
	}
	if cwd == "" {
		return filepath.Clean(path)
	}
	if filepath.IsAbs(cwd) != filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	rel, err := filepath.Rel(cwd, path)
	if err != nil {
		return filepath.Clean(path)
	}
	return rel
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func clearInvalidPermissionModes(messages []Message, validModes map[string]struct{}) {
	for i := range messages {
		if messages[i].Type != MessageTypeUser || messages[i].PermissionMode == "" {
			continue
		}
		if _, ok := validModes[messages[i].PermissionMode]; !ok {
			messages[i].PermissionMode = ""
		}
	}
}

func permissionModeSet(modes []string) map[string]struct{} {
	if len(modes) == 0 {
		modes = defaultPermissionModes
	}
	return stringSet(modes)
}

func terminalToolNameSet(names []string) map[string]struct{} {
	if len(names) == 0 {
		names = defaultTerminalToolNames
	}
	return stringSet(names)
}

func stringSet(values []string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value != "" {
			result[value] = struct{}{}
		}
	}
	return result
}

func filterUnresolvedToolPairing(messages []Message) []Message {
	toolUseIDs := map[string]struct{}{}
	toolResultIDs := map[string]struct{}{}

	for _, message := range messages {
		if message.Type != MessageTypeAssistant && message.Type != MessageTypeUser {
			continue
		}
		for _, block := range message.ContentBlocks {
			switch block.Type {
			case BlockTypeToolUse:
				if block.ID != "" {
					toolUseIDs[block.ID] = struct{}{}
				}
			case BlockTypeToolResult:
				if block.ToolUseID != "" {
					toolResultIDs[block.ToolUseID] = struct{}{}
				}
			}
		}
	}

	resolvedToolUseIDs := map[string]struct{}{}
	for id := range toolUseIDs {
		if _, ok := toolResultIDs[id]; ok {
			resolvedToolUseIDs[id] = struct{}{}
		}
	}

	filtered := make([]Message, 0, len(messages))
	for _, message := range messages {
		if message.Type != MessageTypeAssistant {
			filtered = append(filtered, message)
			continue
		}

		blocks := make([]Block, 0, len(message.ContentBlocks))
		removedToolUse := false
		for _, block := range message.ContentBlocks {
			if block.Type != BlockTypeToolUse {
				blocks = append(blocks, block)
				continue
			}
			if _, ok := resolvedToolUseIDs[block.ID]; ok {
				blocks = append(blocks, block)
				continue
			}
			removedToolUse = true
		}
		if removedToolUse {
			if len(blocks) == 0 {
				continue
			}
			message.ContentBlocks = blocks
		}
		filtered = append(filtered, message)
	}

	remainingToolUseIDs := map[string]struct{}{}
	for _, message := range filtered {
		if message.Type != MessageTypeAssistant {
			continue
		}
		for _, block := range message.ContentBlocks {
			if block.Type == BlockTypeToolUse && block.ID != "" {
				remainingToolUseIDs[block.ID] = struct{}{}
			}
		}
	}

	paired := make([]Message, 0, len(filtered))
	for _, message := range filtered {
		if message.Type != MessageTypeUser {
			paired = append(paired, message)
			continue
		}

		blocks := make([]Block, 0, len(message.ContentBlocks))
		removedToolResult := false
		for _, block := range message.ContentBlocks {
			if block.Type != BlockTypeToolResult {
				blocks = append(blocks, block)
				continue
			}
			if _, ok := remainingToolUseIDs[block.ToolUseID]; ok {
				blocks = append(blocks, block)
				continue
			}
			removedToolResult = true
		}
		if removedToolResult {
			if len(blocks) == 0 {
				continue
			}
			message.ContentBlocks = blocks
		}
		paired = append(paired, message)
	}

	return paired
}

func filterOrphanedThinkingOnlyAssistantMessages(messages []Message) []Message {
	idsWithNonThinking := map[string]struct{}{}
	for _, message := range messages {
		if message.Type != MessageTypeAssistant || message.ID == "" {
			continue
		}
		if hasNonThinkingBlock(message.ContentBlocks) {
			idsWithNonThinking[message.ID] = struct{}{}
		}
	}

	filtered := make([]Message, 0, len(messages))
	for _, message := range messages {
		if message.Type != MessageTypeAssistant || len(message.ContentBlocks) == 0 || !allThinkingBlocks(message.ContentBlocks) {
			filtered = append(filtered, message)
			continue
		}
		if _, ok := idsWithNonThinking[message.ID]; ok {
			filtered = append(filtered, message)
		}
	}
	return filtered
}

func hasNonThinkingBlock(blocks []Block) bool {
	for _, block := range blocks {
		if !isThinkingBlock(block) {
			return true
		}
	}
	return false
}

func allThinkingBlocks(blocks []Block) bool {
	for _, block := range blocks {
		if !isThinkingBlock(block) {
			return false
		}
	}
	return len(blocks) > 0
}

func isThinkingBlock(block Block) bool {
	return block.Type == BlockTypeThinking || block.Type == BlockTypeRedactedThinking
}

func filterWhitespaceOnlyAssistantMessages(messages []Message) []Message {
	filtered := make([]Message, 0, len(messages))
	for _, message := range messages {
		if message.Type == MessageTypeAssistant && hasOnlyWhitespaceTextContent(message.ContentBlocks) {
			continue
		}
		filtered = append(filtered, message)
	}
	return filtered
}

func hasOnlyWhitespaceTextContent(blocks []Block) bool {
	if len(blocks) == 0 {
		return false
	}
	for _, block := range blocks {
		if block.Type != BlockTypeText {
			return false
		}
		if strings.TrimSpace(block.Text) != "" {
			return false
		}
	}
	return true
}

func detectTurnInterruption(messages []Message, terminalToolNames map[string]struct{}) TurnInterruptionState {
	idx := lastTurnRelevantIndex(messages)
	if idx < 0 {
		return TurnInterruptionNone
	}

	lastMessage := messages[idx]
	switch lastMessage.Type {
	case MessageTypeAssistant:
		return TurnInterruptionNone
	case MessageTypeUser:
		if lastMessage.IsMeta || lastMessage.IsCompactSummary {
			return TurnInterruptionNone
		}
		if isToolUseResultMessage(lastMessage) {
			if isTerminalToolResult(lastMessage, messages, idx, terminalToolNames) {
				return TurnInterruptionNone
			}
			return TurnInterruptionInterruptedTurn
		}
		return TurnInterruptionInterruptedPrompt
	case MessageTypeAttachment:
		return TurnInterruptionInterruptedTurn
	default:
		return TurnInterruptionNone
	}
}

func lastTurnRelevantIndex(messages []Message) int {
	for i := len(messages) - 1; i >= 0; i-- {
		message := messages[i]
		if message.Type == MessageTypeSystem || message.Type == MessageTypeProgress {
			continue
		}
		if message.Type == MessageTypeAssistant && message.IsAPIError {
			continue
		}
		return i
	}
	return -1
}

func lastSentinelRelevantIndex(messages []Message) int {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Type == MessageTypeSystem || messages[i].Type == MessageTypeProgress {
			continue
		}
		return i
	}
	return -1
}

func isToolUseResultMessage(message Message) bool {
	return message.Type == MessageTypeUser &&
		len(message.ContentBlocks) > 0 &&
		message.ContentBlocks[0].Type == BlockTypeToolResult
}

func isTerminalToolResult(result Message, messages []Message, resultIdx int, terminalToolNames map[string]struct{}) bool {
	toolUseID := result.ContentBlocks[0].ToolUseID
	if toolUseID == "" {
		return false
	}
	for i := resultIdx - 1; i >= 0; i-- {
		message := messages[i]
		if message.Type != MessageTypeAssistant {
			continue
		}
		for _, block := range message.ContentBlocks {
			if block.Type != BlockTypeToolUse || block.ID != toolUseID {
				continue
			}
			_, ok := terminalToolNames[block.Name]
			return ok
		}
	}
	return false
}

func insertMessage(messages []Message, index int, message Message) []Message {
	messages = append(messages, Message{})
	copy(messages[index+1:], messages[index:])
	messages[index] = message
	return messages
}
