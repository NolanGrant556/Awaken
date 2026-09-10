package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"awaken/src/agentcore"
	"awaken/src/filestore"
	"awaken/src/tool"
)

const maxToolResultBlobBytes = 20 * 1024 * 1024
const maxOffloadedToolResultBytes = 64 * 1024 * 1024

// ToolResultStore owns model-facing media beneath sessions/<thread>/tool-results.
// Storage keys are relative to the active sessions directory.
type ToolResultStore struct{}

func NewToolResultStore() *ToolResultStore { return &ToolResultStore{} }

func (s *ToolResultStore) OffloadToolResult(
	_ context.Context,
	threadID string,
	identity tool.ToolExecutionIdentity,
	content string,
) (agentcore.ToolResultOffloadReceipt, error) {
	if strings.TrimSpace(threadID) == "" || strings.TrimSpace(identity.ToolCallID) == "" {
		return agentcore.ToolResultOffloadReceipt{}, fmt.Errorf("tool result offload requires thread and tool call identity")
	}
	storageKey := filepath.ToSlash(filepath.Join(
		safeTranscriptFileName(threadID), "tool-results", toolResultIdentityKey(identity), "content.txt",
	))
	targetPath, err := ownedToolResultPath(storageKey)
	if err != nil {
		return agentcore.ToolResultOffloadReceipt{}, err
	}
	if err := filestore.EnsureDir(filepath.Dir(targetPath)); err != nil {
		return agentcore.ToolResultOffloadReceipt{}, err
	}
	if err := os.Chmod(filepath.Dir(targetPath), filestore.DirPerm); err != nil {
		return agentcore.ToolResultOffloadReceipt{}, err
	}
	data := []byte(content)
	if err := filestore.WriteFileAtomic(targetPath, data); err != nil {
		return agentcore.ToolResultOffloadReceipt{}, err
	}
	if err := os.Chmod(targetPath, 0o600); err != nil {
		return agentcore.ToolResultOffloadReceipt{}, err
	}
	digest := sha256.Sum256(data)
	preview := strings.TrimSpace(content)
	previewBytes := []byte(preview)
	if len(previewBytes) > 512 {
		previewBytes = previewBytes[:512]
		for len(previewBytes) > 0 && !utf8.Valid(previewBytes) {
			previewBytes = previewBytes[:len(previewBytes)-1]
		}
		preview = string(previewBytes) + "…"
	}
	receipt := agentcore.ToolResultOffloadReceipt{
		StorageKey: storageKey, Digest: hex.EncodeToString(digest[:]), ByteSize: len(data), Preview: preview,
	}
	if err := persistToolResultOwner(targetPath, threadID, identity, receipt); err != nil {
		return agentcore.ToolResultOffloadReceipt{}, err
	}
	return receipt, nil
}

func (s *ToolResultStore) HydrateOffloadedToolResult(
	_ context.Context,
	threadID string,
	receipt agentcore.ToolResultOffloadReceipt,
) (string, error) {
	key := filepath.ToSlash(filepath.Clean(filepath.FromSlash(strings.TrimSpace(receipt.StorageKey))))
	threadPrefix := safeTranscriptFileName(threadID) + "/tool-results/"
	if strings.TrimSpace(threadID) == "" || !strings.HasPrefix(key, threadPrefix) {
		return "", errors.New("offloaded tool result is not owned by this thread")
	}
	path, err := ownedToolResultPath(key)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("stat offloaded tool result: %w", err)
	}
	if info.Size() > maxOffloadedToolResultBytes {
		return "", fmt.Errorf("offloaded tool result exceeds %d bytes", maxOffloadedToolResultBytes)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read offloaded tool result: %w", err)
	}
	if receipt.ByteSize > 0 && len(data) != receipt.ByteSize {
		return "", fmt.Errorf("offloaded tool result byte size mismatch: got %d want %d", len(data), receipt.ByteSize)
	}
	if digest := strings.TrimSpace(receipt.Digest); digest != "" {
		actual := sha256.Sum256(data)
		if !strings.EqualFold(hex.EncodeToString(actual[:]), digest) {
			return "", errors.New("offloaded tool result digest mismatch")
		}
	}
	return string(data), nil
}

func (s *ToolResultStore) PersistToolResult(
	_ context.Context,
	threadID string,
	identity tool.ToolExecutionIdentity,
	parts []tool.ModelContentPart,
) ([]tool.ModelContentPart, error) {
	if len(parts) == 0 {
		return nil, nil
	}
	if strings.TrimSpace(threadID) == "" || strings.TrimSpace(identity.ToolCallID) == "" {
		return nil, fmt.Errorf("tool media persistence requires thread and tool call identity")
	}
	callKey := filepath.ToSlash(filepath.Join(safeTranscriptFileName(threadID), "tool-results", toolResultIdentityKey(identity)))
	callRoot, err := ownedToolResultPath(callKey)
	if err != nil {
		return nil, err
	}
	if err := os.RemoveAll(callRoot); err != nil {
		return nil, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = os.RemoveAll(callRoot)
		}
	}()
	result := tool.CloneModelContent(parts)
	for i := range result {
		data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(result[i].DataBase64))
		if err != nil || len(data) == 0 {
			return nil, fmt.Errorf("tool media part %d has invalid content", i+1)
		}
		if len(data) > maxToolResultBlobBytes {
			return nil, fmt.Errorf("tool media part %d exceeds %d bytes", i+1, maxToolResultBlobBytes)
		}
		extension := toolResultExtension(result[i].MimeType)
		if extension == "" {
			return nil, fmt.Errorf("tool media part %d has unsupported mime type %q", i+1, result[i].MimeType)
		}
		storageKey := filepath.ToSlash(filepath.Join(
			callKey,
			fmt.Sprintf("part-%03d%s", i+1, extension),
		))
		targetPath, err := ownedToolResultPath(storageKey)
		if err != nil {
			return nil, err
		}
		if err := filestore.EnsureDir(filepath.Dir(targetPath)); err != nil {
			return nil, err
		}
		if err := os.Chmod(filepath.Dir(targetPath), filestore.DirPerm); err != nil {
			return nil, err
		}
		if err := os.WriteFile(targetPath, data, 0o600); err != nil {
			return nil, err
		}
		result[i].StorageKey = storageKey
		result[i].ByteSize = int64(len(data))
	}
	committed = true
	return result, nil
}

func toolResultIdentityKey(identity tool.ToolExecutionIdentity) string {
	agentScope := strings.TrimSpace(identity.SubagentTaskID)
	if agentScope == "" {
		agentScope = strings.TrimSpace(identity.AgentID)
	}
	if agentScope == "" {
		agentScope = "root"
	}
	parts := []string{safeTranscriptFileName(agentScope)}
	if runID := strings.TrimSpace(identity.RunID); runID != "" {
		parts = append(parts, safeTranscriptFileName(runID))
	}
	if turnID := strings.TrimSpace(identity.TurnID); turnID != "" {
		parts = append(parts, safeTranscriptFileName(turnID))
	}
	parts = append(parts, safeTranscriptFileName(identity.ToolCallID))
	return filepath.Join(parts...)
}

func (s *ToolResultStore) HydrateToolResult(
	_ context.Context,
	threadID string,
	parts []tool.ModelContentPart,
) ([]tool.ModelContentPart, []tool.OutcomeNotice) {
	result := tool.CloneModelContent(parts)
	var warnings []tool.OutcomeNotice
	for i := range result {
		if strings.TrimSpace(result[i].DataBase64) != "" || strings.TrimSpace(result[i].StorageKey) == "" {
			continue
		}
		key := filepath.ToSlash(filepath.Clean(filepath.FromSlash(result[i].StorageKey)))
		threadPrefix := safeTranscriptFileName(threadID) + "/tool-results/"
		if threadID == "" || !strings.HasPrefix(key, threadPrefix) {
			warnings = append(warnings, tool.OutcomeNotice{Code: "tool_media_blob_missing", Message: "Tool media storage key is not owned by this thread."})
			continue
		}
		path, err := ownedToolResultPath(key)
		if err != nil {
			warnings = append(warnings, tool.OutcomeNotice{Code: "tool_media_blob_missing", Message: "Tool media storage key is invalid."})
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			warnings = append(warnings, tool.OutcomeNotice{Code: "tool_media_blob_missing", Message: fmt.Sprintf("Tool media blob %q is unavailable.", result[i].DisplayName)})
			continue
		}
		result[i].DataBase64 = base64.StdEncoding.EncodeToString(data)
		result[i].ByteSize = int64(len(data))
	}
	return result, warnings
}

func ownedToolResultPath(storageKey string) (string, error) {
	root, err := filepath.Abs(activeSessionsDir())
	if err != nil {
		return "", err
	}
	target, err := filepath.Abs(filepath.Join(root, filepath.FromSlash(storageKey)))
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(root, target)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("tool media storage key escapes sessions directory")
	}
	return target, nil
}

func toolResultExtension(mimeType string) string {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "application/pdf":
		return ".pdf"
	default:
		return ""
	}
}

func deleteThreadToolResults(threadID string) error {
	if strings.TrimSpace(threadID) == "" {
		return nil
	}
	return os.RemoveAll(filepath.Join(activeSessionsDir(), safeTranscriptFileName(threadID), "tool-results"))
}
