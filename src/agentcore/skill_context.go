package agentcore

import (
	"fmt"
	"strings"

	"awaken/src/skillruntime"
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

// compactInvokedSkillContext replaces full Skill bodies with durable,
// on-demand references. The global V2 allocator owns the whole request budget;
// independent 5k/25k Skill quotas would make a hard 10% postcondition
// impossible for multi-Skill tasks.
func (r *Runner) compactInvokedSkillContext() {
	if r == nil || len(r.config.SkillContextBlocks) == 0 {
		return
	}
	r.config.SkillContextBlocks = compactInvokedSkillContextBlocks(r.config.SkillContextBlocks)
	r.rebuildRequestBuilder()
}

// compactInvokedSkillContextBlocks computes the bounded post-compact Skill
// projection without mutating Runner state. V2 compaction uses this draft for
// validation and durable persistence before installing it atomically.
func compactInvokedSkillContextBlocks(input []ContextBlock) []ContextBlock {
	kept := make([]ContextBlock, 0, len(input))
	for _, block := range input {
		content := strings.TrimSpace(block.Content)
		if content == "" {
			continue
		}
		path := strings.TrimSpace(block.SourcePath)
		if path == "" {
			path = "the Skill package path recorded by the host"
		}
		block.Content = fmt.Sprintf(
			"Active Skill reference after context compaction.\nskill_id=%s\nsource_path=%s\ncontent_digest=%s\nBefore applying this Skill again, use Read on %s to reload its full SKILL.md instructions.",
			strings.TrimSpace(block.SkillID), path, digestText(content), path,
		)
		block.Compacted = true
		kept = append(kept, block)
	}
	return kept
}
