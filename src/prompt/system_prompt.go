package prompt

import (
	"fmt"
	"path/filepath"
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
	var builder strings.Builder
	builder.WriteString("### 当前运行环境\n\n")
	if name := strings.TrimSpace(info.ProjectName); name != "" {
		builder.WriteString(fmt.Sprintf("- **当前项目**: %s\n", name))
	}
	builder.WriteString(fmt.Sprintf("- **工作目录**: %s\n", info.WorkingDir))
	if folders := formatRuntimeProjectFolders(info); folders != "" {
		builder.WriteString(folders)
	}
	builder.WriteString(fmt.Sprintf(
		"- **当前日期**: %s\n"+
			"- **当前模型**: %s\n"+
			"- **入口方式**: %s\n"+
			"- **可用工具**: %s\n"+
			"- **可用 Skills**: %s",
		info.CurrentDate,
		info.ModelName,
		info.EntryPoint,
		info.ToolSummary,
		info.SkillSummary,
	))
	if len(info.AttachedFiles) > 0 {
		builder.WriteString("\n- **已授权附件**:\n" + formatRuntimeAttachedFiles(info.AttachedFiles))
	}
	return builder.String()
}

// formatRuntimeProjectFolders renders the project's folder group. A single
// folder is left out because the working directory line already names it; the
// listing exists so a multi-folder project stops looking like a lone directory
// with unrelated siblings above it.
func formatRuntimeProjectFolders(info RuntimeInfo) string {
	if len(info.ProjectFolders) == 0 {
		if strings.TrimSpace(info.ProjectName) == "" {
			return ""
		}
		return "- **项目文件夹**: 无（该项目未绑定文件夹，工作目录是 Awaken 为它准备的会话目录）\n"
	}
	if len(info.ProjectFolders) < 2 {
		return ""
	}
	var builder strings.Builder
	builder.WriteString("- **项目文件夹**:\n")
	for _, folder := range info.ProjectFolders {
		path := strings.TrimSpace(folder.Path)
		if path == "" {
			continue
		}
		suffix := ""
		if folder.IsPrimary {
			suffix = "（主文件夹，即当前工作目录）"
		}
		builder.WriteString(fmt.Sprintf("  - %s%s: %s\n", filepath.Base(path), suffix, path))
	}
	return builder.String()
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
