package plugincommands

import (
	"errors"
	"testing"

	"awaken/src/plugins/ir"
)

func TestExpandSubstitutesJoinedAndPositionalArguments(t *testing.T) {
	component := ir.Component{
		Kind: ir.KindCommand, Mapping: ir.MappingAdapted, Validation: "valid", LogicalID: "command:review",
		Command: &ir.CommandSpec{InvocationName: "review", ArgumentHint: "<file> [focus]", PromptTemplate: "Review $1 with $ARGUMENTS; focus=$2"},
	}
	expansion, err := Expand(component, []string{"app.ts", "security"})
	if err != nil {
		t.Fatal(err)
	}
	if expansion.Prompt != "Review app.ts with app.ts security; focus=security" {
		t.Fatalf("prompt=%q", expansion.Prompt)
	}
}

func TestExpandRejectsMissingArguments(t *testing.T) {
	component := ir.Component{
		Kind: ir.KindCommand, Mapping: ir.MappingNative, Validation: "valid", LogicalID: "command:hello",
		Command: &ir.CommandSpec{InvocationName: "hello", ArgumentHint: "<name>", PromptTemplate: "Hello $1"},
	}
	if _, err := Expand(component, nil); !errors.Is(err, ErrArgumentsRequired) {
		t.Fatalf("err=%v", err)
	}
}

func TestExpandAppendsArgumentsWhenTemplateHasNoPlaceholder(t *testing.T) {
	component := ir.Component{
		Kind: ir.KindCommand, Mapping: ir.MappingAdapted, Validation: "valid", LogicalID: "command:review",
		Command: &ir.CommandSpec{InvocationName: "review", ArgumentHint: "[optional focus]", PromptTemplate: "Review the current changes."},
	}
	expansion, err := Expand(component, []string{"focus on auth"})
	if err != nil {
		t.Fatal(err)
	}
	if expansion.Prompt != "Review the current changes.\n\nARGUMENTS: focus on auth" {
		t.Fatalf("prompt=%q", expansion.Prompt)
	}
	withoutArguments, err := Expand(component, nil)
	if err != nil || withoutArguments.Prompt != "Review the current changes." {
		t.Fatalf("without arguments=%#v err=%v", withoutArguments, err)
	}
}

func TestExpandSupportsClaudeIndexedArgumentsAndZeroShorthand(t *testing.T) {
	component := ir.Component{
		Kind: ir.KindCommand, Mapping: ir.MappingAdapted, Validation: "valid", LogicalID: "command:indexed",
		Command: &ir.CommandSpec{InvocationName: "indexed", PromptTemplate: "first=$ARGUMENTS[0] second=$ARGUMENTS[1] firstAgain=$0"},
	}
	expansion, err := Expand(component, []string{"one", "two"})
	if err != nil {
		t.Fatal(err)
	}
	if expansion.Prompt != "first=one second=two firstAgain=one" {
		t.Fatalf("prompt=%q", expansion.Prompt)
	}
}

func TestArgumentPolicyDistinguishesOptionalAndRequiredHints(t *testing.T) {
	accepts, required := ArgumentPolicy(&ir.CommandSpec{ArgumentHint: "[optional focus]", PromptTemplate: "Review"})
	if !accepts || required {
		t.Fatalf("optional policy accepts=%v required=%v", accepts, required)
	}
	accepts, required = ArgumentPolicy(&ir.CommandSpec{ArgumentHint: "<file>", PromptTemplate: "Review"})
	if !accepts || !required {
		t.Fatalf("required policy accepts=%v required=%v", accepts, required)
	}
}
