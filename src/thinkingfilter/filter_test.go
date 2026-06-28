package thinkingfilter

import "testing"

func TestStripReasoningTagsRemovesReasoningVariants(t *testing.T) {
	input := "" +
		"<think>hidden</think>one\n" +
		"<thinking>hidden</thinking>two\n" +
		"<reasoning>hidden</reasoning>three\n" +
		"<REASONING_SCRATCHPAD>hidden</REASONING_SCRATCHPAD>four\n" +
		"<thought>hidden</thought>five"

	got := StripReasoningTags(input)
	want := "one\ntwo\nthree\nfour\nfive"
	if got != want {
		t.Fatalf("StripReasoningTags() = %q, want %q", got, want)
	}
}

func TestStripReasoningTagsRemovesUnterminatedBlockAtBoundary(t *testing.T) {
	got := StripReasoningTags("Answer first.\n<think>unfinished reasoning")
	want := "Answer first.\n"
	if got != want {
		t.Fatalf("StripReasoningTags() = %q, want %q", got, want)
	}
}

func TestStripReasoningTagsRemovesOrphanClosingTag(t *testing.T) {
	got := StripReasoningTags("leftover</think>Visible answer.")
	want := "leftoverVisible answer."
	if got != want {
		t.Fatalf("StripReasoningTags() = %q, want %q", got, want)
	}
}

func TestStripReasoningTagsPreservesThinkTagMentionedInProse(t *testing.T) {
	input := "Use the <think> tag in docs, and do not hide the rest."
	got := StripReasoningTags(input)
	if got != input {
		t.Fatalf("StripReasoningTags() = %q, want %q", got, input)
	}
}

func TestStripReasoningTagsPreservesInlineThinkPairInProse(t *testing.T) {
	input := "You can document <think>scratch</think> as a literal tag pair."
	got := StripReasoningTags(input)
	if got != input {
		t.Fatalf("StripReasoningTags() = %q, want %q", got, input)
	}
}

func TestStripReasoningTagsFiltersToolCallBlocks(t *testing.T) {
	input := "Before <tool_call>{\"name\":\"read_file\"}</tool_call> middle <function_calls>[]</function_calls>after"
	got := StripReasoningTags(input)
	want := "Before  middle after"
	if got != want {
		t.Fatalf("StripReasoningTags() = %q, want %q", got, want)
	}
}

func TestStripReasoningTagsFiltersGemmaFunctionBlockAtBoundary(t *testing.T) {
	input := "Reading.\n<function name=\"read_file\"><parameter name=\"path\">/tmp/x</parameter></function>\nDone."
	got := StripReasoningTags(input)
	want := "Reading.\n\nDone."
	if got != want {
		t.Fatalf("StripReasoningTags() = %q, want %q", got, want)
	}
}

func TestStripReasoningTagsPreservesInlineFunctionDocsExample(t *testing.T) {
	input := "You can write <function name=\"x\">y</function> inline."
	got := StripReasoningTags(input)
	if got != input {
		t.Fatalf("StripReasoningTags() = %q, want %q", got, input)
	}
}

func TestFilterHandlesSplitOpenAndCloseTags(t *testing.T) {
	var filter Filter
	got := filter.Push("Visible\n<thi")
	got += filter.Push("nk>hidden")
	got += filter.Push("</thi")
	got += filter.Push("nk> answer")
	got += filter.Flush()

	want := "Visible\n answer"
	if got != want {
		t.Fatalf("streamed filter output = %q, want %q", got, want)
	}
}

func TestFilterDropsUnclosedBlockOnFlush(t *testing.T) {
	var filter Filter
	got := filter.Push("Visible\n<thinking>hidden forever")
	got += filter.Flush()

	want := "Visible\n"
	if got != want {
		t.Fatalf("streamed filter output = %q, want %q", got, want)
	}
}

func TestFilterPreservesPlainProseThinkTagAcrossDeltas(t *testing.T) {
	var filter Filter
	got := filter.Push("Use the ")
	got += filter.Push("<thi")
	got += filter.Push("nk>")
	got += filter.Push(" tag in prose.")
	got += filter.Flush()

	want := "Use the <think> tag in prose."
	if got != want {
		t.Fatalf("streamed filter output = %q, want %q", got, want)
	}
}

func TestFilterPreservesInlineThinkPairAcrossDeltas(t *testing.T) {
	var filter Filter
	got := filter.Push("Use <think>scratch")
	got += filter.Push("</think> in docs.")
	got += filter.Flush()

	want := "Use <think>scratch</think> in docs."
	if got != want {
		t.Fatalf("streamed filter output = %q, want %q", got, want)
	}
}

func TestFilterRecognizesWhitespaceOnlyLineBoundary(t *testing.T) {
	var filter Filter
	got := filter.Push("Intro\n  <reason")
	got += filter.Push("ing>hidden</reasoning>Done")
	got += filter.Flush()

	want := "Intro\n  Done"
	if got != want {
		t.Fatalf("streamed filter output = %q, want %q", got, want)
	}
}

func TestFilterRemovesSplitOrphanClosingTag(t *testing.T) {
	var filter Filter
	got := filter.Push("leftover</th")
	got += filter.Push("ink>answer")
	got += filter.Flush()

	want := "leftoveranswer"
	if got != want {
		t.Fatalf("streamed filter output = %q, want %q", got, want)
	}
}

func TestFilterRemovesToolCallBlock(t *testing.T) {
	var filter Filter
	got := filter.Push("Before <tool_")
	got += filter.Push("call>{\"name\":\"x\"}</tool_")
	got += filter.Push("call>after")
	got += filter.Flush()

	want := "Before after"
	if got != want {
		t.Fatalf("streamed filter output = %q, want %q", got, want)
	}
}

func TestFilterFlushesIncompletePlainTag(t *testing.T) {
	var filter Filter
	got := filter.Push("literal <thi")
	got += filter.Flush()

	want := "literal <thi"
	if got != want {
		t.Fatalf("streamed filter output = %q, want %q", got, want)
	}
}
