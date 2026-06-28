package slashcommands

import (
	"errors"
	"strings"
	"testing"
)

func TestParseKnownCommandWithArgumentsPreservesOriginal(t *testing.T) {
	input := "  /plan add focused runtime tests  "

	inv := Parse(input)

	if inv.Kind != ParseKnown {
		t.Fatalf("expected known command, got %s", inv.Kind)
	}
	if inv.CommandName != "plan" {
		t.Fatalf("expected command name plan, got %q", inv.CommandName)
	}
	if inv.Arguments != "add focused runtime tests" {
		t.Fatalf("expected arguments to be trimmed, got %q", inv.Arguments)
	}
	if inv.OriginalInput != input {
		t.Fatalf("expected original input to be preserved, got %q", inv.OriginalInput)
	}
	if inv.Command == nil || inv.Command.Name != "plan" {
		t.Fatalf("expected command definition for plan, got %+v", inv.Command)
	}
}

func TestParseUnknownCommand(t *testing.T) {
	inv := Parse("/unknown please do something")

	if inv.Kind != ParseUnknown {
		t.Fatalf("expected unknown command, got %s", inv.Kind)
	}
	if inv.CommandName != "unknown" {
		t.Fatalf("expected command name unknown, got %q", inv.CommandName)
	}
	if inv.Arguments != "please do something" {
		t.Fatalf("expected arguments to be preserved, got %q", inv.Arguments)
	}
}

func TestParseSlashOnly(t *testing.T) {
	for _, input := range []string{"/", "/   "} {
		inv := Parse(input)
		if inv.Kind != ParseEmpty {
			t.Fatalf("input %q: expected empty slash command, got %s", input, inv.Kind)
		}
	}
}

func TestParsePlainInputAndAbsolutePath(t *testing.T) {
	for _, input := range []string{
		"please make a plan",
		"/tmp/project README",
		"/tmp foo",
		"/var log",
		"/Users/nolangrant/Awaken",
	} {
		inv := Parse(input)
		if inv.Kind != ParsePlain {
			t.Fatalf("input %q: expected plain input, got %s", input, inv.Kind)
		}
	}
}

func TestExpandKnownCommandBuildsAgentInstruction(t *testing.T) {
	expanded, err := ExpandInput("/review src/runtime")
	if err != nil {
		t.Fatalf("ExpandInput returned error: %v", err)
	}

	if !expanded.Applied {
		t.Fatal("expected slash command expansion to be applied")
	}
	for _, want := range []string{
		"Awaken slash command instruction",
		"Slash command:\n/review",
		"Original user input:\n/review src/runtime",
		"Command arguments:\nsrc/runtime",
		"Review the target described by the user.",
	} {
		if !strings.Contains(expanded.AgentInput, want) {
			t.Fatalf("expected expanded instruction to contain %q, got:\n%s", want, expanded.AgentInput)
		}
	}
}

func TestExpandPlainInputLeavesAgentInputUnchanged(t *testing.T) {
	expanded, err := ExpandInput("hello")
	if err != nil {
		t.Fatalf("ExpandInput returned error: %v", err)
	}

	if expanded.Applied {
		t.Fatal("expected plain input not to apply slash expansion")
	}
	if expanded.AgentInput != "hello" {
		t.Fatalf("expected plain input to pass through, got %q", expanded.AgentInput)
	}
}

func TestExpandInvalidSlashCommandsReturnTypedErrors(t *testing.T) {
	if _, err := ExpandInput("/"); !errors.Is(err, ErrEmptyCommand) {
		t.Fatalf("expected ErrEmptyCommand, got %v", err)
	}
	if _, err := ExpandInput("/unknown args"); !errors.Is(err, ErrUnknownCommand) {
		t.Fatalf("expected ErrUnknownCommand, got %v", err)
	}
}

func TestListCommandsExposesBuiltinsInStableOrder(t *testing.T) {
	commands := ListCommands()
	got := make([]string, 0, len(commands))
	for _, command := range commands {
		got = append(got, command.Name)
		if command.Description == "" {
			t.Fatalf("expected description for command %q", command.Name)
		}
	}

	want := []string{"explain", "fix", "plan", "review"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("expected command list %v, got %v", want, got)
	}
}
