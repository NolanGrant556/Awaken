package prompt

import (
	"fmt"
	"strings"
)

type SystemPrompt struct {
	staticContent string
	runtimeInfo   RuntimeInfo
}

func NewSystemPrompt(staticContent string) *SystemPrompt {
	return &SystemPrompt{
		staticContent: staticContent,
		runtimeInfo:   NewRuntimeInfo(),
	}
}

func (p *SystemPrompt) SetRuntimeInfo(info RuntimeInfo) {
	p.runtimeInfo = info
}

func (p *SystemPrompt) GetRuntimeInfo() RuntimeInfo {
	return p.runtimeInfo
}

func (p *SystemPrompt) Build() string {
	return p.buildEffectivePrompt()
}

func (p *SystemPrompt) buildEffectivePrompt() string {
	runtimeInfoStr := FormatRuntimeInfo(p.runtimeInfo)

	return replaceTemplateVar(p.staticContent, runtimeInfoStr)
}

// FormatRuntimeInfo returns the exact provider-visible replacement for the
// RuntimeInfo placeholder. Keeping this formatter public lets context
// accounting attribute the runtime leaf without parsing prompt prose.
func FormatRuntimeInfo(info RuntimeInfo) string {
	runtimeInfoStr := fmt.Sprintf(
		"### 当前运行环境\n\n"+
			"- **工作目录**: %s\n"+
			"- **当前日期**: %s\n"+
			"- **当前模型**: %s\n"+
			"- **入口方式**: %s\n"+
			"- **可用工具**: %s\n"+
			"- **可用 Skills**: %s",
		info.WorkingDir,
		info.CurrentDate,
		info.ModelName,
		info.EntryPoint,
		info.ToolSummary,
		info.SkillSummary,
	)
	if len(info.AttachedFiles) > 0 {
		runtimeInfoStr += "\n- **已授权附件**:\n" + formatRuntimeAttachedFiles(info.AttachedFiles)
	}
	return runtimeInfoStr
}

func formatRuntimeAttachedFiles(files []RuntimeAttachedFile) string {
	var builder strings.Builder
	for _, file := range files {
		path := strings.TrimSpace(file.Path)
		if path == "" {
			continue
		}
		displayName := strings.TrimSpace(file.DisplayName)
		if displayName == "" {
			displayName = path
		}
		builder.WriteString(fmt.Sprintf("  - %s: %s\n", displayName, path))
	}
	return strings.TrimRight(builder.String(), "\n")
}

func replaceTemplateVar(template, value string) string {
	return replaceString(template, "{{.RuntimeInfo}}", value)
}

func replaceString(s, old, new string) string {
	result := ""
	for {
		idx := indexOf(s, old)
		if idx == -1 {
			result += s
			break
		}
		result += s[:idx] + new
		s = s[idx+len(old):]
	}
	return result
}

func indexOf(s, substr string) int {
	if len(substr) == 0 {
		return 0
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
