package agentcore

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestCompactInvokedSkillContextUsesDurableReferencesWithoutDroppingActiveSkills(t *testing.T) {
	base := time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC)
	blocks := make([]ContextBlock, 0, 6)
	for index := 0; index < 6; index++ {
		marker := fmt.Sprintf("skill-%d", index)
		blocks = append(blocks, ContextBlock{
			ID: marker, SkillID: marker, SourcePath: "/skills/" + marker + "/SKILL.md",
			Content:   marker + "\n" + strings.Repeat(string(rune('a'+index)), 25_000),
			InvokedAt: base.Add(time.Duration(index) * time.Minute),
		})
	}
	runner := &Runner{config: AgentConfig{SkillContextBlocks: blocks}}
	runner.compactInvokedSkillContext()

	kept := runner.config.SkillContextBlocks
	if len(kept) != 6 {
		t.Fatalf("reference compaction dropped active Skills, got %d", len(kept))
	}
	if kept[0].ID != "skill-0" || kept[len(kept)-1].ID != "skill-5" {
		t.Fatalf("reference compaction changed invocation order: %+v", kept)
	}
	for _, block := range kept {
		if !block.Compacted {
			t.Fatalf("kept Skill was not marked compacted: %+v", block)
		}
		if !strings.Contains(block.Content, "use Read on "+block.SourcePath) || !strings.Contains(block.Content, "content_digest=") {
			t.Fatalf("Skill ref lacks exact reread path/digest: %s", block.Content)
		}
		if len(block.Content) > 1024 {
			t.Fatalf("Skill ref is unexpectedly large: %d", len(block.Content))
		}
	}
}
