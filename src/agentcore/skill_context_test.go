package agentcore

import (
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestCompactInvokedSkillContextEnforcesBudgetsAndKeepsNewest(t *testing.T) {
	base := time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC)
	blocks := make([]ContextBlock, 0, 6)
	for index := 0; index < 6; index++ {
		marker := fmt.Sprintf("skill-%d", index)
		blocks = append(blocks, ContextBlock{
			ID: marker, SkillID: marker, SourcePath: "/skills/" + marker + "/SKILL.md",
			Content:   marker + "\n" + strings.Repeat(string(rune('a'+index)), postCompactMaxTokensPerSkill*compactCharsPerToken+200),
			InvokedAt: base.Add(time.Duration(index) * time.Minute),
		})
	}
	runner := &Runner{config: AgentConfig{SkillContextBlocks: blocks}}
	runner.compactInvokedSkillContext()

	kept := runner.config.SkillContextBlocks
	if len(kept) != 5 {
		t.Fatalf("total 25k-token budget should keep only the five newest 5k-token Skills, got %d", len(kept))
	}
	if kept[0].ID != "skill-1" || kept[len(kept)-1].ID != "skill-5" {
		t.Fatalf("compaction did not prioritize recent invocations then restore chronology: %+v", kept)
	}
	totalRunes := 0
	for _, block := range kept {
		if !block.Compacted {
			t.Fatalf("kept Skill was not marked compacted: %+v", block)
		}
		if !strings.Contains(block.Content, "use read_file on "+block.SourcePath) {
			t.Fatalf("truncated Skill lacks an exact reread path: %s", block.Content)
		}
		contentBeforeNotice := strings.SplitN(block.Content, "\n\n[... skill content truncated", 2)[0]
		if utf8.RuneCountInString(contentBeforeNotice) > postCompactMaxTokensPerSkill*compactCharsPerToken {
			t.Fatalf("per-Skill 5k-token budget exceeded: %d", utf8.RuneCountInString(contentBeforeNotice))
		}
		totalRunes += utf8.RuneCountInString(contentBeforeNotice)
	}
	if totalRunes > postCompactSkillsTokenBudget*compactCharsPerToken {
		t.Fatalf("total 25k-token content budget exceeded: %d", totalRunes)
	}
}
