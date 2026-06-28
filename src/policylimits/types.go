package policylimits

import "time"

const (
	ClaudeAIInferenceScope = "user:inference"

	PolicyAllowProductFeedback = "allow_product_feedback"
)

type Restriction struct {
	Allowed bool `json:"allowed"`
}

type Restrictions map[string]Restriction

type EligibilityInput struct {
	FirstPartyProvider  bool
	FirstPartyBaseURL   bool
	HasAPIKey           bool
	HasOAuthAccessToken bool
	OAuthScopes         []string
	SubscriptionType    string
}

type AuthInput struct {
	APIKey           string
	OAuthAccessToken string
	OAuthBetaHeader  string
}

type AuthHeaders struct {
	Headers map[string]string
	Error   string
}

type FetchResult struct {
	Success      bool
	Restrictions Restrictions
	NotModified  bool
	StatusCode   int
	ETag         string
	Error        string
	SkipRetry    bool
}

type CacheEntry struct {
	Restrictions Restrictions
	Checksum     string
	ETag         string
}

type LoadDecision struct {
	SessionCache    Restrictions
	HasSessionCache bool
	WriteCache      bool
	DeleteCache     bool
	UsedStaleCache  bool
	FailOpen        bool
	Reason          string
	Checksum        string
	ETag            string
}

type PolicyDecision struct {
	Policy     string
	Allowed    bool
	FailOpen   bool
	FailClosed bool
	Reason     string
}

type PollPlan struct {
	Eligible                     bool
	ShouldCreateLoadingPromise   bool
	ShouldStartBackgroundPolling bool
	LoadingTimeout               time.Duration
	PollInterval                 time.Duration
	Reason                       string
}

type PermissionOption struct {
	ID   string
	Kind PermissionOptionKind
}

type PermissionOptionKind string

const (
	PermissionOptionAllowOnce    PermissionOptionKind = "allow_once"
	PermissionOptionAllowAlways  PermissionOptionKind = "allow_always"
	PermissionOptionRejectOnce   PermissionOptionKind = "reject_once"
	PermissionOptionRejectAlways PermissionOptionKind = "reject_always"
)

type PermissionBridgeInput struct {
	TimedOut  bool
	Err       error
	Responded bool
	OptionID  string
	Options   []PermissionOption
}

type PermissionResult string

const (
	PermissionResultOnce   PermissionResult = "once"
	PermissionResultAlways PermissionResult = "always"
	PermissionResultDeny   PermissionResult = "deny"
)

type PermissionBridgeOutcome struct {
	Result PermissionResult
	Denied bool
	Reason string
}
