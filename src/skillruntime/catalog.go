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
	CatalogModeNamesOnly    CatalogMode = "names_only"
)

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
			Description: truncateCatalogText(def.Description, SkillCatalogDescriptionCap),
			WhenToUse:   truncateCatalogText(def.WhenToUse, SkillCatalogDescriptionCap),
			Source:      def.Source,
		})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	full := renderCatalog(entries, true)
	mode := CatalogModeDescriptions
	listing := full
	if utf8.RuneCountInString(full) > budget {
		mode = CatalogModeNamesOnly
		listing = renderCatalog(entries, false)
	}
	return CatalogSnapshot{Entries: entries, Listing: listing, BudgetChars: budget, Mode: mode}
}

func renderCatalog(entries []CatalogEntry, descriptions bool) string {
	lines := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !descriptions {
			lines = append(lines, "- "+entry.Name)
			continue
		}
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
		lines = append(lines, fmt.Sprintf("- %s [%s]: %s", entry.Name, entry.Source, detail))
	}
	return strings.Join(lines, "\n")
}

func truncateCatalogText(value string, limit int) string {
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if limit <= 0 || len(runes) <= limit {
		return value
	}
	return string(runes[:limit-1]) + "…"
}
