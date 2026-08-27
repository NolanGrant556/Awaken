package webfetch

import (
	"bytes"
	"io"
	"regexp"
	"strings"
	"unicode/utf8"
)

var (
	scriptPattern     = regexp.MustCompile(`(?is)<script\b[^>]*>.*?</script>`)
	stylePattern      = regexp.MustCompile(`(?is)<style\b[^>]*>.*?</style>`)
	noscriptPattern   = regexp.MustCompile(`(?is)<noscript\b[^>]*>.*?</noscript>`)
	templatePattern   = regexp.MustCompile(`(?is)<template\b[^>]*>.*?</template>`)
	tagPattern        = regexp.MustCompile(`(?is)<[^>]+>`)
	whitespacePattern = regexp.MustCompile(`[ \t\x0b\f\r]+`)
	titlePattern      = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
)

func extractHTML(raw []byte) (title, text string) {
	body := string(raw)
	if match := titlePattern.FindStringSubmatch(body); len(match) == 2 {
		title = collapseSpace(stripTags(match[1]))
	}
	stripped := scriptPattern.ReplaceAllString(body, " ")
	stripped = stylePattern.ReplaceAllString(stripped, " ")
	stripped = noscriptPattern.ReplaceAllString(stripped, " ")
	stripped = templatePattern.ReplaceAllString(stripped, " ")
	stripped = strings.ReplaceAll(stripped, "<br", "\n<br")
	stripped = strings.ReplaceAll(stripped, "<p", "\n<p")
	stripped = strings.ReplaceAll(stripped, "<div", "\n<div")
	stripped = strings.ReplaceAll(stripped, "<li", "\n<li")
	stripped = strings.ReplaceAll(stripped, "<h1", "\n<h1")
	stripped = strings.ReplaceAll(stripped, "<h2", "\n<h2")
	stripped = strings.ReplaceAll(stripped, "<tr", "\n<tr")
	text = collapseSpace(stripTags(stripped))
	return title, text
}

func stripTags(input string) string {
	return tagPattern.ReplaceAllString(input, " ")
}

func collapseSpace(input string) string {
	input = strings.ReplaceAll(input, "\u00a0", " ")
	lines := strings.Split(input, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(whitespacePattern.ReplaceAllString(line, " "))
		if line != "" {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n")
}

func decodeUTF8(raw []byte) (string, bool) {
	if !utf8.Valid(raw) {
		return "", false
	}
	text := string(bytes.ReplaceAll(raw, []byte{'\x00'}, nil))
	return text, true
}

func readLimited(reader io.Reader, limit int64) ([]byte, bool, error) {
	limited := io.LimitReader(reader, limit+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, false, err
	}
	truncated := int64(len(data)) > limit
	if truncated {
		data = data[:limit]
	}
	return data, truncated, nil
}
