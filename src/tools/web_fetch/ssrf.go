package webfetch

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const maxURLBytes = 2048

var errBlockedTarget = errors.New("web fetch target blocked")

var credentialQueryKeys = map[string]struct{}{
	"token": {}, "key": {}, "signature": {}, "access_token": {}, "accesstoken": {},
	"api_key": {}, "apikey": {}, "auth": {}, "password": {}, "secret": {},
	"client_secret": {}, "authorization": {},
}

var forbiddenRemotePrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("192.88.99.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("224.0.0.0/4"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("fc00::/7"),
	netip.MustParsePrefix("fe80::/10"),
	netip.MustParsePrefix("ff00::/8"),
}

type parsedTarget struct {
	URL      *url.URL
	Host     string
	Port     string
	Loopback bool
}

func parseFetchURL(raw string, allowTestHTTP bool) (parsedTarget, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > maxURLBytes || strings.Contains(raw, "\\") ||
		strings.IndexFunc(raw, unicode.IsControl) >= 0 || strings.IndexFunc(raw, unicode.IsSpace) >= 0 {
		return parsedTarget{}, fmt.Errorf("url is invalid")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Opaque != "" || parsed.Host == "" {
		return parsedTarget{}, fmt.Errorf("url is invalid")
	}
	scheme := strings.ToLower(parsed.Scheme)
	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	loopback := isLoopbackHost(host)
	switch scheme {
	case "https":
	case "http":
		if !allowTestHTTP || !loopback {
			return parsedTarget{}, fmt.Errorf("url must use HTTPS")
		}
	default:
		return parsedTarget{}, fmt.Errorf("url scheme is not allowed")
	}
	if parsed.User != nil {
		return parsedTarget{}, fmt.Errorf("url must not contain userinfo")
	}
	if strings.Contains(parsed.Host, "@") || strings.Contains(host, "%") || host == "" {
		return parsedTarget{}, fmt.Errorf("url host is invalid")
	}
	port := parsed.Port()
	if scheme == "https" {
		if port != "" && port != "443" {
			return parsedTarget{}, fmt.Errorf("url must use the default HTTPS port")
		}
		if port == "" {
			port = "443"
		}
	} else if port == "" {
		port = "80"
	}
	if looksLikeMetadataHost(host) {
		return parsedTarget{}, fmt.Errorf("url host is not allowed")
	}
	if loopback && !allowTestHTTP {
		return parsedTarget{}, fmt.Errorf("url must not target private or local address space")
	}
	if !loopback && isLocalOrPrivateHost(host) {
		return parsedTarget{}, fmt.Errorf("url must not target private or local address space")
	}
	if queryHasCredentialParam(parsed.Query()) {
		return parsedTarget{}, fmt.Errorf("url must not contain credential query parameters")
	}
	parsed.Scheme = scheme
	parsed.Fragment = ""
	parsed.RawFragment = ""
	return parsedTarget{URL: parsed, Host: host, Port: port, Loopback: loopback}, nil
}

func queryHasCredentialParam(values url.Values) bool {
	for key := range values {
		normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(key), "-", "_"))
		if _, blocked := credentialQueryKeys[normalized]; blocked {
			return true
		}
	}
	return false
}

func looksLikeMetadataHost(host string) bool {
	host = strings.ToLower(host)
	return host == "metadata.google.internal" || host == "metadata" ||
		strings.HasSuffix(host, ".internal") || strings.Contains(host, "metadata")
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func isLocalOrPrivateHost(host string) bool {
	trimmed := strings.TrimSuffix(strings.ToLower(host), ".")
	if trimmed == "localhost" || strings.HasSuffix(trimmed, ".localhost") ||
		strings.HasSuffix(trimmed, ".local") || strings.HasSuffix(trimmed, ".internal") ||
		!strings.Contains(trimmed, ".") {
		return true
	}
	if address := net.ParseIP(trimmed); address != nil {
		return isForbiddenRemoteIP(address)
	}
	// Proxies may accept shortened, octal or hexadecimal IPv4 spellings that
	// net.ParseIP rejects. Do not let these become proxy-resolved hostnames.
	allNumeric := true
	for _, label := range strings.Split(trimmed, ".") {
		base := 10
		if strings.HasPrefix(label, "0x") {
			base, label = 16, strings.TrimPrefix(label, "0x")
		}
		if _, err := strconv.ParseUint(label, base, 32); err != nil {
			allNumeric = false
			break
		}
	}
	if allNumeric {
		return true
	}
	return false
}

func isForbiddenRemoteIP(address net.IP) bool {
	parsed, ok := netip.AddrFromSlice(address)
	if !ok {
		return true
	}
	parsed = parsed.Unmap()
	if parsed.IsLoopback() {
		return true
	}
	if !parsed.IsGlobalUnicast() {
		return true
	}
	for _, prefix := range forbiddenRemotePrefixes {
		if prefix.Contains(parsed) {
			return true
		}
	}
	return false
}

func (t *Tool) resolvePublicIPs(ctx context.Context, host string, allowLoopback bool) ([]net.IP, error) {
	return t.resolveTargetIPs(ctx, host, allowLoopback, false)
}

func (t *Tool) resolveTargetIPs(ctx context.Context, host string, allowLoopback, allowProxySynthetic bool) ([]net.IP, error) {
	if address := net.ParseIP(host); address != nil {
		if allowLoopback && address.IsLoopback() {
			return []net.IP{address}, nil
		}
		if isForbiddenRemoteIP(address) {
			return nil, fmt.Errorf("%w: resolved address is not a public unicast target", errBlockedTarget)
		}
		return []net.IP{address}, nil
	}
	resolver := t.hostResolver()
	resolveCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	addresses, err := resolver(resolveCtx, host)
	if err != nil || len(addresses) == 0 {
		if err == nil {
			err = fmt.Errorf("no addresses")
		}
		return nil, fmt.Errorf("resolve host: %w", err)
	}
	var allowed []net.IP
	for _, address := range addresses {
		if address == nil {
			return nil, fmt.Errorf("%w: resolved address is not a public unicast target", errBlockedTarget)
		}
		if allowLoopback && address.IsLoopback() {
			allowed = append(allowed, address)
			continue
		}
		// Only a hostname routed through a host-configured proxy can use its
		// synthetic DNS mapping. Literal benchmark IPs remain forbidden above.
		if allowProxySynthetic && isProxySyntheticIP(address) {
			allowed = append(allowed, address)
			continue
		}
		if isForbiddenRemoteIP(address) {
			return nil, fmt.Errorf("%w: resolved address is not a public unicast target", errBlockedTarget)
		}
		allowed = append(allowed, address)
	}
	if len(allowed) == 0 {
		return nil, fmt.Errorf("%w: resolved address is not a public unicast target", errBlockedTarget)
	}
	return allowed, nil
}

var proxySyntheticPrefix = netip.MustParsePrefix("198.18.0.0/15")

func isProxySyntheticIP(address net.IP) bool {
	parsed, ok := netip.AddrFromSlice(address)
	return ok && proxySyntheticPrefix.Contains(parsed.Unmap())
}

func (t *Tool) hostResolver() func(context.Context, string) ([]net.IP, error) {
	if t.ResolveHost != nil {
		return t.ResolveHost
	}
	return func(resolveCtx context.Context, hostname string) ([]net.IP, error) {
		return net.DefaultResolver.LookupIP(resolveCtx, "ip", hostname)
	}
}

func (t *Tool) pinnedDialer(allowLoopback bool) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		ips, err := t.resolvePublicIPs(ctx, host, allowLoopback)
		if err != nil {
			return nil, err
		}
		dialer := &net.Dialer{Timeout: 10 * time.Second}
		var last error
		for _, ip := range ips {
			conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			if dialErr == nil {
				return conn, nil
			}
			last = dialErr
		}
		if last == nil {
			last = fmt.Errorf("no public address to dial")
		}
		return nil, last
	}
}
