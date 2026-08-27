package researchruntime

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
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
		fmt.Fprintf(&b, "- [%s] %s — %s\n", row.marker, emptyTitle(row.ev.Title), row.ev.Locator)
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
