package bridgetoken

import (
	"os"
	"sync"
)

const defaultTrustedDeviceEnvVar = "CLAUDE_TRUSTED_DEVICE_TOKEN"

type TrustedDeviceTokenSourceConfig struct {
	EnvToken       func() string
	GateEnabled    func() bool
	GetStoredToken func() (string, error)
	SetStoredToken func(string) error
}

type TrustedDeviceTokenSource struct {
	envToken       func() string
	gateEnabled    func() bool
	getStoredToken func() (string, error)
	setStoredToken func(string) error

	mu          sync.Mutex
	cacheLoaded bool
	cachedToken string
}

func NewTrustedDeviceTokenSource(config TrustedDeviceTokenSourceConfig) *TrustedDeviceTokenSource {
	envToken := config.EnvToken
	if envToken == nil {
		envToken = func() string {
			return os.Getenv(defaultTrustedDeviceEnvVar)
		}
	}
	gateEnabled := config.GateEnabled
	if gateEnabled == nil {
		gateEnabled = func() bool { return false }
	}
	getStoredToken := config.GetStoredToken
	if getStoredToken == nil {
		getStoredToken = func() (string, error) { return "", nil }
	}

	return &TrustedDeviceTokenSource{
		envToken:       envToken,
		gateEnabled:    gateEnabled,
		getStoredToken: getStoredToken,
		setStoredToken: config.SetStoredToken,
	}
}

func (s *TrustedDeviceTokenSource) Token() string {
	if !s.gateEnabled() {
		return ""
	}
	if token := s.envToken(); token != "" {
		return token
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cacheLoaded {
		return s.cachedToken
	}

	token, err := s.getStoredToken()
	if err != nil {
		return ""
	}
	s.cachedToken = token
	s.cacheLoaded = true
	return token
}

func (s *TrustedDeviceTokenSource) ClearCache() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cacheLoaded = false
	s.cachedToken = ""
}

func (s *TrustedDeviceTokenSource) ClearToken() error {
	s.ClearCache()
	if s.setStoredToken == nil {
		return nil
	}
	return s.setStoredToken("")
}
