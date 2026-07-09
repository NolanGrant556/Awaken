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
	runtimeInfoStr := fmt.Sprintf(
		"### 当前运行环境\n\n"+
			"- **工作目录**: %s\n"+
			"- **当前日期**: %s\n"+
			"- **当前模型**: %s\n"+
			"- **入口方式**: %s\n"+
			"- **可用工具**: %s\n"+
			"- **可用 Skills**: %s",
		p.runtimeInfo.WorkingDir,
		p.runtimeInfo.CurrentDate,
		p.runtimeInfo.ModelName,
		p.runtimeInfo.EntryPoint,
		p.runtimeInfo.ToolSummary,
		p.runtimeInfo.SkillSummary,
	)
	if len(p.runtimeInfo.AttachedFiles) > 0 {
		runtimeInfoStr += "\n- **已授权附件**:\n" + formatRuntimeAttachedFiles(p.runtimeInfo.AttachedFiles)
	}

	return replaceTemplateVar(p.staticContent, runtimeInfoStr)
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
