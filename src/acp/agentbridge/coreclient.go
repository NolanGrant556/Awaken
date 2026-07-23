package agentbridge

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"awaken/src/protocol/agui"
	awruntime "awaken/src/runtime"
)

// CoreClient talks to the managed awaken-core over its HTTP control and
// standard routes. It is owned by the agentbridge package and only uses
// Awaken-native types; no ACP SDK types leak through this boundary.
type CoreClient struct {
	BaseURL      string
	ControlToken string
	HTTPClient   *http.Client
}

// NewCoreClient creates a client bound to a launched Core.
func NewCoreClient(baseURL, controlToken string) *CoreClient {
	return &CoreClient{
		BaseURL:      strings.TrimRight(baseURL, "/"),
		ControlToken: controlToken,
		HTTPClient:   &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *CoreClient) authHeader() string {
	return "Bearer " + c.ControlToken
}

// VerifyIdentity calls the authenticated control identity endpoint and checks
// that the manifest values still match.
func (c *CoreClient) VerifyIdentity(ctx context.Context, expected *awruntime.RuntimeIdentity) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/v1/control/runtime/identity", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", c.authHeader())
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("identity request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("identity status %d", resp.StatusCode)
	}
	var body struct {
		RuntimeIdentity string `json:"runtime_identity"`
		InstanceID      string `json:"instance_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return fmt.Errorf("decode identity: %w", err)
	}
	if body.RuntimeIdentity != expected.RuntimeIdentity || body.InstanceID != expected.InstanceID {
		return errors.New("identity mismatch against control endpoint")
	}
	return nil
}

// CreateLease creates a bridge lease and returns the lease ID.
func (c *CoreClient) CreateLease(ctx context.Context, ownerID string) (string, time.Time, error) {
	payload := map[string]string{"owner_kind": "bridge", "owner_id": ownerID}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", time.Time{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/control/leases", bytes.NewReader(data))
	if err != nil {
		return "", time.Time{}, err
	}
	req.Header.Set("Authorization", c.authHeader())
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("lease request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return "", time.Time{}, fmt.Errorf("lease status %d", resp.StatusCode)
	}
	var body struct {
		LeaseID  string    `json:"lease_id"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", time.Time{}, fmt.Errorf("decode lease: %w", err)
	}
	return body.LeaseID, body.ExpiresAt, nil
}

// ReleaseLease releases a bridge lease.
func (c *CoreClient) ReleaseLease(ctx context.Context, leaseID string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.BaseURL+"/v1/control/leases/"+url.PathEscape(leaseID), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", c.authHeader())
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("release lease status %d", resp.StatusCode)
	}
	return nil
}

// HeartbeatLease renews a bridge lease.
func (c *CoreClient) HeartbeatLease(ctx context.Context, leaseID string) (time.Time, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/control/leases/"+url.PathEscape(leaseID)+"/heartbeat", nil)
	if err != nil {
		return time.Time{}, err
	}
	req.Header.Set("Authorization", c.authHeader())
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return time.Time{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return time.Time{}, fmt.Errorf("heartbeat status %d", resp.StatusCode)
	}
	var body struct {
		ExpiresAt time.Time `json:"expires_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return time.Time{}, err
	}
	return body.ExpiresAt, nil
}

// ACPSessionMapping is the bridge-local view of a Core ACP session.
type ACPSessionMapping struct {
	ACPSessionID string `json:"acp_session_id"`
	ProjectID    string `json:"project_id"`
	ThreadID     string `json:"thread_id"`
	CanonicalCWD string `json:"canonical_cwd"`
	Visibility   string `json:"visibility"`
}

// CreateACPSession asks Core to map an ACP session to a project/thread.
func (c *CoreClient) CreateACPSession(ctx context.Context, acpSessionID, cwd, visibility, clientName, clientVersion string) (*ACPSessionMapping, error) {
	payload := map[string]string{
		"acp_session_id":  acpSessionID,
		"cwd":             cwd,
		"visibility":      visibility,
		"client_name":     clientName,
		"client_version":  clientVersion,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/control/acp/sessions", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", c.authHeader())
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("create session request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("create session status %d", resp.StatusCode)
	}
	var mapping ACPSessionMapping
	if err := json.NewDecoder(resp.Body).Decode(&mapping); err != nil {
		return nil, err
	}
	return &mapping, nil
}

// GetACPSession fetches a session mapping.
func (c *CoreClient) GetACPSession(ctx context.Context, acpSessionID string) (*ACPSessionMapping, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/v1/control/acp/sessions/"+url.PathEscape(acpSessionID), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", c.authHeader())
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get session status %d", resp.StatusCode)
	}
	var mapping ACPSessionMapping
	if err := json.NewDecoder(resp.Body).Decode(&mapping); err != nil {
		return nil, err
	}
	return &mapping, nil
}

// CloseACPSession asks Core to close a session.
func (c *CoreClient) CloseACPSession(ctx context.Context, acpSessionID string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/control/acp/sessions/"+url.PathEscape(acpSessionID)+"/close", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", c.authHeader())
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("close session status %d", resp.StatusCode)
	}
	return nil
}

// Run is the bridge-local view of a Core run.
type Run struct {
	RunID    string `json:"run_id"`
	ThreadID string `json:"thread_id"`
	Status   string `json:"status"`
}

// CreateRun starts a run on the mapped thread.
func (c *CoreClient) CreateRun(ctx context.Context, threadID, input, workspaceRoot string) (*Run, error) {
	payload := awruntime.StartRunRequest{
		ThreadID: threadID,
		Input:    input,
		Workspace: awruntime.WorkspaceConfig{
			WorkspaceRoot: workspaceRoot,
		},
		Client: awruntime.ClientInfo{
			EntryPoint: "awaken-acp",
		},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/runs", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("create run status %d", resp.StatusCode)
	}
	var run Run
	if err := json.NewDecoder(resp.Body).Decode(&run); err != nil {
		return nil, err
	}
	return &run, nil
}

// resolveACPPermission resolves an ACP permission request for a run.
func (c *CoreClient) resolveACPPermission(ctx context.Context, runID, interactionID, optionID string) error {
	payload := map[string]any{
		"option_id": optionID,
		"cancelled": optionID == "" || optionID == "cancelled",
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/runs/"+url.PathEscape(runID)+"/acp-permissions/"+url.PathEscape(interactionID)+"/resolve", bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("resolve permission status %d", resp.StatusCode)
	}
	return nil
}

// CancelRun cancels a run.
func (c *CoreClient) CancelRun(ctx context.Context, runID string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/runs/"+url.PathEscape(runID)+"/cancel", nil)
	if err != nil {
		return err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("cancel run status %d", resp.StatusCode)
	}
	return nil
}

// EventFrame is one AG-UI event from the run event stream.
type EventFrame = agui.Event

// StreamRunEvents reads the SSE event stream for a run and emits AG-UI events.
func (c *CoreClient) StreamRunEvents(ctx context.Context, runID string, afterCursor uint64) (<-chan EventFrame, <-chan error) {
	out := make(chan EventFrame)
	errCh := make(chan error, 1)
	go func() {
		defer close(out)
		defer close(errCh)
		u := c.BaseURL + "/v1/runs/" + url.PathEscape(runID) + "/events?after_cursor=" + strconv.FormatUint(afterCursor, 10)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			errCh <- err
			return
		}
		req.Header.Set("Accept", "text/event-stream")
		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			errCh <- err
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			errCh <- fmt.Errorf("event stream status %d", resp.StatusCode)
			return
		}
		if err := readSSE(ctx, resp.Body, func(data []byte) error {
			var frame agui.Event
			if err := json.Unmarshal(data, &frame); err != nil {
				return err
			}
			select {
			case out <- frame:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}); err != nil && !errors.Is(err, context.Canceled) {
			errCh <- err
		}
	}()
	return out, errCh
}

func readSSE(ctx context.Context, r io.Reader, onData func([]byte) error) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 4096), 8*1024*1024)
	var data []byte
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		line := scanner.Text()
		if line == "" {
			if len(data) > 0 {
				if err := onData(data); err != nil {
					return err
				}
			}
			data = nil
			continue
		}
		if strings.HasPrefix(line, "data: ") {
			if len(data) > 0 {
				data = append(data, '\n')
			}
			data = append(data, []byte(strings.TrimPrefix(line, "data: "))...)
		}
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}
