package browsercontrol

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
)

type NavigationAction string

const (
	NavigationBack    NavigationAction = "back"
	NavigationForward NavigationAction = "forward"
	NavigationReload  NavigationAction = "reload"
)

var (
	ErrBrowserRuntimeDisabled   = errors.New("browser runtime is disabled")
	ErrNoActiveBrowserPage      = errors.New("browser runtime has no active page")
	ErrBrowserNavigationBlocked = errors.New("browser navigation action is unavailable")
	ErrInvalidBrowserAction     = errors.New("invalid browser navigation action")
)

type RuntimeStatus struct {
	Available bool   `json:"available"`
	Mode      string `json:"mode"`
	Reason    string `json:"reason,omitempty"`
}

type PageState struct {
	URL          string `json:"url,omitempty"`
	Title        string `json:"title,omitempty"`
	Summary      string `json:"summary,omitempty"`
	CanGoBack    bool   `json:"can_go_back,omitempty"`
	CanGoForward bool   `json:"can_go_forward,omitempty"`
}

type Runtime interface {
	Status() RuntimeStatus
	OpenURL(ctx context.Context, rawURL string) (PageState, error)
	CurrentPage(ctx context.Context) (PageState, error)
	Navigate(ctx context.Context, action NavigationAction) (PageState, error)
}

type disabledRuntime struct {
	reason string
}

func NewDisabledRuntime(reason string) Runtime {
	trimmed := strings.TrimSpace(reason)
	if trimmed == "" {
		trimmed = "no browser runtime is attached to this run"
	}
	return disabledRuntime{reason: trimmed}
}

func (r disabledRuntime) Status() RuntimeStatus {
	return RuntimeStatus{
		Available: false,
		Mode:      "disabled",
		Reason:    r.reason,
	}
}

func (r disabledRuntime) OpenURL(ctx context.Context, rawURL string) (PageState, error) {
	if err := ctx.Err(); err != nil {
		return PageState{}, err
	}
	return PageState{}, ErrBrowserRuntimeDisabled
}

func (r disabledRuntime) CurrentPage(ctx context.Context) (PageState, error) {
	if err := ctx.Err(); err != nil {
		return PageState{}, err
	}
	return PageState{}, ErrBrowserRuntimeDisabled
}

func (r disabledRuntime) Navigate(ctx context.Context, action NavigationAction) (PageState, error) {
	if err := ctx.Err(); err != nil {
		return PageState{}, err
	}
	return PageState{}, ErrBrowserRuntimeDisabled
}

type mockRuntime struct {
	mu      sync.Mutex
	history []mockPage
	index   int
}

type mockPage struct {
	URL     string
	Title   string
	Summary string
}

func NewMockRuntime() Runtime {
	return &mockRuntime{index: -1}
}

func (r *mockRuntime) Status() RuntimeStatus {
	return RuntimeStatus{
		Available: true,
		Mode:      "mock",
		Reason:    "using mock browser runtime; no real browser automation is attached",
	}
}

func (r *mockRuntime) OpenURL(ctx context.Context, rawURL string) (PageState, error) {
	if err := ctx.Err(); err != nil {
		return PageState{}, err
	}

	normalized, err := NormalizeBrowserURL(rawURL)
	if err != nil {
		return PageState{}, err
	}
	page := mockPageFromURL(normalized)

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.index >= 0 && r.index < len(r.history)-1 {
		r.history = append([]mockPage(nil), r.history[:r.index+1]...)
	}
	r.history = append(r.history, page)
	r.index = len(r.history) - 1
	return r.currentLocked(), nil
}

func (r *mockRuntime) CurrentPage(ctx context.Context) (PageState, error) {
	if err := ctx.Err(); err != nil {
		return PageState{}, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.index < 0 || r.index >= len(r.history) {
		return PageState{}, ErrNoActiveBrowserPage
	}
	return r.currentLocked(), nil
}

func (r *mockRuntime) Navigate(ctx context.Context, action NavigationAction) (PageState, error) {
	if err := ctx.Err(); err != nil {
		return PageState{}, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.index < 0 || r.index >= len(r.history) {
		return PageState{}, ErrNoActiveBrowserPage
	}

	switch action {
	case NavigationBack:
		if r.index == 0 {
			return PageState{}, ErrBrowserNavigationBlocked
		}
		r.index--
	case NavigationForward:
		if r.index >= len(r.history)-1 {
			return PageState{}, ErrBrowserNavigationBlocked
		}
		r.index++
	case NavigationReload:
	default:
		return PageState{}, ErrInvalidBrowserAction
	}

	return r.currentLocked(), nil
}

func (r *mockRuntime) currentLocked() PageState {
	page := r.history[r.index]
	return PageState{
		URL:          page.URL,
		Title:        page.Title,
		Summary:      page.Summary,
		CanGoBack:    r.index > 0,
		CanGoForward: r.index >= 0 && r.index < len(r.history)-1,
	}
}

func mockPageFromURL(rawURL string) mockPage {
	parsed, _ := url.Parse(rawURL)
	target := parsed.Host
	if strings.TrimSpace(target) == "" {
		target = rawURL
	}
	return mockPage{
		URL:     rawURL,
		Title:   fmt.Sprintf("Mock page for %s", target),
		Summary: fmt.Sprintf("Mock browser opened %s. This is a local test double and does not execute real browser automation.", rawURL),
	}
}

type runtimeHolder struct {
	mu      sync.RWMutex
	runtime Runtime
}

var browserRuntime = runtimeHolder{runtime: defaultRuntimeFromEnv()}

func defaultRuntimeFromEnv() Runtime {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("AWAKEN_BROWSER_RUNTIME"))) {
	case "", "disabled":
		return NewDisabledRuntime("no browser runtime is attached to this run")
	case "mock":
		return NewMockRuntime()
	default:
		return NewDisabledRuntime(fmt.Sprintf("unsupported browser runtime mode %q", os.Getenv("AWAKEN_BROWSER_RUNTIME")))
	}
}

func CurrentRuntime() Runtime {
	browserRuntime.mu.RLock()
	current := browserRuntime.runtime
	browserRuntime.mu.RUnlock()
	if current == nil {
		return NewDisabledRuntime("no browser runtime is attached to this run")
	}
	return current
}

func SetRuntime(runtime Runtime) {
	if runtime == nil {
		runtime = NewDisabledRuntime("no browser runtime is attached to this run")
	}
	browserRuntime.mu.Lock()
	browserRuntime.runtime = runtime
	browserRuntime.mu.Unlock()
}

func NormalizeBrowserURL(rawURL string) (string, error) {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return "", fmt.Errorf("缺少必需参数: url")
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("无效 URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("仅支持 http 或 https URL")
	}
	if strings.TrimSpace(parsed.Host) == "" {
		return "", fmt.Errorf("URL 缺少 host")
	}
	return parsed.String(), nil
}

func ParseNavigationAction(value string) (NavigationAction, error) {
	action := NavigationAction(strings.ToLower(strings.TrimSpace(value)))
	switch action {
	case NavigationBack, NavigationForward, NavigationReload:
		return action, nil
	default:
		return "", fmt.Errorf("%w: %s", ErrInvalidBrowserAction, value)
	}
}
