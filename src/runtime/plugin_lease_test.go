package runtime

import (
	"reflect"
	"testing"

	"awaken/src/hookruntime"
	"awaken/src/instructions"
)

func TestCollectBoundPluginNamesFromTokensHooksAndRules(t *testing.T) {
	names := collectBoundPluginNames(
		[]ComposerToken{
			{Type: "plugin", Payload: ComposerTokenPayload{PluginName: "acme.tools", PluginDigest: "digest"}},
			{Type: "skill", Payload: ComposerTokenPayload{SkillID: "plugin__notes__write", SourceOrigin: "plugin"}},
			{Type: "skill", Payload: ComposerTokenPayload{SkillID: "pptx", SourceOrigin: "user"}},
			{Type: "rule", Payload: ComposerTokenPayload{PluginName: "rules.pack", RuleLogicalID: "rule:always"}},
		},
		hookruntime.AvailabilitySnapshot{
			Sources: []hookruntime.SourceReceipt{{PluginName: "hook.pack", HookCount: 1}},
			HooksByEvent: map[hookruntime.Event][]hookruntime.CompiledHook{
				hookruntime.EventRunStart: {{PluginName: "hook.pack"}},
			},
		},
		[]instructions.ExtraSource{{DisplayPath: "plugin:rules.pack/rule:always"}},
	)
	want := []string{"acme.tools", "hook.pack", "plugin__notes__write", "rules.pack"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("bound names=%v want=%v", names, want)
	}
}

func TestCollectBoundPluginNamesEmptyWhenRunDoesNotUsePlugins(t *testing.T) {
	names := collectBoundPluginNames(nil, hookruntime.EmptySnapshot(), nil)
	if len(names) != 0 {
		t.Fatalf("empty run bound names=%v", names)
	}
}
