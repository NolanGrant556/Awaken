package dialect

import (
	"os"
	"path/filepath"
	"testing"

	"awaken/src/plugins/ir"
)

func TestVendorHookEventMapsAreExplicitAndDoNotInventEvents(t *testing.T) {
	want := map[string]map[string]string{
		"codex": {
			"SessionStart": "RunStart", "SessionEnd": "RunEnd", "UserPromptSubmit": "UserPromptSubmit",
			"PreToolUse": "PreToolUse", "PostToolUse": "PostToolUse", "PreCompact": "PreCompact",
			"PostCompact": "PostCompact", "SubagentStart": "SubagentStart", "SubagentStop": "SubagentStop", "Stop": "BeforeFinish",
		},
		"claude": {
			"SessionStart": "RunStart", "SessionEnd": "RunEnd", "UserPromptSubmit": "UserPromptSubmit",
			"PreToolUse": "PreToolUse", "PostToolUse": "PostToolUse", "PostToolUseFailure": "PostToolUseFailure",
			"PreCompact": "PreCompact", "PostCompact": "PostCompact", "SubagentStart": "SubagentStart", "SubagentStop": "SubagentStop", "Stop": "BeforeFinish",
		},
		"cursor": {
			"sessionStart": "RunStart", "sessionEnd": "RunEnd", "beforeSubmitPrompt": "UserPromptSubmit",
			"preToolUse": "PreToolUse", "postToolUse": "PostToolUse", "postToolUseFailure": "PostToolUseFailure",
			"preCompact": "PreCompact", "subagentStart": "SubagentStart", "subagentStop": "SubagentStop", "stop": "BeforeFinish",
		},
	}
	actual := map[string]map[string]string{"codex": codexHookEvents, "claude": claudeHookEvents, "cursor": cursorHookEvents}
	for vendor, mappings := range want {
		for source, semantic := range mappings {
			if got := actual[vendor][source]; got != semantic {
				t.Fatalf("%s %s=%q, want %q", vendor, source, got, semantic)
			}
		}
	}
	if _, invented := cursorHookEvents["postCompact"]; invented {
		t.Fatal("Cursor postCompact was invented")
	}
	if _, invented := codexHookEvents["PostToolUseFailure"]; invented {
		t.Fatal("Codex PostToolUseFailure was invented")
	}
}

func TestClaudeHookParserKeepsHandlerConditionAndAsyncRewake(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "hooks"), 0o700); err != nil {
		t.Fatal(err)
	}
	raw := `{"hooks":{"PostToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"python3 review.py","if":"Bash(git commit:*)","asyncRewake":true,"rewakeMessage":"review complete"}]}]}}`
	if err := os.WriteFile(filepath.Join(root, "hooks", "hooks.json"), []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	components, diagnostics := parseHookFileComponents(root, "hooks/hooks.json", ir.DialectClaudeCode, ".claude-plugin/plugin.json", ir.MappingAdapted, claudeHookEvents)
	if len(diagnostics) != 0 || len(components) != 1 || components[0].Hook == nil {
		t.Fatalf("components=%#v diagnostics=%#v", components, diagnostics)
	}
	hook := components[0].Hook
	if hook.Condition != "Bash(git commit:*)" || !hook.Async || !hook.AsyncRewake || hook.RewakeMessage != "review complete" {
		t.Fatalf("hook=%#v", hook)
	}
}
