package attachments

import (
	"context"
	"encoding/base64"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
)

func ExtractInboundMessage(raw RawInboundMessage) (InboundMessage, bool) {
	if raw.Type != "user" && raw.Role != "user" {
		return InboundMessage{}, false
	}

	content := raw.Content
	if raw.Message != nil {
		content = raw.Message.Content
	}
	if content.Empty() {
		return InboundMessage{}, false
	}

	if content.IsBlocks {
		content.Blocks = NormalizeImageBlocks(content.Blocks)
	}

	return InboundMessage{
		UUID:        raw.UUID,
		Content:     content,
		Attachments: raw.FileAttachments,
	}, true
}

func NormalizeImageBlocks(blocks []ContentBlock) []ContentBlock {
	needsNormalization := false
	for _, block := range blocks {
		if malformedBase64Image(block) {
			needsNormalization = true
			break
		}
	}
	if !needsNormalization {
		return blocks
	}

	normalized := make([]ContentBlock, len(blocks))
	copy(normalized, blocks)
	for i, block := range normalized {
		if !malformedBase64Image(block) {
			continue
		}

		source := *block.Source
		if source.CamelMediaType != "" {
			source.MediaType = source.CamelMediaType
		} else {
			source.MediaType = DetectImageMediaTypeFromBase64(source.Data)
		}
		source.CamelMediaType = ""
		normalized[i].Source = &source
	}
	return normalized
}

func DetectImageMediaTypeFromBase64(data string) string {
	if mediaType := mediaTypeFromDataURLHeader(data); mediaType != "" {
		return mediaType
	}

	raw := stripDataURLHeader(data)
	raw = strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, raw)

	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(raw)
		if err != nil {
			return defaultImageMedia
		}
	}
	return detectImageMediaTypeFromBytes(decoded)
}

func SanitizeFileName(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	base := path.Base(name)
	if base == "." || base == "/" {
		return defaultFileName
	}

	var b strings.Builder
	for _, r := range base {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}

	safe := b.String()
	if safe == "" || safe == "." || safe == ".." {
		return defaultFileName
	}
	return safe
}

func ResolveAttachments(ctx context.Context, attachments []Attachment, resolver Resolver) []ResolvedAttachment {
	if len(attachments) == 0 || resolver == nil {
		return nil
	}

	resolved := make([]ResolvedAttachment, 0, len(attachments))
	for _, attachment := range attachments {
		result, err := resolver.ResolveAttachment(ctx, attachment)
		if err != nil || result.Path == "" {
			continue
		}
		result.Attachment = attachment
		result.FileName = SanitizeFileName(attachment.FileName)
		resolved = append(resolved, result)
	}
	return resolved
}

func PrependPathRefs(content InboundContent, paths []string) InboundContent {
	prefix := pathRefsPrefix(paths)
	if prefix == "" {
		return content
	}

	if !content.IsBlocks {
		content.Text = prefix + content.Text
		return content
	}

	blocks := make([]ContentBlock, len(content.Blocks))
	copy(blocks, content.Blocks)
	for i := len(blocks) - 1; i >= 0; i-- {
		if blocks[i].Type != ContentTypeText {
			continue
		}
		blocks[i].Text = prefix + blocks[i].Text
		content.Blocks = blocks
		return content
	}

	blocks = append(blocks, ContentBlock{
		Type: ContentTypeText,
		Text: strings.TrimSpace(prefix),
	})
	content.Blocks = blocks
	content.IsBlocks = true
	return content
}

func BuildNativeImageContentParts(userText string, imagePaths []string) ([]NativeContentPart, []string) {
	parts := make([]NativeContentPart, 0, 1+len(imagePaths))
	skipped := make([]string, 0)

	text := strings.TrimSpace(userText)
	if text != "" {
		parts = append(parts, NativeContentPart{Type: NativePartText, Text: text})
	}

	for _, rawPath := range imagePaths {
		data, err := readRegularFile(rawPath)
		if err != nil {
			skipped = append(skipped, rawPath)
			continue
		}

		mediaType := mediaTypeFromPathOrBytes(rawPath, data)
		encoded := base64.StdEncoding.EncodeToString(data)
		parts = append(parts, NativeContentPart{
			Type:     NativePartImageURL,
			ImageURL: &ImageURLSource{URL: "data:" + mediaType + ";base64," + encoded},
		})
	}

	if text == "" && hasImageURLPart(parts) {
		parts = append([]NativeContentPart{{Type: NativePartText, Text: defaultImagePrompt}}, parts...)
	}

	return parts, skipped
}

func pathRefsPrefix(paths []string) string {
	refs := make([]string, 0, len(paths))
	for _, p := range paths {
		if p == "" {
			continue
		}
		refs = append(refs, "@"+strconv.Quote(p))
	}
	if len(refs) == 0 {
		return ""
	}
	return strings.Join(refs, " ") + " "
}

func malformedBase64Image(block ContentBlock) bool {
	if block.Type != ContentTypeImage || block.Source == nil {
		return false
	}
	source := block.Source
	return source.Type == SourceTypeBase64 && source.MediaType == ""
}

func mediaTypeFromDataURLHeader(data string) string {
	if !strings.HasPrefix(strings.ToLower(data), "data:") {
		return ""
	}
	comma := strings.IndexByte(data, ',')
	if comma == -1 {
		return ""
	}
	header := data[5:comma]
	mediaType := strings.ToLower(strings.TrimSpace(strings.Split(header, ";")[0]))
	if isSupportedImageMediaType(mediaType) {
		return mediaType
	}
	return ""
}

func stripDataURLHeader(data string) string {
	if !strings.HasPrefix(strings.ToLower(data), "data:") {
		return data
	}
	if comma := strings.IndexByte(data, ','); comma != -1 {
		return data[comma+1:]
	}
	return data
}

func detectImageMediaTypeFromBytes(data []byte) string {
	if len(data) >= 8 &&
		data[0] == 0x89 &&
		data[1] == 0x50 &&
		data[2] == 0x4e &&
		data[3] == 0x47 &&
		data[4] == 0x0d &&
		data[5] == 0x0a &&
		data[6] == 0x1a &&
		data[7] == 0x0a {
		return "image/png"
	}
	if len(data) >= 3 && data[0] == 0xff && data[1] == 0xd8 && data[2] == 0xff {
		return "image/jpeg"
	}
	if len(data) >= 6 && (string(data[:6]) == "GIF87a" || string(data[:6]) == "GIF89a") {
		return "image/gif"
	}
	if len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP" {
		return "image/webp"
	}
	return defaultImageMedia
}

func isSupportedImageMediaType(mediaType string) bool {
	switch mediaType {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return true
	default:
		return false
	}
}

func readRegularFile(rawPath string) ([]byte, error) {
	info, err := filepath.EvalSymlinks(rawPath)
	if err != nil {
		return nil, err
	}
	fileInfo, err := filepath.Abs(info)
	if err != nil {
		return nil, err
	}
	return readRegularFileAt(fileInfo)
}

func readRegularFileAt(rawPath string) ([]byte, error) {
	info, err := os.Stat(rawPath)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, filepath.SkipDir
	}
	return osReadFile(rawPath)
}

var osReadFile = os.ReadFile

func mediaTypeFromPathOrBytes(rawPath string, data []byte) string {
	switch strings.ToLower(filepath.Ext(rawPath)) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	}

	contentType := http.DetectContentType(data)
	if isSupportedImageMediaType(contentType) {
		return contentType
	}
	return detectImageMediaTypeFromBytes(data)
}

func hasImageURLPart(parts []NativeContentPart) bool {
	for _, part := range parts {
		if part.Type == NativePartImageURL {
			return true
		}
	}
	return false
}
