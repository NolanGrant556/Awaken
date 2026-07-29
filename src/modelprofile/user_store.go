package modelprofile

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

var (
	ErrNoUsableProfile    = errors.New("no usable model profile is configured")
	ErrProfileUnavailable = errors.New("model profile is unavailable")

	profileIDPattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	envKeyPattern     = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9._-]*$`)
	endpointIDPattern = regexp.MustCompile(`^ep-[A-Za-z0-9_-]+$`)
)

// ProfileSummary is the non-sensitive model selector data exposed to a local
// ACP client. ID is an opaque reference to an Awaken-owned profile.
type ProfileSummary struct {
	ID          string `json:"id"`
	ProviderID  string `json:"provider_id"`
	DisplayName string `json:"display_name"`
	ModelID     string `json:"model_id"`
	IsDefault   bool   `json:"is_default"`
}

// ResolvedProfile is a complete run profile resolved from Awaken-owned local
// configuration. APIKey must never be serialized to an ACP client or log.
type ResolvedProfile struct {
	ProfileSummary
	BaseURL               string
	Protocol              string
	AuthScheme            string
	APIKey                string
	Temperature           float64
	MaxTokens             int
	ContextWindow         int
	CredentialRevision    uint64
	SupportsVision        bool
	SupportsNativePDF     bool
	SupportsVideo         bool
	SupportsTools         string
	ReasoningEffort       string
	ReasoningEffortLevels []string `json:"-"`
	CapabilityModelID     string   `json:"-"`
	EndpointSelector      string   `json:"-"`
	ModelSelectorKind     string   `json:"-"`
}

type ProfileCatalog struct {
	Profiles         []ProfileSummary `json:"profiles"`
	DefaultProfileID string           `json:"default_profile_id"`
}

// UserStore is the read-only Go consumer of the model profile files owned by
// Desktop Main. It exists so a headless Core can resolve an already configured
// profile without accepting model credentials from an ACP client.
type UserStore struct {
	root string
}

func NewUserStore(root string) *UserStore {
	return &UserStore{root: strings.TrimSpace(root)}
}

func (s *UserStore) ListReady() (ProfileCatalog, error) {
	profiles, err := s.loadReady()
	if err != nil {
		return ProfileCatalog{}, err
	}
	summaries := make([]ProfileSummary, 0, len(profiles))
	defaultID := ""
	for _, profile := range profiles {
		summaries = append(summaries, profile.ProfileSummary)
		if defaultID == "" && profile.IsDefault {
			defaultID = profile.ID
		}
	}
	if defaultID == "" && len(summaries) > 0 {
		defaultID = summaries[0].ID
	}
	return ProfileCatalog{Profiles: summaries, DefaultProfileID: defaultID}, nil
}

// Resolve returns the explicitly selected ready profile. An empty ID selects
// the ready default profile, or the first ready configured profile.
func (s *UserStore) Resolve(profileID string) (ResolvedProfile, error) {
	profiles, err := s.loadReady()
	if err != nil {
		return ResolvedProfile{}, err
	}
	requested := strings.TrimSpace(profileID)
	if requested != "" {
		for _, profile := range profiles {
			if profile.ID == requested {
				return profile, nil
			}
		}
		return ResolvedProfile{}, fmt.Errorf("%w: %s", ErrProfileUnavailable, requested)
	}
	for _, profile := range profiles {
		if profile.IsDefault {
			return profile, nil
		}
	}
	if len(profiles) == 0 {
		return ResolvedProfile{}, ErrNoUsableProfile
	}
	return profiles[0], nil
}

type storedProfileFile struct {
	Version  int             `json:"version"`
	Profiles []storedProfile `json:"profiles"`
}

type storedProfile struct {
	ID                  string                    `json:"id"`
	ProviderID          string                    `json:"providerId"`
	DisplayName         string                    `json:"displayName"`
	ModelID             string                    `json:"modelId"`
	BaseURL             string                    `json:"baseURL"`
	Protocol            string                    `json:"protocol"`
	AuthScheme          string                    `json:"authScheme"`
	Enabled             bool                      `json:"enabled"`
	IsDefault           bool                      `json:"isDefault"`
	Advanced            storedProfileAdvanced     `json:"advanced"`
	CredentialRevision  uint64                    `json:"credentialRevision"`
	CredentialReference storedCredentialReference `json:"credentialReference"`
}

type storedCredentialReference struct {
	Kind       string `json:"kind"`
	ProfileID  string `json:"profileId"`
	ProviderID string `json:"providerId"`
	AccountID  string `json:"accountId"`
}

type storedProfileAdvanced struct {
	Temperature            *float64 `json:"temperature"`
	MaxTokens              *int     `json:"maxTokens"`
	EndpointID             string   `json:"endpointId"`
	ContextWindow          *int     `json:"contextWindow"`
	SupportsVision         *bool    `json:"supportsVision"`
	SupportsNativePDF      *bool    `json:"supportsNativePDF"`
	SupportsVideo          *bool    `json:"supportsVideo"`
	SupportsTools          string   `json:"supportsTools"`
	DefaultReasoningEffort string   `json:"defaultReasoningEffort"`
}

type storedAgentPlanAccount struct {
	Version            int    `json:"version"`
	AccountID          string `json:"accountId"`
	CredentialRevision uint64 `json:"credentialRevision"`
}

type capabilityCacheFile struct {
	Version int                     `json:"version"`
	Scopes  []scopedCapabilityCache `json:"scopes"`
}

type scopedCapabilityCache struct {
	Scope  capabilityCacheScope             `json:"scope"`
	Models map[string]cachedModelCapability `json:"models"`
}

type capabilityCacheScope struct {
	ProviderID         string `json:"provider_id"`
	NormalizedBaseURL  string `json:"normalized_base_url"`
	ProtocolFamily     string `json:"protocol_family"`
	ProfileID          string `json:"profile_id"`
	CredentialRevision uint64 `json:"credential_revision"`
	SelectorKind       string `json:"selector_kind"`
	EndpointContractID string `json:"endpoint_contract_id"`
}

type cachedModelCapability struct {
	ContextWindow      *int                       `json:"context_window"`
	MaxOutputTokens    *int                       `json:"max_output_tokens"`
	DefaultMaxTokens   *int                       `json:"default_max_tokens"`
	DefaultTemperature *float64                   `json:"default_temperature"`
	SupportsVision     *bool                      `json:"supports_vision"`
	SupportsNativePDF  *bool                      `json:"supports_native_pdf"`
	SupportsVideo      *bool                      `json:"supports_video"`
	SupportsTools      *string                    `json:"supports_tools"`
	ReasoningEffort    *ReasoningEffortCapability `json:"reasoning_effort"`
}

func (s *UserStore) loadReady() ([]ResolvedProfile, error) {
	if s.root == "" {
		return nil, ErrNoUsableProfile
	}
	raw, err := os.ReadFile(filepath.Join(s.root, "config", "models.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNoUsableProfile
	}
	if err != nil {
		return nil, fmt.Errorf("read model profiles: %w", err)
	}
	var file storedProfileFile
	if err := json.Unmarshal(raw, &file); err != nil {
		return nil, fmt.Errorf("parse model profiles: %w", err)
	}
	if file.Version != 1 && file.Version != 2 {
		return nil, fmt.Errorf("unsupported model profile version %d", file.Version)
	}
	env, err := readEnvFile(filepath.Join(s.root, ".env"))
	if err != nil {
		return nil, fmt.Errorf("read model credentials: %w", err)
	}
	capabilityCache := readCapabilityCache(filepath.Join(s.root, "cache", "model-capabilities.json"))

	providers := BuiltinProviders()
	providerByID := make(map[string]Provider, len(providers))
	for _, provider := range providers {
		providerByID[provider.ID] = provider
	}

	// Desktop read compatibility keeps the later duplicate provider+model.
	// Build in reverse, then restore file order.
	seenIdentity := make(map[string]struct{})
	reversed := make([]ResolvedProfile, 0, len(file.Profiles))
	for i := len(file.Profiles) - 1; i >= 0; i-- {
		stored := file.Profiles[i]
		if !stored.Enabled || !profileIDPattern.MatchString(strings.TrimSpace(stored.ID)) {
			continue
		}
		provider, ok := providerByID[strings.TrimSpace(stored.ProviderID)]
		if !ok {
			continue
		}
		if provider.EndpointModel && !endpointIDPattern.MatchString(strings.TrimSpace(stored.Advanced.EndpointID)) {
			continue
		}
		resolved := normalizeStoredProfile(stored, provider, nil)
		keyName := "AWAKEN_API_KEY_" + resolved.ID
		effectiveRevision := resolved.CredentialRevision
		if effectiveRevision == 0 {
			effectiveRevision = 1
		}
		if stored.CredentialReference.Kind == "provider_account" {
			accountID := strings.TrimSpace(stored.CredentialReference.AccountID)
			if accountID == "" {
				accountID = "volcengine-agent-plan-default"
			}
			keyName = "AWAKEN_AGENT_PLAN_API_KEY_" + accountID
			if account, accountErr := readAgentPlanAccount(filepath.Join(s.root, "config", "agent-plan-account.json")); accountErr == nil && account.CredentialRevision > 0 {
				effectiveRevision = account.CredentialRevision
			}
		}
		resolved.CredentialRevision = effectiveRevision
		if cached := capabilityCache.resolve(resolved); cached != nil {
			resolved = normalizeStoredProfile(stored, provider, cached)
			resolved.CredentialRevision = effectiveRevision
		}
		identity := resolved.ProviderID + "\x00" + resolved.CapabilityModelID
		if _, duplicate := seenIdentity[identity]; duplicate {
			continue
		}
		seenIdentity[identity] = struct{}{}
		key := strings.TrimSpace(env[keyName])
		if key == "" {
			continue
		}
		resolved.APIKey = key
		reversed = append(reversed, resolved)
	}
	slices.Reverse(reversed)
	if len(reversed) == 0 {
		return nil, ErrNoUsableProfile
	}
	return reversed, nil
}

func normalizeStoredProfile(stored storedProfile, provider Provider, cached *cachedModelCapability) ResolvedProfile {
	id := strings.TrimSpace(stored.ID)
	capabilityModelID := strings.TrimSpace(stored.ModelID)
	if capabilityModelID == "" || (!provider.AllowCustomModel && !slices.Contains(provider.Models, capabilityModelID)) {
		capabilityModelID = provider.DefaultModel
	}
	modelID := capabilityModelID
	if provider.EndpointModel {
		if endpoint := strings.TrimSpace(stored.Advanced.EndpointID); endpoint != "" {
			modelID = endpoint
		}
	}
	if !provider.EndpointModel && (modelID == "" || (!provider.AllowCustomModel && !slices.Contains(provider.Models, modelID))) {
		modelID = provider.DefaultModel
	}
	baseURL := strings.TrimRight(strings.TrimSpace(stored.BaseURL), "/")
	if baseURL == "" {
		baseURL = provider.BaseURL
	}
	if provider.ID == "kimi-coding-plan" && baseURL == "https://api.kimi.com/coding" {
		baseURL = provider.BaseURL
	}
	if provider.ID == "volcengine" && baseURL == "https://ark.cn-beijing.volces.com/api/v3" {
		baseURL = provider.BaseURL
	}
	if provider.ID == "volcengine-agent-plan" {
		baseURL = provider.BaseURL
	}
	protocol := strings.TrimSpace(stored.Protocol)
	if protocol == "" {
		protocol = provider.Protocol
	}
	authScheme := strings.TrimSpace(stored.AuthScheme)
	if authScheme == "" {
		authScheme = provider.AuthScheme
	}
	displayName := strings.TrimSpace(stored.DisplayName)
	if displayName == "" {
		displayName = provider.DisplayName
	}

	capability, hasCapability := provider.ModelCapabilities[capabilityModelID]
	if cached != nil {
		capability = mergeCachedCapability(capability, cached)
		hasCapability = true
	}
	contextWindow := 1_000_000
	maxTokens := 8192
	if provider.ID == "volcengine" {
		contextWindow = 256_000
		maxTokens = 32_000
	}
	temperature := float64(0)
	supportsVision := false
	supportsNativePDF := false
	supportsVideo := false
	supportsTools := "ready"
	reasoningEffort := ""
	var reasoningEffortLevels []string
	if hasCapability {
		if capability.ContextWindow > 0 {
			contextWindow = capability.ContextWindow
		}
		if capability.DefaultMaxTokens > 0 {
			maxTokens = capability.DefaultMaxTokens
		} else if capability.MaxOutputTokens > 0 {
			maxTokens = capability.MaxOutputTokens
		}
		temperature = capability.DefaultTemperature
		supportsVision = capability.SupportsVision
		supportsNativePDF = capability.SupportsNativePDF
		supportsVideo = capability.SupportsVideo
		if capability.ReasoningEffort != nil {
			reasoningEffort = capability.ReasoningEffort.DefaultLevel
			reasoningEffortLevels = append([]string(nil), capability.ReasoningEffort.Levels...)
		}
	}
	if stored.Advanced.ContextWindow != nil && *stored.Advanced.ContextWindow > 0 {
		contextWindow = *stored.Advanced.ContextWindow
		if hasCapability && capability.ContextWindow > 0 && contextWindow > capability.ContextWindow {
			contextWindow = capability.ContextWindow
		}
	}
	if stored.Advanced.MaxTokens != nil && *stored.Advanced.MaxTokens > 0 {
		maxTokens = *stored.Advanced.MaxTokens
		if hasCapability && capability.MaxOutputTokens > 0 && maxTokens > capability.MaxOutputTokens {
			maxTokens = capability.MaxOutputTokens
		}
	}
	if stored.Advanced.Temperature != nil {
		temperature = *stored.Advanced.Temperature
	}
	if stored.Advanced.SupportsVision != nil {
		supportsVision = *stored.Advanced.SupportsVision
	}
	if stored.Advanced.SupportsNativePDF != nil {
		supportsNativePDF = *stored.Advanced.SupportsNativePDF
	}
	if stored.Advanced.SupportsVideo != nil {
		supportsVideo = *stored.Advanced.SupportsVideo
	}
	if hasCapability && capability.ReasoningEffort != nil && slices.Contains(capability.ReasoningEffort.Levels, stored.Advanced.DefaultReasoningEffort) {
		reasoningEffort = stored.Advanced.DefaultReasoningEffort
	}
	revision := stored.CredentialRevision
	if revision < 1 {
		revision = 1
	}
	return ResolvedProfile{
		ProfileSummary: ProfileSummary{
			ID:          id,
			ProviderID:  provider.ID,
			DisplayName: displayName,
			ModelID:     modelID,
			IsDefault:   stored.IsDefault,
		},
		BaseURL:               baseURL,
		Protocol:              protocol,
		AuthScheme:            authScheme,
		Temperature:           temperature,
		MaxTokens:             maxTokens,
		ContextWindow:         contextWindow,
		CredentialRevision:    revision,
		SupportsVision:        supportsVision,
		SupportsNativePDF:     supportsNativePDF,
		SupportsVideo:         supportsVideo,
		SupportsTools:         supportsTools,
		ReasoningEffort:       reasoningEffort,
		ReasoningEffortLevels: reasoningEffortLevels,
		CapabilityModelID:     capabilityModelID,
		EndpointSelector: func() string {
			if provider.EndpointModel {
				return modelID
			}
			return ""
		}(),
		ModelSelectorKind: func() string {
			if provider.EndpointModel {
				return "endpoint"
			}
			return "model"
		}(),
	}
}

func mergeCachedCapability(base ModelCapability, cached *cachedModelCapability) ModelCapability {
	if cached == nil {
		return base
	}
	if cached.ContextWindow != nil {
		base.ContextWindow = *cached.ContextWindow
	}
	if cached.MaxOutputTokens != nil {
		base.MaxOutputTokens = *cached.MaxOutputTokens
	}
	if cached.DefaultMaxTokens != nil {
		base.DefaultMaxTokens = *cached.DefaultMaxTokens
	}
	if cached.DefaultTemperature != nil {
		base.DefaultTemperature = *cached.DefaultTemperature
	}
	if cached.SupportsVision != nil {
		base.SupportsVision = *cached.SupportsVision
	}
	if cached.SupportsNativePDF != nil {
		base.SupportsNativePDF = *cached.SupportsNativePDF
	}
	if cached.SupportsVideo != nil {
		base.SupportsVideo = *cached.SupportsVideo
	}
	if cached.SupportsTools != nil {
		base.SupportsTools = *cached.SupportsTools
	}
	if cached.ReasoningEffort != nil {
		base.ReasoningEffort = &ReasoningEffortCapability{
			Levels:       append([]string(nil), cached.ReasoningEffort.Levels...),
			DefaultLevel: cached.ReasoningEffort.DefaultLevel,
		}
	}
	return base
}

func readCapabilityCache(path string) capabilityCacheFile {
	raw, err := os.ReadFile(path)
	if err != nil {
		return capabilityCacheFile{}
	}
	var file capabilityCacheFile
	if json.Unmarshal(raw, &file) != nil || file.Version != 2 {
		// Provider-wide v1 records have no endpoint/profile authority and are a
		// safe cold miss, never a migration source for a run.
		return capabilityCacheFile{}
	}
	return file
}

func (file capabilityCacheFile) resolve(profile ResolvedProfile) *cachedModelCapability {
	if file.Version != 2 {
		return nil
	}
	revision := profile.CredentialRevision
	if revision == 0 {
		revision = 1
	}
	selector := profile.ModelID
	want := capabilityCacheScope{
		ProviderID:         profile.ProviderID,
		NormalizedBaseURL:  normalizeCapabilityBaseURL(profile.BaseURL),
		ProtocolFamily:     strings.ToLower(strings.TrimSpace(profile.Protocol)),
		ProfileID:          profile.ID,
		CredentialRevision: revision,
		SelectorKind:       profile.ModelSelectorKind,
		EndpointContractID: capabilityEndpointContract(profile.Protocol),
	}
	for index := range file.Scopes {
		entry := &file.Scopes[index]
		if !capabilityScopesEqual(entry.Scope, want) {
			continue
		}
		if capability, ok := entry.Models[selector]; ok {
			return &capability
		}
	}
	return nil
}

func capabilityScopesEqual(left, right capabilityCacheScope) bool {
	return strings.EqualFold(strings.TrimSpace(left.ProviderID), strings.TrimSpace(right.ProviderID)) &&
		normalizeCapabilityBaseURL(left.NormalizedBaseURL) == normalizeCapabilityBaseURL(right.NormalizedBaseURL) &&
		strings.EqualFold(strings.TrimSpace(left.ProtocolFamily), strings.TrimSpace(right.ProtocolFamily)) &&
		strings.TrimSpace(left.ProfileID) == strings.TrimSpace(right.ProfileID) &&
		max(uint64(1), left.CredentialRevision) == max(uint64(1), right.CredentialRevision) &&
		strings.EqualFold(strings.TrimSpace(left.SelectorKind), strings.TrimSpace(right.SelectorKind)) &&
		strings.EqualFold(strings.TrimSpace(left.EndpointContractID), strings.TrimSpace(right.EndpointContractID))
}

func normalizeCapabilityBaseURL(value string) string {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return strings.TrimRight(strings.TrimSpace(value), "/")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	if parsed.Path == "/" {
		parsed.Path = ""
	}
	return strings.TrimRight(parsed.String(), "/")
}

func capabilityEndpointContract(protocol string) string {
	switch strings.ToLower(strings.TrimSpace(protocol)) {
	case "minimax_chat":
		return "minimax_chatcompletion_v2"
	case "anthropic_messages":
		return "anthropic_messages_v1"
	default:
		return "openai_chat_completions_v1"
	}
}

func readAgentPlanAccount(path string) (storedAgentPlanAccount, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return storedAgentPlanAccount{}, err
	}
	var account storedAgentPlanAccount
	if err := json.Unmarshal(raw, &account); err != nil {
		return storedAgentPlanAccount{}, err
	}
	return account, nil
}

func readEnvFile(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()

	entries := make(map[string]string)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}
		separator := strings.IndexByte(line, '=')
		if separator <= 0 {
			return nil, errors.New("invalid .env assignment")
		}
		key := strings.TrimSpace(line[:separator])
		if !envKeyPattern.MatchString(key) {
			return nil, errors.New("invalid .env key")
		}
		value, err := decodeEnvValue(line[separator+1:])
		if err != nil {
			return nil, err
		}
		entries[key] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

func decodeEnvValue(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if strings.HasPrefix(value, `"`) {
		var decoded string
		if err := json.Unmarshal([]byte(value), &decoded); err != nil {
			return "", errors.New("invalid quoted .env value")
		}
		return decoded, nil
	}
	if strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'") {
		return strings.TrimSuffix(strings.TrimPrefix(value, "'"), "'"), nil
	}
	return value, nil
}
