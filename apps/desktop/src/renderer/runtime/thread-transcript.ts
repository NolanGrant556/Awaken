import type { RuntimeThreadTranscript } from "../../shared/runtime";
import type { AguiEvent } from "../../shared/agui";
import type { RuntimeAction } from "./reducer";

function timestampMs(value?: string): number | undefined {
  if (!value) {
    return undefined;
  }
  const timestamp = Date.parse(value);
  return Number.isFinite(timestamp) ? timestamp : undefined;
}

function receivedAtMsForReplayEvent(
  run: RuntimeThreadTranscript["runs"][number]["run"],
  event: RuntimeThreadTranscript["runs"][number]["events"][number],
): number | undefined {
  if (
    event.type === "RUN_FINISHED" ||
    (event.type === "RUN_STATUS_CHANGED" && event.status === "finished")
  ) {
    return timestampMs(run.finishedAt);
  }
  return undefined;
}

function hasTextMessageEvents(events: AguiEvent[]): boolean {
  return events.some(
    (event) =>
      event.type === "TEXT_MESSAGE_START" ||
      event.type === "TEXT_MESSAGE_CONTENT" ||
      event.type === "TEXT_MESSAGE_END",
  );
}

// Compatibility fallback for v1 transcripts that persisted run.result but
// omitted final TEXT_MESSAGE_* events. Do not synthesize ToolCall/ToolResult
// or Artifact facts here — those must come from the server SemanticProjector.
function syntheticResultEvents(
  run: RuntimeThreadTranscript["runs"][number]["run"],
): AguiEvent[] {
  const result = run.result?.trim();
  if (!result || (run.status !== "finished" && run.status !== "cancelled")) {
    return [];
  }

  const timestamp = timestampMs(run.finishedAt) ?? timestampMs(run.updatedAt);
  const messageId = `transcript:${run.runId}:result`;
  return [
    {
      type: "TEXT_MESSAGE_START",
      threadId: run.threadId,
      runId: run.runId,
      messageId,
      role: "assistant",
      timestamp,
    },
    {
      type: "TEXT_MESSAGE_CONTENT",
      threadId: run.threadId,
      runId: run.runId,
      messageId,
      role: "assistant",
      delta: result,
      timestamp,
    },
    {
      type: "TEXT_MESSAGE_END",
      threadId: run.threadId,
      runId: run.runId,
      messageId,
      role: "assistant",
      timestamp,
    },
  ];
}

function replayEventsForTranscriptRun(
  entry: RuntimeThreadTranscript["runs"][number],
): AguiEvent[] {
  if (hasTextMessageEvents(entry.events)) {
    return entry.events;
  }

  const syntheticEvents = syntheticResultEvents(entry.run);
  if (syntheticEvents.length === 0) {
    return entry.events;
  }

  const terminalIndex = entry.events.findIndex(
    (event) =>
      event.type === "RUN_FINISHED" ||
      (event.type === "RUN_STATUS_CHANGED" && event.status === "finished"),
  );
  if (terminalIndex < 0) {
    return [...entry.events, ...syntheticEvents];
  }

  return [
    ...entry.events.slice(0, terminalIndex),
    ...syntheticEvents,
    ...entry.events.slice(terminalIndex),
  ];
}

export function actionsForThreadTranscript(
  transcript: RuntimeThreadTranscript,
): RuntimeAction[] {
  const actions: RuntimeAction[] = [
    {
      type: "run/threadReplayStarted",
      threadId: transcript.thread.threadId,
      runId: transcript.thread.runId,
    },
  ];

  transcript.runs.forEach((entry, index) => {
    const hasPromptAttachments = (entry.run.inputAttachments?.length ?? 0) > 0;
    const hasPromptTokens = (entry.run.composerTokens?.length ?? 0) > 0;
    const hasPromptFileComments = (entry.run.fileComments?.length ?? 0) > 0;
    const prompt =
      entry.run.input?.trim() ||
      (!hasPromptAttachments && !hasPromptTokens && !hasPromptFileComments && index === 0
        ? transcript.thread.label
        : "");
    if (prompt || hasPromptAttachments || hasPromptTokens || hasPromptFileComments) {
      actions.push({
        type: "run/requested",
        runId: entry.run.runId,
        prompt,
        promptAttachments: entry.run.inputAttachments,
        promptTokens: entry.run.composerTokens,
        promptFileComments: entry.run.fileComments,
        startedAtMs: timestampMs(entry.run.createdAt),
      });
    }
    actions.push({ type: "run/created", run: entry.run });
    replayEventsForTranscriptRun(entry).forEach((event) => {
      actions.push({
        type: "run/event",
        event,
        receivedAtMs: receivedAtMsForReplayEvent(entry.run, event),
      });
    });
  });

  return actions;
}

export function streamCursorForTranscriptRun(
  transcript: RuntimeThreadTranscript,
  runId: string,
): number {
  const run = transcript.runs.find((entry) => entry.run.runId === runId);
  if (!run) {
    return 0;
  }

  const transcriptCursor = run.streamCursor ?? 0;
  if (transcriptCursor > 0) {
    return transcriptCursor;
  }

  // Compatibility with transcripts produced before stream_cursor existed.
  return run.events.reduce((maxCursor, event) => {
    const cursor = event.streamCursor ?? event.seq;
    if (typeof cursor !== "number") {
      return maxCursor;
    }
    return Math.max(maxCursor, cursor);
  }, 0);
}

/** @deprecated Use streamCursorForTranscriptRun. */
export const afterSeqForTranscriptRun = streamCursorForTranscriptRun;
