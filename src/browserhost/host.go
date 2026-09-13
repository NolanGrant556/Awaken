package browserhost

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type Registration struct {
	BaseURL string `json:"base_url"`
	Token   string `json:"token"`
}

type ActionRequest struct {
	Action   string  `json:"action"`
	ThreadID string  `json:"threadId"`
	URL      string  `json:"url,omitempty"`
	Ref      string  `json:"ref,omitempty"`
	Text     string  `json:"text,omitempty"`
	Seconds  float64 `json:"seconds,omitempty"`
	Start    int     `json:"start,omitempty"`
}

type Result struct {
	OK        bool   `json:"ok"`
	Error     string `json:"error,omitempty"`
	URL       string `json:"url,omitempty"`
	Title     string `json:"title,omitempty"`
	Snapshot  string `json:"snapshot,omitempty"`
	Text      string `json:"text,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
}

var (
	mu      sync.RWMutex
	current *Registration
	client  = &http.Client{Timeout: 45 * time.Second}
)

func Set(reg *Registration) error {
	if reg == nil || strings.TrimSpace(reg.BaseURL) == "" || strings.TrimSpace(reg.Token) == "" {
		mu.Lock()
		current = nil
		mu.Unlock()
		return nil
	}
	parsed, err := url.Parse(strings.TrimSpace(reg.BaseURL))
	if err != nil {
		return fmt.Errorf("invalid browser host url")
	}
	host := parsed.Hostname()
	if parsed.Scheme != "http" || (host != "127.0.0.1" && host != "localhost") {
		return fmt.Errorf("browser host must bind to 127.0.0.1")
	}
	if ip := net.ParseIP(host); ip != nil && !ip.IsLoopback() {
		return fmt.Errorf("browser host must bind to 127.0.0.1")
	}
	copyReg := *reg
	copyReg.BaseURL = strings.TrimRight(strings.TrimSpace(reg.BaseURL), "/")
	copyReg.Token = strings.TrimSpace(reg.Token)
	mu.Lock()
	current = &copyReg
	mu.Unlock()
	return nil
}

func Ready() bool {
	mu.RLock()
	defer mu.RUnlock()
	return current != nil && current.BaseURL != "" && current.Token != ""
}

func Call(ctx context.Context, request ActionRequest) (Result, error) {
	mu.RLock()
	reg := current
	mu.RUnlock()
	if reg == nil {
		return Result{OK: false, Error: "browser_host_unavailable"}, nil
	}
	body, err := json.Marshal(request)
	if err != nil {
		return Result{}, err
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, reg.BaseURL+"/v1/actions", bytes.NewReader(body))
	if err != nil {
		return Result{}, err
	}
	httpRequest.Header.Set("Authorization", "Bearer "+reg.Token)
	httpRequest.Header.Set("Content-Type", "application/json")
	response, err := client.Do(httpRequest)
	if err != nil {
		return Result{OK: false, Error: "browser_host_unreachable"}, nil
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return Result{OK: false, Error: "browser_host_unreachable"}, nil
	}
	var result Result
	if err := json.Unmarshal(payload, &result); err != nil {
		return Result{OK: false, Error: "browser_host_invalid_response"}, nil
	}
	if result.Error == "" && !result.OK && response.StatusCode >= 400 {
		result.Error = fmt.Sprintf("browser_host_http_%d", response.StatusCode)
	}
	return result, nil
}
