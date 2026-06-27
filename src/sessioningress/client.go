package sessioningress

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	HeaderLastUUID       = "Last-Uuid"
	HeaderServerLastUUID = "X-Last-Uuid"

	defaultMaxRetries       = 10
	defaultBaseDelay        = 500 * time.Millisecond
	defaultMaxDelay         = 8 * time.Second
	maxConflictResponseBody = 64 * 1024
)

var (
	ErrUnauthorized     = errors.New("session ingress unauthorized")
	ErrConflict         = errors.New("session ingress conflict")
	ErrRetriesExhausted = errors.New("session ingress retries exhausted")
)

type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

type SleepFunc func(ctx context.Context, delay time.Duration) error

type Appender interface {
	Append(ctx context.Context, sessionID string, entry Entry) error
}

type Client struct {
	appendURL  string
	authToken  string
	method     string
	httpClient HTTPDoer
	sleep      SleepFunc
	maxRetries int
	baseDelay  time.Duration
	maxDelay   time.Duration
	headStore  HeadStore
	gate       *AppendGate
}

type Option func(*Client)

func NewClient(appendURL string, authToken string, opts ...Option) *Client {
	client := &Client{
		appendURL:  appendURL,
		authToken:  authToken,
		method:     http.MethodPut,
		httpClient: &http.Client{Timeout: 20 * time.Second},
		sleep:      sleepContext,
		maxRetries: defaultMaxRetries,
		baseDelay:  defaultBaseDelay,
		maxDelay:   defaultMaxDelay,
		headStore:  NewMemoryHeadStore(),
		gate:       NewAppendGate(),
	}
	for _, opt := range opts {
		opt(client)
	}
	return client
}

func WithHTTPClient(httpClient HTTPDoer) Option {
	return func(c *Client) {
		if httpClient != nil {
			c.httpClient = httpClient
		}
	}
}

func WithSleep(sleep SleepFunc) Option {
	return func(c *Client) {
		if sleep != nil {
			c.sleep = sleep
		}
	}
}

func WithMaxRetries(maxRetries int) Option {
	return func(c *Client) {
		if maxRetries < 0 {
			maxRetries = 0
		}
		c.maxRetries = maxRetries
	}
}

func WithBackoff(baseDelay time.Duration, maxDelay time.Duration) Option {
	return func(c *Client) {
		c.baseDelay = baseDelay
		c.maxDelay = maxDelay
	}
}

func WithMethod(method string) Option {
	return func(c *Client) {
		if strings.TrimSpace(method) != "" {
			c.method = strings.ToUpper(strings.TrimSpace(method))
		}
	}
}

func WithHeadStore(store HeadStore) Option {
	return func(c *Client) {
		if store != nil {
			c.headStore = store
		}
	}
}

func WithAppendGate(gate *AppendGate) Option {
	return func(c *Client) {
		if gate != nil {
			c.gate = gate
		}
	}
}

func (c *Client) Append(ctx context.Context, sessionID string, entry Entry) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(sessionID) == "" {
		return errors.New("session ingress append failed: missing session id")
	}
	if strings.TrimSpace(entry.UUID) == "" {
		return errors.New("session ingress append failed: missing entry uuid")
	}

	return c.gate.Do(ctx, sessionID, func(ctx context.Context) error {
		return c.appendSequential(ctx, sessionID, entry)
	})
}

func (c *Client) LastUUID(ctx context.Context, sessionID string) (string, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return c.headStore.LastUUID(ctx, sessionID)
}

func (c *Client) appendSequential(ctx context.Context, sessionID string, entry Entry) error {
	body, err := json.Marshal(entry)
	if err != nil {
		return errors.New("session ingress append failed: encode entry")
	}

	maxAttempts := c.maxRetries + 1
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		lastUUID, hasLastUUID, err := c.headStore.LastUUID(ctx, sessionID)
		if err != nil {
			return errors.New("session ingress append failed: read local head")
		}

		response, err := c.send(ctx, body, lastUUID, hasLastUUID)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			if attempt == maxAttempts {
				return fmt.Errorf("%w: transport error after %d attempt(s)", ErrRetriesExhausted, attempt)
			}
			if err := c.sleepBeforeRetry(ctx, attempt); err != nil {
				return err
			}
			continue
		}

		switch {
		case response.statusCode == http.StatusOK ||
			response.statusCode == http.StatusCreated ||
			response.statusCode == http.StatusNoContent:
			return c.setHead(ctx, sessionID, entry.UUID)
		case response.statusCode == http.StatusUnauthorized:
			return fmt.Errorf("%w: status %d", ErrUnauthorized, response.statusCode)
		case response.statusCode == http.StatusConflict:
			err := c.handleConflict(ctx, sessionID, entry.UUID, response)
			if err == nil {
				return nil
			}
			if !errors.Is(err, errRetryConflict) {
				return err
			}
			if attempt == maxAttempts {
				return fmt.Errorf("%w: conflict after %d attempt(s)", ErrRetriesExhausted, attempt)
			}
			continue
		case response.statusCode == http.StatusTooManyRequests || response.statusCode >= 500:
			if attempt == maxAttempts {
				return fmt.Errorf("%w: status %d after %d attempt(s)", ErrRetriesExhausted, response.statusCode, attempt)
			}
			if err := c.sleepBeforeRetry(ctx, attempt); err != nil {
				return err
			}
			continue
		default:
			return fmt.Errorf("session ingress append failed: status %d", response.statusCode)
		}
	}

	return fmt.Errorf("%w: append attempts exhausted", ErrRetriesExhausted)
}

func (c *Client) send(ctx context.Context, body []byte, lastUUID string, hasLastUUID bool) (appendResponse, error) {
	req, err := http.NewRequestWithContext(ctx, c.method, c.appendURL, bytes.NewReader(body))
	if err != nil {
		return appendResponse{}, errors.New("request build failed")
	}
	req.Header.Set("Content-Type", "application/json")
	if c.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.authToken)
	}
	if hasLastUUID && lastUUID != "" {
		req.Header.Set(HeaderLastUUID, lastUUID)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return appendResponse{}, errors.New("transport error")
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, maxConflictResponseBody))
	if err != nil {
		responseBody = nil
	}

	return appendResponse{
		statusCode: resp.StatusCode,
		header:     resp.Header.Clone(),
		body:       responseBody,
	}, nil
}

type appendResponse struct {
	statusCode int
	header     http.Header
	body       []byte
}

var errRetryConflict = errors.New("retry conflict")

func (c *Client) handleConflict(ctx context.Context, sessionID string, entryUUID string, response appendResponse) error {
	info := conflictInfoFrom(response.header, response.body)
	if info.lastUUID == entryUUID {
		return c.setHead(ctx, sessionID, entryUUID)
	}
	if info.alreadyStored {
		if info.lastUUID != "" {
			return c.setHead(ctx, sessionID, info.lastUUID)
		}
		return c.setHead(ctx, sessionID, entryUUID)
	}
	if info.lastUUID == "" {
		return fmt.Errorf("%w: server head unavailable", ErrConflict)
	}
	if err := c.setHead(ctx, sessionID, info.lastUUID); err != nil {
		return err
	}
	return errRetryConflict
}

func (c *Client) setHead(ctx context.Context, sessionID string, uuid string) error {
	if err := c.headStore.SetLastUUID(ctx, sessionID, uuid); err != nil {
		return errors.New("session ingress append failed: update local head")
	}
	return nil
}

func (c *Client) sleepBeforeRetry(ctx context.Context, attempt int) error {
	delay := c.retryDelay(attempt)
	if delay <= 0 {
		return nil
	}
	return c.sleep(ctx, delay)
}

func (c *Client) retryDelay(attempt int) time.Duration {
	if c.baseDelay <= 0 {
		return 0
	}
	delay := c.baseDelay
	for i := 1; i < attempt; i++ {
		if c.maxDelay > 0 && delay > c.maxDelay/2 {
			return c.maxDelay
		}
		delay *= 2
	}
	if c.maxDelay > 0 && delay > c.maxDelay {
		return c.maxDelay
	}
	return delay
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

type conflictInfo struct {
	lastUUID      string
	alreadyStored bool
}

func conflictInfoFrom(header http.Header, body []byte) conflictInfo {
	info := conflictInfo{
		lastUUID: firstNonEmpty(header.Get(HeaderServerLastUUID), header.Get(HeaderLastUUID)),
	}

	if len(body) == 0 {
		return info
	}

	var value any
	if err := json.Unmarshal(body, &value); err != nil {
		return info
	}
	if info.lastUUID == "" {
		info.lastUUID, _ = findLastUUIDField(value)
	}
	info.alreadyStored = findBoolField(value, "already_stored", "alreadyStored", "entryAlreadyStored")
	return info
}

func findLastUUIDField(value any) (string, bool) {
	switch typed := value.(type) {
	case map[string]any:
		for _, key := range []string{"last_uuid", "lastUuid", "lastUUID", "last-uuid", "x-last-uuid"} {
			if candidate, ok := typed[key].(string); ok && candidate != "" {
				return candidate, true
			}
		}
		for _, child := range typed {
			if candidate, ok := findLastUUIDField(child); ok {
				return candidate, true
			}
		}
	case []any:
		for _, child := range typed {
			if candidate, ok := findLastUUIDField(child); ok {
				return candidate, true
			}
		}
	}
	return "", false
}

func findBoolField(value any, keys ...string) bool {
	switch typed := value.(type) {
	case map[string]any:
		for _, key := range keys {
			if candidate, ok := typed[key].(bool); ok && candidate {
				return true
			}
		}
		for _, child := range typed {
			if findBoolField(child, keys...) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if findBoolField(child, keys...) {
				return true
			}
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
