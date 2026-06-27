package permissionrules

import "testing"

func TestParseRuleValueSimple(t *testing.T) {
	got := ParseRuleValue("Bash")

	if got.ToolName != "Bash" {
		t.Fatalf("ToolName = %q, want Bash", got.ToolName)
	}
	if got.RuleContent != nil {
		t.Fatalf("RuleContent = %q, want nil", *got.RuleContent)
	}
}

func TestParseRuleValueSpecific(t *testing.T) {
	got := ParseRuleValue("Bash(npm test)")

	if got.ToolName != "Bash" {
		t.Fatalf("ToolName = %q, want Bash", got.ToolName)
	}
	if got.RuleContent == nil || *got.RuleContent != "npm test" {
		t.Fatalf("RuleContent = %v, want npm test", got.RuleContent)
	}
}

func TestParseRuleValueEscaped(t *testing.T) {
	got := ParseRuleValue(`Bash(python -c "print\(1\)")`)

	if got.ToolName != "Bash" {
		t.Fatalf("ToolName = %q, want Bash", got.ToolName)
	}
	if got.RuleContent == nil || *got.RuleContent != `python -c "print(1)"` {
		t.Fatalf("RuleContent = %v, want escaped parentheses unescaped", got.RuleContent)
	}
}

func TestParseRuleValueMalformed(t *testing.T) {
	cases := []string{
		"Bash(npm test",
		"Bash(npm test)extra",
		"(npm test)",
	}

	for _, tc := range cases {
		t.Run(tc, func(t *testing.T) {
			got := ParseRuleValue(tc)
			if got.ToolName != tc {
				t.Fatalf("ToolName = %q, want original value", got.ToolName)
			}
			if got.RuleContent != nil {
				t.Fatalf("RuleContent = %q, want nil", *got.RuleContent)
			}
		})
	}
}

func TestParseRuleValueEmptyAndWildcardAreToolWide(t *testing.T) {
	cases := []string{"Bash()", "Bash(*)"}

	for _, tc := range cases {
		t.Run(tc, func(t *testing.T) {
			got := ParseRuleValue(tc)
			if got.ToolName != "Bash" {
				t.Fatalf("ToolName = %q, want Bash", got.ToolName)
			}
			if got.RuleContent != nil {
				t.Fatalf("RuleContent = %q, want nil", *got.RuleContent)
			}
			if got.String() != "Bash" {
				t.Fatalf("String() = %q, want Bash", got.String())
			}
		})
	}
}

func TestRuleValueStringEscapesContent(t *testing.T) {
	value := RuleValue{ToolName: "Bash", RuleContent: ptr(`python -c "print(1)" && echo \done`)}

	got := value.String()
	want := `Bash(python -c "print\(1\)" && echo \\done)`
	if got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}

	roundTrip := ParseRuleValue(got)
	if roundTrip.RuleContent == nil || *roundTrip.RuleContent != *value.RuleContent {
		t.Fatalf("round trip content = %v, want %q", roundTrip.RuleContent, *value.RuleContent)
	}
}

func TestIsSharedSource(t *testing.T) {
	shared := []Source{SourceProjectSettings, SourcePolicySettings, SourceCommand}
	for _, source := range shared {
		if !IsSharedSource(source) {
			t.Fatalf("IsSharedSource(%q) = false, want true", source)
		}
	}

	personal := []Source{SourceUserSettings, SourceLocalSettings, SourceFlagSettings, SourceCLIArg, SourceSession}
	for _, source := range personal {
		if IsSharedSource(source) {
			t.Fatalf("IsSharedSource(%q) = true, want false", source)
		}
	}
}

func TestDetectUnreachableRulesDenyShadow(t *testing.T) {
	allow := rule(SourceUserSettings, BehaviorAllow, "Bash", ptr("npm test"))
	deny := rule(SourceProjectSettings, BehaviorDeny, "Bash", nil)

	got := DetectUnreachableRules(Context{Rules: []Rule{allow, deny}}, DetectOptions{})

	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].ShadowType != ShadowTypeDeny {
		t.Fatalf("ShadowType = %q, want deny", got[0].ShadowType)
	}
	if got[0].ShadowedBy != deny {
		t.Fatalf("ShadowedBy = %#v, want deny rule", got[0].ShadowedBy)
	}
}

func TestDetectUnreachableRulesAskShadow(t *testing.T) {
	allow := rule(SourceUserSettings, BehaviorAllow, "Bash", ptr("npm test"))
	ask := rule(SourceProjectSettings, BehaviorAsk, "Bash", nil)

	got := DetectUnreachableRules(Context{Rules: []Rule{allow, ask}}, DetectOptions{})

	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].ShadowType != ShadowTypeAsk {
		t.Fatalf("ShadowType = %q, want ask", got[0].ShadowType)
	}
	if got[0].ShadowedBy != ask {
		t.Fatalf("ShadowedBy = %#v, want ask rule", got[0].ShadowedBy)
	}
}

func TestDetectUnreachableRulesDenyTakesPriority(t *testing.T) {
	allow := rule(SourceUserSettings, BehaviorAllow, "Bash", ptr("npm test"))
	ask := rule(SourceProjectSettings, BehaviorAsk, "Bash", nil)
	deny := rule(SourcePolicySettings, BehaviorDeny, "Bash", nil)

	got := DetectUnreachableRules(Context{Rules: []Rule{allow, ask, deny}}, DetectOptions{})

	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].ShadowType != ShadowTypeDeny {
		t.Fatalf("ShadowType = %q, want deny", got[0].ShadowType)
	}
	if got[0].ShadowedBy != deny {
		t.Fatalf("ShadowedBy = %#v, want deny rule", got[0].ShadowedBy)
	}
}

func TestDetectUnreachableRulesSandboxPersonalAskException(t *testing.T) {
	allow := rule(SourceUserSettings, BehaviorAllow, "Bash", ptr("npm test"))
	ask := rule(SourceUserSettings, BehaviorAsk, "Bash", nil)

	got := DetectUnreachableRules(Context{Rules: []Rule{allow, ask}}, DetectOptions{SandboxAutoAllowEnabled: true})

	if len(got) != 0 {
		t.Fatalf("len = %d, want 0", len(got))
	}
}

func TestDetectUnreachableRulesSharedAskStillWarnsWithSandbox(t *testing.T) {
	allow := rule(SourceUserSettings, BehaviorAllow, "Bash", ptr("npm test"))
	ask := rule(SourceProjectSettings, BehaviorAsk, "Bash", nil)

	got := DetectUnreachableRules(Context{Rules: []Rule{allow, ask}}, DetectOptions{SandboxAutoAllowEnabled: true})

	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].ShadowType != ShadowTypeAsk {
		t.Fatalf("ShadowType = %q, want ask", got[0].ShadowType)
	}
}

func TestDetectUnreachableRulesToolWideAllowIsNotShadowed(t *testing.T) {
	allow := rule(SourceUserSettings, BehaviorAllow, "Bash", nil)
	deny := rule(SourceProjectSettings, BehaviorDeny, "Bash", nil)
	ask := rule(SourceProjectSettings, BehaviorAsk, "Bash", nil)

	got := DetectUnreachableRules(Context{Rules: []Rule{allow, deny, ask}}, DetectOptions{})

	if len(got) != 0 {
		t.Fatalf("len = %d, want 0", len(got))
	}
}

func ptr(value string) *string {
	return &value
}

func rule(source Source, behavior Behavior, toolName string, content *string) Rule {
	return Rule{
		Source:   source,
		Behavior: behavior,
		RuleValue: RuleValue{
			ToolName:    toolName,
			RuleContent: content,
		},
	}
}
