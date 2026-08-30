package pluginhooks

import (
	"path/filepath"
	"testing"

	"awaken/src/hookruntime"
	"awaken/src/plugins/ir"
)

func TestCompileCommandExpandsRootAndRejectsShellOperators(t *testing.T) {
	root := t.TempDir()
	component := ir.Component{
		LogicalID: "hook:hooks.json:PreToolUse/one", LogicalIDEncoded: "encoded", RevisionID: "revision",
		Kind: ir.KindHook, Mapping: ir.MappingAdapted, Validation: "valid",
		Hook: &ir.HookSpec{SourceEvent: "PreToolUse", MappedEvent: "PreToolUse", HandlerType: "command", Command: `python3 "${CLAUDE_PLUGIN_ROOT}/hooks/pre.py"`, TimeoutSeconds: 2},
	}
	hooks, diagnostics := Compile([]Entry{{PluginName: "hookify", PluginDigest: "digest", PluginRoot: root, PluginData: filepath.Join(root, "data"), ProjectRoot: root, Dialect: ir.DialectClaudeCode, Component: component}})
	if len(diagnostics) != 0 || len(hooks) != 1 {
		t.Fatalf("hooks=%#v diagnostics=%#v", hooks, diagnostics)
	}
	if got := hooks[0].Command.Command; len(got) != 2 || got[0] != "python3" || got[1] != filepath.Join(root, "hooks", "pre.py") {
		t.Fatalf("argv=%#v", got)
	}
	if hooks[0].SourceKind != hookruntime.SourcePlugin || hooks[0].StableID != "plugin:hookify:"+component.LogicalID {
		t.Fatalf("identity=%#v", hooks[0])
	}
	component.Hook.Command = "python3 ok.py && touch bad"
	if compiled, diagnostics := Compile([]Entry{{PluginName: "bad", PluginRoot: root, PluginData: root, ProjectRoot: root, Dialect: ir.DialectClaudeCode, Component: component}}); len(compiled) != 0 || len(diagnostics) != 1 {
		t.Fatalf("shell operator accepted: %#v %#v", compiled, diagnostics)
	}
}

func TestCompilePreservesVendorHookTimeout(t *testing.T) {
	component := ir.Component{
		LogicalID: "hook:hooks.json:SessionStart/slow", LogicalIDEncoded: "encoded", RevisionID: "revision",
		Kind: ir.KindHook, Mapping: ir.MappingAdapted, Validation: "valid",
		Hook: &ir.HookSpec{SourceEvent: "SessionStart", MappedEvent: "RunStart", HandlerType: "command", Command: "python3 slow.py", TimeoutSeconds: 180},
	}
	hooks, diagnostics := Compile([]Entry{{PluginName: "slow", PluginRoot: t.TempDir(), PluginData: t.TempDir(), ProjectRoot: t.TempDir(), Dialect: ir.DialectClaudeCode, Component: component}})
	if len(diagnostics) != 0 || len(hooks) != 1 {
		t.Fatalf("hooks=%#v diagnostics=%#v", hooks, diagnostics)
	}
	if hooks[0].Command.TimeoutMS != 180_000 {
		t.Fatalf("timeout=%d want=%d", hooks[0].Command.TimeoutMS, 180_000)
	}
}

func TestEffectiveTimeoutUsesVendorCompatibleDefaults(t *testing.T) {
	if got := EffectiveTimeoutMS(nil); got != DefaultPluginHookTimeoutMS {
		t.Fatalf("default timeout=%d want=%d", got, DefaultPluginHookTimeoutMS)
	}
	if got := EffectiveTimeoutMS(&ir.HookSpec{SourceEvent: "PreToolUse"}); got != DefaultPluginHookTimeoutMS {
		t.Fatalf("ordinary timeout=%d want=%d", got, DefaultPluginHookTimeoutMS)
	}
	if got := EffectiveTimeoutMS(&ir.HookSpec{SourceEvent: "SessionEnd"}); got != DefaultPluginSessionEndTimeoutMS {
		t.Fatalf("SessionEnd default=%d want=%d", got, DefaultPluginSessionEndTimeoutMS)
	}
	if got := EffectiveTimeoutMS(&ir.HookSpec{SourceEvent: "sessionEnd", TimeoutSeconds: 180}); got != MaxPluginSessionEndTimeoutMS {
		t.Fatalf("SessionEnd cap=%d want=%d", got, MaxPluginSessionEndTimeoutMS)
	}
}

func TestCompilePreservesAsyncRewakeAndHandlerInputCondition(t *testing.T) {
	root := t.TempDir()
	component := ir.Component{
		LogicalID: "hook:hooks.json:PostToolUse/review", LogicalIDEncoded: "encoded", RevisionID: "revision",
		Kind: ir.KindHook, Mapping: ir.MappingAdapted, Validation: "valid",
		Hook: &ir.HookSpec{
			SourceEvent: "PostToolUse", MappedEvent: "PostToolUse", HandlerType: "command",
			Matcher: "Bash", Condition: "Bash(git commit:*)", Command: "python3 review.py",
			Async: true, AsyncRewake: true, RewakeMessage: "review complete",
		},
	}
	hooks, diagnostics := Compile([]Entry{{PluginName: "review", PluginRoot: root, PluginData: root, ProjectRoot: root, Dialect: ir.DialectClaudeCode, Component: component}})
	if len(diagnostics) != 0 || len(hooks) != 1 {
		t.Fatalf("hooks=%#v diagnostics=%#v", hooks, diagnostics)
	}
	if hooks[0].Matcher.InputPattern != "git commit*" || len(hooks[0].Matcher.ToolNames) != 1 || hooks[0].Matcher.ToolNames[0] != "Bash" {
		t.Fatalf("matcher=%#v", hooks[0].Matcher)
	}
	if !hooks[0].Command.Async || !hooks[0].Command.AsyncRewake || hooks[0].Command.RewakeMessage != "review complete" {
		t.Fatalf("command=%#v", hooks[0].Command)
	}
}

func TestCompileForcesSessionEndHookToRemainSynchronous(t *testing.T) {
	root := t.TempDir()
	component := ir.Component{
		LogicalID: "hook:hooks.json:SessionEnd/cleanup", LogicalIDEncoded: "encoded", RevisionID: "revision",
		Kind: ir.KindHook, Mapping: ir.MappingAdapted, Validation: "valid",
		Hook: &ir.HookSpec{SourceEvent: "SessionEnd", MappedEvent: "RunEnd", HandlerType: "command", Command: "python3 cleanup.py", Async: true, AsyncRewake: true},
	}
	hooks, diagnostics := Compile([]Entry{{PluginName: "cleanup", PluginRoot: root, PluginData: root, ProjectRoot: root, Dialect: ir.DialectClaudeCode, Component: component}})
	if len(diagnostics) != 0 || len(hooks) != 1 || hooks[0].Command.Async || hooks[0].Command.AsyncRewake {
		t.Fatalf("hooks=%#v diagnostics=%#v", hooks, diagnostics)
	}
}
