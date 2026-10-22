package webfetch

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"awaken/src/networkroute"
)

// A real CONNECT tunnel and TLS origin exercise proxy routing, original Host,
// certificate validation and the full typed result without external networking.
func TestWebFetchProxyTransport(t *testing.T) {
	for _, proxyTLS := range []bool{false, true} {
		for _, address := range []string{"198.18.1.94", "93.184.216.34"} {
			t.Run(fmt.Sprintf("proxy_tls_%t/%s", proxyTLS, address), func(t *testing.T) {
				var mu sync.Mutex
				var tunnels []string
				origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Host != "example.com" || r.TLS.ServerName != "example.com" {
						t.Errorf("Host=%s SNI=%s", r.Host, r.TLS.ServerName)
					}
					if !strings.Contains(r.Header.Get("Accept"), "application/json") {
						t.Error("JSON missing from Accept")
					}
					switch r.URL.Path {
					case "/private":
						http.Redirect(w, r, "https://127.0.0.1/", http.StatusFound)
						return
					case "/private-dns":
						http.Redirect(w, r, "https://blocked.example/", http.StatusFound)
						return
					}
					if r.URL.Path == "/start" {
						http.Redirect(w, r, "https://example.com/products.json", http.StatusFound)
						return
					}
					w.Header().Set("Content-Type", "application/json")
					io.WriteString(w, `{"products":[{"id":10402140848641,"title":"儿童服装"}]}`)
				}))
				defer origin.Close()
				newProxy := httptest.NewServer
				if proxyTLS {
					newProxy = httptest.NewTLSServer
				}
				proxy := newProxy(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method != http.MethodConnect {
						t.Errorf("method=%s", r.Method)
						w.WriteHeader(400)
						return
					}
					mu.Lock()
					tunnels = append(tunnels, r.Host)
					mu.Unlock()
					upstream, err := net.Dial("tcp", origin.Listener.Addr().String())
					if err != nil {
						t.Error(err)
						return
					}
					conn, _, err := w.(http.Hijacker).Hijack()
					if err != nil {
						upstream.Close()
						t.Error(err)
						return
					}
					defer conn.Close()
					defer upstream.Close()
					io.WriteString(conn, "HTTP/1.1 200 Connection Established\r\n\r\n")
					done := make(chan struct{})
					go func() { io.Copy(upstream, conn); upstream.Close(); close(done) }()
					io.Copy(conn, upstream)
					conn.Close()
					<-done
				}))
				defer proxy.Close()
				manager := networkroute.NewFromEnvironment([]string{"HTTPS_PROXY=" + proxy.URL, "AWAKEN_PROXY_POLICY=proxy"})
				fetch := &Tool{ResolveHost: func(_ context.Context, host string) ([]net.IP, error) {
					if host == "blocked.example" {
						return []net.IP{net.ParseIP("10.0.0.1")}, nil
					}
					return []net.IP{net.ParseIP(address)}, nil
				}}
				transport := fetch.routedTransport(false)
				transport.proxy = manager.ProxyForRequest
				transport.base.TLSClientConfig = origin.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
				fetch.Transport = transport
				outcome, err := fetch.ExecuteOutcome(context.Background(), `{"url":"https://example.com/start"}`, nil)
				if err != nil || outcome.IsError() {
					t.Fatalf("err=%v outcome=%+v", err, outcome)
				}
				if outcome.Details["final_url"] != "https://example.com/products.json" || !strings.Contains(outcome.Content, "儿童服装") {
					t.Fatalf("receipt=%+v", outcome.Details)
				}
				mu.Lock()
				observed := append([]string(nil), tunnels...)
				mu.Unlock()
				want := address + ":443"
				if address == "198.18.1.94" {
					want = "example.com:443"
				}
				if len(observed) != 2 {
					t.Fatalf("tunnels=%v", tunnels)
				}
				for _, got := range observed {
					if got != want {
						t.Errorf("CONNECT=%s want=%s", got, want)
					}
				}
				for _, path := range []string{"/private", "/private-dns"} {
					blocked, _ := fetch.ExecuteOutcome(context.Background(), `{"url":"https://example.com`+path+`"}`, nil)
					if blocked.ErrorCode != "web_fetch_blocked_target" {
						t.Fatalf("redirect %s: %+v", path, blocked)
					}
				}
				mu.Lock()
				defer mu.Unlock()
				if len(tunnels) != 4 {
					t.Fatalf("unsafe redirect reached proxy: %v", tunnels)
				}
			})
		}
	}
}

func TestWebFetchProxyRejectsUnsafeDNS(t *testing.T) {
	for _, address := range []string{"127.0.0.1", "10.0.0.1", "169.254.169.254", "::1", "fd00::1", "198.51.100.1"} {
		t.Run(address, func(t *testing.T) {
			fetch := &Tool{ResolveHost: func(context.Context, string) ([]net.IP, error) {
				return []net.IP{net.ParseIP("198.18.1.94"), net.ParseIP(address)}, nil
			}}
			transport := fetch.routedTransport(false)
			transport.proxy = func(*http.Request) (*url.URL, error) { return url.Parse("http://127.0.0.1:1") }
			fetch.Transport = transport
			outcome, err := fetch.ExecuteOutcome(context.Background(), `{"url":"https://example.com/"}`, nil)
			if err != nil || outcome.ErrorCode != "web_fetch_blocked_target" {
				t.Fatalf("err=%v outcome=%+v", err, outcome)
			}
		})
	}
}

func TestWebFetchDirectRejectsSyntheticDNS(t *testing.T) {
	fetch := &Tool{ResolveHost: func(context.Context, string) ([]net.IP, error) { return []net.IP{net.ParseIP("198.18.1.94")}, nil }}
	transport := fetch.routedTransport(false)
	transport.proxy = networkroute.NewFromEnvironment([]string{"AWAKEN_PROXY_POLICY=direct"}).ProxyForRequest
	fetch.Transport = transport
	outcome, _ := fetch.ExecuteOutcome(context.Background(), `{"url":"https://example.com/"}`, nil)
	if outcome.ErrorCode != "web_fetch_blocked_target" {
		t.Fatalf("outcome=%+v", outcome)
	}
}

func TestWebFetchProxyOnlyWithoutProxyFailsClosed(t *testing.T) {
	fetch := &Tool{ResolveHost: func(context.Context, string) ([]net.IP, error) {
		t.Fatal("must not fall back to direct DNS")
		return nil, nil
	}}
	transport := fetch.routedTransport(false)
	transport.proxy = networkroute.NewFromEnvironment([]string{"AWAKEN_PROXY_POLICY=proxy"}).ProxyForRequest
	fetch.Transport = transport
	outcome, _ := fetch.ExecuteOutcome(context.Background(), `{"url":"https://example.com/"}`, nil)
	if outcome.ErrorCode != "web_fetch_network_failed" {
		t.Fatalf("outcome=%+v", outcome)
	}
}

func TestWebFetchProxyRejectsLiteralAndAlternativeIPs(t *testing.T) {
	for _, host := range []string{"198.18.1.94", "127.1", "0177.0.0.1", "0x7f.0.0.1", "[::ffff:127.0.0.1]"} {
		if _, err := parseFetchURL("https://"+host+"/", false); err == nil {
			t.Errorf("admitted %s", host)
		}
	}
}

func TestWebFetchNoProxyDoesNotAdmitSyntheticDNS(t *testing.T) {
	manager := networkroute.NewFromEnvironment([]string{"AWAKEN_PROXY_POLICY=proxy", "HTTPS_PROXY=http://127.0.0.1:1", "NO_PROXY=example.com"})
	fetch := &Tool{ResolveHost: func(context.Context, string) ([]net.IP, error) { return []net.IP{net.ParseIP("198.18.1.94")}, nil }}
	transport := fetch.routedTransport(false)
	transport.proxy = manager.ProxyForRequest
	fetch.Transport = transport
	outcome, _ := fetch.ExecuteOutcome(context.Background(), `{"url":"https://example.com/"}`, nil)
	if outcome.ErrorCode != "web_fetch_blocked_target" {
		t.Fatalf("NO_PROXY bypass admitted synthetic DNS: %+v", outcome)
	}
}
