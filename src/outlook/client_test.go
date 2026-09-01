package outlook

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"awaken/src/connectorauth"
	"awaken/src/networkroute"
)

type fakeTokens struct {
	token       string
	err         error
	marked      int
	connectorID string
}

type routeAwareGraphTransport struct {
	requests int
	proxied  int
	direct   int
}

func (transport *routeAwareGraphTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	transport.requests++
	proxyURL, err := networkroute.DefaultProxyForRequest(request)
	if err != nil {
		return nil, err
	}
	if proxyURL != nil {
		transport.proxied++
		return nil, io.EOF
	}
	transport.direct++
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"value":[]}`)),
		Request:    request,
	}, nil
}

func (f *fakeTokens) AccessToken(context.Context, string) (string, error) { return f.token, f.err }
func (f *fakeTokens) MarkNeedsReauth(connectorID string) error {
	f.marked++
	f.connectorID = connectorID
	return nil
}

func TestClientGraphCRUDUsesBearerTokenAndBoundedPaths(t *testing.T) {
	var mu sync.Mutex
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer access-token" {
			t.Errorf("authorization header = %q", request.Header.Get("Authorization"))
		}
		mu.Lock()
		calls = append(calls, request.Method+" "+request.URL.EscapedPath())
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodGet && strings.Contains(request.URL.Path, "/mailFolders/inbox/messages"):
			_, _ = w.Write([]byte(`{"value":[{"id":"m1","subject":"Hello"}]}`))
		case request.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"id":"m1","subject":"Hello","body":{"contentType":"text","content":"Body"}}`))
		case request.Method == http.MethodPost && request.URL.Path == "/me/sendMail":
			w.WriteHeader(http.StatusAccepted)
		case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/reply"):
			w.WriteHeader(http.StatusAccepted)
		case request.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			_, _ = w.Write([]byte(`{"id":"m2","subject":"Draft"}`))
		}
	}))
	defer server.Close()

	tokens := &fakeTokens{token: "access-token"}
	client := NewClient(tokens)
	client.SetHTTPClientForTest(server.Client(), server.URL)
	ctx := context.Background()

	list, err := client.ListMessages(ctx, "", "", 10, true)
	if err != nil || len(list.Messages) != 1 || list.Messages[0].ID != "m1" {
		t.Fatalf("list = %#v, err = %v", list, err)
	}
	if _, err := client.GetMessage(ctx, "m1"); err != nil {
		t.Fatal(err)
	}
	compose := ComposeMessage{Subject: "Subject", Body: "Body", To: []Recipient{{Address: "person@example.com"}}}
	if _, err := client.CreateDraft(ctx, compose); err != nil {
		t.Fatal(err)
	}
	if err := client.SendMail(ctx, compose); err != nil {
		t.Fatal(err)
	}
	if err := client.ReplyMessage(ctx, "m1", "Thanks"); err != nil {
		t.Fatal(err)
	}
	read := true
	if _, err := client.UpdateMessage(ctx, "m1", &read, []string{"Customer"}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.MoveMessage(ctx, "m1", "archive"); err != nil {
		t.Fatal(err)
	}
	if err := client.DeleteMessage(ctx, "m1"); err != nil {
		t.Fatal(err)
	}

	want := []string{
		"GET /me/mailFolders/inbox/messages",
		"GET /me/messages/m1",
		"POST /me/messages",
		"POST /me/sendMail",
		"POST /me/messages/m1/reply",
		"PATCH /me/messages/m1",
		"POST /me/messages/m1/move",
		"DELETE /me/messages/m1",
	}
	mu.Lock()
	defer mu.Unlock()
	if len(calls) != len(want) {
		t.Fatalf("calls = %#v", calls)
	}
	for index := range want {
		if calls[index] != want[index] {
			t.Fatalf("call %d = %q, want %q", index, calls[index], want[index])
		}
	}
}

func TestClientRetriesSafeGraphReadDirectAfterBrokenLoopbackProxy(t *testing.T) {
	previous := networkroute.DefaultManager()
	defer networkroute.SetDefaultManager(previous)
	manager := networkroute.NewFromEnvironment([]string{
		"HTTPS_PROXY=http://127.0.0.1:7897",
	}, networkroute.WithEndpointProbe(func(context.Context, string) bool { return true }),
		networkroute.WithNow(func() time.Time { return time.Unix(1_000, 0) }),
		networkroute.WithProxyFailureCooldown(time.Minute))
	networkroute.SetDefaultManager(manager)
	transport := &routeAwareGraphTransport{}
	client := NewClient(&fakeTokens{token: "access-token"})
	client.SetHTTPClientForTest(&http.Client{Transport: transport}, defaultGraphBaseURL)

	if _, err := client.ListMailFolders(context.Background(), "", "", 10, false); err != nil {
		t.Fatal(err)
	}
	if transport.requests != 2 || transport.proxied != 1 || transport.direct != 1 {
		t.Fatalf("requests=%d proxied=%d direct=%d", transport.requests, transport.proxied, transport.direct)
	}
}

func TestClientDoesNotReplayGraphWriteAfterBrokenLoopbackProxy(t *testing.T) {
	previous := networkroute.DefaultManager()
	defer networkroute.SetDefaultManager(previous)
	manager := networkroute.NewFromEnvironment([]string{
		"HTTPS_PROXY=http://127.0.0.1:7897",
	}, networkroute.WithEndpointProbe(func(context.Context, string) bool { return true }))
	networkroute.SetDefaultManager(manager)
	transport := &routeAwareGraphTransport{}
	client := NewClient(&fakeTokens{token: "access-token"})
	client.SetHTTPClientForTest(&http.Client{Transport: transport}, defaultGraphBaseURL)

	err := client.SendMail(context.Background(), ComposeMessage{
		Subject: "safe replay boundary",
		Body:    "body",
		To:      []Recipient{{Address: "person@example.com"}},
	})
	if err == nil || transport.requests != 1 || transport.proxied != 1 || transport.direct != 0 {
		t.Fatalf("err=%v requests=%d proxied=%d direct=%d", err, transport.requests, transport.proxied, transport.direct)
	}
}

func TestClientMarksConnectorNeedsAuthOnGraph401(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"code":"InvalidAuthenticationToken","message":"expired"}}`))
	}))
	defer server.Close()
	tokens := &fakeTokens{token: "expired-token"}
	client := NewClient(tokens)
	client.SetHTTPClientForTest(server.Client(), server.URL)
	var markedServer string
	client.SetNeedsAuthCallback(func(serverName, _ string) error {
		markedServer = serverName
		return nil
	})

	_, err := client.GetMessage(context.Background(), "m1")
	if !errors.Is(err, ErrNeedsAuth) {
		t.Fatalf("err = %v", err)
	}
	if tokens.marked != 1 || tokens.connectorID != ConnectorID || markedServer != ServerName {
		t.Fatalf("tokens = %#v, markedServer = %q", tokens, markedServer)
	}
}

func TestClientRejectsInvalidComposeInputBeforeNetwork(t *testing.T) {
	client := NewClient(&fakeTokens{token: "token"})
	if err := client.SendMail(context.Background(), ComposeMessage{Body: "hello"}); err == nil {
		t.Fatal("expected missing recipient error")
	}
	if err := client.SendMail(context.Background(), ComposeMessage{To: []Recipient{{Address: "bad\n@example.com"}}}); err == nil {
		t.Fatal("expected invalid recipient error")
	}
	client.SetTokenProvider(&fakeTokens{err: connectorauth.ErrNeedsAuth})
	_, err := client.GetMessage(context.Background(), "m1")
	if !errors.Is(err, ErrNeedsAuth) {
		t.Fatalf("err = %v", err)
	}
}

func TestClientDoesNotMisclassifyTransientTokenRefreshAsNeedsAuth(t *testing.T) {
	client := NewClient(&fakeTokens{err: connectorauth.ErrRefreshUnavailable})
	_, err := client.GetMessage(context.Background(), "m1")
	if err == nil || errors.Is(err, ErrNeedsAuth) || !errors.Is(err, connectorauth.ErrRefreshUnavailable) {
		t.Fatalf("err = %v", err)
	}
}
