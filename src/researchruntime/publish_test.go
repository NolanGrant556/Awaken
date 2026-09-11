package researchruntime

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"

	"awaken/src/subagentruntime"
)

func TestParseIntentRequiresQuery(t *testing.T) {
	intent, err := ParseIntent(nil, "plain question")
	if err != nil || intent != nil {
		t.Fatalf("no token should skip research: intent=%+v err=%v", intent, err)
	}
	_, err = ParseIntent([]ComposerToken{{Type: "workflow", Payload: WorkflowPayload{WorkflowID: WorkflowID}}}, "")
	if err == nil {
		t.Fatal("expected empty query to fail")
	}
	intent, err = ParseIntent([]ComposerToken{{Type: "workflow", Payload: WorkflowPayload{WorkflowID: WorkflowID}}}, "Compare Exa and Tavily")
	if err != nil || intent == nil || intent.Query != "Compare Exa and Tavily" {
		t.Fatalf("intent=%+v err=%v", intent, err)
	}
}

func TestConductorPromptRequiresTeamAndSkills(t *testing.T) {
	prompt := ConductorPrompt(Intent{Query: "What is WebSearch?", SourceHint: SourceBoth}, nil)
	for _, want := range []string{
		"调研主编", "subagent", "general", "followup", "wait(any)",
		"至少派两路", "审核", "过时", "ask_user",
		"word", "excel", "ppt", "pdf", "artifact_publish", "已核实",
		"工作区和公开网页都要用",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q: %s", want, prompt)
		}
	}
	web := ConductorPrompt(Intent{Query: "Only web", SourceHint: SourceWeb}, nil)
	if !strings.Contains(web, "优先公开网页") {
		t.Fatalf("web source hint missing: %s", web)
	}
	workspace := ConductorPrompt(Intent{Query: "Only workspace", SourceHint: SourceWorkspace}, nil)
	if !strings.Contains(workspace, "留在当前工作区") {
		t.Fatalf("workspace source hint missing: %s", workspace)
	}
}

func TestValidateVerifiedCitationsRequiresReceipt(t *testing.T) {
	receipts := []subagentruntime.HostToolReceipt{{
		ToolName: "WebFetch",
		Details: map[string]any{
			"final_url":      "https://example.com/docs",
			"content_sha256": "abc",
		},
	}, {
		ToolName: "Read",
		Details: map[string]any{
			"file_path": "/workspace/docs/tools.md",
			"sha256":    "def",
		},
	}}
	if err := ValidateVerifiedCitations("部分核实 https://missing.example/\n已核实 https://example.com/docs\n已核实 docs/tools.md:1", receipts); err != nil {
		t.Fatalf("grounded verified citations should pass: %v", err)
	}
	err := ValidateVerifiedCitations("已核实 https://missing.example/page", receipts)
	if err == nil || !strings.Contains(err.Error(), "https://missing.example/page") {
		t.Fatalf("ungrounded verified URL should fail: %v", err)
	}
	err = ValidateVerifiedCitations("Status: Verified\nSee https://missing.example/page", receipts)
	if err == nil {
		t.Fatal("overall verified status should require every locator")
	}
	if err := ValidateVerifiedCitations("未核实 https://missing.example/page", receipts); err != nil {
		t.Fatalf("unverified locator should publish: %v", err)
	}
}

func TestExplicitHTTPSURLsKeepQuery(t *testing.T) {
	urls := ExplicitHTTPSURLs("see https://httpbin.org/redirect-to?url=http://127.0.0.1/ now")
	if len(urls) != 1 || urls[0] != "https://httpbin.org/redirect-to?url=http://127.0.0.1/" {
		t.Fatalf("urls=%v", urls)
	}
}

func TestExtractPublishTextReadsOfficeXML(t *testing.T) {
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	file, err := writer.Create("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte(`<w:t>已核实 https://example.com/docs</w:t>`)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	text := ExtractPublishText("brief.docx", buf.Bytes())
	if !strings.Contains(text, "https://example.com/docs") {
		t.Fatalf("office text missing locator: %q", text)
	}
	err = ValidateVerifiedCitations(text, nil)
	if err == nil {
		t.Fatal("office verified citation without receipt must fail")
	}
}
