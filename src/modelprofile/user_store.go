package modelprofile

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

var (
	ErrNoUsableProfile    = errors.New("no usable model profile is configured")
	ErrProfileUnavailable = errors.New("model profile is unavailable")

	profileIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	envKeyPattern    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9._-]*$`)
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
	BaseURL            string
	Protocol           string
	AuthScheme         string
	APIKey             string
	Temperature        float64
	MaxTokens          int
	ContextWindow      int
	CredentialRevision uint64
	SupportsVision     bool
	SupportsNativePDF  bool
	SupportsVideo      bool
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
	ID                 string                `json:"id"`
	ProviderID         string                `json:"providerId"`
	DisplayName        string                `json:"displayName"`
	ModelID            string                `json:"modelId"`
	BaseURL            string                `json:"baseURL"`
	Protocol           string                `json:"protocol"`
	AuthScheme         string                `json:"authScheme"`
	Enabled            bool                  `json:"enabled"`
	IsDefault          bool                  `json:"isDefault"`
	Advanced           storedProfileAdvanced `json:"advanced"`
	CredentialRevision uint64                `json:"credentialRevision"`
}

type storedProfileAdvanced struct {
	Temperature       *float64 `json:"temperature"`
	MaxTokens         *int     `json:"maxTokens"`
	EndpointID        string   `json:"endpointId"`
	ContextWindow     *int     `json:"contextWindow"`
	SupportsVision    *bool    `json:"supportsVision"`
	SupportsNativePDF *bool    `json:"supportsNativePDF"`
	SupportsVideo     *bool    `json:"supportsVideo"`
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
	if file.Version != 1 {
		return nil, fmt.Errorf("unsupported model profile version %d", file.Version)
	}
	env, err := readEnvFile(filepath.Join(s.root, ".env"))
	if err != nil {
		return nil, fmt.Errorf("read model credentials: %w", err)
	}

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
		resolved := normalizeStoredProfile(stored, provider)
		identity := resolved.ProviderID + "\x00" + resolved.ModelID
		if _, duplicate := seenIdentity[identity]; duplicate {
			continue
		}
		seenIdentity[identity] = struct{}{}
		key := strings.TrimSpace(env["AWAKEN_API_KEY_"+resolved.ID])
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

func normalizeStoredProfile(stored storedProfile, provider Provider) ResolvedProfile {
	id := strings.TrimSpace(stored.ID)
	modelID := strings.TrimSpace(stored.ModelID)
	if provider.EndpointModel {
		if endpoint := strings.TrimSpace(stored.Advanced.EndpointID); endpoint != "" {
			modelID = endpoint
		}
	}
	if modelID == "" || (!provider.AllowCustomModel && !slices.Contains(provider.Models, modelID)) {
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

	capability, hasCapability := provider.ModelCapabilities[modelID]
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
		BaseURL:            baseURL,
		Protocol:           protocol,
		AuthScheme:         authScheme,
		Temperature:        temperature,
		MaxTokens:          maxTokens,
		ContextWindow:      contextWindow,
		CredentialRevision: revision,
		SupportsVision:     supportsVision,
		SupportsNativePDF:  supportsNativePDF,
		SupportsVideo:      supportsVideo,
	}
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
