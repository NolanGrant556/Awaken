package urlsafety

import (
	"net/netip"
	"testing"
)

func TestEvaluateMetadataHostBlocked(t *testing.T) {
	decision := Evaluate(Request{
		RawURL:      "http://metadata.google.internal./computeMetadata/v1/",
		ResolvedIPs: []string{"8.8.8.8"},
	})

	if decision.Allowed {
		t.Fatalf("metadata host was allowed: %+v", decision)
	}
	if decision.Reason != ReasonBlockedMetadataHost {
		t.Fatalf("reason = %q, want %q", decision.Reason, ReasonBlockedMetadataHost)
	}
	if decision.Host != "metadata.google.internal" {
		t.Fatalf("host = %q, want normalized metadata host", decision.Host)
	}
}

func TestEvaluateMetadataIPBlockedEvenAllowPrivate(t *testing.T) {
	tests := []string{
		"169.254.169.254",
		"169.254.170.2",
		"169.254.169.253",
		"fd00:ec2::254",
		"100.100.100.200",
		"169.254.42.99",
	}

	for _, ip := range tests {
		t.Run(ip, func(t *testing.T) {
			decision := Evaluate(Request{
				RawURL:      "http://example.com/",
				ResolvedIPs: []string{ip},
				Config:      Config{AllowPrivateURLs: true},
			})

			if decision.Allowed {
				t.Fatalf("metadata or link-local IP %s was allowed: %+v", ip, decision)
			}
			if decision.Reason != ReasonBlockedIP {
				t.Fatalf("reason = %q, want %q", decision.Reason, ReasonBlockedIP)
			}
		})
	}
}

func TestEvaluatePrivateIPBlockedByDefault(t *testing.T) {
	decision := Evaluate(Request{
		RawURL:      "http://internal.example/",
		ResolvedIPs: []string{"192.168.1.20"},
	})

	if decision.Allowed {
		t.Fatalf("private IP was allowed by default: %+v", decision)
	}
	if len(decision.BlockedIPs) != 1 || decision.BlockedIPs[0] != "192.168.1.20" {
		t.Fatalf("blocked IPs = %#v", decision.BlockedIPs)
	}
}

func TestEvaluatePrivateIPAllowedWithConfig(t *testing.T) {
	decision := Evaluate(Request{
		RawURL:      "http://router.local/",
		ResolvedIPs: []string{"192.168.1.1"},
		Config:      Config{AllowPrivateURLs: true},
	})

	if !decision.Allowed {
		t.Fatalf("private IP was blocked with AllowPrivateURLs: %+v", decision)
	}
	if len(decision.Warnings) == 0 {
		t.Fatalf("expected warning for private IP allowance")
	}
}

func TestEvaluateTrustedHTTPSPrivateHostAllowed(t *testing.T) {
	decision := Evaluate(Request{
		RawURL:      "https://media.example.com/download",
		ResolvedIPs: []string{"10.0.0.5"},
		Config: Config{
			TrustedPrivateHosts: []string{"MEDIA.EXAMPLE.COM."},
		},
	})

	if !decision.Allowed {
		t.Fatalf("trusted HTTPS private host was blocked: %+v", decision)
	}
}

func TestEvaluateTrustedHostRequiresHTTPS(t *testing.T) {
	decision := Evaluate(Request{
		RawURL:      "http://media.example.com/download",
		ResolvedIPs: []string{"10.0.0.5"},
		Config: Config{
			TrustedPrivateHosts: []string{"media.example.com"},
		},
	})

	if decision.Allowed {
		t.Fatalf("trusted private host was allowed over HTTP: %+v", decision)
	}
}

func TestEvaluateCGNATBlocked(t *testing.T) {
	for _, cfg := range []Config{{}, {AllowPrivateURLs: true}} {
		decision := Evaluate(Request{
			RawURL:      "http://tailscale-peer.example/",
			ResolvedIPs: []string{"100.64.0.1"},
			Config:      cfg,
		})

		if decision.Allowed {
			t.Fatalf("CGNAT IP was allowed with config %+v: %+v", cfg, decision)
		}
	}
}

func TestEvaluateInvalidOrNoHostFailClosed(t *testing.T) {
	tests := []struct {
		name   string
		rawURL string
		reason string
	}{
		{name: "invalid", rawURL: "http://[::1", reason: ReasonInvalidURL},
		{name: "no host", rawURL: "http:///path", reason: ReasonMissingHost},
		{name: "no scheme", rawURL: "example.com/path", reason: ReasonInvalidURL},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := Evaluate(Request{
				RawURL:      tt.rawURL,
				ResolvedIPs: []string{"93.184.216.34"},
			})

			if decision.Allowed {
				t.Fatalf("%s URL was allowed: %+v", tt.name, decision)
			}
			if decision.Reason != tt.reason {
				t.Fatalf("reason = %q, want %q", decision.Reason, tt.reason)
			}
		})
	}
}

func TestEvaluateIPv6LoopbackAndLinkLocalBlocked(t *testing.T) {
	tests := []string{"::1", "fe80::1"}

	for _, ip := range tests {
		t.Run(ip, func(t *testing.T) {
			decision := Evaluate(Request{
				RawURL:      "https://example.com/",
				ResolvedIPs: []string{ip},
				Config:      Config{AllowPrivateURLs: true},
			})

			if decision.Allowed {
				t.Fatalf("IPv6 loopback/link-local IP %s was allowed: %+v", ip, decision)
			}
		})
	}
}

func TestEvaluatePublicIPAllowed(t *testing.T) {
	decision := Evaluate(Request{
		RawURL:      "https://example.com/",
		ResolvedIPs: []string{"93.184.216.34", "2606:2800:220:1:248:1893:25c8:1946"},
	})

	if !decision.Allowed {
		t.Fatalf("public IPs were blocked: %+v", decision)
	}
	if decision.Reason != ReasonAllowed {
		t.Fatalf("reason = %q, want %q", decision.Reason, ReasonAllowed)
	}
}

func TestEvaluateNoResolvedIPBlocked(t *testing.T) {
	decision := Evaluate(Request{RawURL: "https://example.com/"})

	if decision.Allowed {
		t.Fatalf("URL with no resolved IPs was allowed: %+v", decision)
	}
	if decision.Reason != ReasonMissingResolvedIP {
		t.Fatalf("reason = %q, want %q", decision.Reason, ReasonMissingResolvedIP)
	}
}

func TestEvaluateInvalidResolvedIPFailsClosed(t *testing.T) {
	decision := Evaluate(Request{
		RawURL:      "https://example.com/",
		ResolvedIPs: []string{"93.184.216.34", "not-an-ip"},
	})

	if decision.Allowed {
		t.Fatalf("invalid resolved IP was allowed: %+v", decision)
	}
	if decision.Reason != ReasonInvalidResolvedIP {
		t.Fatalf("reason = %q, want %q", decision.Reason, ReasonInvalidResolvedIP)
	}
}

func TestIsBlockedIP(t *testing.T) {
	blocked := []string{
		"127.0.0.1",
		"10.0.0.1",
		"172.16.0.1",
		"192.168.1.1",
		"169.254.169.254",
		"0.0.0.0",
		"224.0.0.1",
		"255.255.255.255",
		"100.64.0.1",
		"198.18.0.23",
		"::1",
		"fe80::1",
		"fd12::1",
		"ff02::1",
		"::ffff:127.0.0.1",
		"::ffff:169.254.169.254",
	}
	for _, ip := range blocked {
		t.Run("blocked "+ip, func(t *testing.T) {
			if !IsBlockedIP(netip.MustParseAddr(ip)) {
				t.Fatalf("%s was not blocked", ip)
			}
		})
	}

	allowed := []string{
		"8.8.8.8",
		"93.184.216.34",
		"100.0.0.1",
		"2606:4700::1",
		"2001:4860:4860::8888",
	}
	for _, ip := range allowed {
		t.Run("allowed "+ip, func(t *testing.T) {
			if IsBlockedIP(netip.MustParseAddr(ip)) {
				t.Fatalf("%s was blocked", ip)
			}
		})
	}
}
