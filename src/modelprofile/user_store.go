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
	ReasoningEffortLevels []string `json:"reasoning_effort_levels,omitempty"`
	ID                    string   `json:"id"`
	ProviderID            string   `json:"provider_id"`
	DisplayName           string   `json:"display_name"`
	ModelID               string   `json:"model_id"`
	IsDefault             bool     `json:"is_default"`
}

// ResolvedProfile is a complete run profile resolved from Awaken-owned local
// configuration. APIKey must never be serialized to an ACP client or log.
type ResolvedProfile struct {
	ProfileSummary
	BaseURL               string
	InferenceEndpoint     string
	Protocol              string
	AuthScheme            string
	APIKey                string
	Temperature           float64
	MaxTokens             int
	ContextWindow         int
	MaxInputTokens        int
	CredentialRevision    uint64
	SupportsVision        bool
	SupportsNativePDF     bool
	SupportsVideo         bool
	SupportsTools         string
	SupportsPromptCache   bool
	SupportsComputerUse   bool
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
		summary := profile.ProfileSummary
		summary.ReasoningEffortLevels = append([]string(nil), profile.ReasoningEffortLevels...)
		summaries = append(summaries, summary)
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

// ResolveForPurpose applies an explicit pin first, then an optional Host-owned
// purpose default, then the normal ready default. An explicit unavailable pin
// fails closed; a stale optional purpose default safely falls back.
func (s *UserStore) ResolveForPurpose(purpose, explicitProfileID string) (ResolvedProfile, error) {
	if explicit := strings.TrimSpace(explicitProfileID); explicit != "" {
		return s.Resolve(explicit)
	}
	raw, err := os.ReadFile(filepath.Join(s.root, "config", "models.json"))
	if err == nil {
		var file storedProfileFile
		if json.Unmarshal(raw, &file) == nil {
			if routed := strings.TrimSpace(file.PurposeRoutes[strings.TrimSpace(purpose)]); routed != "" {
				if resolved, routeErr := s.Resolve(routed); routeErr == nil {
					return resolved, nil
				}
			}
		}
	}
	return s.Resolve("")
}

type storedProfileFile struct {
	Version       int               `json:"version"`
	Profiles      []storedProfile   `json:"profiles"`
	PurposeRoutes map[string]string `json:"purposeRoutes,omitempty"`
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
	InferenceEndpoint      string   `json:"inferenceEndpoint"`
	MaxInputTokens         *int     `json:"maxInputTokens"`
	ContextWindow          *int     `json:"contextWindow"`
	SupportsVision         *bool    `json:"supportsVision"`
	SupportsNativePDF      *bool    `json:"supportsNativePDF"`
	SupportsVideo          *bool    `json:"supportsVideo"`
	SupportsTools          string   `json:"supportsTools"`
	SupportsPromptCache    *bool    `json:"supportsPromptCache"`
	SupportsComputerUse    *bool    `json:"supportsComputerUse"`
	DefaultReasoningEffort string   `json:"defaultReasoningEffort"`
}

type storedAgentPlanAccount struct {
	Version            int    `json:"version"`
	AccountID          string `json:"accountId"`
	CredentialRevision uint64 `json:"credentialRevision"`
	Catalog            []struct {
		ID   string `json:"id"`
		Kind string `json:"kind"`
	} `json:"catalog"`
}

// AgentPlanCredential is the Core-only credential view for provider tools.
// It is never serialized into tool arguments, outcomes, or renderer payloads.
type AgentPlanCredential struct {
	APIKey             string
	CredentialRevision uint64
}

// ResolveAgentPlanCredential reads the Desktop-owned provider account without
// accepting a secret from a run request. The fixed account id remains part of
// the key name so future accounts do not silently share credentials.
func ResolveAgentPlanCredential(root, accountID string) (AgentPlanCredential, error) {
	root = strings.TrimSpace(root)
	accountID = strings.TrimSpace(accountID)
	if root == "" || accountID == "" || !profileIDPattern.MatchString(accountID) {
		return AgentPlanCredential{}, ErrProfileUnavailable
	}
	account, err := readAgentPlanAccount(filepath.Join(root, "config", "agent-plan-account.json"))
	if err != nil {
		return AgentPlanCredential{}, fmt.Errorf("read Agent Plan account: %w", err)
	}
	if strings.TrimSpace(account.AccountID) != accountID || account.CredentialRevision == 0 {
		return AgentPlanCredential{}, ErrProfileUnavailable
	}
	env, err := readEnvFile(filepath.Join(root, ".env"))
	if err != nil {
		return AgentPlanCredential{}, fmt.Errorf("read Agent Plan credential: %w", err)
	}
	apiKey := strings.TrimSpace(env["AWAKEN_AGENT_PLAN_API_KEY_"+accountID])
	if apiKey == "" {
		return AgentPlanCredential{}, ErrProfileUnavailable
	}
	return AgentPlanCredential{APIKey: apiKey, CredentialRevision: account.CredentialRevision}, nil
}

// ResolveAgentPlanModelKind returns the Main-classified catalog kind used as a
// defense-in-depth check by Core provider tools. Unknown or absent entries fail
// closed instead of letting a hand-edited media configuration select a model.
func ResolveAgentPlanModelKind(root, accountID, modelID string) (string, error) {
	root = strings.TrimSpace(root)
	accountID = strings.TrimSpace(accountID)
	modelID = strings.TrimSpace(modelID)
	if root == "" || accountID == "" || modelID == "" || !profileIDPattern.MatchString(accountID) {
		return "", ErrProfileUnavailable
	}
	account, err := readAgentPlanAccount(filepath.Join(root, "config", "agent-plan-account.json"))
	if err != nil || strings.TrimSpace(account.AccountID) != accountID {
		return "", ErrProfileUnavailable
	}
	for _, entry := range account.Catalog {
		if strings.TrimSpace(entry.ID) == modelID {
			kind := strings.TrimSpace(entry.Kind)
			if kind == "" {
				return "", ErrProfileUnavailable
			}
			return kind, nil
		}
	}
	return "", ErrProfileUnavailable
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
	MaxInputTokens      *int                       `json:"max_input_tokens"`
	ContextWindow       *int                       `json:"context_window"`
	MaxOutputTokens     *int                       `json:"max_output_tokens"`
	DefaultMaxTokens    *int                       `json:"default_max_tokens"`
	DefaultTemperature  *float64                   `json:"default_temperature"`
	SupportsVision      *bool                      `json:"supports_vision"`
	SupportsNativePDF   *bool                      `json:"supports_native_pdf"`
	SupportsVideo       *bool                      `json:"supports_video"`
	SupportsTools       *string                    `json:"supports_tools"`
	SupportsPromptCache *bool                      `json:"supports_prompt_cache"`
	SupportsComputerUse *bool                      `json:"supports_computer_use"`
	ReasoningEffort     *ReasoningEffortCapability `json:"reasoning_effort"`
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
	if file.Version != 1 && file.Version != 2 && file.Version != 3 {
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

	// Desktop read compatibility keeps the later duplicate full profile identity
	// (provider + normalized base URL + selector + protocol).
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
		resolved := normalizeStoredProfile(stored, provider, nil)
		if resolved.ID == "" {
			continue
		}
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
			account, accountErr := readAgentPlanAccount(filepath.Join(s.root, "config", "agent-plan-account.json"))
			if accountErr != nil {
				if stored.ProviderID == "volcengine-agent-plan" {
					continue
				}
			} else if account.CredentialRevision > 0 {
				if stored.ProviderID == "volcengine-agent-plan" && !agentPlanAccountAllowsLLM(account, stored.ModelID) {
					continue
				}
				effectiveRevision = account.CredentialRevision
			}
		}
		resolved.CredentialRevision = effectiveRevision
		if cached := capabilityCache.resolve(resolved); cached != nil {
			resolved = normalizeStoredProfile(stored, provider, cached)
			resolved.CredentialRevision = effectiveRevision
		}
		identity := strings.Join([]string{
			resolved.ProviderID,
			normalizeCapabilityBaseURL(resolved.BaseURL),
			resolved.ModelID,
			strings.ToLower(strings.TrimSpace(resolved.Protocol)),
		}, "\x00")
		if _, duplicate := seenIdentity[identity]; duplicate {
			continue
		}
		seenIdentity[identity] = struct{}{}
		key := strings.TrimSpace(env[keyName])
		if key == "" && !strings.EqualFold(resolved.AuthScheme, "none") {
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
	if provider.ID == "doubao" {
		capabilityModelID = canonicalizeDoubaoModelID(capabilityModelID)
	}
	if capabilityModelID == "" || (!provider.EndpointModel && !provider.AllowCustomModel && !slices.Contains(provider.Models, capabilityModelID)) {
		capabilityModelID = provider.DefaultModel
	}
	modelID := capabilityModelID
	if provider.EndpointModel {
		if endpoint := strings.TrimSpace(stored.Advanced.EndpointID); endpointIDPattern.MatchString(endpoint) {
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
	if provider.ID == "volcengine" && baseURL == "https://ark.cn-beijing.volces.com/api/v3" {
		baseURL = provider.BaseURL
	}
	protocol := strings.ToLower(strings.TrimSpace(stored.Protocol))
	if protocol == "" {
		protocol = provider.Protocol
	}
	if provider.ID == "kimi-cn" && (baseURL == "https://api.moonshot.cn/v1" || baseURL == "https://api.moonshot.cn/anthropic") {
		if protocol == "anthropic_messages" {
			baseURL = "https://api.moonshot.cn/anthropic"
		} else {
			baseURL = provider.BaseURL
		}
	}
	if provider.ID == "kimi-coding-plan" && (baseURL == "https://api.kimi.com/coding" || baseURL == "https://api.kimi.com/coding/v1") {
		if protocol == "anthropic_messages" {
			baseURL = "https://api.kimi.com/coding"
		} else {
			baseURL = provider.BaseURL
		}
	}
	protocolWasRetired := false
	if !validProtocolForProvider(protocol, provider) {
		if !knownProtocol(protocol) {
			return ResolvedProfile{}
		}
		protocol = provider.Protocol
		protocolWasRetired = true
		baseURL = provider.BaseURL
	}
	authScheme := strings.ToLower(strings.TrimSpace(stored.AuthScheme))
	if authScheme == "" {
		authScheme = provider.AuthScheme
	}
	if protocolWasRetired {
		authScheme = provider.AuthScheme
	}
	if !validAuthScheme(authScheme) {
		return ResolvedProfile{}
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
	if provider.ID == "kimi-cn" && protocol != "openai_chat" && capabilityModelID != "kimi-k3" {
		return ResolvedProfile{}
	}
	contextWindow := 0
	maxTokens := 0
	maxInputTokens := 0
	temperature := float64(0)
	supportsVision := false
	supportsNativePDF := false
	supportsVideo := false
	supportsTools := "unknown"
	supportsPromptCache := false
	supportsComputerUse := false
	reasoningEffort := ""
	var reasoningEffortLevels []string
	if hasCapability {
		if capability.ContextWindow > 0 {
			contextWindow = capability.ContextWindow
		}
		if capability.MaxInputTokens > 0 {
			maxInputTokens = capability.MaxInputTokens
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
		supportsComputerUse = capability.SupportsComputerUse
		if capability.SupportsTools != "" {
			supportsTools = capability.SupportsTools
		}
		supportsPromptCache = capability.SupportsPromptCache
		if capability.ReasoningEffort != nil {
			reasoningEffort = capability.ReasoningEffort.DefaultLevel
			reasoningEffortLevels = append([]string(nil), capability.ReasoningEffort.Levels...)
		}
	}
	// Native PDF is a protocol contract as well as a model capability. OpenAI's
	// Responses API accepts input_file blocks; Chat-compatible providers do not
	// inherit that fact merely because their native model API accepts PDFs.
	if provider.ID == "openai" && protocol == "openai_responses" {
		supportsNativePDF = true
	}
	// Custom profiles are user-declared transport contracts. Only the explicit
	// vision toggle is admitted; it enables image input and PDF page rendering,
	// but never upgrades the gateway to native PDF support.
	if provider.ID == "custom" {
		if stored.Advanced.SupportsVision != nil {
			supportsVision = *stored.Advanced.SupportsVision
		}
		supportsNativePDF = false
	}
	if (provider.ID == "kimi-cn" || provider.ID == "kimi-coding-plan") && protocol != "openai_chat" {
		supportsVideo = false
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
		InferenceEndpoint:     strings.TrimSpace(stored.Advanced.InferenceEndpoint),
		Protocol:              protocol,
		AuthScheme:            authScheme,
		Temperature:           temperature,
		MaxTokens:             maxTokens,
		ContextWindow:         contextWindow,
		MaxInputTokens:        maxInputTokens,
		CredentialRevision:    revision,
		SupportsVision:        supportsVision,
		SupportsNativePDF:     supportsNativePDF,
		SupportsVideo:         supportsVideo,
		SupportsTools:         supportsTools,
		SupportsPromptCache:   supportsPromptCache,
		SupportsComputerUse:   supportsComputerUse,
		ReasoningEffort:       reasoningEffort,
		ReasoningEffortLevels: reasoningEffortLevels,
		CapabilityModelID:     capabilityModelID,
		EndpointSelector: func() string {
			if provider.EndpointModel && endpointIDPattern.MatchString(modelID) {
				return modelID
			}
			return ""
		}(),
		ModelSelectorKind: func() string {
			if provider.EndpointModel && endpointIDPattern.MatchString(modelID) {
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
	if cached.MaxInputTokens != nil {
		base.MaxInputTokens = *cached.MaxInputTokens
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
	if cached.SupportsPromptCache != nil {
		base.SupportsPromptCache = *cached.SupportsPromptCache
	}
	if cached.SupportsComputerUse != nil {
		base.SupportsComputerUse = *cached.SupportsComputerUse
	}
	if cached.ReasoningEffort != nil {
		displayLabels := make(map[string]string)
		if base.ReasoningEffort != nil {
			for level, label := range base.ReasoningEffort.DisplayLabels {
				displayLabels[level] = label
			}
		}
		for level, label := range cached.ReasoningEffort.DisplayLabels {
			displayLabels[level] = label
		}
		base.ReasoningEffort = &ReasoningEffortCapability{
			Levels:        append([]string(nil), cached.ReasoningEffort.Levels...),
			DefaultLevel:  cached.ReasoningEffort.DefaultLevel,
			DisplayLabels: displayLabels,
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
	case "openai_responses":
		return "openai_responses_v1"
	default:
		return "openai_chat_completions_v1"
	}
}

func validProtocolForProvider(protocol string, provider Provider) bool {
	if !knownProtocol(protocol) {
		return false
	}
	if provider.ID == "minimax" && protocol == "minimax_chat" {
		return true
	}
	allowed := provider.SupportedProtocols
	if len(allowed) == 0 {
		allowed = []string{provider.Protocol}
	}
	return slices.Contains(allowed, protocol)
}

func knownProtocol(protocol string) bool {
	return protocol == "openai_chat" || protocol == "openai_responses" || protocol == "anthropic_messages" || protocol == "minimax_chat"
}

func validAuthScheme(authScheme string) bool {
	return slices.Contains([]string{"bearer", "x-api-key", "api-key", "x-goog-api-key", "none"}, authScheme)
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

func agentPlanAccountAllowsLLM(account storedAgentPlanAccount, modelID string) bool {
	modelID = strings.TrimSpace(modelID)
	if modelID == "ark-code-latest" {
		return true
	}
	for _, entry := range account.Catalog {
		if strings.TrimSpace(entry.ID) == modelID {
			return strings.TrimSpace(entry.Kind) == "llm"
		}
	}
	return false
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

func canonicalizeDoubaoModelID(modelID string) string {
	switch strings.TrimSpace(modelID) {
	case "doubao-seed-2.1-pro":
		return "doubao-seed-2-1-pro-260628"
	case "deepseek-v4-pro":
		return "deepseek-v4-pro-ga-260813"
	case "deepseek-v4-flash":
		return "deepseek-v4-flash-ga-260731"
	case "glm-5.3-flash":
		return "glm-5-3-flash-260828"
	case "glm-5.2":
		return "glm-5-2-260617"
	default:
		return modelID
	}
}
