package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"awaken/src/agentcore"
	"awaken/src/filestore"
	"awaken/src/tool"
)

type toolResultOwner struct {
	Version  int                                `json:"version"`
	ThreadID string                             `json:"thread_id"`
	AgentID  string                             `json:"agent_id"`
	TaskID   string                             `json:"task_id,omitempty"`
	Receipt  agentcore.ToolResultOffloadReceipt `json:"receipt"`
}

func persistToolResultOwner(path, threadID string, identity tool.ToolExecutionIdentity, receipt agentcore.ToolResultOffloadReceipt) error {
	agentID := strings.TrimSpace(identity.AgentID)
	if agentID == "" && identity.SubagentTaskID == "" {
		agentID = "root"
	}
	encoded, err := json.Marshal(toolResultOwner{Version: 1, ThreadID: threadID, AgentID: agentID, TaskID: identity.SubagentTaskID, Receipt: receipt})
	if err != nil {
		return err
	}
	if err := filestore.WriteFileAtomic(path+".owner.json", encoded); err != nil {
		return fmt.Errorf("persist tool result ownership: %w", err)
	}
	return os.Chmod(path+".owner.json", 0o600)
}

func (s *ToolResultStore) ReadToolResult(ctx context.Context, threadID, agentID string, request tool.ToolResultReadRequest) (tool.ToolResultPage, error) {
	fail := func(code, message string) (tool.ToolResultPage, error) {
		return tool.ToolResultPage{}, &tool.ToolResultReadError{Code: code, Message: message}
	}
	if err := ctx.Err(); err != nil {
		return tool.ToolResultPage{}, err
	}
	if request.Limit == 0 {
		request.Limit = tool.ToolResultReadMaxChars
	}
	if request.Offset < 0 || request.Limit < 1 || request.Limit > tool.ToolResultReadMaxChars {
		return fail("tool_result_invalid_input", "offset must be nonnegative and limit must be 1-2048 characters.")
	}
	key := filepath.ToSlash(filepath.Clean(filepath.FromSlash(request.StorageKey)))
	if threadID == "" || agentID == "" || key != request.StorageKey || !strings.HasPrefix(key, safeTranscriptFileName(threadID)+"/tool-results/") || !strings.HasSuffix(key, "/content.txt") {
		return fail("tool_result_not_owned", "The result is not owned by this thread and Agent.")
	}
	path, err := ownedToolResultPath(key)
	if err != nil {
		return fail("tool_result_not_owned", "Invalid result storage key.")
	}
	data, err := os.ReadFile(path + ".owner.json")
	if err != nil {
		return fail("tool_result_unavailable", "Result ownership metadata is unavailable; legacy results without metadata cannot be read through this entry.")
	}
	var owner toolResultOwner
	if json.Unmarshal(data, &owner) != nil || owner.Version != 1 || owner.Receipt.StorageKey != key || owner.Receipt.Digest == "" {
		return fail("tool_result_corrupt", "Result ownership metadata is invalid.")
	}
	if owner.ThreadID != threadID || owner.AgentID != agentID {
		return fail("tool_result_not_owned", "The result is not owned by this thread and Agent.")
	}
	content, err := s.HydrateOffloadedToolResult(ctx, threadID, owner.Receipt)
	if err != nil {
		return fail("tool_result_corrupt", "Stored result failed integrity verification or is unavailable.")
	}
	if err := ctx.Err(); err != nil {
		return tool.ToolResultPage{}, err
	}
	chars := []rune(content)
	if request.Offset > len(chars) {
		return fail("tool_result_invalid_input", "offset exceeds the complete result length.")
	}
	end := min(request.Offset+request.Limit, len(chars))
	return tool.ToolResultPage{Content: string(chars[request.Offset:end]), Offset: request.Offset, NextOffset: end, TotalChars: len(chars), ByteSize: len(content), SHA256: owner.Receipt.Digest, HasMore: end < len(chars)}, nil
}
