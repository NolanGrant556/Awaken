package terminaltext

import "testing"

func TestIsTerminalOutputRecognizesTerminalTags(t *testing.T) {
	cases := []string{
		"<bash-input>ls</bash-input>",
		"prefix <bash-stdout>ok</bash-stdout>",
		"<bash-stderr>error</bash-stderr>",
		"<local-command-stdout>done</local-command-stdout>",
		"<local-command-stderr>failed</local-command-stderr>",
		"<local-command-caveat>local command output</local-command-caveat>",
		"<bash-stdout data-kind=\"terminal\">ok</bash-stdout>",
	}

	for _, text := range cases {
		if !IsTerminalOutput(text) {
			t.Fatalf("expected %q to be terminal output", text)
		}
	}
}

func TestIsTerminalOutputRejectsNonTerminalText(t *testing.T) {
	cases := []string{
		"",
		"please run ls",
		"bash-stdout without a tag",
		"<bash-stdouts>not exact</bash-stdouts>",
		"<command-output>not a terminal output tag</command-output>",
	}

	for _, text := range cases {
		if IsTerminalOutput(text) {
			t.Fatalf("expected %q to be non-terminal text", text)
		}
	}
}

func TestCountUserPromptsWithPlainContent(t *testing.T) {
	messages := []Message{
		{Type: MessageTypeAssistant, Content: "assistant text"},
		{Type: MessageTypeTool, Content: "tool output"},
		{Type: MessageTypeUser, Content: ""},
		{Type: MessageTypeUser, Content: "  \n\t  "},
		{Type: MessageTypeUser, Content: "<bash-input>go test ./...</bash-input>"},
		{Type: MessageTypeUser, Content: "<local-command-stderr>permission denied</local-command-stderr>"},
		{Type: MessageTypeUser, Content: "please inspect this"},
		{Type: MessageTypeUser, Content: "and summarize it"},
	}

	if got := CountUserPrompts(messages); got != 2 {
		t.Fatalf("CountUserPrompts() = %d, want 2", got)
	}
}

func TestCountUserPromptsWithContentBlocks(t *testing.T) {
	messages := []Message{
		{
			Type: MessageTypeUser,
			ContentBlocks: []ContentBlock{
				{Type: ContentBlockTypeToolResult, Text: "tool output"},
			},
		},
		{
			Type: MessageTypeUser,
			ContentBlocks: []ContentBlock{
				{Type: ContentBlockTypeText, Text: "   "},
			},
		},
		{
			Type: MessageTypeUser,
			ContentBlocks: []ContentBlock{
				{Type: ContentBlockTypeText, Text: "<bash-stdout>ok</bash-stdout>"},
			},
		},
		{
			Type: MessageTypeUser,
			ContentBlocks: []ContentBlock{
				{Type: ContentBlockTypeText, Text: "review this change"},
			},
		},
		{
			Type: MessageTypeUser,
			ContentBlocks: []ContentBlock{
				{Type: ContentBlockTypeImage},
			},
		},
		{
			Type: MessageTypeUser,
			ContentBlocks: []ContentBlock{
				{Type: ContentBlockTypeDocument},
			},
		},
		{
			Type: MessageTypeUser,
			ContentBlocks: []ContentBlock{
				{Type: ContentBlockTypeToolResult, Text: "tool output"},
				{Type: ContentBlockTypeText, Text: "follow up"},
			},
		},
		{
			Type: MessageTypeAssistant,
			ContentBlocks: []ContentBlock{
				{Type: ContentBlockTypeImage},
			},
		},
	}

	if got := CountUserPrompts(messages); got != 4 {
		t.Fatalf("CountUserPrompts() = %d, want 4", got)
	}
}
