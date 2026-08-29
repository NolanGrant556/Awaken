package researchruntime

import (
	"awaken/src/subagentruntime"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
)

type Claim struct {
	ID                 string
	QuestionID         string
	Statement          string
	Confidence         string
	VerificationStatus string
	EvidenceIDs        []string
}

type Evidence struct {
	ID              string
	Kind            string
	Title           string
	Locator         string
	SourceType      string
	IndependenceKey string
	Digest          string
	FetchedAt       string
}

func assignCitations(evidence []Evidence) map[string]string {
	type item struct {
		id      string
		locator string
		digest  string
	}
	items := make([]item, 0, len(evidence))
	for _, ev := range evidence {
		items = append(items, item{id: ev.ID, locator: strings.ToLower(strings.TrimSpace(ev.Locator)), digest: ev.Digest})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].locator == items[j].locator {
			return items[i].digest < items[j].digest
		}
		return items[i].locator < items[j].locator
	})
	seen := map[string]string{}
	out := map[string]string{}
	next := 1
	for _, it := range items {
		key := it.locator + "\n" + it.digest
		if marker, ok := seen[key]; ok {
			out[it.id] = marker
			continue
		}
		marker := fmt.Sprintf("S%d", next)
		next++
		seen[key] = marker
		out[it.id] = marker
	}
	return out
}

func renderReport(intent Intent, outcome string, answer string, claims []Claim, evidence []Evidence, coverage []string) string {
	citations := assignCitations(evidence)
	var b strings.Builder
	fmt.Fprintf(&b, "# Deep Research Report\n\n")
	fmt.Fprintf(&b, "Status: %s\n\n", titleOutcome(outcome))
	fmt.Fprintf(&b, "## Question\n\n%s\n\n", intent.Query)
	fmt.Fprintf(&b, "## Answer\n\n%s\n\n", strings.TrimSpace(answer))
	fmt.Fprintf(&b, "## Findings\n\n")
	for _, claim := range claims {
		markers := make([]string, 0, len(claim.EvidenceIDs))
		for _, id := range claim.EvidenceIDs {
			if marker := citations[id]; marker != "" {
				markers = append(markers, "["+marker+"]")
			}
		}
		fmt.Fprintf(&b, "- **%s** %s %s\n", claim.VerificationStatus, claim.Statement, strings.Join(uniqueStrings(markers), " "))
	}
	if len(coverage) > 0 {
		fmt.Fprintf(&b, "\n## Coverage gaps\n\n")
		for _, note := range coverage {
			fmt.Fprintf(&b, "- %s\n", note)
		}
	}
	fmt.Fprintf(&b, "\n## Sources\n\n")
	type row struct {
		marker string
		ev     Evidence
	}
	rows := make([]row, 0, len(evidence))
	seenMarker := map[string]bool{}
	for _, ev := range evidence {
		marker := citations[ev.ID]
		if marker == "" || seenMarker[marker] {
			continue
		}
		seenMarker[marker] = true
		rows = append(rows, row{marker: marker, ev: ev})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].marker < rows[j].marker })
	for _, row := range rows {
		when := ""
		if strings.TrimSpace(row.ev.FetchedAt) != "" {
			when = " — fetched " + row.ev.FetchedAt
		}
		fmt.Fprintf(&b, "- [%s] %s — %s%s\n", row.marker, escapeMarkdownText(emptyTitle(row.ev.Title)), row.ev.Locator, when)
	}
	return b.String()
}

func titleOutcome(outcome string) string {
	switch strings.ToLower(strings.TrimSpace(outcome)) {
	case "verified":
		return "Verified"
	case "partial":
		return "Partial"
	case "failed":
		return "Failed"
	default:
		return strings.TrimSpace(outcome)
	}
}

func applyHostVerificationRules(claims []Claim, evidence []Evidence) []Claim {
	byID := map[string]Evidence{}
	for _, ev := range evidence {
		byID[ev.ID] = ev
	}
	for i, claim := range claims {
		if len(claim.EvidenceIDs) == 0 {
			claims[i].VerificationStatus = "rejected"
			continue
		}
		if claim.VerificationStatus != "verified" {
			continue
		}
		keys := map[string]bool{}
		authoritative := false
		for _, id := range claim.EvidenceIDs {
			ev, ok := byID[id]
			if !ok {
				continue
			}
			if strings.TrimSpace(ev.IndependenceKey) != "" {
				keys[ev.IndependenceKey] = true
			}
			if strings.EqualFold(ev.SourceType, "primary") || ev.Kind == "workspace" {
				authoritative = true
			}
		}
		if !authoritative && len(keys) < 2 {
			claims[i].VerificationStatus = "partial"
		}
	}
	return claims
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func emptyTitle(title string) string {
	if strings.TrimSpace(title) == "" {
		return "Source"
	}
	return title
}

func escapeMarkdownText(value string) string {
	replacer := strings.NewReplacer("\\", "\\\\", "[", "\\[", "]", "\\]", "<", "&lt;", ">", "&gt;", "\n", " ", "\r", " ")
	return replacer.Replace(strings.TrimSpace(value))
}

func digestOf(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func independenceKey(ev RawEvidence) string {
	locator := strings.ToLower(strings.TrimSpace(ev.Locator))
	if ev.Kind == "workspace" {
		path, _, _ := strings.Cut(locator, ":")
		return "workspace:" + path
	}
	host := locator
	host = strings.TrimPrefix(host, "https://")
	host = strings.TrimPrefix(host, "http://")
	if slash := strings.Index(host, "/"); slash >= 0 {
		host = host[:slash]
	}
	if colon := strings.Index(host, ":"); colon >= 0 {
		host = host[:colon]
	}
	parts := strings.Split(host, ".")
	if len(parts) >= 2 {
		host = strings.Join(parts[len(parts)-2:], ".")
	}
	return "web:" + host
}

func groundEvidence(raw RawEvidence, receipts []subagentruntime.HostToolReceipt) (Evidence, bool) {
	kind := strings.ToLower(strings.TrimSpace(raw.Kind))
	for _, receipt := range receipts {
		switch kind {
		case "web":
			if receipt.ToolName != "WebFetch" {
				continue
			}
			finalURL := detailString(receipt.Details, "final_url")
			digest := detailString(receipt.Details, "content_sha256")
			receiptMatch := strings.TrimSpace(raw.ReceiptID) != "" && raw.ReceiptID == receipt.ToolCallID
			if digest == "" || (!receiptMatch && !sameWebLocator(raw.Locator, finalURL)) {
				continue
			}
			title := detailString(receipt.Details, "title")
			if title == "" {
				title = raw.Title
			}
			return Evidence{Kind: kind, Title: title, Locator: finalURL, SourceType: raw.SourceType, IndependenceKey: independenceKey(RawEvidence{Kind: kind, Locator: finalURL}), Digest: digest, FetchedAt: detailString(receipt.Details, "fetched_at")}, true
		case "workspace":
			if receipt.ToolName != "Read" {
				continue
			}
			path := detailString(receipt.Details, "file_path")
			digest := detailString(receipt.Details, "sha256")
			if digest == "" || !sameWorkspaceLocator(raw.Locator, path) {
				continue
			}
			return Evidence{Kind: kind, Title: raw.Title, Locator: raw.Locator, SourceType: raw.SourceType, IndependenceKey: independenceKey(raw), Digest: digest}, true
		}
	}
	return Evidence{}, false
}

func receiptsSupportClaim(claim Claim, evidence []Evidence, receipts []subagentruntime.HostToolReceipt) bool {
	wanted := map[string]bool{}
	for _, id := range claim.EvidenceIDs {
		wanted[id] = true
	}
	for _, ev := range evidence {
		if !wanted[ev.ID] {
			continue
		}
		for _, receipt := range receipts {
			if ev.Kind == "web" && receipt.ToolName == "WebFetch" && sameWebLocator(ev.Locator, detailString(receipt.Details, "final_url")) && ev.Digest == detailString(receipt.Details, "content_sha256") {
				return true
			}
			if ev.Kind == "workspace" && receipt.ToolName == "Read" && sameWorkspaceLocator(ev.Locator, detailString(receipt.Details, "file_path")) && ev.Digest == detailString(receipt.Details, "sha256") {
				return true
			}
		}
	}
	return false
}

func detailString(details map[string]any, key string) string {
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
