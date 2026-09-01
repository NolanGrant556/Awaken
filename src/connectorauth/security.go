package connectorauth

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"awaken/src/networkroute"
)

func validateActionURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return errors.New("authorization URL is invalid")
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	loopback := host == "localhost"
	if ip := net.ParseIP(host); ip != nil {
		loopback = ip.IsLoopback()
		if !loopback && blockedAddress(ip) {
			return errors.New("authorization URL uses a blocked address")
		}
	}
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && loopback) {
		return errors.New("authorization URL must use public HTTPS or loopback HTTP")
	}
	for key := range parsed.Query() {
		switch strings.ToLower(key) {
		case "access_token", "token", "client_secret", "password", "api_key":
			return errors.New("authorization URL contains a credential query parameter")
		}
	}
	return nil
}

type safeRoundTripper struct{ transport *http.Transport }

func (r safeRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	if r.transport == nil || request == nil || request.URL == nil {
		return nil, errors.New("OAuth transport is unavailable")
	}
	if err := validateActionURL(request.URL.String()); err != nil {
		return nil, err
	}
	selectedProxy, err := networkroute.DefaultProxyForRequest(request)
	if err != nil {
		return nil, err
	}
	if selectedProxy != nil {
		probeTransport := r.transport.Clone()
		probeTransport.Proxy = http.ProxyURL(selectedProxy)
		if err := preflightSensitiveOAuthRequest(
			probeTransport,
			request,
			func(*http.Request) (*url.URL, error) { return selectedProxy, nil },
			networkroute.ReportDefaultProxyFailure,
		); err != nil {
			return nil, err
		}
		// A failed loopback preflight may have put the proxy into cooldown.
		// Re-read the route before the one credential-bearing request.
		selectedProxy, err = networkroute.DefaultProxyForRequest(request)
		if err != nil {
			return nil, err
		}
	}

	var response *http.Response
	var roundTripErr error
	if selectedProxy == nil {
		response, roundTripErr = r.roundTripDirect(request)
	} else {
		requestTransport := r.transport.Clone()
		requestTransport.Proxy = http.ProxyURL(selectedProxy)
		response, roundTripErr = requestTransport.RoundTrip(request)
	}
	if roundTripErr != nil {
		networkroute.ReportDefaultProxyFailure(request)
	}
	return response, roundTripErr
}

func (r safeRoundTripper) roundTripDirect(request *http.Request) (*http.Response, error) {
	address, err := resolveAllowedOAuthAddress(request.Context(), request.URL.Hostname())
	if err != nil {
		return nil, err
	}
	declaredHost := strings.TrimSuffix(strings.ToLower(request.URL.Hostname()), ".")
	transport := r.transport.Clone()
	transport.Proxy = nil
	transport.DialContext = func(ctx context.Context, network, target string) (net.Conn, error) {
		host, port, splitErr := net.SplitHostPort(target)
		if splitErr != nil {
			return nil, splitErr
		}
		if strings.TrimSuffix(strings.ToLower(host), ".") != declaredHost {
			return nil, errors.New("OAuth direct route changed target host")
		}
		return (&net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}).DialContext(
			ctx,
			network,
			net.JoinHostPort(address.String(), port),
		)
	}
	return transport.RoundTrip(request)
}

func resolveAllowedOAuthAddress(ctx context.Context, host string) (netip.Addr, error) {
	addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil || len(addresses) == 0 {
		return netip.Addr{}, errors.New("OAuth endpoint DNS lookup failed")
	}
	declaredLoopback := isLoopbackHost(host)
	for _, candidate := range addresses {
		if candidate.IP.IsLoopback() && !declaredLoopback {
			continue
		}
		if blockedAddress(candidate.IP) && !(declaredLoopback && candidate.IP.IsLoopback()) && !isProxySyntheticAddress(candidate.IP) {
			continue
		}
		address, ok := netip.AddrFromSlice(candidate.IP)
		if ok {
			return address.Unmap(), nil
		}
	}
	return netip.Addr{}, errors.New("OAuth endpoint DNS resolved to a blocked address")
}

func preflightSensitiveOAuthRequest(
	transport http.RoundTripper,
	request *http.Request,
	proxyForRequest func(*http.Request) (*url.URL, error),
	reportProxyFailure func(*http.Request) bool,
) error {
	if transport == nil || request == nil || request.URL == nil || request.Method != http.MethodPost || request.Body == nil {
		return nil
	}
	proxyURL, err := proxyForRequest(request)
	if err != nil {
		return err
	}
	if proxyURL == nil || !isLoopbackHost(proxyURL.Hostname()) {
		return nil
	}

	// Validate the CONNECT/TLS path with an empty HEAD before transmitting an
	// authorization code or refresh token. If the local proxy listener is
	// alive but its tunnel is broken, auto policy can switch the one real POST
	// to direct without replaying any credential-bearing request.
	probe := request.Clone(request.Context())
	probe.Method = http.MethodHead
	probe.Body = nil
	probe.GetBody = nil
	probe.ContentLength = 0
	probe.Header = make(http.Header)
	response, probeErr := transport.RoundTrip(probe)
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	if probeErr == nil {
		return nil
	}
	if reportProxyFailure != nil && reportProxyFailure(request) {
		return nil
	}
	return probeErr
}

func newSafeHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = networkroute.DefaultProxyForRequest
	return &http.Client{
		Transport: safeRoundTripper{transport: transport}, Timeout: 30 * time.Second,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many OAuth redirects")
			}
			return validateActionURL(request.URL.String())
		},
	}
}

var proxySyntheticIPv4 = netip.MustParsePrefix("198.18.0.0/15")

func isProxySyntheticAddress(ip net.IP) bool {
	address, ok := netip.AddrFromSlice(ip)
	return ok && proxySyntheticIPv4.Contains(address.Unmap())
}

func isLoopbackHost(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func blockedAddress(ip net.IP) bool {
	return ip.IsPrivate() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsMulticast()
}
