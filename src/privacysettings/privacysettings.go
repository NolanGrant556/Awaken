package privacysettings

const (
	FallbackMessage       = "Review and manage your privacy settings at https://claude.ai/settings/data-privacy-controls"
	DismissedMessage      = "Privacy settings dialog dismissed"
	RefreshFailureMessage = "Unable to retrieve updated privacy settings"

	GrovePolicyToggledEvent = "tengu_grove_policy_toggled"
	SettingsLocation        = "settings"
)

type FetchResult[T any] struct {
	Success bool
	Data    T
}

type AccountSettings struct {
	GroveEnabled *bool
}

type GroveConfig struct {
	DomainExcluded bool
}

type DialogKind string

const (
	DialogKindNone            DialogKind = ""
	DialogKindPrivacySettings DialogKind = "privacy_settings"
	DialogKindGroveTerms      DialogKind = "grove_terms"
)

type DoneDisplay string

const (
	DoneDisplayDefault DoneDisplay = ""
	DoneDisplaySystem  DoneDisplay = "system"
)

type Decision string

const (
	DecisionEscape Decision = "escape"
	DecisionDefer  Decision = "defer"
	DecisionAccept Decision = "accept"
	DecisionUpdate Decision = "update"
)

type CommandInput struct {
	Qualified bool
	Settings  FetchResult[AccountSettings]
	Config    FetchResult[GroveConfig]
}

type CommandPlan struct {
	Dialog *DialogPlan
	Done   *DoneMessage
}

type DialogPlan struct {
	Kind                DialogKind
	Settings            AccountSettings
	DomainExcluded      bool
	DomainExcludedKnown bool
	Location            string
}

type DoneMessage struct {
	Text    string
	Display DoneDisplay
}

type DecisionInput struct {
	Decision        Decision
	InitialSettings AccountSettings
	UpdatedSettings FetchResult[AccountSettings]
}

type DecisionPlan struct {
	Done        DoneMessage
	AuditEvents []AuditEventPlan
}

type AuditEventPlan struct {
	Name     string
	State    bool
	Location string
}

func PlanCommand(input CommandInput) CommandPlan {
	if !input.Qualified || !input.Settings.Success {
		return CommandPlan{
			Done: &DoneMessage{Text: FallbackMessage},
		}
	}

	settings := input.Settings.Data
	if settings.GroveEnabled != nil {
		return CommandPlan{
			Dialog: &DialogPlan{
				Kind:                DialogKindPrivacySettings,
				Settings:            settings,
				DomainExcluded:      input.Config.Success && input.Config.Data.DomainExcluded,
				DomainExcludedKnown: input.Config.Success,
				Location:            SettingsLocation,
			},
		}
	}

	return CommandPlan{
		Dialog: &DialogPlan{
			Kind:     DialogKindGroveTerms,
			Settings: settings,
			Location: SettingsLocation,
		},
	}
}

func ResolveDecision(input DecisionInput) DecisionPlan {
	switch input.Decision {
	case DecisionEscape, DecisionDefer:
		return DecisionPlan{
			Done: DoneMessage{
				Text:    DismissedMessage,
				Display: DoneDisplaySystem,
			},
		}
	case DecisionAccept, DecisionUpdate:
		return resolveSettingsRefresh(input.InitialSettings, input.UpdatedSettings)
	default:
		return resolveSettingsRefresh(input.InitialSettings, input.UpdatedSettings)
	}
}

func resolveSettingsRefresh(initial AccountSettings, updatedResult FetchResult[AccountSettings]) DecisionPlan {
	if !updatedResult.Success {
		return DecisionPlan{
			Done: DoneMessage{
				Text:    RefreshFailureMessage,
				Display: DoneDisplaySystem,
			},
		}
	}

	updated := updatedResult.Data
	enabled := boolValue(updated.GroveEnabled)
	plan := DecisionPlan{
		Done: DoneMessage{Text: `"Help improve Claude" set to ` + boolString(enabled) + "."},
	}

	if initial.GroveEnabled != nil && updated.GroveEnabled != nil && *initial.GroveEnabled != *updated.GroveEnabled {
		plan.AuditEvents = append(plan.AuditEvents, AuditEventPlan{
			Name:     GrovePolicyToggledEvent,
			State:    *updated.GroveEnabled,
			Location: SettingsLocation,
		})
	}

	return plan
}

func boolValue(value *bool) bool {
	return value != nil && *value
}

func boolString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}
