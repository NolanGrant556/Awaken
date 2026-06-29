import type { AguiEvent } from "../../shared/agui";
import type { RuntimeThreadTranscript } from "../../shared/runtime";
import type { RuntimeAction } from "./reducer";

function isTerminalEvent(event: AguiEvent): boolean {
  if (event.type === "RUN_FINISHED" || event.type === "RUN_ERROR") {
    return true;
  }
  if (event.type === "RUN_STATUS_CHANGED") {
    return (
      event.status === "cancelled" ||
      event.status === "finished" ||
      event.status === "error"
    );
  }
  return false;
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
    // Backfill startedAtMs from the persisted run's createdAt so the answer
    // footer can render "Agent ran ..." elapsed time during transcript replay.
    const startedAtMs = Date.parse(entry.run.createdAt);
    const validStartedAtMs = Number.isFinite(startedAtMs) ? startedAtMs : undefined;
    if (prompt) {
      actions.push({ type: "run/requested", prompt, startedAtMs: validStartedAtMs });
    }
    actions.push({ type: "run/created", run: entry.run, startedAtMs: validStartedAtMs });

    // Find the terminal event and compute receivedAtMs from its timestamp so
    // runCompletedAtMs is populated and elapsedMs can be attached to the
    // replayed answer. Fall back to the run's finishedAt / updatedAt.
    let terminalReceivedAtMs: number | undefined;
    entry.events.forEach((event) => {
      if (isTerminalEvent(event) && typeof event.timestamp === "number") {
        terminalReceivedAtMs = event.timestamp;
      }
    });
    if (terminalReceivedAtMs === undefined) {
      const fallback = Date.parse(entry.run.finishedAt ?? entry.run.updatedAt);
      terminalReceivedAtMs = Number.isFinite(fallback) ? fallback : undefined;
    }

    entry.events.forEach((event) => {
      const isTerminal = isTerminalEvent(event);
      actions.push({
        type: "run/event",
        event,
        receivedAtMs: isTerminal ? terminalReceivedAtMs : undefined,
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
