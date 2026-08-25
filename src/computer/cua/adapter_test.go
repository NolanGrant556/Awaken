package cua

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"awaken/src/computer"
)

// hostWorkspace returns the product CUA Host resource used by Desktop builds.
func hostWorkspace(t *testing.T) (scriptPath, workingDir string) {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate test file")
	}
	cuaDir := filepath.Dir(file)
	root := filepath.Join(cuaDir, "..", "..", "..")
	root, err := filepath.Abs(root)
	if err != nil {
		t.Fatalf("resolve project root: %v", err)
	}
	workingDir = filepath.Join(root, "apps", "desktop", "resources", "cua-driver")
	scriptPath = filepath.Join(workingDir, "host.mjs")
	return scriptPath, workingDir
}

func newTestHostClient(t *testing.T) HostClient {
	return newTestHostClientWithEnv(t)
}

func newTestHostClientWithEnv(t *testing.T, extraEnv ...string) HostClient {
	t.Helper()
	scriptPath, workingDir := hostWorkspace(t)
	if _, err := os.Stat(scriptPath); err != nil {
		t.Fatalf("host script not found at %s: %v", scriptPath, err)
	}
	client, err := NewHostClient(HostOptions{
		HostScriptPath: scriptPath,
		WorkingDir:     workingDir,
		ExtraEnv:       extraEnv,
		StartupTimeout: 30 * time.Second,
	})
	if err != nil {
		t.Fatalf("create host client: %v", err)
	}
	t.Cleanup(func() {
		_ = client.Close(context.Background())
	})
	return client
}

func TestHostClientHealth(t *testing.T) {
	client := newTestHostClient(t)
	ctx := context.Background()
	hr, err := client.Health(ctx)
	if err != nil {
		t.Fatalf("health failed: %v", err)
	}
	if !hr.Healthy {
		t.Fatalf("expected driver to be healthy: %+v", hr)
	}
	if hr.Version == "" {
		t.Fatal("expected driver version")
	}
	if hr.Version != "0.22.0" {
		t.Fatalf("expected pinned driver version 0.22.0, got %s", hr.Version)
	}
	if hr.ProtocolVersion != 2 {
		t.Fatalf("expected protocol version 2, got %d", hr.ProtocolVersion)
	}
}

func TestNormalizeScreenshotToCoordinateSpace(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 4, 4))
	source.Set(2, 2, color.RGBA{R: 255, A: 255})
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, source); err != nil {
		t.Fatalf("encode source: %v", err)
	}
	normalized, err := normalizeScreenshotToCoordinateSpace(encoded.Bytes(), 2, 2)
	if err != nil {
		t.Fatalf("normalize screenshot: %v", err)
	}
	result, _, err := image.Decode(bytes.NewReader(normalized))
	if err != nil {
		t.Fatalf("decode normalized screenshot: %v", err)
	}
	if got := result.Bounds().Size(); got.X != 2 || got.Y != 2 {
		t.Fatalf("normalized screenshot size = %v, want 2x2", got)
	}
	if r, _, _, _ := result.At(1, 1).RGBA(); r == 0 {
		t.Fatal("expected normalized pixel to sample the matching source point")
	}
}

func TestDriverDefaultObservationTTLAllowsSlowModelTurn(t *testing.T) {
	driver, err := NewDriver(DriverOptions{Host: newTestHostClient(t)})
	if err != nil {
		t.Fatalf("create driver: %v", err)
	}
	adapter, ok := driver.(*cuaDriverAdapter)
	if !ok {
		t.Fatalf("driver type = %T, want *cuaDriverAdapter", driver)
	}
	if adapter.ttl != 5*time.Minute {
		t.Fatalf("default observation TTL = %s, want 5m", adapter.ttl)
	}
}

func TestNormalizeActionDeliveryUsesBindingAndScope(t *testing.T) {
	base := computer.ActionSpec{Kind: computer.ActionClick, Target: computer.ElementRef{Locator: "s00000001:4"}, Parameters: map[string]any{}}
	window := computer.Observation{Scope: computer.CaptureWindow}
	desktop := computer.Observation{Scope: computer.CaptureDesktop}

	backgroundBinding := computer.Binding{AllowBackgroundWindowActions: true}
	normalized, err := normalizeActionDelivery(backgroundBinding, window, base)
	if err != nil {
		t.Fatalf("normalize background action: %v", err)
	}
	if got := normalized.Parameters["delivery_mode"]; got != "background" {
		t.Fatalf("window delivery mode = %v, want background", got)
	}
	if _, exists := base.Parameters["delivery_mode"]; exists {
		t.Fatal("normalization mutated caller parameters")
	}

	foregroundBinding := computer.Binding{AllowBackgroundWindowActions: false}
	pixelAction := computer.ActionSpec{Kind: computer.ActionClick, Parameters: map[string]any{"x": 1, "y": 2}}
	normalized, err = normalizeActionDelivery(foregroundBinding, window, pixelAction)
	if err != nil {
		t.Fatalf("normalize foreground action: %v", err)
	}
	if got := normalized.Parameters["delivery_mode"]; got != "foreground" {
		t.Fatalf("window delivery mode = %v, want foreground", got)
	}

	explicitBackground := pixelAction
	explicitBackground.Parameters = map[string]any{"x": 1, "y": 2, "delivery_mode": "background"}
	if _, err := normalizeActionDelivery(foregroundBinding, window, explicitBackground); err == nil {
		t.Fatal("expected disabled background delivery to fail closed")
	}
	if _, err := normalizeActionDelivery(backgroundBinding, desktop, explicitBackground); err == nil {
		t.Fatal("expected desktop background delivery to fail closed")
	}
	if _, err := normalizeActionDelivery(backgroundBinding, window, explicitBackground); err == nil || !strings.Contains(err.Error(), "element_token") {
		t.Fatalf("expected background pixel click to require an element token, got %v", err)
	}
}

func TestDriverWindowObservationRequiresExactTarget(t *testing.T) {
	driver := newTestDriver(t, computer.CaptureWindow)
	ctx := context.Background()

	bind := newTestBinding(t, computer.CaptureWindow)
	sess, err := driver.OpenSession(ctx, bind, newTestSession(t, bind, computer.CaptureWindow), computer.PolicySnapshot{ID: bind.PolicySnapshotID}, bind.FenceToken)
	if err != nil {
		t.Fatalf("open session: %v", err)
	}
	if sess.HostSessionRef == "" {
		t.Fatal("expected host session ref")
	}

	_, err = driver.Observe(ctx, sess.SessionID, computer.CaptureWindow, sess.Target, computer.ObserveOptions{
		IncludeScreenshot:    true,
		IncludeAccessibility: true,
	})
	if err == nil || !strings.Contains(err.Error(), "window target requires a positive") {
		t.Fatalf("expected exact-window target error, got %v", err)
	}

	if err := driver.CloseSession(ctx, sess.SessionID, "test_done"); err != nil {
		t.Fatalf("close session: %v", err)
	}
}

func TestDriverQuiesceAndResumeUseHostBarrier(t *testing.T) {
	driver := newTestDriver(t, computer.CaptureDesktop)
	ctx := context.Background()

	bind := newTestBinding(t, computer.CaptureDesktop)
	sess, err := driver.OpenSession(ctx, bind, newTestSession(t, bind, computer.CaptureDesktop), computer.PolicySnapshot{ID: bind.PolicySnapshotID}, bind.FenceToken)
	if err != nil {
		t.Fatalf("open session: %v", err)
	}
	defer driver.CloseSession(ctx, sess.SessionID, "test_done")

	nextEpoch := sess.ControlEpoch + 1
	if err := driver.Quiesce(ctx, sess.SessionID, nextEpoch); err != nil {
		t.Fatalf("quiesce: %v", err)
	}
	if _, err := driver.Observe(ctx, sess.SessionID, computer.CaptureDesktop, sess.Target, computer.ObserveOptions{}); err == nil || !strings.Contains(err.Error(), "cannot capture") {
		t.Fatalf("expected capture to be blocked during takeover, got %v", err)
	}
	resumed, err := driver.Resume(ctx, sess.SessionID, computer.PolicySnapshot{ID: bind.PolicySnapshotID}, sess.FenceToken, nextEpoch+1)
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if resumed.ControlEpoch != nextEpoch+1 {
		t.Fatalf("resumed epoch = %s", resumed.ControlEpoch)
	}
}

func TestDriverOpenObserveCloseDesktop(t *testing.T) {
	var host HostClient
	if runtime.GOOS == "windows" {
		host = newTestHostClientWithEnv(t, "AWAKEN_CUA_WINDOWS_SCALE_FACTOR=1.25")
	} else {
		host = newTestHostClient(t)
	}
	driver, err := NewDriver(DriverOptions{
		Host:              host,
		ScreenshotWorkDir: t.TempDir(),
		ObservationTTL:    30 * time.Second,
	})
	if err != nil {
		t.Fatalf("create driver: %v", err)
	}
	ctx := context.Background()

	bind := newTestBinding(t, computer.CaptureDesktop)
	sess, err := driver.OpenSession(ctx, bind, newTestSession(t, bind, computer.CaptureDesktop), computer.PolicySnapshot{ID: bind.PolicySnapshotID}, bind.FenceToken)
	if err != nil {
		t.Fatalf("open session: %v", err)
	}

	obs, err := driver.Observe(ctx, sess.SessionID, computer.CaptureDesktop, sess.Target, computer.ObserveOptions{
		IncludeScreenshot: true,
	})
	if err != nil {
		// Desktop observation needs Screen Recording TCC. The protocol works,
		// but the actual capture may fail in CI/terminal. Do not fail the test
		// for the expected permission failure.
		t.Logf("desktop observe failed (expected if no TCC): %v", err)
	} else {
		t.Logf("desktop observation produced %d screenshot ref", countMediaRefs(&obs))
		if obs.CoordinateSpace == nil {
			t.Fatal("expected coordinate metadata")
		}
		t.Logf("desktop coordinate space: %+v", *obs.CoordinateSpace)
		if runtime.GOOS == "windows" {
			if obs.CoordinateSpace.Unit != "logical_pixel" ||
				obs.CoordinateSpace.ScaleX != 1.25 || obs.CoordinateSpace.ScaleY != 1.25 ||
				obs.CoordinateSpace.Width != obs.CoordinateSpace.SourceScreenshotWidth ||
				obs.CoordinateSpace.Height != obs.CoordinateSpace.SourceScreenshotHeight {
				t.Fatalf("Windows desktop coordinates must expose the logical-to-physical DPI transform: %+v", obs.CoordinateSpace)
			}
		} else if obs.CoordinateSpace.Unit != "screen_point" {
			t.Fatalf("expected screen-point coordinate metadata, got %+v", obs.CoordinateSpace)
		}
		if obs.Screenshot != nil &&
			(obs.Screenshot.Width != obs.CoordinateSpace.Width ||
				obs.Screenshot.Height != obs.CoordinateSpace.Height) {
			t.Fatalf("model screenshot and pointer canvas differ: screenshot=%+v coordinates=%+v", obs.Screenshot, obs.CoordinateSpace)
		}
	}

	if err := driver.CloseSession(ctx, sess.SessionID, "test_done"); err != nil {
		t.Fatalf("close session: %v", err)
	}
}

func TestHostClientWindowObservationUsesExactTarget(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows background-window conformance")
	}
	client := newTestHostClient(t)
	ctx := context.Background()
	const discoverySession = "awaken-window-discovery-test"
	if _, err := client.StartSession(ctx, discoverySession, computer.CaptureDesktop, 1, 1); err != nil {
		t.Fatalf("start discovery session: %v", err)
	}
	state, err := client.GetDesktopState(ctx, discoverySession)
	if err != nil || state.IsError {
		_ = client.EndSession(ctx, discoverySession)
		t.Skipf("desktop discovery unavailable: err=%v text=%s", err, state.Text)
	}
	if err := client.EndSession(ctx, discoverySession); err != nil {
		t.Fatalf("end discovery session: %v", err)
	}
	if len(state.AvailableWindows) == 0 {
		t.Skip("no on-screen windows available")
	}

	candidate := state.AvailableWindows[0]
	for _, window := range state.AvailableWindows {
		if strings.Contains(window.WindowTitle, "计算器") || strings.Contains(strings.ToLower(window.WindowTitle), "calculator") {
			candidate = window
			break
		}
	}
	target := computer.TargetRef{
		PID:             candidate.PID,
		AppName:         candidate.AppName,
		WindowID:        candidate.WindowID,
		WindowTitle:     candidate.WindowTitle,
		Generation:      1,
		CoordinateSpace: string(computer.CaptureWindow),
	}
	const windowSession = "awaken-exact-window-test"
	if _, err := client.StartSession(ctx, windowSession, computer.CaptureWindow, 1, 1); err != nil {
		t.Fatalf("start window session: %v", err)
	}
	defer client.EndSession(ctx, windowSession)
	windowState, err := client.GetWindowState(ctx, windowSession, target)
	if err != nil {
		t.Fatalf("get exact window state: %v", err)
	}
	if windowState.IsError {
		t.Fatalf("exact window state failed: %s", windowState.Text)
	}
	if windowState.Target.PID != target.PID || windowState.Target.WindowID != target.WindowID {
		t.Fatalf("window target changed: got %+v want %+v", windowState.Target, target)
	}
	if windowState.CoordinateSpace.Unit != "window_pixel" || windowState.CoordinateSpace.Width <= 0 || windowState.CoordinateSpace.Height <= 0 {
		t.Fatalf("invalid window coordinate space: %+v", windowState.CoordinateSpace)
	}
	if len(windowState.Images) == 0 {
		t.Fatal("window observation returned no screenshot")
	}
	t.Logf("exact window %q state: %s", candidate.WindowTitle, windowState.StructuredJSON)
}

func TestWindowsCalculatorBackgroundAcceptance(t *testing.T) {
	if runtime.GOOS != "windows" || os.Getenv("AWAKEN_CUA_REAL_CALCULATOR_ACCEPTANCE") != "1" {
		t.Skip("set AWAKEN_CUA_REAL_CALCULATOR_ACCEPTANCE=1 for real Windows GUI acceptance")
	}
	client := newTestHostClient(t)
	ctx := context.Background()
	const discoverySession = "awaken-background-discovery"
	if _, err := client.StartSession(ctx, discoverySession, computer.CaptureDesktop, 1, 1); err != nil {
		t.Fatalf("start discovery session: %v", err)
	}
	discovery, err := client.GetDesktopState(ctx, discoverySession)
	if closeErr := client.EndSession(ctx, discoverySession); err == nil && closeErr != nil {
		err = closeErr
	}
	if err != nil {
		t.Fatalf("discover calculator: %v", err)
	}
	var calculator computer.WindowRef
	for _, window := range discovery.AvailableWindows {
		title := strings.ToLower(window.WindowTitle)
		if strings.Contains(title, "计算器") || strings.Contains(title, "calculator") {
			calculator = window
			break
		}
	}
	if calculator.TargetRef == "" {
		t.Fatal("Windows Calculator must be open for background acceptance")
	}

	policy := computer.DefaultPolicy()
	policy.AllowBackgroundWindowActions = true
	policy.AllowedScopes = []computer.CaptureScope{computer.CaptureWindow}
	policy.AllowedActionKinds = []computer.ActionKind{computer.ActionClick}
	policy, err = computer.NewPolicySnapshot(policy)
	if err != nil {
		t.Fatalf("policy snapshot: %v", err)
	}
	bind, err := computer.NewBinding("calculator_background_acceptance", "thread_acceptance", "agent_acceptance", "host://local/desktop", policy)
	if err != nil {
		t.Fatalf("binding: %v", err)
	}
	target := computer.TargetRef{
		PID:             calculator.PID,
		AppName:         calculator.AppName,
		WindowID:        calculator.WindowID,
		WindowTitle:     calculator.WindowTitle,
		Generation:      1,
		CoordinateSpace: string(computer.CaptureWindow),
	}
	proposed, err := computer.NewSession(bind, computer.CaptureWindow, target)
	if err != nil {
		t.Fatalf("new session: %v", err)
	}
	workDir := t.TempDir()
	driver, err := NewDriver(DriverOptions{Host: client, ScreenshotWorkDir: workDir})
	if err != nil {
		t.Fatalf("create driver: %v", err)
	}
	sess, err := driver.OpenSession(ctx, bind, proposed, policy, bind.FenceToken)
	if err != nil {
		t.Fatalf("open exact calculator session: %v", err)
	}
	defer driver.CloseSession(ctx, sess.SessionID, "acceptance_done")

	steps := [][]string{
		{"清除", "Clear"},
		{"七", "Seven"},
		{"乘以", "Multiply"},
		{"八", "Eight"},
		{"等于", "Equals"},
	}
	var finalObservation computer.Observation
	for _, labels := range steps {
		obs, err := driver.Observe(ctx, sess.SessionID, computer.CaptureWindow, target, computer.ObserveOptions{IncludeScreenshot: true, IncludeAccessibility: true})
		if err != nil {
			t.Fatalf("observe before %q: %v", labels[0], err)
		}
		locator := calculatorElementToken(t, workDir, obs, labels...)
		spec := computer.ActionSpec{
			Kind:           computer.ActionClick,
			Target:         computer.ElementRef{Locator: locator},
			Parameters:     map[string]any{"delivery_mode": "background"},
			Intent:         "Windows Calculator background acceptance",
			ExpectedEffect: "calculator state changes without foreground or cursor movement",
			IdempotencyKey: "calculator-background-" + labels[0],
		}
		prepared, err := driver.Prepare(ctx, sess.SessionID, obs, spec)
		if err != nil {
			t.Fatalf("prepare %q: %v", labels[0], err)
		}
		cursorX, cursorY, cursorOK := observedCursorPosition()
		foreground, foregroundOK := observedForegroundWindow()
		result, err := driver.PerformChecked(ctx, sess.SessionID, prepared, sess.FenceToken, sess.ControlEpoch)
		if err != nil {
			t.Fatalf("perform %q: %v", labels[0], err)
		}
		if result.Status != computer.ActionSucceeded {
			t.Fatalf("perform %q status=%s code=%s message=%s cua=%s", labels[0], result.Status, result.ErrorCode, result.ErrorMessage, result.CUAStatus)
		}
		if result.CUADeliveryMode != "background" {
			t.Fatalf("perform %q delivery=%q route=%q effect=%q, want background", labels[0], result.CUADeliveryMode, result.CUARoute, result.CUAEffect)
		}
		if afterX, afterY, ok := observedCursorPosition(); cursorOK && ok && (afterX != cursorX || afterY != cursorY) {
			t.Fatalf("perform %q moved physical cursor from (%d,%d) to (%d,%d)", labels[0], cursorX, cursorY, afterX, afterY)
		}
		if after, ok := observedForegroundWindow(); foregroundOK && ok && after != foreground {
			t.Fatalf("perform %q changed foreground window from %#x to %#x", labels[0], foreground, after)
		}
		t.Logf("%s: delivery=%s route=%s effect=%s", labels[0], result.CUADeliveryMode, result.CUARoute, result.CUAEffect)
		finalObservation = obs
	}
	finalObservation, err = driver.Observe(ctx, sess.SessionID, computer.CaptureWindow, target, computer.ObserveOptions{IncludeScreenshot: true, IncludeAccessibility: true})
	if err != nil {
		t.Fatalf("final calculator observation: %v", err)
	}
	if !calculatorTreeContains(t, workDir, finalObservation, "56") {
		t.Fatal("final calculator observation does not show 56")
	}
}

type calculatorTree struct {
	Elements []struct {
		Label        string `json:"label"`
		Value        string `json:"value"`
		ElementToken string `json:"element_token"`
	} `json:"elements"`
}

func readCalculatorTree(t *testing.T, workDir string, obs computer.Observation) calculatorTree {
	t.Helper()
	if obs.AccessibilityTree == nil || obs.AccessibilityTree.SHA256 == "" {
		t.Fatal("calculator observation has no accessibility tree")
	}
	data, err := os.ReadFile(filepath.Join(workDir, obs.AccessibilityTree.SHA256+".ax.json"))
	if err != nil {
		t.Fatalf("read calculator tree: %v", err)
	}
	var tree calculatorTree
	if err := json.Unmarshal(data, &tree); err != nil {
		t.Fatalf("parse calculator tree: %v", err)
	}
	return tree
}

func calculatorElementToken(t *testing.T, workDir string, obs computer.Observation, labels ...string) string {
	t.Helper()
	tree := readCalculatorTree(t, workDir, obs)
	for _, element := range tree.Elements {
		for _, label := range labels {
			if strings.Contains(strings.ToLower(element.Label), strings.ToLower(label)) && element.ElementToken != "" {
				return element.ElementToken
			}
		}
	}
	t.Fatalf("calculator element not found: %v", labels)
	return ""
}

func calculatorTreeContains(t *testing.T, workDir string, obs computer.Observation, value string) bool {
	t.Helper()
	tree := readCalculatorTree(t, workDir, obs)
	for _, element := range tree.Elements {
		if strings.Contains(element.Label, value) || strings.Contains(element.Value, value) {
			return true
		}
	}
	return false
}

func TestDriverPreparePerformWait(t *testing.T) {
	driver := newTestDriver(t, computer.CaptureDesktop)
	ctx := context.Background()

	bind := newTestBinding(t, computer.CaptureDesktop)
	sess, err := driver.OpenSession(ctx, bind, newTestSession(t, bind, computer.CaptureDesktop), computer.PolicySnapshot{ID: bind.PolicySnapshotID}, bind.FenceToken)
	if err != nil {
		t.Fatalf("open session: %v", err)
	}
	defer driver.CloseSession(ctx, sess.SessionID, "test_done")

	obs, err := driver.Observe(ctx, sess.SessionID, computer.CaptureDesktop, sess.Target, computer.ObserveOptions{})
	if err != nil {
		t.Fatalf("observe failed: %v", err)
	}

	prepared, err := driver.Prepare(ctx, sess.SessionID, obs, computer.ActionSpec{
		Kind:           computer.ActionWait,
		Target:         computer.ElementRef{ObservationID: string(obs.ObservationID), SessionID: string(sess.SessionID)},
		Parameters:     map[string]any{"ms": 50},
		Intent:         "wait briefly",
		ExpectedEffect: "time passes",
	})
	if err != nil {
		t.Fatalf("prepare failed: %v", err)
	}
	if prepared.ActionDigest == "" {
		t.Fatal("expected action digest")
	}

	res, err := driver.PerformChecked(ctx, sess.SessionID, prepared, sess.FenceToken, sess.ControlEpoch)
	if err != nil {
		t.Fatalf("perform failed: %v", err)
	}
	if res.Status != computer.ActionSucceeded {
		t.Fatalf("expected wait to succeed, got %s", res.Status)
	}
}

func TestHostRejectsInvalidActionParametersBeforeInput(t *testing.T) {
	client := newTestHostClient(t)
	ctx := context.Background()
	const sessionID = "host-validation-test"
	if _, err := client.StartSession(ctx, sessionID, computer.CaptureDesktop, 1, 1); err != nil {
		t.Fatalf("start session: %v", err)
	}
	defer client.EndSession(context.Background(), sessionID)

	state, err := client.GetDesktopState(ctx, sessionID)
	if err != nil {
		t.Fatalf("get desktop state: %v", err)
	}
	invalid := computer.PreparedAction{
		ActionID:         "act_invalid_coordinates",
		ObservationID:    "obs_host_validation",
		SessionID:        computer.SessionID(sessionID),
		FenceToken:       1,
		ControlEpoch:     1,
		TargetGeneration: 1,
		Kind:             computer.ActionClick,
		ActionDigest:     "sha256:invalid-coordinate-test",
		ExpiresAt:        time.Now().UTC().Add(time.Minute),
	}
	if err := client.RegisterPreparedAction(ctx, invalid, state.ObservationToken); err != nil {
		t.Fatalf("register invalid action: %v", err)
	}
	result, err := client.PerformChecked(ctx, invalid, map[string]any{"x": 0})
	if err != nil {
		t.Fatalf("host returned transport error for deterministic validation failure: %v", err)
	}
	if !result.IsError || result.Status != string(computer.ActionFailed) || result.Degraded ||
		result.ErrorCode != "invalid_action_parameters" || result.ErrorCategory != "validation" {
		t.Fatalf("invalid parameters must be a confirmed pre-input failure: %+v", result)
	}
	if strings.Contains(result.Status, "unknown") {
		t.Fatalf("pre-input validation was marked unknown: %+v", result)
	}

	if runtime.GOOS == "windows" {
		unsupported := invalid
		unsupported.ActionID = "act_unsupported_windows_shell_hotkey"
		unsupported.Kind = computer.ActionHotkey
		unsupported.ActionDigest = "sha256:unsupported-windows-shell-hotkey"
		if err := client.RegisterPreparedAction(ctx, unsupported, state.ObservationToken); err != nil {
			t.Fatalf("register unsupported hotkey: %v", err)
		}
		result, err = client.PerformChecked(ctx, unsupported, map[string]any{"keys": []string{"win", "r"}})
		if err != nil {
			t.Fatalf("host returned transport error for unsupported Windows shell hotkey: %v", err)
		}
		if !result.IsError || result.Status != string(computer.ActionFailed) || result.Degraded ||
			result.ErrorCode != "invalid_action_parameters" ||
			!strings.Contains(result.Text, "Windows Shell global hotkeys are not reliably supported") {
			t.Fatalf("Windows shell hotkey must fail before input: %+v", result)
		}
	}

	valid := invalid
	valid.ActionID = "act_valid_after_coordinates"
	valid.Kind = computer.ActionWait
	valid.ActionDigest = "sha256:valid-after-coordinate-test"
	if err := client.RegisterPreparedAction(ctx, valid, state.ObservationToken); err != nil {
		t.Fatalf("register corrected action: %v", err)
	}
	result, err = client.PerformChecked(ctx, valid, map[string]any{"ms": 0})
	if err != nil {
		t.Fatalf("perform corrected action: %v", err)
	}
	if result.IsError || result.Status != string(computer.ActionSucceeded) {
		t.Fatalf("corrected action was not allowed after validation failure: %+v", result)
	}
}

func TestDriverCancelledPerformDoesNotBlockSessionClose(t *testing.T) {
	driver := newTestDriver(t, computer.CaptureDesktop)
	ctx := context.Background()

	bind := newTestBinding(t, computer.CaptureDesktop)
	sess, err := driver.OpenSession(ctx, bind, newTestSession(t, bind, computer.CaptureDesktop), computer.PolicySnapshot{ID: bind.PolicySnapshotID}, bind.FenceToken)
	if err != nil {
		t.Fatalf("open session: %v", err)
	}
	obs, err := driver.Observe(ctx, sess.SessionID, computer.CaptureDesktop, sess.Target, computer.ObserveOptions{})
	if err != nil {
		t.Fatalf("observe failed: %v", err)
	}
	prepared, err := driver.Prepare(ctx, sess.SessionID, obs, computer.ActionSpec{
		Kind:           computer.ActionWait,
		Target:         computer.ElementRef{ObservationID: string(obs.ObservationID), SessionID: string(sess.SessionID)},
		Parameters:     map[string]any{"ms": 10_000},
		Intent:         "wait until stopped",
		ExpectedEffect: "time passes",
	})
	if err != nil {
		t.Fatalf("prepare failed: %v", err)
	}

	performCtx, cancelPerform := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancelPerform()
	result, err := driver.PerformChecked(performCtx, sess.SessionID, prepared, sess.FenceToken, sess.ControlEpoch)
	if err != nil {
		t.Fatalf("perform returned unexpected adapter error: %v", err)
	}
	if result.Status != computer.ActionUnknownOutcome || !strings.Contains(result.ErrorMessage, context.DeadlineExceeded.Error()) {
		t.Fatalf("perform result = %+v, want deadline-caused unknown outcome", result)
	}

	closeCtx, cancelClose := context.WithTimeout(ctx, time.Second)
	defer cancelClose()
	if err := driver.CloseSession(closeCtx, sess.SessionID, "user_stopped"); err != nil {
		t.Fatalf("close remained blocked behind cancelled perform: %v", err)
	}
}

func TestDriverPerformMoveCursor(t *testing.T) {
	// move_cursor is a desktop-scope action in the CUA SDK, so the session must
	// use the desktop scope. This requires Screen Recording TCC for the terminal.
	host := newTestHostClient(t)
	if runtime.GOOS == "windows" {
		host = newTestHostClientWithEnv(t, "AWAKEN_CUA_WINDOWS_SCALE_FACTOR=1.25")
	}
	driver, err := NewDriver(DriverOptions{
		Host:              host,
		ScreenshotWorkDir: t.TempDir(),
		ObservationTTL:    30 * time.Second,
	})
	if err != nil {
		t.Fatalf("create driver: %v", err)
	}
	ctx := context.Background()

	bind := newTestBinding(t, computer.CaptureDesktop)
	bind.AllowedActionKinds = []computer.ActionKind{computer.ActionClick, computer.ActionMoveCursor}
	sess, err := driver.OpenSession(ctx, bind, newTestSession(t, bind, computer.CaptureDesktop), computer.PolicySnapshot{ID: bind.PolicySnapshotID}, bind.FenceToken)
	if err != nil {
		t.Fatalf("open session: %v", err)
	}
	defer driver.CloseSession(ctx, sess.SessionID, "test_done")

	obs, err := driver.Observe(ctx, sess.SessionID, computer.CaptureDesktop, sess.Target, computer.ObserveOptions{})
	if err != nil {
		t.Fatalf("observe failed: %v", err)
	}

	prepared, err := driver.Prepare(ctx, sess.SessionID, obs, computer.ActionSpec{
		Kind:           computer.ActionMoveCursor,
		Target:         computer.ElementRef{ObservationID: string(obs.ObservationID), SessionID: string(sess.SessionID)},
		Parameters:     map[string]any{"x": 100, "y": 100},
		Intent:         "move cursor to safe coordinates",
		ExpectedEffect: "cursor moves",
	})
	if err != nil {
		t.Fatalf("prepare failed: %v", err)
	}

	res, err := driver.PerformChecked(ctx, sess.SessionID, prepared, sess.FenceToken, sess.ControlEpoch)
	if err != nil {
		t.Fatalf("perform failed: %v", err)
	}
	if res.Status != computer.ActionSucceeded {
		t.Fatalf("expected move_cursor to succeed, got %s", res.Status)
	}
	if cursorX, cursorY, ok := observedCursorPosition(); ok {
		wantX := int(100 * obs.CoordinateSpace.ScaleX)
		wantY := int(100 * obs.CoordinateSpace.ScaleY)
		if cursorX != wantX || cursorY != wantY {
			t.Fatalf("cursor landed at physical (%d,%d), want (%d,%d) from logical (100,100) and scale %.2fx%.2f", cursorX, cursorY, wantX, wantY, obs.CoordinateSpace.ScaleX, obs.CoordinateSpace.ScaleY)
		}
	}
}

func TestDriverObserveActVerifyLoop(t *testing.T) {
	// End-to-end desktop observe -> act(move_cursor) -> verify loop against the
	// real CUA Node Host. Requires Screen Recording TCC.
	driver := newTestDriver(t, computer.CaptureDesktop)
	bind := newTestBinding(t, computer.CaptureDesktop)
	bind.AllowedActionKinds = []computer.ActionKind{computer.ActionWait, computer.ActionMoveCursor}
	rt := computer.NewRuntime(bind, driver)
	ctx := context.Background()

	sess, err := rt.OpenSession(ctx, computer.CaptureDesktop, computer.TargetRef{PID: int32(os.Getpid()), AppName: "awaken-test", Generation: 1, CoordinateSpace: string(computer.CaptureDesktop)})
	if err != nil {
		t.Fatalf("open session: %v", err)
	}
	defer rt.Close(ctx, "test_done")

	obs, err := rt.Observe(ctx, computer.ObserveOptions{IncludeScreenshot: true})
	if err != nil {
		t.Fatalf("observe failed: %v", err)
	}
	if obs.Screenshot == nil {
		t.Fatal("expected desktop screenshot")
	}

	prepared, err := rt.Prepare(ctx, obs, computer.ActionSpec{
		Kind:           computer.ActionMoveCursor,
		Target:         computer.ElementRef{ObservationID: string(obs.ObservationID), SessionID: string(sess.SessionID)},
		Parameters:     map[string]any{"x": 200, "y": 200},
		Intent:         "move cursor",
		ExpectedEffect: "cursor moves",
	})
	if err != nil {
		t.Fatalf("prepare failed: %v", err)
	}

	res, err := rt.Perform(ctx, prepared, nil)
	if err != nil {
		t.Fatalf("perform failed: %v", err)
	}
	if res.Status != computer.ActionSucceeded {
		t.Fatalf("expected move_cursor to succeed, got %s", res.Status)
	}

	afterObs, err := rt.Observe(ctx, computer.ObserveOptions{})
	if err != nil {
		t.Fatalf("after observe failed: %v", err)
	}
	verifyRes, err := rt.Verify(ctx, prepared.ActionID, afterObs, nil)
	if err != nil {
		t.Fatalf("verify failed: %v", err)
	}
	if verifyRes.Status != computer.VerificationInconclusive {
		t.Fatalf("expected inconclusive verify status, got %s", verifyRes.Status)
	}
}

func newTestDriver(t *testing.T, scope computer.CaptureScope) computer.Driver {
	t.Helper()
	host := newTestHostClient(t)
	workDir := t.TempDir()
	driver, err := NewDriver(DriverOptions{
		Host:              host,
		ScreenshotWorkDir: workDir,
		ObservationTTL:    30 * time.Second,
	})
	if err != nil {
		t.Fatalf("create driver: %v", err)
	}
	return driver
}

func newTestBinding(t *testing.T, scope computer.CaptureScope) computer.Binding {
	t.Helper()
	policy := computer.DefaultPolicy()
	policy.AllowedScopes = []computer.CaptureScope{scope}
	policy.AllowedActionKinds = []computer.ActionKind{computer.ActionWait}
	policy, err := computer.NewPolicySnapshot(policy)
	if err != nil {
		t.Fatalf("policy snapshot: %v", err)
	}
	bind, err := computer.NewBinding("run_test", "thread_test", "agent_test", "host://local/desktop", policy)
	if err != nil {
		t.Fatalf("binding: %v", err)
	}
	return bind
}

func newTestSession(t *testing.T, bind computer.Binding, scope computer.CaptureScope) computer.Session {
	t.Helper()
	sess, err := computer.NewSession(bind, scope, computer.TargetRef{
		PID:             int32(os.Getpid()),
		AppName:         "awaken-test",
		Generation:      1,
		CoordinateSpace: string(scope),
	})
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	return sess
}

func countMediaRefs(obs *computer.Observation) int {
	n := 0
	if obs.Screenshot != nil {
		n++
	}
	if obs.AccessibilityTree != nil {
		n++
	}
	return n
}
