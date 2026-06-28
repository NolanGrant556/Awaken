package keybindingcore

import (
	"fmt"
	"strings"
)

func ParseKeystroke(input string) (Keystroke, error) {
	if input == "" {
		return Keystroke{}, fmt.Errorf("empty keystroke")
	}

	var ks Keystroke
	parts := strings.Split(input, "+")
	for _, raw := range parts {
		part := strings.ToLower(strings.TrimSpace(raw))
		if part == "" {
			return Keystroke{}, fmt.Errorf("empty key part in %q", input)
		}

		switch part {
		case "ctrl", "control":
			ks.Ctrl = true
		case "alt", "opt", "option":
			ks.Alt = true
		case "shift":
			ks.Shift = true
		case "meta":
			ks.Meta = true
		case "cmd", "command", "super", "win":
			ks.Super = true
		default:
			if ks.Key != "" {
				return Keystroke{}, fmt.Errorf("multiple keys in %q", input)
			}
			ks.Key = normalizeKeyName(part)
		}
	}

	if ks.Key == "" {
		return Keystroke{}, fmt.Errorf("missing key in %q", input)
	}

	return ks, nil
}

func ParseChord(input string) (Chord, error) {
	if input == " " {
		ks, err := ParseKeystroke("space")
		if err != nil {
			return nil, err
		}
		return Chord{ks}, nil
	}

	parts := strings.Fields(input)
	if len(parts) == 0 {
		return nil, fmt.Errorf("empty chord")
	}

	chord := make(Chord, 0, len(parts))
	for _, part := range parts {
		ks, err := ParseKeystroke(part)
		if err != nil {
			return nil, err
		}
		chord = append(chord, ks)
	}
	return chord, nil
}

func ParseBindings(blocks []Block) ([]Binding, error) {
	bindings := make([]Binding, 0)
	for _, block := range blocks {
		for key, action := range block.Bindings {
			chord, err := ParseChord(key)
			if err != nil {
				return nil, fmt.Errorf("parse %q in %s: %w", key, block.Context, err)
			}
			bindings = append(bindings, Binding{
				Chord:   chord,
				Action:  action,
				Context: block.Context,
			})
		}
	}
	return bindings, nil
}

func KeystrokeToString(ks Keystroke) string {
	parts := make([]string, 0, 6)
	if ks.Ctrl {
		parts = append(parts, "ctrl")
	}
	if ks.Alt {
		parts = append(parts, "alt")
	}
	if ks.Shift {
		parts = append(parts, "shift")
	}
	if ks.Meta {
		parts = append(parts, "meta")
	}
	if ks.Super {
		parts = append(parts, "cmd")
	}
	if ks.Key != "" {
		parts = append(parts, keyToDisplayName(ks.Key))
	}
	return strings.Join(parts, "+")
}

func ChordToString(chord Chord) string {
	parts := make([]string, 0, len(chord))
	for _, ks := range chord {
		parts = append(parts, KeystrokeToString(ks))
	}
	return strings.Join(parts, " ")
}

func KeystrokeToDisplayString(ks Keystroke, platform Platform) string {
	parts := make([]string, 0, 5)
	if ks.Ctrl {
		parts = append(parts, "ctrl")
	}
	if ks.Alt || ks.Meta {
		if platform == PlatformMacOS {
			parts = append(parts, "opt")
		} else {
			parts = append(parts, "alt")
		}
	}
	if ks.Shift {
		parts = append(parts, "shift")
	}
	if ks.Super {
		if platform == PlatformMacOS {
			parts = append(parts, "cmd")
		} else {
			parts = append(parts, "super")
		}
	}
	if ks.Key != "" {
		parts = append(parts, keyToDisplayName(ks.Key))
	}
	return strings.Join(parts, "+")
}

func ChordToDisplayString(chord Chord, platform Platform) string {
	parts := make([]string, 0, len(chord))
	for _, ks := range chord {
		parts = append(parts, KeystrokeToDisplayString(ks, platform))
	}
	return strings.Join(parts, " ")
}

func normalizeKeyName(key string) string {
	switch key {
	case "esc":
		return "escape"
	case "return":
		return "enter"
	case "space":
		return " "
	case "↑", "arrowup":
		return "up"
	case "↓", "arrowdown":
		return "down"
	case "←", "arrowleft":
		return "left"
	case "→", "arrowright":
		return "right"
	default:
		return key
	}
}

func keyToDisplayName(key string) string {
	switch key {
	case "escape":
		return "Esc"
	case " ":
		return "Space"
	case "enter":
		return "Enter"
	case "backspace":
		return "Backspace"
	case "delete":
		return "Delete"
	case "up":
		return "↑"
	case "down":
		return "↓"
	case "left":
		return "←"
	case "right":
		return "→"
	case "pageup":
		return "PageUp"
	case "pagedown":
		return "PageDown"
	case "home":
		return "Home"
	case "end":
		return "End"
	default:
		return key
	}
}
