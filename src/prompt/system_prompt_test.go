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

func TestLoadDefaultPrompt(t *testing.T) {
	content, err := LoadDefaultPrompt()
	if err != nil {
		t.Fatalf("expected default prompt to load: %v", err)
	}
	if !strings.Contains(content, "{{.RuntimeInfo}}") {
		t.Fatal("expected default prompt to contain runtime placeholder")
	}
}
