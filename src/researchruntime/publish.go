package researchruntime

import (
	"archive/zip"
	"bytes"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"awaken/src/subagentruntime"
)

var (
	httpsURLPattern = regexp.MustCompile(`https://[^\s<>"'，。；！？、（）【】]+`)
	workspacePathPattern = regexp.MustCompile(`(?i)(?:^|[\s"'(\[]|])((?:\./)?(?:[A-Za-z0-9._-]+/)+[A-Za-z0-9._-]+\.[A-Za-z0-9]{1,8}(?::\d+(?:-\d+)?)?)`)
	overallVerifiedPattern = regexp.MustCompile(`(?i)(?:status|状态)\s*[:：]\s*(verified|已核实)\b`)
)

type PublishCitationError struct {
	Locators []string
}

func (e *PublishCitationError) Error() string {
	if e == nil || len(e.Locators) == 0 {
		return "research citations are not grounded"
	}
	return "cannot mark as verified without a successful Read/WebFetch receipt: " + strings.Join(e.Locators, ", ")
}

func ExtractPublishText(filename string, data []byte) string {
	name := strings.ToLower(strings.TrimSpace(filename))
	if strings.HasSuffix(name, ".docx") || strings.HasSuffix(name, ".pptx") || strings.HasSuffix(name, ".xlsx") {
		if text := extractZipXMLText(data); strings.TrimSpace(text) != "" {
			return text
		}
	}
	if utf8.Valid(data) {
		return string(data)
	}
	return string(bytes.ToValidUTF8(data, []byte("")))
}

func ValidateVerifiedCitations(text string, receipts []subagentruntime.HostToolReceipt) error {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	locators := extractLocators(text)
	if len(locators) == 0 {
		if claimsOverallVerified(text) {
			return &PublishCitationError{Locators: []string{"(document claims verified with no extractable locator)"}}
		}
		return nil
	}
	required := map[string]bool{}
	if claimsOverallVerified(text) {
		for _, locator := range locators {
			required[locator] = true
		}
	}
	for _, line := range strings.Split(text, "\n") {
		if !isVerifiedLine(line) {
			continue
		}
		for _, locator := range extractLocators(line) {
			required[locator] = true
		}
	}
	var missing []string
	for locator := range required {
		if !receiptGroundsLocator(locator, receipts) {
			missing = append(missing, locator)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return &PublishCitationError{Locators: uniqueStrings(missing)}
}

func extractLocators(text string) []string {
	var out []string
	for _, match := range httpsURLPattern.FindAllString(text, -1) {
		match = strings.TrimRight(match, "，。；！？、,;!?)）】}>")
		if match != "" {
			out = append(out, match)
		}
	}
	for _, match := range workspacePathPattern.FindAllStringSubmatch(text, -1) {
		if len(match) > 1 && strings.TrimSpace(match[1]) != "" {
			out = append(out, strings.TrimSpace(match[1]))
		}
	}
	return uniqueStrings(out)
}

func isVerifiedLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}
	lower := strings.ToLower(trimmed)
	if strings.Contains(trimmed, "未核实") || strings.Contains(trimmed, "部分核实") || strings.Contains(trimmed, "未核验") {
		return false
	}
	if strings.Contains(lower, "partial") && !strings.Contains(lower, "verified") && !strings.Contains(trimmed, "已核实") {
		return false
	}
	return strings.Contains(trimmed, "已核实") || regexp.MustCompile(`(?i)\bverified\b`).MatchString(trimmed)
}

func claimsOverallVerified(text string) bool {
	return overallVerifiedPattern.MatchString(text)
}

func receiptGroundsLocator(locator string, receipts []subagentruntime.HostToolReceipt) bool {
	locator = strings.TrimSpace(locator)
	if locator == "" {
		return false
	}
	for _, receipt := range receipts {
		switch receipt.ToolName {
		case "WebFetch":
			finalURL := detailString(receipt.Details, "final_url")
			requested := detailString(receipt.Details, "requested_url")
			digest := detailString(receipt.Details, "content_sha256")
			if digest == "" {
				continue
			}
			if sameWebLocator(locator, finalURL) || sameWebLocator(locator, requested) {
				return true
			}
		case "Read":
			path := detailString(receipt.Details, "file_path")
			digest := detailString(receipt.Details, "sha256")
			if digest == "" {
				continue
			}
			if sameWorkspaceLocator(locator, path) {
				return true
			}
		}
	}
	return false
}

func extractZipXMLText(data []byte) string {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return ""
	}
	var b strings.Builder
	for _, file := range reader.File {
		name := strings.ToLower(file.Name)
		if !strings.HasSuffix(name, ".xml") && !strings.HasSuffix(name, ".rels") {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			continue
		}
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(rc)
		_ = rc.Close()
		b.Write(buf.Bytes())
		b.WriteByte('\n')
	}
	return b.String()
}

func detailString(details map[string]any, key string) string {
	if details == nil {
		return ""
	}
	value, _ := details[key].(string)
	return strings.TrimSpace(value)
}

func sameWebLocator(left, right string) bool {
	canonical := func(raw string) string {
		parsed, err := url.Parse(strings.TrimSpace(raw))
		if err != nil || !strings.EqualFold(parsed.Scheme, "https") || parsed.Host == "" {
			return ""
		}
		parsed.Scheme = "https"
		parsed.Host = strings.ToLower(parsed.Host)
		parsed.Fragment = ""
		parsed.RawFragment = ""
		return parsed.String()
	}
	a, b := canonical(left), canonical(right)
	return a != "" && a == b
}

func sameWorkspaceLocator(locator, canonicalPath string) bool {
	locator = strings.TrimSpace(locator)
	if index := strings.LastIndex(locator, ":"); index >= 0 {
		lineSpec := locator[index+1:]
		if lineSpec != "" && strings.IndexFunc(lineSpec, func(r rune) bool { return (r < '0' || r > '9') && r != '-' }) < 0 {
			locator = locator[:index]
		}
	}
	locator = filepath.ToSlash(filepath.Clean(locator))
	canonicalPath = filepath.ToSlash(filepath.Clean(canonicalPath))
	return locator != "." && canonicalPath != "." && (canonicalPath == locator || strings.HasSuffix(canonicalPath, "/"+strings.TrimPrefix(locator, "./")))
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}
