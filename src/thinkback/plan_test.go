package thinkback

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestTargetForUserTypeSelectsExternalAndAntPluginIDs(t *testing.T) {
	external := TargetForUserType(UserTypeExternal)
	if external.MarketplaceName != OfficialMarketplaceName {
		t.Fatalf("expected external marketplace %q, got %q", OfficialMarketplaceName, external.MarketplaceName)
	}
	if external.MarketplaceRepo != OfficialMarketplaceRepo {
		t.Fatalf("expected external repo %q, got %q", OfficialMarketplaceRepo, external.MarketplaceRepo)
	}
	if external.PluginID != "thinkback@claude-plugins-official" {
		t.Fatalf("unexpected external plugin id: %q", external.PluginID)
	}

	ant := TargetForUserType(UserTypeAnt)
	if ant.MarketplaceName != InternalMarketplaceName {
		t.Fatalf("expected ant marketplace %q, got %q", InternalMarketplaceName, ant.MarketplaceName)
	}
	if ant.MarketplaceRepo != InternalMarketplaceRepo {
		t.Fatalf("expected ant repo %q, got %q", InternalMarketplaceRepo, ant.MarketplaceRepo)
	}
	if ant.PluginID != "thinkback@claude-code-marketplace" {
		t.Fatalf("unexpected ant plugin id: %q", ant.PluginID)
	}
}

func TestBuildInstallPlanMarketplaceMissingPlansInstall(t *testing.T) {
	plan := BuildInstallPlan(InstallInput{UserType: UserTypeExternal})

	if plan.Phase != PhaseInstallingMarketplace {
		t.Fatalf("expected phase %s, got %s", PhaseInstallingMarketplace, plan.Phase)
	}
	if plan.Target.MarketplaceName != OfficialMarketplaceName || plan.Target.MarketplaceRepo != OfficialMarketplaceRepo {
		t.Fatalf("unexpected target: %+v", plan.Target)
	}
	if !hasStep(plan.Steps, PhaseInstallingMarketplace, ActionInstallMarketplace) {
		t.Fatalf("expected install marketplace step, got %+v", plan.Steps)
	}
	if !hasStep(plan.Steps, PhaseInstallingPlugin, ActionInstallPlugin) {
		t.Fatalf("expected install plugin step, got %+v", plan.Steps)
	}
}

func TestBuildInstallPlanMarketplaceInstalledPluginMissingPlansRefresh(t *testing.T) {
	plan := BuildInstallPlan(InstallInput{
		UserType: UserTypeExternal,
		Marketplaces: map[string]bool{
			OfficialMarketplaceName: true,
		},
	})

	if plan.Phase != PhaseRefreshingMarketplace {
		t.Fatalf("expected phase %s, got %s", PhaseRefreshingMarketplace, plan.Phase)
	}
	if !hasStep(plan.Steps, PhaseRefreshingMarketplace, ActionRefreshMarketplace) {
		t.Fatalf("expected refresh marketplace step, got %+v", plan.Steps)
	}
	if !hasStep(plan.Steps, PhaseInstallingPlugin, ActionInstallPlugin) {
		t.Fatalf("expected install plugin step, got %+v", plan.Steps)
	}
}

func TestBuildInstallPlanPluginDisabledPlansEnable(t *testing.T) {
	plan := BuildInstallPlan(InstallInput{
		UserType: UserTypeExternal,
		Marketplaces: map[string]bool{
			OfficialMarketplaceName: true,
		},
		Plugins: []PluginState{{
			ID:          "thinkback@claude-plugins-official",
			InstallPath: t.TempDir(),
			Enabled:     false,
		}},
	})

	if plan.Phase != PhaseEnablingPlugin {
		t.Fatalf("expected phase %s, got %s", PhaseEnablingPlugin, plan.Phase)
	}
	if !plan.PluginInstalled || plan.PluginEnabled {
		t.Fatalf("unexpected plugin status: installed=%v enabled=%v", plan.PluginInstalled, plan.PluginEnabled)
	}
	if !hasStep(plan.Steps, PhaseEnablingPlugin, ActionEnablePlugin) {
		t.Fatalf("expected enable plugin step, got %+v", plan.Steps)
	}
}

func TestBuildInstallPlanReadySkillDir(t *testing.T) {
	installPath := t.TempDir()
	skillDir := filepath.Join(installPath, "skills", SkillName)
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("failed to create skill dir: %v", err)
	}

	plan := BuildInstallPlan(InstallInput{
		UserType: UserTypeExternal,
		Marketplaces: map[string]bool{
			OfficialMarketplaceName: true,
		},
		Plugins: []PluginState{{
			ID:          "thinkback@claude-plugins-official",
			InstallPath: installPath,
			Enabled:     true,
		}},
	})

	if plan.Phase != PhaseReady {
		t.Fatalf("expected phase %s, got %s (%s)", PhaseReady, plan.Phase, plan.Message)
	}
	if plan.SkillDir != skillDir {
		t.Fatalf("expected skill dir %q, got %q", skillDir, plan.SkillDir)
	}
	if !plan.MarketplaceInstalled || !plan.PluginInstalled || !plan.PluginEnabled {
		t.Fatalf("expected all install checks to be true, got %+v", plan)
	}
	if !hasStep(plan.Steps, PhaseReady, ActionUseSkillDir) {
		t.Fatalf("expected ready skill dir step, got %+v", plan.Steps)
	}
}

func TestBuildPlaybackPlanReportsMissingAnimation(t *testing.T) {
	plan := BuildPlaybackPlan(PlaybackInput{SkillDir: t.TempDir()})

	if plan.Phase != PhaseError {
		t.Fatalf("expected error phase, got %s", plan.Phase)
	}
	if !strings.Contains(plan.Message, "No animation found") {
		t.Fatalf("expected missing animation message, got %q", plan.Message)
	}
}

func TestBuildPlaybackPlanReportsPlayerMissing(t *testing.T) {
	skillDir := t.TempDir()
	writeFile(t, filepath.Join(skillDir, AnimationFilename))

	plan := BuildPlaybackPlan(PlaybackInput{SkillDir: skillDir})

	if plan.Phase != PhaseError {
		t.Fatalf("expected error phase, got %s", plan.Phase)
	}
	if !strings.Contains(plan.Message, "player missing") {
		t.Fatalf("expected player missing message, got %q", plan.Message)
	}
}

func TestBuildPlaybackPlanReportsInstallPathMissing(t *testing.T) {
	plan := BuildPlaybackPlan(PlaybackInput{})

	if plan.Phase != PhaseError {
		t.Fatalf("expected error phase, got %s", plan.Phase)
	}
	if !strings.Contains(plan.Message, "install path missing") {
		t.Fatalf("expected install path missing message, got %q", plan.Message)
	}
}

func TestBuildPlaybackPlanIncludesOptionalHTMLOpen(t *testing.T) {
	skillDir := t.TempDir()
	writeFile(t, filepath.Join(skillDir, AnimationFilename))
	writeFile(t, filepath.Join(skillDir, PlayerFilename))
	writeFile(t, filepath.Join(skillDir, HTMLFilename))

	plan := BuildPlaybackPlan(PlaybackInput{SkillDir: skillDir})

	if plan.Phase != PhaseReady {
		t.Fatalf("expected ready phase, got %s (%s)", plan.Phase, plan.Message)
	}
	if !plan.AlternateScreen {
		t.Fatal("expected playback plan to require alternate screen")
	}
	wantCommand := []string{"node", filepath.Join(skillDir, PlayerFilename)}
	if !reflect.DeepEqual(plan.Command, wantCommand) {
		t.Fatalf("expected command %+v, got %+v", wantCommand, plan.Command)
	}
	if plan.WorkingDir != skillDir {
		t.Fatalf("expected working dir %q, got %q", skillDir, plan.WorkingDir)
	}
	if !plan.HTMLOpenOptional || plan.HTMLPath != filepath.Join(skillDir, HTMLFilename) {
		t.Fatalf("expected optional html open, got optional=%v path=%q", plan.HTMLOpenOptional, plan.HTMLPath)
	}
	if !hasStep(plan.Steps, PhaseReady, ActionEnterAltScreen) ||
		!hasStep(plan.Steps, PhaseReady, ActionRunNodePlayer) ||
		!hasStep(plan.Steps, PhaseReady, ActionOpenHTML) {
		t.Fatalf("expected alternate screen, node player, and html open steps, got %+v", plan.Steps)
	}
	if stepIndex(plan.Steps, ActionOpenHTML) <= stepIndex(plan.Steps, ActionRunNodePlayer) {
		t.Fatalf("expected html open to be after node playback, got %+v", plan.Steps)
	}
}

func TestBuildPlaybackPlanCanSkipOptionalHTMLOpen(t *testing.T) {
	skillDir := t.TempDir()
	writeFile(t, filepath.Join(skillDir, AnimationFilename))
	writeFile(t, filepath.Join(skillDir, PlayerFilename))
	writeFile(t, filepath.Join(skillDir, HTMLFilename))

	plan := BuildPlaybackPlan(PlaybackInput{SkillDir: skillDir, DisableHTMLOpen: true})

	if plan.Phase != PhaseReady {
		t.Fatalf("expected ready phase, got %s (%s)", plan.Phase, plan.Message)
	}
	if plan.HTMLOpenOptional || plan.HTMLPath != "" {
		t.Fatalf("expected html open to be skipped, got optional=%v path=%q", plan.HTMLOpenOptional, plan.HTMLPath)
	}
	if hasStep(plan.Steps, PhaseReady, ActionOpenHTML) {
		t.Fatalf("did not expect html open step, got %+v", plan.Steps)
	}
}

func TestScratchpadConversionAndIncompleteDetection(t *testing.T) {
	content := "before <REASONING_SCRATCHPAD>hidden</REASONING_SCRATCHPAD> after"
	converted := ConvertScratchpadToThink(content)
	if converted != "before <think>hidden</think> after" {
		t.Fatalf("unexpected converted scratchpad: %q", converted)
	}
	if HasIncompleteScratchpad(content) {
		t.Fatal("complete scratchpad should not be marked incomplete")
	}
	if !HasIncompleteScratchpad("before <REASONING_SCRATCHPAD>hidden") {
		t.Fatal("expected incomplete scratchpad to be detected")
	}
	if got := ConvertScratchpadToThink("plain text"); got != "plain text" {
		t.Fatalf("plain text should be unchanged, got %q", got)
	}
}

func hasStep(steps []PlanStep, phase Phase, action Action) bool {
	for _, step := range steps {
		if step.Phase == phase && step.Action == action {
			return true
		}
	}
	return false
}

func stepIndex(steps []PlanStep, action Action) int {
	for i, step := range steps {
		if step.Action == action {
			return i
		}
	}
	return -1
}

func writeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to write %s: %v", path, err)
	}
}
