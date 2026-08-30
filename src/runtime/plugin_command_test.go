package runtime

import (
	"strings"
	"testing"
)

func TestResolvePluginCommandTokenUsesHostExpansion(t *testing.T) {
	tokens := []ComposerToken{{
		ID: "command", Type: "command", LeadingText: "Please ", Label: "review",
		Payload: ComposerTokenPayload{PluginName: "acme", PluginDigest: "digest", CommandLogicalID: "command:review", CommandArgumentsJSON: `["app.ts","security"]`},
	}}
	resolved, err := resolvePluginCommandTokens(tokens, func(name, digest, logicalID string, arguments []string) (string, error) {
		if name != "acme" || digest != "digest" || logicalID != "command:review" || len(arguments) != 2 || arguments[1] != "security" {
			t.Fatalf("unexpected identity: %s %s %s %#v", name, digest, logicalID, arguments)
		}
		return "Review app.ts for security", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	effective := effectiveInputForComposerTokens("Please now", resolved)
	if !strings.Contains(effective, "Please Review app.ts for security") || !strings.HasSuffix(effective, "now") {
		t.Fatalf("effective=%q", effective)
	}
	if tokens[0].Payload.CommandPrompt != "" {
		t.Fatal("source token was mutated")
	}
}

func TestResolvePluginCommandTokenRejectsRendererPromptAndInvalidArguments(t *testing.T) {
	token := ComposerToken{ID: "command", Type: "command", Payload: ComposerTokenPayload{
		PluginName: "acme", PluginDigest: "digest", CommandLogicalID: "command:review",
		CommandArgumentsJSON: "not-json", CommandPrompt: "renderer-controlled",
	}}
	if _, err := resolvePluginCommandTokens([]ComposerToken{token}, func(string, string, string, []string) (string, error) {
		return "host", nil
	}); err == nil {
		t.Fatal("invalid encoded arguments were accepted")
	}
}

func TestResolvePluginAgentTokenIsDigestBoundAndHostResolved(t *testing.T) {
	token := ComposerToken{ID: "agent", Type: "agent", Payload: ComposerTokenPayload{PluginName: "acme", PluginDigest: "digest", AgentLogicalID: "agent:reviewer", AgentType: "renderer-controlled"}}
	resolved, err := resolvePluginAgentTokens([]ComposerToken{token}, func(name, digest, logicalID string) (PluginAgentSelection, error) {
		if name != "acme" || digest != "digest" || logicalID != "agent:reviewer" {
			t.Fatalf("identity=%s %s %s", name, digest, logicalID)
		}
		return PluginAgentSelection{AgentType: "plugin__acme__reviewer"}, nil
	})
	if err != nil || resolved[0].Payload.AgentType != "plugin__acme__reviewer" || token.Payload.AgentType != "renderer-controlled" {
		t.Fatalf("resolved=%#v err=%v", resolved, err)
	}
	effective := effectiveInputForComposerTokens("Review this change", resolved)
	if !strings.Contains(effective, `agent_type "plugin__acme__reviewer"`) ||
		!strings.Contains(effective, `workspace_mode "shared"`) ||
		!strings.Contains(effective, "wait for that child to reach a terminal state") {
		t.Fatalf("Plugin Agent instruction does not require terminal handoff: %q", effective)
	}
}

func TestResolvePluginAgentTokenPinsReadOnlyWorkspaceMode(t *testing.T) {
	resolved, err := resolvePluginAgentTokens([]ComposerToken{{
		ID: "agent", Type: "agent",
		Payload: ComposerTokenPayload{PluginName: "acme", PluginDigest: "digest", AgentLogicalID: "agent:reader"},
	}}, func(string, string, string) (PluginAgentSelection, error) {
		return PluginAgentSelection{AgentType: "plugin__acme__reader", ReadOnly: true}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	effective := effectiveInputForComposerTokens("Inspect this", resolved)
	if !strings.Contains(effective, `workspace_mode "shared_readonly"`) {
		t.Fatalf("read-only Plugin Agent workspace mode is not pinned: %q", effective)
	}
}
