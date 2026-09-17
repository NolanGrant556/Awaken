package agentcore

import (
	"encoding/json"
	"strings"
	"testing"

	convcontext "awaken/src/context"
)

func TestRequestProjectionUnloadsOlderRetrievedPagesAndKeepsNearTail(t *testing.T) {
	page0 := strings.Repeat("PAGE-ZERO-", 80)
	page1 := strings.Repeat("PAGE-ONE--", 80)
	page2 := strings.Repeat("PAGE-TWO--", 80)
	messages := []convcontext.Message{
		{Role: convcontext.RoleUser, Content: "full inbound"},
		pagedReadAssistant("read-0", `{"storage_key":"thread/tool-results/root/in/content.txt","offset":0}`),
		pagedReadResult("read-0", "tool_result_read", pagePayload(page0, 0)),
		pagedReadAssistant("read-1", `{"storage_key":"thread/tool-results/root/in/content.txt","offset":2048}`),
		pagedReadResult("read-1", "tool_result_read", pagePayload(page1, 2048)),
		pagedReadAssistant("read-2", `{"storage_key":"thread/tool-results/root/in/content.txt","offset":4096}`),
		pagedReadResult("read-2", "tool_result_read", pagePayload(page2, 4096)),
	}
	original := cloneContextMessages(messages)

	projected := applyWindowRequestProjection(messages, requestProjectionOptions{})
	if projected[2].Content == messages[2].Content || !strings.Contains(projected[2].Content, priorToolResultUnloadedPrefix) {
		t.Fatalf("older page 0 should unload, got %q", projected[2].Content)
	}
	if !strings.Contains(projected[2].Content, `"prior_result_unloaded":true`) {
		t.Fatalf("unloaded JSON should keep a prior_result_unloaded receipt, got %q", projected[2].Content)
	}
	if strings.Contains(projected[2].Content, "PAGE-ZERO-") {
		t.Fatal("unloaded page 0 leaked original body into the request")
	}
	if !strings.Contains(projected[4].Content, priorToolResultUnloadedPrefix) || strings.Contains(projected[4].Content, "PAGE-ONE--") {
		t.Fatalf("older page 1 should unload, got %q", projected[4].Content)
	}
	if strings.Contains(projected[6].Content, priorToolResultUnloadedPrefix) || !strings.Contains(projected[6].Content, "PAGE-TWO--") {
		t.Fatalf("near-tail page should stay, got %q", projected[6].Content)
	}
	if original[2].Content != messages[2].Content || !strings.Contains(original[2].Content, "PAGE-ZERO-") {
		t.Fatal("durable conversation must keep the full retrieved page")
	}
	if err := convcontext.ValidateToolResultPairing(projected); err != nil {
		t.Fatalf("unloading pages must not break pairing: %v", err)
	}
}

func TestNewUserTurnUnloadsPreviousRetrievedPages(t *testing.T) {
	oldPage := strings.Repeat("INBOUND-JSON-", 100)
	messages := []convcontext.Message{
		{Role: convcontext.RoleUser, Content: "full inbound"},
		pagedReadAssistant("old-read", `{"storage_key":"thread/tool-results/root/in/content.txt","offset":0}`),
		pagedReadResult("old-read", "tool_result_read", oldPage),
		{Role: convcontext.RoleUser, Content: "full outbound now"},
	}
	projected := applyWindowRequestProjection(messages, requestProjectionOptions{})
	if !strings.Contains(projected[2].Content, priorToolResultUnloadedPrefix) {
		t.Fatalf("previous-turn page should unload when a new user turn starts, got %q", projected[2].Content)
	}
	if strings.Contains(projected[2].Content, "INBOUND-JSON-") {
		t.Fatal("previous-turn page body still in the live request")
	}
	if !strings.Contains(projected[2].Content, "arguments=") || !strings.Contains(projected[2].Content, "storage_key") {
		t.Fatalf("unload stub should keep the original read arguments, got %q", projected[2].Content)
	}
}

func TestPagedProjectionLeavesOffloadStubsAndErrorsAlone(t *testing.T) {
	offload := "[TOOL_RESULT_OFFLOADED]\nstorage_key=thread/tool-results/root/in/content.txt\nsha256=abc\nbytes=52000"
	messages := []convcontext.Message{
		{Role: convcontext.RoleUser, Content: "run"},
		{Role: convcontext.RoleAssistant, ToolCalls: []convcontext.ToolCall{{
			ID: "src", Function: convcontext.FunctionCall{Name: "wait", Arguments: `{}`},
		}}},
		{Role: convcontext.RoleTool, ToolCallID: "src", Name: "wait", Content: offload},
		pagedReadAssistant("bad", `{"storage_key":"thread/tool-results/root/in/content.txt","offset":0}`),
		{
			Role: convcontext.RoleTool, ToolCallID: "bad", Name: "tool_result_read", IsError: true,
			Content: "tool_result_not_owned: denied",
		},
		{Role: convcontext.RoleUser, Content: "continue"},
	}
	projected := applyWindowRequestProjection(messages, requestProjectionOptions{})
	if projected[2].Content != offload {
		t.Fatalf("original offload stub should stay, got %q", projected[2].Content)
	}
	if projected[4].Content != "tool_result_not_owned: denied" {
		t.Fatalf("error pages should stay, got %q", projected[4].Content)
	}
}

func TestBuildLLMRequestUnloadsOlderToolResultReadPages(t *testing.T) {
	oldPage := strings.Repeat("RECEIPT-LINE-", 120)
	latest := strings.Repeat("NOTICE-LINE-", 40)
	conv := convcontext.NewConversation(nil)
	conv.Context.SetMessages([]convcontext.Message{
		{Role: convcontext.RoleUser, Content: "inbound first"},
		pagedReadAssistant("old", `{"storage_key":"thread/tool-results/task/in/content.txt","offset":0,"limit":2048}`),
		pagedReadResult("old", "tool_result_read", pagePayload(oldPage, 0)),
		{Role: convcontext.RoleUser, Content: "outbound next"},
		pagedReadAssistant("new", `{"storage_key":"thread/tool-results/task/notice/content.txt","offset":0,"limit":2048}`),
		pagedReadResult("new", "tool_result_read", pagePayload(latest, 0)),
	})
	runner := NewRunner(AgentConfig{}, conv, NewRequestBuilder(AgentConfig{}).BindThread("thread"))
	runner.threadID = "thread"
	req, err := runner.buildLLMRequest()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(req.Messages[2].Content, priorToolResultUnloadedPrefix) || strings.Contains(req.Messages[2].Content, "RECEIPT-LINE-") {
		t.Fatalf("previous-turn retrieved page should leave the live request, got %q", req.Messages[2].Content)
	}
	if strings.Contains(req.Messages[5].Content, priorToolResultUnloadedPrefix) || !strings.Contains(req.Messages[5].Content, "NOTICE-LINE-") {
		t.Fatalf("current-turn page should stay, got %q", req.Messages[5].Content)
	}
	durable := conv.Context.GetMessages()
	if !strings.Contains(durable[2].Content, "RECEIPT-LINE-") {
		t.Fatal("journal/conversation still needs the full previous page")
	}
}

func TestThreadHistoryReadPagesUnloadLikeToolResultRead(t *testing.T) {
	page := strings.Repeat("JOURNAL-ITEM-", 90)
	messages := []convcontext.Message{
		{Role: convcontext.RoleUser, Content: "what happened earlier"},
		pagedReadAssistant("hist-0", `{"query":"receipt","limit":20}`),
		pagedReadResult("hist-0", "thread_history_read", page),
		{Role: convcontext.RoleUser, Content: "continue outbound"},
	}
	projected := applyWindowRequestProjection(messages, requestProjectionOptions{})
	if !strings.Contains(projected[2].Content, priorToolResultUnloadedPrefix) || strings.Contains(projected[2].Content, "JOURNAL-ITEM-") {
		t.Fatalf("older thread_history_read page should unload, got %q", projected[2].Content)
	}
}

func TestNewUserTurnUnloadsPreviousInlineMCPResults(t *testing.T) {
	old := strings.Repeat("PO-260917007-PREPARE-JSON-", 80)
	latest := strings.Repeat("PO-260917012-EXECUTE-JSON-", 40)
	messages := []convcontext.Message{
		{Role: convcontext.RoleUser, Content: "出采购单"},
		{
			Role: convcontext.RoleAssistant,
			ToolCalls: []convcontext.ToolCall{{
				ID: "prep", Function: convcontext.FunctionCall{Name: "tianhui_material_receipt", Arguments: `{"action":"prepare","bill_num":"PO-260917007"}`},
			}},
		},
		{Role: convcontext.RoleTool, ToolCallID: "prep", Name: "tianhui_material_receipt", Content: old},
		{Role: convcontext.RoleUser, Content: "全量入仓"},
		{
			Role: convcontext.RoleAssistant,
			ToolCalls: []convcontext.ToolCall{{
				ID: "exec", Function: convcontext.FunctionCall{Name: "tianhui_material_receipt", Arguments: `{"action":"execute","plan_id":"abc"}`},
			}},
		},
		{Role: convcontext.RoleTool, ToolCallID: "exec", Name: "tianhui_material_receipt", Content: latest},
	}
	projected := applyWindowRequestProjection(messages, requestProjectionOptions{})
	if !strings.Contains(projected[2].Content, priorToolResultUnloadedPrefix) || strings.Contains(projected[2].Content, "PO-260917007-PREPARE-JSON-") {
		t.Fatalf("previous-turn MCP body should unload, got %q", projected[2].Content)
	}
	if !strings.Contains(projected[2].Content, "tool_name=tianhui_material_receipt") || !strings.Contains(projected[2].Content, "PO-260917007") {
		t.Fatalf("unload stub should name the tool and keep bounded arguments, got %q", projected[2].Content)
	}
	if strings.Contains(projected[5].Content, priorToolResultUnloadedPrefix) || !strings.Contains(projected[5].Content, "PO-260917012-EXECUTE-JSON-") {
		t.Fatalf("current-turn MCP body should stay, got %q", projected[5].Content)
	}
	if utf8Count := len([]rune(old)); utf8Count <= priorToolResultKeepRunes {
		t.Fatal("fixture too small to exercise unload")
	}
}

func TestSameUserTurnUnloadsEarlierInlineMCPPairings(t *testing.T) {
	first := strings.Repeat("PO-260917007-PREPARE-JSON-", 80)
	second := strings.Repeat("PO-260917007-EXECUTE-JSON-", 80)
	latest := strings.Repeat("PO-260917008-PREPARE-JSON-", 40)
	messages := []convcontext.Message{
		{Role: convcontext.RoleUser, Content: "全量入仓"},
		{
			Role: convcontext.RoleAssistant,
			ToolCalls: []convcontext.ToolCall{{
				ID: "prep-7", Function: convcontext.FunctionCall{Name: "tianhui_material_receipt", Arguments: `{"action":"prepare","bill_num":"PO-260917007"}`},
			}},
		},
		{Role: convcontext.RoleTool, ToolCallID: "prep-7", Name: "tianhui_material_receipt", Content: first},
		{
			Role: convcontext.RoleAssistant,
			ToolCalls: []convcontext.ToolCall{{
				ID: "exec-7", Function: convcontext.FunctionCall{Name: "tianhui_material_receipt", Arguments: `{"action":"execute","plan_id":"plan-7"}`},
			}},
		},
		{Role: convcontext.RoleTool, ToolCallID: "exec-7", Name: "tianhui_material_receipt", Content: second},
		{
			Role: convcontext.RoleAssistant,
			ToolCalls: []convcontext.ToolCall{{
				ID: "prep-8", Function: convcontext.FunctionCall{Name: "tianhui_material_receipt", Arguments: `{"action":"prepare","bill_num":"PO-260917008"}`},
			}},
		},
		{Role: convcontext.RoleTool, ToolCallID: "prep-8", Name: "tianhui_material_receipt", Content: latest},
	}
	projected := applyWindowRequestProjection(messages, requestProjectionOptions{})
	if !strings.Contains(projected[2].Content, priorToolResultUnloadedPrefix) || strings.Contains(projected[2].Content, "PO-260917007-PREPARE-JSON-") {
		t.Fatalf("earlier prepare in the same user turn should unload, got %q", projected[2].Content)
	}
	if !strings.Contains(projected[4].Content, priorToolResultUnloadedPrefix) || strings.Contains(projected[4].Content, "PO-260917007-EXECUTE-JSON-") {
		t.Fatalf("earlier execute in the same user turn should unload, got %q", projected[4].Content)
	}
	if strings.Contains(projected[6].Content, priorToolResultUnloadedPrefix) || !strings.Contains(projected[6].Content, "PO-260917008-PREPARE-JSON-") {
		t.Fatalf("latest pairing should stay, got %q", projected[6].Content)
	}
}

func TestProjectionKeepsShortPriorToolResults(t *testing.T) {
	messages := []convcontext.Message{
		{Role: convcontext.RoleUser, Content: "run"},
		{Role: convcontext.RoleAssistant, ToolCalls: []convcontext.ToolCall{{
			ID: "ok", Function: convcontext.FunctionCall{Name: "Bash", Arguments: `{"command":"true"}`},
		}}},
		{Role: convcontext.RoleTool, ToolCallID: "ok", Name: "Bash", Content: "ok"},
		{Role: convcontext.RoleUser, Content: "next"},
	}
	projected := applyWindowRequestProjection(messages, requestProjectionOptions{})
	if projected[2].Content != "ok" {
		t.Fatalf("short prior results should stay, got %q", projected[2].Content)
	}
}

func pagedReadAssistant(id, arguments string) convcontext.Message {
	name := "tool_result_read"
	if strings.Contains(arguments, `"query"`) {
		name = "thread_history_read"
	}
	return convcontext.Message{
		Role: convcontext.RoleAssistant,
		ToolCalls: []convcontext.ToolCall{{
			ID: id, Type: "function",
			Function: convcontext.FunctionCall{Name: name, Arguments: arguments},
		}},
	}
}

func pagedReadResult(id, name, content string) convcontext.Message {
	return convcontext.Message{
		Role:       convcontext.RoleTool,
		ToolCallID: id,
		Name:       name,
		Content:    content,
	}
}

func pagePayload(page string, offset int) string {
	payload, err := json.Marshal(map[string]any{
		"schema_version": 2,
		"status":         "succeeded",
		"content":        page,
		"truncated":      false,
		"details":        map[string]any{"offset": offset, "next_offset": offset + 2048, "has_more": true},
	})
	if err != nil {
		panic(err)
	}
	return string(payload)
}

func TestRewritePagedRetrievalContentKeepsDetails(t *testing.T) {
	raw := pagePayload("BODY", 2048)
	got := rewritePagedRetrievalContent(raw, priorToolResultUnloadedPrefix+"\nstub")
	var payload map[string]any
	if err := json.Unmarshal([]byte(got), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["truncated"] != true {
		t.Fatalf("truncated=%v", payload["truncated"])
	}
	details, _ := payload["details"].(map[string]any)
	if details["prior_result_unloaded"] != true || details["offset"].(float64) != 2048 {
		t.Fatalf("details=%v", details)
	}
}
