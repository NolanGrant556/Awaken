package deeplink

import (
	"fmt"
	"net/url"
	"strings"
	"unicode/utf8"
)

const (
	DefaultProtocol       = "awaken-cli"
	DefaultMaxQueryLength = 5000
	DefaultMaxCWDLength   = 4096
)

type Action struct {
	Query string
	CWD   string
	Repo  string
}

type Options struct {
	Protocol       string
	MaxQueryLength int
	MaxCWDLength   int
}

func Parse(uri string) (Action, error) {
	return ParseWithOptions(uri, Options{})
}

func ParseWithOptions(uri string, options Options) (Action, error) {
	options, err := normalizeOptions(options)
	if err != nil {
		return Action{}, err
	}
	if containsASCIIControl(uri) {
		return Action{}, fmt.Errorf("deep link URI contains disallowed control characters")
	}

	normalized, err := normalizeURI(uri, options.Protocol)
	if err != nil {
		return Action{}, err
	}

	parsed, err := url.Parse(normalized)
	if err != nil {
		return Action{}, fmt.Errorf("invalid deep link URL: %q", uri)
	}
	if !strings.EqualFold(parsed.Scheme, options.Protocol) {
		return Action{}, fmt.Errorf("invalid deep link: expected %s scheme, got %q", options.Protocol, parsed.Scheme)
	}
	if parsed.User != nil || parsed.Host != "open" || parsed.Path != "" {
		return Action{}, fmt.Errorf("unknown deep link action: %q", parsed.Host)
	}

	queryValues, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		return Action{}, fmt.Errorf("invalid deep link query: %w", err)
	}

	var action Action
	if rawQuery, ok := firstQueryValue(queryValues, "q"); ok {
		if containsASCIIControl(rawQuery) {
			return Action{}, fmt.Errorf("deep link query contains disallowed control characters")
		}
		query := strings.TrimSpace(rawQuery)
		if query != "" {
			query = strings.TrimSpace(sanitizeHiddenUnicode(query))
			if query != "" {
				if utf8.RuneCountInString(query) > options.MaxQueryLength {
					return Action{}, fmt.Errorf("deep link query exceeds %d characters", options.MaxQueryLength)
				}
				action.Query = query
			}
		}
	}

	if cwd, ok := firstQueryValue(queryValues, "cwd"); ok {
		if containsASCIIControl(cwd) {
			return Action{}, fmt.Errorf("deep link cwd contains disallowed control characters")
		}
		if len(cwd) > options.MaxCWDLength {
			return Action{}, fmt.Errorf("deep link cwd exceeds %d characters", options.MaxCWDLength)
		}
		if !isAbsoluteCWD(cwd) {
			return Action{}, fmt.Errorf("invalid cwd in deep link: must be an absolute path, got %q", cwd)
		}
		action.CWD = cwd
	}

	if repo, ok := firstQueryValue(queryValues, "repo"); ok {
		if containsASCIIControl(repo) {
			return Action{}, fmt.Errorf("deep link repo contains disallowed control characters")
		}
		if !isValidRepoSlug(repo) {
			return Action{}, fmt.Errorf("invalid repo in deep link: expected owner/repo, got %q", repo)
		}
		action.Repo = repo
	}

	return action, nil
}

func Build(action Action) string {
	params := make([]string, 0, 3)
	if query := strings.TrimSpace(sanitizeHiddenUnicode(action.Query)); query != "" {
		params = append(params, "q="+url.QueryEscape(query))
	}
	if action.CWD != "" {
		params = append(params, "cwd="+url.QueryEscape(action.CWD))
	}
	if action.Repo != "" {
		params = append(params, "repo="+url.QueryEscape(action.Repo))
	}

	uri := DefaultProtocol + "://open"
	if len(params) > 0 {
		uri += "?" + strings.Join(params, "&")
	}
	return uri
}

func ResolveCWD(action Action, repoPaths []string, exists func(string) bool, home string) (string, bool) {
	if action.CWD != "" {
		return action.CWD, false
	}
	if action.Repo != "" && exists != nil {
		for _, path := range repoPaths {
			if exists(path) {
				return path, true
			}
		}
	}
	return home, false
}

func TerminalPreferenceFromTermProgram(platform, termProgram string) (string, bool) {
	if strings.ToLower(strings.TrimSpace(platform)) != "darwin" {
		return "", false
	}

	switch strings.ToLower(strings.TrimSpace(termProgram)) {
	case "iterm", "iterm.app":
		return "iTerm", true
	case "ghostty":
		return "Ghostty", true
	case "kitty":
		return "kitty", true
	case "alacritty":
		return "Alacritty", true
	case "wezterm":
		return "WezTerm", true
	case "apple_terminal":
		return "Terminal", true
	default:
		return "", false
	}
}

func normalizeOptions(options Options) (Options, error) {
	if options.Protocol == "" {
		options.Protocol = DefaultProtocol
	} else {
		options.Protocol = strings.TrimSpace(options.Protocol)
	}
	if !isValidProtocol(options.Protocol) {
		return Options{}, fmt.Errorf("invalid deep link protocol: %q", options.Protocol)
	}
	if options.MaxQueryLength <= 0 {
		options.MaxQueryLength = DefaultMaxQueryLength
	}
	if options.MaxCWDLength <= 0 {
		options.MaxCWDLength = DefaultMaxCWDLength
	}
	return options, nil
}

func normalizeURI(uri, protocol string) (string, error) {
	colon := strings.IndexByte(uri, ':')
	if colon <= 0 {
		return "", fmt.Errorf("invalid deep link: expected %s scheme, got %q", protocol, uri)
	}

	scheme := uri[:colon]
	if !strings.EqualFold(scheme, protocol) {
		return "", fmt.Errorf("invalid deep link: expected %s scheme, got %q", protocol, scheme)
	}

	rest := uri[colon+1:]
	if strings.HasPrefix(rest, "//") {
		return uri, nil
	}
	return scheme + "://" + rest, nil
}

func firstQueryValue(values url.Values, key string) (string, bool) {
	items, ok := values[key]
	if !ok {
		return "", false
	}
	if len(items) == 0 {
		return "", true
	}
	return items[0], true
}

func containsASCIIControl(value string) bool {
	for i := 0; i < len(value); i++ {
		if value[i] <= 0x1f || value[i] == 0x7f {
			return true
		}
	}
	return false
}

func sanitizeHiddenUnicode(value string) string {
	var builder strings.Builder
	for _, r := range value {
		if isHiddenUnicode(r) {
			continue
		}
		builder.WriteRune(r)
	}
	return builder.String()
}

func isHiddenUnicode(r rune) bool {
	switch {
	case r == 0x00ad || r == 0x034f || r == 0x061c || r == 0x180e || r == 0xfeff:
		return true
	case r >= 0x200b && r <= 0x200f:
		return true
	case r >= 0x202a && r <= 0x202e:
		return true
	case r >= 0x2060 && r <= 0x206f:
		return true
	case r >= 0xe0000 && r <= 0xe007f:
		return true
	default:
		return false
	}
}

func isAbsoluteCWD(cwd string) bool {
	if strings.HasPrefix(cwd, "/") {
		return true
	}
	return len(cwd) >= 3 &&
		isASCIIAlpha(cwd[0]) &&
		cwd[1] == ':' &&
		(cwd[2] == '/' || cwd[2] == '\\')
}

func isValidRepoSlug(repo string) bool {
	if strings.Count(repo, "/") != 1 {
		return false
	}

	parts := strings.Split(repo, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return false
		}
		for i := 0; i < len(part); i++ {
			if !isRepoSlugByte(part[i]) {
				return false
			}
		}
	}
	return true
}

func isRepoSlugByte(b byte) bool {
	return isASCIIAlpha(b) ||
		(b >= '0' && b <= '9') ||
		b == '_' ||
		b == '.' ||
		b == '-'
}

func isValidProtocol(protocol string) bool {
	if protocol == "" || !isASCIIAlpha(protocol[0]) {
		return false
	}
	for i := 1; i < len(protocol); i++ {
		b := protocol[i]
		if !isASCIIAlpha(b) && (b < '0' || b > '9') && b != '+' && b != '-' && b != '.' {
			return false
		}
	}
	return true
}

func isASCIIAlpha(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}
