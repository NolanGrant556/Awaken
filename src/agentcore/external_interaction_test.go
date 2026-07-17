package agentcore

import (
	"strings"
	"testing"

	"awaken/src/tool"
)

func TestDetectCLIAuthInteractionDWSExit4(t *testing.T) {
	payload := `{"stdout":"{\"data\":{\"authorizationUrl\":\"https://login.dingtalk.com/oauth2/auth?x=1\"}}\n","stderr":"","exit_code":4}`
	exit := 4
	view, ok := DetectCLIAuthInteraction("Bash", tool.ExecutionOutcome{
		Content:  payload,
		Status:   tool.OutcomeFailed,
		ExitCode: &exit,
	})
	if !ok {
		t.Fatal("expected DWS auth interaction")
	}
	if view.Provider != "dingtalk-workspace" {
		t.Fatalf("provider=%q", view.Provider)
	}
	if view.VerificationURL != "https://login.dingtalk.com/oauth2/auth?x=1" {
		t.Fatalf("url=%q", view.VerificationURL)
	}
	if !view.RequiresReplay {
		t.Fatal("expected replay")
	}
}

func TestDetectCLIAuthInteractionLarkVerificationURL(t *testing.T) {
	payload := `{"stdout":"{\"verification_url\":\"https://open.feishu.cn/auth?code=abc\",\"device_code\":\"SECRET\"}\n","stderr":"","exit_code":0}`
	view, ok := DetectCLIAuthInteraction("bash", tool.ExecutionOutcome{Content: payload, Status: tool.OutcomeSucceeded})
	if !ok {
		t.Fatal("expected Lark auth interaction")
	}
	if view.Provider != "lark-cli" {
		t.Fatalf("provider=%q", view.Provider)
	}
	if view.VerificationURL != "https://open.feishu.cn/auth?code=abc" {
		t.Fatalf("url=%q", view.VerificationURL)
	}
	if strings.Contains(view.SafeMessage, "SECRET") {
		t.Fatal("device code must not leak into safe view")
	}
}

func TestDetectCLIAuthInteractionRejectsHTTP(t *testing.T) {
	payload := `{"stdout":"{\"data\":{\"authorizationUrl\":\"http://evil.example/auth\"}}\n","stderr":"","exit_code":4}`
	exit := 4
	if _, ok := DetectCLIAuthInteraction("bash", tool.ExecutionOutcome{Content: payload, ExitCode: &exit}); ok {
		t.Fatal("http URLs must be rejected")
	}
}
