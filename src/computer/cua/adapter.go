package cua

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"time"

	"awaken/src/computer"
)

const defaultObservationTTL = 5 * time.Minute

// DriverOptions configures the CUA driver adapter.
type DriverOptions struct {
	// Host is the pre-connected HostClient. Most callers use NewHostClient and
	// pass the result here.
	Host HostClient

	// MediaVault isolates and bounds the lifetime of screenshots and
	// accessibility trees. The historical name is retained for API stability.
	MediaVault computer.MediaVault

	// ScreenshotWorkDir is a writable directory for temporary screenshot files.
	// If empty, os.TempDir() is used.
	ScreenshotWorkDir string

	// ObservationTTL is the freshness window for observations produced by
	// Observe. It defaults to five minutes. Real model turns can exceed two
	// minutes before emitting the checked action, especially with screenshots
	// and large tool catalogs, so a shorter default rejects otherwise current
	// observations before the model can use them.
	ObservationTTL time.Duration
}

// NewDriver returns a computer.Driver backed by the CUA SDK host.
func NewDriver(opts DriverOptions) (computer.Driver, error) {
	if opts.Host == nil {
		return nil, fmt.Errorf("HostClient is required")
	}
	workDir := opts.ScreenshotWorkDir
	if workDir == "" {
		workDir = os.TempDir()
	}
	if err := os.MkdirAll(workDir, 0700); err != nil {
		return nil, fmt.Errorf("screenshot work dir: %w", err)
	}
	ttl := opts.ObservationTTL
	if ttl <= 0 {
		ttl = defaultObservationTTL
	}
	return &cuaDriverAdapter{
		host:     opts.Host,
		vault:    opts.MediaVault,
		workDir:  workDir,
		ttl:      ttl,
		sessions: make(map[computer.SessionID]*cuaSession),
	}, nil
}

type cuaSession struct {
	bind              computer.Binding
	session           computer.Session
	policy            computer.PolicySnapshot
	observationTokens map[computer.ObservationID]string
}

type cuaDriverAdapter struct {
	host    HostClient
	vault   computer.MediaVault
	workDir string
	ttl     time.Duration

	// sessions caches the opened sessions for digest/epoch validation. The host
	// is the authoritative owner of the session handle; this map only stores the
	// Awaken view.
	sessions map[computer.SessionID]*cuaSession
}

func (d *cuaDriverAdapter) Health(ctx context.Context) (computer.HealthResult, error) {
	health, err := d.host.Health(ctx)
	if err != nil {
		return health, err
	}
	if health.ProtocolVersion != hostProtocolVersion {
		health.Healthy = false
		health.Message = fmt.Sprintf("unsupported CUA Host protocol %d; want %d", health.ProtocolVersion, hostProtocolVersion)
	}
	return health, nil
}

func (d *cuaDriverAdapter) OpenSession(ctx context.Context, bind computer.Binding, proposed computer.Session, policy computer.PolicySnapshot, fence computer.FenceToken) (computer.Session, error) {
	if bind.BindingID != proposed.BindingID {
		return computer.Session{}, fmt.Errorf("binding id mismatch")
	}
	if proposed.SessionID == "" {
		return computer.Session{}, fmt.Errorf("proposed session id is required")
	}
	// The proposed session carries the authoritative Awaken IDs; the host uses
	// the Awaken session ID as its stable SDK session identifier.
	res, err := d.host.StartSession(ctx, string(proposed.SessionID), proposed.CaptureScope, fence, proposed.ControlEpoch)
	if err != nil {
		return computer.Session{}, fmt.Errorf("start cua session: %w", err)
	}
	sess := proposed.Clone()
	sess.HostSessionRef = res.HostSessionRef
	sess.PolicySnapshotID = policy.ID
	sess.FenceToken = fence
	if err := sess.MarkReady(); err != nil {
		_ = d.host.EndSession(ctx, string(sess.SessionID))
		return computer.Session{}, fmt.Errorf("mark session ready: %w", err)
	}
	d.sessions[sess.SessionID] = &cuaSession{
		bind:              bind,
		session:           sess,
		policy:            policy,
		observationTokens: make(map[computer.ObservationID]string),
	}
	return sess, nil
}

func (d *cuaDriverAdapter) Observe(ctx context.Context, sessionID computer.SessionID, scope computer.CaptureScope, target computer.TargetRef, opts computer.ObserveOptions) (computer.Observation, error) {
	rec, ok := d.sessions[sessionID]
	if !ok {
		return computer.Observation{}, fmt.Errorf("session %s not found", sessionID)
	}

	var state HostDesktopState
	var err error
	if scope == computer.CaptureWindow {
		state, err = d.host.GetWindowState(ctx, string(sessionID), target)
	} else {
		state, err = d.host.GetDesktopState(ctx, string(sessionID))
	}
	if err != nil {
		return computer.Observation{}, fmt.Errorf("get %s state: %w", scope, err)
	}
	if state.IsError {
		message := state.Text
		if message == "" {
			message = "CUA host returned an unsuccessful observation"
		}
		return computer.Observation{}, fmt.Errorf("get %s state: %s", scope, message)
	}
	if scope == computer.CaptureWindow {
		if state.Target.PID <= 0 || state.Target.WindowID == "" {
			return computer.Observation{}, fmt.Errorf("get window state: host omitted exact window identity")
		}
		state.Target.Generation = target.Generation
		state.Target.AppName = target.AppName
		state.Target.WindowTitle = target.WindowTitle
		target = state.Target
	}

	now := time.Now().UTC()
	obs, err := computer.NewObservation(rec.session, scope, target, computer.Freshness{
		CapturedAt: now,
		FreshUntil: now.Add(d.ttl),
	})
	if err != nil {
		return computer.Observation{}, err
	}
	obs.AvailableWindows = append([]computer.WindowRef(nil), state.AvailableWindows...)
	if state.CoordinateSpace.Width > 0 && state.CoordinateSpace.Height > 0 {
		obs.CoordinateSpace = &computer.CoordinateSpace{
			Unit:                   state.CoordinateSpace.Unit,
			Width:                  state.CoordinateSpace.Width,
			Height:                 state.CoordinateSpace.Height,
			SourceScreenshotWidth:  state.CoordinateSpace.SourceScreenshotWidth,
			SourceScreenshotHeight: state.CoordinateSpace.SourceScreenshotHeight,
			ScaleX:                 state.CoordinateSpace.ScaleX,
			ScaleY:                 state.CoordinateSpace.ScaleY,
		}
	}

	if opts.IncludeScreenshot && len(state.Images) > 0 {
		ref, err := d.saveHostImage(sessionID, state.Images[0], obs.CoordinateSpace)
		if err != nil {
			return computer.Observation{}, fmt.Errorf("save screenshot: %w", err)
		}
		obs.Screenshot = &ref
	}

	if opts.IncludeAccessibility {
		ref, err := d.saveAccessibilityTree(sessionID, state.StructuredJSON)
		if err != nil {
			return computer.Observation{}, fmt.Errorf("save accessibility tree: %w", err)
		}
		obs.AccessibilityTree = &ref
	}

	rec.session.LastObservation = &obs.ObservationID
	rec.observationTokens[obs.ObservationID] = state.ObservationToken
	return obs, nil
}

func normalizeActionDelivery(bind computer.Binding, obs computer.Observation, spec computer.ActionSpec) (computer.ActionSpec, error) {
	spec.Parameters = computer.CloneActionParameters(spec.Parameters)
	deliveryMode, hasDeliveryMode := spec.Parameters["delivery_mode"]
	if obs.Scope == computer.CaptureWindow && spec.Kind != computer.ActionWait {
		if !hasDeliveryMode {
			if bind.AllowBackgroundWindowActions {
				spec.Parameters["delivery_mode"] = "background"
			} else {
				spec.Parameters["delivery_mode"] = "foreground"
			}
		} else if deliveryMode == "background" && !bind.AllowBackgroundWindowActions {
			return computer.ActionSpec{}, fmt.Errorf("background window actions are disabled by the run binding")
		}
	} else if deliveryMode == "background" {
		return computer.ActionSpec{}, fmt.Errorf("background delivery requires a window-scoped observation")
	}
	if mode, _ := spec.Parameters["delivery_mode"].(string); mode == "background" &&
		(spec.Kind == computer.ActionClick || spec.Kind == computer.ActionDoubleClick) &&
		strings.TrimSpace(spec.Target.Locator) == "" {
		return computer.ActionSpec{}, fmt.Errorf("background window %s requires target.locator with an exact element_token from the same accessibility observation; screenshot coordinates use an unverified synthetic-event route", spec.Kind)
	}
	return spec, nil
}

func (d *cuaDriverAdapter) Prepare(ctx context.Context, sessionID computer.SessionID, obs computer.Observation, spec computer.ActionSpec) (computer.PreparedAction, error) {
	rec, ok := d.sessions[sessionID]
	if !ok {
		return computer.PreparedAction{}, fmt.Errorf("session %s not found", sessionID)
	}
	var err error
	spec, err = normalizeActionDelivery(rec.bind, obs, spec)
	if err != nil {
		return computer.PreparedAction{}, err
	}
	if err := computer.ValidateActionSpec(spec); err != nil {
		return computer.PreparedAction{}, fmt.Errorf("invalid action parameters: %w", err)
	}
	risk := computer.RiskForAction(rec.bind, spec.Kind, spec.Target, spec.Intent, spec.Parameters)
	if policyRisk := rec.policy.RiskForKind(spec.Kind); policyRisk == computer.RiskForbidden || policyRisk == computer.RiskReview {
		risk = policyRisk
	}
	expires := time.Now().UTC().Add(5 * time.Minute)
	prepared, err := computer.PrepareAction(rec.session, obs, spec, risk, expires)
	if err != nil {
		return computer.PreparedAction{}, err
	}
	token, ok := rec.observationTokens[obs.ObservationID]
	if !ok || token == "" {
		return computer.PreparedAction{}, fmt.Errorf("host observation token missing")
	}
	if err := d.host.RegisterPreparedAction(ctx, prepared, token); err != nil {
		return computer.PreparedAction{}, err
	}
	return prepared, nil
}

func (d *cuaDriverAdapter) PerformChecked(ctx context.Context, sessionID computer.SessionID, action computer.PreparedAction, fence computer.FenceToken, epoch computer.ControlEpoch) (computer.ActionResult, error) {
	rec, ok := d.sessions[sessionID]
	if !ok {
		return computer.ActionResult{}, fmt.Errorf("session %s not found", sessionID)
	}
	if action.SessionID != sessionID {
		return computer.ActionResult{}, fmt.Errorf("action session mismatch")
	}
	if fence != rec.session.FenceToken {
		return computer.ActionResult{}, fmt.Errorf("stale fence")
	}
	if epoch != rec.session.ControlEpoch {
		return computer.ActionResult{}, fmt.Errorf("stale epoch")
	}

	parameters := computer.CloneActionParameters(action.Parameters)
	if action.Target.Locator != "" {
		if _, ok := parameters["element_token"]; !ok {
			parameters["element_token"] = action.Target.Locator
		}
	}
	if point := action.Target.PixelFallback; point != nil {
		if _, ok := parameters["x"]; !ok {
			parameters["x"] = point.X
		}
		if _, ok := parameters["y"]; !ok {
			parameters["y"] = point.Y
		}
	}
	result, err := d.host.PerformChecked(ctx, action, parameters)
	if err != nil {
		return computer.ActionResult{
			ActionID:      action.ActionID,
			SessionID:     sessionID,
			Status:        computer.ActionUnknownOutcome,
			StartedAt:     time.Now().UTC(),
			FinishedAt:    time.Now().UTC(),
			ErrorCode:     "host_perform_unknown",
			ErrorCategory: "runtime",
			ErrorMessage:  err.Error(),
		}, nil
	}
	status := computer.ActionSucceeded
	if result.Status == string(computer.ActionUnknownOutcome) {
		status = computer.ActionUnknownOutcome
	} else if result.IsError {
		status = computer.ActionFailed
	}
	res := computer.NewActionResult(action.ActionID, sessionID, status)
	res.CUAStatus = result.Text
	if result.StructuredJSON != "" {
		var details struct {
			Effect   string `json:"effect"`
			Route    string `json:"route"`
			Delivery *struct {
				Mode string `json:"mode"`
			} `json:"delivery"`
		}
		if json.Unmarshal([]byte(result.StructuredJSON), &details) == nil {
			res.CUAEffect = details.Effect
			res.CUARoute = details.Route
			if details.Delivery != nil {
				res.CUADeliveryMode = details.Delivery.Mode
			}
		}
	}
	if result.IsError && status != computer.ActionUnknownOutcome {
		res.ErrorCode = result.ErrorCode
		if res.ErrorCode == "" {
			res.ErrorCode = "cua_tool_error"
		}
		res.ErrorCategory = result.ErrorCategory
		if res.ErrorCategory == "" {
			res.ErrorCategory = "runtime"
		}
		res.ErrorMessage = result.Text
	}
	return res, nil
}

func (d *cuaDriverAdapter) Quiesce(ctx context.Context, sessionID computer.SessionID, nextEpoch computer.ControlEpoch) error {
	rec, ok := d.sessions[sessionID]
	if !ok {
		return fmt.Errorf("session %s not found", sessionID)
	}
	if nextEpoch <= rec.session.ControlEpoch {
		return fmt.Errorf("next epoch must be greater than current epoch")
	}
	if err := d.host.Quiesce(ctx, string(sessionID), nextEpoch); err != nil {
		return err
	}
	rec.session.ControlEpoch = nextEpoch
	rec.observationTokens = make(map[computer.ObservationID]string)
	return nil
}

func (d *cuaDriverAdapter) Resume(ctx context.Context, sessionID computer.SessionID, policy computer.PolicySnapshot, fence computer.FenceToken, epoch computer.ControlEpoch) (computer.Session, error) {
	rec, ok := d.sessions[sessionID]
	if !ok {
		return computer.Session{}, fmt.Errorf("session %s not found", sessionID)
	}
	if epoch <= rec.session.ControlEpoch {
		return computer.Session{}, fmt.Errorf("resume epoch must be greater than current epoch")
	}
	if err := d.host.Resume(ctx, string(sessionID), fence, epoch); err != nil {
		return computer.Session{}, err
	}
	rec.policy = policy
	rec.session.FenceToken = fence
	rec.session.ControlEpoch = epoch
	rec.observationTokens = make(map[computer.ObservationID]string)
	return rec.session, nil
}

func (d *cuaDriverAdapter) CloseSession(ctx context.Context, sessionID computer.SessionID, reason string) error {
	if _, ok := d.sessions[sessionID]; !ok {
		return nil
	}
	_ = d.host.EndSession(ctx, string(sessionID))
	delete(d.sessions, sessionID)
	return nil
}

func (d *cuaDriverAdapter) saveHostImage(sessionID computer.SessionID, img HostImage, space *computer.CoordinateSpace) (computer.MediaRef, error) {
	data, err := base64.StdEncoding.DecodeString(img.DataBase64)
	if err != nil {
		return computer.MediaRef{}, fmt.Errorf("decode screenshot: %w", err)
	}
	mimeType := img.MIMEType
	width, height := 0, 0
	if space != nil && space.Width > 0 && space.Height > 0 {
		data, err = normalizeScreenshotToCoordinateSpace(data, space.Width, space.Height)
		if err != nil {
			return computer.MediaRef{}, fmt.Errorf("normalize screenshot coordinates: %w", err)
		}
		mimeType = "image/png"
		width, height = space.Width, space.Height
	}

	if d.vault != nil {
		ref, err := d.vault.Store(context.Background(), sessionID, computer.MediaKindScreenshot, data)
		if err != nil {
			return computer.MediaRef{}, err
		}
		ref.MIMEType = mimeType
		ref.Width = width
		ref.Height = height
		return ref, nil
	}

	// Fallback for tests and PoC configurations without a session media store.
	sum := sha256sum(data)
	path := filepath.Join(d.workDir, sum+".png")
	if err := os.WriteFile(path, data, 0600); err != nil {
		return computer.MediaRef{}, fmt.Errorf("write screenshot: %w", err)
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return computer.MediaRef{}, fmt.Errorf("resolve screenshot path: %w", err)
	}
	return computer.MediaRef{
		StorageKey: absPath,
		MIMEType:   mimeType,
		Width:      width,
		Height:     height,
		ByteSize:   int64(len(data)),
		SHA256:     sum,
		Redacted:   false,
	}, nil
}

func normalizeScreenshotToCoordinateSpace(data []byte, width, height int) ([]byte, error) {
	if width <= 0 || height <= 0 {
		return data, nil
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	bounds := src.Bounds()
	if bounds.Dx() == width && bounds.Dy() == height {
		return data, nil
	}
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		srcY := bounds.Min.Y + y*bounds.Dy()/height
		for x := 0; x < width; x++ {
			srcX := bounds.Min.X + x*bounds.Dx()/width
			dst.Set(x, y, src.At(srcX, srcY))
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, dst); err != nil {
		return nil, err
	}
	return encoded.Bytes(), nil
}

func (d *cuaDriverAdapter) saveAccessibilityTree(sessionID computer.SessionID, jsonText string) (computer.MediaRef, error) {
	if jsonText == "" {
		jsonText = "{}"
	}
	data := []byte(jsonText)

	if d.vault != nil {
		ref, err := d.vault.Store(context.Background(), sessionID, computer.MediaKindAccessibilityTree, data)
		if err != nil {
			return computer.MediaRef{}, err
		}
		ref.MIMEType = "application/json"
		return ref, nil
	}

	// Fallback for tests and PoC configurations without a session media store.
	sum := sha256sum(data)
	key := "computer-poc://" + sum + ".ax.json"
	path := filepath.Join(d.workDir, sum+".ax.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		return computer.MediaRef{}, err
	}
	return computer.MediaRef{
		StorageKey: key,
		MIMEType:   "application/json",
		ByteSize:   int64(len(data)),
		SHA256:     sum,
		Redacted:   false,
	}, nil
}

var _ computer.Driver = (*cuaDriverAdapter)(nil)
