package skillruntime

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"awaken/src/skills"
)

const (
	SkillCatalogContextPercent = 0.01
	SkillCatalogCharsPerToken  = 4
	SkillCatalogDefaultChars   = 8000
	SkillCatalogDescriptionCap = 250
)

type CatalogMode string

const (
	CatalogModeDescriptions CatalogMode = "descriptions"
	CatalogModeTruncated    CatalogMode = "truncated_descriptions"
	CatalogModeNamesOnly    CatalogMode = "names_only"
)

// SkillCatalogMinTruncatedDescChars is the floor below which a per-entry
// truncated description stops being useful for the model to judge a match;
// below it the whole catalog degrades to names-only instead of unreadable
// fragments. Mirrors Claude Code's SkillTool prompt MIN_DESC_LENGTH.
const SkillCatalogMinTruncatedDescChars = 20

type CatalogEntry struct {
	Name        string              `json:"name"`
	Description string              `json:"description,omitempty"`
	WhenToUse   string              `json:"when_to_use,omitempty"`
	Source      skills.SourceOrigin `json:"source"`
}

type CatalogSnapshot struct {
	Entries     []CatalogEntry `json:"entries"`
	Listing     string         `json:"listing"`
	BudgetChars int            `json:"budget_chars"`
	Mode        CatalogMode    `json:"mode"`
}

// CompileCatalog degrades in three steps as the budget tightens: full
// per-entry descriptions, then descriptions uniformly truncated to fit the
// remaining budget (never below SkillCatalogMinTruncatedDescChars), then
// names-only. This keeps as much model-facing signal as the budget allows
// instead of jumping straight from full descriptions to bare names.
func CompileCatalog(definitions []*skills.SkillDefinition, contextWindowTokens int) CatalogSnapshot {
	budget := SkillCatalogDefaultChars
	if contextWindowTokens > 0 {
		budget = int(float64(contextWindowTokens*SkillCatalogCharsPerToken) * SkillCatalogContextPercent)
		if budget < 1 {
			budget = 1
		}
	}
	entries := make([]CatalogEntry, 0, len(definitions))
	for _, def := range definitions {
		if def == nil || def.State != skills.StateLoaded || def.DisableModelInvocation {
			continue
		}
		entries = append(entries, CatalogEntry{
			Name:        def.ID,
			Description: strings.Join(strings.Fields(def.Description), " "),
			WhenToUse:   strings.Join(strings.Fields(def.WhenToUse), " "),
			Source:      def.Source,
		})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })

	details := make([]string, len(entries))
	for i, entry := range entries {
		details[i] = truncateCatalogText(catalogEntryDetail(entry), SkillCatalogDescriptionCap)
	}

	full := renderCatalogWithDetails(entries, details)
	if utf8.RuneCountInString(full) <= budget {
		return CatalogSnapshot{Entries: entries, Listing: full, BudgetChars: budget, Mode: CatalogModeDescriptions}
	}

	if len(entries) == 0 {
		return CatalogSnapshot{Entries: entries, Listing: full, BudgetChars: budget, Mode: CatalogModeDescriptions}
	}

	nameOverhead := 0
	for _, entry := range entries {
		nameOverhead += utf8.RuneCountInString(fmt.Sprintf("- %s [%s]: ", entry.Name, entry.Source))
	}
	nameOverhead += len(entries) - 1
	availableForDescs := budget - nameOverhead
	maxDescLen := 0
	if availableForDescs > 0 {
		maxDescLen = availableForDescs / len(entries)
	}

	if maxDescLen < SkillCatalogMinTruncatedDescChars {
		return CatalogSnapshot{Entries: entries, Listing: renderCatalog(entries, false), BudgetChars: budget, Mode: CatalogModeNamesOnly}
	}

	truncatedDetails := make([]string, len(entries))
	for i, entry := range entries {
		truncatedDetails[i] = truncateCatalogText(catalogEntryDetail(entry), maxDescLen)
	}
	truncated := renderCatalogWithDetails(entries, truncatedDetails)
	return CatalogSnapshot{Entries: entries, Listing: truncated, BudgetChars: budget, Mode: CatalogModeTruncated}
}

func catalogEntryDetail(entry CatalogEntry) string {
	detail := strings.TrimSpace(entry.Description)
	if when := strings.TrimSpace(entry.WhenToUse); when != "" && when != detail {
		if detail != "" {
			detail += " — "
		}
		detail += when
	}
	if detail == "" {
		detail = "No description provided."
	}
	return detail
}

func renderCatalog(entries []CatalogEntry, descriptions bool) string {
	if !descriptions {
		lines := make([]string, 0, len(entries))
		for _, entry := range entries {
			lines = append(lines, "- "+entry.Name)
		}
		return strings.Join(lines, "\n")
	}
	details := make([]string, len(entries))
	for i, entry := range entries {
		details[i] = truncateCatalogText(catalogEntryDetail(entry), SkillCatalogDescriptionCap)
	}
	return renderCatalogWithDetails(entries, details)
}

func renderCatalogWithDetails(entries []CatalogEntry, details []string) string {
	lines := make([]string, 0, len(entries))
	for i, entry := range entries {
		lines = append(lines, fmt.Sprintf("- %s [%s]: %s", entry.Name, entry.Source, details[i]))
	}
	return strings.Join(lines, "\n")
}

func truncateCatalogText(value string, limit int) string {
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if limit <= 0 || len(runes) <= limit {
		return value
	}
	if limit == 1 {
		return "…"
	}
	return string(runes[:limit-1]) + "…"
}
