package fastmode

import "time"

type AuthType string

const (
	AuthAPIKey AuthType = "api-key"
	AuthOAuth  AuthType = "oauth"
)

type ProviderKind string

const (
	ProviderFirstParty ProviderKind = "firstParty"
	ProviderBedrock    ProviderKind = "bedrock"
	ProviderVertex     ProviderKind = "vertex"
	ProviderFoundry    ProviderKind = "foundry"
)

type OrgState string

const (
	OrgPending  OrgState = "pending"
	OrgEnabled  OrgState = "enabled"
	OrgDisabled OrgState = "disabled"
)

type OrgDisabledReason string

const (
	OrgReasonFree               OrgDisabledReason = "free"
	OrgReasonPreference         OrgDisabledReason = "preference"
	OrgReasonExtraUsageDisabled OrgDisabledReason = "extra_usage_disabled"
	OrgReasonNetworkError       OrgDisabledReason = "network_error"
	OrgReasonUnknown            OrgDisabledReason = "unknown"
)

type OrgStatus struct {
	State  OrgState
	Reason OrgDisabledReason
}

type RuntimeStatus string

const (
	RuntimeActive   RuntimeStatus = "active"
	RuntimeCooldown RuntimeStatus = "cooldown"
)

type CooldownReason string

const (
	CooldownRateLimit  CooldownReason = "rate_limit"
	CooldownOverloaded CooldownReason = "overloaded"
)

type RuntimeState struct {
	Status  RuntimeStatus
	ResetAt time.Time
	Reason  CooldownReason
}

type Settings struct {
	FastMode                *bool
	FastModePerSessionOptIn bool
	FlagFastMode            bool
	Opus1MMergeEnabled      bool
}

type Environment struct {
	FeatureDisabled                bool
	StatsigReason                  string
	NativeBinaryRequired           bool
	BundledMode                    bool
	NonInteractiveSession          bool
	PreferThirdPartyAuthentication bool
	KairosActive                   bool
	Provider                       ProviderKind
	Auth                           AuthType
	Org                            OrgStatus
	SkipNetworkErrors              bool
}

type Decision struct {
	Available         bool
	UnavailableReason string
}

type RoutePlan struct {
	FastMode            bool
	Model               string
	ModelChanged        bool
	ClearSessionModel   bool
	ClearCooldown       bool
	Settings            Settings
	Org                 OrgStatus
	PermanentlyDisabled bool
	Message             string
	Events              []EventPlan
}

type EventType string

const (
	EventCooldownTriggered EventType = "cooldown_triggered"
	EventCooldownExpired   EventType = "cooldown_expired"
	EventOrgChanged        EventType = "org_fast_mode_changed"
	EventOverageRejected   EventType = "overage_rejected"
)

type EventPlan struct {
	Type       EventType
	At         time.Time
	ResetAt    time.Time
	Reason     CooldownReason
	Message    string
	OrgEnabled bool
}

func Bool(value bool) *bool {
	return &value
}

func DisabledOrg(reason OrgDisabledReason) OrgStatus {
	if reason == "" {
		reason = OrgReasonPreference
	}
	return OrgStatus{State: OrgDisabled, Reason: reason}
}
