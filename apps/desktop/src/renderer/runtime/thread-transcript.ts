import type { RuntimeThreadTranscript } from "../../shared/runtime";
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
    const prompt = entry.run.input?.trim() || (index === 0 ? transcript.thread.label : "");
    if (prompt) {
      actions.push({
        type: "run/requested",
        prompt,
        startedAtMs: timestampMs(entry.run.createdAt),
      });
    }
    actions.push({ type: "run/created", run: entry.run });
    entry.events.forEach((event) => {
      actions.push({
        type: "run/event",
        event,
        receivedAtMs: receivedAtMsForReplayEvent(entry.run, event),
      });
    });
  });

  return actions;
}

export function afterSeqForTranscriptRun(
  transcript: RuntimeThreadTranscript,
  runId: string,
): number {
  const run = transcript.runs.find((entry) => entry.run.runId === runId);
  if (!run) {
    return 0;
  }

  return run.events.reduce((maxSeq, event) => {
    if (typeof event.seq !== "number") {
      return maxSeq;
    }
    return Math.max(maxSeq, event.seq);
  }, 0);
}
