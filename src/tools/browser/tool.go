package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"awaken/src/browserhost"
	"awaken/src/tool"
)

const (
	ToolNavigate = "browser_navigate"
	ToolSnapshot = "browser_snapshot"
	ToolClick    = "browser_click"
	ToolType     = "browser_type"
	ToolWait     = "browser_wait"
	ToolExtract  = "browser_extract"
)

type browserTool struct {
	name        string
	action      string
	displayName string
	description string
	purpose     string
	readOnly    bool
	interact    bool
	parameters  []tool.ToolParameter
	inputSchema tool.ToolParametersSchema
	outputDesc  string
	whenToUse   []string
	whenNotToUse []string
	examples    []tool.ToolExample
	failures    []tool.ToolFailureMode
}

func (t *browserTool) Name() string { return t.name }
func (t *browserTool) Description() string {
	return t.description
}
func (t *browserTool) Parameters() []tool.ToolParameter { return t.parameters }
func (t *browserTool) Definition() tool.ToolDefinition {
	capabilityClass := tool.CapabilityBrowserRead
	if t.interact {
		capabilityClass = tool.CapabilityBrowserInteract
	}
	return tool.BuildBuiltinDefinition(t, tool.ToolDefinition{
		Name:         t.name,
		DisplayName:  t.displayName,
		Description:  t.description,
		Purpose:      t.purpose,
		InputSchema:  t.inputSchema,
		WhenToUse:    t.whenToUse,
		WhenNotToUse: t.whenNotToUse,
		SideEffects: []tool.ToolSideEffect{{
			Kind:          tool.SideEffectNetwork,
			Resources:     []string{"Desktop embedded browser page"},
			Description:   "Talks to the visible Awaken browser tab through the Desktop host.",
			Reversibility: tool.ReversibilityNotApplicable,
		}},
		Examples:     t.examples,
		FailureModes: t.failures,
		OutputSchema: tool.ToolParam{Type: "object", Description: t.outputDesc, AdditionalProperties: tool.BoolPtr(true)},
		Annotations: tool.ToolAnnotations{
			ReadOnly:        t.readOnly,
			OpenWorld:       true,
			CapabilityClass: capabilityClass,
			TrustedOrigin:   tool.TrustedOriginBuiltin,
			NetworkScope:    tool.NetworkScopeOpen,
		},
	})
}

func (t *browserTool) IncludeInRunSnapshot(tool.ExecutionEnvironmentSnapshot) bool {
	return browserhost.Ready()
}

func (t *browserTool) Execute(ctx context.Context, input string) (string, error) {
	outcome, err := t.ExecuteOutcome(ctx, input, nil)
	if err != nil {
		return outcome.Content, err
	}
	if outcome.IsError() {
		return outcome.Content, fmt.Errorf("%s", outcome.ErrorMessage)
	}
	return outcome.Content, nil
}

func (t *browserTool) ExecuteOutcome(ctx context.Context, input string, _ tool.ToolObserver) (tool.ExecutionOutcome, error) {
	started := time.Now().UTC()
	if !browserhost.Ready() {
		outcome := tool.FailedOutcome("Embedded browser host is not available.", "browser_host_unavailable", tool.ErrorCategoryUnsupported, true)
		outcome.RecoveryOptions = []string{"open_awaken_desktop", "retry_after_desktop_ready"}
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}
	threadID := strings.TrimSpace(tool.ToolThreadIDFromContext(ctx))
	if threadID == "" {
		outcome := tool.FailedOutcome("browser tools require an active thread.", "thread_id_required", tool.ErrorCategoryValidation, false)
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}
	request, err := parseAction(t.action, threadID, input)
	if err != nil {
		outcome := tool.FailedOutcome(err.Error(), "browser_invalid_input", tool.ErrorCategoryValidation, false)
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}
	result, err := browserhost.Call(ctx, request)
	if err != nil {
		outcome := tool.FailedOutcome(err.Error(), "browser_host_unreachable", tool.ErrorCategoryRuntime, true)
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}
	if !result.OK {
		code := strings.TrimSpace(result.Error)
		if code == "" {
			code = "browser_action_failed"
		}
		category := tool.ErrorCategoryDomain
		retryable := false
		switch code {
		case "browser_view_not_visible", "browser_not_shown", "browser_host_unavailable":
			category = tool.ErrorCategoryUnsupported
			retryable = true
		case "origin_lease_expired":
			category = tool.ErrorCategoryPolicy
		case "unsupported_scheme", "element_ref_required", "thread_id_required":
			category = tool.ErrorCategoryValidation
		}
		outcome := tool.FailedOutcome("Browser action failed: "+code, code, category, retryable)
		outcome.Diagnostic = map[string]any{"url": result.URL, "title": result.Title}
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}
	content := formatResult(result)
	outcome := tool.SucceededOutcome(content)
	outcome.Details = map[string]any{
		"url":       result.URL,
		"title":     result.Title,
		"snapshot":  result.Snapshot,
		"text":      result.Text,
		"truncated": result.Truncated,
	}
	return outcome.WithTiming(started, time.Now().UTC()), nil
}

type actionInput struct {
	URL     string  `json:"url"`
	Ref     string  `json:"ref"`
	Text    string  `json:"text"`
	Seconds float64 `json:"seconds"`
	Start   int     `json:"start"`
}

func parseAction(action, threadID, input string) (browserhost.ActionRequest, error) {
	request := browserhost.ActionRequest{Action: action, ThreadID: threadID}
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		if action == "navigate" {
			return request, fmt.Errorf("url is required")
		}
		return request, nil
	}
	var params actionInput
	if err := json.Unmarshal([]byte(trimmed), &params); err != nil {
		if action == "navigate" {
			request.URL = trimmed
			return request, nil
		}
		return request, fmt.Errorf("invalid browser tool input")
	}
	request.URL = params.URL
	request.Ref = params.Ref
	request.Text = params.Text
	request.Seconds = params.Seconds
	request.Start = params.Start
	if action == "navigate" && strings.TrimSpace(request.URL) == "" {
		return request, fmt.Errorf("url is required")
	}
	if (action == "click" || action == "type") && strings.TrimSpace(request.Ref) == "" {
		return request, fmt.Errorf("ref is required")
	}
	if action == "type" && request.Text == "" {
		return request, fmt.Errorf("text is required")
	}
	return request, nil
}

func formatResult(result browserhost.Result) string {
	var b strings.Builder
	if result.Title != "" {
		b.WriteString(result.Title)
		b.WriteString("\n")
	}
	if result.URL != "" {
		b.WriteString(result.URL)
		b.WriteString("\n")
	}
	if result.Snapshot != "" {
		b.WriteString(result.Snapshot)
	}
	if result.Text != "" {
		b.WriteString(result.Text)
	}
	if result.Truncated {
		b.WriteString("\n[truncated]")
	}
	text := strings.TrimSpace(b.String())
	if text == "" {
		return "ok"
	}
	return text
}

func init() {
	tool.MustRegisterToolWithMode(&browserTool{
		name: ToolNavigate, action: "navigate", displayName: "Browser Navigate", interact: true,
		description: "Open one http(s) page in the current conversation's Awaken browser tab. Do not use WebFetch or Bash/curl to drive the visible page.",
		purpose:     "Navigate the Desktop embedded browser to a public http(s) URL and admit that origin.",
		parameters:  []tool.ToolParameter{{Name: "url", Description: "http(s) URL or host to open.", Type: "string", Required: true}},
		inputSchema: tool.ToolParametersSchema{Type: "object", Properties: map[string]tool.ToolParam{"url": {Type: "string", Description: "http(s) URL or host."}}, Required: []string{"url"}, AdditionalProperties: tool.BoolPtr(false)},
		outputDesc:  "Loaded URL and title.",
		whenToUse:   []string{"The user asked to open a website in Awaken", "A later click or type needs a live page"},
		whenNotToUse: []string{"Use WebFetch to read page text without a visible tab", "Use WebSearch to discover URLs", "Do not open local HTML, file:, or javascript: URLs"},
		examples:    []tool.ToolExample{{Scenario: "Open a docs site", Input: map[string]any{"url": "example.com"}, Outcome: "The browser tab loads https://example.com/"}},
		failures: []tool.ToolFailureMode{
			{Code: "browser_host_unavailable", Cause: "Desktop browser host is not registered", Retryable: true},
			{Code: "unsupported_scheme", Cause: "URL is not http(s)", Retryable: false},
			{Code: "browser_view_not_visible", Cause: "The conversation browser tab is not shown", Retryable: true},
		},
	}, tool.DefinitionValidationStrictBuiltin)
	tool.MustRegisterToolWithMode(&browserTool{
		name: ToolSnapshot, action: "snapshot", displayName: "Browser Snapshot", readOnly: true,
		description: "Read numbered interactive elements from the visible Awaken browser tab. Do not use this for local HTML preview.",
		purpose:     "Capture a bounded accessibility snapshot of the visible public webpage.",
		inputSchema: tool.ToolParametersSchema{Type: "object", Properties: map[string]tool.ToolParam{}, AdditionalProperties: tool.BoolPtr(false)},
		outputDesc:  "Page URL, title, and numbered element refs.",
		whenToUse:   []string{"Before clicking or typing in the embedded browser", "To inspect the current public page"},
		whenNotToUse: []string{"Use WebFetch for a static text extract", "Use computer_observe for the OS desktop", "Do not snapshot project HTML preview"},
		examples:    []tool.ToolExample{{Scenario: "List clickable controls", Input: map[string]any{}, Outcome: "Returns [1] a Home and other numbered refs"}},
		failures: []tool.ToolFailureMode{
			{Code: "browser_host_unavailable", Cause: "Desktop browser host is not registered", Retryable: true},
			{Code: "browser_not_shown", Cause: "The browser tab is not the visible surface", Retryable: true},
		},
	}, tool.DefinitionValidationStrictBuiltin)
	tool.MustRegisterToolWithMode(&browserTool{
		name: ToolClick, action: "click", displayName: "Browser Click", interact: true,
		description: "Click a numbered element from the latest browser_snapshot on the visible public page.",
		purpose:     "Activate one snapshot ref in the current origin lease.",
		parameters:  []tool.ToolParameter{{Name: "ref", Description: "Element ref such as 1 or [1].", Type: "string", Required: true}},
		inputSchema: tool.ToolParametersSchema{Type: "object", Properties: map[string]tool.ToolParam{"ref": {Type: "string", Description: "Snapshot element ref."}}, Required: []string{"ref"}, AdditionalProperties: tool.BoolPtr(false)},
		outputDesc:  "URL after the click.",
		whenToUse:   []string{"After browser_snapshot when a visible control must be activated"},
		whenNotToUse: []string{"Use computer_act for OS UI", "Do not click after a cross-origin navigation without a new snapshot"},
		examples:    []tool.ToolExample{{Scenario: "Click the first link", Input: map[string]any{"ref": "[1]"}, Outcome: "Clicks the element tagged data-awaken-ref=1"}},
		failures: []tool.ToolFailureMode{
			{Code: "browser_view_not_visible", Cause: "The page is not painted", Retryable: true},
			{Code: "origin_lease_expired", Cause: "The page origin changed after admission", Retryable: true, Recovery: []string{"browser_navigate", "browser_snapshot"}},
			{Code: "element_not_found", Cause: "The snapshot ref is gone", Retryable: true},
		},
	}, tool.DefinitionValidationStrictBuiltin)
	tool.MustRegisterToolWithMode(&browserTool{
		name: ToolType, action: "type", displayName: "Browser Type", interact: true,
		description: "Type into a numbered input from the latest browser_snapshot. Do not enter secrets.",
		purpose:     "Fill one snapshot text control in the current origin lease.",
		parameters: []tool.ToolParameter{
			{Name: "ref", Description: "Element ref such as 1 or [1].", Type: "string", Required: true},
			{Name: "text", Description: "Non-secret text to enter.", Type: "string", Required: true},
		},
		inputSchema: tool.ToolParametersSchema{Type: "object", Properties: map[string]tool.ToolParam{
			"ref":  {Type: "string", Description: "Snapshot element ref."},
			"text": {Type: "string", Description: "Non-secret text."},
		}, Required: []string{"ref", "text"}, AdditionalProperties: tool.BoolPtr(false)},
		outputDesc:  "URL after typing.",
		whenToUse:   []string{"After browser_snapshot when a visible text field must be filled"},
		whenNotToUse: []string{"Do not type passwords, OTP, or API keys", "Use computer_act for OS inputs"},
		examples:    []tool.ToolExample{{Scenario: "Fill a search box", Input: map[string]any{"ref": "2", "text": "awaken"}, Outcome: "Sets the input value and dispatches input"}},
		failures: []tool.ToolFailureMode{
			{Code: "browser_view_not_visible", Cause: "The page is not painted", Retryable: true},
			{Code: "origin_lease_expired", Cause: "The page origin changed after admission", Retryable: true},
			{Code: "element_not_found", Cause: "The snapshot ref is gone", Retryable: true},
		},
	}, tool.DefinitionValidationStrictBuiltin)
	tool.MustRegisterToolWithMode(&browserTool{
		name: ToolWait, action: "wait", displayName: "Browser Wait", readOnly: true,
		description: "Wait on the visible Awaken browser tab while a public page settles.",
		purpose:     "Pause briefly so a live page can finish loading before the next snapshot.",
		parameters:  []tool.ToolParameter{{Name: "seconds", Description: "Seconds to wait, between 0.2 and 120.", Type: "number"}},
		inputSchema: tool.ToolParametersSchema{Type: "object", Properties: map[string]tool.ToolParam{"seconds": {Type: "number", Description: "Seconds to wait."}}, AdditionalProperties: tool.BoolPtr(false)},
		outputDesc:  "URL and title after waiting.",
		whenToUse:   []string{"After navigate or click when the page is still loading"},
		whenNotToUse: []string{"Do not use as a substitute for a snapshot", "Use computer_act wait for OS UI"},
		examples:    []tool.ToolExample{{Scenario: "Wait one second", Input: map[string]any{"seconds": 1}, Outcome: "Returns after the delay"}},
		failures: []tool.ToolFailureMode{
			{Code: "browser_not_shown", Cause: "The browser tab is hidden", Retryable: true},
		},
	}, tool.DefinitionValidationStrictBuiltin)
	tool.MustRegisterToolWithMode(&browserTool{
		name: ToolExtract, action: "extract", displayName: "Browser Extract", readOnly: true,
		description: "Read bounded visible text from the current public page. Use WebFetch when no visible tab is needed.",
		purpose:     "Extract a slice of the painted page body text.",
		parameters:  []tool.ToolParameter{{Name: "start", Description: "Character offset into the page text.", Type: "integer"}},
		inputSchema: tool.ToolParametersSchema{Type: "object", Properties: map[string]tool.ToolParam{"start": {Type: "integer", Description: "Character offset."}}, AdditionalProperties: tool.BoolPtr(false)},
		outputDesc:  "URL, title, text slice, and truncation flag.",
		whenToUse:   []string{"The visible page text is needed after interacting with it"},
		whenNotToUse: []string{"Use WebFetch for a fresh independent fetch", "Do not extract project HTML preview"},
		examples:    []tool.ToolExample{{Scenario: "Read the article body", Input: map[string]any{"start": 0}, Outcome: "Returns up to 16000 characters"}},
		failures: []tool.ToolFailureMode{
			{Code: "browser_not_shown", Cause: "The browser tab is hidden", Retryable: true},
		},
	}, tool.DefinitionValidationStrictBuiltin)
}
