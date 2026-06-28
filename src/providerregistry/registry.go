package providerregistry

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
)

var (
	ErrProviderExists   = errors.New("provider already exists")
	ErrProviderNotFound = errors.New("provider not found")
	ErrModelNotFound    = errors.New("model not found")
)

type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider
	order     []string
}

func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[string]Provider),
	}
}

func (r *Registry) Add(provider Provider) (ProviderSnapshot, error) {
	normalized, err := normalizeProvider(provider)
	if err != nil {
		return ProviderSnapshot{}, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.providers[normalized.ID]; ok {
		return ProviderSnapshot{}, fmt.Errorf("%w: %s", ErrProviderExists, normalized.ID)
	}

	r.providers[normalized.ID] = normalized
	r.order = append(r.order, normalized.ID)
	return snapshotProvider(normalized), nil
}

func (r *Registry) Update(provider Provider) (ProviderSnapshot, error) {
	normalized, err := normalizeProvider(provider)
	if err != nil {
		return ProviderSnapshot{}, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.providers[normalized.ID]; !ok {
		return ProviderSnapshot{}, fmt.Errorf("%w: %s", ErrProviderNotFound, normalized.ID)
	}

	r.providers[normalized.ID] = normalized
	return snapshotProvider(normalized), nil
}

func (r *Registry) Remove(providerID string) error {
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		return errors.New("provider id is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.providers[providerID]; !ok {
		return fmt.Errorf("%w: %s", ErrProviderNotFound, providerID)
	}

	delete(r.providers, providerID)
	r.order = slices.DeleteFunc(r.order, func(id string) bool {
		return id == providerID
	})
	return nil
}

func (r *Registry) List() []ProviderSnapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()

	snapshots := make([]ProviderSnapshot, 0, len(r.order))
	for _, id := range r.order {
		provider, ok := r.providers[id]
		if !ok {
			continue
		}
		snapshots = append(snapshots, snapshotProvider(provider))
	}
	return snapshots
}

func (r *Registry) Get(providerID string) (ProviderSnapshot, bool) {
	providerID = strings.TrimSpace(providerID)

	r.mu.RLock()
	defer r.mu.RUnlock()

	provider, ok := r.providers[providerID]
	if !ok {
		return ProviderSnapshot{}, false
	}
	return snapshotProvider(provider), true
}

func (r *Registry) EnableProvider(providerID string, enabled bool) (ProviderSnapshot, error) {
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		return ProviderSnapshot{}, errors.New("provider id is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	provider, ok := r.providers[providerID]
	if !ok {
		return ProviderSnapshot{}, fmt.Errorf("%w: %s", ErrProviderNotFound, providerID)
	}
	provider.Enabled = enabled
	r.providers[providerID] = provider
	return snapshotProvider(provider), nil
}

func (r *Registry) EnableModel(providerID, modelID string, enabled bool) (ProviderSnapshot, error) {
	providerID = strings.TrimSpace(providerID)
	modelID = strings.TrimSpace(modelID)
	if providerID == "" {
		return ProviderSnapshot{}, errors.New("provider id is required")
	}
	if modelID == "" {
		return ProviderSnapshot{}, errors.New("model id is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	provider, ok := r.providers[providerID]
	if !ok {
		return ProviderSnapshot{}, fmt.Errorf("%w: %s", ErrProviderNotFound, providerID)
	}

	modelIndex := slices.IndexFunc(provider.Models, func(model Model) bool {
		return model.ID == modelID
	})
	if modelIndex < 0 {
		return ProviderSnapshot{}, fmt.Errorf("%w: %s", ErrModelNotFound, modelID)
	}

	provider.Models[modelIndex].Enabled = enabled
	r.providers[providerID] = provider
	return snapshotProvider(provider), nil
}

func (r *Registry) SetHealth(providerID, modelID string, health HealthCheck) (ProviderSnapshot, error) {
	providerID = strings.TrimSpace(providerID)
	modelID = strings.TrimSpace(modelID)
	if providerID == "" {
		return ProviderSnapshot{}, errors.New("provider id is required")
	}
	if modelID == "" {
		return ProviderSnapshot{}, errors.New("model id is required")
	}

	normalizedHealth, err := normalizeHealth(health)
	if err != nil {
		return ProviderSnapshot{}, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	provider, ok := r.providers[providerID]
	if !ok {
		return ProviderSnapshot{}, fmt.Errorf("%w: %s", ErrProviderNotFound, providerID)
	}
	if !slices.ContainsFunc(provider.Models, func(model Model) bool { return model.ID == modelID }) {
		return ProviderSnapshot{}, fmt.Errorf("%w: %s", ErrModelNotFound, modelID)
	}
	if provider.Health == nil {
		provider.Health = make(map[string]HealthCheck)
	}
	provider.Health[modelID] = normalizedHealth
	r.providers[providerID] = provider
	return snapshotProvider(provider), nil
}

func (r *Registry) DiscoveryPlan(providerID string) (DiscoveryPlan, error) {
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		return DiscoveryPlan{}, errors.New("provider id is required")
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	provider, ok := r.providers[providerID]
	if !ok {
		return DiscoveryPlan{}, fmt.Errorf("%w: %s", ErrProviderNotFound, providerID)
	}
	return discoveryPlanFor(provider), nil
}

func NormalizeProvider(provider Provider) (Provider, error) {
	return normalizeProvider(provider)
}

func ValidateProvider(provider Provider) error {
	_, err := normalizeProvider(provider)
	return err
}

func normalizeProvider(provider Provider) (Provider, error) {
	provider.ID = strings.TrimSpace(provider.ID)
	provider.Name = strings.TrimSpace(provider.Name)
	if provider.ID == "" {
		return Provider{}, errors.New("provider id is required")
	}
	if provider.Name == "" {
		return Provider{}, errors.New("provider name is required")
	}

	if provider.Kind == "" {
		provider.Kind = ProviderKindCustom
	}
	if !isValidProviderKind(provider.Kind) {
		return Provider{}, fmt.Errorf("invalid provider kind: %s", provider.Kind)
	}

	if provider.Protocol == "" {
		provider.Protocol = defaultProtocol(provider.Kind)
	}
	if !isValidProtocol(provider.Protocol) {
		return Provider{}, fmt.Errorf("invalid protocol: %s", provider.Protocol)
	}

	if provider.AuthScheme == "" {
		provider.AuthScheme = defaultAuthScheme(provider.Kind)
	}
	if !isValidAuthScheme(provider.AuthScheme) {
		return Provider{}, fmt.Errorf("invalid auth scheme: %s", provider.AuthScheme)
	}

	normalizedBaseURL, err := normalizeBaseURL(provider.BaseURL, requiresBaseURL(provider.Kind))
	if err != nil {
		return Provider{}, err
	}
	provider.BaseURL = normalizedBaseURL

	provider.Credential = normalizeCredentialRef(provider.Credential, provider.AuthScheme)

	models, err := normalizeModels(provider.Models, provider.Protocol)
	if err != nil {
		return Provider{}, err
	}
	provider.Models = models

	provider.Health = normalizeHealthMap(provider.Health)
	provider.Annotations = cloneStringMap(provider.Annotations)
	return provider, nil
}

func normalizeModels(models []Model, providerProtocol Protocol) ([]Model, error) {
	if len(models) == 0 {
		return nil, errors.New("at least one model is required")
	}

	seen := make(map[string]struct{}, len(models))
	normalized := make([]Model, 0, len(models))
	for _, model := range models {
		model.ID = strings.TrimSpace(model.ID)
		model.Name = strings.TrimSpace(model.Name)
		if model.ID == "" {
			return nil, errors.New("model id is required")
		}
		if _, ok := seen[model.ID]; ok {
			continue
		}
		seen[model.ID] = struct{}{}

		if model.Protocol == "" {
			model.Protocol = providerProtocol
		}
		if !isValidProtocol(model.Protocol) {
			return nil, fmt.Errorf("invalid model protocol for %s: %s", model.ID, model.Protocol)
		}
		model.Capabilities = normalizeCapabilities(model.Capabilities)
		normalized = append(normalized, model)
	}
	return normalized, nil
}

func normalizeCapabilities(capabilities []ModelCapability) []ModelCapability {
	if len(capabilities) == 0 {
		return nil
	}

	seen := make(map[ModelCapability]struct{}, len(capabilities))
	normalized := make([]ModelCapability, 0, len(capabilities))
	for _, capability := range capabilities {
		if capability == "" {
			continue
		}
		if _, ok := seen[capability]; ok {
			continue
		}
		seen[capability] = struct{}{}
		normalized = append(normalized, capability)
	}
	return normalized
}

func normalizeHealth(health HealthCheck) (HealthCheck, error) {
	if health.Status == "" {
		health.Status = HealthUnknown
	}
	if !isValidHealthStatus(health.Status) {
		return HealthCheck{}, fmt.Errorf("invalid health status: %s", health.Status)
	}
	if health.LatencyMillis < 0 {
		return HealthCheck{}, errors.New("health latency cannot be negative")
	}
	if health.CheckedAt.IsZero() {
		health.CheckedAt = time.Now()
	}
	health.Message = redactSecretLikeText(strings.TrimSpace(health.Message))
	return health, nil
}

func normalizeHealthMap(health map[string]HealthCheck) map[string]HealthCheck {
	if len(health) == 0 {
		return nil
	}

	normalized := make(map[string]HealthCheck, len(health))
	for modelID, check := range health {
		modelID = strings.TrimSpace(modelID)
		if modelID == "" {
			continue
		}
		cleaned, err := normalizeHealth(check)
		if err != nil {
			continue
		}
		normalized[modelID] = cleaned
	}
	return normalized
}

func normalizeCredentialRef(ref CredentialRef, scheme AuthScheme) CredentialRef {
	ref.Name = strings.TrimSpace(ref.Name)
	if ref.Source == "" {
		ref.Source = defaultCredentialSource(scheme)
	}
	if ref.Source == CredentialSourceNone || scheme == AuthSchemeNone {
		return CredentialRef{Source: CredentialSourceNone}
	}
	if looksSecretLike(ref.Name) {
		ref.Name = "<redacted>"
		ref.Present = true
	}
	return ref
}

func normalizeBaseURL(raw string, required bool) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		if required {
			return "", errors.New("base url is required")
		}
		return "", nil
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid base url: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("base url must be absolute")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("unsupported base url scheme: %s", parsed.Scheme)
	}
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func snapshotProvider(provider Provider) ProviderSnapshot {
	return ProviderSnapshot{
		ID:          provider.ID,
		Name:        provider.Name,
		Kind:        provider.Kind,
		Protocol:    provider.Protocol,
		AuthScheme:  provider.AuthScheme,
		Credential:  normalizeCredentialRef(provider.Credential, provider.AuthScheme),
		BaseURL:     provider.BaseURL,
		Models:      cloneModels(provider.Models),
		Health:      cloneHealthMap(provider.Health),
		Enabled:     provider.Enabled,
		Annotations: cloneStringMap(provider.Annotations),
	}
}

func cloneModels(models []Model) []Model {
	if len(models) == 0 {
		return nil
	}
	cloned := make([]Model, len(models))
	for i, model := range models {
		cloned[i] = model
		cloned[i].Capabilities = slices.Clone(model.Capabilities)
	}
	return cloned
}

func cloneHealthMap(health map[string]HealthCheck) map[string]HealthCheck {
	if len(health) == 0 {
		return nil
	}
	cloned := make(map[string]HealthCheck, len(health))
	for modelID, check := range health {
		check.Message = redactSecretLikeText(check.Message)
		cloned[modelID] = check
	}
	return cloned
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func discoveryPlanFor(provider Provider) DiscoveryPlan {
	path, method := discoveryPathAndMethod(provider)
	return DiscoveryPlan{
		ProviderID:       provider.ID,
		ProviderKind:     provider.Kind,
		Method:           method,
		Path:             path,
		AuthHeaderStyle:  authHeaderStyle(provider.AuthScheme),
		ExpectedProtocol: provider.Protocol,
		BaseURL:          provider.BaseURL,
	}
}

func discoveryPathAndMethod(provider Provider) (string, string) {
	switch provider.Kind {
	case ProviderKindAnthropic:
		return pathWithSuffix(provider.BaseURL, "/v1/models", "/v1"), "GET"
	case ProviderKindGemini:
		return pathWithSuffix(provider.BaseURL, "/v1beta/models", "/v1beta"), "GET"
	case ProviderKindBedrock:
		return "/foundation-models", "GET"
	case ProviderKindVertex:
		return "/v1/projects/{project}/locations/{location}/publishers/google/models", "GET"
	default:
		return pathWithSuffix(provider.BaseURL, "/models", ""), "GET"
	}
}

func pathWithSuffix(baseURL, suffix, existingPrefix string) string {
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Path == "" {
		return suffix
	}
	basePath := strings.TrimRight(parsed.Path, "/")
	if existingPrefix != "" && strings.HasSuffix(basePath, existingPrefix) {
		return basePath + ensureLeadingSlash(strings.TrimPrefix(suffix, existingPrefix))
	}
	return basePath + suffix
}

func ensureLeadingSlash(path string) string {
	if path == "" || strings.HasPrefix(path, "/") {
		return path
	}
	return "/" + path
}

func authHeaderStyle(scheme AuthScheme) string {
	switch scheme {
	case AuthSchemeBearer:
		return "Authorization: Bearer <redacted>"
	case AuthSchemeXAPIKey:
		return "x-api-key: <redacted>"
	case AuthSchemeGeminiAPIKey:
		return "x-goog-api-key: <redacted>"
	case AuthSchemeAWSSigV4:
		return "AWS SigV4"
	case AuthSchemeGoogleADC:
		return "Authorization: Bearer <google-adc-token>"
	case AuthSchemeNone:
		return "none"
	default:
		return "custom"
	}
}

func isValidProviderKind(kind ProviderKind) bool {
	switch kind {
	case ProviderKindOpenAICompatible, ProviderKindAnthropic, ProviderKindGemini, ProviderKindBedrock, ProviderKindVertex, ProviderKindCustom:
		return true
	default:
		return false
	}
}

func isValidProtocol(protocol Protocol) bool {
	switch protocol {
	case ProtocolOpenAIChat, ProtocolAnthropicMessages, ProtocolGeminiGenerateContent, ProtocolBedrockConverse, ProtocolVertexGemini, ProtocolCustom:
		return true
	default:
		return false
	}
}

func isValidAuthScheme(scheme AuthScheme) bool {
	switch scheme {
	case AuthSchemeNone, AuthSchemeBearer, AuthSchemeXAPIKey, AuthSchemeGeminiAPIKey, AuthSchemeAWSSigV4, AuthSchemeGoogleADC, AuthSchemeCustom:
		return true
	default:
		return false
	}
}

func isValidHealthStatus(status HealthStatus) bool {
	switch status {
	case HealthUnknown, HealthHealthy, HealthUnhealthy:
		return true
	default:
		return false
	}
}

func defaultProtocol(kind ProviderKind) Protocol {
	switch kind {
	case ProviderKindOpenAICompatible:
		return ProtocolOpenAIChat
	case ProviderKindAnthropic:
		return ProtocolAnthropicMessages
	case ProviderKindGemini:
		return ProtocolGeminiGenerateContent
	case ProviderKindBedrock:
		return ProtocolBedrockConverse
	case ProviderKindVertex:
		return ProtocolVertexGemini
	default:
		return ProtocolCustom
	}
}

func defaultAuthScheme(kind ProviderKind) AuthScheme {
	switch kind {
	case ProviderKindOpenAICompatible:
		return AuthSchemeBearer
	case ProviderKindAnthropic:
		return AuthSchemeXAPIKey
	case ProviderKindGemini:
		return AuthSchemeGeminiAPIKey
	case ProviderKindBedrock:
		return AuthSchemeAWSSigV4
	case ProviderKindVertex:
		return AuthSchemeGoogleADC
	default:
		return AuthSchemeCustom
	}
}

func defaultCredentialSource(scheme AuthScheme) CredentialSource {
	switch scheme {
	case AuthSchemeAWSSigV4:
		return CredentialSourceAWSProfile
	case AuthSchemeGoogleADC:
		return CredentialSourceGoogleADC
	case AuthSchemeNone:
		return CredentialSourceNone
	default:
		return CredentialSourceEnv
	}
}

func requiresBaseURL(kind ProviderKind) bool {
	switch kind {
	case ProviderKindBedrock, ProviderKindVertex:
		return false
	default:
		return true
	}
}

var secretLikePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(authorization:\s*(?:bearer\s+)?)[^\s,;]+`),
	regexp.MustCompile(`(?i)(x-api-key:\s*)[^\s,;]+`),
	regexp.MustCompile(`(?i)(api[_-]?key\s*[=:]\s*)[^\s,;&]+`),
	regexp.MustCompile(`(?i)(secret[_-]?access[_-]?key\s*[=:]\s*)[^\s,;&]+`),
	regexp.MustCompile(`sk-[A-Za-z0-9_-]{8,}`),
}

func redactSecretLikeText(value string) string {
	for _, pattern := range secretLikePatterns {
		value = pattern.ReplaceAllStringFunc(value, func(match string) string {
			if strings.HasPrefix(strings.ToLower(match), "sk-") {
				return "<redacted>"
			}
			prefix, _, ok := strings.Cut(match, ":")
			if ok && strings.Contains(strings.ToLower(prefix), "authorization") {
				return prefix + ": Bearer <redacted>"
			}
			for _, separator := range []string{":", "=", " "} {
				if index := strings.LastIndex(match, separator); index >= 0 {
					return match[:index+1] + "<redacted>"
				}
			}
			return "<redacted>"
		})
	}
	return value
}

func looksSecretLike(value string) bool {
	if value == "" {
		return false
	}
	redacted := redactSecretLikeText(value)
	return redacted != value || strings.Contains(strings.ToLower(value), "bearer ")
}
