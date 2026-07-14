package runtime

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	convcontext "awaken/src/context"
	"awaken/src/filestore"
)

const (
	maxRunInputAttachmentBytes        = 8 * 1024 * 1024
	defaultInputAttachmentThreadLabel = "附件任务"
)

const runtimeUserDataDirEnv = "AWAKEN_USER_DATA_DIR"

type RunInputAttachmentKind string

const (
	RunInputAttachmentKindImage RunInputAttachmentKind = "image"
	RunInputAttachmentKindFile  RunInputAttachmentKind = "file"
)

type RunInputAttachment struct {
	ID          string                 `json:"id"`
	Kind        RunInputAttachmentKind `json:"kind"`
	MimeType    string                 `json:"mime_type"`
	Path        string                 `json:"path,omitempty"`
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
	Path        string                 `json:"path,omitempty"`
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
			Path:        attachment.Path,
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
			Path:        attachment.Path,
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
			Path:        attachment.Path,
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
			Path:        hydrated.Path,
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
			Path:        attachment.Path,
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

func transcriptAttachmentsToRunInputAttachments(attachments []transcriptMessageAttachment) []RunInputAttachment {
	if len(attachments) == 0 {
		return nil
	}
	converted := make([]RunInputAttachment, 0, len(attachments))
	for _, attachment := range attachments {
		next := RunInputAttachment{
			ID:          attachment.ID,
			Kind:        attachment.Kind,
			MimeType:    attachment.MimeType,
			Path:        attachment.Path,
			DisplayName: attachment.DisplayName,
			Width:       attachment.Width,
			Height:      attachment.Height,
			ByteSize:    attachment.ByteSize,
			StorageKey:  attachment.StorageKey,
		}
		if hydrated, err := hydrateRunInputAttachment(next); err == nil {
			next = hydrated
		}
		// The durable journal owns attachment identity. A missing image blob
		// prevents model hydration, but must not erase the historical Desktop
		// card or cause recovery to infer a replacement from disk.
		converted = append(converted, next)
	}
	return converted
}

func persistRunInputAttachments(threadID string, attachments []RunInputAttachment) ([]RunInputAttachment, error) {
	if len(attachments) == 0 {
		return nil, nil
	}
	persisted := make([]RunInputAttachment, 0, len(attachments))
	for _, attachment := range attachments {
		normalized, data, shouldPersist, err := normalizeRunInputAttachment(attachment)
		if err != nil {
			return nil, err
		}
		if shouldPersist {
			storageKey := filepath.ToSlash(filepath.Join(
				safeTranscriptFileName(threadID),
				"attachments",
				fmt.Sprintf("%s.%s", safeTranscriptFileName(normalized.ID), imageFileExtension(normalized.MimeType)),
			))
			targetPath := filepath.Join(runInputAttachmentRootDir(), filepath.FromSlash(storageKey))
			attachmentDir := filepath.Dir(targetPath)
			if err := filestore.EnsureDir(attachmentDir); err != nil {
				return nil, err
			}
			if err := os.Chmod(attachmentDir, filestore.DirPerm); err != nil {
				return nil, err
			}
			if err := os.WriteFile(targetPath, data, 0o600); err != nil {
				return nil, err
			}
			normalized.StorageKey = storageKey
		}
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
	// Only the attachments subdirectory is owned here; the session directory
	// itself (metadata + journal) is removed by its store.
	return os.RemoveAll(filepath.Join(runInputAttachmentRootDir(), safeTranscriptFileName(threadID), "attachments"))
}

func cleanupPersistedRunInputAttachments(attachments []RunInputAttachment) error {
	var errs []error
	for _, attachment := range attachments {
		storageKey := strings.TrimSpace(attachment.StorageKey)
		if storageKey == "" {
			continue
		}
		path := filepath.Join(runInputAttachmentRootDir(), filepath.FromSlash(storageKey))
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func validateRunInputAttachments(attachments []RunInputAttachment) error {
	if len(attachments) == 0 {
		return nil
	}
	for _, attachment := range attachments {
		if _, _, _, err := normalizeRunInputAttachment(attachment); err != nil {
			return err
		}
	}
	return nil
}

func normalizeRunInputAttachment(attachment RunInputAttachment) (RunInputAttachment, []byte, bool, error) {
	normalized := attachment
	if strings.TrimSpace(normalized.ID) == "" {
		return RunInputAttachment{}, nil, false, fmt.Errorf("%w: 附件缺少 id", ErrInvalidInput)
	}
	switch normalized.Kind {
	case RunInputAttachmentKindImage:
		if imageFileExtension(normalized.MimeType) == "" {
			return RunInputAttachment{}, nil, false, fmt.Errorf("%w: 当前仅支持 png、jpeg、webp 图片", ErrInvalidInput)
		}
		if strings.TrimSpace(normalized.DataBase64) == "" {
			return RunInputAttachment{}, nil, false, fmt.Errorf("%w: 图片附件缺少内容", ErrInvalidInput)
		}
		decoded, err := base64.StdEncoding.DecodeString(normalized.DataBase64)
		if err != nil {
			return RunInputAttachment{}, nil, false, fmt.Errorf("%w: 图片附件内容无法解析", ErrInvalidInput)
		}
		if len(decoded) > maxRunInputAttachmentBytes {
			return RunInputAttachment{}, nil, false, fmt.Errorf("%w: 单张图片不能超过 %d MiB", ErrInvalidInput, maxRunInputAttachmentBytes/(1024*1024))
		}
		normalized.ByteSize = len(decoded)
		if normalized.Width < 0 || normalized.Height < 0 {
			return RunInputAttachment{}, nil, false, fmt.Errorf("%w: 图片尺寸无效", ErrInvalidInput)
		}
		return normalized, decoded, true, nil
	case RunInputAttachmentKindFile:
		if strings.TrimSpace(normalized.DataBase64) != "" {
			return RunInputAttachment{}, nil, false, fmt.Errorf("%w: 文件附件不能内联 base64 内容", ErrInvalidInput)
		}
		path := strings.TrimSpace(normalized.Path)
		if path == "" {
			return RunInputAttachment{}, nil, false, fmt.Errorf("%w: 文件附件缺少 path", ErrInvalidInput)
		}
		if !filepath.IsAbs(path) {
			return RunInputAttachment{}, nil, false, fmt.Errorf("%w: 文件附件 path 必须是绝对路径", ErrInvalidInput)
		}
		info, err := os.Stat(path)
		if err != nil {
			return RunInputAttachment{}, nil, false, fmt.Errorf("%w: 文件附件不存在或无法访问", ErrInvalidInput)
		}
		normalized.Path = filepath.Clean(path)
		if normalized.ByteSize <= 0 {
			normalized.ByteSize = int(info.Size())
		}
		return normalized, nil, false, nil
	default:
		return RunInputAttachment{}, nil, false, fmt.Errorf("%w: 当前仅支持 image 或 file 附件", ErrInvalidInput)
	}
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

// runInputAttachmentRootDir resolves the sessions directory of the active
// FileStore generation: runtime-owned attachment blobs live inside their
// session directory (sessions/<thread-id>/attachments/) so deleting a session
// removes its blobs with it. StorageKey stays relative to this root.
func runInputAttachmentRootDir() string {
	return activeSessionsDir()
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
	return defaultInputAttachmentThreadLabel
}

func isInputRequiredError(err error) bool {
	return errors.Is(err, ErrInvalidInput)
}
