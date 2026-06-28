package webuibridge

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestPairingCodeTTLAndExchange(t *testing.T) {
	now := time.Date(2026, 6, 28, 10, 0, 0, 0, time.UTC)
	svc := NewService(BridgeConfig{
		Now: func() time.Time { return now },
		Security: SecurityPolicy{
			PairingCodeTTL: time.Minute,
		},
	})

	pairing, err := svc.CreatePairingCode(PairingCodeRequest{ClientName: "phone"})
	if err != nil {
		t.Fatalf("CreatePairingCode() error = %v", err)
	}
	if pairing.Code == "" {
		t.Fatal("CreatePairingCode() returned an empty code")
	}
	if !pairing.ExpiresAt.Equal(now.Add(time.Minute)) {
		t.Fatalf("ExpiresAt = %v, want %v", pairing.ExpiresAt, now.Add(time.Minute))
	}

	client, err := svc.ExchangePairingCode(PairingCodeExchange{
		Code:      pairing.Code,
		Token:     "paired-token",
		Origin:    "http://localhost:5174",
		Host:      "localhost:5174",
		UserAgent: "mobile",
	})
	if err != nil {
		t.Fatalf("ExchangePairingCode() error = %v", err)
	}
	if client.Name != "phone" {
		t.Fatalf("client.Name = %q, want phone", client.Name)
	}

	if _, err := svc.ExchangePairingCode(PairingCodeExchange{Code: pairing.Code, Token: "second-token"}); !errors.Is(err, ErrPairingCodeUsed) {
		t.Fatalf("second ExchangePairingCode() error = %v, want %v", err, ErrPairingCodeUsed)
	}

	expiring, err := svc.CreatePairingCode(PairingCodeRequest{ClientName: "tablet"})
	if err != nil {
		t.Fatalf("CreatePairingCode() error = %v", err)
	}
	now = now.Add(2 * time.Minute)
	if _, err := svc.ExchangePairingCode(PairingCodeExchange{Code: expiring.Code, Token: "late-token"}); !errors.Is(err, ErrPairingCodeExpired) {
		t.Fatalf("expired ExchangePairingCode() error = %v, want %v", err, ErrPairingCodeExpired)
	}
}

func TestTokenIsHashedAndRedacted(t *testing.T) {
	rawToken := "awaken-super-secret-token-123456"
	svc := NewService(BridgeConfig{})

	client, err := svc.RegisterClient(ClientRegistration{
		ClientID:   "client-token",
		ClientName: "browser",
		Token:      rawToken,
		Scopes:     []ClientScope{ScopeRuntimeRead},
	})
	if err != nil {
		t.Fatalf("RegisterClient() error = %v", err)
	}
	if client.Token.Digest == "" || client.Token.Digest == rawToken {
		t.Fatalf("Token.Digest = %q, want hashed digest", client.Token.Digest)
	}
	if client.Token.Preview == "" || strings.Contains(client.Token.Preview, rawToken) || strings.Contains(client.Token.Preview, "123456") {
		t.Fatalf("Token.Preview = %q, want digest-based redaction", client.Token.Preview)
	}
	if !client.Token.Matches(rawToken) {
		t.Fatal("Token.Matches(rawToken) = false, want true")
	}

	payload, err := json.Marshal(client)
	if err != nil {
		t.Fatalf("json.Marshal(client) error = %v", err)
	}
	if strings.Contains(string(payload), rawToken) {
		t.Fatalf("client JSON leaked raw token: %s", payload)
	}

	listPayload, err := json.Marshal(svc.ListClients())
	if err != nil {
		t.Fatalf("json.Marshal(ListClients()) error = %v", err)
	}
	if strings.Contains(string(listPayload), rawToken) {
		t.Fatalf("ListClients JSON leaked raw token: %s", listPayload)
	}
}

func TestRouteAllowlistAndScopes(t *testing.T) {
	svc := NewService(BridgeConfig{})
	readWrite, err := svc.RegisterClient(ClientRegistration{
		ClientID: "client-rw",
		Token:    "rw-token",
		Scopes:   []ClientScope{ScopeRuntimeRead, ScopeRuntimeWrite, ScopeEventsSubscribe},
	})
	if err != nil {
		t.Fatalf("RegisterClient(readWrite) error = %v", err)
	}

	plan, err := svc.BuildProxyPlan(ProxyPlanRequest{
		ClientID: readWrite.ID,
		Method:   "GET",
		Path:     "/v1/runtime/info?verbose=1",
		Origin:   "http://localhost:5174",
		Host:     "localhost:5174",
	})
	if err != nil {
		t.Fatalf("BuildProxyPlan(runtime info) error = %v", err)
	}
	if plan.RouteName != "runtime-info" || plan.TargetPath != "/v1/runtime/info?verbose=1" {
		t.Fatalf("plan = %+v, want runtime-info with query preserved", plan)
	}

	if _, err := svc.BuildProxyPlan(ProxyPlanRequest{ClientID: readWrite.ID, Method: "GET", Path: "/api/fs/read"}); !errors.Is(err, ErrRouteDenied) {
		t.Fatalf("BuildProxyPlan(/api/fs/read) error = %v, want %v", err, ErrRouteDenied)
	}
	if _, err := svc.BuildProxyPlan(ProxyPlanRequest{ClientID: readWrite.ID, Method: "GET", Path: "/v1/runs/abc/../../secrets"}); !errors.Is(err, ErrRouteDenied) {
		t.Fatalf("BuildProxyPlan(path traversal) error = %v, want %v", err, ErrRouteDenied)
	}

	readOnly, err := svc.RegisterClient(ClientRegistration{
		ClientID: "client-ro",
		Token:    "ro-token",
		Scopes:   []ClientScope{ScopeRuntimeRead},
	})
	if err != nil {
		t.Fatalf("RegisterClient(readOnly) error = %v", err)
	}
	if _, err := svc.BuildProxyPlan(ProxyPlanRequest{ClientID: readOnly.ID, Method: "POST", Path: "/v1/runs"}); !errors.Is(err, ErrScopeDenied) {
		t.Fatalf("BuildProxyPlan(POST /v1/runs) error = %v, want %v", err, ErrScopeDenied)
	}
}

func TestSubscriptionLifecycle(t *testing.T) {
	svc := NewService(BridgeConfig{})
	client, err := svc.RegisterClient(ClientRegistration{
		ClientID: "client-events",
		Token:    "events-token",
		Scopes:   []ClientScope{ScopeEventsSubscribe},
	})
	if err != nil {
		t.Fatalf("RegisterClient() error = %v", err)
	}

	first := svc.RecordEvent(ConnectionEvent{Type: EventProxyPlanBuilt, Topic: "runtime", Message: "first"})
	svc.RecordEvent(ConnectionEvent{Type: EventProxyPlanBuilt, Topic: "ui", Message: "skip"})

	sub, err := svc.Subscribe(SubscriptionRequest{
		ClientID: client.ID,
		Topics:   []string{"runtime"},
		AfterSeq: 0,
	})
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	if !sub.Active || len(sub.History) != 1 || sub.History[0].Seq != first.Seq {
		t.Fatalf("subscription history = %+v, want only first runtime event", sub)
	}

	next := svc.RecordEvent(ConnectionEvent{Type: EventWebSocketPlanBuilt, Topic: "runtime", Message: "next"})
	select {
	case got := <-sub.Events:
		if got.Seq != next.Seq {
			t.Fatalf("received Seq = %d, want %d", got.Seq, next.Seq)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for subscription event")
	}

	closed, err := svc.Unsubscribe(sub.ID)
	if err != nil {
		t.Fatalf("Unsubscribe() error = %v", err)
	}
	if closed.Active || closed.ClosedAt == nil {
		t.Fatalf("closed subscription = %+v, want inactive with ClosedAt", closed)
	}
	if _, ok := <-sub.Events; ok {
		t.Fatal("subscription channel is still open after Unsubscribe()")
	}
}

func TestClientRevocationClosesAccess(t *testing.T) {
	svc := NewService(BridgeConfig{})
	client, err := svc.RegisterClient(ClientRegistration{
		ClientID: "client-revoke",
		Token:    "revoke-token",
		Scopes:   []ClientScope{ScopeRuntimeRead, ScopeEventsSubscribe},
	})
	if err != nil {
		t.Fatalf("RegisterClient() error = %v", err)
	}
	if _, err := svc.BuildWebSocketPlan(WebSocketPlanRequest{ClientID: client.ID, Path: "/ws"}); err != nil {
		t.Fatalf("BuildWebSocketPlan() before revoke error = %v", err)
	}
	sub, err := svc.Subscribe(SubscriptionRequest{ClientID: client.ID})
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}

	revoked, err := svc.RevokeClient(client.ID)
	if err != nil {
		t.Fatalf("RevokeClient() error = %v", err)
	}
	if revoked.RevokedAt == nil {
		t.Fatal("RevokedAt is nil")
	}
	if _, err := svc.BuildProxyPlan(ProxyPlanRequest{ClientID: client.ID, Method: "GET", Path: "/v1/runtime/info"}); !errors.Is(err, ErrClientRevoked) {
		t.Fatalf("BuildProxyPlan() after revoke error = %v, want %v", err, ErrClientRevoked)
	}
	if _, err := svc.BuildWebSocketPlan(WebSocketPlanRequest{ClientID: client.ID, Path: "/ws"}); !errors.Is(err, ErrClientRevoked) {
		t.Fatalf("BuildWebSocketPlan() after revoke error = %v, want %v", err, ErrClientRevoked)
	}
	if _, ok := <-sub.Events; ok {
		t.Fatal("subscription channel is still open after RevokeClient()")
	}
}

func TestEventOrderingAndReplayAfterSeq(t *testing.T) {
	now := time.Date(2026, 6, 28, 11, 0, 0, 0, time.UTC)
	svc := NewService(BridgeConfig{Now: func() time.Time { return now }})

	one := svc.RecordEvent(ConnectionEvent{Type: EventClientRegistered, Topic: "runtime"})
	now = now.Add(time.Second)
	two := svc.RecordEvent(ConnectionEvent{Type: EventProxyPlanBuilt, Topic: "runtime"})
	now = now.Add(time.Second)
	three := svc.RecordEvent(ConnectionEvent{Type: EventWebSocketPlanBuilt, Topic: "runtime"})

	if one.Seq != 1 || two.Seq != 2 || three.Seq != 3 {
		t.Fatalf("seqs = %d, %d, %d; want 1, 2, 3", one.Seq, two.Seq, three.Seq)
	}
	if !one.Timestamp.Before(two.Timestamp) || !two.Timestamp.Before(three.Timestamp) {
		t.Fatalf("timestamps not ordered: %v %v %v", one.Timestamp, two.Timestamp, three.Timestamp)
	}

	client, err := svc.RegisterClient(ClientRegistration{
		ClientID: "client-replay",
		Token:    "replay-token",
		Scopes:   []ClientScope{ScopeEventsSubscribe},
	})
	if err != nil {
		t.Fatalf("RegisterClient() error = %v", err)
	}
	sub, err := svc.Subscribe(SubscriptionRequest{ClientID: client.ID, Topics: []string{"runtime"}, AfterSeq: one.Seq})
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	if len(sub.History) != 2 || sub.History[0].Seq != two.Seq || sub.History[1].Seq != three.Seq {
		t.Fatalf("history seqs = %+v, want seq 2 then 3", sub.History)
	}
}

func TestOriginAndHostPolicy(t *testing.T) {
	svc := NewService(BridgeConfig{
		Security: SecurityPolicy{
			AllowedOrigins: []string{"https://trusted.example"},
			AllowedHosts:   []string{"bridge.example"},
		},
	})

	if _, err := svc.RegisterClient(ClientRegistration{
		ClientID: "bad-origin",
		Token:    "bad-token",
		Origin:   "https://evil.example",
		Host:     "bridge.example",
		Scopes:   []ClientScope{ScopeRuntimeRead},
	}); !errors.Is(err, ErrOriginDenied) {
		t.Fatalf("RegisterClient(bad origin) error = %v, want %v", err, ErrOriginDenied)
	}

	client, err := svc.RegisterClient(ClientRegistration{
		ClientID: "trusted",
		Token:    "trusted-token",
		Origin:   "https://trusted.example",
		Host:     "bridge.example",
		Scopes:   []ClientScope{ScopeRuntimeRead},
	})
	if err != nil {
		t.Fatalf("RegisterClient(trusted) error = %v", err)
	}

	if _, err := svc.BuildProxyPlan(ProxyPlanRequest{
		ClientID: client.ID,
		Method:   "GET",
		Path:     "/v1/runtime/info",
		Origin:   "https://evil.example",
		Host:     "bridge.example",
	}); !errors.Is(err, ErrOriginDenied) {
		t.Fatalf("BuildProxyPlan(bad origin) error = %v, want %v", err, ErrOriginDenied)
	}
	if _, err := svc.BuildProxyPlan(ProxyPlanRequest{
		ClientID: client.ID,
		Method:   "GET",
		Path:     "/v1/runtime/info",
		Origin:   "https://trusted.example",
		Host:     "evil.example",
	}); !errors.Is(err, ErrHostDenied) {
		t.Fatalf("BuildProxyPlan(bad host) error = %v, want %v", err, ErrHostDenied)
	}
	if _, err := svc.BuildProxyPlan(ProxyPlanRequest{
		ClientID: client.ID,
		Method:   "GET",
		Path:     "/v1/runtime/info",
		Origin:   "https://trusted.example",
		Host:     "bridge.example",
	}); err != nil {
		t.Fatalf("BuildProxyPlan(trusted) error = %v", err)
	}
}
