package sessionshare

import (
	"errors"
	"strings"
	"testing"
)

func TestPublicBundleBlocksUnredactedSecret(t *testing.T) {
	messages := []Message{{
		Role:    RoleUser,
		Content: "Use api_key=sk-proj-1234567890abcdef for the request",
	}}

	bundle, err := Build(messages, Options{
		Visibility:    VisibilityPublic,
		Action:        ActionShare,
		RedactSecrets: false,
	})

	if !errors.Is(err, ErrUnsafePublicBundle) {
		t.Fatalf("expected ErrUnsafePublicBundle, got %v", err)
	}
	if !bundle.Privacy.Blocked || bundle.Privacy.UnredactedSecrets == 0 {
		t.Fatalf("expected blocked privacy report with unredacted secret, got %+v", bundle.Privacy)
	}
	if bundle.Plan.Allowed {
		t.Fatalf("expected plan to be blocked")
	}
	if strings.Contains(bundle.Messages[0].Content, redactedSecret) {
		t.Fatalf("secret should be detected but not redacted when RedactSecrets=false")
	}
}

func TestPrivateBundleAllowsUnredactedSecretWithWarning(t *testing.T) {
	messages := []Message{{
		Role:    RoleUser,
		Content: "Authorization: Bearer abcdefghijklmnop",
	}}

	bundle, err := Build(messages, Options{
		Visibility:    VisibilityPrivate,
		Action:        ActionExport,
		RedactSecrets: false,
	})

	if err != nil {
		t.Fatalf("expected private bundle to be allowed, got %v", err)
	}
	if bundle.Privacy.Blocked {
		t.Fatalf("did not expect private bundle to be blocked")
	}
	if len(bundle.Privacy.Warnings) == 0 || bundle.Privacy.UnredactedSecrets == 0 {
		t.Fatalf("expected warning for unredacted secret, got %+v", bundle.Privacy)
	}
	if !strings.Contains(bundle.Messages[0].Content, "Bearer abcdefghijklmnop") {
		t.Fatalf("expected private unredacted content to remain, got %q", bundle.Messages[0].Content)
	}
}

func TestToolOutputSummaryAndOmission(t *testing.T) {
	longOutput := "start\n" + strings.Repeat("0123456789\n", 20) + "finish"
	messages := []Message{{
		Role:       RoleTool,
		ToolName:   "grep_search",
		ToolCallID: "call_1",
		Content:    longOutput,
	}}

	summarized, err := Build(messages, Options{
		Visibility:         VisibilityPrivate,
		Action:             ActionExport,
		IncludeToolOutputs: true,
		MaxToolOutputChars: 40,
		SummaryChars:       24,
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	output := summarized.Messages[0].ToolOutput
	if output == nil || output.Mode != ToolOutputSummarized {
		t.Fatalf("expected summarized tool output, got %+v", output)
	}
	if output.Handle == "" || output.ContentHash == "" || output.Summary == "" {
		t.Fatalf("expected handle, hash, and summary, got %+v", output)
	}
	if summarized.Messages[0].Content != "" {
		t.Fatalf("expected large tool output content to be replaced, got %q", summarized.Messages[0].Content)
	}
	if summarized.Manifest.MessageCounts.ToolOutputsSummarized != 1 || summarized.Manifest.OmittedCounts.SummarizedToolOutputs != 1 {
		t.Fatalf("expected summarized counts, got %+v", summarized.Manifest)
	}

	omitted, err := Build(messages, Options{
		Visibility:         VisibilityPrivate,
		Action:             ActionExport,
		IncludeToolOutputs: false,
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	omittedOutput := omitted.Messages[0].ToolOutput
	if omittedOutput == nil || omittedOutput.Mode != ToolOutputOmitted {
		t.Fatalf("expected omitted tool output, got %+v", omittedOutput)
	}
	if omitted.Messages[0].Content != "" || omitted.Manifest.OmittedCounts.ToolOutputs != 1 {
		t.Fatalf("expected tool content omitted and counted, got message=%+v manifest=%+v", omitted.Messages[0], omitted.Manifest)
	}
}

func TestContentHashStableAndIndependentOfAction(t *testing.T) {
	messages := []Message{
		{ID: "m1", Role: RoleUser, Content: "hello"},
		{ID: "m2", Role: RoleAssistant, Content: "hi"},
	}

	first, err := Build(messages, Options{Visibility: VisibilityPrivate, Action: ActionCopy})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	second, err := Build(messages, Options{Visibility: VisibilityPrivate, Action: ActionShare})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	if first.Manifest.ContentHash == "" {
		t.Fatalf("expected content hash")
	}
	if first.Manifest.ContentHash != second.Manifest.ContentHash {
		t.Fatalf("expected stable content hash, got %s and %s", first.Manifest.ContentHash, second.Manifest.ContentHash)
	}
}

func TestCopyExportSharePlans(t *testing.T) {
	tests := []struct {
		action         Action
		destination    string
		requiresUpload bool
	}{
		{ActionCopy, "clipboard", false},
		{ActionExport, "file", false},
		{ActionShare, "remote-share", true},
	}

	for _, tt := range tests {
		bundle, err := Build([]Message{{Role: RoleUser, Content: "hello"}}, Options{
			Visibility: VisibilityOrg,
			Action:     tt.action,
		})
		if err != nil {
			t.Fatalf("Build(%s) returned error: %v", tt.action, err)
		}
		if bundle.Plan.Action != tt.action || bundle.Plan.Destination != tt.destination {
			t.Fatalf("unexpected plan for %s: %+v", tt.action, bundle.Plan)
		}
		if bundle.Plan.RequiresUpload != tt.requiresUpload {
			t.Fatalf("unexpected upload requirement for %s: %+v", tt.action, bundle.Plan)
		}
		if !bundle.Plan.Allowed {
			t.Fatalf("expected plan for %s to be allowed", tt.action)
		}
	}
}

func TestPathRedaction(t *testing.T) {
	messages := []Message{{
		Role:    RoleAssistant,
		Content: "Read /Users/nolangrant/Awaken/src/main.go and /Users/nolangrant/High Agent/tools/session_search_tool.py",
	}}

	bundle, err := Build(messages, Options{
		Visibility:  VisibilityPrivate,
		Action:      ActionExport,
		RedactPaths: true,
		PathRoots:   []string{"/Users/nolangrant/High Agent"},
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	content := bundle.Messages[0].Content
	if strings.Contains(content, "/Users/nolangrant/Awaken") || strings.Contains(content, "/Users/nolangrant/High Agent") {
		t.Fatalf("expected paths to be redacted, got %q", content)
	}
	if !strings.Contains(content, redactedPath) || bundle.Privacy.PathFindings == 0 {
		t.Fatalf("expected redacted path finding, got content=%q privacy=%+v", content, bundle.Privacy)
	}
}

func TestMessageOrderAndCounts(t *testing.T) {
	messages := []Message{
		{ID: "first", Role: RoleUser, Content: "question"},
		{ID: "second", Role: RoleAssistant, Content: "calling tool"},
		{ID: "third", Role: RoleTool, ToolName: "calculator", ToolCallID: "call_1", Content: "4"},
	}

	bundle, err := Build(messages, Options{
		Visibility:         VisibilityPrivate,
		Action:             ActionExport,
		IncludeToolOutputs: true,
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	if len(bundle.Messages) != len(messages) {
		t.Fatalf("expected %d messages, got %d", len(messages), len(bundle.Messages))
	}
	for i, want := range []string{"first", "second", "third"} {
		if bundle.Messages[i].ID != want || bundle.Messages[i].Sequence != i+1 {
			t.Fatalf("message order changed at %d: %+v", i, bundle.Messages[i])
		}
	}
	counts := bundle.Manifest.MessageCounts
	if counts.Total != 3 || counts.ByRole[string(RoleUser)] != 1 || counts.ByRole[string(RoleAssistant)] != 1 || counts.ByRole[string(RoleTool)] != 1 {
		t.Fatalf("unexpected message counts: %+v", counts)
	}
	if counts.ToolOutputs != 1 || counts.ToolOutputsIncluded != 1 {
		t.Fatalf("unexpected tool output counts: %+v", counts)
	}
}
