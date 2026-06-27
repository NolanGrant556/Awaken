package slashcommands

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

type ParseKind string

const (
	ParsePlain   ParseKind = "plain"
	ParseEmpty   ParseKind = "empty"
	ParseKnown   ParseKind = "known"
	ParseUnknown ParseKind = "unknown"
)

var (
	ErrEmptyCommand   = errors.New("slash command name is required")
	ErrUnknownCommand = errors.New("unknown slash command")
)

type CommandInstruction struct {
	Goal         string
	Requirements []string
	Output       string
}

type CommandDefinition struct {
	Name         string
	Description  string
	ArgumentHint string
	Instruction  CommandInstruction
}

type CommandInfo struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	ArgumentHint string `json:"argument_hint,omitempty"`
}

type Invocation struct {
	Kind          ParseKind
	OriginalInput string
	CommandName   string
	Arguments     string
	Command       *CommandDefinition
}

type Expansion struct {
	Invocation Invocation
	AgentInput string
	Applied    bool
}

var registry = map[string]CommandDefinition{
	"plan": {
		Name:         "plan",
		Description:  "Turn the request into a concise, verifiable implementation plan.",
		ArgumentHint: "[task]",
		Instruction: CommandInstruction{
			Goal: "Turn the user's request into a concrete plan before implementation.",
			Requirements: []string{
				"State the assumptions and any blockers that matter.",
				"Break the work into short, verifiable steps.",
				"Include the checks that would prove the plan is complete.",
				"Do not edit files unless the user explicitly asks for implementation.",
			},
			Output: "Reply with a concise plan and any necessary clarification questions.",
		},
	},
	"review": {
		Name:         "review",
		Description:  "Review the target for bugs, regressions, and missing tests.",
		ArgumentHint: "[target]",
		Instruction: CommandInstruction{
			Goal: "Review the target described by the user.",
			Requirements: []string{
				"Prioritize concrete bugs, behavioral regressions, security risks, data loss risks, and missing tests.",
				"Lead with findings ordered by severity and grounded in file or behavior evidence when available.",
				"Keep summaries brief and secondary to actionable findings.",
				"Do not modify files unless the user explicitly asks for fixes.",
			},
			Output: "Return findings first, then open questions or residual risk.",
		},
	},
	"fix": {
		Name:         "fix",
		Description:  "Implement the narrowest safe fix and verify it.",
		ArgumentHint: "[bug or task]",
		Instruction: CommandInstruction{
			Goal: "Diagnose and implement the narrowest safe fix for the user's request.",
			Requirements: []string{
				"Inspect the relevant code before editing.",
				"Change only the files needed for the fix.",
				"Preserve unrelated local work.",
				"Run focused verification that proves the fix works.",
			},
			Output: "Report changed files, verification, and remaining risk.",
		},
	},
	"explain": {
		Name:         "explain",
		Description:  "Explain the requested code, behavior, or concept without changing files.",
		ArgumentHint: "[topic]",
		Instruction: CommandInstruction{
			Goal: "Explain the topic or target described by the user.",
			Requirements: []string{
				"Ground the explanation in concrete files, APIs, or runtime behavior when available.",
				"Separate confirmed facts from inference.",
				"Keep the answer clear and proportional to the user's question.",
				"Do not modify files.",
			},
			Output: "Return a direct explanation with references when useful.",
		},
	},
}

func ListCommands() []CommandInfo {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)

	commands := make([]CommandInfo, 0, len(names))
	for _, name := range names {
		def := registry[name]
		commands = append(commands, CommandInfo{
			Name:         def.Name,
			Description:  def.Description,
			ArgumentHint: def.ArgumentHint,
		})
	}
	return commands
}

func Parse(input string) Invocation {
	inv := Invocation{
		Kind:          ParsePlain,
		OriginalInput: input,
	}

	trimmedLeft := strings.TrimLeftFunc(input, unicode.IsSpace)
	if !strings.HasPrefix(trimmedLeft, "/") {
		return inv
	}

	afterSlash := trimmedLeft[1:]
	if strings.TrimSpace(afterSlash) == "" {
		inv.Kind = ParseEmpty
		return inv
	}

	name, rest, ok := splitCommandToken(afterSlash)
	if !ok {
		return inv
	}

	inv.CommandName = name
	inv.Arguments = strings.TrimSpace(rest)

	def, found := registry[name]
	if !found {
		inv.Kind = ParseUnknown
		return inv
	}

	inv.Kind = ParseKnown
	inv.Command = &def
	return inv
}

func ExpandInput(input string) (Expansion, error) {
	inv := Parse(input)
	switch inv.Kind {
	case ParsePlain:
		return Expansion{
			Invocation: inv,
			AgentInput: strings.TrimSpace(input),
			Applied:    false,
		}, nil
	case ParseKnown:
		return Expansion{
			Invocation: inv,
			AgentInput: BuildAgentInstruction(inv),
			Applied:    true,
		}, nil
	case ParseEmpty:
		return Expansion{Invocation: inv}, ErrEmptyCommand
	case ParseUnknown:
		return Expansion{Invocation: inv}, fmt.Errorf("%w: /%s", ErrUnknownCommand, inv.CommandName)
	default:
		return Expansion{Invocation: inv}, nil
	}
}

func BuildAgentInstruction(inv Invocation) string {
	if inv.Kind != ParseKnown || inv.Command == nil {
		return strings.TrimSpace(inv.OriginalInput)
	}

	args := strings.TrimSpace(inv.Arguments)
	if args == "" {
		args = "(none)"
	}

	sections := []instructionSection{
		{Title: "Slash command", Body: "/" + inv.Command.Name},
		{Title: "Original user input", Body: inv.OriginalInput},
		{Title: "Command arguments", Body: args},
		{Title: "Goal", Body: inv.Command.Instruction.Goal},
		{Title: "Requirements", Items: inv.Command.Instruction.Requirements},
		{Title: "Expected output", Body: inv.Command.Instruction.Output},
	}
	return renderInstruction(sections)
}

type instructionSection struct {
	Title string
	Body  string
	Items []string
}

func renderInstruction(sections []instructionSection) string {
	var b strings.Builder
	b.WriteString("Awaken slash command instruction\n")
	for _, section := range sections {
		b.WriteString("\n")
		b.WriteString(section.Title)
		b.WriteString(":\n")
		if len(section.Items) > 0 {
			for _, item := range section.Items {
				b.WriteString("- ")
				b.WriteString(item)
				b.WriteString("\n")
			}
			continue
		}
		b.WriteString(section.Body)
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

func splitCommandToken(input string) (name, rest string, ok bool) {
	offset := 0
	for offset < len(input) {
		r, size := utf8.DecodeRuneInString(input[offset:])
		if !isCommandRune(r, offset == 0) {
			break
		}
		offset += size
	}

	if offset == 0 {
		return "", "", false
	}
	if offset < len(input) {
		r, _ := utf8.DecodeRuneInString(input[offset:])
		if !unicode.IsSpace(r) {
			return "", "", false
		}
	}
	return input[:offset], input[offset:], true
}

func isCommandRune(r rune, first bool) bool {
	if first {
		return r >= 'a' && r <= 'z'
	}
	return (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_'
}
