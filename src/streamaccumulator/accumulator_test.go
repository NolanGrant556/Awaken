package streamaccumulator

import (
	"reflect"
	"testing"
)

func TestAccumulateSingleBlock(t *testing.T) {
	acc := New()

	start := messageStart("start-1", "session-1", "", "msg-1")
	got := acc.Accumulate([]StreamEvent{
		start,
		textDelta("delta-1", "session-1", "", 0, "Hello"),
	})

	want := []StreamEvent{
		start,
		textSnapshot("delta-1", "session-1", "", 0, "Hello"),
	}
	assertEvents(t, got, want)
}

func TestAccumulateAcrossFlushes(t *testing.T) {
	acc := New()

	acc.Accumulate([]StreamEvent{
		messageStart("start-1", "session-1", "", "msg-1"),
		textDelta("delta-1", "session-1", "", 0, "Hel"),
	})

	got := acc.Accumulate([]StreamEvent{
		textDelta("delta-2", "session-1", "", 0, "lo"),
	})

	want := []StreamEvent{
		textSnapshot("delta-2", "session-1", "", 0, "Hello"),
	}
	assertEvents(t, got, want)
}

func TestAccumulateMergesSameFlush(t *testing.T) {
	acc := New()

	start := messageStart("start-1", "session-1", "", "msg-1")
	got := acc.Accumulate([]StreamEvent{
		start,
		textDelta("delta-1", "session-1", "", 0, "Hel"),
		textDelta("delta-2", "session-1", "", 0, "lo"),
	})

	want := []StreamEvent{
		start,
		textSnapshot("delta-1", "session-1", "", 0, "Hello"),
	}
	assertEvents(t, got, want)
}

func TestAccumulateMultipleBlocks(t *testing.T) {
	acc := New()

	start := messageStart("start-1", "session-1", "", "msg-1")
	got := acc.Accumulate([]StreamEvent{
		start,
		textDelta("delta-1", "session-1", "", 0, "A"),
		textDelta("delta-2", "session-1", "", 1, "X"),
		textDelta("delta-3", "session-1", "", 0, "B"),
		textDelta("delta-4", "session-1", "", 1, "Y"),
	})

	want := []StreamEvent{
		start,
		textSnapshot("delta-1", "session-1", "", 0, "AB"),
		textSnapshot("delta-2", "session-1", "", 1, "XY"),
	}
	assertEvents(t, got, want)
}

func TestDeltaWithoutMessageStartPassesThrough(t *testing.T) {
	acc := New()

	raw := textDelta("delta-1", "session-1", "", 0, "fragment")
	got := acc.Accumulate([]StreamEvent{raw})

	assertEvents(t, got, []StreamEvent{raw})
}

func TestMessageStartWithoutIDDoesNotCreateState(t *testing.T) {
	acc := New()

	start := StreamEvent{
		UUID:      "start-1",
		SessionID: "session-1",
		Event: Event{
			Type:    EventMessageStart,
			Message: &Message{},
		},
	}
	raw := textDelta("delta-1", "session-1", "", 0, "fragment")
	got := acc.Accumulate([]StreamEvent{start, raw})

	assertEvents(t, got, []StreamEvent{start, raw})
}

func TestMessageStartReplacesScope(t *testing.T) {
	acc := New()

	acc.Accumulate([]StreamEvent{
		messageStart("start-1", "session-1", "tool-1", "msg-1"),
		textDelta("delta-1", "session-1", "tool-1", 0, "old"),
	})

	start := messageStart("start-2", "session-1", "tool-1", "msg-2")
	got := acc.Accumulate([]StreamEvent{
		start,
		textDelta("delta-2", "session-1", "tool-1", 0, "new"),
	})
	want := []StreamEvent{
		start,
		textSnapshot("delta-2", "session-1", "tool-1", 0, "new"),
	}
	assertEvents(t, got, want)

	got = acc.Accumulate([]StreamEvent{
		textDelta("delta-3", "session-1", "tool-1", 0, "er"),
	})
	want = []StreamEvent{
		textSnapshot("delta-3", "session-1", "tool-1", 0, "newer"),
	}
	assertEvents(t, got, want)
}

func TestClearCompletedRemovesState(t *testing.T) {
	acc := New()

	acc.Accumulate([]StreamEvent{
		messageStart("start-1", "session-1", "tool-1", "msg-1"),
		textDelta("delta-1", "session-1", "tool-1", 0, "Hel"),
	})
	acc.ClearCompleted("session-1", "tool-1", "msg-1")

	raw := textDelta("delta-2", "session-1", "tool-1", 0, "lo")
	got := acc.Accumulate([]StreamEvent{raw})

	assertEvents(t, got, []StreamEvent{raw})
}

func TestNonTextDeltaAndUnknownEventsPassThrough(t *testing.T) {
	acc := New()

	start := messageStart("start-1", "session-1", "", "msg-1")
	nonText := StreamEvent{
		UUID:      "delta-1",
		SessionID: "session-1",
		Event: Event{
			Type:  EventContentBlockDelta,
			Index: 0,
			Delta: &Delta{Type: "input_json_delta", Text: `{"a":`},
		},
	}
	unknown := StreamEvent{
		UUID:      "event-1",
		SessionID: "session-1",
		Event:     Event{Type: "content_block_stop"},
	}

	got := acc.Accumulate([]StreamEvent{
		start,
		nonText,
		unknown,
		textDelta("delta-2", "session-1", "", 0, "text"),
	})

	want := []StreamEvent{
		start,
		nonText,
		unknown,
		textSnapshot("delta-2", "session-1", "", 0, "text"),
	}
	assertEvents(t, got, want)
}

func assertEvents(t *testing.T, got, want []StreamEvent) {
	t.Helper()

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("events mismatch\n got: %#v\nwant: %#v", got, want)
	}
}

func messageStart(uuid, sessionID, parentToolUseID, messageID string) StreamEvent {
	return StreamEvent{
		UUID:            uuid,
		SessionID:       sessionID,
		ParentToolUseID: parentToolUseID,
		Event: Event{
			Type:    EventMessageStart,
			Message: &Message{ID: messageID},
		},
	}
}

func textDelta(uuid, sessionID, parentToolUseID string, index int, text string) StreamEvent {
	return StreamEvent{
		UUID:            uuid,
		SessionID:       sessionID,
		ParentToolUseID: parentToolUseID,
		Event: Event{
			Type:  EventContentBlockDelta,
			Index: index,
			Delta: &Delta{Type: DeltaText, Text: text},
		},
	}
}

func textSnapshot(uuid, sessionID, parentToolUseID string, index int, text string) StreamEvent {
	return textDelta(uuid, sessionID, parentToolUseID, index, text)
}
