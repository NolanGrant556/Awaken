package hookruntime

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
)

func TestBlockedHookAddressCoversPrivateMetadataCGNATAndMappedIPv6(t *testing.T) {
	blocked := []string{"0.0.0.1", "10.0.0.1", "100.100.100.200", "169.254.169.254", "172.16.0.1", "192.168.1.1", "::", "fc00::1", "fe80::1", "::ffff:169.254.169.254"}
	for _, address := range blocked {
		if !blockedHookAddress(address, false) {
			t.Errorf("expected %s blocked", address)
		}
	}
	if !blockedHookAddress("127.0.0.1", false) || blockedHookAddress("127.0.0.1", true) {
		t.Fatal("loopback must require explicit allowLoopback")
	}
	if blockedHookAddress("8.8.8.8", false) || blockedHookAddress("2606:4700:4700::1111", false) {
		t.Fatal("public addresses unexpectedly blocked")
	}
}

func TestHTTPHookLoopbackSmokeAndSecretHeaderRedaction(t *testing.T) {
	var gotSecret string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		gotSecret = req.Header.Get("Authorization")
		if req.Method != http.MethodPost || !strings.Contains(req.Header.Get("Content-Type"), "application/json") {
			t.Errorf("unexpected request: %s %s", req.Method, req.Header.Get("Content-Type"))
		}
		_, _ = fmt.Fprint(w, `{"protocolVersion":1,"action":"continue","additionalContext":"from-http"}`)
	}))
	defer server.Close()
	hook := CompiledHook{StableID: "user:http", Event: EventRunStart, SourceKind: SourceUser, Command: CommandHook{ID: "http", Type: "http", URL: server.URL, AllowLoopback: true, SecretHeaders: map[string]string{"Authorization": "local-token"}, TimeoutMS: 2000, MaxOutputBytes: 4096}}
	dispatcher := NewDispatcher(AvailabilitySnapshot{HooksByEvent: map[Event][]CompiledHook{EventRunStart: {hook}}}, os.Getenv("PATH"))
	dispatcher.SetSecretResolver(SecretResolverFunc(func(_ context.Context, ref string) (string, error) {
		if ref != "local-token" {
			return "", fmt.Errorf("unknown ref")
		}
		return "Bearer super-secret", nil
	}))
	recorder := &captureHookRecorder{}
	result, err := dispatcher.Dispatch(t.Context(), EventRunStart, map[string]any{"safe": true}, InvocationIdentity{RunID: "run-http", WorkspaceRoot: t.TempDir()}, recorder)
	if err != nil || gotSecret != "Bearer super-secret" || len(result.AdditionalContext) != 1 || result.AdditionalContext[0] != "from-http" {
		t.Fatalf("HTTP Hook failed: result=%+v secret=%q err=%v", result, gotSecret, err)
	}
	if strings.Contains(fmt.Sprintf("%+v", recorder.completed), "super-secret") {
		t.Fatal("secret leaked into Hook outcome")
	}
}

func TestHTTPHookRejectsLoopbackByDefaultAndDNSRebinding(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("blocked target received request") }))
	defer server.Close()
	hook := CompiledHook{StableID: "user:http", Event: EventRunStart, SourceKind: SourceUser, Command: CommandHook{ID: "http", Type: "http", URL: server.URL, TimeoutMS: 1000, MaxOutputBytes: 1024}}
	dispatcher := NewDispatcher(AvailabilitySnapshot{HooksByEvent: map[Event][]CompiledHook{EventRunStart: {hook}}}, "")
	recorder := &captureHookRecorder{}
	_, err := dispatcher.Dispatch(t.Context(), EventRunStart, nil, InvocationIdentity{RunID: "run", WorkspaceRoot: t.TempDir()}, recorder)
	if err != nil || len(recorder.completed) != 1 || recorder.completed[0].ReasonCode != "hook_http_target_blocked" {
		t.Fatalf("loopback did not fail closed: %+v err=%v", recorder.completed, err)
	}

	originalLookup := hookLookupIP
	t.Cleanup(func() { hookLookupIP = originalLookup })
	calls := 0
	hookLookupIP = func(context.Context, string) ([]net.IPAddr, error) {
		calls++
		if calls == 1 {
			return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
		}
		return []net.IPAddr{{IP: net.ParseIP("169.254.169.254")}}, nil
	}
	target, _ := url.Parse("http://example.com/hook")
	if err := validateHookURL(t.Context(), target, false); err != nil {
		t.Fatalf("initial public resolution rejected: %v", err)
	}
	if _, err := guardedHookDialer(false)(t.Context(), "tcp", "example.com:80"); err == nil {
		t.Fatal("rebinding to metadata address was not blocked")
	}
}

func TestHTTPHookRedactsReflectedSecret(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		_, _ = fmt.Fprintf(w, `{"protocolVersion":1,"action":"continue","additionalContext":%q}`, req.Header.Get("X-Hook-Token"))
	}))
	defer server.Close()
	hook := CompiledHook{StableID: "user:http", Event: EventRunStart, SourceKind: SourceUser, Command: CommandHook{ID: "http", Type: "http", URL: server.URL, AllowLoopback: true, SecretHeaders: map[string]string{"X-Hook-Token": "token-ref"}, TimeoutMS: 2000, MaxOutputBytes: 4096}}
	dispatcher := NewDispatcher(AvailabilitySnapshot{HooksByEvent: map[Event][]CompiledHook{EventRunStart: {hook}}}, "")
	dispatcher.SetSecretResolver(SecretResolverFunc(func(context.Context, string) (string, error) { return "secret-value", nil }))
	result, err := dispatcher.Dispatch(t.Context(), EventRunStart, nil, InvocationIdentity{RunID: "run", WorkspaceRoot: t.TempDir()}, &captureHookRecorder{})
	if err != nil || len(result.AdditionalContext) != 1 || result.AdditionalContext[0] != "[REDACTED]" {
		t.Fatalf("reflected secret was not redacted: result=%+v err=%v", result, err)
	}
}

func TestHTTPHookDropsSecretHeadersOnCrossOriginRedirect(t *testing.T) {
	var redirectedSecret string
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		redirectedSecret = req.Header.Get("Authorization")
		_, _ = fmt.Fprint(w, `{"protocolVersion":1,"action":"continue"}`)
	}))
	defer destination.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Header.Get("Authorization") != "Bearer fixture" {
			t.Error("origin did not receive configured secret")
		}
		http.Redirect(w, req, destination.URL, http.StatusTemporaryRedirect)
	}))
	defer origin.Close()
	hook := CompiledHook{StableID: "user:http", Event: EventRunStart, SourceKind: SourceUser, Command: CommandHook{ID: "http", Type: "http", URL: origin.URL, AllowLoopback: true, SecretHeaders: map[string]string{"Authorization": "token-ref"}, TimeoutMS: 2000, MaxOutputBytes: 4096}}
	dispatcher := NewDispatcher(AvailabilitySnapshot{HooksByEvent: map[Event][]CompiledHook{EventRunStart: {hook}}}, "")
	dispatcher.SetSecretResolver(SecretResolverFunc(func(context.Context, string) (string, error) { return "Bearer fixture", nil }))
	if _, err := dispatcher.Dispatch(t.Context(), EventRunStart, nil, InvocationIdentity{RunID: "run", WorkspaceRoot: t.TempDir()}, &captureHookRecorder{}); err != nil {
		t.Fatal(err)
	}
	if redirectedSecret != "" {
		t.Fatalf("secret leaked across origin redirect: %q", redirectedSecret)
	}
}
