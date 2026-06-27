package envguard

import (
	"errors"
	"os"
	"sort"
	"strings"
	"sync"
	"unicode"
)

var (
	ErrInvalidEnvName = errors.New("invalid environment variable name")
	ErrSensitiveEnv   = errors.New("sensitive environment variable is blocked")
)

type Manager struct{}

func NewManager() *Manager {
	return &Manager{}
}

func (m *Manager) NewSession() *Session {
	return NewSession()
}

type Session struct {
	mu              sync.RWMutex
	allowedEnv      map[string]struct{}
	credentialFiles map[string]credentialFileRegistration
}

func NewSession() *Session {
	return &Session{
		allowedEnv:      make(map[string]struct{}),
		credentialFiles: make(map[string]credentialFileRegistration),
	}
}

func (s *Session) AddAllowedEnv(name string) error {
	name = normalizeEnvName(name)
	if !isValidEnvName(name) {
		return ErrInvalidEnvName
	}
	if IsSensitiveEnvName(name) {
		return ErrSensitiveEnv
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.allowedEnv[name] = struct{}{}
	return nil
}

func (s *Session) AllowedEnv() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	names := make([]string, 0, len(s.allowedEnv))
	for name := range s.allowedEnv {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (s *Session) BuildPassthroughEnv(source map[string]string) map[string]string {
	s.mu.RLock()
	allowed := make([]string, 0, len(s.allowedEnv))
	for name := range s.allowedEnv {
		allowed = append(allowed, name)
	}
	s.mu.RUnlock()

	result := make(map[string]string)
	for _, name := range allowed {
		if IsSensitiveEnvName(name) {
			continue
		}
		if value, ok := source[name]; ok {
			result[name] = value
		}
	}
	return result
}

func (s *Session) BuildPassthroughEnvFromOS() map[string]string {
	return s.BuildPassthroughEnv(environMap(os.Environ()))
}

func IsSensitiveEnvName(name string) bool {
	name = normalizeEnvName(name)
	if name == "" {
		return false
	}

	exact := map[string]bool{
		"API_KEY":     true,
		"TOKEN":       true,
		"SECRET":      true,
		"SECRET_KEY":  true,
		"PRIVATE_KEY": true,
		"CREDENTIALS": true,
	}
	if exact[name] {
		return true
	}

	sensitiveSuffixes := []string{
		"_API_KEY",
		"_ACCESS_TOKEN",
		"_REFRESH_TOKEN",
		"_AUTH_TOKEN",
		"_TOKEN",
		"_CLIENT_SECRET",
		"_SECRET_KEY",
		"_SECRET",
		"_PRIVATE_KEY",
		"_CREDENTIALS",
	}
	for _, suffix := range sensitiveSuffixes {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	return false
}

func normalizeEnvName(name string) string {
	return strings.ToUpper(strings.TrimSpace(name))
}

func isValidEnvName(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if i == 0 && unicode.IsDigit(r) {
			return false
		}
		if r != '_' && !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func environMap(environ []string) map[string]string {
	result := make(map[string]string, len(environ))
	for _, entry := range environ {
		name, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		result[name] = value
	}
	return result
}
