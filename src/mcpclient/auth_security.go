package mcpclient

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type oauthSafeRoundTripper struct{ transport *http.Transport }

func (r oauthSafeRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	if err := validateOAuthEndpoint(request.Context(), request.URL); err != nil {
		return nil, err
	}
	return r.transport.RoundTrip(request)
}

func oauthSafeHTTPClient() *http.Client {
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
		for _, candidate := range addresses {
			if oauthAddressAllowed(candidate.IP) {
				return (&net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}).DialContext(ctx, network, net.JoinHostPort(candidate.IP.String(), port))
			}
		}
		return nil, errors.New("OAuth endpoint did not resolve to an allowed address")
	}
	client := &http.Client{Transport: oauthSafeRoundTripper{transport: transport}, Timeout: 30 * time.Second}
	client.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many OAuth redirects")
		}
		return validateOAuthEndpoint(request.Context(), request.URL)
	}
	return client
}

func validateOAuthEndpoint(ctx context.Context, target *url.URL) error {
	if target == nil || target.Host == "" || target.User != nil || target.Fragment != "" {
		return errors.New("invalid OAuth endpoint")
	}
	host := strings.TrimSuffix(strings.ToLower(target.Hostname()), ".")
	loopback := host == "localhost"
	if ip := net.ParseIP(host); ip != nil {
		loopback = ip.IsLoopback()
		if !oauthAddressAllowed(ip) {
			return errors.New("OAuth endpoint uses a blocked address")
		}
	}
	if target.Scheme != "https" && !(target.Scheme == "http" && loopback) {
		return errors.New("OAuth endpoint must use public HTTPS or loopback HTTP")
	}
	addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil || len(addresses) == 0 {
		return errors.New("OAuth endpoint DNS lookup failed")
	}
	for _, candidate := range addresses {
		if !oauthAddressAllowed(candidate.IP) {
			return errors.New("OAuth endpoint DNS resolved to a blocked address")
		}
	}
	return nil
}

func oauthAddressAllowed(ip net.IP) bool {
	return ip.IsLoopback() || (!ip.IsPrivate() && !ip.IsUnspecified() && !ip.IsLinkLocalUnicast() && !ip.IsMulticast())
}
