// Package protocol isolates third-party ACP SDK types from the rest of Awaken.
// The only allowed imports of github.com/coder/acp-go-sdk live in this package.
package protocol

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"awaken/src/acp"
	"awaken/src/managedenv"

	acpsdk "github.com/coder/acp-go-sdk"
)

const protocolVersionV1 = 1

// Client adapts the third-party ACP SDK to Awaken's internal acp.Client interface.
type Client struct {
	conn       *acpsdk.ClientSideConnection
	cmd        *exec.Cmd
	capability acp.AgentCapabilitiesSnapshot
	maxFrame   int
	maxTurn    int
	logger     *slog.Logger

	mu            sync.Mutex
	currentPrompt *promptState

	processMu          sync.RWMutex
	processDone        chan struct{}
	processErr         error
	processStop        context.CancelFunc
	closeOnce          sync.Once
	closeErr           error
	environmentRelease func()
}

// NewClient spawns a local stdio ACP runner and establishes a client-side connection.
func NewClient(ctx context.Context, snapshot acp.RunnerSnapshot, cwd string) (*Client, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// The ACP process belongs to the Manager-owned conversation, not to the
	// context of the start tool call. Startup and prompt requests keep their
	// own bounded contexts; this private context is cancelled only by Close.
	environment := acp.MergeProcessEnv(os.Environ(), snapshot.Env)
	launch := managedenv.Launch{Executable: snapshot.Command, Args: snapshot.Args}
	release := func() {}
	if m := managedenv.Default(); m != nil {
		var err error
		launch, environment, release, err = m.PrepareCommand(ctx, snapshot.Command, snapshot.Args, environment, "acp")
		if err != nil {
			return nil, err
		}
	} else if _, recognized := managedenv.RecognizePackageCommand(snapshot.Command, snapshot.Args); recognized {
		return nil, fmt.Errorf("environment_manager_unavailable: 共享环境管理器不可用，未启动包管理器")
	}
	started := false
	defer func() {
		if !started {
			release()
		}
	}()
	processCtx, processStop := context.WithCancel(context.Background())
	cmd := exec.CommandContext(processCtx, launch.Executable, launch.Args...)
	if cwd != "" {
		cmd.Dir = cwd
	}
	cmd.Env = environment
	acp.ConfigureProcessTree(cmd)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		processStop()
		return nil, fmt.Errorf("%s: stdin pipe: %w", acp.ErrCodeStartTimeout, err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		processStop()
		return nil, fmt.Errorf("%s: stdout pipe: %w", acp.ErrCodeStartTimeout, err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		processStop()
		return nil, fmt.Errorf("%s: stderr pipe: %w", acp.ErrCodeStartTimeout, err)
	}
	go drainStderr(stderr, snapshot.MaxFrameBytes)

	if err := cmd.Start(); err != nil {
		processStop()
		return nil, fmt.Errorf("%s: %w", acp.ErrCodeRunnerNotReady, err)
	}
	if launch.ManagedInstanceID != "" {
		if m := managedenv.Default(); m != nil {
			hold, err := m.HoldProcess(managedenv.Binding{InstanceID: launch.ManagedInstanceID}, cmd.Process.Pid)
			if err != nil {
				processStop()
				_ = cmd.Wait()
				return nil, err
			}
			originalRelease := release
			release = func() { hold(); originalRelease() }
		}
	}

	impl := &sdkClientImpl{}
	conn := acpsdk.NewClientSideConnection(impl, stdin, stdout)
	impl.conn = conn

	maxFrame := snapshot.MaxFrameBytes
	if maxFrame <= 0 {
		maxFrame = 8 * 1024 * 1024
	}
	c := &Client{
		conn:               conn,
		cmd:                cmd,
		maxFrame:           maxFrame,
		maxTurn:            maxFrame * 4,
		logger:             slog.Default(),
		processDone:        make(chan struct{}),
		processStop:        processStop,
		environmentRelease: release,
	}
	started = true
	go c.waitProcess()

	initCtx, cancel := context.WithTimeout(ctx, time.Duration(snapshot.StartupTimeoutSeconds)*time.Second)
	defer cancel()
	if err := c.initialize(initCtx); err != nil {
		_ = c.Close()
		return nil, err
	}
	impl.client = c
	return c, nil
}

func (c *Client) initialize(ctx context.Context) error {
	resp, err := c.conn.Initialize(ctx, acpsdk.InitializeRequest{
		ProtocolVersion: protocolVersionV1,
		ClientInfo: &acpsdk.Implementation{
			Name:    "awaken",
			Version: currentVersion(),
		},
		ClientCapabilities: acpsdk.ClientCapabilities{
			Fs:       acpsdk.FileSystemCapabilities{ReadTextFile: false, WriteTextFile: false},
			Terminal: false,
		},
	})
	if err != nil {
		if reqErr, ok := err.(*acpsdk.RequestError); ok && reqErr != nil {
			return fmt.Errorf("%s: %s", acp.ErrCodeProtocolVersionMismatch, reqErr.Message)
		}
		return fmt.Errorf("%s: %w", acp.ErrCodeStartTimeout, err)
	}
	if resp.ProtocolVersion != protocolVersionV1 {
		return fmt.Errorf("%s: negotiated version %d != %d", acp.ErrCodeProtocolVersionMismatch, int(resp.ProtocolVersion), protocolVersionV1)
	}
	c.capability = fromSDKCapabilities(resp.AgentCapabilities)
	c.capability.ProtocolVersion = int(resp.ProtocolVersion)
	return nil
}

// Initialize implements acp.Client.
func (c *Client) Initialize(ctx context.Context) (acp.AgentCapabilitiesSnapshot, error) {
	return c.capability, nil
}

// NewSession implements acp.Client.
func (c *Client) NewSession(ctx context.Context, cwd string) (string, error) {
	resp, err := c.conn.NewSession(ctx, acpsdk.NewSessionRequest{
		Cwd:        cwd,
		McpServers: []acpsdk.McpServer{},
	})
	if err != nil {
		return "", fmt.Errorf("%s: %w", acp.ErrCodeTransportClosed, err)
	}
	return string(resp.SessionId), nil
}

// Prompt sends a text prompt and blocks until the turn completes or an error occurs.
func (c *Client) Prompt(ctx context.Context, sessionID, turnID, text string, handler acp.UpdateHandler, permission acp.PermissionHandler) (acp.TurnResult, error) {
	if !c.capability.SupportsTextPrompt {
		return acp.TurnResult{
			ErrorCode:       acp.ErrCodeCapabilityUnsupported,
			ErrorMessage:    "text prompt not supported",
			SideEffectState: "none",
		}, fmt.Errorf("%s: text prompt not supported", acp.ErrCodeCapabilityUnsupported)
	}
	promptCtx, promptCancel := context.WithCancel(ctx)
	defer promptCancel()

	state := &promptState{
		client:     c,
		handler:    handler,
		permission: permission,
		turnID:     turnID,
		toolBuffer: make(map[string]acp.ToolCallSnapshot),
		result: acp.TurnResult{
			SideEffectState: "unknown",
		},
	}
	c.setCurrentPrompt(state)
	defer c.clearCurrentPrompt()
	c.conn.SetLogger(c.logger)

	resp, err := c.conn.Prompt(promptCtx, acpsdk.PromptRequest{
		SessionId: acpsdk.SessionId(sessionID),
		Prompt:    []acpsdk.ContentBlock{acpsdk.TextBlock(text)},
	})
	if err != nil {
		switch {
		case errors.Is(promptCtx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded):
			state.result.ErrorCode = acp.ErrCodeTurnTimeout
			state.result.ErrorMessage = "turn deadline exceeded"
		case errors.Is(promptCtx.Err(), context.Canceled) || errors.Is(err, context.Canceled):
			state.result.ErrorCode = acp.ErrCodeTurnCancelled
			state.result.ErrorMessage = "turn cancelled"
		default:
			state.result.ErrorCode = acp.ErrCodeTransportClosed
			state.result.ErrorMessage = err.Error()
		}
		return state.result, err
	}
	state.result.StopReason = string(resp.StopReason)
	state.result.AssistantText = state.textBuffer.String()
	state.result.ToolSummary = state.toolSummary()
	if state.truncated {
		state.result.Truncated = true
		state.result.AssistantText = truncateBytes(state.textBuffer.String(), c.maxTurn)
	}
	return state.result, nil
}

// Cancel sends a session/cancel notification.
func (c *Client) Cancel(ctx context.Context, sessionID string) error {
	return c.conn.Cancel(ctx, acpsdk.CancelNotification{SessionId: acpsdk.SessionId(sessionID)})
}

// CloseSession implements acp.Client.
func (c *Client) CloseSession(ctx context.Context, sessionID string) error {
	_, err := c.conn.CloseSession(ctx, acpsdk.CloseSessionRequest{SessionId: acpsdk.SessionId(sessionID)})
	return err
}

// Close terminates the process and pipes.
func (c *Client) Close() error {
	c.closeOnce.Do(func() {
		if c.cmd == nil || c.cmd.Process == nil {
			return
		}
		if c.processStop != nil {
			c.processStop()
		} else if c.cmd.Cancel != nil {
			if err := c.cmd.Cancel(); err != nil && !errors.Is(err, os.ErrProcessDone) {
				c.closeErr = err
			}
		} else if err := c.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			c.closeErr = err
		}

		select {
		case <-c.processDone:
		case <-time.After(4 * time.Second):
			if c.cmd.Cancel != nil {
				_ = c.cmd.Cancel()
			} else {
				_ = c.cmd.Process.Kill()
			}
			select {
			case <-c.processDone:
			case <-time.After(2 * time.Second):
				if c.closeErr == nil {
					c.closeErr = fmt.Errorf("ACP process did not exit after close")
				}
			}
		}
	})
	return c.closeErr
}

func (c *Client) waitProcess() {
	err := c.cmd.Wait()
	if c.environmentRelease != nil {
		c.environmentRelease()
	}
	c.processMu.Lock()
	c.processErr = err
	c.processMu.Unlock()
	close(c.processDone)
}

// Done implements acp.ClientLifecycle.
func (c *Client) Done() <-chan struct{} {
	return c.processDone
}

// ExitError implements acp.ClientLifecycle.
func (c *Client) ExitError() error {
	c.processMu.RLock()
	defer c.processMu.RUnlock()
	return c.processErr
}

// Process returns the underlying os/exec.Cmd for tests and process-tree cleanup.
func (c *Client) Process() *exec.Cmd {
	return c.cmd
}

func (c *Client) setCurrentPrompt(state *promptState) {
	c.mu.Lock()
	c.currentPrompt = state
	c.mu.Unlock()
}

func (c *Client) clearCurrentPrompt() {
	c.mu.Lock()
	c.currentPrompt = nil
	c.mu.Unlock()
}

func (c *Client) currentPromptState() *promptState {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.currentPrompt
}

func currentVersion() string {
	return "0.1.0"
}

func truncateBytes(s string, maxBytes int) string {
	if maxBytes <= 0 {
		return s
	}
	b := []byte(s)
	if len(b) <= maxBytes {
		return s
	}
	for maxBytes > 0 && b[maxBytes] >= 0x80 && b[maxBytes] < 0xC0 {
		maxBytes--
	}
	return string(b[:maxBytes])
}

func drainStderr(r io.Reader, maxBytes int) {
	if maxBytes <= 0 {
		maxBytes = 8 * 1024
	}
	buf := make([]byte, maxBytes)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			// Discard; diagnostics are intentionally not retained here.
		}
		if err != nil {
			return
		}
	}
}

type promptState struct {
	client     *Client
	handler    acp.UpdateHandler
	permission acp.PermissionHandler
	turnID     string
	textBuffer strings.Builder
	textBytes  int
	truncated  bool
	toolBuffer map[string]acp.ToolCallSnapshot
	result     acp.TurnResult
}

func (s *promptState) onUpdate(update acp.SessionUpdate) {
	switch update.Kind {
	case acp.SessionUpdateKindAgentText:
		if !s.truncated {
			deltaBytes := len(update.TextDelta)
			limit := s.client.maxTurn
			if s.textBytes+deltaBytes > limit {
				allowed := limit - s.textBytes
				// Do not split a UTF-8 code point; move the cut back to the
				// previous rune boundary so the accumulated text stays valid.
				for allowed > 0 && !utf8.RuneStart(update.TextDelta[allowed]) {
					allowed--
				}
				if allowed > 0 {
					s.textBuffer.WriteString(update.TextDelta[:allowed])
					s.textBytes += allowed
				}
				s.truncated = true
			} else {
				s.textBuffer.WriteString(update.TextDelta)
				s.textBytes += deltaBytes
			}
		}
	case acp.SessionUpdateKindAgentThought:
		// Thoughts are not persisted; only surface live progress if needed.
	case acp.SessionUpdateKindToolCall, acp.SessionUpdateKindToolCallUpdate:
		if update.ToolCall != nil {
			s.toolBuffer[update.ToolCall.ToolCallID] = *update.ToolCall
		}
	}
	if s.handler != nil {
		s.handler.OnUpdate(update)
	}
}

func (s *promptState) toolSummary() acp.ToolSummary {
	var summary acp.ToolSummary
	for _, snap := range s.toolBuffer {
		summary.Total++
		switch snap.Status {
		case "completed":
			summary.Completed++
		case "failed":
			summary.Failed++
		}
	}
	return summary
}

// sdkClientImpl implements the SDK's acp.Client interface.
type sdkClientImpl struct {
	client *Client
	conn   *acpsdk.ClientSideConnection
}

var _ acpsdk.Client = (*sdkClientImpl)(nil)

func (s *sdkClientImpl) RequestPermission(ctx context.Context, params acpsdk.RequestPermissionRequest) (acpsdk.RequestPermissionResponse, error) {
	state := s.client.currentPromptState()
	if state == nil || state.permission == nil {
		return cancelledPermissionResponse(), nil
	}
	req := fromSDKPermissionRequest(params)
	res, err := state.permission.RequestPermission(ctx, req)
	if err != nil {
		return cancelledPermissionResponse(), nil
	}
	if res.Cancelled {
		return cancelledPermissionResponse(), nil
	}
	return acpsdk.RequestPermissionResponse{
		Outcome: acpsdk.RequestPermissionOutcome{
			Selected: &acpsdk.RequestPermissionOutcomeSelected{
				OptionId: acpsdk.PermissionOptionId(res.OptionID),
				Outcome:  "selected",
			},
		},
	}, nil
}

func cancelledPermissionResponse() acpsdk.RequestPermissionResponse {
	return acpsdk.RequestPermissionResponse{
		Outcome: acpsdk.RequestPermissionOutcome{
			Cancelled: &acpsdk.RequestPermissionOutcomeCancelled{Outcome: "cancelled"},
		},
	}
}

func (s *sdkClientImpl) SessionUpdate(ctx context.Context, params acpsdk.SessionNotification) error {
	state := s.client.currentPromptState()
	if state == nil {
		return nil
	}
	update, ok := fromSDKSessionUpdate(params)
	if !ok {
		return nil
	}
	state.onUpdate(update)
	return nil
}

func (s *sdkClientImpl) WriteTextFile(ctx context.Context, params acpsdk.WriteTextFileRequest) (acpsdk.WriteTextFileResponse, error) {
	return acpsdk.WriteTextFileResponse{}, fmt.Errorf("unsupported")
}

func (s *sdkClientImpl) ReadTextFile(ctx context.Context, params acpsdk.ReadTextFileRequest) (acpsdk.ReadTextFileResponse, error) {
	return acpsdk.ReadTextFileResponse{}, fmt.Errorf("unsupported")
}

func (s *sdkClientImpl) CreateTerminal(ctx context.Context, params acpsdk.CreateTerminalRequest) (acpsdk.CreateTerminalResponse, error) {
	return acpsdk.CreateTerminalResponse{}, fmt.Errorf("unsupported")
}

func (s *sdkClientImpl) TerminalOutput(ctx context.Context, params acpsdk.TerminalOutputRequest) (acpsdk.TerminalOutputResponse, error) {
	return acpsdk.TerminalOutputResponse{}, fmt.Errorf("unsupported")
}

func (s *sdkClientImpl) ReleaseTerminal(ctx context.Context, params acpsdk.ReleaseTerminalRequest) (acpsdk.ReleaseTerminalResponse, error) {
	return acpsdk.ReleaseTerminalResponse{}, fmt.Errorf("unsupported")
}

func (s *sdkClientImpl) WaitForTerminalExit(ctx context.Context, params acpsdk.WaitForTerminalExitRequest) (acpsdk.WaitForTerminalExitResponse, error) {
	return acpsdk.WaitForTerminalExitResponse{}, fmt.Errorf("unsupported")
}

func (s *sdkClientImpl) KillTerminal(ctx context.Context, params acpsdk.KillTerminalRequest) (acpsdk.KillTerminalResponse, error) {
	return acpsdk.KillTerminalResponse{}, fmt.Errorf("unsupported")
}
