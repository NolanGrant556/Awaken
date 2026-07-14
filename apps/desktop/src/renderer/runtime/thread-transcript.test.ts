import { describe, expect, it } from "vitest";
import type { RuntimeThreadTranscript } from "../../shared/runtime";
import {
  createInitialRuntimeState,
  reduceRuntimeState,
  type RuntimeAction,
} from "./reducer";
import {
  actionsForThreadTranscript,
  streamCursorForTranscriptRun,
} from "./thread-transcript";

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
  it("replays durable attachment cards and composer tokens without text inference", () => {
    const transcript: RuntimeThreadTranscript = {
      thread: {
        threadId: "thread-structured-input",
        label: "附件任务",
        runId: "run-structured-input",
        workspace: {
          workspaceRoot: "/tmp/awaken",
          writableRoots: ["/tmp/awaken"],
        },
        createdAt: "2026-07-11T00:00:00Z",
        updatedAt: "2026-07-11T00:00:10Z",
      },
      runs: [
        {
          run: {
            runId: "run-structured-input",
            threadId: "thread-structured-input",
            input: "",
            inputAttachments: [
              {
                id: "file-1",
                kind: "file",
                mimeType: "text/plain",
                path: "/tmp/acceptance.txt",
                displayName: "acceptance.txt",
                byteSize: 17,
              },
            ],
            composerTokens: [
              {
                id: "skill:docs:user",
                type: "skill",
                label: "docs",
                payload: {
                  skillId: "docs",
                  sourceOrigin: "user",
                  skillFilePath: "/tmp/skills/docs/SKILL.md",
                },
              },
            ],
            status: "finished",
            result: "done",
            cancelRequested: false,
            createdAt: "2026-07-11T00:00:00Z",
            updatedAt: "2026-07-11T00:00:10Z",
            finishedAt: "2026-07-11T00:00:10Z",
          },
          events: [],
        },
      ],
    };

    const actions = actionsForThreadTranscript(transcript);
    expect(actions).toContainEqual({
      type: "run/requested",
      prompt: "",
      promptAttachments: transcript.runs[0].run.inputAttachments,
      promptTokens: transcript.runs[0].run.composerTokens,
      startedAtMs: Date.parse("2026-07-11T00:00:00Z"),
    });

    const state = actions.reduce(reduceRuntimeState, createInitialRuntimeState());
    expect(state.narrative.committedItems).toContainEqual({
      kind: "prompt",
      id: "prompt:thread-structured-input:1",
      text: "",
      attachments: transcript.runs[0].run.inputAttachments,
      tokens: transcript.runs[0].run.composerTokens,
    });
  });

  it("replays inline token position through durable leadingText", () => {
    const leadingText = "现在我需要你帮我分析现有的财务数据风险，然后使用 ";
    const transcript: RuntimeThreadTranscript = {
      thread: {
        threadId: "thread-inline-token",
        label: "财务风险报告",
        runId: "run-inline-token",
        workspace: {
          workspaceRoot: "/tmp/awaken",
          writableRoots: ["/tmp/awaken"],
        },
        createdAt: "2026-07-13T00:00:00Z",
        updatedAt: "2026-07-13T00:00:10Z",
      },
      runs: [
        {
          run: {
            runId: "run-inline-token",
            threadId: "thread-inline-token",
            input:
              "现在我需要你帮我分析现有的财务数据风险，然后使用 输出一份风险报告给我",
            composerTokens: [
              {
                id: "skill:pptx:user",
                type: "skill",
                label: "pptx",
                leadingText,
                payload: {
                  skillId: "pptx",
                  sourceOrigin: "user",
                  skillFilePath: "/tmp/skills/pptx/SKILL.md",
                },
              },
            ],
            status: "finished",
            result: "done",
            cancelRequested: false,
            createdAt: "2026-07-13T00:00:00Z",
            updatedAt: "2026-07-13T00:00:10Z",
            finishedAt: "2026-07-13T00:00:10Z",
          },
          events: [],
        },
      ],
    };

    const actions = actionsForThreadTranscript(transcript);
    const state = actions.reduce(reduceRuntimeState, createInitialRuntimeState());
    const promptItem = state.narrative.committedItems.find(
      (item) => item.kind === "prompt",
    );
    expect(promptItem?.kind).toBe("prompt");
    if (promptItem?.kind !== "prompt") {
      throw new Error("prompt item missing");
    }
    expect(promptItem.tokens?.[0]?.leadingText).toBe(leadingText);
    expect(promptItem.text).toBe(
      "现在我需要你帮我分析现有的财务数据风险，然后使用 输出一份风险报告给我",
    );
  });

  it("restores assistant text when one semantic record projects to frames with the same seq", () => {
    const transcript: RuntimeThreadTranscript = {
      thread: {
        threadId: "thread-semantic",
        label: "历史重放",
        runId: "run-semantic",
        workspace: {
          workspaceRoot: "/tmp/awaken",
          writableRoots: ["/tmp/awaken"],
        },
        createdAt: "2026-07-10T00:00:00Z",
        updatedAt: "2026-07-10T00:00:10Z",
      },
      runs: [
        {
          run: {
            runId: "run-semantic",
            threadId: "thread-semantic",
            input: "恢复历史",
            status: "finished",
            cancelRequested: false,
            createdAt: "2026-07-10T00:00:00Z",
            updatedAt: "2026-07-10T00:00:10Z",
            finishedAt: "2026-07-10T00:00:10Z",
          },
          events: [
            {
              type: "RUN_STARTED",
              seq: 1,
              runId: "run-semantic",
              threadId: "thread-semantic",
            },
            {
              type: "TEXT_MESSAGE_START",
              seq: 2,
              runId: "run-semantic",
              threadId: "thread-semantic",
              messageId: "msg-semantic",
              role: "assistant",
            },
            {
              type: "TEXT_MESSAGE_CONTENT",
              seq: 2,
              runId: "run-semantic",
              threadId: "thread-semantic",
              messageId: "msg-semantic",
              role: "assistant",
              delta: "历史回答正文",
            },
            {
              type: "TEXT_MESSAGE_END",
              seq: 2,
              runId: "run-semantic",
              threadId: "thread-semantic",
              messageId: "msg-semantic",
              role: "assistant",
            },
            {
              type: "RUN_FINISHED",
              seq: 3,
              runId: "run-semantic",
              threadId: "thread-semantic",
              result: "历史回答正文",
            },
          ],
        },
      ],
    };

    const state = actionsForThreadTranscript(transcript).reduce(
      reduceRuntimeState,
      createInitialRuntimeState(),
    );

    expect(state.narrative.committedItems).toContainEqual({
      kind: "answer",
      id: "answer:run-semantic:msg-semantic",
      markdown: "历史回答正文",
      status: "done",
      isFinal: true,
      elapsedMs: 10000,
    });
  });

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

  it("keeps live and replay elapsed time identical across client and SSE latency", () => {
    const startedAtMs = Date.parse("2026-07-10T06:59:38.906Z");
    const finishedAtMs = Date.parse("2026-07-10T07:00:22.299Z");
    const run: RuntimeThreadTranscript["runs"][number]["run"] = {
      runId: "run-duration",
      threadId: "thread-duration",
      input: "完整验收",
      status: "finished",
      result: "验收完成",
      cancelRequested: false,
      createdAt: new Date(startedAtMs).toISOString(),
      updatedAt: new Date(finishedAtMs).toISOString(),
      finishedAt: new Date(finishedAtMs).toISOString(),
    };
    const startedEvent = {
      type: "RUN_STARTED" as const,
      runId: run.runId,
      threadId: run.threadId,
      timestamp: startedAtMs,
    };
    const contentEvent = {
      type: "TEXT_MESSAGE_CONTENT" as const,
      runId: run.runId,
      threadId: run.threadId,
      messageId: "msg-duration",
      delta: "验收完成",
    };
    const finishedEvent = {
      type: "RUN_FINISHED" as const,
      runId: run.runId,
      threadId: run.threadId,
      result: "验收完成",
      timestamp: finishedAtMs,
    };

    const liveActions: RuntimeAction[] = [
      {
        type: "run/requested",
        prompt: "完整验收",
        startedAtMs: startedAtMs - 4100,
      },
      { type: "run/created", run },
      {
        type: "run/event",
        event: startedEvent,
        receivedAtMs: startedAtMs + 400,
      },
      { type: "run/event", event: contentEvent },
      {
        type: "run/event",
        event: finishedEvent,
        receivedAtMs: finishedAtMs + 5000,
      },
    ];
    const liveState = liveActions.reduce(
      reduceRuntimeState,
      createInitialRuntimeState(),
    );

    const transcript: RuntimeThreadTranscript = {
      thread: {
        threadId: run.threadId,
        label: "完整验收",
        runId: run.runId,
        workspace: {
          workspaceRoot: "/tmp/awaken",
          writableRoots: ["/tmp/awaken"],
        },
        createdAt: run.createdAt,
        updatedAt: run.updatedAt,
      },
      runs: [
        {
          run,
          events: [startedEvent, contentEvent, finishedEvent],
        },
      ],
    };
    const replayState = actionsForThreadTranscript(transcript).reduce(
      reduceRuntimeState,
      createInitialRuntimeState(),
    );

    const elapsedFor = (state: typeof liveState) =>
      state.narrative.committedItems.find(
        (item) => item.kind === "answer" && item.id.includes("msg-duration"),
      );
    expect(elapsedFor(liveState)).toMatchObject({ elapsedMs: 43393 });
    expect(elapsedFor(replayState)).toMatchObject({ elapsedMs: 43393 });
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
    expect(streamCursorForTranscriptRun(makeTranscript(), "run-2")).toBe(80);
    expect(streamCursorForTranscriptRun(makeTranscript(), "missing")).toBe(0);
  });

  it("prefers the active transcript stream cursor over semantic frame seq", () => {
    const transcript = makeTranscript();
    transcript.runs[1].streamCursor = 511;
    transcript.runs[1].events[0].seq = 9999;
    expect(streamCursorForTranscriptRun(transcript, "run-2")).toBe(511);
  });
});
