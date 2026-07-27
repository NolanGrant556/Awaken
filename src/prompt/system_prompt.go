package prompt

import (
	"fmt"
	"html"
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
	builder.WriteString("<environment_context>\n")
	projectID := strings.TrimSpace(info.ProjectID)
	projectName := strings.TrimSpace(info.ProjectName)
	if projectID != "" || projectName != "" {
		builder.WriteString(fmt.Sprintf("  <project id=\"%s\" name=\"%s\" />\n", xmlEscape(projectID), xmlEscape(projectName)))
	} else {
		builder.WriteString("  <scope kind=\"conversation\" />\n")
	}
	builder.WriteString(fmt.Sprintf("  <cwd>%s</cwd>\n", xmlEscape(info.WorkingDir)))
	builder.WriteString(formatRuntimeProjectFolders(info))
	builder.WriteString("  <permission_profile type=\"disabled\">\n")
	builder.WriteString("    <file_system type=\"unrestricted\" />\n")
	builder.WriteString("  </permission_profile>\n")
	builder.WriteString("</environment_context>\n\n")
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

// formatRuntimeProjectFolders renders the exact Host-owned roots. Even a single
// root is kept: the model must not infer a broader common parent from prose.
func formatRuntimeProjectFolders(info RuntimeInfo) string {
	if len(info.ProjectFolders) == 0 {
		if strings.TrimSpace(info.ProjectName) == "" {
			return fmt.Sprintf("  <workspace_roots mode=\"managed\">\n    <root primary=\"true\">%s</root>\n  </workspace_roots>\n", xmlEscape(info.WorkingDir))
		}
		return fmt.Sprintf("  <workspace_roots mode=\"managed\">\n    <root primary=\"true\">%s</root>\n  </workspace_roots>\n  <workspace_note>该项目未绑定文件夹，工作目录是 Awaken 为它准备的会话目录</workspace_note>\n", xmlEscape(info.WorkingDir))
	}
	var builder strings.Builder
	builder.WriteString("  <workspace_roots>\n")
	for _, folder := range info.ProjectFolders {
		path := strings.TrimSpace(folder.Path)
		if path == "" {
			continue
		}
		builder.WriteString(fmt.Sprintf("    <root name=\"%s\" primary=\"%t\">%s</root>\n", xmlEscape(filepath.Base(path)), folder.IsPrimary, xmlEscape(path)))
	}
	builder.WriteString("  </workspace_roots>\n")
	return builder.String()
}

func xmlEscape(value string) string {
	return html.EscapeString(strings.TrimSpace(value))
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
