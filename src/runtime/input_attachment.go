package runtime

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	convcontext "awaken/src/context"
)

const (
	maxRunInputAttachments      = 4
	maxRunInputAttachmentBytes  = 8 * 1024 * 1024
	defaultImagePromptThreadLabel = "Image prompt"
)

const runtimeUserDataDirEnv = "AWAKEN_USER_DATA_DIR"

type RunInputAttachmentKind string

const RunInputAttachmentKindImage RunInputAttachmentKind = "image"

type RunInputAttachment struct {
	ID          string                 `json:"id"`
	Kind        RunInputAttachmentKind `json:"kind"`
	MimeType    string                 `json:"mime_type"`
	DataBase64  string                 `json:"data_base64,omitempty"`
	DisplayName string                 `json:"display_name,omitempty"`
	Width       int                    `json:"width,omitempty"`
	Height      int                    `json:"height,omitempty"`
	ByteSize    int                    `json:"byte_size,omitempty"`
	StorageKey  string                 `json:"-"`
}

type transcriptMessageAttachment struct {
	ID          string                 `json:"id"`
	Kind        RunInputAttachmentKind `json:"kind"`
	MimeType    string                 `json:"mime_type"`
	DisplayName string                 `json:"display_name,omitempty"`
	Width       int                    `json:"width,omitempty"`
	Height      int                    `json:"height,omitempty"`
	ByteSize    int                    `json:"byte_size,omitempty"`
	StorageKey  string                 `json:"storage_key,omitempty"`
}

func cloneRunInputAttachments(attachments []RunInputAttachment) []RunInputAttachment {
	if len(attachments) == 0 {
		return nil
	}
	cloned := make([]RunInputAttachment, len(attachments))
	copy(cloned, attachments)
	return cloned
}

func cloneTranscriptAttachments(attachments []transcriptMessageAttachment) []transcriptMessageAttachment {
	if len(attachments) == 0 {
		return nil
	}
	cloned := make([]transcriptMessageAttachment, len(attachments))
	copy(cloned, attachments)
	return cloned
}

func inputAttachmentsToContextAttachments(attachments []RunInputAttachment) []convcontext.MessageAttachment {
	if len(attachments) == 0 {
		return nil
	}
	converted := make([]convcontext.MessageAttachment, 0, len(attachments))
	for _, attachment := range attachments {
		converted = append(converted, convcontext.MessageAttachment{
			ID:          attachment.ID,
			Kind:        string(attachment.Kind),
			MimeType:    attachment.MimeType,
			DataBase64:  attachment.DataBase64,
			DisplayName: attachment.DisplayName,
			Width:       attachment.Width,
			Height:      attachment.Height,
			ByteSize:    attachment.ByteSize,
			StorageKey:  attachment.StorageKey,
		})
	}
	return converted
}

func contextAttachmentsToTranscriptAttachments(attachments []convcontext.MessageAttachment) []transcriptMessageAttachment {
	if len(attachments) == 0 {
		return nil
	}
	converted := make([]transcriptMessageAttachment, 0, len(attachments))
	for _, attachment := range attachments {
		converted = append(converted, transcriptMessageAttachment{
			ID:          attachment.ID,
			Kind:        RunInputAttachmentKind(attachment.Kind),
			MimeType:    attachment.MimeType,
			DisplayName: attachment.DisplayName,
			Width:       attachment.Width,
			Height:      attachment.Height,
			ByteSize:    attachment.ByteSize,
			StorageKey:  attachment.StorageKey,
		})
	}
	return converted
}

func transcriptAttachmentsToContextAttachments(attachments []transcriptMessageAttachment) []convcontext.MessageAttachment {
	if len(attachments) == 0 {
		return nil
	}
	converted := make([]convcontext.MessageAttachment, 0, len(attachments))
	for _, attachment := range attachments {
		hydrated, err := hydrateRunInputAttachment(RunInputAttachment{
			ID:          attachment.ID,
			Kind:        attachment.Kind,
			MimeType:    attachment.MimeType,
			DisplayName: attachment.DisplayName,
			Width:       attachment.Width,
			Height:      attachment.Height,
			ByteSize:    attachment.ByteSize,
			StorageKey:  attachment.StorageKey,
		})
		if err != nil {
			continue
		}
		converted = append(converted, convcontext.MessageAttachment{
			ID:          hydrated.ID,
			Kind:        string(hydrated.Kind),
			MimeType:    hydrated.MimeType,
			DataBase64:  hydrated.DataBase64,
			DisplayName: hydrated.DisplayName,
			Width:       hydrated.Width,
			Height:      hydrated.Height,
			ByteSize:    hydrated.ByteSize,
			StorageKey:  hydrated.StorageKey,
		})
	}
	return converted
}

func contextAttachmentsToRunInputAttachments(attachments []convcontext.MessageAttachment) []RunInputAttachment {
	if len(attachments) == 0 {
		return nil
	}
	converted := make([]RunInputAttachment, 0, len(attachments))
	for _, attachment := range attachments {
		converted = append(converted, RunInputAttachment{
			ID:          attachment.ID,
			Kind:        RunInputAttachmentKind(attachment.Kind),
			MimeType:    attachment.MimeType,
			DataBase64:  attachment.DataBase64,
			DisplayName: attachment.DisplayName,
			Width:       attachment.Width,
			Height:      attachment.Height,
			ByteSize:    attachment.ByteSize,
			StorageKey:  attachment.StorageKey,
		})
	}
	return converted
}

func persistRunInputAttachments(threadID string, attachments []RunInputAttachment) ([]RunInputAttachment, error) {
	if len(attachments) == 0 {
		return nil, nil
	}
	persisted := make([]RunInputAttachment, 0, len(attachments))
	for _, attachment := range attachments {
		data, normalized, err := decodeRunInputAttachment(attachment)
		if err != nil {
			return nil, err
		}
		storageKey := filepath.ToSlash(filepath.Join(
			safeTranscriptFileName(threadID),
			fmt.Sprintf("%s.%s", safeTranscriptFileName(normalized.ID), imageFileExtension(normalized.MimeType)),
		))
		targetPath := filepath.Join(runInputAttachmentRootDir(), filepath.FromSlash(storageKey))
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(targetPath, data, 0o600); err != nil {
			return nil, err
		}
		normalized.StorageKey = storageKey
		persisted = append(persisted, normalized)
	}
	return persisted, nil
}

func hydrateRunInputAttachments(attachments []RunInputAttachment) []RunInputAttachment {
	if len(attachments) == 0 {
		return nil
	}
	hydrated := make([]RunInputAttachment, 0, len(attachments))
	for _, attachment := range attachments {
		next, err := hydrateRunInputAttachment(attachment)
		if err != nil {
			continue
		}
		hydrated = append(hydrated, next)
	}
	return hydrated
}

func hydrateRunInputAttachment(attachment RunInputAttachment) (RunInputAttachment, error) {
	if strings.TrimSpace(attachment.DataBase64) != "" || strings.TrimSpace(attachment.StorageKey) == "" {
		return attachment, nil
	}
	targetPath := filepath.Join(runInputAttachmentRootDir(), filepath.FromSlash(attachment.StorageKey))
	data, err := os.ReadFile(targetPath)
	if err != nil {
		return RunInputAttachment{}, err
	}
	attachment.DataBase64 = base64.StdEncoding.EncodeToString(data)
	if attachment.ByteSize <= 0 {
		attachment.ByteSize = len(data)
	}
	return attachment, nil
}

func deleteThreadInputAttachments(threadID string) error {
	if strings.TrimSpace(threadID) == "" {
		return nil
	}
	return os.RemoveAll(filepath.Join(runInputAttachmentRootDir(), safeTranscriptFileName(threadID)))
}

func validateRunInputAttachments(attachments []RunInputAttachment) error {
	if len(attachments) == 0 {
		return nil
	}
	if len(attachments) > maxRunInputAttachments {
		return fmt.Errorf("%w: 最多只能附加 %d 张图片", ErrInvalidInput, maxRunInputAttachments)
	}
	for _, attachment := range attachments {
		if _, _, err := decodeRunInputAttachment(attachment); err != nil {
			return err
		}
	}
	return nil
}

func decodeRunInputAttachment(attachment RunInputAttachment) ([]byte, RunInputAttachment, error) {
	normalized := attachment
	if strings.TrimSpace(normalized.ID) == "" {
		return nil, RunInputAttachment{}, fmt.Errorf("%w: 图片附件缺少 id", ErrInvalidInput)
	}
	if normalized.Kind != RunInputAttachmentKindImage {
		return nil, RunInputAttachment{}, fmt.Errorf("%w: 当前仅支持图片附件", ErrInvalidInput)
	}
	if imageFileExtension(normalized.MimeType) == "" {
		return nil, RunInputAttachment{}, fmt.Errorf("%w: 当前仅支持 png、jpeg、webp 图片", ErrInvalidInput)
	}
	if strings.TrimSpace(normalized.DataBase64) == "" {
		return nil, RunInputAttachment{}, fmt.Errorf("%w: 图片附件缺少内容", ErrInvalidInput)
	}
	decoded, err := base64.StdEncoding.DecodeString(normalized.DataBase64)
	if err != nil {
		return nil, RunInputAttachment{}, fmt.Errorf("%w: 图片附件内容无法解析", ErrInvalidInput)
	}
	if len(decoded) > maxRunInputAttachmentBytes {
		return nil, RunInputAttachment{}, fmt.Errorf("%w: 单张图片不能超过 %d MiB", ErrInvalidInput, maxRunInputAttachmentBytes/(1024*1024))
	}
	normalized.ByteSize = len(decoded)
	if normalized.Width < 0 || normalized.Height < 0 {
		return nil, RunInputAttachment{}, fmt.Errorf("%w: 图片尺寸无效", ErrInvalidInput)
	}
	return decoded, normalized, nil
}

func imageFileExtension(mimeType string) string {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "image/png":
		return "png"
	case "image/jpeg":
		return "jpg"
	case "image/webp":
		return "webp"
	default:
		return ""
	}
}

func runInputAttachmentRootDir() string {
	if userDataDir := strings.TrimSpace(os.Getenv(runtimeUserDataDirEnv)); userDataDir != "" {
		return filepath.Join(userDataDir, "runtime-input-attachments")
	}
	if configDir, err := os.UserConfigDir(); err == nil && strings.TrimSpace(configDir) != "" {
		return filepath.Join(configDir, "awaken", "runtime-input-attachments")
	}
	return filepath.Join(os.TempDir(), "awaken-runtime-input-attachments")
}

func validateVisionSupport(profile *ModelProfileConfig, attachments []RunInputAttachment) error {
	if len(attachments) == 0 {
		return nil
	}
	if profile == nil || !profile.SupportsVision {
		return fmt.Errorf("%w: 当前模型不支持图片输入，请切换模型或移除图片", ErrUnsupportedInputAttachment)
	}
	return nil
}

func threadLabelForInput(input string, attachments []RunInputAttachment) string {
	if trimmed := trimThreadLabel(strings.TrimSpace(input)); trimmed != "" {
		return trimmed
	}
	if len(attachments) > 0 {
		if label := trimThreadLabel(strings.TrimSpace(attachments[0].DisplayName)); label != "" {
			return label
		}
	}
	return defaultImagePromptThreadLabel
}

func isInputRequiredError(err error) bool {
	return errors.Is(err, ErrInvalidInput)
}
