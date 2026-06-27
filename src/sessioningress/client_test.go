package sessioningress

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestClientAppendsSequentiallyPerSession(t *testing.T) {
	ctx := context.Background()
	firstArrived := make(chan struct{})
	releaseFirst := make(chan struct{})
	records := make(chan requestRecord, 2)
	var inFlight atomic.Int32

	doer := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if got := inFlight.Add(1); got != 1 {
			t.Errorf("expected one in-flight append, got %d", got)
		}
		defer inFlight.Add(-1)

		record := decodeRequestRecord(t, req)
		records <- record

		if record.uuid == "entry-1" {
			close(firstArrived)
			<-releaseFirst
		}

		return responseWithStatus(http.StatusCreated, nil), nil
	})
	client := NewClient(
		"https://session-ingress.example.test/append",
		"token",
		WithHTTPClient(doer),
		WithMaxRetries(0),
		WithSleep(noopSleep),
	)

	errs := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		errs <- client.Append(ctx, "session-a", testEntry("entry-1"))
	}()

	select {
	case <-firstArrived:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for first request")
	}

	first := <-records
	wg.Add(1)
	go func() {
		defer wg.Done()
		errs <- client.Append(ctx, "session-a", testEntry("entry-2"))
	}()

	select {
	case second := <-records:
		t.Fatalf("second request reached transport before first completed: %+v", second)
	case <-time.After(40 * time.Millisecond):
	}

	close(releaseFirst)

	var second requestRecord
	select {
	case second = <-records:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for second request")
	}

	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("append failed: %v", err)
		}
	}

	if first.uuid != "entry-1" {
		t.Fatalf("expected first uuid entry-1, got %q", first.uuid)
	}
	if first.lastUUID != "" {
		t.Fatalf("expected no Last-Uuid on first request, got %q", first.lastUUID)
	}
	if second.uuid != "entry-2" {
		t.Fatalf("expected second uuid entry-2, got %q", second.uuid)
	}
	if second.lastUUID != "entry-1" {
		t.Fatalf("expected second request Last-Uuid entry-1, got %q", second.lastUUID)
	}
}

func TestClientAdoptsServerHeadOnConflictAndRetries(t *testing.T) {
	ctx := context.Background()
	var calls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch calls.Add(1) {
		case 1:
			if got := r.Header.Get(HeaderLastUUID); got != "" {
				t.Errorf("expected no Last-Uuid on first attempt, got %q", got)
			}
			w.Header().Set(HeaderServerLastUUID, "server-head")
			w.WriteHeader(http.StatusConflict)
		case 2:
			if got := r.Header.Get(HeaderLastUUID); got != "server-head" {
				t.Errorf("expected adopted server head, got %q", got)
			}
			w.WriteHeader(http.StatusCreated)
		default:
			t.Errorf("unexpected extra request")
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	sleeper := &recordingSleeper{}
	client := NewClient(
		server.URL,
		"token",
		WithMaxRetries(1),
		WithSleep(sleeper.sleep),
	)
	entry := testEntry("entry-adopt")
	if err := client.Append(ctx, "session-a", entry); err != nil {
		t.Fatalf("append failed: %v", err)
	}

	if got := calls.Load(); got != 2 {
		t.Fatalf("expected 2 attempts, got %d", got)
	}
	if got := sleeper.delays(); len(got) != 0 {
		t.Fatalf("409 adoption should retry immediately, got sleeps %v", got)
	}
	assertHead(t, client, "session-a", entry.UUID)
}

func TestClientTreatsConflictWithEntryAsServerHeadAsSuccess(t *testing.T) {
	ctx := context.Background()
	var calls atomic.Int32
	entry := testEntry("entry-stored")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set(HeaderServerLastUUID, entry.UUID)
		w.WriteHeader(http.StatusConflict)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", WithMaxRetries(3), WithSleep(noopSleep))
	if err := client.Append(ctx, "session-a", entry); err != nil {
		t.Fatalf("append failed: %v", err)
	}

	if got := calls.Load(); got != 1 {
		t.Fatalf("expected 1 attempt, got %d", got)
	}
	assertHead(t, client, "session-a", entry.UUID)
}

func TestClientTreatsAlreadyStoredConflictWithAdvancedServerHeadAsSuccess(t *testing.T) {
	ctx := context.Background()
	var calls atomic.Int32
	entry := testEntry("entry-stored")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set(HeaderServerLastUUID, "server-newer-head")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"already_stored":true}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", WithMaxRetries(3), WithSleep(noopSleep))
	if err := client.Append(ctx, "session-a", entry); err != nil {
		t.Fatalf("append failed: %v", err)
	}

	if got := calls.Load(); got != 1 {
		t.Fatalf("expected 1 attempt, got %d", got)
	}
	assertHead(t, client, "session-a", "server-newer-head")
}

func TestClientDoesNotRetryUnauthorized(t *testing.T) {
	ctx := context.Background()
	token := "secret-token"
	var calls atomic.Int32
	sleeper := &recordingSleeper{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if got := r.Header.Get("Authorization"); got != "Bearer "+token {
			t.Errorf("expected Authorization header, got %q", got)
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client := NewClient(
		server.URL,
		token,
		WithMaxRetries(5),
		WithSleep(sleeper.sleep),
	)
	err := client.Append(ctx, "session-a", testEntry("entry-auth"))
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("expected no retry on 401, got %d calls", got)
	}
	if got := sleeper.delays(); len(got) != 0 {
		t.Fatalf("expected no sleep on 401, got %v", got)
	}
	if strings.Contains(err.Error(), token) {
		t.Fatalf("error leaked token: %q", err.Error())
	}
}

func TestClientRetriesServerError(t *testing.T) {
	ctx := context.Background()
	var calls atomic.Int32
	sleeper := &recordingSleeper{}
	entry := testEntry("entry-retry")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch calls.Add(1) {
		case 1:
			w.WriteHeader(http.StatusInternalServerError)
		case 2:
			w.WriteHeader(http.StatusCreated)
		default:
			t.Errorf("unexpected extra request")
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	client := NewClient(
		server.URL,
		"token",
		WithMaxRetries(1),
		WithSleep(sleeper.sleep),
		WithBackoff(10*time.Millisecond, time.Second),
	)
	if err := client.Append(ctx, "session-a", entry); err != nil {
		t.Fatalf("append failed: %v", err)
	}

	if got := calls.Load(); got != 2 {
		t.Fatalf("expected one retry after 500, got %d calls", got)
	}
	delays := sleeper.delays()
	if len(delays) != 1 || delays[0] != 10*time.Millisecond {
		t.Fatalf("expected one 10ms backoff, got %v", delays)
	}
	assertHead(t, client, "session-a", entry.UUID)
}

func TestClientDoesNotLeakTokenOrPayloadInErrors(t *testing.T) {
	ctx := context.Background()
	token := "secret-token-value"
	payloadSecret := "payload-secret-value"

	doer := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("transport saw token %s and payload %s", token, payloadSecret)
	})
	client := NewClient(
		"https://session-ingress.example.test/append",
		token,
		WithHTTPClient(doer),
		WithMaxRetries(0),
		WithSleep(noopSleep),
	)

	err := client.Append(ctx, "session-a", Entry{
		UUID: "entry-redact",
		Type: "message",
		Payload: map[string]any{
			"text": payloadSecret,
		},
	})
	if !errors.Is(err, ErrRetriesExhausted) {
		t.Fatalf("expected retry exhaustion, got %v", err)
	}
	msg := err.Error()
	if strings.Contains(msg, token) {
		t.Fatalf("error leaked token: %q", msg)
	}
	if strings.Contains(msg, payloadSecret) {
		t.Fatalf("error leaked payload: %q", msg)
	}
}

type requestRecord struct {
	uuid     string
	lastUUID string
}

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) Do(req *http.Request) (*http.Response, error) {
	return f(req)
}

func decodeRequestRecord(t *testing.T, req *http.Request) requestRecord {
	t.Helper()
	defer req.Body.Close()

	var body map[string]any
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode request body: %v", err)
	}
	uuid, ok := body["uuid"].(string)
	if !ok {
		t.Fatalf("request body missing uuid: %#v", body)
	}
	return requestRecord{
		uuid:     uuid,
		lastUUID: req.Header.Get(HeaderLastUUID),
	}
}

func responseWithStatus(status int, header http.Header) *http.Response {
	if header == nil {
		header = make(http.Header)
	}
	return &http.Response{
		StatusCode: status,
		Header:     header,
		Body:       io.NopCloser(strings.NewReader("")),
	}
}

func testEntry(uuid string) Entry {
	return Entry{
		UUID: uuid,
		Type: "message",
		Payload: map[string]any{
			"text": "payload for " + uuid,
		},
	}
}

func assertHead(t *testing.T, client *Client, sessionID string, expected string) {
	t.Helper()
	got, ok, err := client.LastUUID(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("failed to read local head: %v", err)
	}
	if !ok {
		t.Fatalf("expected local head %q, got none", expected)
	}
	if got != expected {
		t.Fatalf("expected local head %q, got %q", expected, got)
	}
}

func noopSleep(ctx context.Context, delay time.Duration) error {
	return nil
}

type recordingSleeper struct {
	mu     sync.Mutex
	values []time.Duration
}

func (s *recordingSleeper) sleep(ctx context.Context, delay time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values = append(s.values, delay)
	return nil
}

func (s *recordingSleeper) delays() []time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]time.Duration, len(s.values))
	copy(out, s.values)
	return out
}
