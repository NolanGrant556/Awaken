package terminalsetup

import (
	"fmt"
	"strings"
)

type Terminal string

const (
	TerminalApple     Terminal = "Apple_Terminal"
	TerminalVSCode    Terminal = "vscode"
	TerminalCursor    Terminal = "cursor"
	TerminalWindsurf  Terminal = "windsurf"
	TerminalAlacritty Terminal = "alacritty"
	TerminalZed       Terminal = "zed"
)

type Environment struct {
	Terminal  string
	Platform  string
	HomeDir   string
	Variables map[string]string
}

type Config struct {
	ShiftEnterKeyBindingInstalled bool
	OptionAsMetaKeyInstalled      bool
	HasUsedBackslashReturn        bool
}

type PlanResult struct {
	Supported            bool
	Native               bool
	AlreadyInstalled     bool
	RequiresLocalMachine bool
	TerminalDisplayName  string
	Actions              []Action
	Messages             []string
	ConfigUpdates        []ConfigUpdate
	FilesToEdit          []FileEdit
	BackupPolicy         BackupPolicy
	Snippet              string
}

type Action struct {
	ID          string
	Title       string
	Description string
	Target      Terminal
	Manual      bool
}

type ConfigUpdate struct {
	Key    string
	Value  bool
	Reason string
}

type FileEdit struct {
	Path             string
	AlternativePaths []string
	Description      string
	Snippet          string
	CreateIfMissing  bool
	BackupExisting   bool
	TargetMachine    string
}

type BackupPolicy struct {
	Required bool
	Summary  string
}

var nativeCSIuTerminals = map[string]string{
	"ghostty":      "Ghostty",
	"kitty":        "Kitty",
	"iTerm.app":    "iTerm2",
	"WezTerm":      "WezTerm",
	"WarpTerminal": "Warp",
}

var nativeCSIuAliases = map[string]string{
	"ghostty":      "Ghostty",
	"kitty":        "Kitty",
	"iterm":        "iTerm2",
	"iterm.app":    "iTerm2",
	"iterm2":       "iTerm2",
	"wezterm":      "WezTerm",
	"warp":         "Warp",
	"warpterminal": "Warp",
}

func Plan(env Environment, config Config) PlanResult {
	result := PlanResult{
		Supported:           ShouldOffer(env),
		TerminalDisplayName: terminalDisplayName(env.Terminal),
		BackupPolicy: BackupPolicy{
			Required: false,
			Summary:  "Planner only; no files are written.",
		},
	}

	if display := NativeCSIuDisplayName(env.Terminal); display != "" {
		result.Native = true
		result.TerminalDisplayName = display
		result.Messages = append(result.Messages,
			fmt.Sprintf("Shift+Enter is natively supported in %s.", display),
			"No terminal setup is needed.",
		)
		return result
	}

	if !result.Supported {
		result.Messages = unsupportedMessages(env, config)
		return result
	}

	target, _ := supportedKeybindingTarget(env.Terminal)
	if target == "" && terminalKey(env.Terminal) == string(TerminalApple) {
		return planAppleTerminal(env, config, result)
	}

	if config.ShiftEnterKeyBindingInstalled {
		result.AlreadyInstalled = true
		result.Messages = append(result.Messages,
			fmt.Sprintf("%s Shift+Enter keybinding is already marked installed; no actions planned.", targetDisplayName(target)),
		)
		return result
	}

	if isRemoteEditorSession(env) && isVSCodeFamily(target) {
		return planRemoteEditor(env, target, config, result)
	}

	switch target {
	case TerminalVSCode, TerminalCursor, TerminalWindsurf:
		return planEditorKeybinding(env, target, config, result)
	case TerminalZed:
		return planZedKeybinding(env, config, result)
	case TerminalAlacritty:
		return planAlacrittyKeybinding(env, config, result)
	default:
		result.Supported = false
		result.Messages = unsupportedMessages(env, config)
		return result
	}
}

func ShouldOffer(env Environment) bool {
	if NativeCSIuDisplayName(env.Terminal) != "" {
		return false
	}

	if terminalKey(env.Terminal) == string(TerminalApple) {
		return platformKey(env.Platform) == "darwin"
	}

	_, ok := supportedKeybindingTarget(env.Terminal)
	return ok
}

func NativeCSIuDisplayName(terminal string) string {
	terminal = strings.TrimSpace(terminal)
	if display, ok := nativeCSIuTerminals[terminal]; ok {
		return display
	}
	return nativeCSIuAliases[strings.ToLower(terminal)]
}

func KeybindingSnippet(target Terminal) string {
	switch normalizeTarget(target) {
	case TerminalVSCode, TerminalCursor, TerminalWindsurf:
		return `[
  {
    "key": "shift+enter",
    "command": "workbench.action.terminal.sendSequence",
    "args": { "text": "\u001b\r" },
    "when": "terminalFocus"
  }
]`
	case TerminalZed:
		return `[
  {
    "context": "Terminal",
    "bindings": {
      "shift-enter": ["terminal::SendText", "\u001b\r"]
    }
  }
]`
	case TerminalAlacritty:
		return `[[keyboard.bindings]]
key = "Return"
mods = "Shift"
chars = "\u001B\r"`
	default:
		return ""
	}
}

func planAppleTerminal(env Environment, config Config, result PlanResult) PlanResult {
	if config.OptionAsMetaKeyInstalled {
		result.AlreadyInstalled = true
		result.Messages = append(result.Messages,
			"Terminal.app Option-as-Meta setup is already marked installed; no actions planned.",
		)
		return result
	}

	path := joinPath(platformKey(env.Platform), homeDir(env), "Library", "Preferences", "com.apple.Terminal.plist")
	result.Actions = append(result.Actions, Action{
		ID:          "terminal.app.option-as-meta",
		Title:       "Configure Terminal.app Option-as-Meta",
		Description: `Enable "Use Option as Meta key" and switch the default/startup profiles to visual bell.`,
		Target:      TerminalApple,
	})
	result.FilesToEdit = append(result.FilesToEdit, FileEdit{
		Path:            path,
		Description:     `Set useOptionAsMetaKey=true and Bell=false for Terminal.app default and startup profiles.`,
		CreateIfMissing: false,
		BackupExisting:  true,
		TargetMachine:   "current machine",
	})
	result.ConfigUpdates = append(result.ConfigUpdates, ConfigUpdate{
		Key:    "optionAsMetaKeyInstalled",
		Value:  true,
		Reason: "Terminal.app profile preferences are planned for Option-as-Meta.",
	})
	result.BackupPolicy = BackupPolicy{
		Required: true,
		Summary:  "Back up com.apple.Terminal.plist before editing; if any profile update fails, restore from that backup.",
	}
	result.Messages = append(result.Messages,
		`Enable "Use Option as Meta key" in Apple Terminal so Option+Enter sends a newline.`,
		"Restart Terminal.app after applying the planned preference changes.",
		backslashFallbackMessage(config),
	)
	return result
}

func planEditorKeybinding(env Environment, target Terminal, config Config, result PlanResult) PlanResult {
	snippet := KeybindingSnippet(target)
	path := editorKeybindingsPath(env, target)
	result.Snippet = snippet
	result.Actions = append(result.Actions, Action{
		ID:          fmt.Sprintf("%s.shift-enter-keybinding", target),
		Title:       fmt.Sprintf("Install %s terminal Shift+Enter keybinding", targetDisplayName(target)),
		Description: "Send ESC+Return to the integrated terminal when Shift+Enter is pressed.",
		Target:      target,
	})
	result.FilesToEdit = append(result.FilesToEdit, FileEdit{
		Path:            path,
		Description:     "Add the Shift+Enter terminal sendSequence keybinding to keybindings.json.",
		Snippet:         snippet,
		CreateIfMissing: true,
		BackupExisting:  true,
		TargetMachine:   "current machine",
	})
	result.ConfigUpdates = append(result.ConfigUpdates, shiftEnterConfigUpdate(target))
	result.BackupPolicy = backupExistingPolicy(path)
	result.Messages = append(result.Messages,
		fmt.Sprintf("Add a %s integrated terminal Shift+Enter keybinding.", targetDisplayName(target)),
		backslashFallbackMessage(config),
	)
	return result
}

func planRemoteEditor(_ Environment, target Terminal, config Config, result PlanResult) PlanResult {
	snippet := KeybindingSnippet(target)
	path := fmt.Sprintf("local %s Keyboard Shortcuts JSON", targetDisplayName(target))
	result.RequiresLocalMachine = true
	result.Snippet = snippet
	result.Actions = append(result.Actions, Action{
		ID:          fmt.Sprintf("%s.local-shift-enter-keybinding", target),
		Title:       fmt.Sprintf("Install %s keybinding on the local machine", targetDisplayName(target)),
		Description: "Remote SSH sessions cannot modify local editor keybindings from the remote server.",
		Target:      target,
		Manual:      true,
	})
	result.FilesToEdit = append(result.FilesToEdit, FileEdit{
		Path:            path,
		Description:     "Open the local Keyboard Shortcuts JSON file and add this Shift+Enter terminal binding.",
		Snippet:         snippet,
		CreateIfMissing: true,
		BackupExisting:  true,
		TargetMachine:   "local machine",
	})
	result.BackupPolicy = BackupPolicy{
		Required: true,
		Summary:  "On the local machine, back up the existing editor keybindings JSON before editing; if backup fails, stop.",
	}
	result.Messages = append(result.Messages,
		fmt.Sprintf("Cannot install keybindings from a remote %s session.", targetDisplayName(target)),
		fmt.Sprintf("%s keybindings must be installed on your local machine, not the remote server.", targetDisplayName(target)),
		backslashFallbackMessage(config),
	)
	return result
}

func planZedKeybinding(env Environment, config Config, result PlanResult) PlanResult {
	snippet := KeybindingSnippet(TerminalZed)
	path := joinPath(platformKey(env.Platform), homeDir(env), ".config", "zed", "keymap.json")
	result.Snippet = snippet
	result.Actions = append(result.Actions, Action{
		ID:          "zed.shift-enter-keybinding",
		Title:       "Install Zed terminal Shift+Enter keybinding",
		Description: "Send ESC+Return to the Zed terminal when Shift+Enter is pressed.",
		Target:      TerminalZed,
	})
	result.FilesToEdit = append(result.FilesToEdit, FileEdit{
		Path:            path,
		Description:     "Add a Terminal-context shift-enter binding to keymap.json.",
		Snippet:         snippet,
		CreateIfMissing: true,
		BackupExisting:  true,
		TargetMachine:   "current machine",
	})
	result.ConfigUpdates = append(result.ConfigUpdates, shiftEnterConfigUpdate(TerminalZed))
	result.BackupPolicy = backupExistingPolicy(path)
	result.Messages = append(result.Messages,
		"Add a Zed Terminal-context Shift+Enter keybinding.",
		backslashFallbackMessage(config),
	)
	return result
}

func planAlacrittyKeybinding(env Environment, config Config, result PlanResult) PlanResult {
	snippet := KeybindingSnippet(TerminalAlacritty)
	paths := alacrittyConfigPaths(env)
	path := ""
	if len(paths) > 0 {
		path = paths[0]
	}
	result.Snippet = snippet
	result.Actions = append(result.Actions, Action{
		ID:          "alacritty.shift-enter-keybinding",
		Title:       "Install Alacritty terminal Shift+Enter keybinding",
		Description: "Send ESC+Return from Alacritty when Shift+Enter is pressed.",
		Target:      TerminalAlacritty,
	})
	result.FilesToEdit = append(result.FilesToEdit, FileEdit{
		Path:             path,
		AlternativePaths: paths[1:],
		Description:      "Append a Shift+Return keyboard binding to alacritty.toml.",
		Snippet:          snippet,
		CreateIfMissing:  true,
		BackupExisting:   true,
		TargetMachine:    "current machine",
	})
	result.ConfigUpdates = append(result.ConfigUpdates, shiftEnterConfigUpdate(TerminalAlacritty))
	result.BackupPolicy = backupExistingPolicy(path)
	result.Messages = append(result.Messages,
		"Add an Alacritty Shift+Return binding and restart Alacritty if needed.",
		backslashFallbackMessage(config),
	)
	return result
}

func shiftEnterConfigUpdate(target Terminal) ConfigUpdate {
	return ConfigUpdate{
		Key:    "shiftEnterKeyBindingInstalled",
		Value:  true,
		Reason: fmt.Sprintf("%s Shift+Enter keybinding is planned.", targetDisplayName(target)),
	}
}

func backupExistingPolicy(path string) BackupPolicy {
	if path == "" {
		return BackupPolicy{
			Required: true,
			Summary:  "Back up the existing target file before editing; if backup fails, stop.",
		}
	}
	return BackupPolicy{
		Required: true,
		Summary:  fmt.Sprintf("If %s exists, copy it to %s.<random>.bak before editing; if backup fails, stop.", path, path),
	}
}

func unsupportedMessages(env Environment, config Config) []string {
	name := terminalDisplayName(env.Terminal)
	messages := []string{
		fmt.Sprintf("Terminal setup cannot be run from %s.", name),
		"This planner configures an optional Shift+Enter shortcut for multi-line prompts.",
		backslashFallbackMessage(config),
	}

	if isMultiplexer(env.Terminal) {
		messages = append(messages, "Exit tmux/screen temporarily, run terminal setup directly in a supported terminal, then return; settings will persist.")
	} else {
		messages = append(messages, "Run terminal setup directly in a supported terminal.")
	}

	switch platformKey(env.Platform) {
	case "darwin":
		messages = append(messages, "Supported macOS terminal setup target: Apple Terminal.")
	case "windows":
		messages = append(messages, "On Windows, use an IDE terminal target for keybinding setup.")
	}

	messages = append(messages,
		"Supported IDE targets: VSCode, Cursor, Windsurf, Zed.",
		"Supported terminal config target: Alacritty.",
		"Native no-setup terminals: iTerm2, WezTerm, Ghostty, Kitty, Warp.",
	)
	return messages
}

func backslashFallbackMessage(config Config) string {
	if config.HasUsedBackslashReturn {
		return `Backslash (\) + Return was already used as a newline fallback; Shift+Enter setup remains optional.`
	}
	return `You can already use backslash (\) + Return as a newline fallback while setup is pending.`
}

func isRemoteEditorSession(env Environment) bool {
	askpass := envValue(env, "VSCODE_GIT_ASKPASS_MAIN")
	path := envValue(env, "PATH")
	return containsAny(askpass, ".vscode-server", ".cursor-server", ".windsurf-server") ||
		containsAny(path, ".vscode-server", ".cursor-server", ".windsurf-server")
}

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func isVSCodeFamily(target Terminal) bool {
	switch normalizeTarget(target) {
	case TerminalVSCode, TerminalCursor, TerminalWindsurf:
		return true
	default:
		return false
	}
}

func isMultiplexer(terminal string) bool {
	lower := strings.ToLower(strings.TrimSpace(terminal))
	return strings.Contains(lower, "tmux") || strings.Contains(lower, "screen")
}

func supportedKeybindingTarget(terminal string) (Terminal, bool) {
	switch strings.ToLower(strings.TrimSpace(terminal)) {
	case "vscode", "code":
		return TerminalVSCode, true
	case "cursor":
		return TerminalCursor, true
	case "windsurf":
		return TerminalWindsurf, true
	case "alacritty":
		return TerminalAlacritty, true
	case "zed":
		return TerminalZed, true
	default:
		return "", false
	}
}

func normalizeTarget(target Terminal) Terminal {
	normalized, ok := supportedKeybindingTarget(string(target))
	if ok {
		return normalized
	}
	return target
}

func targetDisplayName(target Terminal) string {
	switch normalizeTarget(target) {
	case TerminalVSCode:
		return "VSCode"
	case TerminalCursor:
		return "Cursor"
	case TerminalWindsurf:
		return "Windsurf"
	case TerminalAlacritty:
		return "Alacritty"
	case TerminalZed:
		return "Zed"
	case TerminalApple:
		return "Apple Terminal"
	default:
		if target == "" {
			return "current terminal"
		}
		return string(target)
	}
}

func terminalDisplayName(terminal string) string {
	if display := NativeCSIuDisplayName(terminal); display != "" {
		return display
	}
	if target, ok := supportedKeybindingTarget(terminal); ok {
		return targetDisplayName(target)
	}
	if terminalKey(terminal) == string(TerminalApple) {
		return "Apple Terminal"
	}
	if strings.TrimSpace(terminal) == "" {
		return "your current terminal"
	}
	return terminal
}

func editorKeybindingsPath(env Environment, target Terminal) string {
	editorDir := targetDisplayName(target)
	if target == TerminalVSCode {
		editorDir = "Code"
	}

	platform := platformKey(env.Platform)
	switch platform {
	case "windows":
		appData := envValue(env, "APPDATA")
		if appData == "" {
			appData = "%APPDATA%"
		}
		return joinPath(platform, appData, editorDir, "User", "keybindings.json")
	case "darwin":
		return joinPath(platform, homeDir(env), "Library", "Application Support", editorDir, "User", "keybindings.json")
	default:
		return joinPath(platform, homeDir(env), ".config", editorDir, "User", "keybindings.json")
	}
}

func alacrittyConfigPaths(env Environment) []string {
	platform := platformKey(env.Platform)
	paths := []string{}
	if xdg := envValue(env, "XDG_CONFIG_HOME"); xdg != "" {
		paths = append(paths, joinPath(platform, xdg, "alacritty", "alacritty.toml"))
	} else {
		paths = append(paths, joinPath(platform, homeDir(env), ".config", "alacritty", "alacritty.toml"))
	}
	if platform == "windows" {
		if appData := envValue(env, "APPDATA"); appData != "" {
			paths = append(paths, joinPath(platform, appData, "alacritty", "alacritty.toml"))
		}
	}
	return paths
}

func homeDir(env Environment) string {
	if strings.TrimSpace(env.HomeDir) == "" {
		return "~"
	}
	return env.HomeDir
}

func envValue(env Environment, key string) string {
	if env.Variables == nil {
		return ""
	}
	return env.Variables[key]
}

func platformKey(platform string) string {
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case "macos", "mac", "darwin":
		return "darwin"
	case "win32", "windows", "win":
		return "windows"
	case "linux":
		return "linux"
	default:
		return strings.ToLower(strings.TrimSpace(platform))
	}
}

func terminalKey(terminal string) string {
	return strings.TrimSpace(terminal)
}

func joinPath(platform string, parts ...string) string {
	separator := "/"
	if platform == "windows" {
		separator = `\`
	}

	clean := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		clean = append(clean, strings.TrimRight(part, `/\`))
	}
	return strings.Join(clean, separator)
}
