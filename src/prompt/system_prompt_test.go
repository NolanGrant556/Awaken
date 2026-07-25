package prompt

import (
	"strings"
	"testing"
)

func TestBuildEffectiveSystemPrompt(t *testing.T) {
	staticContent := "# Test Agent\n\n## Identity\n\n你是一个测试助手。\n\n{{.RuntimeInfo}}"

	runtimeInfo := NewRuntimeInfo()
	runtimeInfo.SetModelName("test-model")
	runtimeInfo.SetEntryPoint("test-entry")
	runtimeInfo.SetToolSummary("tool1, tool2")

	effectivePrompt := BuildEffectiveSystemPrompt(staticContent, runtimeInfo)

	if len(effectivePrompt) == 0 {
		t.Fatal("expected non-empty effective prompt")
	}

	if !strings.Contains(effectivePrompt, "测试助手") {
		t.Fatal("expected prompt to contain '测试助手'")
	}

	if !strings.Contains(effectivePrompt, "test-model") {
		t.Fatal("expected prompt to contain model name")
	}

	if !strings.Contains(effectivePrompt, "test-entry") {
		t.Fatal("expected prompt to contain entry point")
	}

	if strings.Contains(effectivePrompt, "{{.RuntimeInfo}}") {
		t.Fatal("expected template variable to be replaced")
	}
}

func TestRuntimeInfoUpdates(t *testing.T) {
	runtimeInfo := NewRuntimeInfo()

	runtimeInfo.UpdateCurrentDate()
	if runtimeInfo.CurrentDate == "" {
		t.Fatal("expected non-empty current date")
	}

	runtimeInfo.SetModelName("new-model")
	if runtimeInfo.ModelName != "new-model" {
		t.Fatal("expected model name to be updated")
	}

	runtimeInfo.SetEntryPoint("desktop")
	if runtimeInfo.EntryPoint != "desktop" {
		t.Fatal("expected entry point to be updated")
	}

	runtimeInfo.SetToolSummary("tool1")
	if runtimeInfo.ToolSummary != "tool1" {
		t.Fatal("expected tool summary to be updated")
	}
}

func TestRuntimeDateUsesDayGranularityForStablePromptPrefix(t *testing.T) {
	info := NewRuntimeInfo()
	if len(info.CurrentDate) != len("2006-01-02") {
		t.Fatalf("runtime date must be day-granular, got %q", info.CurrentDate)
	}
	if strings.Contains(info.CurrentDate, ":") || strings.Contains(info.CurrentDate, " ") {
		t.Fatalf("runtime date leaked time-of-day into system prefix: %q", info.CurrentDate)
	}
}

func TestBuildEffectiveSystemPromptIncludesAttachedFiles(t *testing.T) {
	runtimeInfo := NewRuntimeInfo()
	runtimeInfo.SetAttachedFiles([]RuntimeAttachedFile{
		{Path: "/Users/nolangrant/Desktop/report.pdf", DisplayName: "report.pdf"},
	})

	effectivePrompt := BuildEffectiveSystemPrompt("{{.RuntimeInfo}}", runtimeInfo)

	if !strings.Contains(effectivePrompt, "已授权附件") {
		t.Fatal("expected prompt to include attached file section")
	}
	if !strings.Contains(effectivePrompt, "report.pdf: /Users/nolangrant/Desktop/report.pdf") {
		t.Fatalf("expected prompt to include attached file path, got %s", effectivePrompt)
	}
}

func TestRuntimeInfoNamesTheProjectAndItsFolderGroup(t *testing.T) {
	runtimeInfo := NewRuntimeInfo()
	runtimeInfo.SetWorkingDir("/data/公司/09_经营管理部/01_汇总")
	runtimeInfo.SetProjectName("测试项目")
	runtimeInfo.SetProjectFolders([]RuntimeProjectFolder{
		{Path: "/data/公司/09_经营管理部/01_汇总", IsPrimary: true},
		{Path: "/data/公司/09_经营管理部/02_跟踪"},
	})

	rendered := FormatRuntimeInfo(runtimeInfo)

	if !strings.Contains(rendered, "**当前项目**: 测试项目") {
		t.Fatalf("project name missing: %s", rendered)
	}
	if !strings.Contains(rendered, "01_汇总（主文件夹，即当前工作目录）: /data/公司/09_经营管理部/01_汇总") {
		t.Fatalf("primary folder not marked: %s", rendered)
	}
	if !strings.Contains(rendered, "02_跟踪: /data/公司/09_经营管理部/02_跟踪") {
		t.Fatalf("secondary folder missing: %s", rendered)
	}
}

func TestRuntimeInfoOmitsFolderListForASingleFolderProject(t *testing.T) {
	runtimeInfo := NewRuntimeInfo()
	runtimeInfo.SetWorkingDir("/data/solo")
	runtimeInfo.SetProjectName("Solo")
	runtimeInfo.SetProjectFolders([]RuntimeProjectFolder{{Path: "/data/solo", IsPrimary: true}})

	rendered := FormatRuntimeInfo(runtimeInfo)

	// The working directory line already names it; repeating it as a group of
	// one only adds noise.
	if strings.Contains(rendered, "项目文件夹") {
		t.Fatalf("single folder should not render a folder group: %s", rendered)
	}
	if !strings.Contains(rendered, "**当前项目**: Solo") {
		t.Fatalf("project name missing: %s", rendered)
	}
}

func TestRuntimeInfoSaysAFolderlessProjectRunsInItsSessionDirectory(t *testing.T) {
	runtimeInfo := NewRuntimeInfo()
	runtimeInfo.SetProjectName("空项目")
	runtimeInfo.SetProjectFolders(nil)

	rendered := FormatRuntimeInfo(runtimeInfo)

	if !strings.Contains(rendered, "**项目文件夹**: 无（该项目未绑定文件夹") {
		t.Fatalf("folder-less projects must say so: %s", rendered)
	}
}

func TestRuntimeInfoStaysSilentAboutProjectsWhenThereIsNone(t *testing.T) {
	rendered := FormatRuntimeInfo(NewRuntimeInfo())

	if strings.Contains(rendered, "当前项目") || strings.Contains(rendered, "项目文件夹") {
		t.Fatalf("a run without a project must not claim one: %s", rendered)
	}
}

func TestDefaultPromptStatesTheWorkingScopeConvention(t *testing.T) {
	content, err := LoadDefaultPrompt()
	if err != nil {
		t.Fatalf("expected default prompt to load: %v", err)
	}
	for _, expected := range []string{
		"项目文件夹与作业范围",
		"默认在主文件夹作业",
		"跨文件夹检索省略",
		"不是访问边界",
	} {
		if !strings.Contains(content, expected) {
			t.Fatalf("default prompt lost the working-scope convention %q", expected)
		}
	}
}

func TestLoadDefaultPrompt(t *testing.T) {
	content, err := LoadDefaultPrompt()
	if err != nil {
		t.Fatalf("expected default prompt to load: %v", err)
	}
	if !strings.Contains(content, "{{.RuntimeInfo}}") {
		t.Fatal("expected default prompt to contain runtime placeholder")
	}
}
