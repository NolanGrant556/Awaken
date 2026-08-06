package networkroute

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestSnapshotUsesStableEmptyProxyArrays(t *testing.T) {
	manager := NewFromEnvironment(nil)
	payload, err := json.Marshal(manager.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), `"proxies":null`) {
		t.Fatalf("empty proxy routes must remain arrays: %s", payload)
	}
}

func TestAutoFallsBackFromUnavailableLoopbackProxyAndReturnsWhenReachable(t *testing.T) {
	reachable := false
	manager := NewFromEnvironment([]string{
		"HTTPS_PROXY=http://127.0.0.1:7897",
	}, WithProbeTTL(0), WithEndpointProbe(func(context.Context, string) bool {
		return reachable
	}))
	request, _ := http.NewRequest(http.MethodGet, "https://api.example.com/v1", nil)

	proxyURL, err := manager.ProxyForRequest(request)
	if err != nil || proxyURL != nil {
		t.Fatalf("unreachable local proxy should fall back direct, proxy=%v err=%v", proxyURL, err)
	}

	reachable = true
	proxyURL, err = manager.ProxyForRequest(request)
	if err != nil || proxyURL == nil || proxyURL.String() != "http://127.0.0.1:7897" {
		t.Fatalf("reachable local proxy should be restored, proxy=%v err=%v", proxyURL, err)
	}
}

func TestDialFailureInvalidatesLoopbackProbeCache(t *testing.T) {
	reachable := true
	manager := NewFromEnvironment([]string{
		"HTTPS_PROXY=http://127.0.0.1:7897",
	}, WithProbeTTL(time.Minute), WithEndpointProbe(func(context.Context, string) bool {
		return reachable
	}))
	request, _ := http.NewRequest(http.MethodGet, "https://api.example.com/v1", nil)

	proxyURL, err := manager.ProxyForRequest(request)
	if err != nil || proxyURL == nil {
		t.Fatalf("initial loopback proxy should be available, proxy=%v err=%v", proxyURL, err)
	}
	reachable = false
	manager.ReportDialFailure("127.0.0.1:7897")
	proxyURL, err = manager.ProxyForRequest(request)
	if err != nil || proxyURL != nil {
		t.Fatalf("dial failure should force an immediate re-probe, proxy=%v err=%v", proxyURL, err)
	}
}

func TestDialFailureFeedbackInvalidatesDefaultRouteCache(t *testing.T) {
	previous := DefaultManager()
	defer SetDefaultManager(previous)
	reachable := true
	manager := NewFromEnvironment([]string{
		"HTTPS_PROXY=http://127.0.0.1:7897",
	}, WithProbeTTL(time.Minute), WithEndpointProbe(func(context.Context, string) bool {
		return reachable
	}))
	SetDefaultManager(manager)
	request, _ := http.NewRequest(http.MethodGet, "https://api.example.com/v1", nil)
	if proxyURL, err := manager.ProxyForRequest(request); err != nil || proxyURL == nil {
		t.Fatalf("initial loopback proxy should be available, proxy=%v err=%v", proxyURL, err)
	}

	reachable = false
	dial := WithRouteFailureFeedback(func(context.Context, string, string) (net.Conn, error) {
		return nil, errors.New("dial failed")
	})
	if _, err := dial(context.Background(), "tcp", "127.0.0.1:7897"); err == nil {
		t.Fatal("dial feedback test must return the original failure")
	}
	if proxyURL, err := manager.ProxyForRequest(request); err != nil || proxyURL != nil {
		t.Fatalf("feedback should force an immediate re-probe, proxy=%v err=%v", proxyURL, err)
	}
}

func TestProxyPolicyFailsClosed(t *testing.T) {
	manager := NewFromEnvironment([]string{
		"AWAKEN_PROXY_POLICY=proxy",
	}, WithEndpointProbe(func(context.Context, string) bool { return false }))
	request, _ := http.NewRequest(http.MethodGet, "https://api.example.com/v1", nil)
	if _, err := manager.ProxyForRequest(request); !errors.Is(err, ErrProxyRequired) {
		t.Fatalf("proxy-only policy should fail closed, err=%v", err)
	}
}

func TestSystemSnapshotHotUpdate(t *testing.T) {
	manager := NewFromEnvironment(nil, WithEndpointProbe(func(context.Context, string) bool { return true }))
	_, err := manager.UpdateSystemSnapshot(SystemSnapshot{
		SchemaVersion: 1,
		Generation:    2,
		Policy:        PolicyAuto,
		HTTP:          Route{Proxies: []string{"http://127.0.0.1:7890"}, Direct: true},
		HTTPS:         Route{Proxies: []string{"http://127.0.0.1:7890"}, Direct: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	request, _ := http.NewRequest(http.MethodGet, "https://api.example.com/v1", nil)
	proxyURL, err := manager.ProxyForRequest(request)
	if err != nil || proxyURL == nil || proxyURL.String() != "http://127.0.0.1:7890" {
		t.Fatalf("system proxy not applied, proxy=%v err=%v", proxyURL, err)
	}
}

func TestEnvironmentForChildRemovesStaleProxyAndPreservesLoopbackBypass(t *testing.T) {
	manager := NewFromEnvironment([]string{
		"HTTP_PROXY=http://127.0.0.1:7897",
		"HTTPS_PROXY=http://127.0.0.1:7897",
		"NO_PROXY=internal.example",
	}, WithEndpointProbe(func(context.Context, string) bool { return false }))
	environment, err := manager.EnvironmentForChild([]string{
		"PATH=/usr/bin",
		"HTTPS_PROXY=http://127.0.0.1:7897",
		"NO_PROXY=internal.example",
	})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(environment, "\n")
	if strings.Contains(joined, "HTTPS_PROXY=http://127.0.0.1:7897") {
		t.Fatalf("stale proxy leaked to child: %s", joined)
	}
	if !strings.Contains(joined, "PATH=/usr/bin") || !strings.Contains(joined, "internal.example") || !strings.Contains(joined, "127.0.0.1") {
		t.Fatalf("child environment lost required values: %s", joined)
	}
}

func TestNoProxyAndDirectPolicy(t *testing.T) {
	manager := NewFromEnvironment([]string{
		"HTTPS_PROXY=http://127.0.0.1:7897",
		"NO_PROXY=.example.com",
	}, WithEndpointProbe(func(context.Context, string) bool { return true }), WithNow(func() time.Time { return time.Unix(1, 0) }))
	request, _ := http.NewRequest(http.MethodGet, "https://api.example.com/v1", nil)
	proxyURL, err := manager.ProxyForRequest(request)
	if err != nil || proxyURL != nil {
		t.Fatalf("NO_PROXY should bypass proxy, proxy=%v err=%v", proxyURL, err)
	}

	direct := NewFromEnvironment([]string{
		"AWAKEN_PROXY_POLICY=direct",
		"HTTPS_PROXY=http://127.0.0.1:7897",
	}, WithEndpointProbe(func(context.Context, string) bool { return true }))
	proxyURL, err = direct.ProxyForRequest(request)
	if err != nil || proxyURL != nil {
		t.Fatalf("direct policy should bypass proxy, proxy=%v err=%v", proxyURL, err)
	}
}
