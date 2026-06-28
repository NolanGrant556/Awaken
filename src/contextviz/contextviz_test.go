package contextviz

import (
	"reflect"
	"strings"
	"testing"
)

func TestPercentage(t *testing.T) {
	tests := []struct {
		name         string
		tokens       int
		rawMaxTokens int
		want         int
	}{
		{name: "rounds to whole percent", tokens: 25, rawMaxTokens: 200, want: 13},
		{name: "exact percent", tokens: 50, rawMaxTokens: 200, want: 25},
		{name: "zero max", tokens: 50, rawMaxTokens: 0, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Percentage(tt.tokens, tt.rawMaxTokens); got != tt.want {
				t.Fatalf("Percentage(%d, %d) = %d, want %d", tt.tokens, tt.rawMaxTokens, got, tt.want)
			}
		})
	}
}

func TestFormatTokens(t *testing.T) {
	tests := []struct {
		count int
		want  string
	}{
		{count: 999, want: "999"},
		{count: 1000, want: "1k"},
		{count: 1200, want: "1.2k"},
		{count: 999950, want: "1m"},
		{count: 1200000, want: "1.2m"},
		{count: -1200, want: "-1.2k"},
	}

	for _, tt := range tests {
		if got := FormatTokens(tt.count); got != tt.want {
			t.Fatalf("FormatTokens(%d) = %q, want %q", tt.count, got, tt.want)
		}
	}
}

func TestCategoriesOrderFreeAndAutocompact(t *testing.T) {
	categories := []Category{
		{Name: CategoryFreeSpace, Tokens: 5600},
		{Name: "System prompt", Tokens: 1200},
		{Name: CategoryAutocompactBuffer, Tokens: 1000},
		{Name: "Messages", Tokens: 2200},
		{Name: "MCP tools", Tokens: 0},
	}

	visible := VisibleCategories(categories)
	gotNames := make([]string, 0, len(visible))
	for _, cat := range visible {
		gotNames = append(gotNames, cat.Name)
	}
	wantNames := []string{"System prompt", "Messages"}
	if !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("visible category names = %v, want %v", gotNames, wantNames)
	}

	output := FormatMarkdownTable(Data{
		Model:        "claude-sonnet",
		TotalTokens:  3400,
		RawMaxTokens: 10000,
		Categories:   categories,
	}, Options{})

	assertOrdered(t, output,
		"| System prompt | 1.2k | 12.0% |",
		"| Messages | 2.2k | 22.0% |",
		"| Free space | 5.6k | 56.0% |",
		"| Autocompact buffer | 1k | 10.0% |",
	)
	if strings.Contains(output, "| MCP tools |") {
		t.Fatalf("zero-token category should not be rendered:\n%s", output)
	}
}

func TestAntOnlyGating(t *testing.T) {
	data := Data{
		Model:        "claude-sonnet",
		RawMaxTokens: 200000,
		SystemTools:  []TokenItem{{Name: "Read", Tokens: 300}},
		SystemPromptSections: []TokenItem{
			{Name: "base", Tokens: 700},
		},
		MessageBreakdown: &MessageBreakdown{
			ToolCallTokens:         10,
			ToolResultTokens:       20,
			AssistantMessageTokens: 30,
			UserMessageTokens:      40,
		},
	}

	withoutAnt := FormatMarkdownTable(data, Options{})
	if strings.Contains(withoutAnt, "[ANT-ONLY]") || strings.Contains(withoutAnt, "Message Breakdown") {
		t.Fatalf("ant-only sections should be hidden without AntOnly option:\n%s", withoutAnt)
	}

	withAnt := FormatMarkdownTable(data, Options{AntOnly: true})
	for _, want := range []string{
		"### [ANT-ONLY] System Tools",
		"| Read | 300 |",
		"### [ANT-ONLY] System Prompt Sections",
		"| base | 700 |",
		"### [ANT-ONLY] Message Breakdown",
		"| Tool calls | 10 |",
	} {
		if !strings.Contains(withAnt, want) {
			t.Fatalf("ant output missing %q:\n%s", want, withAnt)
		}
	}
}

func TestDetailSections(t *testing.T) {
	output := FormatMarkdownTable(Data{
		Model:        "claude-sonnet",
		RawMaxTokens: 200000,
		MCPTools: []MCPTool{
			{Name: "lookup", ServerName: "docs", Tokens: 1200},
		},
		Agents: []Agent{
			{AgentType: "planner", Source: "projectSettings", Tokens: 400},
			{AgentType: "policy-agent", Source: "policySettings", Tokens: 300},
		},
		MemoryFiles: []MemoryFile{
			{Type: "project", Path: "/repo/AGENTS.md", Tokens: 800},
		},
		Skills: SkillInfo{
			Tokens: 600,
			SkillFrontmatter: []SkillFrontmatter{
				{Name: "reviewer", Source: "policySettings", Tokens: 600},
			},
		},
	}, Options{})

	for _, want := range []string{
		"### MCP Tools",
		"| lookup | docs | 1.2k |",
		"### Custom Agents",
		"| planner | Project | 400 |",
		"| policy-agent | Policy | 300 |",
		"### Memory Files",
		"| project | /repo/AGENTS.md | 800 |",
		"### Skills",
		"| reviewer | Managed | 600 |",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestCollapseStatsStrings(t *testing.T) {
	t.Run("summarized and staged", func(t *testing.T) {
		output := FormatMarkdownTable(Data{
			Model:        "claude-sonnet",
			RawMaxTokens: 200000,
			CollapseStats: CollapseStats{
				Enabled:           true,
				CollapsedSpans:    2,
				CollapsedMessages: 12,
				StagedSpans:       1,
			},
		}, Options{})

		want := "**Context strategy:** collapse (2 spans summarized (12 messages), 1 staged)"
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	})

	t.Run("errors include truncated last error", func(t *testing.T) {
		lastError := strings.Repeat("x", 90)
		output := FormatMarkdownTable(Data{
			Model:        "claude-sonnet",
			RawMaxTokens: 200000,
			CollapseStats: CollapseStats{
				Enabled:     true,
				TotalSpawns: 3,
				TotalErrors: 1,
				LastError:   lastError,
			},
		}, Options{})

		want := "**Collapse errors:** 1/3 spawns failed (last: " + strings.Repeat("x", 80) + ")"
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	})

	t.Run("idle warning", func(t *testing.T) {
		output := FormatMarkdownTable(Data{
			Model:        "claude-sonnet",
			RawMaxTokens: 200000,
			CollapseStats: CollapseStats{
				Enabled:                  true,
				TotalSpawns:              2,
				EmptySpawnWarningEmitted: true,
				TotalEmptySpawns:         4,
			},
		}, Options{})

		for _, want := range []string{
			"**Context strategy:** collapse (2 spawns, nothing staged yet)",
			"**Collapse idle:** 4 consecutive empty runs",
		} {
			if !strings.Contains(output, want) {
				t.Fatalf("output missing %q:\n%s", want, output)
			}
		}
	})

	t.Run("waiting for first trigger", func(t *testing.T) {
		output := FormatMarkdownTable(Data{
			Model:        "claude-sonnet",
			RawMaxTokens: 200000,
			CollapseStats: CollapseStats{
				Enabled: true,
			},
		}, Options{})

		want := "**Context strategy:** collapse (waiting for first trigger)"
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	})
}

func TestEmptyCategoriesMinimalOutput(t *testing.T) {
	output := FormatMarkdownTable(Data{
		Model:        "claude-sonnet",
		RawMaxTokens: 200000,
	}, Options{})

	want := "## Context Usage\n\n**Model:** claude-sonnet  \n**Tokens:** 0 / 200k (0%)\n\n"
	if output != want {
		t.Fatalf("minimal output mismatch:\ngot:\n%q\nwant:\n%q", output, want)
	}
}

func assertOrdered(t *testing.T, haystack string, needles ...string) {
	t.Helper()

	lastIndex := -1
	for _, needle := range needles {
		index := strings.Index(haystack, needle)
		if index == -1 {
			t.Fatalf("output missing %q:\n%s", needle, haystack)
		}
		if index <= lastIndex {
			t.Fatalf("output does not preserve expected order for %q:\n%s", needle, haystack)
		}
		lastIndex = index
	}
}
