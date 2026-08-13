package hookruntime

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"awaken/src/networkroute"
)

const maxHTTPRedirects = 5

var hookLookupIP = net.DefaultResolver.LookupIPAddr

func (d *Dispatcher) executeHTTP(parent context.Context, envelope InputEnvelope, hook CompiledHook) (out Outcome, response Response, elapsed time.Duration) {
	started := time.Now()
	defer func() {
		elapsed = time.Since(started)
		out.DurationMS = elapsed.Milliseconds()
	}()
	out = Outcome{InvocationID: envelope.InvocationID, HookID: hook.StableID, Event: envelope.Event, SourceKind: hook.SourceKind, Status: "failed", SpawnState: "not_started", ProcessState: "unknown", Action: "continue"}
	ctx, cancel := context.WithTimeout(parent, time.Duration(hook.Command.TimeoutMS)*time.Millisecond)
	defer cancel()
	body, _ := json.Marshal(envelope)
	headers, err := d.resolveHTTPHeaders(ctx, hook.Command.SecretHeaders)
	if err != nil {
		out.ReasonCode = "hook_secret_unavailable"
		return out, Response{Action: "continue"}, time.Since(started)
	}
	transport := &http.Transport{
		DialContext:         guardedHookDialer(hook.Command.AllowLoopback),
		TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12},
		ForceAttemptHTTP2:   true,
		MaxIdleConns:        4,
		IdleConnTimeout:     15 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}
	client := &http.Client{
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxHTTPRedirects {
				return errors.New("hook redirect limit exceeded")
			}
			if err := validateHookURL(ctx, req.URL, hook.Command.AllowLoopback); err != nil {
				return err
			}
			if len(via) > 0 && !sameHookOrigin(via[0].URL, req.URL) {
				for name := range hook.Command.SecretHeaders {
					req.Header.Del(name)
				}
			}
			return nil
		},
	}
	target, _ := url.Parse(hook.Command.URL)
	if err := validateHookURL(ctx, target, hook.Command.AllowLoopback); err != nil {
		out.ReasonCode, out.OutputPreview = "hook_http_target_blocked", boundedPreview([]byte(err.Error()), 512)
		return out, Response{Action: "continue"}, time.Since(started)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target.String(), bytes.NewReader(body))
	if err != nil {
		out.ReasonCode = "hook_http_request_invalid"
		return out, Response{Action: "continue"}, time.Since(started)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	proxyURL, proxyErr := networkroute.DefaultProxyForRequest(req)
	if proxyErr != nil {
		out.ReasonCode = "hook_proxy_unavailable"
		return out, Response{Action: "continue"}, time.Since(started)
	}
	if proxyURL != nil {
		transport.Proxy = http.ProxyURL(proxyURL)
		transport.DialContext = (&net.Dialer{Timeout: 10 * time.Second}).DialContext
	}
	out.SpawnState, out.ProcessState = "started", "running"
	resp, err := client.Do(req)
	if err != nil {
		out.ProcessState = "exited"
		if ctx.Err() != nil {
			out.Status, out.ProcessState, out.ReasonCode = "timed_out", "timeout", "hook_timed_out"
			if errors.Is(parent.Err(), context.Canceled) {
				out.Status, out.ProcessState, out.ReasonCode = "cancelled", "cancelled", "hook_cancelled"
			}
		} else {
			out.ReasonCode, out.OutputPreview = "hook_http_failed", boundedPreview([]byte(err.Error()), 512)
		}
		return out, Response{Action: "continue"}, time.Since(started)
	}
	defer resp.Body.Close()
	limited, readErr := io.ReadAll(io.LimitReader(resp.Body, int64(hook.Command.MaxOutputBytes)+1))
	out.ProcessState = "exited"
	if readErr != nil {
		out.ReasonCode = "hook_http_read_failed"
		return out, Response{Action: "continue"}, time.Since(started)
	}
	if len(limited) > hook.Command.MaxOutputBytes {
		out.ReasonCode = "hook_output_limit_exceeded"
		return out, Response{Action: "continue"}, time.Since(started)
	}
	limited = redactHookSecrets(limited, headers)
	out.StdoutDigest = shortDigest(limited)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		out.ReasonCode = "hook_http_status"
		out.OutputPreview = fmt.Sprintf("HTTP %d", resp.StatusCode)
		return out, Response{Action: "continue"}, time.Since(started)
	}
	var parseErr error
	response, parseErr = parseResponse(limited, envelope.Event)
	if parseErr != nil {
		out.ReasonCode, out.OutputPreview = "hook_protocol_invalid", "HTTP Hook returned an invalid response"
		return out, Response{Action: "continue"}, time.Since(started)
	}
	out.Status, out.Action, out.Reason = "success", response.Action, response.Reason
	if isBlockingAction(response.Action) {
		out.Status = "blocked"
	}
	return out, response, time.Since(started)
}

func sameHookOrigin(left, right *url.URL) bool {
	return left != nil && right != nil && strings.EqualFold(left.Scheme, right.Scheme) && strings.EqualFold(left.Host, right.Host)
}

func redactHookSecrets(body []byte, headers map[string]string) []byte {
	redacted := append([]byte(nil), body...)
	for _, value := range headers {
		if value != "" {
			encodedValue, _ := json.Marshal(value)
			encodedReplacement, _ := json.Marshal("[REDACTED]")
			redacted = bytes.ReplaceAll(redacted, encodedValue[1:len(encodedValue)-1], encodedReplacement[1:len(encodedReplacement)-1])
		}
	}
	return redacted
}

func (d *Dispatcher) resolveHTTPHeaders(ctx context.Context, refs map[string]string) (map[string]string, error) {
	values := make(map[string]string, len(refs))
	for name, ref := range refs {
		if d.secretResolver == nil {
			return nil, errors.New("Hook secret resolver unavailable")
		}
		value, err := d.secretResolver.ResolveHookSecret(ctx, ref)
		if err != nil || strings.ContainsAny(value, "\r\n\x00") {
			return nil, errors.New("Hook secret unavailable")
		}
		values[name] = value
	}
	return values, nil
}

func validateHookURL(ctx context.Context, target *url.URL, allowLoopback bool) error {
	if target == nil || target.Host == "" || (target.Scheme != "http" && target.Scheme != "https") || target.User != nil {
		return errors.New("invalid Hook URL")
	}
	addresses, err := hookLookupIP(ctx, target.Hostname())
	if err != nil || len(addresses) == 0 {
		return errors.New("Hook target DNS lookup failed")
	}
	for _, address := range addresses {
		if blockedHookAddress(address.IP.String(), allowLoopback) {
			return fmt.Errorf("Hook target resolves to blocked address %s", address.IP)
		}
	}
	return nil
}

func guardedHookDialer(allowLoopback bool) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		addresses, err := hookLookupIP(ctx, host)
		if err != nil || len(addresses) == 0 {
			return nil, errors.New("Hook dial DNS lookup failed")
		}
		for _, candidate := range addresses {
			if blockedHookAddress(candidate.IP.String(), allowLoopback) {
				return nil, fmt.Errorf("Hook dial blocked address %s", candidate.IP)
			}
		}
		// Pin the connection to the exact address already validated above.
		return (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, network, net.JoinHostPort(addresses[0].IP.String(), port))
	}
}

func blockedHookAddress(raw string, allowLoopback bool) bool {
	address, err := netip.ParseAddr(raw)
	if err != nil {
		return true
	}
	address = address.Unmap()
	if address.IsLoopback() {
		return !allowLoopback
	}
	if address.IsUnspecified() || address.IsPrivate() || address.IsLinkLocalUnicast() || address.IsLinkLocalMulticast() || address.IsMulticast() {
		return true
	}
	if address.Is4() {
		for _, prefix := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "224.0.0.0/4", "240.0.0.0/4"} {
			if netip.MustParsePrefix(prefix).Contains(address) {
				return true
			}
		}
	} else {
		for _, prefix := range []string{"2001:db8::/32", "fc00::/7", "fe80::/10"} {
			if netip.MustParsePrefix(prefix).Contains(address) {
				return true
			}
		}
	}
	return false
}
