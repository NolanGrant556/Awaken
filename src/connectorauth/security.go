package connectorauth

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
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
	if err := validateActionURL(request.URL.String()); err != nil {
		return nil, err
	}
	addresses, err := net.DefaultResolver.LookupIPAddr(request.Context(), request.URL.Hostname())
	if err != nil || len(addresses) == 0 {
		return nil, errors.New("OAuth endpoint DNS lookup failed")
	}
	declaredLoopback := isLoopbackHost(request.URL.Hostname())
	for _, candidate := range addresses {
		if (candidate.IP.IsLoopback() && !declaredLoopback) || (blockedAddress(candidate.IP) && !(declaredLoopback && candidate.IP.IsLoopback())) {
			return nil, errors.New("OAuth endpoint DNS resolved to a blocked address")
		}
	}
	return r.transport.RoundTrip(request)
}

func newSafeHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = http.ProxyFromEnvironment
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, err
		}
		declaredLoopback := isLoopbackHost(host)
		for _, candidate := range addresses {
			if candidate.IP.IsLoopback() && !declaredLoopback {
				continue
			}
			if !blockedAddress(candidate.IP) || declaredLoopback && candidate.IP.IsLoopback() {
				return (&net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}).DialContext(ctx, network, net.JoinHostPort(candidate.IP.String(), port))
			}
		}
		return nil, errors.New("OAuth endpoint did not resolve to an allowed address")
	}
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
