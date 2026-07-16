package agentcore

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"awaken/src/skillruntime"
)

const (
	postCompactMaxTokensPerSkill = 5_000
	postCompactSkillsTokenBudget = 25_000
	compactCharsPerToken         = 4
)

func skillActivationSourcePath(plan *skillruntime.SkillActivationPlan) string {
	if plan == nil {
		return ""
	}
	root := strings.TrimSpace(plan.RootDir)
	if root == "" {
		return ""
	}
	return strings.TrimRight(root, "/\\") + "/SKILL.md"
}

// compactInvokedSkillContext mirrors the Claude Code recovery budget: most
// recent invocations first, at most 5k estimated tokens per Skill and 25k in
// total. The source path remains in the recovery notice so the model can use
// Read when it needs the full package instructions again.
func (r *Runner) compactInvokedSkillContext() {
	if r == nil || len(r.config.SkillContextBlocks) == 0 {
		return
	}
	blocks := append([]ContextBlock(nil), r.config.SkillContextBlocks...)
	sort.SliceStable(blocks, func(i, j int) bool {
		if blocks[i].InvokedAt.Equal(blocks[j].InvokedAt) {
			return i > j
		}
		return blocks[i].InvokedAt.After(blocks[j].InvokedAt)
	})

	perSkillChars := postCompactMaxTokensPerSkill * compactCharsPerToken
	totalChars := postCompactSkillsTokenBudget * compactCharsPerToken
	used := 0
	kept := make([]ContextBlock, 0, len(blocks))
	for _, block := range blocks {
		content := strings.TrimSpace(block.Content)
		if content == "" || used >= totalChars {
			continue
		}
		limit := perSkillChars
		if remaining := totalChars - used; remaining < limit {
			limit = remaining
		}
		content, truncated := truncateSkillContext(content, limit)
		if truncated {
			path := strings.TrimSpace(block.SourcePath)
			if path == "" {
				path = "the Skill package path recorded by the host"
			}
			content += fmt.Sprintf("\n\n[... skill content truncated for compaction; use Read on %s if you need the full text]", path)
		}
		block.Content = content
		block.Compacted = true
		used += utf8.RuneCountInString(content)
		kept = append(kept, block)
	}
	// System blocks keep invocation order even though selection used recency.
	sort.SliceStable(kept, func(i, j int) bool { return kept[i].InvokedAt.Before(kept[j].InvokedAt) })
	r.config.SkillContextBlocks = kept
	r.reqBuilder = NewRequestBuilder(r.config)
}

func truncateSkillContext(value string, maxRunes int) (string, bool) {
	if maxRunes <= 0 {
		return "", strings.TrimSpace(value) != ""
	}
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value, false
	}
	return strings.TrimSpace(string(runes[:maxRunes])), true
}
