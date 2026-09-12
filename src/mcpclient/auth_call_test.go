package mcpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/oauth2"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestHTTPCallAuthReplayBudget(t *testing.T) {
	for _, tc := range []struct {
		name                            string
		read                            bool
		status                          int
		repeated, business, refreshFail bool
		challenge                       string
		sends, refresh                  int64
		needs                           bool
	}{
		{name: "read refresh", read: true, status: 401, challenge: `Bearer error="invalid_token"`, sends: 2, refresh: 1},
		{name: "write no replay", status: 401, challenge: `Bearer error="invalid_token"`, sends: 1, needs: true},
		{name: "repeat 401", read: true, status: 401, repeated: true, challenge: `Bearer error="invalid_token"`, sends: 2, refresh: 1, needs: true},
		{name: "403", read: true, status: 403, sends: 1, needs: true},
		{name: "scope expansion", read: true, status: 401, challenge: `Bearer error="insufficient_scope", scope="write"`, sends: 1, needs: true},
		{name: "refresh failed", read: true, status: 401, challenge: `Bearer error="invalid_token"`, refreshFail: true, sends: 1, refresh: 1, needs: true},
		{name: "business 401", read: true, business: true, sends: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var sends, refresh atomic.Int64
			server := mcp.NewServer(&mcp.Implementation{Name: "auth-fixture", Version: "1"}, nil)
			mcp.AddTool(server, &mcp.Tool{Name: "lookup", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: tc.read, DestructiveHint: boolPtr(!tc.read), OpenWorldHint: boolPtr(false)}}, func(context.Context, *mcp.CallToolRequest, map[string]any) (*mcp.CallToolResult, any, error) {
				return &mcp.CallToolResult{IsError: tc.business, Content: []mcp.Content{&mcp.TextContent{Text: "provider business 401"}}}, nil, nil
			})
			handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)
			httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/token" {
					refresh.Add(1)
					w.Header().Set("Content-Type", "application/json")
					if tc.refreshFail {
						w.WriteHeader(400)
						io.WriteString(w, `{"error":"invalid_grant"}`)
						return
					}
					io.WriteString(w, `{"access_token":"new","token_type":"Bearer","expires_in":3600,"refresh_token":"rotated"}`)
					return
				}
				if r.Method == "POST" {
					data, _ := io.ReadAll(r.Body)
					r.Body = io.NopCloser(bytes.NewReader(data))
					var body struct{ Method string }
					_ = json.Unmarshal(data, &body)
					if body.Method == "tools/call" {
						sends.Add(1)
						if !tc.business && (r.Header.Get("Authorization") != "Bearer new" || tc.repeated) {
							w.Header().Set("WWW-Authenticate", tc.challenge)
							w.WriteHeader(tc.status)
							return
						}
					}
				}
				handler.ServeHTTP(w, r)
			}))
			defer httpServer.Close()
			cfg := McpServerConfig{HTTP: &HTTPTransportConfig{URL: httpServer.URL + "/mcp"}}
			store := NewInMemoryCredentialStore()
			if err := store.SaveTokenSource(CredentialIdentity(ScopeProject, "fixture", cfg), StoredTokenSource{Token: &oauth2.Token{AccessToken: "old", TokenType: "Bearer", RefreshToken: "refresh", Expiry: time.Now().Add(time.Hour)}, ServerURL: httpServer.URL + "/mcp", TokenURL: httpServer.URL + "/token", ClientID: "fixture", Scopes: []string{"read"}}); err != nil {
				t.Fatal(err)
			}
			mgr := NewManager(WithLogger(noopLogger{}))
			mgr.SetCredentialStore(store)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			defer mgr.Shutdown(context.Background())
			mgr.Start(ctx, []ScopedMcpServerConfig{{Name: "fixture", Scope: ScopeProject, Config: cfg}})
			for {
				snap, _ := mgr.SnapshotServer("fixture")
				if snap.ToolsLoaded {
					break
				}
				select {
				case <-ctx.Done():
					t.Fatalf("connect timeout: %+v", snap)
				case <-time.After(time.Millisecond * 5):
				}
			}
			result, err := mgr.CallTool(ctx, "mcp__fixture__lookup", map[string]any{})
			var need *NeedsAuthError
			if errors.As(err, &need) != tc.needs {
				t.Fatalf("needs=%v err=%v result=%+v", tc.needs, err, result)
			}
			if !tc.needs && err != nil {
				t.Fatal(err)
			}
			if need != nil && (!need.CallStarted || need.ReadOnly != tc.read) {
				t.Fatalf("execution evidence lost: %+v", need)
			}
			if sends.Load() != tc.sends || refresh.Load() != tc.refresh {
				t.Fatalf("sends=%d refresh=%d expected %d/%d", sends.Load(), refresh.Load(), tc.sends, tc.refresh)
			}
		})
	}
}

func TestRejectedTokenRefreshCoalesces(t *testing.T) {
	var count atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count.Add(1)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"access_token":"new","token_type":"Bearer","expires_in":3600}`)
	}))
	defer server.Close()
	store := NewInMemoryCredentialStore()
	_ = store.SaveTokenSource("same", StoredTokenSource{ServerURL: server.URL, TokenURL: server.URL, ClientID: "client", Scopes: []string{"read"}, Token: &oauth2.Token{AccessToken: "old", RefreshToken: "r", Expiry: time.Now().Add(time.Hour)}})
	h := newPersistentOAuthHandler("same", AuthFlowConfig{ServerURL: server.URL}, store)
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := h.refreshRejectedToken(context.Background(), "old", []string{"read"}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if count.Load() != 1 {
		t.Fatalf("refresh count=%d", count.Load())
	}
}

func TestAuthCallRejectsChangedConnectionAndSpentBudget(t *testing.T) {
	mgr := NewManager(WithLogger(noopLogger{}))
	server := &managedServer{name: "read", state: StateConnected, scope: ScopeProject, config: McpServerConfig{HTTP: &HTTPTransportConfig{URL: "https://example.invalid/mcp"}}, tools: []*mcp.Tool{{Name: "read", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: boolPtr(false)}}}}
	mgr.servers["read"] = server
	ctx := mgr.callAuthContext(context.Background(), server, "read", nil)
	call := ctx.Value(authCallKey{}).(*authCall)
	if !call.current() {
		t.Fatal("initial identity invalid")
	}
	if !call.budget.Take() || call.budget.Take() {
		t.Fatal("budget not one")
	}
	mgr.mu.Lock()
	server.connectionGeneration++
	mgr.mu.Unlock()
	if call.current() {
		t.Fatal("replayed across connection replacement")
	}
}

func TestAuthBudgetIgnoresShutdownProtocolMessages(t *testing.T) {
	var sends int
	call := &authCall{current: func() bool { return false }}
	ctx := context.WithValue(context.Background(), authCallKey{}, call)
	transport := authCallTransport{base: authTestRoundTripper(func(req *http.Request) (*http.Response, error) {
		sends++
		return &http.Response{StatusCode: 202, Body: io.NopCloser(bytes.NewReader(nil))}, nil
	})}
	for _, body := range []string{`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":1}}`, `{"jsonrpc":"2.0","id":2,"result":{"action":"cancel"}}`} {
		req, _ := http.NewRequestWithContext(ctx, "POST", "http://localhost/mcp", bytes.NewBufferString(body))
		resp, err := transport.RoundTrip(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}
	if sends != 2 || call.sends != 0 || call.failure != nil {
		t.Fatalf("protocol traffic poisoned auth: sends=%d call=%+v", sends, call)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	req, _ := http.NewRequestWithContext(cancelled, "POST", "http://localhost/mcp", bytes.NewBufferString(`{"method":"tools/call"}`))
	if _, err := transport.RoundTrip(req); !errors.Is(err, context.Canceled) || call.failure != nil {
		t.Fatalf("cancellation became auth failure: %v", err)
	}
}

type authTestRoundTripper func(*http.Request) (*http.Response, error)

func (f authTestRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }
