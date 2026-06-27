package lifecyclehooks

import "testing"

func TestParseConfigFileSupportsRuntimeHookEvents(t *testing.T) {
	cfg := ParseConfigFile(`
[llm]
model_id = "ignored"

[runtime.hooks.before_run]
command = "/tmp/before-run"
args = ["one", "two"]
timeout_ms = 25

[[runtime.hooks.after_tool]]
command = "/tmp/after-tool"
enabled = false
`)

	beforeRun := cfg.CommandsFor(EventBeforeRun)
	if len(beforeRun) != 1 {
		t.Fatalf("expected one before_run hook, got %+v", beforeRun)
	}
	if beforeRun[0].Command != "/tmp/before-run" {
		t.Fatalf("unexpected command: %+v", beforeRun[0])
	}
	if len(beforeRun[0].Args) != 2 || beforeRun[0].Args[0] != "one" || beforeRun[0].Args[1] != "two" {
		t.Fatalf("unexpected args: %+v", beforeRun[0].Args)
	}
	if beforeRun[0].TimeoutMS != 25 {
		t.Fatalf("unexpected timeout: %+v", beforeRun[0])
	}

	afterTool := cfg.CommandsFor(EventAfterTool)
	if len(afterTool) != 1 {
		t.Fatalf("expected one after_tool hook, got %+v", afterTool)
	}
	if afterTool[0].Enabled == nil || *afterTool[0].Enabled {
		t.Fatalf("expected after_tool hook to be disabled, got %+v", afterTool[0])
	}
	if cfg.HasHooks() != true {
		t.Fatalf("enabled before_run hook should make config active")
	}
}

func TestParseConfigFileIgnoresUnsupportedHookEvents(t *testing.T) {
	cfg := ParseConfigFile(`
[runtime.hooks.before_prompt]
command = "/tmp/nope"
`)

	if cfg.HasHooks() {
		t.Fatalf("unsupported hook event should be ignored, got %+v", cfg)
	}
}
