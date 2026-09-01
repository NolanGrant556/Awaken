package outlook

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type UnsubscribeInfo struct {
	Available     bool   `json:"available"`
	OneClick      bool   `json:"oneClick"`
	HTTPSHost     string `json:"httpsHost,omitempty"`
	MailtoAddress string `json:"mailtoAddress,omitempty"`
	Method        string `json:"method,omitempty"`
}

type UnsubscribeResult struct {
	Success bool   `json:"success"`
	Method  string `json:"method"`
	Target  string `json:"target"`
}

type unsubscribeTargets struct {
	oneClick bool
	httpsURL *url.URL
	mailto   *url.URL
}

func (c *Client) GetUnsubscribeInfo(ctx context.Context, mailbox, messageID string) (UnsubscribeInfo, error) {
	message, err := c.getMessageWithHeaders(ctx, mailbox, messageID)
	if err != nil {
		return UnsubscribeInfo{}, err
	}
	targets := parseUnsubscribeTargets(message.InternetHeaders)
	return targets.info(), nil
}

func (c *Client) UnsubscribeMessage(ctx context.Context, mailbox, messageID, preferredMethod string) (UnsubscribeResult, error) {
	message, err := c.getMessageWithHeaders(ctx, mailbox, messageID)
	if err != nil {
		return UnsubscribeResult{}, err
	}
	targets := parseUnsubscribeTargets(message.InternetHeaders)
	preferredMethod = strings.ToLower(strings.TrimSpace(preferredMethod))
	if preferredMethod != "" && preferredMethod != "one_click" && preferredMethod != "mailto" {
		return UnsubscribeResult{}, fmt.Errorf("preferredMethod must be one_click or mailto")
	}
	if preferredMethod != "mailto" && targets.oneClick && targets.httpsURL != nil {
		if err := c.postOneClickUnsubscribe(ctx, targets.httpsURL); err != nil {
			return UnsubscribeResult{}, err
		}
		return UnsubscribeResult{Success: true, Method: "one_click", Target: targets.httpsURL.Hostname()}, nil
	}
	if targets.mailto != nil {
		address := strings.TrimSpace(targets.mailto.Opaque)
		if address == "" {
			address = strings.TrimPrefix(targets.mailto.Path, "/")
		}
		if address == "" || strings.ContainsAny(address, "\r\n") {
			return UnsubscribeResult{}, fmt.Errorf("unsubscribe mailto address is invalid")
		}
		query := targets.mailto.Query()
		subject := firstNonBlank(query.Get("subject"), "Unsubscribe")
		body := firstNonBlank(query.Get("body"), "Please unsubscribe this email address.")
		if err := c.SendMailFromMailbox(ctx, mailbox, ComposeMessage{
			Subject: subject, Body: body, To: []Recipient{{Address: address}},
		}); err != nil {
			return UnsubscribeResult{}, err
		}
		return UnsubscribeResult{Success: true, Method: "mailto", Target: address}, nil
	}
	if targets.httpsURL != nil && !targets.oneClick {
		return UnsubscribeResult{}, fmt.Errorf("the message exposes an HTTPS unsubscribe page but not an RFC 8058 one-click action")
	}
	return UnsubscribeResult{}, fmt.Errorf("the message does not expose a supported one-click or mailto unsubscribe action")
}

func parseUnsubscribeTargets(headers []InternetHeader) unsubscribeTargets {
	var listValue, postValue string
	for _, header := range headers {
		switch strings.ToLower(strings.TrimSpace(header.Name)) {
		case "list-unsubscribe":
			listValue = header.Value
		case "list-unsubscribe-post":
			postValue = header.Value
		}
	}
	targets := unsubscribeTargets{oneClick: strings.Contains(strings.ToLower(postValue), "list-unsubscribe=one-click")}
	for _, raw := range strings.Split(listValue, ",") {
		raw = strings.TrimSpace(strings.Trim(raw, "<>"))
		parsed, err := url.Parse(raw)
		if err != nil {
			continue
		}
		switch strings.ToLower(parsed.Scheme) {
		case "https":
			if targets.httpsURL == nil && parsed.Hostname() != "" && parsed.User == nil {
				targets.httpsURL = parsed
			}
		case "mailto":
			if targets.mailto == nil {
				targets.mailto = parsed
			}
		}
	}
	return targets
}

func (t unsubscribeTargets) info() UnsubscribeInfo {
	info := UnsubscribeInfo{Available: t.httpsURL != nil || t.mailto != nil, OneClick: t.oneClick && t.httpsURL != nil}
	if t.httpsURL != nil {
		info.HTTPSHost = t.httpsURL.Hostname()
	}
	if t.mailto != nil {
		info.MailtoAddress = firstNonBlank(strings.TrimSpace(t.mailto.Opaque), strings.TrimPrefix(t.mailto.Path, "/"))
	}
	switch {
	case info.OneClick:
		info.Method = "one_click"
	case info.MailtoAddress != "":
		info.Method = "mailto"
	case info.HTTPSHost != "":
		info.Method = "external_page"
	}
	return info
}

func (c *Client) postOneClickUnsubscribe(ctx context.Context, target *url.URL) error {
	client, err := c.safeUnsubscribeClient(ctx, target)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, target.String(), bytes.NewBufferString("List-Unsubscribe=One-Click"))
	if err != nil {
		return fmt.Errorf("build one-click unsubscribe request: %w", err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("one-click unsubscribe failed: %w", err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64*1024))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("one-click unsubscribe returned HTTP %d", response.StatusCode)
	}
	return nil
}

func (c *Client) safeUnsubscribeClient(ctx context.Context, target *url.URL) (*http.Client, error) {
	if c.baseURL != defaultGraphBaseURL && (target.Scheme == "http" || target.Scheme == "https") {
		return &http.Client{Timeout: 30 * time.Second, Transport: c.httpClient.Transport, CheckRedirect: rejectRedirect}, nil
	}
	if target.Scheme != "https" || target.User != nil || target.Hostname() == "" || target.Port() != "" {
		return nil, fmt.Errorf("one-click unsubscribe target must be a public HTTPS URL on the default port")
	}
	lookupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	addresses, err := net.DefaultResolver.LookupIPAddr(lookupCtx, target.Hostname())
	if err != nil || len(addresses) == 0 {
		return nil, fmt.Errorf("resolve one-click unsubscribe target")
	}
	for _, address := range addresses {
		if blockedUnsubscribeIP(address.IP) {
			return nil, fmt.Errorf("one-click unsubscribe target resolves to a private or local address")
		}
	}
	pinned := addresses[0].IP.String()
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	transport.DialContext = func(dialCtx context.Context, network, _ string) (net.Conn, error) {
		return dialer.DialContext(dialCtx, network, net.JoinHostPort(pinned, "443"))
	}
	return &http.Client{Timeout: 30 * time.Second, Transport: transport, CheckRedirect: rejectRedirect}, nil
}

func rejectRedirect(_ *http.Request, _ []*http.Request) error {
	return fmt.Errorf("redirects are not allowed for one-click unsubscribe")
}

func blockedUnsubscribeIP(ip net.IP) bool {
	if ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsMulticast() {
		return true
	}
	_, cgnat, _ := net.ParseCIDR("100.64.0.0/10")
	return cgnat.Contains(ip)
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
