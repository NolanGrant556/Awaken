package agentcore

import (
	"fmt"
	"regexp"
	"strings"

	convcontext "awaken/src/context"
)

var compactSummarySections = []string{
	"Active Task",
	"Goal",
	"Constraints & Preferences",
	"Completed Actions",
	"Active State",
	"In Progress",
	"Blocked",
	"Key Decisions",
	"Resolved Questions",
	"Pending User Asks",
	"Relevant Files",
	"Remaining Work",
	"Critical Context",
}

var compactSummaryHeadingPattern = regexp.MustCompile(`^(#{1,6}\s*)?(.+?)(:)?$`)

func compactSummarySystemPrompt() string {
	return strings.Join([]string{
		"You are compacting older Awaken conversation history into a handoff summary.",
		"Return concise factual bullets only.",
		"Use exactly these section headings, in this exact order, once each:",
		strings.Join(compactSummarySections, " | "),
		"Rules:",
		"- Keep each section to 1-4 bullet lines beginning with '- '.",
		"- Preserve concrete user goals, constraints, decisions, unresolved asks, important tool outcomes, and file paths.",
		"- Do not invent missing facts. Use '- None.' when the source history does not provide an item.",
		"- Do not wrap the answer in code fences, XML, JSON, or commentary.",
	}, "\n")
}

func compactSummaryUserInstruction(boundaryID string, envelope ContextEnvelope, tailMessages []convcontext.Message) string {
	parts := []string{
		fmt.Sprintf("Create the compact handoff summary for boundary %s.", boundaryID),
		"Summarize only the older conversation history already shown in this request.",
		"The newest protected tail will remain as raw messages after the compact boundary, so only mention it when needed for continuity.",
	}
	if envelope.RuntimeSnapshot.WorkspaceRoot != "" {
		parts = append(parts, "Workspace root: "+envelope.RuntimeSnapshot.WorkspaceRoot)
	}
	if len(tailMessages) > 0 {
		parts = append(parts, "Protected tail anchor:")
		parts = append(parts, "- "+compactTailAnchor(tailMessages))
	}
	return strings.Join(parts, "\n")
}

func normalizeCompactSummary(raw string) string {
	sections := parseCompactSummarySections(raw)
	lines := make([]string, 0, len(compactSummarySections)*3)
	for _, section := range compactSummarySections {
		lines = append(lines, section)
		body := sanitizeCompactSummaryBody(sections[section])
		if len(body) == 0 {
			lines = append(lines, "- None.")
		} else {
			lines = append(lines, body...)
		}
		lines = append(lines, "")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func parseCompactSummarySections(raw string) map[string][]string {
	result := make(map[string][]string, len(compactSummarySections))
	valid := make(map[string]struct{}, len(compactSummarySections))
	for _, section := range compactSummarySections {
		valid[strings.ToLower(section)] = struct{}{}
	}

	current := ""
	for _, line := range strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		matches := compactSummaryHeadingPattern.FindStringSubmatch(trimmed)
		if len(matches) > 2 {
			title := strings.TrimSpace(matches[2])
			if _, ok := valid[strings.ToLower(title)]; ok {
				current = title
				if _, exists := result[current]; !exists {
					result[current] = nil
				}
				continue
			}
		}
		if current == "" {
			continue
		}
		result[current] = append(result[current], trimmed)
	}
	return result
}

func sanitizeCompactSummaryBody(lines []string) []string {
	sanitized := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		line = strings.TrimPrefix(line, "*")
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "- ") {
			line = "- " + strings.TrimLeft(line, "-• ")
		}
		sanitized = append(sanitized, singleLineCompactText(line))
	}
	return sanitized
}

func compactFallbackSummary(marker string, reason string, compactedMessages []convcontext.Message, tailMessages []convcontext.Message, envelope ContextEnvelope, toolResultsPruned int, prunedGroups int, prunedMessages int) string {
	latestUser := latestMessageContent(compactedMessages, convcontext.RoleUser)
	if latestUser == "" {
		latestUser = latestMessageContent(tailMessages, convcontext.RoleUser)
	}
	completed := collectCompactToolNames(compactedMessages)
	files := extractRelevantFiles(compactedMessages, tailMessages)

	sections := map[string][]string{
		"Active Task":               {"- Fallback summary generated after semantic compact failed.", fallbackUserTaskLine(latestUser)},
		"Goal":                      {"- Preserve continuity across the compact boundary without silently dropping older context."},
		"Constraints & Preferences": {"- Fallback marker must remain explicit in the compact summary.", "- Protected tail messages remain raw after the boundary."},
		"Completed Actions":         {"- Structural pre-pass compressed oversized tool results outside the protected tail.", "- Older safe groups prepared for semantic summary."},
		"Active State":              {"- Compacted older message count: " + fmt.Sprintf("%d", len(compactedMessages)), "- Protected tail message count: " + fmt.Sprintf("%d", len(tailMessages))},
		"In Progress":               {"- Semantic handoff summary unavailable; heuristic fallback inserted instead."},
		"Blocked":                   {"- Summary generation reason: " + summarizeFallbackValue(reason)},
		"Key Decisions":             {"- Keep explicit fallback marker in-band for downstream turns.", "- Preserve tool-call and tool-result pairing after compaction."},
		"Resolved Questions":        {"- None."},
		"Pending User Asks":         {"- Review the preserved tail and continue from the latest raw turn."},
		"Remaining Work":            {"- Continue the run using the protected tail plus this fallback summary.", "- Replace fallback with semantic summary in a later compact if needed."},
		"Critical Context":          {"- " + marker, fmt.Sprintf("- Structural tool results pruned: %d", toolResultsPruned), fmt.Sprintf("- Summary prompt pruning applied: safe_groups=%d messages=%d", prunedGroups, prunedMessages)},
	}
	if len(completed) > 0 {
		sections["Completed Actions"] = append(sections["Completed Actions"], "- Tool activity seen before compact: "+strings.Join(completed, ", "))
	}
	if len(files) > 0 {
		sections["Relevant Files"] = []string{"- " + strings.Join(files, ", ")}
	}
	return normalizeCompactSummary(renderCompactSummarySections(sections))
}

func renderCompactSummarySections(sections map[string][]string) string {
	lines := make([]string, 0, len(compactSummarySections)*3)
	for _, section := range compactSummarySections {
		lines = append(lines, section)
		body := sanitizeCompactSummaryBody(sections[section])
		if len(body) == 0 {
			lines = append(lines, "- None.")
		} else {
			lines = append(lines, body...)
		}
		lines = append(lines, "")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func compactTailAnchor(messages []convcontext.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if text := strings.TrimSpace(messages[i].Content); text != "" {
			return limitSummaryText(singleLineCompactText(text))
		}
	}
	return "No textual tail anchor."
}

func latestMessageContent(messages []convcontext.Message, role convcontext.Role) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == role && strings.TrimSpace(messages[i].Content) != "" {
			return singleLineCompactText(messages[i].Content)
		}
	}
	return ""
}

func collectCompactToolNames(messages []convcontext.Message) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0)
	for _, message := range messages {
		if message.Role == convcontext.RoleAssistant {
			for _, call := range message.ToolCalls {
				if call.Function.Name == "" {
					continue
				}
				if _, ok := seen[call.Function.Name]; ok {
					continue
				}
				seen[call.Function.Name] = struct{}{}
				out = append(out, call.Function.Name)
			}
		}
		if message.Role == convcontext.RoleTool && message.Name != "" {
			if _, ok := seen[message.Name]; ok {
				continue
			}
			seen[message.Name] = struct{}{}
			out = append(out, message.Name)
		}
	}
	return out
}

func summarizeFallbackValue(value string) string {
	value = singleLineCompactText(value)
	if value == "" {
		return "Unknown."
	}
	return limitSummaryText(value)
}

func fallbackUserTaskLine(value string) string {
	if strings.TrimSpace(value) == "" {
		return "- Latest visible user request before compact is unknown."
	}
	return "- Latest visible user request before compact is preserved in this fallback summary without raw transcript replay."
}

func extractRelevantFiles(messageSets ...[]convcontext.Message) []string {
	seen := make(map[string]struct{})
	files := make([]string, 0)
	re := regexp.MustCompile(`(?:[A-Za-z0-9._-]+/)*[A-Za-z0-9._-]+\.[A-Za-z0-9._-]+`)
	for _, messages := range messageSets {
		for _, message := range messages {
			for _, match := range re.FindAllString(message.Content, -1) {
				if strings.Contains(match, "://") {
					continue
				}
				if _, ok := seen[match]; ok {
					continue
				}
				seen[match] = struct{}{}
				files = append(files, match)
				if len(files) >= 6 {
					return files
				}
			}
		}
	}
	return files
}

func singleLineCompactText(text string) string {
	text = strings.ReplaceAll(text, "\r", " ")
	text = strings.ReplaceAll(text, "\n", " ")
	return strings.Join(strings.Fields(text), " ")
}
