// Package computer registers the three strict-v2 Computer Use tools. Direct
// execution is deliberately unsupported: real calls are routed through the
// run-bound ComputerExecutor so session, policy, review and Host fences cannot
// be bypassed.
package computer

import (
	"context"
	"encoding/json"

	"awaken/src/tool"
)

const (
	ToolNameObserve = "computer_observe"
	ToolNameAct     = "computer_act"
	ToolNameVerify  = "computer_verify"
)

type observeTool struct{}
type actTool struct{}
type verifyTool struct{}

func (t *observeTool) Name() string { return ToolNameObserve }
func (t *observeTool) Description() string {
	return "Capture a fresh screenshot and accessibility state through the run-bound Computer lane; terminal output is not Computer observation evidence."
}
func (t *observeTool) Parameters() []tool.ToolParameter {
	return []tool.ToolParameter{
		{Name: "scope", Description: "Capture scope: window or desktop.", Type: "string"},
		{Name: "target_ref", Description: "Exact pid:<pid>/window:<window_id> reference returned by a desktop observation.", Type: "string"},
		{Name: "include", Description: "List of what to include: screenshot, accessibility.", Type: "array"},
		{Name: "reason", Description: "Why this observation is needed.", Type: "string"},
	}
}
func (t *observeTool) Definition() tool.ToolDefinition {
	return tool.BuildBuiltinDefinition(t, tool.ToolDefinition{
		Name:         ToolNameObserve,
		DisplayName:  "Computer Observe",
		Description:  t.Description(),
		Purpose:      "Get a fresh, authorized observation of the current Computer scope to inform the next action.",
		WhenToUse:    []string{"Before any direct GUI action", "When the previous observation may be stale", "To verify the visible result of an action"},
		WhenNotToUse: []string{"For Python, Typst, builds, converters, or Skill scripts; use Bash", "When typed tools, MCP tools or browser automation can solve the task", "When Computer Use is not enabled for the run; do not impersonate it with terminal GUI scripts"},
		InputSchema: tool.ToolParametersSchema{
			Type: "object",
			Properties: map[string]tool.ToolParam{
				"scope":      {Type: "string", Description: "Capture scope: window or desktop.", Enum: []any{"window", "desktop"}},
				"target_ref": {Type: "string", Description: "Required for window scope. Use the exact pid:<pid>/window:<window_id> value returned in a desktop observation's available_windows."},
				"include":    {Type: "array", Description: "List of what to include: screenshot, accessibility.", Items: &tool.ToolParam{Type: "string", Enum: []any{"screenshot", "accessibility"}}},
				"reason":     {Type: "string", Description: "Why this observation is needed."},
			},
			AdditionalProperties: tool.BoolPtr(false),
		},
		OutputSchema: tool.ToolParam{
			Type: "object", Description: "Observation ID, session, scope, target generation, capture timestamp and requested accessibility elements.",
			AdditionalProperties: tool.BoolPtr(false),
			Properties: map[string]tool.ToolParam{
				"observation_id":           {Type: "string", Description: "Fresh observation identifier."},
				"session_id":               {Type: "string", Description: "Computer session identifier."},
				"scope":                    {Type: "string", Description: "Resolved capture scope."},
				"target_generation":        {Type: "integer", Description: "Target generation bound to the observation."},
				"captured_at":              {Type: "string", Description: "RFC3339 capture timestamp."},
				"coordinate_unit":          {Type: "string", Description: "Pointer coordinate unit. Current CUA observations use screen_point."},
				"screen_width":             {Type: "integer", Description: "Width of the model-visible screenshot and pointer coordinate canvas."},
				"screen_height":            {Type: "integer", Description: "Height of the model-visible screenshot and pointer coordinate canvas."},
				"source_screenshot_width":  {Type: "integer", Description: "Original native screenshot width before coordinate normalization."},
				"source_screenshot_height": {Type: "integer", Description: "Original native screenshot height before coordinate normalization."},
				"backing_scale_x":          {Type: "number", Description: "Native backing-pixel scale applied by the trusted Host."},
				"backing_scale_y":          {Type: "number", Description: "Native backing-pixel scale applied by the trusted Host."},
				"target_ref":               {Type: "string", Description: "Exact target bound to a window observation."},
				"target_pid":               {Type: "integer", Description: "Exact process ID bound to a window observation."},
				"target_window_id":         {Type: "string", Description: "Exact native window ID bound to a window observation."},
				"available_windows":        {Type: "array", Description: "On-screen exact window candidates returned by a desktop observation. Select one target_ref before requesting window scope.", Items: &tool.ToolParam{Type: "object", AdditionalProperties: tool.BoolPtr(true)}},
				"accessibility":            {Type: "object", Description: "Requested structured accessibility state. For reliable background window clicks, select an element by label/role and pass its exact element_token as target.locator with the same observation_id.", AdditionalProperties: tool.BoolPtr(true)},
			},
			Required: []string{"observation_id", "session_id", "scope", "target_generation", "captured_at"},
		},
		SideEffects: []tool.ToolSideEffect{{Kind: tool.SideEffectRead, Resources: []string{"authorized screen/accessibility state"}, Description: "Reads the authorized screen or accessibility state.", Reversibility: tool.ReversibilityReversible}},
		Examples: []tool.ToolExample{
			{Scenario: "Capture the current window", Input: map[string]any{"scope": "window", "include": []string{"screenshot", "accessibility"}, "reason": "Plan the next click"}, Outcome: "Returns observation_id, scope and media references."},
		},
		FailureModes: []tool.ToolFailureMode{
			{Code: "computer_unsupported", Cause: "Computer runtime is not bound", Retryable: false, Recovery: []string{"enable_computer_runtime", "use_typed_tools"}},
			{Code: "computer_permission_denied", Cause: "Screen Recording or Accessibility permission missing", Retryable: true, Recovery: []string{"grant_permissions", "retry"}},
		},
		Annotations:  tool.ToolAnnotations{ReadOnly: true, CapabilityClass: tool.CapabilityClassComputerObserve, TrustedOrigin: tool.TrustedOriginBuiltin},
		Permissions:  tool.ToolPermissionSpec{CapabilityClass: tool.CapabilityClassComputerObserve, NetworkScope: tool.NetworkScopeNone},
		ProviderID:   tool.ProviderID("builtin:computer_observe"),
		Capabilities: []tool.Capability{tool.CapabilityComputerObserve},
	})
}
func (t *observeTool) Execute(ctx context.Context, input string) (string, error) {
	outcome, err := t.ExecuteOutcome(ctx, input, nil)
	return outcome.Content, err
}
func (t *observeTool) ExecuteOutcome(ctx context.Context, input string, observer tool.ToolObserver) (tool.ExecutionOutcome, error) {
	return unsupportedOutcome(), nil
}
func (t *observeTool) ProviderReadiness(env tool.ExecutionEnvironmentSnapshot) tool.ProviderReadinessReport {
	return computerReadiness(env)
}

func (t *actTool) Name() string { return ToolNameAct }
func (t *actTool) Description() string {
	return "Propose a strongly-typed GUI action through the run-bound Computer lane based on a fresh observation. Windows Shell global shortcuts using the Windows/Meta/Command key are not reliably supported by the current desktop route; click visible system UI from a fresh screenshot instead."
}
func (t *actTool) Parameters() []tool.ToolParameter {
	return []tool.ToolParameter{
		{Name: "observation_id", Description: "ID of the observation this action is based on.", Type: "string", Required: true},
		{Name: "action", Description: "Action kind: click, double_click, scroll, drag, key, type_text, wait.", Type: "string", Required: true},
		{Name: "target", Description: "Target element reference.", Type: "object", Required: true},
		{Name: "parameters", Description: "Action-specific parameters.", Type: "object"},
		{Name: "expected_context", Description: "Expected context after the action.", Type: "object"},
		{Name: "intent", Description: "What the action is trying to accomplish.", Type: "string"},
		{Name: "expected_effect", Description: "Expected external effect.", Type: "string"},
		{Name: "idempotency_key", Description: "Key for idempotency.", Type: "string"},
	}
}
func (t *actTool) Definition() tool.ToolDefinition {
	return tool.BuildBuiltinDefinition(t, tool.ToolDefinition{
		Name:         ToolNameAct,
		DisplayName:  "Computer Act",
		Description:  t.Description(),
		Purpose:      "Execute a checked GUI action after policy evaluation and any required human review.",
		WhenToUse:    []string{"When a typed tool or API is unavailable", "After a fresh observation", "When the action is within the authorized scope and risk level"},
		WhenNotToUse: []string{"For Python, Typst, builds, converters, or Skill scripts; use Bash", "For secret input, authentication, security warnings or bypassing access control", "Without a fresh observation", "For Windows Shell global shortcuts using win/windows/meta/cmd/command; the current CUA route cannot verify that the Shell handled them, so click visible system UI instead", "When Computer Use is not enabled; do not replace it with terminal GUI scripts unless the user explicitly requested terminal automation"},
		InputSchema: tool.ToolParametersSchema{
			Type: "object",
			Properties: map[string]tool.ToolParam{
				"observation_id": {Type: "string", Description: "ID of the observation this action is based on."},
				"action":         {Type: "string", Description: "Action kind.", Enum: []any{"click", "double_click", "scroll", "drag", "key", "hotkey", "type_text", "move_cursor", "wait"}},
				"target": {Type: "object", Description: "Target element or coordinates. Background window click/double-click requires the exact element_token from that observation as locator; foreground pointer actions use a complete x/y pair or pixel_fallback point.", AdditionalProperties: tool.BoolPtr(true), Properties: map[string]tool.ToolParam{
					"locator": {Type: "string", Description: "Required for a background window click/double-click. Use the exact element_token from the bound accessibility observation."},
					"x":       {Type: "integer", Minimum: tool.FloatPtr(0), Description: "Horizontal coordinate in the model-visible screenshot. Use that screenshot's full pixel width directly; the Host applies any platform DPI/backing transform."},
					"y":       {Type: "integer", Minimum: tool.FloatPtr(0), Description: "Vertical coordinate in the model-visible screenshot. Use that screenshot's full pixel height directly; the Host applies any platform DPI/backing transform."},
					"pixel_fallback": {Type: "object", Description: "Explicit complete screen-point fallback.", Required: []string{"x", "y"}, AdditionalProperties: tool.BoolPtr(false), Properties: map[string]tool.ToolParam{
						"x": {Type: "integer", Minimum: tool.FloatPtr(0), Description: "Horizontal screen-point coordinate."},
						"y": {Type: "integer", Minimum: tool.FloatPtr(0), Description: "Vertical screen-point coordinate."},
					}},
				}},
				"parameters":       {Type: "object", Description: "Action-specific parameters. Window observations default to delivery_mode=background, which must not move the physical cursor, raise the target, or change foreground. Background click/double-click requires target.locator with the exact element_token from the same observation; screenshot-coordinate background clicks fail closed because synthetic event delivery cannot prove the target handled the input. Foreground pointer actions use window-local screenshot pixels. Desktop observations use full-screen coordinates and foreground delivery only. Use click/move_cursor x/y; scroll x/y/direction/amount; drag from_x/from_y/to_x/to_y. Keyboard: key key; hotkey keys; type_text text. Optional delivery_mode is background or foreground. Windows Shell global hotkeys containing win/windows/meta/cmd/command are rejected on the desktop route. Wait: ms. Required fields are validated before any OS input.", AdditionalProperties: tool.BoolPtr(true)},
				"expected_context": {Type: "object", Description: "Expected context after the action.", AdditionalProperties: tool.BoolPtr(true)},
				"intent":           {Type: "string", Description: "What the action is trying to accomplish."},
				"expected_effect":  {Type: "string", Description: "Expected external effect."},
				"idempotency_key":  {Type: "string", Description: "Key for idempotency."},
			},
			Required:             []string{"observation_id", "action", "target"},
			AdditionalProperties: tool.BoolPtr(false),
		},
		OutputSchema: tool.ToolParam{
			Type: "object", Description: "Action ID, status, observation IDs and risk.",
			AdditionalProperties: tool.BoolPtr(false),
			Properties: map[string]tool.ToolParam{
				"action_id":             {Type: "string", Description: "Prepared action identifier."},
				"action_status":         {Type: "string", Description: "Checked action outcome."},
				"source_observation_id": {Type: "string", Description: "Observation used to prepare the action."},
				"observation_id":        {Type: "string", Description: "Fresh post-action observation when available."},
				"risk":                  {Type: "string", Description: "Runtime risk classification."},
				"review_id":             {Type: "string", Description: "Human review identifier when required."},
				"handoff":               {Type: "string", Description: "Handoff completion state when applicable."},
				"decision":              {Type: "string", Description: "Human review decision when applicable."},
				"actor":                 {Type: "string", Description: "Review decision actor when applicable."},
				"host_error_code":       {Type: "string", Description: "Stable host failure code when the checked action fails."},
				"host_error_category":   {Type: "string", Description: "Host failure category when the checked action fails."},
				"host_error_message":    {Type: "string", Description: "Diagnostic failure message returned by the CUA host."},
				"cua_status":            {Type: "string", Description: "Raw bounded CUA result text for diagnostics."},
				"cua_effect":            {Type: "string", Description: "CUA-confirmed effect classification."},
				"cua_route":             {Type: "string", Description: "CUA delivery route such as accessibility or synthetic_events."},
				"cua_delivery_mode":     {Type: "string", Description: "Actual CUA delivery mode: background, foreground, not_applicable, or unknown."},
			},
		},
		SideEffects: []tool.ToolSideEffect{{Kind: tool.SideEffectExecute, Resources: []string{"authorized GUI target"}, Description: "May inject mouse/keyboard input into the authorized scope.", Reversibility: tool.ReversibilityUnknown}},
		Examples: []tool.ToolExample{
			{Scenario: "Background-click an OK button", Input: map[string]any{"observation_id": "obs_xxx", "action": "click", "target": map[string]any{"locator": "s00000001:12"}, "parameters": map[string]any{"delivery_mode": "background"}, "intent": "Dismiss the dialog", "expected_effect": "Dialog closes", "idempotency_key": "idemp-1"}, Outcome: "Returns action_id and actual CUA delivery diagnostics."},
		},
		FailureModes: []tool.ToolFailureMode{
			{Code: "computer_unsupported", Cause: "Computer runtime is not bound", Retryable: false, Recovery: []string{"enable_computer_runtime", "use_typed_tools"}},
			{Code: "computer_action_forbidden", Cause: "Action violates policy", Retryable: false, Recovery: []string{"change_approach", "ask_user"}},
			{Code: "computer_stale_observation", Cause: "The observation changed before perform", Retryable: true, Recovery: []string{"computer_observe", "retry"}},
			{Code: tool.ErrorCodeValidationError, Cause: "The selected action is missing required parameters, contains an incomplete coordinate set, or attempts a background click without an observation-bound element token", Retryable: false, Recovery: []string{"request_accessibility_if_background_clicking", "correct_action_parameters", "observe_again_if_the_screen_changed"}},
		},
		Annotations:  tool.ToolAnnotations{ReadOnly: false, Destructive: true, CapabilityClass: tool.CapabilityClassComputerAct, TrustedOrigin: tool.TrustedOriginBuiltin},
		Permissions:  tool.ToolPermissionSpec{CapabilityClass: tool.CapabilityClassComputerAct, NetworkScope: tool.NetworkScopeNone, RequiresReview: "high-risk GUI actions"},
		ProviderID:   tool.ProviderID("builtin:computer_act"),
		Capabilities: []tool.Capability{tool.CapabilityComputerAct},
	})
}
func (t *actTool) Execute(ctx context.Context, input string) (string, error) {
	outcome, err := t.ExecuteOutcome(ctx, input, nil)
	return outcome.Content, err
}
func (t *actTool) ExecuteOutcome(ctx context.Context, input string, observer tool.ToolObserver) (tool.ExecutionOutcome, error) {
	return unsupportedOutcome(), nil
}
func (t *actTool) ProviderReadiness(env tool.ExecutionEnvironmentSnapshot) tool.ProviderReadinessReport {
	return computerReadiness(env)
}

func (t *verifyTool) Name() string { return ToolNameVerify }
func (t *verifyTool) Description() string {
	return "Verify through fresh Computer-lane evidence that the expected effect of a previous Computer action occurred."
}
func (t *verifyTool) Parameters() []tool.ToolParameter {
	return []tool.ToolParameter{
		{Name: "action_id", Description: "ID of the action to verify.", Type: "string", Required: true},
		{Name: "expected_effect", Description: "Expected effect.", Type: "string"},
		{Name: "assertions", Description: "Machine-checkable assertions.", Type: "array"},
	}
}
func (t *verifyTool) Definition() tool.ToolDefinition {
	return tool.BuildBuiltinDefinition(t, tool.ToolDefinition{
		Name:         ToolNameVerify,
		DisplayName:  "Computer Verify",
		Description:  t.Description(),
		Purpose:      "Independently check whether a GUI action produced its expected effect using a fresh observation.",
		WhenToUse:    []string{"After computer_act", "When the agent needs to confirm a state change before proceeding"},
		WhenNotToUse: []string{"For validating Bash or terminal-driven GUI effects as if they were Computer actions", "As a substitute for a fresh observation", "When Computer Use is not enabled"},
		InputSchema: tool.ToolParametersSchema{
			Type: "object",
			Properties: map[string]tool.ToolParam{
				"action_id":       {Type: "string", Description: "ID of the action to verify."},
				"expected_effect": {Type: "string", Description: "Expected effect."},
				"assertions":      {Type: "array", Description: "Machine-checkable assertions.", Items: &tool.ToolParam{Type: "object", AdditionalProperties: tool.BoolPtr(true)}},
			},
			Required:             []string{"action_id"},
			AdditionalProperties: tool.BoolPtr(false),
		},
		OutputSchema: tool.ToolParam{
			Type: "object", Description: "Verification status, action ID, observation ID and evaluated assertions.",
			AdditionalProperties: tool.BoolPtr(false),
			Properties: map[string]tool.ToolParam{
				"action_id":           {Type: "string", Description: "Verified action identifier."},
				"verification_status": {Type: "string", Description: "Verification result."},
				"observation_id":      {Type: "string", Description: "Fresh observation used for verification."},
				"assertions":          {Type: "array", Description: "Evaluated assertions.", Items: &tool.ToolParam{Type: "object", Description: "One evaluated assertion.", AdditionalProperties: tool.BoolPtr(true)}},
			},
			Required: []string{"action_id", "verification_status", "observation_id", "assertions"},
		},
		SideEffects: []tool.ToolSideEffect{{Kind: tool.SideEffectRead, Resources: []string{"authorized screen/accessibility state"}, Description: "Reads the authorized screen or accessibility state.", Reversibility: tool.ReversibilityReversible}},
		Examples: []tool.ToolExample{
			{Scenario: "Verify a dialog closed", Input: map[string]any{"action_id": "act_xxx", "expected_effect": "Dialog is gone", "assertions": []map[string]any{{"type": "element_absent", "target": "dialog:Confirm", "expected": "true"}}}, Outcome: "Returns verified/not_verified/inconclusive."},
		},
		FailureModes: []tool.ToolFailureMode{
			{Code: "computer_unsupported", Cause: "Computer runtime is not bound", Retryable: false, Recovery: []string{"enable_computer_runtime", "use_typed_tools"}},
			{Code: "computer_verification_inconclusive", Cause: "Cannot determine expected effect from observation", Retryable: true, Recovery: []string{"computer_observe", "refine_assertions"}},
		},
		Annotations:  tool.ToolAnnotations{ReadOnly: true, CapabilityClass: tool.CapabilityClassComputerVerify, TrustedOrigin: tool.TrustedOriginBuiltin},
		Permissions:  tool.ToolPermissionSpec{CapabilityClass: tool.CapabilityClassComputerVerify, NetworkScope: tool.NetworkScopeNone},
		ProviderID:   tool.ProviderID("builtin:computer_verify"),
		Capabilities: []tool.Capability{tool.CapabilityComputerVerify},
	})
}
func (t *verifyTool) Execute(ctx context.Context, input string) (string, error) {
	outcome, err := t.ExecuteOutcome(ctx, input, nil)
	return outcome.Content, err
}
func (t *verifyTool) ExecuteOutcome(ctx context.Context, input string, observer tool.ToolObserver) (tool.ExecutionOutcome, error) {
	return unsupportedOutcome(), nil
}
func (t *verifyTool) ProviderReadiness(env tool.ExecutionEnvironmentSnapshot) tool.ProviderReadinessReport {
	return computerReadiness(env)
}

func computerReadiness(env tool.ExecutionEnvironmentSnapshot) tool.ProviderReadinessReport {
	if !env.ComputerEnabled {
		return tool.ProviderReadinessReport{
			Readiness:                 tool.ProviderUnavailable,
			Reason:                    "computer runtime is not enabled for this run",
			RetryableAfterRemediation: false,
		}
	}
	return tool.ProviderReadinessReport{
		Readiness:                 tool.ProviderReady,
		Reason:                    "run-bound computer runtime and host are ready",
		RetryableAfterRemediation: false,
	}
}

func unsupportedOutcome() tool.ExecutionOutcome {
	msg := "Computer Use is not yet available in this run. Enable a Computer runtime or use typed tools/MCP instead."
	outcome := tool.ExecutionOutcome{
		Status:          tool.OutcomeUnsupported,
		TransportStatus: tool.TransportNotStarted,
		SemanticStatus:  tool.SemanticFailure,
		Content:         msg,
		Summary:         "Computer Use not available",
		ErrorCode:       "computer_unsupported",
		ErrorCategory:   tool.ErrorCategoryUnsupported,
		ErrorMessage:    msg,
		Retryable:       false,
	}
	return outcome
}

func marshalJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func init() {
	// Force registration so tests and the registry can see the definitions.
	// Runtime availability is determined by the run snapshot, not by this
	// global registration.
	tool.MustRegisterToolWithMode(&observeTool{}, tool.DefinitionValidationStrictBuiltin)
	tool.MustRegisterToolWithMode(&actTool{}, tool.DefinitionValidationStrictBuiltin)
	tool.MustRegisterToolWithMode(&verifyTool{}, tool.DefinitionValidationStrictBuiltin)
}
