package outputpersist

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

type Plan struct {
	Path    string
	Ext     string
	Message string
}

func ExtensionForMIMEType(mime string) string {
	switch normalizedMIMEType(mime) {
	case "application/pdf":
		return "pdf"
	case "application/json":
		return "json"
	case "application/xml":
		return "xml"
	case "application/javascript":
		return "js"
	case "application/x-www-form-urlencoded":
		return "txt"
	case "text/csv":
		return "csv"
	case "text/plain":
		return "txt"
	case "text/html":
		return "html"
	case "text/markdown":
		return "md"
	case "application/zip":
		return "zip"
	case "application/vnd.openxmlformats-officedocument.wordprocessingml.document":
		return "docx"
	case "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":
		return "xlsx"
	case "application/vnd.openxmlformats-officedocument.presentationml.presentation":
		return "pptx"
	case "application/msword":
		return "doc"
	case "application/vnd.ms-excel":
		return "xls"
	case "audio/mpeg":
		return "mp3"
	case "audio/wav":
		return "wav"
	case "audio/ogg":
		return "ogg"
	case "video/mp4":
		return "mp4"
	case "video/webm":
		return "webm"
	case "image/png":
		return "png"
	case "image/jpeg":
		return "jpg"
	case "image/gif":
		return "gif"
	case "image/webp":
		return "webp"
	case "image/svg+xml":
		return "svg"
	default:
		return "bin"
	}
}

func IsBinaryContentType(contentType string) bool {
	mt := normalizedMIMEType(contentType)
	if mt == "" {
		return false
	}

	if strings.HasPrefix(mt, "image/") || strings.HasPrefix(mt, "audio/") || strings.HasPrefix(mt, "video/") {
		return true
	}
	if mt == "application/pdf" || mt == "application/zip" {
		return true
	}
	if mt == "application/vnd.openxmlformats-officedocument.wordprocessingml.document" ||
		mt == "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" ||
		mt == "application/vnd.openxmlformats-officedocument.presentationml.presentation" {
		return true
	}

	if strings.HasPrefix(mt, "text/") {
		return false
	}
	if mt == "application/json" || strings.HasSuffix(mt, "+json") {
		return false
	}
	if mt == "application/xml" || strings.HasSuffix(mt, "+xml") {
		return false
	}
	if mt == "application/javascript" || mt == "application/x-www-form-urlencoded" {
		return false
	}

	return true
}

func BuildBinaryBlobMessage(path, mime string, size int64, source string) string {
	mt := strings.TrimSpace(mime)
	if mt == "" {
		mt = "unknown type"
	}
	return fmt.Sprintf("%sBinary content (%s, %s) saved to %s", source, mt, formatFileSize(size), path)
}

func LargeOutputInstructions(path string, length int, format string, maxReadLength int) string {
	if strings.TrimSpace(format) == "" {
		format = "Plain text"
	}

	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("Error: result (%s characters) exceeds maximum allowed tokens. Output has been saved to %s.\n", formatCount(length), path))
	builder.WriteString(fmt.Sprintf("Format: %s\n", format))
	builder.WriteString("Use offset and limit parameters to read specific portions of the file, search within it for specific content, and jq to make structured queries.\n")
	builder.WriteString("REQUIREMENTS FOR SUMMARIZATION/ANALYSIS/REVIEW:\n")
	builder.WriteString(fmt.Sprintf("- You MUST read the content from the file at %s in sequential chunks until 100%% of the content has been read.\n", path))
	if maxReadLength > 0 {
		builder.WriteString(fmt.Sprintf("- If you receive truncation warnings when reading the file (\"[N lines truncated]\"), reduce the chunk size until you have read 100%% of the content without truncation ***DO NOT PROCEED UNTIL YOU HAVE DONE THIS***. Bash output is limited to %s chars.\n", formatCount(maxReadLength)))
	} else {
		builder.WriteString("- If you receive truncation warnings when reading the file, reduce the chunk size until you have read 100% of the content without truncation.\n")
	}
	builder.WriteString("- Before producing ANY summary or analysis, you MUST explicitly describe what portion of the content you have read. ***If you did not read the entire content, you MUST explicitly state this.***\n")
	return builder.String()
}

func PlanBinaryPersist(baseDir, persistID, mime string, size int64) Plan {
	ext := ExtensionForMIMEType(mime)
	path := filepath.Join(baseDir, sanitizePersistID(persistID)+"."+ext)
	return Plan{
		Path:    path,
		Ext:     ext,
		Message: BuildBinaryBlobMessage(path, mime, size, ""),
	}
}

func normalizedMIMEType(value string) string {
	mediaType := strings.Split(value, ";")[0]
	return strings.ToLower(strings.TrimSpace(mediaType))
}

func sanitizePersistID(value string) string {
	value = strings.TrimSpace(value)
	var builder strings.Builder
	lastUnderscore := false

	for _, r := range value {
		safe := (r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') ||
			r == '-' ||
			r == '_'
		if safe {
			builder.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			builder.WriteByte('_')
			lastUnderscore = true
		}
	}

	sanitized := strings.Trim(builder.String(), "_-")
	if sanitized == "" {
		return "output"
	}
	if len(sanitized) > 120 {
		return sanitized[:120]
	}
	return sanitized
}

func formatFileSize(size int64) string {
	kb := float64(size) / 1024
	if kb < 1 {
		return fmt.Sprintf("%d bytes", size)
	}
	if kb < 1024 {
		return formatDecimal(kb) + "KB"
	}
	mb := kb / 1024
	if mb < 1024 {
		return formatDecimal(mb) + "MB"
	}
	gb := mb / 1024
	return formatDecimal(gb) + "GB"
}

func formatDecimal(value float64) string {
	formatted := fmt.Sprintf("%.1f", value)
	return strings.TrimSuffix(formatted, ".0")
}

func formatCount(value int) string {
	raw := strconv.Itoa(value)
	if len(raw) <= 3 {
		return raw
	}

	sign := ""
	if strings.HasPrefix(raw, "-") {
		sign = "-"
		raw = strings.TrimPrefix(raw, "-")
	}

	firstGroup := len(raw) % 3
	if firstGroup == 0 {
		firstGroup = 3
	}

	var builder strings.Builder
	builder.WriteString(sign)
	builder.WriteString(raw[:firstGroup])
	for i := firstGroup; i < len(raw); i += 3 {
		builder.WriteByte(',')
		builder.WriteString(raw[i : i+3])
	}
	return builder.String()
}
