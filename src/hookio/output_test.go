package hookio

import (
	"strings"
	"testing"
)

func TestParseOutputValidCases(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		assert func(t *testing.T, output Output)
	}{
		{
			name:  "async output",
			input: `{"async":true,"asyncTimeout":12.5}`,
			assert: func(t *testing.T, output Output) {
				if output.Kind != OutputKindAsync || output.Async == nil || !output.Async.Async {
					t.Fatalf("expected async output, got %+v", output)
				}
				if output.Async.AsyncTimeout == nil || *output.Async.AsyncTimeout != 12.5 {
					t.Fatalf("unexpected async timeout: %+v", output.Async.AsyncTimeout)
				}
			},
		},
		{
			name:  "sync continue false with stop reason",
			input: `{"continue":false,"suppressOutput":true,"stopReason":"blocked by policy"}`,
			assert: func(t *testing.T, output Output) {
				if output.Kind != OutputKindSync || output.Sync == nil {
					t.Fatalf("expected sync output, got %+v", output)
				}
				if output.Sync.Continue == nil || *output.Sync.Continue {
					t.Fatalf("expected continue=false, got %+v", output.Sync.Continue)
				}
				if output.Sync.StopReason != "blocked by policy" {
					t.Fatalf("unexpected stop reason %q", output.Sync.StopReason)
				}
				if output.Sync.SuppressOutput == nil || !*output.Sync.SuppressOutput {
					t.Fatalf("expected suppressOutput=true, got %+v", output.Sync.SuppressOutput)
				}
			},
		},
		{
			name:  "pre tool use updated input",
			input: `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"allow","reason":"sanitized","updatedInput":{"command":"ls -la"},"additionalContext":"safe command"}}`,
			assert: func(t *testing.T, output Output) {
				hook := requireHookOutput(t, output)
				if hook["hookEventName"] != string(HookEventPreToolUse) {
					t.Fatalf("unexpected hook event: %+v", hook["hookEventName"])
				}
				updatedInput := requireMap(t, hook["updatedInput"])
				if updatedInput["command"] != "ls -la" {
					t.Fatalf("unexpected updated input: %+v", updatedInput)
				}
			},
		},
		{
			name:  "permission request allow",
			input: `{"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":{"behavior":"allow","updatedInput":{"path":"README.md"},"updatedPermissions":[{"type":"addDirectories","directories":["/tmp"],"destination":"session"}]}}}`,
			assert: func(t *testing.T, output Output) {
				decision := requireMap(t, requireHookOutput(t, output)["decision"])
				if decision["behavior"] != "allow" {
					t.Fatalf("unexpected permission behavior: %+v", decision)
				}
				updatedInput := requireMap(t, decision["updatedInput"])
				if updatedInput["path"] != "README.md" {
					t.Fatalf("unexpected updated input: %+v", updatedInput)
				}
			},
		},
		{
			name:  "permission request deny",
			input: `{"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":{"behavior":"deny","message":"nope","interrupt":true}}}`,
			assert: func(t *testing.T, output Output) {
				decision := requireMap(t, requireHookOutput(t, output)["decision"])
				if decision["behavior"] != "deny" || decision["message"] != "nope" || decision["interrupt"] != true {
					t.Fatalf("unexpected deny decision: %+v", decision)
				}
			},
		},
		{
			name:  "elicitation action",
			input: `{"hookSpecificOutput":{"hookEventName":"Elicitation","action":"accept","content":{"value":"yes"}}}`,
			assert: func(t *testing.T, output Output) {
				hook := requireHookOutput(t, output)
				if hook["action"] != "accept" {
					t.Fatalf("unexpected elicitation action: %+v", hook)
				}
				content := requireMap(t, hook["content"])
				if content["value"] != "yes" {
					t.Fatalf("unexpected elicitation content: %+v", content)
				}
			},
		},
		{
			name:  "post tool use updated output",
			input: `{"hookSpecificOutput":{"hookEventName":"PostToolUse","updatedMCPToolOutput":{"text":"patched"},"additionalContext":"tool context"}}`,
			assert: func(t *testing.T, output Output) {
				updatedOutput := requireMap(t, requireHookOutput(t, output)["updatedMCPToolOutput"])
				if updatedOutput["text"] != "patched" {
					t.Fatalf("unexpected updated MCP output: %+v", updatedOutput)
				}
			},
		},
		{
			name:  "session start watch paths",
			input: `{"hookSpecificOutput":{"hookEventName":"SessionStart","watchPaths":["/tmp/project"],"additionalContext":"session context"}}`,
			assert: func(t *testing.T, output Output) {
				watchPaths, ok := requireHookOutput(t, output)["watchPaths"].([]any)
				if !ok || len(watchPaths) != 1 || watchPaths[0] != "/tmp/project" {
					t.Fatalf("unexpected watch paths: %+v", watchPaths)
				}
			},
		},
		{
			name:  "worktree create path",
			input: `{"hookSpecificOutput":{"hookEventName":"WorktreeCreate","worktreePath":"/tmp/worktree"}}`,
			assert: func(t *testing.T, output Output) {
				if requireHookOutput(t, output)["worktreePath"] != "/tmp/worktree" {
					t.Fatalf("unexpected worktree path: %+v", output)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := ParseOutput([]byte(tt.input))
			if err != nil {
				t.Fatalf("ParseOutput returned error: %v", err)
			}
			tt.assert(t, output)
		})
	}
}

func TestParseOutputRejectsInvalidCases(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{
			name:    "illegal top level decision",
			input:   `{"decision":"allow"}`,
			wantErr: `decision must be one of approve, block`,
		},
		{
			name:    "illegal async false",
			input:   `{"async":false}`,
			wantErr: `async must be true`,
		},
		{
			name:    "continue false missing stop reason",
			input:   `{"continue":false}`,
			wantErr: `stopReason is required`,
		},
		{
			name:    "hook output missing event name",
			input:   `{"hookSpecificOutput":{"additionalContext":"ctx"}}`,
			wantErr: `hookSpecificOutput.hookEventName is required`,
		},
		{
			name:    "unknown hook event",
			input:   `{"hookSpecificOutput":{"hookEventName":"UnknownHook"}}`,
			wantErr: `unknown hookEventName "UnknownHook"`,
		},
		{
			name:    "permission request missing decision",
			input:   `{"hookSpecificOutput":{"hookEventName":"PermissionRequest"}}`,
			wantErr: `hookSpecificOutput.decision is required`,
		},
		{
			name:    "permission request illegal behavior",
			input:   `{"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":{"behavior":"maybe"}}}`,
			wantErr: `hookSpecificOutput.decision.behavior must be one of allow, deny`,
		},
		{
			name:    "elicitation illegal action",
			input:   `{"hookSpecificOutput":{"hookEventName":"Elicitation","action":"maybe"}}`,
			wantErr: `hookSpecificOutput.action must be one of accept, decline, cancel`,
		},
		{
			name:    "worktree create missing path",
			input:   `{"hookSpecificOutput":{"hookEventName":"WorktreeCreate"}}`,
			wantErr: `hookSpecificOutput.worktreePath is required`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseOutput([]byte(tt.input))
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}

func TestValidateOutputCopiesInputMaps(t *testing.T) {
	input := map[string]any{
		"hookSpecificOutput": map[string]any{
			"hookEventName":            "PreToolUse",
			"permissionDecision":       "allow",
			"permissionDecisionReason": "safe",
			"updatedInput": map[string]any{
				"command": "ls",
				"nested": map[string]any{
					"flag": "old",
				},
			},
		},
	}

	output, err := ValidateOutput(input)
	if err != nil {
		t.Fatalf("ValidateOutput returned error: %v", err)
	}

	inputHook := input["hookSpecificOutput"].(map[string]any)
	inputUpdated := inputHook["updatedInput"].(map[string]any)
	inputUpdated["command"] = "rm -rf /"
	inputUpdated["nested"].(map[string]any)["flag"] = "new"

	outputHook := requireHookOutput(t, output)
	outputUpdated := requireMap(t, outputHook["updatedInput"])
	if outputUpdated["command"] != "ls" {
		t.Fatalf("expected output map to be isolated from input mutation, got %+v", outputUpdated)
	}
	outputNested := requireMap(t, outputUpdated["nested"])
	if outputNested["flag"] != "old" {
		t.Fatalf("expected nested output map to be isolated from input mutation, got %+v", outputNested)
	}
}

func requireHookOutput(t *testing.T, output Output) map[string]any {
	t.Helper()
	if output.Sync == nil || output.Sync.HookSpecificOutput == nil {
		t.Fatalf("expected sync hookSpecificOutput, got %+v", output)
	}
	return output.Sync.HookSpecificOutput
}

func requireMap(t *testing.T, value any) map[string]any {
	t.Helper()
	typed, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T (%+v)", value, value)
	}
	return typed
}
