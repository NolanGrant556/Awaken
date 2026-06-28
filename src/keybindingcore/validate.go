package keybindingcore

import (
	"fmt"
	"regexp"
)

var commandActionPattern = regexp.MustCompile(`^command:[a-zA-Z0-9:\-_]+$`)

func ValidateBlocks(blocks []Block, platforms ...Platform) []ValidationIssue {
	platform := PlatformUnknown
	if len(platforms) > 0 {
		platform = platforms[0]
	}

	var issues []ValidationIssue
	for blockIndex, block := range blocks {
		contextValid := validContexts[block.Context]
		if !contextValid {
			issues = append(issues, ValidationIssue{
				Type:       IssueInvalidContext,
				Severity:   SeverityError,
				Message:    fmt.Sprintf("Unknown context %q", block.Context),
				Context:    block.Context,
				Suggestion: "Use one of the supported keybinding contexts",
			})
		}

		if block.Bindings == nil {
			issues = append(issues, ValidationIssue{
				Type:     IssueParseError,
				Severity: SeverityError,
				Message:  fmt.Sprintf("Keybinding block %d missing bindings", blockIndex+1),
				Context:  block.Context,
			})
			continue
		}

		for key, action := range block.Bindings {
			chord, err := ParseChord(key)
			if err != nil {
				issues = append(issues, ValidationIssue{
					Type:     IssueParseError,
					Severity: SeverityError,
					Message:  err.Error(),
					Key:      key,
					Context:  block.Context,
				})
				continue
			}

			if action != nil {
				issues = append(issues, validateAction(key, block.Context, contextValid, *action)...)
			}

			issues = append(issues, validateReservedShortcut(Binding{
				Chord:   chord,
				Action:  action,
				Context: block.Context,
			}, platform)...)
		}
	}
	return issues
}

func validateAction(key, context string, contextValid bool, action string) []ValidationIssue {
	if commandAction(action) {
		var issues []ValidationIssue
		if !commandActionPattern.MatchString(action) {
			issues = append(issues, ValidationIssue{
				Type:     IssueInvalidAction,
				Severity: SeverityWarning,
				Message:  fmt.Sprintf("Invalid command binding %q", action),
				Key:      key,
				Context:  context,
				Action:   action,
			})
		}
		if contextValid && context != "Chat" {
			issues = append(issues, ValidationIssue{
				Type:       IssueInvalidAction,
				Severity:   SeverityWarning,
				Message:    fmt.Sprintf("Command binding %q must be in Chat context", action),
				Key:        key,
				Context:    context,
				Action:     action,
				Suggestion: `Move this binding to a block with context "Chat"`,
			})
		}
		return issues
	}

	if !validActions[action] {
		return []ValidationIssue{{
			Type:     IssueInvalidAction,
			Severity: SeverityError,
			Message:  fmt.Sprintf("Invalid action %q", action),
			Key:      key,
			Context:  context,
			Action:   action,
		}}
	}
	return nil
}

func commandAction(action string) bool {
	return len(action) >= len("command:") && action[:len("command:")] == "command:"
}

func validateReservedShortcut(binding Binding, platform Platform) []ValidationIssue {
	var issues []ValidationIssue
	for _, reserved := range reservedShortcuts(platform) {
		chord, err := ParseChord(reserved.key)
		if err != nil {
			continue
		}
		if chordExactlyMatches(binding.Chord, chord) {
			action := ""
			if binding.Action != nil {
				action = *binding.Action
			}
			keyDisplay := ChordToString(binding.Chord)
			issues = append(issues, ValidationIssue{
				Type:     IssueReserved,
				Severity: reserved.severity,
				Message:  fmt.Sprintf("%q may not work: %s", keyDisplay, reserved.reason),
				Key:      keyDisplay,
				Context:  binding.Context,
				Action:   action,
			})
		}
	}
	return issues
}

type reservedShortcut struct {
	key      string
	reason   string
	severity Severity
}

func reservedShortcuts(platform Platform) []reservedShortcut {
	shortcuts := []reservedShortcut{
		{
			key:      "ctrl+c",
			reason:   "Cannot be rebound - used for interrupt/exit",
			severity: SeverityError,
		},
		{
			key:      "ctrl+d",
			reason:   "Cannot be rebound - used for exit",
			severity: SeverityError,
		},
		{
			key:      "ctrl+m",
			reason:   "Cannot be rebound - identical to Enter in terminals",
			severity: SeverityError,
		},
		{
			key:      "ctrl+z",
			reason:   "Unix process suspend",
			severity: SeverityWarning,
		},
	}

	if platform == PlatformMacOS {
		shortcuts = append(shortcuts,
			reservedShortcut{key: "cmd+c", reason: "macOS system copy", severity: SeverityError},
			reservedShortcut{key: "cmd+v", reason: "macOS system paste", severity: SeverityError},
			reservedShortcut{key: "cmd+x", reason: "macOS system cut", severity: SeverityError},
			reservedShortcut{key: "cmd+q", reason: "macOS quit application", severity: SeverityError},
			reservedShortcut{key: "cmd+w", reason: "macOS close window/tab", severity: SeverityError},
			reservedShortcut{key: "cmd+tab", reason: "macOS app switcher", severity: SeverityError},
			reservedShortcut{key: "cmd+space", reason: "macOS Spotlight", severity: SeverityError},
		)
	}
	return shortcuts
}

var validContexts = map[string]bool{
	"Global":          true,
	"Chat":            true,
	"Autocomplete":    true,
	"Confirmation":    true,
	"Help":            true,
	"Transcript":      true,
	"HistorySearch":   true,
	"Task":            true,
	"ThemePicker":     true,
	"Settings":        true,
	"Tabs":            true,
	"Attachments":     true,
	"Footer":          true,
	"MessageSelector": true,
	"DiffDialog":      true,
	"ModelPicker":     true,
	"Select":          true,
	"Plugin":          true,
}

var validActions = map[string]bool{
	"app:interrupt":                  true,
	"app:exit":                       true,
	"app:toggleTodos":                true,
	"app:toggleTranscript":           true,
	"app:toggleBrief":                true,
	"app:toggleTeammatePreview":      true,
	"app:toggleTerminal":             true,
	"app:redraw":                     true,
	"app:globalSearch":               true,
	"app:quickOpen":                  true,
	"history:search":                 true,
	"history:previous":               true,
	"history:next":                   true,
	"chat:cancel":                    true,
	"chat:killAgents":                true,
	"chat:cycleMode":                 true,
	"chat:modelPicker":               true,
	"chat:fastMode":                  true,
	"chat:thinkingToggle":            true,
	"chat:submit":                    true,
	"chat:newline":                   true,
	"chat:undo":                      true,
	"chat:externalEditor":            true,
	"chat:stash":                     true,
	"chat:imagePaste":                true,
	"chat:messageActions":            true,
	"autocomplete:accept":            true,
	"autocomplete:dismiss":           true,
	"autocomplete:previous":          true,
	"autocomplete:next":              true,
	"confirm:yes":                    true,
	"confirm:no":                     true,
	"confirm:previous":               true,
	"confirm:next":                   true,
	"confirm:nextField":              true,
	"confirm:previousField":          true,
	"confirm:cycleMode":              true,
	"confirm:toggle":                 true,
	"confirm:toggleExplanation":      true,
	"tabs:next":                      true,
	"tabs:previous":                  true,
	"transcript:toggleShowAll":       true,
	"transcript:exit":                true,
	"historySearch:next":             true,
	"historySearch:accept":           true,
	"historySearch:cancel":           true,
	"historySearch:execute":          true,
	"task:background":                true,
	"theme:toggleSyntaxHighlighting": true,
	"help:dismiss":                   true,
	"attachments:next":               true,
	"attachments:previous":           true,
	"attachments:remove":             true,
	"attachments:exit":               true,
	"footer:up":                      true,
	"footer:down":                    true,
	"footer:next":                    true,
	"footer:previous":                true,
	"footer:openSelected":            true,
	"footer:clearSelection":          true,
	"footer:close":                   true,
	"messageSelector:up":             true,
	"messageSelector:down":           true,
	"messageSelector:top":            true,
	"messageSelector:bottom":         true,
	"messageSelector:select":         true,
	"diff:dismiss":                   true,
	"diff:previousSource":            true,
	"diff:nextSource":                true,
	"diff:back":                      true,
	"diff:viewDetails":               true,
	"diff:previousFile":              true,
	"diff:nextFile":                  true,
	"modelPicker:decreaseEffort":     true,
	"modelPicker:increaseEffort":     true,
	"select:next":                    true,
	"select:previous":                true,
	"select:accept":                  true,
	"select:cancel":                  true,
	"plugin:toggle":                  true,
	"plugin:install":                 true,
	"permission:toggleDebug":         true,
	"settings:search":                true,
	"settings:retry":                 true,
	"settings:close":                 true,
	"voice:pushToTalk":               true,
}
