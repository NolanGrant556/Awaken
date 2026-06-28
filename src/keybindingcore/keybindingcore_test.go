package keybindingcore

import "testing"

func TestParseKeystrokeModifierAliasesAndKeys(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  Keystroke
	}{
		{
			name:  "ctrl/control and uppercase key",
			input: "Control+Shift+K",
			want:  Keystroke{Key: "k", Ctrl: true, Shift: true},
		},
		{
			name:  "option alias",
			input: "option+esc",
			want:  Keystroke{Key: "escape", Alt: true},
		},
		{
			name:  "command alias and space",
			input: "command+space",
			want:  Keystroke{Key: " ", Super: true},
		},
		{
			name:  "return alias",
			input: "return",
			want:  Keystroke{Key: "enter"},
		},
		{
			name:  "arrow symbol",
			input: "ctrl+↑",
			want:  Keystroke{Key: "up", Ctrl: true},
		},
		{
			name:  "win alias",
			input: "win+left",
			want:  Keystroke{Key: "left", Super: true},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ParseKeystroke(test.input)
			if err != nil {
				t.Fatalf("ParseKeystroke returned error: %v", err)
			}
			if got != test.want {
				t.Fatalf("ParseKeystroke(%q) = %#v, want %#v", test.input, got, test.want)
			}
		})
	}
}

func TestParseChordSpaceAndMultipleKeystrokes(t *testing.T) {
	space, err := ParseChord(" ")
	if err != nil {
		t.Fatalf("ParseChord space returned error: %v", err)
	}
	if len(space) != 1 || space[0].Key != " " {
		t.Fatalf("ParseChord space = %#v, want one space keystroke", space)
	}

	chord, err := ParseChord("ctrl+x ctrl+k")
	if err != nil {
		t.Fatalf("ParseChord chord returned error: %v", err)
	}
	if got := ChordToString(chord); got != "ctrl+x ctrl+k" {
		t.Fatalf("ChordToString = %q, want %q", got, "ctrl+x ctrl+k")
	}
}

func TestDisplayStrings(t *testing.T) {
	ks := mustParseKeystroke(t, "alt+cmd+space")
	if got := KeystrokeToString(ks); got != "alt+cmd+Space" {
		t.Fatalf("KeystrokeToString = %q, want %q", got, "alt+cmd+Space")
	}
	if got := KeystrokeToDisplayString(ks, PlatformMacOS); got != "opt+cmd+Space" {
		t.Fatalf("macOS display = %q, want %q", got, "opt+cmd+Space")
	}
	if got := KeystrokeToDisplayString(ks, PlatformLinux); got != "alt+super+Space" {
		t.Fatalf("linux display = %q, want %q", got, "alt+super+Space")
	}

	chord := mustParseChord(t, "alt+→ super+tab")
	if got := ChordToDisplayString(chord, PlatformMacOS); got != "opt+→ cmd+tab" {
		t.Fatalf("ChordToDisplayString = %q, want %q", got, "opt+→ cmd+tab")
	}
}

func TestResolveChordPrefixExactAndNullOverride(t *testing.T) {
	bindings := []Binding{
		binding(t, "Chat", "ctrl+x", "chat:externalEditor"),
		binding(t, "Chat", "ctrl+x ctrl+k", "chat:killAgents"),
	}

	start := ResolveChordState(InputKey{Key: "x", Ctrl: true}, []string{"Chat"}, bindings, nil)
	if start.Type != ResolveChordStarted {
		t.Fatalf("first key result = %#v, want chord_started", start)
	}
	if got := ChordToString(start.Pending); got != "ctrl+x" {
		t.Fatalf("pending = %q, want ctrl+x", got)
	}

	match := ResolveChordState(InputKey{Key: "k", Ctrl: true}, []string{"Chat"}, bindings, start.Pending)
	if match.Type != ResolveMatch || match.Action != "chat:killAgents" {
		t.Fatalf("second key result = %#v, want chat:killAgents match", match)
	}

	nullOverride := []Binding{
		binding(t, "Chat", "ctrl+x", "chat:externalEditor"),
		binding(t, "Chat", "ctrl+x ctrl+k", "chat:killAgents"),
		nullBinding(t, "Chat", "ctrl+x ctrl+k"),
	}
	exact := ResolveChordState(InputKey{Key: "x", Ctrl: true}, []string{"Chat"}, nullOverride, nil)
	if exact.Type != ResolveMatch || exact.Action != "chat:externalEditor" {
		t.Fatalf("null override result = %#v, want exact prefix binding to win", exact)
	}
}

func TestResolveNullOverrideUnbindsExact(t *testing.T) {
	bindings := []Binding{
		binding(t, "Chat", "ctrl+s", "chat:stash"),
		nullBinding(t, "Chat", "ctrl+s"),
	}

	got := ResolveKey(InputKey{Key: "s", Ctrl: true}, []string{"Chat"}, bindings)
	if got.Type != ResolveUnbound {
		t.Fatalf("ResolveKey = %#v, want unbound", got)
	}
}

func TestResolveContextLastWins(t *testing.T) {
	bindings := []Binding{
		binding(t, "Global", "enter", "app:redraw"),
		binding(t, "Chat", "enter", "chat:submit"),
		binding(t, "Global", "enter", "app:quickOpen"),
	}

	got := ResolveKey(InputKey{Key: "enter"}, []string{"Global", "Chat"}, bindings)
	if got.Type != ResolveMatch || got.Action != "app:quickOpen" {
		t.Fatalf("ResolveKey = %#v, want last active binding", got)
	}
}

func TestResolveAltMetaEquivalentAndSuperDistinct(t *testing.T) {
	metaBindings := []Binding{binding(t, "Chat", "meta+k", "chat:modelPicker")}
	got := ResolveKey(InputKey{Key: "k", Alt: true}, []string{"Chat"}, metaBindings)
	if got.Type != ResolveMatch || got.Action != "chat:modelPicker" {
		t.Fatalf("alt input should match meta binding, got %#v", got)
	}

	superBindings := []Binding{binding(t, "Chat", "super+k", "chat:fastMode")}
	got = ResolveKey(InputKey{Key: "k", Meta: true}, []string{"Chat"}, superBindings)
	if got.Type != ResolveNone {
		t.Fatalf("meta input should not match super binding, got %#v", got)
	}

	got = ResolveKey(InputKey{Key: "k", Super: true}, []string{"Chat"}, superBindings)
	if got.Type != ResolveMatch || got.Action != "chat:fastMode" {
		t.Fatalf("super input should match super binding, got %#v", got)
	}
}

func TestResolveEscapeDoesNotInheritMetaAndCancelsPendingChord(t *testing.T) {
	bindings := []Binding{binding(t, "Chat", "escape", "chat:cancel")}
	got := ResolveKey(InputKey{Key: "escape", Meta: true}, []string{"Chat"}, bindings)
	if got.Type != ResolveMatch || got.Action != "chat:cancel" {
		t.Fatalf("escape with terminal meta should match bare escape, got %#v", got)
	}

	pending := mustParseChord(t, "ctrl+x")
	cancelled := ResolveChordState(InputKey{Key: "escape", Meta: true}, []string{"Chat"}, bindings, pending)
	if cancelled.Type != ResolveChordCancelled {
		t.Fatalf("escape while pending = %#v, want chord_cancelled", cancelled)
	}
}

func TestValidateBlocksContextActionAndCommandRules(t *testing.T) {
	issues := ValidateBlocks([]Block{
		{
			Context: "Unknown",
			Bindings: map[string]*string{
				"ctrl+k": Action("chat:submit"),
			},
		},
		{
			Context: "Chat",
			Bindings: map[string]*string{
				"ctrl+j": Action("bad:action"),
				"ctrl+l": Action("command:help!"),
			},
		},
		{
			Context: "Global",
			Bindings: map[string]*string{
				"ctrl+o": Action("command:help"),
			},
		},
	})

	assertIssue(t, issues, IssueInvalidContext, SeverityError, "", "Unknown", "")
	assertIssue(t, issues, IssueInvalidAction, SeverityError, "ctrl+j", "Chat", "bad:action")
	assertIssue(t, issues, IssueInvalidAction, SeverityWarning, "ctrl+l", "Chat", "command:help!")
	assertIssue(t, issues, IssueInvalidAction, SeverityWarning, "ctrl+o", "Global", "command:help")
}

func TestValidateReservedShortcuts(t *testing.T) {
	issues := ValidateBlocks([]Block{
		{
			Context: "Chat",
			Bindings: map[string]*string{
				"ctrl+c": Action("chat:cancel"),
				"ctrl+d": Action("chat:cancel"),
				"ctrl+m": Action("chat:submit"),
				"ctrl+z": Action("chat:cancel"),
			},
		},
	})

	assertIssue(t, issues, IssueReserved, SeverityError, "ctrl+c", "Chat", "chat:cancel")
	assertIssue(t, issues, IssueReserved, SeverityError, "ctrl+d", "Chat", "chat:cancel")
	assertIssue(t, issues, IssueReserved, SeverityError, "ctrl+m", "Chat", "chat:submit")
	assertIssue(t, issues, IssueReserved, SeverityWarning, "ctrl+z", "Chat", "chat:cancel")
}

func TestValidateMacOSReservedShortcuts(t *testing.T) {
	issues := ValidateBlocks([]Block{
		{
			Context: "Global",
			Bindings: map[string]*string{
				"cmd+c":     Action("app:quickOpen"),
				"command+v": Action("app:quickOpen"),
				"super+x":   Action("app:quickOpen"),
				"win+q":     Action("app:quickOpen"),
				"cmd+w":     Action("app:quickOpen"),
				"cmd+tab":   Action("app:quickOpen"),
				"cmd+space": Action("app:quickOpen"),
			},
		},
	}, PlatformMacOS)

	for _, key := range []string{"cmd+c", "cmd+v", "cmd+x", "cmd+q", "cmd+w", "cmd+tab", "cmd+Space"} {
		assertIssue(t, issues, IssueReserved, SeverityError, key, "Global", "app:quickOpen")
	}
}

func mustParseKeystroke(t *testing.T, input string) Keystroke {
	t.Helper()
	ks, err := ParseKeystroke(input)
	if err != nil {
		t.Fatalf("ParseKeystroke(%q): %v", input, err)
	}
	return ks
}

func mustParseChord(t *testing.T, input string) Chord {
	t.Helper()
	chord, err := ParseChord(input)
	if err != nil {
		t.Fatalf("ParseChord(%q): %v", input, err)
	}
	return chord
}

func binding(t *testing.T, context, key, action string) Binding {
	t.Helper()
	return Binding{
		Chord:   mustParseChord(t, key),
		Action:  Action(action),
		Context: context,
	}
}

func nullBinding(t *testing.T, context, key string) Binding {
	t.Helper()
	return Binding{
		Chord:   mustParseChord(t, key),
		Context: context,
	}
}

func assertIssue(t *testing.T, issues []ValidationIssue, issueType IssueType, severity Severity, key, context, action string) {
	t.Helper()
	for _, issue := range issues {
		if issue.Type != issueType || issue.Severity != severity || issue.Context != context {
			continue
		}
		if key != "" && issue.Key != key {
			continue
		}
		if action != "" && issue.Action != action {
			continue
		}
		return
	}
	t.Fatalf("missing issue type=%s severity=%s key=%q context=%q action=%q in %#v", issueType, severity, key, context, action, issues)
}
