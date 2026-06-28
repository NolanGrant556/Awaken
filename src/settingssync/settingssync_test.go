package settingssync

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestPlanUploadDiffOnlyIncludesChangedEntries(t *testing.T) {
	local := NewLocalSnapshot([]Entry{
		{Key: "~/.awaken/settings.json", Value: `{"theme":"dark"}`, Source: SourceUser, Scope: ScopeGlobal},
		{Key: "~/.awaken/AGENTS.md", Value: "local memory", Source: SourceUser, Scope: ScopeGlobal},
		{Key: "projects/repo/settings.local.json", Value: `{"approval_policy":"ask"}`, Source: SourceProject, Scope: ScopeProject},
	}, SnapshotOptions{})

	plan := PlanUploadDiff(local, map[string]string{
		"~/.awaken/settings.json":           `{"theme":"dark"}`,
		"~/.awaken/AGENTS.md":               "remote memory",
		"projects/repo/settings.local.json": `{"approval_policy":"never"}`,
	})

	if !plan.HasChanges() {
		t.Fatalf("expected changed entries")
	}
	if len(plan.Changed) != 2 {
		t.Fatalf("expected 2 changed entries, got %d", len(plan.Changed))
	}
	if len(plan.Unchanged) != 1 {
		t.Fatalf("expected 1 unchanged entry, got %d", len(plan.Unchanged))
	}

	changed := plan.ChangedEntryValues()
	if changed["~/.awaken/AGENTS.md"] != "local memory" {
		t.Fatalf("expected local memory to be uploaded, got %q", changed["~/.awaken/AGENTS.md"])
	}
	if _, ok := changed["~/.awaken/settings.json"]; ok {
		t.Fatalf("unchanged settings should not be uploaded")
	}
}

func TestPlanRemoteApplySkipsUnchangedAndAppliesChangedEntries(t *testing.T) {
	local := NewLocalSnapshot([]Entry{
		{Key: "settings.json", Value: `{"theme":"dark"}`, Source: SourceUser, Scope: ScopeGlobal},
		{Key: "memory.md", Value: "same", Source: SourceUser, Scope: ScopeGlobal},
	}, SnapshotOptions{})

	plan := PlanRemoteApply([]Entry{
		{Key: "settings.json", Value: `{"theme":"light"}`, Source: SourceRemote, Scope: ScopeGlobal},
		{Key: "memory.md", Value: "same", Source: SourceRemote, Scope: ScopeGlobal},
	}, local, RemoteApplyOptions{})

	toApply := plan.EntriesToApply()
	if len(toApply) != 1 {
		t.Fatalf("expected 1 entry to apply, got %d", len(toApply))
	}
	if toApply[0].Key != "settings.json" {
		t.Fatalf("expected settings.json to apply, got %q", toApply[0].Key)
	}

	applied := map[string]string{}
	result := ApplyRemotePlan(plan, func(entry Entry) error {
		applied[entry.Key] = entry.Value
		return nil
	})
	if !result.Success() {
		t.Fatalf("expected apply success, got %s", result.Summary())
	}
	if applied["settings.json"] != `{"theme":"light"}` {
		t.Fatalf("expected changed setting to be applied, got %q", applied["settings.json"])
	}
	if _, ok := applied["memory.md"]; ok {
		t.Fatalf("unchanged entry should not be applied")
	}
}

func TestSizeLimitSkipsSnapshotAndRemoteApplyEntries(t *testing.T) {
	largeValue := strings.Repeat("x", 11)
	local := NewLocalSnapshot([]Entry{
		{Key: "too-large", Value: largeValue, Source: SourceUser, Scope: ScopeGlobal},
		{Key: "ok", Value: "small", Source: SourceUser, Scope: ScopeGlobal},
	}, SnapshotOptions{MaxEntryBytes: 10})

	if len(local.Skipped) != 1 {
		t.Fatalf("expected 1 skipped local entry, got %d", len(local.Skipped))
	}
	if local.Skipped[0].Reason != SnapshotSkipTooLarge {
		t.Fatalf("expected too_large skip, got %q", local.Skipped[0].Reason)
	}
	if _, ok := local.Entry("too-large"); ok {
		t.Fatalf("too-large entry should not be in snapshot")
	}

	plan := PlanRemoteApply([]Entry{
		{Key: "remote-too-large", Value: largeValue, Source: SourceRemote, Scope: ScopeGlobal},
	}, local, RemoteApplyOptions{MaxEntryBytes: 10})
	if len(plan.Decisions) != 1 {
		t.Fatalf("expected one decision, got %d", len(plan.Decisions))
	}
	if plan.Decisions[0].Action != ApplyActionSkipTooLarge {
		t.Fatalf("expected remote entry to be skipped as too large, got %q", plan.Decisions[0].Action)
	}
}

func TestMigrationRunnerHonorsOnceGuardAndDryRun(t *testing.T) {
	state := NewMemoryMigrationState()
	appliedCount := 0
	migration := Migration{
		ID: "move-project-mcp-settings",
		Plan: func(context.Context) (MigrationPlan, error) {
			return MigrationPlan{
				Changes: []MigrationChange{{Key: "enableAllProjectMcpServers", OldValue: "true", NewValue: "settings.local.json"}},
			}, nil
		},
		Apply: func(context.Context) (MigrationApplyResult, error) {
			appliedCount++
			return MigrationApplyResult{Applied: true}, nil
		},
	}

	dryRun := RunMigrations(context.Background(), []Migration{migration}, state, MigrationRunOptions{DryRun: true})
	if len(dryRun.Results) != 1 || dryRun.Results[0].Status != MigrationStatusPlanned {
		t.Fatalf("expected dry-run planned status, got %#v", dryRun.Results)
	}
	if appliedCount != 0 {
		t.Fatalf("dry-run should not apply migration")
	}

	first := RunMigrations(context.Background(), []Migration{migration}, state, MigrationRunOptions{})
	if first.Results[0].Status != MigrationStatusApplied {
		t.Fatalf("expected applied status, got %q", first.Results[0].Status)
	}
	second := RunMigrations(context.Background(), []Migration{migration}, state, MigrationRunOptions{})
	if second.Results[0].Status != MigrationStatusSkipped {
		t.Fatalf("expected skipped status, got %q", second.Results[0].Status)
	}
	if appliedCount != 1 {
		t.Fatalf("expected migration to apply once, applied %d times", appliedCount)
	}
}

func TestManagedSettingsSecurityCheckFlagsDangerousChanges(t *testing.T) {
	changes := []EntryChange{
		{Key: "approval_policy", OldValue: "ask", NewValue: "never", Source: SourceRemoteManaged, Scope: ScopeGlobal},
		{Key: "theme", OldValue: "dark", NewValue: "light", Source: SourceRemoteManaged, Scope: ScopeGlobal},
	}

	check := CheckManagedSettingChanges(changes, ManagedSettingsSecurityPolicy{})
	if !check.RequiresConfirmation {
		t.Fatalf("expected dangerous change to require confirmation")
	}
	if len(check.Dangerous) != 1 {
		t.Fatalf("expected 1 dangerous change, got %d", len(check.Dangerous))
	}
	if check.Dangerous[0].Key != "approval_policy" {
		t.Fatalf("expected approval_policy to be dangerous, got %q", check.Dangerous[0].Key)
	}

	local := NewLocalSnapshot([]Entry{
		{Key: "approval_policy", Value: "ask", Source: SourceUser, Scope: ScopeGlobal},
	}, SnapshotOptions{})
	plan := PlanRemoteApply([]Entry{
		{Key: "approval_policy", Value: "never", Source: SourceRemoteManaged, Scope: ScopeGlobal},
	}, local, RemoteApplyOptions{})
	if len(plan.BlockedDangerousChanges()) != 1 {
		t.Fatalf("expected managed dangerous setting to be blocked")
	}

	confirmed := PlanRemoteApply([]Entry{
		{Key: "approval_policy", Value: "never", Source: SourceRemoteManaged, Scope: ScopeGlobal},
	}, local, RemoteApplyOptions{ConfirmedDangerousKey: map[string]bool{"approval_policy": true}})
	if len(confirmed.EntriesToApply()) != 1 {
		t.Fatalf("expected confirmed dangerous setting to be applyable")
	}
}

func TestMigrationFailureAndApplyErrorDoNotPanic(t *testing.T) {
	state := NewMemoryMigrationState()
	appliedSafeMigration := false
	result := RunMigrations(context.Background(), []Migration{
		{
			ID: "panic-migration",
			Apply: func(context.Context) (MigrationApplyResult, error) {
				panic("boom")
			},
		},
		{
			ID: "error-migration",
			Apply: func(context.Context) (MigrationApplyResult, error) {
				return MigrationApplyResult{}, errors.New("write failed")
			},
		},
		{
			ID: "safe-migration",
			Apply: func(context.Context) (MigrationApplyResult, error) {
				appliedSafeMigration = true
				return MigrationApplyResult{Applied: true}, nil
			},
		},
	}, state, MigrationRunOptions{})

	if result.Success() {
		t.Fatalf("expected failed result because two migrations failed")
	}
	if result.Results[0].Status != MigrationStatusFailed || !strings.Contains(result.Results[0].Error, "panic") {
		t.Fatalf("expected panic migration to be captured, got %#v", result.Results[0])
	}
	if result.Results[1].Status != MigrationStatusFailed || !strings.Contains(result.Results[1].Error, "write failed") {
		t.Fatalf("expected error migration to be captured, got %#v", result.Results[1])
	}
	if result.Results[2].Status != MigrationStatusApplied || !appliedSafeMigration {
		t.Fatalf("expected safe migration to continue after failures, got %#v", result.Results[2])
	}

	applyResult := ApplyRemotePlan(RemoteApplyPlan{Decisions: []ApplyDecision{
		{Key: "settings.json", Action: ApplyActionWrite, Entry: Entry{Key: "settings.json", Value: "{}"}},
	}}, func(Entry) error {
		panic("disk unavailable")
	})
	if len(applyResult.Errors) != 1 || !strings.Contains(applyResult.Errors[0].Error(), "panic") {
		t.Fatalf("expected apply panic to be captured, got %#v", applyResult.Errors)
	}
}

func TestStringOutputRedactsSensitiveValues(t *testing.T) {
	entry := Entry{Key: "LLM_API_KEY", Value: "sk-proj-1234567890secret", Source: SourceUser, Scope: ScopeGlobal}
	if text := entry.String(); strings.Contains(text, entry.Value) || !strings.Contains(text, RedactedValue) {
		t.Fatalf("expected entry String to redact secret, got %s", text)
	}

	change := MigrationChange{Key: "client_secret", OldValue: "old-secret-value", NewValue: "new-secret-value"}
	if text := change.String(); strings.Contains(text, "old-secret-value") || strings.Contains(text, "new-secret-value") {
		t.Fatalf("expected migration change String to redact secrets, got %s", text)
	}

	dangerous := DangerousChange{Key: "auth.token", OldValue: "ghp_1234567890abcdef", NewValue: "ghp_abcdef1234567890"}
	if text := dangerous.String(); strings.Contains(text, dangerous.OldValue) || strings.Contains(text, dangerous.NewValue) {
		t.Fatalf("expected dangerous change String to redact secrets, got %s", text)
	}
}
