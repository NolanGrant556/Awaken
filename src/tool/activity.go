package tool

import "strings"

// ToolActivity is a host-generated activity descriptor shown before/during tool execution.
type ToolActivity struct {
	Stage  string `json:"stage"`
	Title  string `json:"title"`
	Detail string `json:"detail,omitempty"`
}

// ActivityDescribingTool lets tools provide host-owned activity titles.
type ActivityDescribingTool interface {
	DescribeActivity(input string) ToolActivity
}

// DefaultActivity returns a stable fallback activity for tools without DescribeActivity.
func DefaultActivity(toolName string) ToolActivity {
	name := strings.TrimSpace(toolName)
	if name == "" {
		name = "tool"
	}
	return ToolActivity{
		Stage: "started",
		Title: "正在执行 " + name,
	}
}
