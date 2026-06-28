package keybindingcore

import (
	"strings"
	"unicode/utf8"
)

func ResolveKey(input InputKey, activeContexts []string, bindings []Binding) ResolveResult {
	ks, ok := KeystrokeFromInput(input)
	if !ok {
		return ResolveResult{Type: ResolveNone}
	}

	contexts := contextSet(activeContexts)
	matchIndex := -1
	for i := range bindings {
		binding := bindings[i]
		if len(binding.Chord) != 1 || !contexts[binding.Context] {
			continue
		}
		if KeystrokesEqual(ks, binding.Chord[0]) {
			matchIndex = i
		}
	}

	if matchIndex == -1 {
		return ResolveResult{Type: ResolveNone}
	}
	return bindingResult(bindings[matchIndex])
}

func ResolveChordState(input InputKey, activeContexts []string, bindings []Binding, pending Chord) ResolveResult {
	if key, ok := inputKeyName(input); ok && key == "escape" && pending != nil {
		return ResolveResult{Type: ResolveChordCancelled}
	}

	current, ok := KeystrokeFromInput(input)
	if !ok {
		if pending != nil {
			return ResolveResult{Type: ResolveChordCancelled}
		}
		return ResolveResult{Type: ResolveNone}
	}

	testChord := make(Chord, 0, len(pending)+1)
	if pending != nil {
		testChord = append(testChord, pending...)
	}
	testChord = append(testChord, current)

	contexts := contextSet(activeContexts)
	chordWinners := map[string]*string{}
	for i := range bindings {
		binding := bindings[i]
		if !contexts[binding.Context] {
			continue
		}
		if len(binding.Chord) > len(testChord) && chordPrefixMatches(testChord, binding.Chord) {
			chordWinners[chordComparisonKey(binding.Chord)] = binding.Action
		}
	}

	for _, action := range chordWinners {
		if action != nil {
			return ResolveResult{
				Type:    ResolveChordStarted,
				Pending: testChord,
			}
		}
	}

	matchIndex := -1
	for i := range bindings {
		binding := bindings[i]
		if !contexts[binding.Context] {
			continue
		}
		if chordExactlyMatches(testChord, binding.Chord) {
			matchIndex = i
		}
	}

	if matchIndex != -1 {
		return bindingResult(bindings[matchIndex])
	}

	if pending != nil {
		return ResolveResult{Type: ResolveChordCancelled}
	}
	return ResolveResult{Type: ResolveNone}
}

func KeystrokeFromInput(input InputKey) (Keystroke, bool) {
	key, ok := inputKeyName(input)
	if !ok {
		return Keystroke{}, false
	}

	alt := input.Alt
	meta := input.Meta
	if key == "escape" {
		alt = false
		meta = false
	}

	return Keystroke{
		Key:   key,
		Ctrl:  input.Ctrl,
		Alt:   alt,
		Shift: input.Shift,
		Meta:  meta,
		Super: input.Super,
	}, true
}

func KeystrokesEqual(a, b Keystroke) bool {
	return a.Key == b.Key &&
		a.Ctrl == b.Ctrl &&
		a.Shift == b.Shift &&
		(a.Alt || a.Meta) == (b.Alt || b.Meta) &&
		a.Super == b.Super
}

func chordPrefixMatches(prefix, chord Chord) bool {
	if len(prefix) >= len(chord) {
		return false
	}
	for i := range prefix {
		if !KeystrokesEqual(prefix[i], chord[i]) {
			return false
		}
	}
	return true
}

func chordExactlyMatches(candidate, chord Chord) bool {
	if len(candidate) != len(chord) {
		return false
	}
	for i := range candidate {
		if !KeystrokesEqual(candidate[i], chord[i]) {
			return false
		}
	}
	return true
}

func bindingResult(binding Binding) ResolveResult {
	if binding.Action == nil {
		return ResolveResult{Type: ResolveUnbound}
	}
	return ResolveResult{Type: ResolveMatch, Action: *binding.Action}
}

func contextSet(activeContexts []string) map[string]bool {
	contexts := make(map[string]bool, len(activeContexts))
	for _, context := range activeContexts {
		contexts[context] = true
	}
	return contexts
}

func inputKeyName(input InputKey) (string, bool) {
	if input.Key != "" {
		return normalizeKeyName(strings.ToLower(input.Key)), true
	}

	switch input.Input {
	case " ":
		return " ", true
	case "\x1b":
		return "escape", true
	case "\r", "\n":
		return "enter", true
	case "\t":
		return "tab", true
	}

	if utf8.RuneCountInString(input.Input) == 1 {
		return strings.ToLower(input.Input), true
	}
	return "", false
}

func chordComparisonKey(chord Chord) string {
	parts := make([]string, 0, len(chord))
	for _, ks := range chord {
		parts = append(parts, keystrokeComparisonKey(ks))
	}
	return strings.Join(parts, " ")
}

func keystrokeComparisonKey(ks Keystroke) string {
	parts := make([]string, 0, 5)
	if ks.Ctrl {
		parts = append(parts, "ctrl")
	}
	if ks.Alt || ks.Meta {
		parts = append(parts, "altmeta")
	}
	if ks.Shift {
		parts = append(parts, "shift")
	}
	if ks.Super {
		parts = append(parts, "super")
	}
	parts = append(parts, ks.Key)
	return strings.Join(parts, "+")
}
