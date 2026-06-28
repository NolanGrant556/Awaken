package privacysettings

import "testing"

func TestPlanCommandNotQualifiedFallback(t *testing.T) {
	plan := PlanCommand(CommandInput{
		Qualified: false,
		Settings:  successSettings(true),
		Config:    successConfig(false),
	})

	assertDone(t, plan.Done, FallbackMessage, DoneDisplayDefault)
	if plan.Dialog != nil {
		t.Fatalf("expected no dialog, got %+v", plan.Dialog)
	}
}

func TestPlanCommandSettingsFailureFallback(t *testing.T) {
	plan := PlanCommand(CommandInput{
		Qualified: true,
		Settings:  FetchResult[AccountSettings]{Success: false},
		Config:    successConfig(false),
	})

	assertDone(t, plan.Done, FallbackMessage, DoneDisplayDefault)
	if plan.Dialog != nil {
		t.Fatalf("expected no dialog, got %+v", plan.Dialog)
	}
}

func TestPlanCommandExistingSettingOpensPrivacySettingsDialog(t *testing.T) {
	plan := PlanCommand(CommandInput{
		Qualified: true,
		Settings:  successSettings(true),
		Config:    successConfig(true),
	})

	if plan.Done != nil {
		t.Fatalf("expected no done message, got %+v", plan.Done)
	}
	if plan.Dialog == nil {
		t.Fatalf("expected dialog plan")
	}
	if plan.Dialog.Kind != DialogKindPrivacySettings {
		t.Fatalf("expected privacy settings dialog, got %q", plan.Dialog.Kind)
	}
	if plan.Dialog.Settings.GroveEnabled == nil || !*plan.Dialog.Settings.GroveEnabled {
		t.Fatalf("expected existing grove setting true, got %+v", plan.Dialog.Settings)
	}
	if !plan.Dialog.DomainExcludedKnown || !plan.Dialog.DomainExcluded {
		t.Fatalf("expected known domain exclusion, got known=%v value=%v", plan.Dialog.DomainExcludedKnown, plan.Dialog.DomainExcluded)
	}
	if plan.Dialog.Location != SettingsLocation {
		t.Fatalf("expected location %q, got %q", SettingsLocation, plan.Dialog.Location)
	}
}

func TestPlanCommandConfigFailureKeepsSettingsDialogWithUnknownDomainExclusion(t *testing.T) {
	plan := PlanCommand(CommandInput{
		Qualified: true,
		Settings:  successSettings(false),
		Config:    FetchResult[GroveConfig]{Success: false},
	})

	if plan.Dialog == nil {
		t.Fatalf("expected dialog plan")
	}
	if plan.Dialog.Kind != DialogKindPrivacySettings {
		t.Fatalf("expected privacy settings dialog, got %q", plan.Dialog.Kind)
	}
	if plan.Dialog.DomainExcludedKnown {
		t.Fatalf("expected domain exclusion to be unknown")
	}
	if plan.Dialog.DomainExcluded {
		t.Fatalf("expected unknown domain exclusion to default false")
	}
}

func TestPlanCommandNilSettingOpensGroveDialog(t *testing.T) {
	plan := PlanCommand(CommandInput{
		Qualified: true,
		Settings: FetchResult[AccountSettings]{
			Success: true,
			Data:    AccountSettings{GroveEnabled: nil},
		},
		Config: successConfig(true),
	})

	if plan.Done != nil {
		t.Fatalf("expected no done message, got %+v", plan.Done)
	}
	if plan.Dialog == nil {
		t.Fatalf("expected dialog plan")
	}
	if plan.Dialog.Kind != DialogKindGroveTerms {
		t.Fatalf("expected Grove terms dialog, got %q", plan.Dialog.Kind)
	}
	if plan.Dialog.Location != SettingsLocation {
		t.Fatalf("expected location %q, got %q", SettingsLocation, plan.Dialog.Location)
	}
}

func TestResolveDecisionEscapeAndDeferDismissed(t *testing.T) {
	for _, decision := range []Decision{DecisionEscape, DecisionDefer} {
		t.Run(string(decision), func(t *testing.T) {
			plan := ResolveDecision(DecisionInput{
				Decision:        decision,
				InitialSettings: AccountSettings{GroveEnabled: boolPtr(true)},
				UpdatedSettings: successSettings(false),
			})

			if plan.Done.Text != DismissedMessage {
				t.Fatalf("expected dismissed message, got %q", plan.Done.Text)
			}
			if plan.Done.Display != DoneDisplaySystem {
				t.Fatalf("expected system display, got %q", plan.Done.Display)
			}
			if len(plan.AuditEvents) != 0 {
				t.Fatalf("expected no audit events, got %+v", plan.AuditEvents)
			}
		})
	}
}

func TestResolveDecisionToggleEmitsEvent(t *testing.T) {
	plan := ResolveDecision(DecisionInput{
		Decision:        DecisionUpdate,
		InitialSettings: AccountSettings{GroveEnabled: boolPtr(true)},
		UpdatedSettings: successSettings(false),
	})

	if plan.Done.Text != `"Help improve Claude" set to false.` {
		t.Fatalf("unexpected done message %q", plan.Done.Text)
	}
	if plan.Done.Display != DoneDisplayDefault {
		t.Fatalf("expected default display, got %q", plan.Done.Display)
	}
	if len(plan.AuditEvents) != 1 {
		t.Fatalf("expected one audit event, got %+v", plan.AuditEvents)
	}
	event := plan.AuditEvents[0]
	if event.Name != GrovePolicyToggledEvent {
		t.Fatalf("expected event %q, got %q", GrovePolicyToggledEvent, event.Name)
	}
	if event.State {
		t.Fatalf("expected toggled state false")
	}
	if event.Location != SettingsLocation {
		t.Fatalf("expected location %q, got %q", SettingsLocation, event.Location)
	}
}

func TestResolveDecisionSettingsRefreshFailureMessage(t *testing.T) {
	plan := ResolveDecision(DecisionInput{
		Decision:        DecisionAccept,
		InitialSettings: AccountSettings{GroveEnabled: boolPtr(false)},
		UpdatedSettings: FetchResult[AccountSettings]{Success: false},
	})

	if plan.Done.Text != RefreshFailureMessage {
		t.Fatalf("expected refresh failure message, got %q", plan.Done.Text)
	}
	if plan.Done.Display != DoneDisplaySystem {
		t.Fatalf("expected system display, got %q", plan.Done.Display)
	}
	if len(plan.AuditEvents) != 0 {
		t.Fatalf("expected no audit events, got %+v", plan.AuditEvents)
	}
}

func assertDone(t *testing.T, done *DoneMessage, text string, display DoneDisplay) {
	t.Helper()
	if done == nil {
		t.Fatalf("expected done message")
	}
	if done.Text != text {
		t.Fatalf("expected done text %q, got %q", text, done.Text)
	}
	if done.Display != display {
		t.Fatalf("expected display %q, got %q", display, done.Display)
	}
}

func successSettings(enabled bool) FetchResult[AccountSettings] {
	return FetchResult[AccountSettings]{
		Success: true,
		Data: AccountSettings{
			GroveEnabled: boolPtr(enabled),
		},
	}
}

func successConfig(domainExcluded bool) FetchResult[GroveConfig] {
	return FetchResult[GroveConfig]{
		Success: true,
		Data: GroveConfig{
			DomainExcluded: domainExcluded,
		},
	}
}

func boolPtr(value bool) *bool {
	return &value
}
