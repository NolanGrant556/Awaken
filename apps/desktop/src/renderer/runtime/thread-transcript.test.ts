import { describe, expect, it } from "vitest";
import type { RuntimeThreadTranscript } from "../../shared/runtime";
import { actionsForThreadTranscript, afterSeqForTranscriptRun } from "./thread-transcript";

function makeTranscript(): RuntimeThreadTranscript {
  return {
    thread: {
      threadId: "thread-1",
      label: "你好",
      runId: "run-2",
      workspace: {
        workspaceRoot: "/tmp/awaken",
        writableRoots: ["/tmp/awaken"],
      },
      createdAt: "2026-06-24T00:00:00Z",
      updatedAt: "2026-06-24T00:00:00Z",
    },
    runs: [
      {
        run: {
          runId: "run-1",
          threadId: "thread-1",
          input: "你好",
          status: "finished",
          cancelRequested: false,
          createdAt: "2026-06-24T00:00:00Z",
          updatedAt: "2026-06-24T00:00:10Z",
        },
        events: [
          { type: "RUN_STARTED", seq: 16, runId: "run-1", threadId: "thread-1" },
          { type: "TEXT_MESSAGE_CONTENT", seq: 32, runId: "run-1", threadId: "thread-1", messageId: "msg-1", delta: "first" },
          { type: "RUN_FINISHED", seq: 48, runId: "run-1", threadId: "thread-1", result: "first" },
        ],
      },
      {
        run: {
          runId: "run-2",
          threadId: "thread-1",
          input: "再次告诉我",
          status: "running",
          cancelRequested: false,
          createdAt: "2026-06-24T00:01:00Z",
          updatedAt: "2026-06-24T00:01:10Z",
        },
        events: [
          { type: "RUN_STARTED", seq: 64, runId: "run-2", threadId: "thread-1" },
          { type: "REASONING_MESSAGE_CONTENT", seq: 80, runId: "run-2", threadId: "thread-1", messageId: "reason-2", delta: "second thought" },
        ],
      },
    ],
  };
}

describe("thread transcript replay helpers", () => {
  it("builds ordered reducer actions for a multi-turn transcript", () => {
    const actions = actionsForThreadTranscript(makeTranscript());

    expect(actions[0]).toEqual({
      type: "run/threadReplayStarted",
      threadId: "thread-1",
      runId: "run-2",
    });
    expect(actions).toContainEqual(
      expect.objectContaining({ type: "run/requested", prompt: "你好" }),
    );
    expect(actions).toContainEqual(
      expect.objectContaining({ type: "run/requested", prompt: "再次告诉我" }),
    );
    expect(actions).toContainEqual(
      expect.objectContaining({
        type: "run/created",
        run: expect.objectContaining({ runId: "run-2", input: "再次告诉我" }),
      }),
    );
    expect(actions.filter((action) => action.type === "run/event")).toHaveLength(5);
  });

  it("returns the last replayed frame seq for the resumed run", () => {
    expect(afterSeqForTranscriptRun(makeTranscript(), "run-2")).toBe(80);
    expect(afterSeqForTranscriptRun(makeTranscript(), "missing")).toBe(0);
  });
});
