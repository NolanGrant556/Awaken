package autodream

import (
	"fmt"
	"strings"
)

const directoryExistsGuidance = "This directory already exists - write to it directly with the Write tool (do not run mkdir or check for its existence)."

func BuildConsolidationPrompt(memoryRoot, transcriptDir string, sessionIDs []string) string {
	extra := buildAutoRunExtra(sessionIDs)
	return fmt.Sprintf(`# Dream: Memory Consolidation

You are performing a dream - a reflective pass over memory files. Synthesize what you learned recently into durable, well-organized memories so future sessions can orient quickly.

Memory directory: %s
%s

Session transcripts: %s (large JSONL files - grep narrowly, don't read whole transcripts)

---

## Phase 1 - Orient

- List the memory directory to see what already exists.
- Read %s to understand the current index.
- Skim existing topic files so you improve them rather than creating duplicates.
- If logs/ or sessions/ subdirectories exist, review recent entries there.

## Phase 2 - Gather recent signal

Look for new information worth persisting. Sources in rough priority order:

1. Daily logs (logs/YYYY/MM/YYYY-MM-DD.md) if present - these are the append-only stream.
2. Existing memories that drifted - facts that contradict something you see in the codebase now.
3. Transcript search - if you need specific context, grep the JSONL transcripts for narrow terms:
   grep -rn "<narrow term>" %s/ --include="*.jsonl" | tail -50

Do not exhaustively read transcripts. Look only for things you already suspect matter.

## Phase 3 - Consolidate

For each thing worth remembering, write or update a memory file at the top level of the memory directory. Use the memory file format and type conventions from the active system prompt's auto-memory section.

Focus on:
- Merging new signal into existing topic files rather than creating near-duplicates.
- Converting relative dates ("yesterday", "last week") to absolute dates so they remain interpretable after time passes.
- Deleting contradicted facts - if today's investigation disproves an old memory, fix it at the source.

## Phase 4 - Prune and index

Update %s so it stays under MAX lines (%d) AND under ~25KB. It is an index, not a dump - each entry should be one line under ~150 characters: - [Title](file.md) - one-line hook. Never write memory content directly into it.

- Remove pointers to memories that are now stale, wrong, or superseded.
- Demote verbose entries: if an index line is over ~200 chars, it is carrying content that belongs in the topic file - shorten the line and move the detail.
- Add pointers to newly important memories.
- Resolve contradictions - if two files disagree, fix the wrong one.

---

Return a brief summary of what you consolidated, updated, or pruned. If nothing changed, say so.

## Additional context

%s`, inlineCode(memoryRoot), directoryExistsGuidance, inlineCode(transcriptDir), inlineCode(MemoryIndexName), transcriptDir, inlineCode(MemoryIndexName), MaxMemoryIndexLines, extra)
}

func buildAutoRunExtra(sessionIDs []string) string {
	var b strings.Builder
	b.WriteString("**Tool constraints for this run:** Bash is restricted to read-only commands (`ls`, `find`, `grep`, `cat`, `stat`, `wc`, `head`, `tail`, and similar). Anything that writes, redirects to a file, or modifies state will be denied. Plan exploration with this in mind - no need to probe.\n\n")
	b.WriteString(fmt.Sprintf("Sessions since last consolidation (%d):", len(sessionIDs)))
	if len(sessionIDs) == 0 {
		b.WriteString("\n- none")
		return b.String()
	}
	for _, id := range sessionIDs {
		b.WriteString("\n- ")
		b.WriteString(id)
	}
	return b.String()
}

func inlineCode(value string) string {
	return "`" + value + "`"
}
