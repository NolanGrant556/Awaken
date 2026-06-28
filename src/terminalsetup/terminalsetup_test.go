package terminalsetup

import (
	"strings"
	"testing"
)

func TestNativeTerminalNoSetup(t *testing.T) {
	for _, terminal := range []string{"ghostty", "kitty", "iTerm.app", "WezTerm", "WarpTerminal"} {
		result := Plan(Environment{Terminal: terminal, Platform: "darwin"}, Config{})
		if !result.Native {
			t.Fatalf("expected %s to be native", terminal)
		}
		if result.Supported {
			t.Fatalf("expected %s not to need offered setup", terminal)
		}
		if len(result.Actions) != 0 || len(result.ConfigUpdates) != 0 || len(result.FilesToEdit) != 0 {
			t.Fatalf("expected no setup outputs for %s, got %+v", terminal, result)
		}
		if !strings.Contains(joinMessages(result.Messages), "natively supported") {
			t.Fatalf("expected native support message, got %q", result.Messages)
		}
	}

	if got := NativeCSIuDisplayName("iTerm.app"); got != "iTerm2" {
		t.Fatalf("expected iTerm.app display name iTerm2, got %q", got)
	}
	if ShouldOffer(Environment{Terminal: "ghostty", Platform: "darwin"}) {
		t.Fatalf("expected native terminal not to be offered setup")
	}
}

func TestAppleTerminalPlan(t *testing.T) {
	env := Environment{Terminal: "Apple_Terminal", Platform: "darwin", HomeDir: "/Users/tester"}
	result := Plan(env, Config{})

	if !ShouldOffer(env) || !result.Supported {
		t.Fatalf("expected Apple Terminal on darwin to be supported")
	}
	if len(result.Actions) != 1 {
		t.Fatalf("expected one action, got %+v", result.Actions)
	}
	if result.Actions[0].Target != TerminalApple {
		t.Fatalf("expected Apple Terminal action, got %+v", result.Actions[0])
	}
	if !result.BackupPolicy.Required {
		t.Fatalf("expected backup policy to be required")
	}
	if len(result.FilesToEdit) != 1 || !strings.Contains(result.FilesToEdit[0].Path, "com.apple.Terminal.plist") {
		t.Fatalf("expected Terminal plist file edit, got %+v", result.FilesToEdit)
	}
	assertConfigUpdate(t, result.ConfigUpdates, "optionAsMetaKeyInstalled")
	if !strings.Contains(joinMessages(result.Messages), "Option as Meta") {
		t.Fatalf("expected Option-as-Meta message, got %q", result.Messages)
	}
}

func TestVSCodePlanIncludesKeybindingsJSON(t *testing.T) {
	env := Environment{Terminal: "vscode", Platform: "darwin", HomeDir: "/Users/tester"}
	result := Plan(env, Config{})

	if !result.Supported {
		t.Fatalf("expected VSCode to be supported")
	}
	if len(result.FilesToEdit) != 1 {
		t.Fatalf("expected one file edit, got %+v", result.FilesToEdit)
	}
	file := result.FilesToEdit[0]
	if file.Path != "/Users/tester/Library/Application Support/Code/User/keybindings.json" {
		t.Fatalf("unexpected keybindings path: %q", file.Path)
	}
	if !file.BackupExisting || !file.CreateIfMissing {
		t.Fatalf("expected backup and create policy, got %+v", file)
	}
	if !strings.Contains(file.Snippet, `"command": "workbench.action.terminal.sendSequence"`) {
		t.Fatalf("expected VSCode command snippet, got %s", file.Snippet)
	}
	if !strings.Contains(file.Snippet, `"text": "\u001b\r"`) {
		t.Fatalf("expected ESC+Return text snippet, got %s", file.Snippet)
	}
	assertConfigUpdate(t, result.ConfigUpdates, "shiftEnterKeyBindingInstalled")
}

func TestRemoteSSHWarning(t *testing.T) {
	env := Environment{
		Terminal: "vscode",
		Platform: "linux",
		HomeDir:  "/home/remote",
		Variables: map[string]string{
			"VSCODE_GIT_ASKPASS_MAIN": "/home/remote/.vscode-server/bin/askpass.sh",
		},
	}
	result := Plan(env, Config{})

	if !result.RequiresLocalMachine {
		t.Fatalf("expected remote plan to require local machine")
	}
	if len(result.ConfigUpdates) != 0 {
		t.Fatalf("remote plan must not mark config installed, got %+v", result.ConfigUpdates)
	}
	if len(result.Actions) != 1 || !result.Actions[0].Manual {
		t.Fatalf("expected one manual local action, got %+v", result.Actions)
	}
	if result.FilesToEdit[0].TargetMachine != "local machine" {
		t.Fatalf("expected local machine target, got %+v", result.FilesToEdit[0])
	}
	if strings.Contains(result.FilesToEdit[0].Path, "/home/remote") {
		t.Fatalf("remote plan must not point at remote home path, got %+v", result.FilesToEdit[0])
	}
	messages := joinMessages(result.Messages)
	if !strings.Contains(messages, "remote VSCode session") || !strings.Contains(messages, "local machine") {
		t.Fatalf("expected remote/local warning, got %q", result.Messages)
	}
}

func TestUnsupportedTerminalSuggestions(t *testing.T) {
	env := Environment{Terminal: "tmux-256color", Platform: "darwin"}
	result := Plan(env, Config{})

	if result.Supported || ShouldOffer(env) {
		t.Fatalf("expected tmux to be unsupported")
	}
	if len(result.Actions) != 0 {
		t.Fatalf("expected no actions for unsupported terminal, got %+v", result.Actions)
	}
	messages := joinMessages(result.Messages)
	for _, want := range []string{"cannot be run", "tmux/screen", "Apple Terminal", "VSCode", "iTerm2"} {
		if !strings.Contains(messages, want) {
			t.Fatalf("expected unsupported message to contain %q, got %q", want, messages)
		}
	}
}

func TestInstalledConfigSkipsAction(t *testing.T) {
	result := Plan(Environment{Terminal: "cursor", Platform: "darwin"}, Config{ShiftEnterKeyBindingInstalled: true})

	if !result.AlreadyInstalled {
		t.Fatalf("expected installed flag")
	}
	if len(result.Actions) != 0 || len(result.FilesToEdit) != 0 || len(result.ConfigUpdates) != 0 {
		t.Fatalf("expected installed config to skip actions, got %+v", result)
	}
	if !strings.Contains(joinMessages(result.Messages), "already marked installed") {
		t.Fatalf("expected already-installed message, got %q", result.Messages)
	}
}

func TestBackslashFallbackMessage(t *testing.T) {
	result := Plan(Environment{Terminal: "screen", Platform: "linux"}, Config{HasUsedBackslashReturn: true})
	messages := joinMessages(result.Messages)

	if !strings.Contains(messages, `Backslash (\) + Return`) {
		t.Fatalf("expected backslash fallback message, got %q", messages)
	}
	if !strings.Contains(messages, "already used") {
		t.Fatalf("expected used fallback state, got %q", messages)
	}
}

func TestZedAndAlacrittySnippets(t *testing.T) {
	zed := KeybindingSnippet(TerminalZed)
	if !strings.Contains(zed, `"context": "Terminal"`) || !strings.Contains(zed, "terminal::SendText") {
		t.Fatalf("unexpected Zed snippet: %s", zed)
	}

	alacritty := KeybindingSnippet(TerminalAlacritty)
	if !strings.Contains(alacritty, "[[keyboard.bindings]]") || !strings.Contains(alacritty, `chars = "\u001B\r"`) {
		t.Fatalf("unexpected Alacritty snippet: %s", alacritty)
	}

	zedPlan := Plan(Environment{Terminal: "zed", Platform: "linux", HomeDir: "/home/tester"}, Config{})
	if zedPlan.FilesToEdit[0].Path != "/home/tester/.config/zed/keymap.json" {
		t.Fatalf("unexpected Zed path: %+v", zedPlan.FilesToEdit)
	}

	alacrittyPlan := Plan(Environment{Terminal: "alacritty", Platform: "linux", HomeDir: "/home/tester"}, Config{})
	if alacrittyPlan.FilesToEdit[0].Path != "/home/tester/.config/alacritty/alacritty.toml" {
		t.Fatalf("unexpected Alacritty path: %+v", alacrittyPlan.FilesToEdit)
	}
}

func assertConfigUpdate(t *testing.T, updates []ConfigUpdate, key string) {
	t.Helper()
	for _, update := range updates {
		if update.Key == key && update.Value {
			return
		}
	}
	t.Fatalf("expected config update %q=true, got %+v", key, updates)
}

func joinMessages(messages []string) string {
	return strings.Join(messages, "\n")
}
