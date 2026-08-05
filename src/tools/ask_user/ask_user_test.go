package askuser

import (
	"encoding/json"
	"strings"
	"testing"

	"awaken/src/tool"
)

func TestParseQuestions_ValidSingleQuestion(t *testing.T) {
	input := `[{"question": "Choose a theme", "options": [{"label": "Dark"}, {"label": "Light"}]}]`
	questions, err := ParseQuestions(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(questions) != 1 {
		t.Fatalf("expected 1 question, got %d", len(questions))
	}
	if questions[0].Question != "Choose a theme" {
		t.Fatalf("unexpected question: %q", questions[0].Question)
	}
	if len(questions[0].Options) != 2 {
		t.Fatalf("expected 2 options, got %d", len(questions[0].Options))
	}
	if questions[0].Options[0].Label != "Dark" {
		t.Fatalf("unexpected first option: %q", questions[0].Options[0].Label)
	}
}

func TestParseQuestions_MultiSelect(t *testing.T) {
	input := `[{"question": "Pick features", "options": [{"label": "A"}, {"label": "B"}, {"label": "C"}], "multi_select": true}]`
	questions, err := ParseQuestions(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !questions[0].MultiSelect {
		t.Fatalf("expected multi_select=true")
	}
}

func TestParseQuestions_MissingOptions(t *testing.T) {
	input := `[{"question": "Choose a theme", "options": [{"label": "Dark"}]}]`
	_, err := ParseQuestions(input)
	if err == nil {
		t.Fatalf("expected error for too few options")
	}
}

func TestParseQuestions_TooManyOptions(t *testing.T) {
	options := []QuestionOption{
		{Label: "A"}, {Label: "B"}, {Label: "C"}, {Label: "D"}, {Label: "E"},
	}
	data, _ := json.Marshal(map[string]any{
		"questions": []Question{{Question: "Q", Options: options}},
	})
	_, err := ParseQuestions(string(data))
	if err == nil {
		t.Fatalf("expected error for too many options")
	}
}

func TestFormatAnswerResult(t *testing.T) {
	questions := []Question{{Question: "Choose a theme"}}
	answers := map[string]string{"Choose a theme": "Dark"}
	result := FormatAnswerResult(questions, answers)
	expected := `User has answered your questions: "Choose a theme"="Dark"`
	if result != expected {
		t.Fatalf("expected %q, got %q", expected, result)
	}
}

func TestFormatAnswerTextResultStripsSingleQuestionPrefix(t *testing.T) {
	questions := []Question{{Question: "Choose a theme"}}
	result := FormatAnswerTextResult(questions, "Choose a theme: Dark")
	expected := `User has answered your questions: "Choose a theme"="Dark"`
	if result != expected {
		t.Fatalf("expected %q, got %q", expected, result)
	}
}

func TestFormatAnswerTextResultKeepsMultiQuestionLines(t *testing.T) {
	questions := []Question{
		{Question: "Choose a theme"},
		{Question: "Pick output"},
	}
	result := FormatAnswerTextResult(questions, "Choose a theme: Dark\nPick output: PDF")
	expected := "User has answered your questions:\nChoose a theme: Dark\nPick output: PDF"
	if result != expected {
		t.Fatalf("expected %q, got %q", expected, result)
	}
}

func TestToolDefinition(t *testing.T) {
	askTool := &AskUserTool{}
	if askTool.Name() != ToolName {
		t.Fatalf("unexpected tool name: %q", askTool.Name())
	}
	def := askTool.Definition()
	if def.Source != "builtin" {
		t.Fatalf("expected builtin source, got %q", def.Source)
	}
	if !def.Annotations.ReadOnly {
		t.Fatalf("expected read-only tool")
	}
	if len(def.InputSchema.OneOf) != 0 {
		t.Fatalf("model schema must not expose ambiguous oneOf branches: %+v", def.InputSchema.OneOf)
	}
	if len(def.InputSchema.Required) != 1 || def.InputSchema.Required[0] != "questions" {
		t.Fatalf("model schema must require only questions: %+v", def.InputSchema.Required)
	}
	if _, ok := def.InputSchema.Properties["questions_json"]; ok {
		t.Fatalf("legacy questions_json leaked into model schema: %+v", def.InputSchema.Properties)
	}
	if def.InputSchema.AdditionalProperties == nil || *def.InputSchema.AdditionalProperties {
		t.Fatalf("ask_user schema must reject unknown top-level properties")
	}
	if params := askTool.Parameters(); len(params) != 1 || params[0].Name != "questions" || !params[0].Required {
		t.Fatalf("legacy parameter leaked into runtime parameter contract: %+v", params)
	}
}

func TestCanonicalSchemaRejectsEmptyAndLegacyModelCalls(t *testing.T) {
	schema := (&AskUserTool{}).InputSchema()
	for name, input := range map[string]string{
		"empty object": `{}`,
		"legacy field": `{"questions_json":"[]"}`,
		"bare array":   `[]`,
	} {
		t.Run(name, func(t *testing.T) {
			if err := tool.ValidateInputAgainstSchema(input, schema); err == nil {
				t.Fatalf("canonical schema accepted %s", input)
			}
		})
	}
}

func TestCanonicalizeLegacyArguments(t *testing.T) {
	canonical := `{"questions":[{"question":"Theme?","options":[{"label":"Dark"},{"label":"Light"}]}]}`
	if got, err := CanonicalizeLegacyArguments(canonical); err != nil || got != canonical {
		t.Fatalf("canonical input changed: got=%q err=%v", got, err)
	}

	for name, legacy := range map[string]string{
		"encoded field": `{"questions_json":"[{\"question\":\"Theme?\",\"options\":[{\"label\":\"Dark\"},{\"label\":\"Light\"}]}]"}`,
		"bare array":    `[{"question":"Theme?","options":[{"label":"Dark"},{"label":"Light"}]}]`,
	} {
		t.Run(name, func(t *testing.T) {
			got, err := CanonicalizeLegacyArguments(legacy)
			if err != nil {
				t.Fatalf("legacy normalization failed: %v", err)
			}
			if !strings.Contains(got, `"questions"`) || strings.Contains(got, "questions_json") {
				t.Fatalf("legacy input did not become canonical: %s", got)
			}
			if err := tool.ValidateInputAgainstSchema(got, (&AskUserTool{}).InputSchema()); err != nil {
				t.Fatalf("normalized input failed canonical schema: %v", err)
			}
		})
	}
}
