package cua

import (
	"bytes"
	"context"
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
	if hr.Version != "0.19.0" {
		t.Fatalf("expected pinned driver version 0.19.0, got %s", hr.Version)
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

func TestDriverWindowObservationReturnsStrictScopeError(t *testing.T) {
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
	if err == nil || !strings.Contains(err.Error(), "strict window session") {
		t.Fatalf("expected explicit strict-window observation error, got %v", err)
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
