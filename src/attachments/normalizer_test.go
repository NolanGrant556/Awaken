package attachments

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractInboundMessageSkipsNonUserAndEmptyContent(t *testing.T) {
	if _, ok := ExtractInboundMessage(RawInboundMessage{
		Type:    "assistant",
		Message: &RawMessageEnvelope{Content: TextContent("hello")},
	}); ok {
		t.Fatalf("expected non-user message to be skipped")
	}

	if _, ok := ExtractInboundMessage(RawInboundMessage{
		Type:    "user",
		Message: &RawMessageEnvelope{Content: TextContent("")},
	}); ok {
		t.Fatalf("expected empty string content to be skipped")
	}

	if _, ok := ExtractInboundMessage(RawInboundMessage{
		Type:    "user",
		Message: &RawMessageEnvelope{Content: BlockContent(nil)},
	}); ok {
		t.Fatalf("expected empty block content to be skipped")
	}
}

func TestExtractInboundMessageSupportsStringAndContentBlocks(t *testing.T) {
	text, ok := ExtractInboundMessage(RawInboundMessage{
		Type:    "user",
		UUID:    "message-1",
		Message: &RawMessageEnvelope{Content: TextContent("hello")},
	})
	if !ok {
		t.Fatalf("expected user text message to be extracted")
	}
	if text.UUID != "message-1" || text.Content.Text != "hello" || text.Content.IsBlocks {
		t.Fatalf("unexpected text extraction: %#v", text)
	}

	blocks, ok := ExtractInboundMessage(RawInboundMessage{
		Type: "user",
		Message: &RawMessageEnvelope{Content: BlockContent([]ContentBlock{
			{Type: ContentTypeText, Text: "look"},
		})},
	})
	if !ok {
		t.Fatalf("expected user block message to be extracted")
	}
	if !blocks.Content.IsBlocks || len(blocks.Content.Blocks) != 1 {
		t.Fatalf("unexpected block extraction: %#v", blocks)
	}
}

func TestNormalizeImageBlocksNormalizesCamelCaseMediaType(t *testing.T) {
	blocks := []ContentBlock{
		{
			Type: ContentTypeImage,
			Source: &ImageSource{
				Type:           SourceTypeBase64,
				CamelMediaType: "image/jpeg",
				Data:           base64Bytes([]byte{0xff, 0xd8, 0xff, 0xdb}),
			},
		},
	}

	normalized := NormalizeImageBlocks(blocks)
	if normalized[0].Source.MediaType != "image/jpeg" {
		t.Fatalf("mediaType was not normalized: %#v", normalized[0].Source)
	}
	if normalized[0].Source.CamelMediaType != "" {
		t.Fatalf("camelCase mediaType should not survive normalization: %#v", normalized[0].Source)
	}
	if blocks[0].Source.MediaType != "" {
		t.Fatalf("expected normalization to avoid mutating the input block")
	}
}

func TestDetectImageMediaTypeFromBase64(t *testing.T) {
	tests := map[string][]byte{
		"image/png":  {0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a},
		"image/jpeg": {0xff, 0xd8, 0xff, 0xe0},
		"image/gif":  []byte("GIF89a"),
		"image/webp": []byte("RIFFxxxxWEBP"),
	}

	for want, data := range tests {
		if got := DetectImageMediaTypeFromBase64(base64Bytes(data)); got != want {
			t.Fatalf("DetectImageMediaTypeFromBase64() = %q, want %q", got, want)
		}
	}

	if got := DetectImageMediaTypeFromBase64("data:image/jpeg;base64,not-valid-base64"); got != "image/jpeg" {
		t.Fatalf("expected data URL header to win, got %q", got)
	}
	if got := DetectImageMediaTypeFromBase64("not valid"); got != defaultImageMedia {
		t.Fatalf("expected invalid data to fall back to %q, got %q", defaultImageMedia, got)
	}
}

func TestSanitizeFileName(t *testing.T) {
	tests := map[string]string{
		"../../secret.txt":            "secret.txt",
		`C:\Users\Nolan\my file?.txt`: "my_file_.txt",
		"nested/report(final).md":     "report_final_.md",
		"..":                          defaultFileName,
		"":                            defaultFileName,
	}

	for input, want := range tests {
		if got := SanitizeFileName(input); got != want {
			t.Fatalf("SanitizeFileName(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestPrependPathRefsTargetsLastTextBlock(t *testing.T) {
	content := BlockContent([]ContentBlock{
		{Type: ContentTypeText, Text: "first"},
		{Type: ContentTypeImage, Source: &ImageSource{Type: SourceTypeBase64, MediaType: "image/png"}},
		{Type: ContentTypeText, Text: "second"},
	})

	got := PrependPathRefs(content, []string{"/tmp/a b.txt", `/tmp/quote"file.txt`})
	if got.Blocks[0].Text != "first" {
		t.Fatalf("first text block should be unchanged, got %q", got.Blocks[0].Text)
	}

	wantPrefix := `@"/tmp/a b.txt" @"/tmp/quote\"file.txt" `
	if got.Blocks[2].Text != wantPrefix+"second" {
		t.Fatalf("last text block = %q, want %q", got.Blocks[2].Text, wantPrefix+"second")
	}
}

func TestPrependPathRefsAppendsTextBlockWhenMissing(t *testing.T) {
	content := BlockContent([]ContentBlock{
		{Type: ContentTypeImage, Source: &ImageSource{Type: SourceTypeBase64, MediaType: "image/png"}},
	})

	got := PrependPathRefs(content, []string{"/tmp/a b.txt"})
	if len(got.Blocks) != 2 {
		t.Fatalf("expected appended text block, got %#v", got.Blocks)
	}
	if got.Blocks[1].Type != ContentTypeText || got.Blocks[1].Text != `@"/tmp/a b.txt"` {
		t.Fatalf("unexpected appended text block: %#v", got.Blocks[1])
	}
}

func TestResolveAttachmentsBestEffortSkipsFailures(t *testing.T) {
	resolver := fakeResolver{
		results: map[string]ResolvedAttachment{
			"ok": {Path: "/tmp/ok.txt"},
		},
		fail: map[string]error{
			"bad": errors.New("download failed"),
		},
	}

	got := ResolveAttachments(context.Background(), []Attachment{
		{FileUUID: "ok", FileName: "../../unsafe name.txt"},
		{FileUUID: "bad", FileName: "bad.txt"},
	}, resolver)
	if len(got) != 1 {
		t.Fatalf("expected one resolved attachment, got %#v", got)
	}
	if got[0].FileName != "unsafe_name.txt" {
		t.Fatalf("expected sanitized filename to be filled, got %#v", got[0])
	}
}

func TestFetchWriteResolverSanitizesBeforeWrite(t *testing.T) {
	writer := &fakeWriter{}
	resolver := FetchWriteResolver{
		Fetcher: fakeFetcher{data: []byte("hello")},
		Writer:  writer,
	}

	got, err := resolver.ResolveAttachment(context.Background(), Attachment{
		FileUUID: "file-1",
		FileName: "../../my file.txt",
	})
	if err != nil {
		t.Fatalf("ResolveAttachment returned error: %v", err)
	}
	if writer.fileName != "my_file.txt" {
		t.Fatalf("writer got filename %q, want sanitized name", writer.fileName)
	}
	if got.Path != "/tmp/my_file.txt" || got.Size != len("hello") {
		t.Fatalf("unexpected resolved attachment: %#v", got)
	}
}

func TestBuildNativeImageContentPartsSkipsUnreadablePaths(t *testing.T) {
	dir := t.TempDir()
	pngPath := filepath.Join(dir, "image.png")
	if err := os.WriteFile(pngPath, []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}, 0o600); err != nil {
		t.Fatalf("write png fixture: %v", err)
	}
	missingPath := filepath.Join(dir, "missing.png")

	parts, skipped := BuildNativeImageContentParts("  inspect this  ", []string{pngPath, missingPath})
	if len(skipped) != 1 || skipped[0] != missingPath {
		t.Fatalf("unexpected skipped paths: %#v", skipped)
	}
	if len(parts) != 2 {
		t.Fatalf("expected text plus one image part, got %#v", parts)
	}
	if parts[0].Type != NativePartText || parts[0].Text != "inspect this" {
		t.Fatalf("unexpected text part: %#v", parts[0])
	}
	if parts[1].Type != NativePartImageURL || parts[1].ImageURL == nil {
		t.Fatalf("unexpected image part: %#v", parts[1])
	}
	if !strings.HasPrefix(parts[1].ImageURL.URL, "data:image/png;base64,") {
		t.Fatalf("unexpected image data URL: %q", parts[1].ImageURL.URL)
	}
}

type fakeResolver struct {
	results map[string]ResolvedAttachment
	fail    map[string]error
}

func (f fakeResolver) ResolveAttachment(_ context.Context, attachment Attachment) (ResolvedAttachment, error) {
	if err := f.fail[attachment.FileUUID]; err != nil {
		return ResolvedAttachment{}, err
	}
	result := f.results[attachment.FileUUID]
	result.Attachment = attachment
	return result, nil
}

type fakeFetcher struct {
	data []byte
	err  error
}

func (f fakeFetcher) FetchAttachment(_ context.Context, _ Attachment) ([]byte, error) {
	return f.data, f.err
}

type fakeWriter struct {
	fileName string
}

func (f *fakeWriter) WriteAttachment(_ context.Context, fileName string, _ []byte) (string, error) {
	f.fileName = fileName
	return "/tmp/" + fileName, nil
}

func base64Bytes(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}
