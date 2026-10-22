package webfetch

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"awaken/src/networkroute"
)

// Each redirect selects its own current host route. A proxy address is trusted
// host configuration, never an argument accepted from the model or a web page.
type routedTransport struct {
	tool          *Tool
	base          *http.Transport
	allowLoopback bool
	proxy         func(*http.Request) (*url.URL, error)
}

func (t *Tool) routedTransport(allowLoopback bool) *routedTransport {
	return &routedTransport{
		tool: t, allowLoopback: allowLoopback, proxy: networkroute.DefaultProxyForRequest,
		base: &http.Transport{
			DisableCompression: true,
			// This transport lives for one bounded fetch; don't retain idle tunnels.
			DisableKeepAlives: true,
			ForceAttemptHTTP2: true, TLSHandshakeTimeout: 10 * time.Second,
			TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
		},
	}
}

func (r *routedTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	target, err := parseFetchURL(request.URL.String(), r.allowLoopback)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid target URL", errBlockedTarget)
	}
	selectedProxy, err := r.proxy(request)
	if err != nil {
		return nil, err // In proxy-only mode, never silently connect directly.
	}
	transport := r.base.Clone()
	if selectedProxy == nil {
		transport.Proxy = nil
		transport.DialContext = r.tool.pinnedDialer(r.allowLoopback && target.Loopback)
		return transport.RoundTrip(request)
	}

	addresses, err := r.tool.resolveTargetIPs(request.Context(), target.Host, false, true)
	if err != nil {
		return nil, err
	}
	transport.Proxy = http.ProxyURL(selectedProxy)
	transport.DialContext = networkroute.WithRouteFailureFeedback((&net.Dialer{Timeout: 10 * time.Second}).DialContext)
	if selectedProxy.Scheme == "https" {
		// The outer proxy TLS certificate belongs to the proxy hostname; the
		// inner origin TLS handshake below still verifies the requested hostname.
		proxyTLS := r.base.TLSClientConfig.Clone()
		proxyTLS.ServerName = selectedProxy.Hostname()
		transport.DialTLSContext = (&tls.Dialer{
			NetDialer: &net.Dialer{Timeout: 10 * time.Second}, Config: proxyTLS,
		}).DialContext
	}
	proxied := request.Clone(request.Context())
	// Preserve hostname TLS verification and HTTP Host. For ordinary public DNS,
	// CONNECT to the validated IP so a second proxy-side DNS lookup cannot rebind.
	// Synthetic answers are meaningful only to the configured proxy: let it
	// resolve the original hostname, never CONNECT to the benchmark address.
	if !isProxySyntheticIP(addresses[0]) {
		copiedURL := *request.URL
		copiedURL.Host = net.JoinHostPort(addresses[0].String(), target.Port)
		proxied.URL = &copiedURL
	}
	proxied.Host = request.URL.Host
	transport.TLSClientConfig.ServerName = target.Host
	response, err := transport.RoundTrip(proxied)
	if response != nil {
		response.Request = request
	}
	return response, err
}
