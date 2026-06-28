package thinkback

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	UserTypeAnt      = "ant"
	UserTypeExternal = "external"

	InternalMarketplaceName = "claude-code-marketplace"
	InternalMarketplaceRepo = "anthropics/claude-code-marketplace"
	OfficialMarketplaceName = "claude-plugins-official"
	OfficialMarketplaceRepo = "anthropics/claude-plugins-official"

	PluginName = "thinkback"
	SkillName  = "thinkback"

	AnimationFilename = "year_in_review.js"
	PlayerFilename    = "player.js"
	HTMLFilename      = "year_in_review.html"
)

type Phase string

const (
	PhaseChecking              Phase = "checking"
	PhaseInstallingMarketplace Phase = "installing-marketplace"
	PhaseRefreshingMarketplace Phase = "refreshing-marketplace"
	PhaseInstallingPlugin      Phase = "installing-plugin"
	PhaseEnablingPlugin        Phase = "enabling-plugin"
	PhaseReady                 Phase = "ready"
	PhaseError                 Phase = "error"
)

type Action string

const (
	ActionCheck              Action = "check"
	ActionInstallMarketplace Action = "install-marketplace"
	ActionRefreshMarketplace Action = "refresh-marketplace"
	ActionInstallPlugin      Action = "install-plugin"
	ActionEnablePlugin       Action = "enable-plugin"
	ActionUseSkillDir        Action = "use-skill-dir"
	ActionEnterAltScreen     Action = "enter-alternate-screen"
	ActionRunNodePlayer      Action = "run-node-player"
	ActionOpenHTML           Action = "open-html"
)

type Target struct {
	UserType        string `json:"userType"`
	MarketplaceName string `json:"marketplaceName"`
	MarketplaceRepo string `json:"marketplaceRepo"`
	PluginID        string `json:"pluginId"`
	PluginName      string `json:"pluginName"`
	SkillName       string `json:"skillName"`
}

type PlanStep struct {
	Phase           Phase    `json:"phase"`
	Action          Action   `json:"action"`
	Message         string   `json:"message"`
	MarketplaceName string   `json:"marketplaceName,omitempty"`
	MarketplaceRepo string   `json:"marketplaceRepo,omitempty"`
	PluginID        string   `json:"pluginId,omitempty"`
	Path            string   `json:"path,omitempty"`
	WorkingDir      string   `json:"workingDir,omitempty"`
	Command         []string `json:"command,omitempty"`
	Optional        bool     `json:"optional,omitempty"`
}

type PluginState struct {
	ID          string `json:"id,omitempty"`
	Name        string `json:"name,omitempty"`
	Source      string `json:"source,omitempty"`
	InstallPath string `json:"installPath,omitempty"`
	SkillDir    string `json:"skillDir,omitempty"`
	Enabled     bool   `json:"enabled"`
}

type InstallInput struct {
	UserType     string          `json:"userType"`
	Marketplaces map[string]bool `json:"marketplaces,omitempty"`
	Plugins      []PluginState   `json:"plugins,omitempty"`
}

type InstallPlan struct {
	Target               Target     `json:"target"`
	Phase                Phase      `json:"phase"`
	Message              string     `json:"message"`
	Steps                []PlanStep `json:"steps"`
	SkillDir             string     `json:"skillDir,omitempty"`
	MarketplaceInstalled bool       `json:"marketplaceInstalled"`
	PluginInstalled      bool       `json:"pluginInstalled"`
	PluginEnabled        bool       `json:"pluginEnabled"`
}

type PlaybackInput struct {
	InstallPath     string `json:"installPath,omitempty"`
	SkillDir        string `json:"skillDir,omitempty"`
	DisableHTMLOpen bool   `json:"disableHtmlOpen,omitempty"`
}

type PlaybackPlan struct {
	Phase            Phase      `json:"phase"`
	Message          string     `json:"message"`
	Steps            []PlanStep `json:"steps"`
	SkillDir         string     `json:"skillDir,omitempty"`
	AnimationPath    string     `json:"animationPath,omitempty"`
	PlayerPath       string     `json:"playerPath,omitempty"`
	HTMLPath         string     `json:"htmlPath,omitempty"`
	WorkingDir       string     `json:"workingDir,omitempty"`
	Command          []string   `json:"command,omitempty"`
	AlternateScreen  bool       `json:"alternateScreen"`
	HTMLOpenOptional bool       `json:"htmlOpenOptional"`
}

func TargetForUserType(userType string) Target {
	normalized := normalizeUserType(userType)
	if normalized == UserTypeAnt {
		return Target{
			UserType:        UserTypeAnt,
			MarketplaceName: InternalMarketplaceName,
			MarketplaceRepo: InternalMarketplaceRepo,
			PluginID:        PluginName + "@" + InternalMarketplaceName,
			PluginName:      PluginName,
			SkillName:       SkillName,
		}
	}

	return Target{
		UserType:        UserTypeExternal,
		MarketplaceName: OfficialMarketplaceName,
		MarketplaceRepo: OfficialMarketplaceRepo,
		PluginID:        PluginName + "@" + OfficialMarketplaceName,
		PluginName:      PluginName,
		SkillName:       SkillName,
	}
}

func BuildInstallPlan(input InstallInput) InstallPlan {
	target := TargetForUserType(input.UserType)
	steps := []PlanStep{{
		Phase:           PhaseChecking,
		Action:          ActionCheck,
		Message:         "Check thinkback marketplace, plugin, and skill directory.",
		MarketplaceName: target.MarketplaceName,
		PluginID:        target.PluginID,
	}}

	marketplaceInstalled := marketplaceInstalled(input.Marketplaces, target.MarketplaceName)
	if !marketplaceInstalled {
		steps = append(steps,
			PlanStep{
				Phase:           PhaseInstallingMarketplace,
				Action:          ActionInstallMarketplace,
				Message:         "Install the thinkback marketplace source.",
				MarketplaceName: target.MarketplaceName,
				MarketplaceRepo: target.MarketplaceRepo,
			},
			PlanStep{
				Phase:    PhaseInstallingPlugin,
				Action:   ActionInstallPlugin,
				Message:  "Install the thinkback plugin after the marketplace is available.",
				PluginID: target.PluginID,
			},
		)
		return InstallPlan{
			Target:               target,
			Phase:                PhaseInstallingMarketplace,
			Message:              fmt.Sprintf("Marketplace %s is not installed.", target.MarketplaceName),
			Steps:                steps,
			MarketplaceInstalled: false,
			PluginInstalled:      false,
			PluginEnabled:        false,
		}
	}

	plugin, pluginInstalled := findPlugin(input.Plugins, target)
	if !pluginInstalled {
		steps = append(steps,
			PlanStep{
				Phase:           PhaseRefreshingMarketplace,
				Action:          ActionRefreshMarketplace,
				Message:         "Refresh the marketplace before installing the thinkback plugin.",
				MarketplaceName: target.MarketplaceName,
			},
			PlanStep{
				Phase:    PhaseInstallingPlugin,
				Action:   ActionInstallPlugin,
				Message:  "Install the thinkback plugin.",
				PluginID: target.PluginID,
			},
		)
		return InstallPlan{
			Target:               target,
			Phase:                PhaseRefreshingMarketplace,
			Message:              fmt.Sprintf("Plugin %s is not installed.", target.PluginID),
			Steps:                steps,
			MarketplaceInstalled: true,
			PluginInstalled:      false,
			PluginEnabled:        false,
		}
	}

	if !plugin.Enabled {
		steps = append(steps, PlanStep{
			Phase:    PhaseEnablingPlugin,
			Action:   ActionEnablePlugin,
			Message:  "Enable the thinkback plugin.",
			PluginID: target.PluginID,
		})
		return InstallPlan{
			Target:               target,
			Phase:                PhaseEnablingPlugin,
			Message:              fmt.Sprintf("Plugin %s is installed but disabled.", target.PluginID),
			Steps:                steps,
			MarketplaceInstalled: true,
			PluginInstalled:      true,
			PluginEnabled:        false,
		}
	}

	skillDir := pluginSkillDir(*plugin)
	if skillDir == "" {
		return installErrorPlan(target, steps, "install path missing: Thinkback plugin installation path not found.", true, true, true)
	}

	exists, err := dirExists(skillDir)
	if err != nil {
		return installErrorPlan(target, steps, fmt.Sprintf("Could not access thinkback skill directory: %v", err), true, true, true)
	}
	if !exists {
		return installErrorPlan(target, steps, fmt.Sprintf("Could not find thinkback skill directory: %s", skillDir), true, true, true)
	}

	steps = append(steps, PlanStep{
		Phase:   PhaseReady,
		Action:  ActionUseSkillDir,
		Message: "Thinkback skill directory is ready.",
		Path:    skillDir,
	})
	return InstallPlan{
		Target:               target,
		Phase:                PhaseReady,
		Message:              "Thinkback plugin is ready.",
		Steps:                steps,
		SkillDir:             skillDir,
		MarketplaceInstalled: true,
		PluginInstalled:      true,
		PluginEnabled:        true,
	}
}

func BuildPlaybackPlan(input PlaybackInput) PlaybackPlan {
	skillDir := strings.TrimSpace(input.SkillDir)
	if skillDir == "" {
		installPath := strings.TrimSpace(input.InstallPath)
		if installPath == "" {
			return playbackErrorPlan("install path missing: Thinkback plugin installation path not found.", nil)
		}
		skillDir = filepath.Join(installPath, "skills", SkillName)
	}

	animationPath := filepath.Join(skillDir, AnimationFilename)
	playerPath := filepath.Join(skillDir, PlayerFilename)
	htmlPath := filepath.Join(skillDir, HTMLFilename)
	steps := []PlanStep{{
		Phase:   PhaseChecking,
		Action:  ActionCheck,
		Message: "Check thinkback playback files.",
		Path:    skillDir,
	}}

	animationExists, err := fileExists(animationPath)
	if err != nil {
		return playbackErrorPlan(fmt.Sprintf("Could not access animation data: %v", err), steps)
	}
	if !animationExists {
		return playbackErrorPlan("No animation found. Run /think-back first to generate one.", steps)
	}

	playerExists, err := fileExists(playerPath)
	if err != nil {
		return playbackErrorPlan(fmt.Sprintf("Could not access player script: %v", err), steps)
	}
	if !playerExists {
		return playbackErrorPlan("player missing: Player script not found. The player.js file is missing from the thinkback skill.", steps)
	}

	command := []string{"node", playerPath}
	steps = append(steps,
		PlanStep{
			Phase:   PhaseReady,
			Action:  ActionEnterAltScreen,
			Message: "Enter alternate screen before playback.",
		},
		PlanStep{
			Phase:      PhaseReady,
			Action:     ActionRunNodePlayer,
			Message:    "Run the thinkback player with Node.",
			Path:       playerPath,
			WorkingDir: skillDir,
			Command:    command,
		},
	)

	htmlExists, err := fileExists(htmlPath)
	if err == nil && htmlExists && !input.DisableHTMLOpen {
		steps = append(steps, PlanStep{
			Phase:      PhaseReady,
			Action:     ActionOpenHTML,
			Message:    "Open the generated HTML after playback if the caller chooses to.",
			Path:       htmlPath,
			WorkingDir: skillDir,
			Optional:   true,
		})
	}

	plan := PlaybackPlan{
		Phase:           PhaseReady,
		Message:         "Year in review animation ready to play.",
		Steps:           steps,
		SkillDir:        skillDir,
		AnimationPath:   animationPath,
		PlayerPath:      playerPath,
		WorkingDir:      skillDir,
		Command:         command,
		AlternateScreen: true,
	}
	if htmlExists && !input.DisableHTMLOpen {
		plan.HTMLPath = htmlPath
		plan.HTMLOpenOptional = true
	}
	return plan
}

func ConvertScratchpadToThink(content string) string {
	if content == "" || !strings.Contains(content, "<REASONING_SCRATCHPAD>") {
		return content
	}
	return strings.ReplaceAll(
		strings.ReplaceAll(content, "<REASONING_SCRATCHPAD>", "<think>"),
		"</REASONING_SCRATCHPAD>",
		"</think>",
	)
}

func HasIncompleteScratchpad(content string) bool {
	return content != "" &&
		strings.Contains(content, "<REASONING_SCRATCHPAD>") &&
		!strings.Contains(content, "</REASONING_SCRATCHPAD>")
}

func normalizeUserType(userType string) string {
	if strings.EqualFold(strings.TrimSpace(userType), UserTypeAnt) {
		return UserTypeAnt
	}
	return UserTypeExternal
}

func marketplaceInstalled(marketplaces map[string]bool, name string) bool {
	for marketplaceName, installed := range marketplaces {
		if installed && strings.EqualFold(marketplaceName, name) {
			return true
		}
	}
	return false
}

func findPlugin(plugins []PluginState, target Target) (*PluginState, bool) {
	for i := range plugins {
		if strings.EqualFold(plugins[i].ID, target.PluginID) {
			return &plugins[i], true
		}
	}
	for i := range plugins {
		if containsFold(plugins[i].Source, target.PluginID) {
			return &plugins[i], true
		}
	}
	for i := range plugins {
		plugin := &plugins[i]
		if !strings.EqualFold(plugin.Name, PluginName) {
			continue
		}
		if plugin.Source == "" ||
			containsFold(plugin.Source, target.MarketplaceName) ||
			containsFold(plugin.Source, target.PluginID) {
			return plugin, true
		}
	}
	return nil, false
}

func pluginSkillDir(plugin PluginState) string {
	if strings.TrimSpace(plugin.SkillDir) != "" {
		return strings.TrimSpace(plugin.SkillDir)
	}
	if strings.TrimSpace(plugin.InstallPath) == "" {
		return ""
	}
	return filepath.Join(plugin.InstallPath, "skills", SkillName)
}

func installErrorPlan(target Target, steps []PlanStep, message string, marketplaceInstalled, pluginInstalled, pluginEnabled bool) InstallPlan {
	steps = append(steps, PlanStep{
		Phase:   PhaseError,
		Action:  ActionCheck,
		Message: message,
	})
	return InstallPlan{
		Target:               target,
		Phase:                PhaseError,
		Message:              message,
		Steps:                steps,
		MarketplaceInstalled: marketplaceInstalled,
		PluginInstalled:      pluginInstalled,
		PluginEnabled:        pluginEnabled,
	}
}

func playbackErrorPlan(message string, steps []PlanStep) PlaybackPlan {
	steps = append(steps, PlanStep{
		Phase:   PhaseError,
		Action:  ActionCheck,
		Message: message,
	})
	return PlaybackPlan{
		Phase:   PhaseError,
		Message: message,
		Steps:   steps,
	}
}

func containsFold(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

func dirExists(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return info.IsDir(), nil
}

func fileExists(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return !info.IsDir(), nil
}
