package executors

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"awaken/src/computer"
	"awaken/src/computer/fake"
	"awaken/src/tool"
	"awaken/src/tool/profile"
)

func TestComputerExecutorNeedsRuntimeWhenUnbound(t *testing.T) {
	ctx := context.Background()
	exec := NewComputerExecutor(nil)
	toolObj := &toolStub{name: "computer_observe"}
	intent := profile.ExecutionIntent{ToolName: "computer_observe", Lane: profile.LaneComputer}
	out, err := exec.Execute(ctx, intent, profile.DefaultProfile(), toolObj, `{"scope":"window","include":["screenshot"]}`, nil)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if out.Outcome.Status != tool.OutcomeNeedsRuntime {
		t.Fatalf("expected needs_runtime, got %s", out.Outcome.Status)
	}
}

func TestComputerActInputNormalizesModelFriendlyTargetFields(t *testing.T) {
	x, y := 120, 240
	input := actInput{
		Action: string(computer.ActionKey),
		Target: actTargetInput{
			X:   &x,
			Y:   &y,
			Key: "command+space",
		},
	}
	target, parameters := input.normalizedTargetAndParameters()
	if target.PixelFallback == nil || target.PixelFallback.X != x || target.PixelFallback.Y != y {
		t.Fatalf("pixel target was not normalized: %+v", target.PixelFallback)
	}
	if parameters["key"] != "command+space" {
		t.Fatalf("key was not normalized: %+v", parameters)
	}
}

func TestSplitHotkeyAcceptsModelStringForm(t *testing.T) {
	keys := splitHotkey("command + space")
	if len(keys) != 2 || keys[0] != "command" || keys[1] != "space" {
		t.Fatalf("unexpected hotkey normalization: %#v", keys)
	}
}

func TestNormalizedKeyChordConvertsPrintableSymbols(t *testing.T) {
	for input, want := range map[string][]string{
		"*": {"shift", "8"},
		"+": {"shift", "="},
	} {
		got := normalizedKeyChord(input)
		if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
			t.Fatalf("normalizedKeyChord(%q) = %#v, want %#v", input, got, want)
		}
	}
}

func TestEndedHostSessionErrorDetection(t *testing.T) {
	if !isEndedHostSessionError(fmt.Errorf("CUA session 'abc' has ended")) {
		t.Fatal("expected ended host session error to be detected")
	}
	if isEndedHostSessionError(fmt.Errorf("screen capture failed")) {
		t.Fatal("unrelated errors must not trigger session recovery")
	}
}

func TestComputerExecutorUnknownTool(t *testing.T) {
	ctx := context.Background()
	exec := NewComputerExecutor(nil)
	toolObj := &toolStub{name: "computer_unknown"}
	intent := profile.ExecutionIntent{ToolName: "computer_unknown", Lane: profile.LaneComputer}
	out, err := exec.Execute(ctx, intent, profile.DefaultProfile(), toolObj, `{}`, nil)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if out.Outcome.Status != tool.OutcomeDenied {
		t.Fatalf("expected denied, got %s", out.Outcome.Status)
	}
}

func TestComputerExecutorObserveActVerify(t *testing.T) {
	ctx := context.Background()
	policy, _ := computer.NewPolicySnapshot(computer.DefaultPolicy())
	bind, _ := computer.NewBinding("run-1", "thread-1", "agent-1", "", policy)
	driver := fake.NewDriver()
	runtime := computer.NewRuntime(bind, driver)
	exec := NewComputerExecutor(runtime)

	toolObj := &toolStub{name: "computer_observe"}
	intent := profile.ExecutionIntent{ToolName: "computer_observe", Lane: profile.LaneComputer}
	out, err := exec.Execute(ctx, intent, profile.DefaultProfile(), toolObj, `{"scope":"window","include":["screenshot"],"reason":"start"}`, nil)
	if err != nil {
		t.Fatalf("observe execute: %v", err)
	}
	if out.Outcome.Status != tool.OutcomeSucceeded {
		t.Fatalf("expected observe succeeded, got %s (%s)", out.Outcome.Status, out.Outcome.ErrorMessage)
	}
	if _, active := runtime.Session(); !active {
		t.Fatal("expected first observe to lazily open a Computer session")
	}
}

func TestComputerExecutorObserveDefaultsToDesktop(t *testing.T) {
	policy, _ := computer.NewPolicySnapshot(computer.DefaultPolicy())
	bind, _ := computer.NewBinding("run-1", "thread-1", "agent-1", "", policy)
	driver := fake.NewDriver()
	runtime := computer.NewRuntime(bind, driver)
	exec := NewComputerExecutor(runtime)

	out := mustExecute(t, exec, "computer_observe", `{"include":["screenshot"]}`)
	if out.Outcome.Status != tool.OutcomeSucceeded {
		t.Fatalf("observe failed: %s", out.Outcome.ErrorMessage)
	}
	sess, active := runtime.Session()
	if !active {
		t.Fatal("expected active session")
	}
	if sess.CaptureScope != computer.CaptureDesktop {
		t.Fatalf("expected default desktop scope, got %s", sess.CaptureScope)
	}
	windows, ok := out.Outcome.Details["available_windows"].([]computer.WindowRef)
	if !ok {
		t.Fatal("desktop observation must always expose available_windows, even when discovery is empty")
	}
	if windows == nil || len(windows) != 0 {
		t.Fatalf("expected a non-nil empty available_windows array, got %#v", windows)
	}
	if !strings.Contains(out.Content, `"available_windows":[]`) {
		t.Fatalf("model-visible desktop observation must encode an empty array, got %s", out.Content)
	}
}

func TestComputerExecutorObserveReopensWhenScopeChanges(t *testing.T) {
	policy, _ := computer.NewPolicySnapshot(computer.DefaultPolicy())
	bind, _ := computer.NewBinding("run-1", "thread-1", "agent-1", "", policy)
	driver := fake.NewDriver()
	runtime := computer.NewRuntime(bind, driver)
	exec := NewComputerExecutor(runtime)

	first := mustExecute(t, exec, "computer_observe", `{"scope":"window"}`)
	if first.Outcome.Status != tool.OutcomeSucceeded {
		t.Fatalf("window observe failed: %s", first.Outcome.ErrorMessage)
	}
	firstSession := first.Outcome.Details["session_id"].(string)
	second := mustExecute(t, exec, "computer_observe", `{"scope":"desktop"}`)
	if second.Outcome.Status != tool.OutcomeSucceeded {
		t.Fatalf("desktop observe failed: %s", second.Outcome.ErrorMessage)
	}
	if second.Outcome.Details["session_id"] == firstSession {
		t.Fatal("expected scope change to reopen the computer session")
	}
}

func TestComputerExecutorObserveRecoversEndedHostSessionOnce(t *testing.T) {
	policy, _ := computer.NewPolicySnapshot(computer.DefaultPolicy())
	bind, _ := computer.NewBinding("run-1", "thread-1", "agent-1", "", policy)
	driver := fake.NewDriver()
	runtime := computer.NewRuntime(bind, driver)
	exec := NewComputerExecutor(runtime)

	observeCalls := 0
	var opened computer.Session
	driver.OnOpenSession = func(_ computer.Binding, proposed computer.Session, _ computer.PolicySnapshot, _ computer.FenceToken) (computer.Session, error) {
		opened = proposed
		opened.HostSessionRef = "host-" + string(proposed.SessionID)
		if err := opened.MarkReady(); err != nil {
			return computer.Session{}, err
		}
		return opened, nil
	}
	driver.OnObserve = func(_ computer.SessionID, scope computer.CaptureScope, target computer.TargetRef, _ computer.ObserveOptions) (computer.Observation, error) {
		observeCalls++
		if observeCalls == 1 {
			return computer.Observation{}, fmt.Errorf("CUA session 'expired' has ended")
		}
		now := time.Now().UTC()
		return computer.NewObservation(opened, scope, target, computer.Freshness{
			CapturedAt: now,
			FreshUntil: now.Add(time.Minute),
		})
	}

	out := mustExecute(t, exec, "computer_observe", `{"scope":"desktop"}`)
	if out.Outcome.Status != tool.OutcomeSucceeded {
		t.Fatalf("observe recovery failed: %s", out.Outcome.ErrorMessage)
	}
	if observeCalls != 2 {
		t.Fatalf("expected one recovery retry, got %d observe calls", observeCalls)
	}
	openCalls := 0
	for _, call := range driver.Calls {
		if call.Method == "OpenSession" {
			openCalls++
		}
	}
	if openCalls != 2 {
		t.Fatalf("expected host session to reopen, got %d open calls", openCalls)
	}
}

func TestComputerExecutorObserveIncludesScreenshotModelContent(t *testing.T) {
	ctx := context.Background()
	policy, _ := computer.NewPolicySnapshot(computer.DefaultPolicy())
	bind, _ := computer.NewBinding("run-1", "thread-1", "agent-1", "", policy)
	driver := fake.NewDriver()
	runtime := computer.NewRuntime(bind, driver)
	exec := NewComputerExecutor(runtime)

	sess, err := runtime.OpenSession(ctx, computer.CaptureWindow, computer.TargetRef{AppName: "FakeApp"})
	if err != nil {
		t.Fatalf("open session: %v", err)
	}

	// Write a tiny real PNG file for the fake driver to reference.
	workDir := t.TempDir()
	pngPath := filepath.Join(workDir, "shot.png")
	// Minimal valid 1x1 PNG.
	pngBytes, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==")
	if err := os.WriteFile(pngPath, pngBytes, 0600); err != nil {
		t.Fatalf("write png: %v", err)
	}

	driver.OnObserve = func(sessionID computer.SessionID, scope computer.CaptureScope, target computer.TargetRef, opts computer.ObserveOptions) (computer.Observation, error) {
		now := time.Now().UTC()
		obs, _ := computer.NewObservation(sess, scope, target, computer.Freshness{CapturedAt: now, FreshUntil: now.Add(30 * time.Second)})
		obs.Screenshot = &computer.MediaRef{
			StorageKey: pngPath,
			MIMEType:   "image/png",
			ByteSize:   int64(len(pngBytes)),
			SHA256:     "0000",
		}
		return obs, nil
	}

	toolObj := &toolStub{name: "computer_observe"}
	intent := profile.ExecutionIntent{ToolName: "computer_observe", Lane: profile.LaneComputer}
	out, err := exec.Execute(ctx, intent, profile.DefaultProfile(), toolObj, `{"scope":"window","include":["screenshot"]}`, nil)
	if err != nil {
		t.Fatalf("observe execute: %v", err)
	}
	if out.Outcome.Status != tool.OutcomeSucceeded {
		t.Fatalf("expected observe succeeded, got %s (%s)", out.Outcome.Status, out.Outcome.ErrorMessage)
	}
	if len(out.Outcome.ModelContent) != 1 {
		t.Fatalf("expected 1 model content part, got %d", len(out.Outcome.ModelContent))
	}
	part := out.Outcome.ModelContent[0]
	if part.Type != "image" || part.MimeType != "image/png" {
		t.Fatalf("expected image/png part, got %s/%s", part.Type, part.MimeType)
	}
	if part.DataBase64 == "" {
		t.Fatal("expected screenshot base64 data")
	}
}

func TestComputerExecutorObserveExposesAccessibilityElementTokens(t *testing.T) {
	ctx := context.Background()
	policy, _ := computer.NewPolicySnapshot(computer.DefaultPolicy())
	bind, _ := computer.NewBinding("run-1", "thread-1", "agent-1", "", policy)
	driver := fake.NewDriver()
	runtime := computer.NewRuntime(bind, driver)
	exec := NewComputerExecutor(runtime)

	sess, err := runtime.OpenSession(ctx, computer.CaptureWindow, computer.TargetRef{AppName: "FakeApp"})
	if err != nil {
		t.Fatalf("open session: %v", err)
	}
	workDir := t.TempDir()
	accessibilityPath := filepath.Join(workDir, "accessibility.json")
	accessibilityJSON := []byte(`{"elements":[{"role":"button","label":"Five","element_token":"s00000001:5"}],"elements_complete":true}`)
	if err := os.WriteFile(accessibilityPath, accessibilityJSON, 0600); err != nil {
		t.Fatalf("write accessibility state: %v", err)
	}
	driver.OnObserve = func(sessionID computer.SessionID, scope computer.CaptureScope, target computer.TargetRef, opts computer.ObserveOptions) (computer.Observation, error) {
		now := time.Now().UTC()
		obs, _ := computer.NewObservation(sess, scope, target, computer.Freshness{CapturedAt: now, FreshUntil: now.Add(30 * time.Second)})
		obs.AccessibilityTree = &computer.MediaRef{
			StorageKey: accessibilityPath,
			MIMEType:   "application/json",
			ByteSize:   int64(len(accessibilityJSON)),
		}
		return obs, nil
	}

	out := mustExecute(t, exec, "computer_observe", `{"scope":"window","include":["accessibility"]}`)
	if out.Outcome.Status != tool.OutcomeSucceeded {
		t.Fatalf("observe failed: %s", out.Outcome.ErrorMessage)
	}
	if !strings.Contains(out.Content, `"element_token":"s00000001:5"`) {
		t.Fatalf("model-facing observation omitted the element token: %s", out.Content)
	}
	if _, ok := out.Outcome.Details["accessibility"].(map[string]any); !ok {
		t.Fatalf("accessibility details missing or malformed: %#v", out.Outcome.Details["accessibility"])
	}
}

func TestComputerExecutorActUsesObservedObservation(t *testing.T) {
	ctx := context.Background()
	policy, _ := computer.NewPolicySnapshot(computer.DefaultPolicy())
	bind, _ := computer.NewBinding("run-1", "thread-1", "agent-1", "", policy)
	driver := fake.NewDriver()
	runtime := computer.NewRuntime(bind, driver)
	exec := NewComputerExecutor(runtime)

	if _, err := runtime.OpenSession(ctx, computer.CaptureWindow, computer.TargetRef{AppName: "FakeApp"}); err != nil {
		t.Fatalf("open session: %v", err)
	}

	observeOut := mustExecute(t, exec, "computer_observe", `{"scope":"window"}`)
	if observeOut.Outcome.Status != tool.OutcomeSucceeded {
		t.Fatalf("observe failed: %s", observeOut.Outcome.ErrorMessage)
	}
	obsID := observeOut.Outcome.Details["observation_id"].(string)

	actInput := fmt.Sprintf(`{"observation_id":%q,"action":"wait","parameters":{"ms":10},"target":{},"intent":"wait","expected_effect":"time passes"}`, obsID)
	actOut := mustExecute(t, exec, "computer_act", actInput)
	if actOut.Outcome.Status != tool.OutcomeSucceeded {
		t.Fatalf("expected act succeeded, got %s (%s)", actOut.Outcome.Status, actOut.Outcome.ErrorMessage)
	}
	if actOut.Outcome.Details["source_observation_id"] != obsID {
		t.Fatalf("act should preserve source observation id")
	}
	if actOut.Outcome.Details["observation_id"] == "" || actOut.Outcome.Details["observation_id"] == obsID {
		t.Fatalf("act should return a fresh post-action observation id")
	}
}

func TestComputerExecutorActExposesFreshPostActionAccessibilityTokens(t *testing.T) {
	ctx := context.Background()
	policy, _ := computer.NewPolicySnapshot(computer.DefaultPolicy())
	bind, _ := computer.NewBinding("run-1", "thread-1", "agent-1", "", policy)
	driver := fake.NewDriver()
	runtime := computer.NewRuntime(bind, driver)
	exec := NewComputerExecutor(runtime)

	sess, err := runtime.OpenSession(ctx, computer.CaptureWindow, computer.TargetRef{
		PID: 42, WindowID: "84", AppName: "FakeApp", Generation: 1,
	})
	if err != nil {
		t.Fatalf("open session: %v", err)
	}
	axPath := filepath.Join(t.TempDir(), "accessibility.json")
	driver.OnObserve = func(_ computer.SessionID, scope computer.CaptureScope, target computer.TargetRef, opts computer.ObserveOptions) (computer.Observation, error) {
		obs, err := computer.NewObservation(sess, scope, target, computer.Freshness{
			CapturedAt: time.Now().UTC(), FreshUntil: time.Now().UTC().Add(time.Minute),
		})
		if err != nil {
			return computer.Observation{}, err
		}
		if opts.IncludeAccessibility {
			contents := fmt.Sprintf(`{"snapshot_id":%q,"elements":[{"element_token":%q,"name":"Next"}]}`, "snapshot-"+string(obs.ObservationID), "token-"+string(obs.ObservationID))
			if err := os.WriteFile(axPath, []byte(contents), 0o600); err != nil {
				return computer.Observation{}, err
			}
			obs.AccessibilityTree = &computer.MediaRef{StorageKey: axPath, MIMEType: "application/json", ByteSize: int64(len(contents))}
		}
		return obs, nil
	}

	observeOut := mustExecute(t, exec, "computer_observe", `{"scope":"window","include":["accessibility"]}`)
	sourceObservationID := observeOut.Outcome.Details["observation_id"].(string)
	actInput := fmt.Sprintf(`{"observation_id":%q,"action":"wait","parameters":{"ms":1},"target":{},"intent":"wait","expected_effect":"state remains actionable"}`, sourceObservationID)
	actOut := mustExecute(t, exec, "computer_act", actInput)
	if actOut.Outcome.Status != tool.OutcomeSucceeded {
		t.Fatalf("expected act succeeded, got %+v", actOut.Outcome)
	}
	post, ok := actOut.Outcome.Details["post_action_observation"].(map[string]any)
	if !ok {
		t.Fatalf("post-action observation missing from model-visible details: %+v", actOut.Outcome.Details)
	}
	if post["observation_id"] == "" || post["observation_id"] == sourceObservationID {
		t.Fatalf("post-action observation must replace source observation: %+v", post)
	}
	accessibility, ok := post["accessibility"].(map[string]any)
	if !ok {
		t.Fatalf("post-action accessibility missing: %+v", post)
	}
	elements := accessibility["elements"].([]any)
	if token := elements[0].(map[string]any)["element_token"].(string); !strings.Contains(token, post["observation_id"].(string)) {
		t.Fatalf("fresh token %q is not bound to replacement observation %q", token, post["observation_id"])
	}
	if !strings.Contains(actOut.Content, "post_action_observation") || !strings.Contains(actOut.Content, "element_token") {
		t.Fatalf("compatibility content omitted fresh accessibility: %s", actOut.Content)
	}
}

func TestShouldCaptureAfterActionDoesNotSupersedeTokensForPreInputFailure(t *testing.T) {
	for _, result := range []computer.ActionResult{
		{Status: computer.ActionFailed, ErrorCode: "stale_element_token"},
		{Status: computer.ActionFailed, ErrorCode: "invalid_action_parameters"},
		{Status: computer.ActionFailed, ErrorCategory: "validation"},
	} {
		if shouldCaptureAfterAction(result) {
			t.Fatalf("pre-input failure should preserve the current accessibility snapshot: %+v", result)
		}
	}
	if !shouldCaptureAfterAction(computer.ActionResult{Status: computer.ActionSucceeded}) {
		t.Fatal("successful action must still capture a fresh post-action observation")
	}
}

func TestComputerExecutorActRejectsIncompleteCoordinatesBeforePrepare(t *testing.T) {
	ctx := context.Background()
	policy, _ := computer.NewPolicySnapshot(computer.DefaultPolicy())
	bind, _ := computer.NewBinding("run-1", "thread-1", "agent-1", "", policy)
	driver := fake.NewDriver()
	runtime := computer.NewRuntime(bind, driver)
	exec := NewComputerExecutor(runtime)

	if _, err := runtime.OpenSession(ctx, computer.CaptureWindow, computer.TargetRef{AppName: "FakeApp"}); err != nil {
		t.Fatalf("open session: %v", err)
	}
	observeOut := mustExecute(t, exec, "computer_observe", `{"scope":"window"}`)
	obsID := observeOut.Outcome.Details["observation_id"].(string)
	prepareCallsBefore := countDriverCalls(driver.Calls, "Prepare")

	actInput := fmt.Sprintf(`{"observation_id":%q,"action":"click","target":{"x":0},"intent":"click","expected_effect":"clicked"}`, obsID)
	actOut := mustExecute(t, exec, "computer_act", actInput)
	if !actOut.IsError || actOut.Outcome.ErrorCode != tool.ErrorCodeValidationError {
		t.Fatalf("expected validation error, got %+v", actOut.Outcome)
	}
	if actOut.Outcome.TransportStatus != tool.TransportNotStarted || actOut.Outcome.Operation == nil ||
		actOut.Outcome.Operation.SideEffectState != tool.OutcomeSideEffectNotStarted {
		t.Fatalf("incomplete coordinates must be a confirmed pre-input failure: %+v", actOut.Outcome)
	}
	if !strings.Contains(actOut.Outcome.ErrorMessage, "parameters.y is required") {
		t.Fatalf("expected missing-y diagnostic, got %q", actOut.Outcome.ErrorMessage)
	}
	if got := countDriverCalls(driver.Calls, "Prepare"); got != prepareCallsBefore {
		t.Fatalf("invalid action reached driver Prepare: before=%d after=%d", prepareCallsBefore, got)
	}
}

func TestComputerExecutorActAcceptsCompleteZeroCoordinatePair(t *testing.T) {
	ctx := context.Background()
	policy, _ := computer.NewPolicySnapshot(computer.DefaultPolicy())
	bind, _ := computer.NewBinding("run-1", "thread-1", "agent-1", "", policy)
	driver := fake.NewDriver()
	runtime := computer.NewRuntime(bind, driver)
	exec := NewComputerExecutor(runtime)

	if _, err := runtime.OpenSession(ctx, computer.CaptureWindow, computer.TargetRef{AppName: "FakeApp"}); err != nil {
		t.Fatalf("open session: %v", err)
	}
	observeOut := mustExecute(t, exec, "computer_observe", `{"scope":"window"}`)
	obsID := observeOut.Outcome.Details["observation_id"].(string)
	actInput := fmt.Sprintf(`{"observation_id":%q,"action":"click","target":{"x":0,"y":0},"intent":"click safe corner","expected_effect":"clicked"}`, obsID)
	actOut := mustExecute(t, exec, "computer_act", actInput)
	if actOut.Outcome.Status != tool.OutcomeSucceeded {
		t.Fatalf("complete zero coordinate pair was rejected: %+v", actOut.Outcome)
	}
}

func TestComputerExecutorActRejectsStaleObservation(t *testing.T) {
	ctx := context.Background()
	policy, _ := computer.NewPolicySnapshot(computer.DefaultPolicy())
	bind, _ := computer.NewBinding("run-1", "thread-1", "agent-1", "", policy)
	driver := fake.NewDriver()
	runtime := computer.NewRuntime(bind, driver)
	exec := NewComputerExecutor(runtime)

	if _, err := runtime.OpenSession(ctx, computer.CaptureWindow, computer.TargetRef{AppName: "FakeApp"}); err != nil {
		t.Fatalf("open session: %v", err)
	}

	actInput := `{"observation_id":"obs_does_not_exist","action":"wait","parameters":{"ms":10},"target":{},"intent":"wait","expected_effect":"time passes"}`
	actOut := mustExecute(t, exec, "computer_act", actInput)
	if actOut.Outcome.Status != tool.OutcomeFailed {
		t.Fatalf("expected act failed, got %s", actOut.Outcome.Status)
	}
	if actOut.Outcome.ErrorCode != "computer_stale_observation" {
		t.Fatalf("expected computer_stale_observation, got %s", actOut.Outcome.ErrorCode)
	}
}

func TestComputerExecutorVerifyReturnsObservationID(t *testing.T) {
	ctx := context.Background()
	policy, _ := computer.NewPolicySnapshot(computer.DefaultPolicy())
	bind, _ := computer.NewBinding("run-1", "thread-1", "agent-1", "", policy)
	driver := fake.NewDriver()
	runtime := computer.NewRuntime(bind, driver)
	exec := NewComputerExecutor(runtime)

	if _, err := runtime.OpenSession(ctx, computer.CaptureWindow, computer.TargetRef{AppName: "FakeApp"}); err != nil {
		t.Fatalf("open session: %v", err)
	}

	observeOut := mustExecute(t, exec, "computer_observe", `{"scope":"window"}`)
	obsID := observeOut.Outcome.Details["observation_id"].(string)
	actInput := fmt.Sprintf(`{"observation_id":%q,"action":"wait","parameters":{"ms":1},"target":{},"intent":"wait","expected_effect":"window remains available"}`, obsID)
	actOut := mustExecute(t, exec, "computer_act", actInput)
	actionID := actOut.Outcome.Details["action_id"].(string)
	verifyInput := fmt.Sprintf(`{"action_id":%q,"expected_effect":"window still open","assertions":[{"type":"text_contains","target":"title","expected":"Untitled","actual":"Untitled","matched":true}]}`, actionID)
	verifyOut := mustExecute(t, exec, "computer_verify", verifyInput)
	if verifyOut.Outcome.Status != tool.OutcomeSucceeded {
		t.Fatalf("expected verify succeeded, got %s (%s)", verifyOut.Outcome.Status, verifyOut.Outcome.ErrorMessage)
	}
	if verifyOut.Outcome.Details["observation_id"] == "" {
		t.Fatal("expected verify output to include observation_id")
	}
	if verifyOut.Outcome.Details["verification_status"] != string(computer.VerificationInconclusive) {
		t.Fatalf("model-supplied actual must not verify an assertion: %+v", verifyOut.Outcome.Details)
	}
	assertions := verifyOut.Outcome.Details["assertions"].([]computer.Assertion)
	if assertions[0].Actual != "" || assertions[0].Matched {
		t.Fatalf("model-supplied verification fields were trusted: %+v", assertions[0])
	}
}

func TestComputerExecutorActRequiresReview(t *testing.T) {
	ctx := context.Background()
	policy, _ := computer.NewPolicySnapshot(computer.DefaultPolicy())
	bind, _ := computer.NewBinding("run-1", "thread-1", "agent-1", "", policy)
	driver := fake.NewDriver()
	runtime := computer.NewRuntime(bind, driver)
	exec := NewComputerExecutor(runtime)

	reviewIDs := make(chan computer.ReviewID, 1)
	runtime.SetRecordFact(func(_ context.Context, kind string, payload map[string]any) (string, error) {
		if kind == "computer_review_requested" {
			if id, ok := payload["review_id"].(string); ok {
				reviewIDs <- computer.ReviewID(id)
			}
		}
		return "", nil
	})

	if _, err := runtime.OpenSession(ctx, computer.CaptureWindow, computer.TargetRef{AppName: "FakeApp"}); err != nil {
		t.Fatalf("open session: %v", err)
	}
	observeOut := mustExecute(t, exec, "computer_observe", `{"scope":"window"}`)
	obsID := observeOut.Outcome.Details["observation_id"].(string)

	actInput := fmt.Sprintf(`{"observation_id":%q,"action":"click","parameters":{"label":"Send"},"target":{"x":100,"y":100},"intent":"send the email","expected_effect":"email sent"}`, obsID)

	var actOut ToolExecutionOutput
	var actErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		actOut, actErr = exec.Execute(context.Background(), profile.ExecutionIntent{ToolName: "computer_act", Lane: profile.LaneComputer}, profile.DefaultProfile(), &toolStub{name: "computer_act"}, actInput, nil)
	}()

	var reviewID computer.ReviewID
	select {
	case reviewID = <-reviewIDs:
	case <-time.After(2 * time.Second):
		t.Fatal("expected a pending review to be created")
	}
	if _, err := runtime.ResolveReview(ctx, reviewID, computer.ReviewApprovedOnce, "test-actor", "test-req"); err != nil {
		t.Fatalf("resolve review: %v", err)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("act execution did not complete after review resolution")
	}
	if actErr != nil {
		t.Fatalf("execute act: %v", actErr)
	}
	if actOut.Outcome.Status != tool.OutcomeSucceeded {
		t.Fatalf("expected succeeded after approved review, got %s", actOut.Outcome.Status)
	}
	if actOut.Outcome.Details["review_id"] == "" {
		t.Fatal("expected review_id in outcome details")
	}
}

func TestComputerExecutorActForbidden(t *testing.T) {
	ctx := context.Background()
	policy := computer.DefaultPolicy()
	policy.ForbiddenKinds = []computer.ActionKind{computer.ActionClick}
	policySnapshot, _ := computer.NewPolicySnapshot(policy)
	bind, _ := computer.NewBinding("run-1", "thread-1", "agent-1", "", policySnapshot)
	driver := fake.NewDriver()
	runtime := computer.NewRuntime(bind, driver)
	exec := NewComputerExecutor(runtime)

	if _, err := runtime.OpenSession(ctx, computer.CaptureWindow, computer.TargetRef{AppName: "FakeApp"}); err != nil {
		t.Fatalf("open session: %v", err)
	}
	observeOut := mustExecute(t, exec, "computer_observe", `{"scope":"window"}`)
	obsID := observeOut.Outcome.Details["observation_id"].(string)

	actInput := fmt.Sprintf(`{"observation_id":%q,"action":"click","target":{"x":100,"y":100},"intent":"click","expected_effect":"clicked"}`, obsID)
	actOut := mustExecute(t, exec, "computer_act", actInput)
	if actOut.Outcome.Status != tool.OutcomeFailed {
		t.Fatalf("expected failed, got %s", actOut.Outcome.Status)
	}
	if actOut.Outcome.ErrorCode != "computer_action_forbidden" {
		t.Fatalf("expected computer_action_forbidden, got %s", actOut.Outcome.ErrorCode)
	}
}

func TestComputerExecutorActHandoff(t *testing.T) {
	ctx := context.Background()
	policy, _ := computer.NewPolicySnapshot(computer.DefaultPolicy())
	bind, _ := computer.NewBinding("run-1", "thread-1", "agent-1", "", policy)
	driver := fake.NewDriver()
	runtime := computer.NewRuntime(bind, driver)
	exec := NewComputerExecutor(runtime)

	if _, err := runtime.OpenSession(ctx, computer.CaptureWindow, computer.TargetRef{AppName: "FakeApp"}); err != nil {
		t.Fatalf("open session: %v", err)
	}
	observeOut := mustExecute(t, exec, "computer_observe", `{"scope":"window"}`)
	obsID := observeOut.Outcome.Details["observation_id"].(string)

	actInput := fmt.Sprintf(`{"observation_id":%q,"action":"type_text","parameters":{"text":"password","secret":true},"target":{},"intent":"enter password","expected_effect":"password entered"}`, obsID)
	resultCh := make(chan ToolExecutionOutput, 1)
	go func() {
		resultCh <- mustExecute(t, exec, "computer_act", actInput)
	}()
	deadline := time.Now().Add(2 * time.Second)
	for {
		session, ok := runtime.Session()
		if ok && session.State == computer.SessionTakenOver {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("handoff did not enter taken_over")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err := runtime.Return(ctx); err != nil {
		t.Fatalf("return control: %v", err)
	}
	actOut := <-resultCh
	if actOut.Outcome.Status != tool.OutcomeSucceeded {
		t.Fatalf("expected succeeded after handoff return, got %s", actOut.Outcome.Status)
	}
	if actOut.Outcome.Details["handoff"] != "completed" {
		t.Fatalf("expected completed handoff, got %#v", actOut.Outcome.Details["handoff"])
	}
}

func mustExecute(t *testing.T, exec *ComputerExecutor, toolName, input string) ToolExecutionOutput {
	t.Helper()
	toolObj := &toolStub{name: toolName}
	intent := profile.ExecutionIntent{ToolName: toolName, Lane: profile.LaneComputer}
	out, err := exec.Execute(context.Background(), intent, profile.DefaultProfile(), toolObj, input, nil)
	if err != nil {
		t.Fatalf("execute %s: %v", toolName, err)
	}
	return out
}

func countDriverCalls(calls []fake.CallRecord, method string) int {
	count := 0
	for _, call := range calls {
		if call.Method == method {
			count++
		}
	}
	return count
}

type toolStub struct {
	name string
}

func (s *toolStub) Name() string                                              { return s.name }
func (s *toolStub) Description() string                                       { return "stub" }
func (s *toolStub) Parameters() []tool.ToolParameter                          { return nil }
func (s *toolStub) Definition() tool.ToolDefinition                           { return tool.ToolDefinition{} }
func (s *toolStub) Execute(ctx context.Context, input string) (string, error) { return "", nil }
