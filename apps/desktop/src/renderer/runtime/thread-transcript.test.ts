import { describe, expect, it } from "vitest";
import type { RuntimeThreadTranscript } from "../../shared/runtime";
import { createInitialRuntimeState, reduceRuntimeState } from "./reducer";
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
          finishedAt: "2026-06-24T00:00:10Z",
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
    expect(actions).toContainEqual({
      type: "run/requested",
      prompt: "你好",
      startedAtMs: Date.parse("2026-06-24T00:00:00Z"),
    });
    expect(actions).toContainEqual({
      type: "run/requested",
      prompt: "再次告诉我",
      startedAtMs: Date.parse("2026-06-24T00:01:00Z"),
    });
    expect(actions).toContainEqual({
      type: "run/event",
      event: { type: "RUN_FINISHED", seq: 48, runId: "run-1", threadId: "thread-1", result: "first" },
      receivedAtMs: Date.parse("2026-06-24T00:00:10Z"),
    });
    expect(actions).toContainEqual({
      type: "run/created",
      run: expect.objectContaining({ runId: "run-2", input: "再次告诉我" }),
    });
    expect(actions.filter((action) => action.type === "run/event")).toHaveLength(5);
  });

  it("preserves elapsed runtime when replaying a finished transcript run", () => {
    const state = actionsForThreadTranscript(makeTranscript()).reduce(
      reduceRuntimeState,
      createInitialRuntimeState(),
    );

    expect(state.narrative.committedItems).toContainEqual({
      kind: "answer",
      id: "answer:run-1:msg-1",
      markdown: "first",
      status: "done",
      isFinal: true,
      elapsedMs: 10000,
    });
  });

  it("synthesizes a replay answer from finished run result when text events are missing", () => {
    const transcript: RuntimeThreadTranscript = {
      thread: {
        threadId: "thread-result",
        label: "恢复历史",
        runId: "run-result",
        workspace: {
          workspaceRoot: "/tmp/awaken",
          writableRoots: ["/tmp/awaken"],
        },
        createdAt: "2026-06-24T00:00:00Z",
        updatedAt: "2026-06-24T00:00:10Z",
      },
      runs: [
        {
          run: {
            runId: "run-result",
            threadId: "thread-result",
            input: "恢复历史",
            status: "finished",
            result: "这是历史最终回答",
            cancelRequested: false,
            createdAt: "2026-06-24T00:00:00Z",
            updatedAt: "2026-06-24T00:00:10Z",
            finishedAt: "2026-06-24T00:00:10Z",
          },
          events: [
            { type: "RUN_STARTED", seq: 16, runId: "run-result", threadId: "thread-result" },
            { type: "RUN_FINISHED", seq: 32, runId: "run-result", threadId: "thread-result", result: "这是历史最终回答" },
          ],
        },
      ],
    };

    const actions = actionsForThreadTranscript(transcript);
    expect(actions).toContainEqual({
      type: "run/event",
      event: expect.objectContaining({
        type: "TEXT_MESSAGE_CONTENT",
        runId: "run-result",
        delta: "这是历史最终回答",
      }),
      receivedAtMs: undefined,
    });

    const state = actions.reduce(reduceRuntimeState, createInitialRuntimeState());
    expect(state.narrative.committedItems).toContainEqual({
      kind: "answer",
      id: "answer:run-result:transcript:run-result:result",
      markdown: "这是历史最终回答",
      status: "done",
      isFinal: true,
      elapsedMs: 10000,
    });
  });

  it("synthesizes a replay answer from cancelled run result when text events are missing", () => {
    const transcript: RuntimeThreadTranscript = {
      thread: {
        threadId: "thread-cancelled",
        label: "恢复中断",
        runId: "run-cancelled",
        workspace: {
          workspaceRoot: "/tmp/awaken",
          writableRoots: ["/tmp/awaken"],
        },
        createdAt: "2026-06-24T00:00:00Z",
        updatedAt: "2026-06-24T00:00:10Z",
      },
      runs: [
        {
          run: {
            runId: "run-cancelled",
            threadId: "thread-cancelled",
            input: "恢复中断",
            status: "cancelled",
            result: "这是中断前的回答",
            cancelRequested: true,
            createdAt: "2026-06-24T00:00:00Z",
            updatedAt: "2026-06-24T00:00:10Z",
            finishedAt: "2026-06-24T00:00:10Z",
          },
          events: [
            { type: "RUN_STARTED", seq: 16, runId: "run-cancelled", threadId: "thread-cancelled" },
            {
              type: "RUN_ERROR",
              seq: 32,
              runId: "run-cancelled",
              threadId: "thread-cancelled",
              code: "RUN_CANCELLED",
              message: "你停止了生成本次回答",
            },
          ],
        },
      ],
    };

    const actions = actionsForThreadTranscript(transcript);
    expect(actions).toContainEqual({
      type: "run/event",
      event: expect.objectContaining({
        type: "TEXT_MESSAGE_CONTENT",
        runId: "run-cancelled",
        delta: "这是中断前的回答",
      }),
      receivedAtMs: undefined,
    });

    const state = actions.reduce(reduceRuntimeState, createInitialRuntimeState());
    expect(state.narrative.committedItems).toContainEqual({
      kind: "answer",
      id: "answer:run-cancelled:transcript:run-cancelled:result",
      markdown: "这是中断前的回答",
      status: "done",
    });
    expect(state.narrative.committedItems).toContainEqual({
      kind: "hint",
      id: "hint:run-cancelled:run-cancelled",
      text: "你停止了生成本次回答",
    });
  });

  it("does not synthesize a duplicate answer when text events already exist", () => {
    const actions = actionsForThreadTranscript(makeTranscript());
    const textContentActions = actions.filter(
      (action) => action.type === "run/event" && action.event.type === "TEXT_MESSAGE_CONTENT",
    );

    expect(textContentActions).toHaveLength(1);
  });

  it("returns the last replayed frame seq for the resumed run", () => {
    expect(afterSeqForTranscriptRun(makeTranscript(), "run-2")).toBe(80);
    expect(afterSeqForTranscriptRun(makeTranscript(), "missing")).toBe(0);
  });
});
