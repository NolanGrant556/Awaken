package providerregistry

import "time"

type ProviderKind string

const (
	ProviderKindOpenAICompatible ProviderKind = "openai_compatible"
	ProviderKindAnthropic        ProviderKind = "anthropic"
	ProviderKindGemini           ProviderKind = "gemini"
	ProviderKindBedrock          ProviderKind = "bedrock"
	ProviderKindVertex           ProviderKind = "vertex"
	ProviderKindCustom           ProviderKind = "custom"
)

type Protocol string

const (
	ProtocolOpenAIChat            Protocol = "openai_chat"
	ProtocolAnthropicMessages     Protocol = "anthropic_messages"
	ProtocolGeminiGenerateContent Protocol = "gemini_generate_content"
	ProtocolBedrockConverse       Protocol = "bedrock_converse"
	ProtocolVertexGemini          Protocol = "vertex_gemini"
	ProtocolCustom                Protocol = "custom"
)

type AuthScheme string

const (
	AuthSchemeNone         AuthScheme = "none"
	AuthSchemeBearer       AuthScheme = "bearer"
	AuthSchemeXAPIKey      AuthScheme = "x-api-key"
	AuthSchemeGeminiAPIKey AuthScheme = "gemini-api-key"
	AuthSchemeAWSSigV4     AuthScheme = "aws_sigv4"
	AuthSchemeGoogleADC    AuthScheme = "google_adc"
	AuthSchemeCustom       AuthScheme = "custom"
)

type CredentialSource string

const (
	CredentialSourceNone        CredentialSource = "none"
	CredentialSourceEnv         CredentialSource = "env"
	CredentialSourceKeychain    CredentialSource = "keychain"
	CredentialSourceSecretStore CredentialSource = "secret_store"
	CredentialSourceOAuth       CredentialSource = "oauth"
	CredentialSourceAWSProfile  CredentialSource = "aws_profile"
	CredentialSourceGoogleADC   CredentialSource = "google_adc"
	CredentialSourceCustom      CredentialSource = "custom"
)

type CredentialRef struct {
	Source  CredentialSource `json:"source"`
	Name    string           `json:"name,omitempty"`
	Present bool             `json:"present"`
}

type ModelCapability string

const (
	ModelCapabilityText             ModelCapability = "text"
	ModelCapabilityVision           ModelCapability = "vision"
	ModelCapabilityToolCalling      ModelCapability = "tool_calling"
	ModelCapabilityStreaming        ModelCapability = "streaming"
	ModelCapabilityReasoning        ModelCapability = "reasoning"
	ModelCapabilityStructuredOutput ModelCapability = "structured_output"
	ModelCapabilityPromptCaching    ModelCapability = "prompt_caching"
	ModelCapabilityLongContext      ModelCapability = "long_context"
	ModelCapabilityEmbeddings       ModelCapability = "embeddings"
	ModelCapabilityExcludePrimary   ModelCapability = "exclude_from_primary"
)

type Model struct {
	ID              string            `json:"id"`
	Name            string            `json:"name,omitempty"`
	Protocol        Protocol          `json:"protocol,omitempty"`
	Capabilities    []ModelCapability `json:"capabilities,omitempty"`
	MaxInputTokens  int               `json:"max_input_tokens,omitempty"`
	MaxOutputTokens int               `json:"max_output_tokens,omitempty"`
	ContextWindow   int               `json:"context_window,omitempty"`
	Enabled         bool              `json:"enabled"`
}

type HealthStatus string

const (
	HealthUnknown   HealthStatus = "unknown"
	HealthHealthy   HealthStatus = "healthy"
	HealthUnhealthy HealthStatus = "unhealthy"
)

type HealthCheck struct {
	Status        HealthStatus `json:"status"`
	CheckedAt     time.Time    `json:"checked_at,omitempty"`
	LatencyMillis int          `json:"latency_ms,omitempty"`
	Message       string       `json:"message,omitempty"`
}

type Provider struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Kind        ProviderKind           `json:"kind"`
	Protocol    Protocol               `json:"protocol"`
	AuthScheme  AuthScheme             `json:"auth_scheme"`
	Credential  CredentialRef          `json:"credential"`
	BaseURL     string                 `json:"base_url,omitempty"`
	Models      []Model                `json:"models"`
	Health      map[string]HealthCheck `json:"health,omitempty"`
	Enabled     bool                   `json:"enabled"`
	Annotations map[string]string      `json:"annotations,omitempty"`
}

type ProviderSnapshot struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Kind        ProviderKind           `json:"kind"`
	Protocol    Protocol               `json:"protocol"`
	AuthScheme  AuthScheme             `json:"auth_scheme"`
	Credential  CredentialRef          `json:"credential"`
	BaseURL     string                 `json:"base_url,omitempty"`
	Models      []Model                `json:"models"`
	Health      map[string]HealthCheck `json:"health,omitempty"`
	Enabled     bool                   `json:"enabled"`
	Annotations map[string]string      `json:"annotations,omitempty"`
}

type DiscoveryPlan struct {
	ProviderID       string       `json:"provider_id"`
	ProviderKind     ProviderKind `json:"provider_kind"`
	Method           string       `json:"method"`
	Path             string       `json:"path"`
	AuthHeaderStyle  string       `json:"auth_header_style"`
	ExpectedProtocol Protocol     `json:"expected_protocol"`
	BaseURL          string       `json:"base_url,omitempty"`
}
