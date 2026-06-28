package attachments

import (
	"context"
	"encoding/json"
	"fmt"
)

const (
	ContentTypeText    = "text"
	ContentTypeImage   = "image"
	SourceTypeBase64   = "base64"
	NativePartText     = "text"
	NativePartImageURL = "image_url"
	defaultImagePrompt = "What do you see in this image?"
	defaultImageMedia  = "image/png"
	defaultFileName    = "attachment"
)

type RawInboundMessage struct {
	Type            string              `json:"type,omitempty"`
	Role            string              `json:"role,omitempty"`
	UUID            string              `json:"uuid,omitempty"`
	Message         *RawMessageEnvelope `json:"message,omitempty"`
	Content         InboundContent      `json:"content,omitempty"`
	FileAttachments []Attachment        `json:"file_attachments,omitempty"`
}

type RawMessageEnvelope struct {
	Content InboundContent `json:"content,omitempty"`
}

type InboundMessage struct {
	UUID        string
	Content     InboundContent
	Attachments []Attachment
}

type InboundContent struct {
	Text     string
	Blocks   []ContentBlock
	IsBlocks bool
}

func TextContent(text string) InboundContent {
	return InboundContent{Text: text}
}

func BlockContent(blocks []ContentBlock) InboundContent {
	return InboundContent{Blocks: blocks, IsBlocks: true}
}

func (c InboundContent) Empty() bool {
	if c.IsBlocks {
		return len(c.Blocks) == 0
	}
	return c.Text == ""
}

func (c *InboundContent) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*c = InboundContent{}
		return nil
	}

	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		*c = TextContent(text)
		return nil
	}

	var blocks []ContentBlock
	if err := json.Unmarshal(data, &blocks); err == nil {
		*c = BlockContent(blocks)
		return nil
	}

	return fmt.Errorf("inbound content must be a string or content blocks")
}

func (c InboundContent) MarshalJSON() ([]byte, error) {
	if c.IsBlocks {
		return json.Marshal(c.Blocks)
	}
	return json.Marshal(c.Text)
}

type ContentBlock struct {
	Type   string       `json:"type"`
	Text   string       `json:"text,omitempty"`
	Source *ImageSource `json:"source,omitempty"`
}

type ImageSource struct {
	Type           string `json:"type"`
	MediaType      string `json:"media_type,omitempty"`
	CamelMediaType string `json:"mediaType,omitempty"`
	Data           string `json:"data,omitempty"`
}

type Attachment struct {
	FileUUID string `json:"file_uuid"`
	FileName string `json:"file_name"`
}

type ResolvedAttachment struct {
	Attachment Attachment
	FileName   string
	Path       string
	Size       int
}

type Fetcher interface {
	FetchAttachment(ctx context.Context, attachment Attachment) ([]byte, error)
}

type Writer interface {
	WriteAttachment(ctx context.Context, fileName string, data []byte) (string, error)
}

type Resolver interface {
	ResolveAttachment(ctx context.Context, attachment Attachment) (ResolvedAttachment, error)
}

type FetchWriteResolver struct {
	Fetcher Fetcher
	Writer  Writer
}

func (r FetchWriteResolver) ResolveAttachment(ctx context.Context, attachment Attachment) (ResolvedAttachment, error) {
	if r.Fetcher == nil {
		return ResolvedAttachment{}, fmt.Errorf("attachment fetcher is nil")
	}
	if r.Writer == nil {
		return ResolvedAttachment{}, fmt.Errorf("attachment writer is nil")
	}

	data, err := r.Fetcher.FetchAttachment(ctx, attachment)
	if err != nil {
		return ResolvedAttachment{}, err
	}

	fileName := SanitizeFileName(attachment.FileName)
	outPath, err := r.Writer.WriteAttachment(ctx, fileName, data)
	if err != nil {
		return ResolvedAttachment{}, err
	}
	if outPath == "" {
		return ResolvedAttachment{}, fmt.Errorf("attachment writer returned an empty path")
	}

	return ResolvedAttachment{
		Attachment: attachment,
		FileName:   fileName,
		Path:       outPath,
		Size:       len(data),
	}, nil
}

type NativeContentPart struct {
	Type     string          `json:"type"`
	Text     string          `json:"text,omitempty"`
	ImageURL *ImageURLSource `json:"image_url,omitempty"`
}

type ImageURLSource struct {
	URL string `json:"url"`
}
