package contextviz

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

const (
	CategoryFreeSpace         = "Free space"
	CategoryAutocompactBuffer = "Autocompact buffer"
)

type Options struct {
	AntOnly bool
}

type Data struct {
	Model                string
	TotalTokens          int
	RawMaxTokens         int
	Categories           []Category
	MemoryFiles          []MemoryFile
	MCPTools             []MCPTool
	Agents               []Agent
	Skills               SkillInfo
	MessageBreakdown     *MessageBreakdown
	SystemTools          []TokenItem
	SystemPromptSections []TokenItem
	CollapseStats        CollapseStats
}

func (d Data) Percentage() int {
	return Percentage(d.TotalTokens, d.RawMaxTokens)
}

type Category struct {
	Name       string
	Tokens     int
	IsDeferred bool
}

type MemoryFile struct {
	Type   string
	Path   string
	Tokens int
}

type MCPTool struct {
	Name       string
	ServerName string
	Tokens     int
}

type Agent struct {
	AgentType string
	Source    string
	Tokens    int
}

type SkillInfo struct {
	TotalSkills      int
	IncludedSkills   int
	Tokens           int
	SkillFrontmatter []SkillFrontmatter
}

type SkillFrontmatter struct {
	Name   string
	Source string
	Tokens int
}

type TokenItem struct {
	Name   string
	Tokens int
}

type MessageBreakdown struct {
	ToolCallTokens         int
	ToolResultTokens       int
	AttachmentTokens       int
	AssistantMessageTokens int
	UserMessageTokens      int
	ToolCallsByType        []ToolCallBreakdown
	AttachmentsByType      []TokenItem
}

type ToolCallBreakdown struct {
	Name         string
	CallTokens   int
	ResultTokens int
}

type CollapseStats struct {
	Enabled                  bool
	CollapsedSpans           int
	CollapsedMessages        int
	StagedSpans              int
	TotalSpawns              int
	TotalErrors              int
	LastError                string
	EmptySpawnWarningEmitted bool
	TotalEmptySpawns         int
}

func Percentage(tokens, rawMaxTokens int) int {
	if rawMaxTokens <= 0 {
		return 0
	}
	return int(math.Round(float64(tokens) / float64(rawMaxTokens) * 100))
}

func FormatTokens(count int) string {
	if count > -1000 && count < 1000 {
		return strconv.Itoa(count)
	}

	type unit struct {
		value  float64
		suffix string
	}

	units := []unit{
		{value: 1_000_000_000_000, suffix: "t"},
		{value: 1_000_000_000, suffix: "b"},
		{value: 1_000_000, suffix: "m"},
		{value: 1_000, suffix: "k"},
	}

	sign := ""
	abs := float64(count)
	if abs < 0 {
		sign = "-"
		abs = -abs
	}

	for i, u := range units {
		if abs < u.value {
			continue
		}
		rounded := math.Round(abs/u.value*10) / 10
		if rounded >= 1000 && i > 0 {
			next := units[i-1]
			rounded = math.Round(abs/next.value*10) / 10
			formatted := strconv.FormatFloat(rounded, 'f', 1, 64)
			formatted = strings.TrimSuffix(formatted, ".0")
			return sign + formatted + next.suffix
		}
		formatted := strconv.FormatFloat(rounded, 'f', 1, 64)
		formatted = strings.TrimSuffix(formatted, ".0")
		return sign + formatted + u.suffix
	}

	return strconv.Itoa(count)
}

func VisibleCategories(categories []Category) []Category {
	visible := make([]Category, 0, len(categories))
	for _, cat := range categories {
		if cat.Tokens <= 0 {
			continue
		}
		if cat.Name == CategoryFreeSpace || cat.Name == CategoryAutocompactBuffer {
			continue
		}
		visible = append(visible, cat)
	}
	return visible
}

func FormatMarkdownTable(data Data, opts Options) string {
	var output strings.Builder

	output.WriteString("## Context Usage\n\n")
	fmt.Fprintf(&output, "**Model:** %s  \n", data.Model)
	fmt.Fprintf(
		&output,
		"**Tokens:** %s / %s (%d%%)\n",
		FormatTokens(data.TotalTokens),
		FormatTokens(data.RawMaxTokens),
		data.Percentage(),
	)

	writeCollapseStats(&output, data.CollapseStats)
	output.WriteString("\n")

	writeCategories(&output, data.Categories, data.RawMaxTokens)
	writeMCPTools(&output, data.MCPTools)
	if opts.AntOnly {
		writeSystemTools(&output, data.SystemTools)
		writeSystemPromptSections(&output, data.SystemPromptSections)
	}
	writeAgents(&output, data.Agents)
	writeMemoryFiles(&output, data.MemoryFiles)
	writeSkills(&output, data.Skills)
	if opts.AntOnly {
		writeMessageBreakdown(&output, data.MessageBreakdown)
	}

	return output.String()
}

func writeCollapseStats(output *strings.Builder, stats CollapseStats) {
	if !stats.Enabled {
		return
	}

	parts := make([]string, 0, 2)
	if stats.CollapsedSpans > 0 {
		parts = append(parts, fmt.Sprintf(
			"%d %s summarized (%d messages)",
			stats.CollapsedSpans,
			plural(stats.CollapsedSpans, "span"),
			stats.CollapsedMessages,
		))
	}
	if stats.StagedSpans > 0 {
		parts = append(parts, fmt.Sprintf("%d staged", stats.StagedSpans))
	}

	summary := "waiting for first trigger"
	if len(parts) > 0 {
		summary = strings.Join(parts, ", ")
	} else if stats.TotalSpawns > 0 {
		summary = fmt.Sprintf("%d %s, nothing staged yet", stats.TotalSpawns, plural(stats.TotalSpawns, "spawn"))
	}

	fmt.Fprintf(output, "**Context strategy:** collapse (%s)\n", summary)

	if stats.TotalErrors > 0 {
		fmt.Fprintf(output, "**Collapse errors:** %d/%d spawns failed", stats.TotalErrors, stats.TotalSpawns)
		if stats.LastError != "" {
			fmt.Fprintf(output, " (last: %s)", truncateRunes(stats.LastError, 80))
		}
		output.WriteString("\n")
	} else if stats.EmptySpawnWarningEmitted {
		fmt.Fprintf(output, "**Collapse idle:** %d consecutive empty runs\n", stats.TotalEmptySpawns)
	}
}

func writeCategories(output *strings.Builder, categories []Category, rawMaxTokens int) {
	visible := VisibleCategories(categories)
	if len(visible) == 0 {
		return
	}

	output.WriteString("### Estimated usage by category\n\n")
	output.WriteString("| Category | Tokens | Percentage |\n")
	output.WriteString("|----------|--------|------------|\n")

	for _, cat := range visible {
		fmt.Fprintf(output, "| %s | %s | %s |\n", cat.Name, FormatTokens(cat.Tokens), formatPercent(cat.Tokens, rawMaxTokens))
	}

	if freeSpace, ok := findCategory(categories, CategoryFreeSpace); ok && freeSpace.Tokens > 0 {
		fmt.Fprintf(output, "| %s | %s | %s |\n", CategoryFreeSpace, FormatTokens(freeSpace.Tokens), formatPercent(freeSpace.Tokens, rawMaxTokens))
	}

	if autocompact, ok := findCategory(categories, CategoryAutocompactBuffer); ok && autocompact.Tokens > 0 {
		fmt.Fprintf(output, "| %s | %s | %s |\n", CategoryAutocompactBuffer, FormatTokens(autocompact.Tokens), formatPercent(autocompact.Tokens, rawMaxTokens))
	}

	output.WriteString("\n")
}

func writeMCPTools(output *strings.Builder, tools []MCPTool) {
	if len(tools) == 0 {
		return
	}

	output.WriteString("### MCP Tools\n\n")
	output.WriteString("| Tool | Server | Tokens |\n")
	output.WriteString("|------|--------|--------|\n")
	for _, tool := range tools {
		fmt.Fprintf(output, "| %s | %s | %s |\n", tool.Name, tool.ServerName, FormatTokens(tool.Tokens))
	}
	output.WriteString("\n")
}

func writeSystemTools(output *strings.Builder, tools []TokenItem) {
	if len(tools) == 0 {
		return
	}

	output.WriteString("### [ANT-ONLY] System Tools\n\n")
	output.WriteString("| Tool | Tokens |\n")
	output.WriteString("|------|--------|\n")
	for _, tool := range tools {
		fmt.Fprintf(output, "| %s | %s |\n", tool.Name, FormatTokens(tool.Tokens))
	}
	output.WriteString("\n")
}

func writeSystemPromptSections(output *strings.Builder, sections []TokenItem) {
	if len(sections) == 0 {
		return
	}

	output.WriteString("### [ANT-ONLY] System Prompt Sections\n\n")
	output.WriteString("| Section | Tokens |\n")
	output.WriteString("|---------|--------|\n")
	for _, section := range sections {
		fmt.Fprintf(output, "| %s | %s |\n", section.Name, FormatTokens(section.Tokens))
	}
	output.WriteString("\n")
}

func writeAgents(output *strings.Builder, agents []Agent) {
	if len(agents) == 0 {
		return
	}

	output.WriteString("### Custom Agents\n\n")
	output.WriteString("| Agent Type | Source | Tokens |\n")
	output.WriteString("|------------|--------|--------|\n")
	for _, agent := range agents {
		fmt.Fprintf(output, "| %s | %s | %s |\n", agent.AgentType, agentSourceDisplayName(agent.Source), FormatTokens(agent.Tokens))
	}
	output.WriteString("\n")
}

func writeMemoryFiles(output *strings.Builder, files []MemoryFile) {
	if len(files) == 0 {
		return
	}

	output.WriteString("### Memory Files\n\n")
	output.WriteString("| Type | Path | Tokens |\n")
	output.WriteString("|------|------|--------|\n")
	for _, file := range files {
		fmt.Fprintf(output, "| %s | %s | %s |\n", file.Type, file.Path, FormatTokens(file.Tokens))
	}
	output.WriteString("\n")
}

func writeSkills(output *strings.Builder, skills SkillInfo) {
	if skills.Tokens <= 0 || len(skills.SkillFrontmatter) == 0 {
		return
	}

	output.WriteString("### Skills\n\n")
	output.WriteString("| Skill | Source | Tokens |\n")
	output.WriteString("|-------|--------|--------|\n")
	for _, skill := range skills.SkillFrontmatter {
		fmt.Fprintf(output, "| %s | %s | %s |\n", skill.Name, sourceDisplayName(skill.Source), FormatTokens(skill.Tokens))
	}
	output.WriteString("\n")
}

func writeMessageBreakdown(output *strings.Builder, breakdown *MessageBreakdown) {
	if breakdown == nil {
		return
	}

	output.WriteString("### [ANT-ONLY] Message Breakdown\n\n")
	output.WriteString("| Category | Tokens |\n")
	output.WriteString("|----------|--------|\n")
	fmt.Fprintf(output, "| Tool calls | %s |\n", FormatTokens(breakdown.ToolCallTokens))
	fmt.Fprintf(output, "| Tool results | %s |\n", FormatTokens(breakdown.ToolResultTokens))
	fmt.Fprintf(output, "| Attachments | %s |\n", FormatTokens(breakdown.AttachmentTokens))
	fmt.Fprintf(output, "| Assistant messages (non-tool) | %s |\n", FormatTokens(breakdown.AssistantMessageTokens))
	fmt.Fprintf(output, "| User messages (non-tool-result) | %s |\n", FormatTokens(breakdown.UserMessageTokens))
	output.WriteString("\n")

	if len(breakdown.ToolCallsByType) > 0 {
		output.WriteString("#### Top Tools\n\n")
		output.WriteString("| Tool | Call Tokens | Result Tokens |\n")
		output.WriteString("|------|-------------|---------------|\n")
		for _, tool := range breakdown.ToolCallsByType {
			fmt.Fprintf(output, "| %s | %s | %s |\n", tool.Name, FormatTokens(tool.CallTokens), FormatTokens(tool.ResultTokens))
		}
		output.WriteString("\n")
	}

	if len(breakdown.AttachmentsByType) > 0 {
		output.WriteString("#### Top Attachments\n\n")
		output.WriteString("| Attachment | Tokens |\n")
		output.WriteString("|------------|--------|\n")
		for _, attachment := range breakdown.AttachmentsByType {
			fmt.Fprintf(output, "| %s | %s |\n", attachment.Name, FormatTokens(attachment.Tokens))
		}
		output.WriteString("\n")
	}
}

func formatPercent(tokens, rawMaxTokens int) string {
	if rawMaxTokens <= 0 {
		return "0.0%"
	}
	return fmt.Sprintf("%.1f%%", float64(tokens)/float64(rawMaxTokens)*100)
}

func findCategory(categories []Category, name string) (Category, bool) {
	for _, cat := range categories {
		if cat.Name == name {
			return cat, true
		}
	}
	return Category{}, false
}

func sourceDisplayName(source string) string {
	switch source {
	case "userSettings":
		return "User"
	case "projectSettings":
		return "Project"
	case "localSettings":
		return "Local"
	case "flagSettings":
		return "Flag"
	case "policySettings":
		return "Managed"
	case "plugin":
		return "Plugin"
	case "built-in":
		return "Built-in"
	default:
		return source
	}
}

func agentSourceDisplayName(source string) string {
	if source == "policySettings" {
		return "Policy"
	}
	return sourceDisplayName(source)
}

func plural(count int, singular string) string {
	if count == 1 {
		return singular
	}
	return singular + "s"
}

func truncateRunes(s string, limit int) string {
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit])
}
