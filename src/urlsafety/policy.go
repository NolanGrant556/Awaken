package urlsafety

import (
	"net"
	"net/netip"
	"net/url"
	"strings"
)

const (
	ReasonAllowed             = "allowed"
	ReasonBlockedIP           = "blocked_ip"
	ReasonBlockedMetadataHost = "blocked_metadata_host"
	ReasonInvalidResolvedIP   = "invalid_resolved_ip"
	ReasonInvalidURL          = "invalid_url"
	ReasonMissingHost         = "missing_host"
	ReasonMissingResolvedIP   = "missing_resolved_ip"
	ReasonUnsupportedScheme   = "unsupported_scheme"
)

// Config contains policy toggles controlled by the caller.
type Config struct {
	// AllowPrivateURLs allows ordinary RFC1918/ULA private addresses. Metadata,
	// link-local, loopback, reserved, multicast, unspecified, and CGNAT
	// addresses remain blocked.
	AllowPrivateURLs bool

	// TrustedPrivateHosts contains exact hostnames that may resolve to ordinary
	// private addresses when the URL scheme is HTTPS.
	TrustedPrivateHosts []string
}

// Request is the pure input to Evaluate.
type Request struct {
	RawURL      string
	ResolvedIPs []string
	Config      Config
}

// Decision describes the URL policy outcome without including the raw URL.
type Decision struct {
	Allowed    bool
	Reason     string
	Host       string
	Scheme     string
	BlockedIPs []string
	Warnings   []string
}

var blockedMetadataHosts = map[string]struct{}{
	"metadata.google.internal": {},
	"metadata.goog":            {},
}

var alwaysBlockedIPs = map[netip.Addr]struct{}{
	netip.MustParseAddr("169.254.169.254"): {},
	netip.MustParseAddr("169.254.170.2"):   {},
	netip.MustParseAddr("169.254.169.253"): {},
	netip.MustParseAddr("fd00:ec2::254"):   {},
	netip.MustParseAddr("100.100.100.200"): {},
}

var cgnatPrefix = netip.MustParsePrefix("100.64.0.0/10")

var reservedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("255.255.255.255/32"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001:2::/48"),
	netip.MustParsePrefix("2001:db8::/32"),
}

// NormalizeHost canonicalizes a URL hostname or host-like string for exact
// policy comparisons.
func NormalizeHost(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}

	if strings.HasPrefix(host, "[") {
		if end := strings.Index(host, "]"); end >= 0 {
			host = host[1:end]
		}
	} else if strings.Count(host, ":") == 1 {
		if splitHost, _, err := net.SplitHostPort(host); err == nil {
			host = splitHost
		}
	}

	host = strings.TrimSuffix(host, ".")
	return strings.ToLower(host)
}

// IsBlockedIP reports whether an address is blocked by the default SSRF
// policy. It fails closed for the zero netip.Addr value.
func IsBlockedIP(addr netip.Addr) bool {
	class := classifyIP(addr)
	return class.alwaysBlocked || class.privateBlocked || class.otherBlocked
}

// Evaluate applies the URL safety policy to the supplied URL and resolved IPs.
func Evaluate(req Request) Decision {
	decision := Decision{Reason: ReasonInvalidURL}

	parsed, err := url.Parse(req.RawURL)
	if err != nil {
		return decision
	}

	host := NormalizeHost(parsed.Hostname())
	scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
	decision.Host = host
	decision.Scheme = scheme

	if scheme == "" {
		decision.Reason = ReasonInvalidURL
		return decision
	}
	if scheme != "http" && scheme != "https" {
		decision.Reason = ReasonUnsupportedScheme
		return decision
	}
	if host == "" {
		decision.Reason = ReasonMissingHost
		return decision
	}
	if _, blocked := blockedMetadataHosts[host]; blocked {
		decision.Reason = ReasonBlockedMetadataHost
		return decision
	}
	if len(req.ResolvedIPs) == 0 {
		decision.Reason = ReasonMissingResolvedIP
		return decision
	}

	trustedPrivateHost := scheme == "https" && isTrustedPrivateHost(host, req.Config.TrustedPrivateHosts)
	for _, rawIP := range req.ResolvedIPs {
		addr, err := netip.ParseAddr(strings.TrimSpace(rawIP))
		if err != nil {
			decision.Reason = ReasonInvalidResolvedIP
			decision.Warnings = append(decision.Warnings, "resolved IP could not be parsed")
			return decision
		}

		class := classifyIP(addr)
		if class.alwaysBlocked || class.otherBlocked {
			decision.Reason = ReasonBlockedIP
			decision.BlockedIPs = append(decision.BlockedIPs, addr.String())
			continue
		}
		if class.privateBlocked && !req.Config.AllowPrivateURLs && !trustedPrivateHost {
			decision.Reason = ReasonBlockedIP
			decision.BlockedIPs = append(decision.BlockedIPs, addr.String())
			continue
		}
		if class.privateBlocked {
			if trustedPrivateHost {
				decision.Warnings = append(decision.Warnings, "private IP allowed for trusted HTTPS host")
			} else {
				decision.Warnings = append(decision.Warnings, "private IP allowed by config")
			}
		}
	}

	if len(decision.BlockedIPs) > 0 {
		return decision
	}

	decision.Allowed = true
	decision.Reason = ReasonAllowed
	return decision
}

func isTrustedPrivateHost(host string, trustedHosts []string) bool {
	for _, trusted := range trustedHosts {
		if host == NormalizeHost(trusted) {
			return true
		}
	}
	return false
}

type ipClass struct {
	alwaysBlocked  bool
	privateBlocked bool
	otherBlocked   bool
}

func classifyIP(addr netip.Addr) ipClass {
	if !addr.IsValid() {
		return ipClass{alwaysBlocked: true}
	}

	normalized := addr.Unmap()
	if _, blocked := alwaysBlockedIPs[normalized]; blocked {
		return ipClass{alwaysBlocked: true}
	}
	if normalized.IsLinkLocalUnicast() {
		return ipClass{alwaysBlocked: true}
	}

	if normalized.IsPrivate() {
		return ipClass{privateBlocked: true}
	}

	if normalized.IsLoopback() ||
		normalized.IsMulticast() ||
		normalized.IsUnspecified() ||
		cgnatPrefix.Contains(normalized) ||
		isReservedIP(normalized) {
		return ipClass{otherBlocked: true}
	}

	return ipClass{}
}

func isReservedIP(addr netip.Addr) bool {
	for _, prefix := range reservedPrefixes {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}
