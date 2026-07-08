package askuser

import (
	"encoding/json"
	"testing"
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

func TestToolDefinition(t *testing.T) {
	tool := &AskUserTool{}
	if tool.Name() != ToolName {
		t.Fatalf("unexpected tool name: %q", tool.Name())
	}
	def := tool.Definition()
	if def.Source != "builtin" {
		t.Fatalf("expected builtin source, got %q", def.Source)
	}
	if !def.Annotations.ReadOnly {
		t.Fatalf("expected read-only tool")
	}
}
