// Package cua provides the Awaken computer.Driver adapter for the
// @trycua/cua-driver SDK. It spawns a Node.js sidecar that loads the SDK and
// communicates over a private stdin/stdout protocol. Awaken protocol v2 adds
// observation/action binding, fences, epochs, checked perform and quiesce on
// top of the SDK's raw input primitives.
package cua

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"awaken/src/computer"
)

const hostProtocolVersion = 2

// HostClient is the minimal JSON-RPC surface exposed by the Node CUA Host. It
// is intentionally smaller than the full SDK; the Awaken Driver contract is
// built on top of it in cuaDriverAdapter.
type HostClient interface {
	// Health reports driver readiness and macOS permission status.
	Health(ctx context.Context) (computer.HealthResult, error)

	// StartSession asks the host to create a CUA session with the Awaken session
	// ID as the stable SDK session identifier.
	StartSession(ctx context.Context, sessionID string, scope computer.CaptureScope, fence computer.FenceToken, epoch computer.ControlEpoch) (HostSessionResult, error)

	// GetDesktopState captures the current desktop/window state.
	GetDesktopState(ctx context.Context, sessionID string) (HostDesktopState, error)

	// GetSessionState returns the SDK session state without capturing media.
	GetSessionState(ctx context.Context, sessionID string) (HostSessionState, error)

	// RegisterPreparedAction binds an immutable Awaken action digest to the
	// latest Host observation before any input can be attempted.
	RegisterPreparedAction(ctx context.Context, action computer.PreparedAction, observationToken string) error

	// PerformChecked atomically revalidates the registered action, fence and
	// control epoch in the Host process before invoking CUA input.
	PerformChecked(ctx context.Context, action computer.PreparedAction, params map[string]any) (HostToolResult, error)

	// Quiesce drains all earlier input requests and changes the Host epoch.
	Quiesce(ctx context.Context, sessionID string, nextEpoch computer.ControlEpoch) error

	// Resume re-enables capture/input under a fresh control epoch and fence.
	Resume(ctx context.Context, sessionID string, fence computer.FenceToken, epoch computer.ControlEpoch) error

	// EndSession closes the SDK session.
	EndSession(ctx context.Context, sessionID string) error

	// Close shuts down the host subprocess.
	Close(ctx context.Context) error
}

// HostSessionResult carries the host response from StartSession.
type HostSessionResult struct {
	HostSessionRef string `json:"host_session_ref"`
	CaptureScope   string `json:"capture_scope"`
	Active         bool   `json:"active"`
	Revived        bool   `json:"revived"`
}

// HostDesktopState carries the host response from GetDesktopState.
type HostDesktopState struct {
	Text             string              `json:"text"`
	Images           []HostImage         `json:"images"`
	StructuredJSON   string              `json:"structured_json,omitempty"`
	RawJSON          string              `json:"raw_json,omitempty"`
	IsError          bool                `json:"is_error"`
	Degraded         bool                `json:"degraded"`
	ObservationToken string              `json:"observation_token"`
	CoordinateSpace  HostCoordinateSpace `json:"coordinate_space"`
}

// HostCoordinateSpace is the desktop geometry resolved by the trusted Host.
type HostCoordinateSpace struct {
	Unit                   string  `json:"unit"`
	Width                  int     `json:"width"`
	Height                 int     `json:"height"`
	SourceScreenshotWidth  int     `json:"source_screenshot_width"`
	SourceScreenshotHeight int     `json:"source_screenshot_height"`
	ScaleX                 float64 `json:"scale_x"`
	ScaleY                 float64 `json:"scale_y"`
}

// HostImage is a screenshot returned by the host.
type HostImage struct {
	MIMEType   string `json:"mime_type"`
	DataBase64 string `json:"data_base64"`
}

// HostToolResult is the host response from Perform.
type HostToolResult struct {
	Text           string      `json:"text"`
	Images         []HostImage `json:"images"`
	StructuredJSON string      `json:"structured_json,omitempty"`
	RawJSON        string      `json:"raw_json,omitempty"`
	IsError        bool        `json:"is_error"`
	Degraded       bool        `json:"degraded"`
	Status         string      `json:"status,omitempty"`
	ErrorCode      string      `json:"error_code,omitempty"`
	ErrorCategory  string      `json:"error_category,omitempty"`
}

// HostSessionState is the host response from GetSessionState.
type HostSessionState struct {
	Session         string `json:"session"`
	CaptureScope    string `json:"capture_scope"`
	EffectiveScope  string `json:"effective_scope"`
	DesktopUnlocked bool   `json:"desktop_unlocked"`
}

// stdioHostClient talks to a Node CUA Host over stdin/stdout.
type stdioHostClient struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	stderr io.ReadCloser

	mu       sync.Mutex
	writeMu  sync.Mutex
	pending  map[string]chan rpcResponse
	nextID   atomic.Uint64
	closed   bool
	closeErr error
}

// HostOptions configures the Node host subprocess.
type HostOptions struct {
	// NodePath is the path to the node binary. Empty uses "node" from PATH.
	NodePath string

	// HostScriptPath is the path to the host.mjs reference implementation.
	HostScriptPath string

	// WorkingDir is the directory from which to run the host. The host needs to
	// resolve @trycua/cua-driver, so this should be the directory containing
	// package.json and node_modules.
	WorkingDir string

	// ExtraEnv is appended to the host environment.
	ExtraEnv []string

	// StartupTimeout bounds how long Health may take during startup.
	StartupTimeout time.Duration
}

// NewHostClient spawns the Node CUA Host and returns a HostClient.
func NewHostClient(opts HostOptions) (HostClient, error) {
	node := opts.NodePath
	if node == "" {
		node = "node"
	}
	if opts.HostScriptPath == "" {
		return nil, fmt.Errorf("HostScriptPath is required")
	}
	if opts.WorkingDir == "" {
		return nil, fmt.Errorf("WorkingDir is required")
	}

	cmd := exec.Command(node, opts.HostScriptPath)
	cmd.Dir = opts.WorkingDir
	cmd.Env = append(os.Environ(), opts.ExtraEnv...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("host stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("host stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("host stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start host: %w", err)
	}

	client := &stdioHostClient{
		cmd:     cmd,
		stdin:   stdin,
		stdout:  bufio.NewReader(stdout),
		stderr:  stderr,
		pending: make(map[string]chan rpcResponse),
	}

	go client.readLoop()
	go client.logStderr()

	// Wait briefly for the driver to initialize and prove the protocol works.
	startupCtx := context.Background()
	if opts.StartupTimeout > 0 {
		var cancel context.CancelFunc
		startupCtx, cancel = context.WithTimeout(startupCtx, opts.StartupTimeout)
		defer cancel()
	}
	if _, err := client.Health(startupCtx); err != nil {
		_ = client.Close(context.Background())
		return nil, fmt.Errorf("host health check failed: %w", err)
	}

	return client, nil
}

// rpcRequest is the line-delimited JSON-RPC shape used by this PoC.
type rpcRequest struct {
	ID     string `json:"id"`
	Method string `json:"method"`
	Params any    `json:"params"`
}

// rpcResponse matches the host output.
type rpcResponse struct {
	ID     string          `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *rpcError       `json:"error"`
}

// rpcError matches the host error envelope.
type rpcError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"`
}

func (e *rpcError) Error() string {
	if e.Detail != "" {
		return fmt.Sprintf("%s: %s (%s)", e.Code, e.Message, e.Detail)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (c *stdioHostClient) readLoop() {
	defer c.closeLocked(fmt.Errorf("host read loop ended"))
	for {
		line, err := c.stdout.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				c.closeLocked(fmt.Errorf("host stdout read: %w", err))
			}
			return
		}
		line = line[:len(line)-1] // trim trailing newline
		if line == "" {
			continue
		}
		var resp rpcResponse
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			// Unparseable line: broadcast an error to all pending waiters and stop.
			c.closeLocked(fmt.Errorf("parse host response %q: %w", line, err))
			return
		}
		c.mu.Lock()
		ch, ok := c.pending[resp.ID]
		if ok {
			delete(c.pending, resp.ID)
		}
		c.mu.Unlock()
		if ok {
			ch <- resp
		}
	}
}

func (c *stdioHostClient) logStderr() {
	// Discard stderr in production tests to avoid polluting test output. The
	// host logs are available by running the host directly.
	_, _ = io.Copy(io.Discard, c.stderr)
}

func (c *stdioHostClient) closeLocked(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.closed = true
	c.closeErr = err
	for id, ch := range c.pending {
		ch <- rpcResponse{Error: &rpcError{Code: "HOST_CLOSED", Message: err.Error()}}
		delete(c.pending, id)
	}
	_ = c.stdin.Close()
}

func (c *stdioHostClient) request(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := strconv.FormatUint(c.nextID.Add(1), 10)
	ch := make(chan rpcResponse, 1)

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, fmt.Errorf("host client is closed: %w", c.closeErr)
	}
	c.pending[id] = ch
	c.mu.Unlock()

	req := rpcRequest{ID: id, Method: method, Params: params}
	data, err := json.Marshal(req)
	if err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	c.writeMu.Lock()
	_, writeErr := fmt.Fprintln(c.stdin, string(data))
	c.writeMu.Unlock()
	if writeErr != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, fmt.Errorf("write request: %w", writeErr)
	}

	select {
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		c.cancelRequest(id)
		return nil, ctx.Err()
	case resp := <-ch:
		if resp.Error != nil {
			return nil, resp.Error
		}
		return resp.Result, nil
	}
}

// cancelRequest is an out-of-band host notification. The Node host handles it
// before its serialized driver queue so a long-running GUI request cannot
// delay session termination.
func (c *stdioHostClient) cancelRequest(requestID string) {
	data, err := json.Marshal(rpcRequest{
		ID:     "cancel-" + requestID,
		Method: "cancelRequest",
		Params: map[string]string{"request_id": requestID},
	})
	if err != nil {
		return
	}
	c.writeMu.Lock()
	_, _ = fmt.Fprintln(c.stdin, string(data))
	c.writeMu.Unlock()
}

func (c *stdioHostClient) Health(ctx context.Context) (computer.HealthResult, error) {
	raw, err := c.request(ctx, "health", struct{}{})
	if err != nil {
		return computer.HealthResult{}, err
	}
	var hr struct {
		Healthy            bool   `json:"healthy"`
		Version            string `json:"version"`
		ProtocolVersion    int    `json:"protocol_version"`
		OS                 string `json:"os"`
		TCCScreenRecording bool   `json:"tcc_screen_recording"`
		TCCAccessibility   bool   `json:"tcc_accessibility"`
		Message            string `json:"message"`
	}
	if err := json.Unmarshal(raw, &hr); err != nil {
		return computer.HealthResult{}, fmt.Errorf("decode health: %w", err)
	}
	return computer.HealthResult{
		Healthy:            hr.Healthy,
		Version:            hr.Version,
		ProtocolVersion:    hr.ProtocolVersion,
		OS:                 hr.OS,
		TCCScreenRecording: hr.TCCScreenRecording,
		TCCAccessibility:   hr.TCCAccessibility,
		Message:            hr.Message,
	}, nil
}

func (c *stdioHostClient) StartSession(ctx context.Context, sessionID string, scope computer.CaptureScope, fence computer.FenceToken, epoch computer.ControlEpoch) (HostSessionResult, error) {
	params := map[string]any{
		"sessionID":    sessionID,
		"captureScope": string(scope),
		"fenceToken":   uint64(fence),
		"controlEpoch": uint64(epoch),
	}
	raw, err := c.request(ctx, "startSession", params)
	if err != nil {
		return HostSessionResult{}, err
	}
	var res HostSessionResult
	if err := json.Unmarshal(raw, &res); err != nil {
		return HostSessionResult{}, fmt.Errorf("decode startSession: %w", err)
	}
	return res, nil
}

func (c *stdioHostClient) RegisterPreparedAction(ctx context.Context, action computer.PreparedAction, observationToken string) error {
	_, err := c.request(ctx, "registerPreparedAction", map[string]any{
		"sessionID":        string(action.SessionID),
		"actionID":         string(action.ActionID),
		"actionDigest":     action.ActionDigest,
		"observationID":    string(action.ObservationID),
		"observationToken": observationToken,
		"fenceToken":       uint64(action.FenceToken),
		"controlEpoch":     uint64(action.ControlEpoch),
		"targetGeneration": action.TargetGeneration,
		"expiresAt":        action.ExpiresAt.Format(time.RFC3339Nano),
	})
	if err != nil {
		return fmt.Errorf("register prepared action: %w", err)
	}
	return nil
}

func (c *stdioHostClient) PerformChecked(ctx context.Context, action computer.PreparedAction, params map[string]any) (HostToolResult, error) {
	raw, err := c.request(ctx, "performChecked", map[string]any{
		"sessionID":        string(action.SessionID),
		"actionID":         string(action.ActionID),
		"actionDigest":     action.ActionDigest,
		"observationID":    string(action.ObservationID),
		"fenceToken":       uint64(action.FenceToken),
		"controlEpoch":     uint64(action.ControlEpoch),
		"targetGeneration": action.TargetGeneration,
		"action":           string(action.Kind),
		"parameters":       params,
	})
	if err != nil {
		return HostToolResult{}, err
	}
	var res HostToolResult
	if err := json.Unmarshal(raw, &res); err != nil {
		return HostToolResult{}, fmt.Errorf("decode performChecked: %w", err)
	}
	return res, nil
}

func (c *stdioHostClient) Quiesce(ctx context.Context, sessionID string, nextEpoch computer.ControlEpoch) error {
	_, err := c.request(ctx, "quiesce", map[string]any{
		"sessionID":    sessionID,
		"controlEpoch": uint64(nextEpoch),
	})
	if err != nil {
		return fmt.Errorf("quiesce: %w", err)
	}
	return nil
}

func (c *stdioHostClient) Resume(ctx context.Context, sessionID string, fence computer.FenceToken, epoch computer.ControlEpoch) error {
	_, err := c.request(ctx, "resume", map[string]any{
		"sessionID":    sessionID,
		"fenceToken":   uint64(fence),
		"controlEpoch": uint64(epoch),
	})
	if err != nil {
		return fmt.Errorf("resume: %w", err)
	}
	return nil
}

func (c *stdioHostClient) GetDesktopState(ctx context.Context, sessionID string) (HostDesktopState, error) {
	params := map[string]any{"sessionID": sessionID}
	raw, err := c.request(ctx, "getDesktopState", params)
	if err != nil {
		return HostDesktopState{}, err
	}
	var res HostDesktopState
	if err := json.Unmarshal(raw, &res); err != nil {
		return HostDesktopState{}, fmt.Errorf("decode getDesktopState: %w", err)
	}
	return res, nil
}

func (c *stdioHostClient) GetSessionState(ctx context.Context, sessionID string) (HostSessionState, error) {
	params := map[string]any{"sessionID": sessionID}
	raw, err := c.request(ctx, "getSessionState", params)
	if err != nil {
		return HostSessionState{}, err
	}
	var res HostSessionState
	if err := json.Unmarshal(raw, &res); err != nil {
		return HostSessionState{}, fmt.Errorf("decode getSessionState: %w", err)
	}
	return res, nil
}

func (c *stdioHostClient) EndSession(ctx context.Context, sessionID string) error {
	params := map[string]any{"sessionID": sessionID}
	_, err := c.request(ctx, "endSession", params)
	return err
}

func (c *stdioHostClient) Close(ctx context.Context) error {
	c.closeLocked(fmt.Errorf("client closed"))
	if c.cmd.Process != nil {
		_ = c.cmd.Process.Signal(os.Interrupt)
	}
	done := make(chan error, 1)
	go func() {
		done <- c.cmd.Wait()
	}()
	select {
	case <-ctx.Done():
		_ = c.cmd.Process.Kill()
		return ctx.Err()
	case err := <-done:
		return err
	}
}

var _ HostClient = (*stdioHostClient)(nil)

// sha256sum returns the hex-encoded SHA-256 of data.
func sha256sum(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
