package outlook

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	maxAttachmentBytes   = int64(150 * 1024 * 1024)
	smallAttachmentBytes = int64(3_000_000)
	uploadChunkBytes     = int64(12 * 320 * 1024)
)

type Attachment struct {
	ODataType            string `json:"@odata.type,omitempty"`
	ID                   string `json:"id"`
	Name                 string `json:"name"`
	ContentType          string `json:"contentType,omitempty"`
	Size                 int64  `json:"size,omitempty"`
	IsInline             bool   `json:"isInline,omitempty"`
	LastModifiedDateTime string `json:"lastModifiedDateTime,omitempty"`
}

type AttachmentList struct {
	Attachments []Attachment `json:"attachments"`
	NextLink    string       `json:"nextLink,omitempty"`
}

type graphAttachmentList struct {
	Value    []Attachment `json:"value"`
	NextLink string       `json:"@odata.nextLink"`
}

type DownloadedAttachment struct {
	Path  string `json:"path"`
	Bytes int64  `json:"bytes"`
}

func (c *Client) ListAttachments(ctx context.Context, mailbox, messageID string) (AttachmentList, error) {
	root, err := mailboxRoot(mailbox)
	if err != nil {
		return AttachmentList{}, err
	}
	messageID, err = requiredIdentifier("messageId", messageID)
	if err != nil {
		return AttachmentList{}, err
	}
	var response graphAttachmentList
	path := root + "/messages/" + url.PathEscape(messageID) + "/attachments?$select=id,name,contentType,size,isInline,lastModifiedDateTime"
	if err := c.do(ctx, http.MethodGet, path, nil, &response); err != nil {
		return AttachmentList{}, err
	}
	return AttachmentList{Attachments: response.Value, NextLink: response.NextLink}, nil
}

func (c *Client) DownloadAttachment(ctx context.Context, mailbox, messageID, attachmentID, outputPath string) (DownloadedAttachment, error) {
	root, err := mailboxRoot(mailbox)
	if err != nil {
		return DownloadedAttachment{}, err
	}
	messageID, err = requiredIdentifier("messageId", messageID)
	if err != nil {
		return DownloadedAttachment{}, err
	}
	attachmentID, err = requiredIdentifier("attachmentId", attachmentID)
	if err != nil {
		return DownloadedAttachment{}, err
	}
	outputPath = filepath.Clean(strings.TrimSpace(outputPath))
	if !filepath.IsAbs(outputPath) || outputPath == string(filepath.Separator) {
		return DownloadedAttachment{}, fmt.Errorf("outputPath must be an absolute file path")
	}
	if _, err := os.Lstat(outputPath); err == nil {
		return DownloadedAttachment{}, fmt.Errorf("outputPath already exists")
	} else if !os.IsNotExist(err) {
		return DownloadedAttachment{}, fmt.Errorf("inspect outputPath: %w", err)
	}
	parent := filepath.Dir(outputPath)
	parentInfo, err := os.Stat(parent)
	if err != nil || !parentInfo.IsDir() {
		return DownloadedAttachment{}, fmt.Errorf("outputPath parent directory must already exist")
	}
	temporary, err := os.CreateTemp(parent, ".awaken-outlook-attachment-*")
	if err != nil {
		return DownloadedAttachment{}, fmt.Errorf("create attachment staging file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return DownloadedAttachment{}, fmt.Errorf("secure attachment staging file: %w", err)
	}
	path := root + "/messages/" + url.PathEscape(messageID) + "/attachments/" + url.PathEscape(attachmentID) + "/$value"
	written, err := c.downloadGraphFile(ctx, path, temporary)
	if err != nil {
		return DownloadedAttachment{}, err
	}
	if err := temporary.Sync(); err != nil {
		return DownloadedAttachment{}, fmt.Errorf("sync attachment: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return DownloadedAttachment{}, fmt.Errorf("close attachment: %w", err)
	}
	if err := os.Rename(temporaryPath, outputPath); err != nil {
		return DownloadedAttachment{}, fmt.Errorf("publish attachment: %w", err)
	}
	return DownloadedAttachment{Path: outputPath, Bytes: written}, nil
}

func (c *Client) AddAttachments(ctx context.Context, mailbox, messageID string, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	if len(paths) > 20 {
		return fmt.Errorf("attachment list cannot contain more than 20 files")
	}
	for _, path := range paths {
		if _, err := c.AddAttachment(ctx, mailbox, messageID, path); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) AddAttachment(ctx context.Context, mailbox, messageID, filePath string) (Attachment, error) {
	root, err := mailboxRoot(mailbox)
	if err != nil {
		return Attachment{}, err
	}
	messageID, err = requiredIdentifier("messageId", messageID)
	if err != nil {
		return Attachment{}, err
	}
	resolved, info, err := attachmentFile(filePath)
	if err != nil {
		return Attachment{}, err
	}
	if info.Size() < smallAttachmentBytes {
		content, err := os.ReadFile(resolved)
		if err != nil {
			return Attachment{}, fmt.Errorf("read attachment: %w", err)
		}
		contentType := mime.TypeByExtension(filepath.Ext(resolved))
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		payload := map[string]any{
			"@odata.type": "#microsoft.graph.fileAttachment",
			"name":        filepath.Base(resolved), "contentType": contentType,
			"contentBytes": base64.StdEncoding.EncodeToString(content),
		}
		var attachment Attachment
		path := root + "/messages/" + url.PathEscape(messageID) + "/attachments"
		if err := c.do(ctx, http.MethodPost, path, payload, &attachment); err != nil {
			return Attachment{}, err
		}
		return attachment, nil
	}
	return c.uploadLargeAttachment(ctx, root, messageID, resolved, info.Size())
}

func (c *Client) DeleteAttachment(ctx context.Context, mailbox, messageID, attachmentID string) error {
	root, err := mailboxRoot(mailbox)
	if err != nil {
		return err
	}
	messageID, err = requiredIdentifier("messageId", messageID)
	if err != nil {
		return err
	}
	attachmentID, err = requiredIdentifier("attachmentId", attachmentID)
	if err != nil {
		return err
	}
	path := root + "/messages/" + url.PathEscape(messageID) + "/attachments/" + url.PathEscape(attachmentID)
	return c.do(ctx, http.MethodDelete, path, nil, nil)
}

func attachmentFile(filePath string) (string, os.FileInfo, error) {
	filePath = filepath.Clean(strings.TrimSpace(filePath))
	if !filepath.IsAbs(filePath) {
		return "", nil, fmt.Errorf("attachment path must be absolute")
	}
	resolved, err := filepath.EvalSymlinks(filePath)
	if err != nil {
		return "", nil, fmt.Errorf("resolve attachment path: %w", err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", nil, fmt.Errorf("inspect attachment: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", nil, fmt.Errorf("attachment must be a regular file")
	}
	if info.Size() > maxAttachmentBytes {
		return "", nil, fmt.Errorf("attachment exceeds 150 MiB")
	}
	return resolved, info, nil
}

type graphUploadSession struct {
	UploadURL string `json:"uploadUrl"`
}

func (c *Client) uploadLargeAttachment(ctx context.Context, root, messageID, filePath string, size int64) (Attachment, error) {
	payload := map[string]any{"AttachmentItem": map[string]any{
		"attachmentType": "file", "name": filepath.Base(filePath), "size": size,
	}}
	var session graphUploadSession
	path := root + "/messages/" + url.PathEscape(messageID) + "/attachments/createUploadSession"
	if err := c.do(ctx, http.MethodPost, path, payload, &session); err != nil {
		return Attachment{}, err
	}
	if err := c.validateUploadURL(session.UploadURL); err != nil {
		return Attachment{}, err
	}
	file, err := os.Open(filePath)
	if err != nil {
		return Attachment{}, fmt.Errorf("open attachment: %w", err)
	}
	defer file.Close()
	buffer := make([]byte, uploadChunkBytes)
	var offset int64
	var attachment Attachment
	for offset < size {
		remaining := size - offset
		chunkSize := int64(len(buffer))
		if remaining < chunkSize {
			chunkSize = remaining
		}
		if _, err := io.ReadFull(file, buffer[:chunkSize]); err != nil {
			return Attachment{}, fmt.Errorf("read attachment chunk: %w", err)
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodPut, session.UploadURL, bytes.NewReader(buffer[:chunkSize]))
		if err != nil {
			return Attachment{}, fmt.Errorf("build attachment upload request: %w", err)
		}
		request.Header.Set("Content-Type", "application/octet-stream")
		request.Header.Set("Content-Length", strconv.FormatInt(chunkSize, 10))
		request.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", offset, offset+chunkSize-1, size))
		response, err := c.performRequest(request)
		if err != nil {
			return Attachment{}, fmt.Errorf("upload attachment chunk: %w", err)
		}
		responseBody, readErr := io.ReadAll(io.LimitReader(response.Body, maxGraphResponse+1))
		response.Body.Close()
		if readErr != nil {
			return Attachment{}, fmt.Errorf("read attachment upload response: %w", readErr)
		}
		if len(responseBody) > maxGraphResponse {
			return Attachment{}, fmt.Errorf("attachment upload response is too large")
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return Attachment{}, fmt.Errorf("attachment upload failed with HTTP %d", response.StatusCode)
		}
		offset += chunkSize
		if offset == size && len(bytes.TrimSpace(responseBody)) > 0 {
			_ = json.Unmarshal(responseBody, &attachment)
		}
	}
	if attachment.Name == "" {
		attachment.Name = filepath.Base(filePath)
		attachment.Size = size
	}
	return attachment, nil
}

func (c *Client) validateUploadURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Hostname() == "" || parsed.User != nil {
		return fmt.Errorf("Microsoft Graph returned an invalid attachment upload URL")
	}
	if c.baseURL != defaultGraphBaseURL {
		if parsed.Scheme == "http" || parsed.Scheme == "https" {
			return nil
		}
	}
	if parsed.Scheme != "https" {
		return fmt.Errorf("Microsoft Graph attachment upload URL must use HTTPS")
	}
	host := strings.ToLower(parsed.Hostname())
	allowed := host == "outlook.office.com" || host == "graph.microsoft.com" ||
		strings.HasSuffix(host, ".outlook.com") || strings.HasSuffix(host, ".office.com") || strings.HasSuffix(host, ".office365.com")
	if !allowed {
		return fmt.Errorf("Microsoft Graph returned an untrusted attachment upload host")
	}
	return nil
}

func (c *Client) downloadGraphFile(ctx context.Context, path string, destination io.Writer) (int64, error) {
	if c == nil || c.tokens == nil {
		return 0, fmt.Errorf("outlook connector auth broker is unavailable")
	}
	token, err := c.tokens.AccessToken(ctx, ConnectorID)
	if err != nil {
		return 0, outlookAccessTokenError(err)
	}
	if strings.TrimSpace(token) == "" {
		return 0, ErrNeedsAuth
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(c.baseURL, "/")+path, nil)
	if err != nil {
		return 0, fmt.Errorf("build Graph request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := c.performRequest(request)
	if err != nil {
		return 0, fmt.Errorf("call Microsoft Graph: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusUnauthorized {
		_ = c.tokens.MarkNeedsReauth(ConnectorID)
		if c.onNeedsAuth != nil {
			_ = c.onNeedsAuth(ServerName, "Microsoft Outlook authorization expired")
		}
		return 0, ErrNeedsAuth
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return 0, fmt.Errorf("Microsoft Graph attachment download failed with HTTP %d", response.StatusCode)
	}
	written, err := io.Copy(destination, io.LimitReader(response.Body, maxAttachmentBytes+1))
	if err != nil {
		return 0, fmt.Errorf("download attachment: %w", err)
	}
	if written > maxAttachmentBytes {
		return 0, fmt.Errorf("attachment exceeds 150 MiB")
	}
	return written, nil
}
