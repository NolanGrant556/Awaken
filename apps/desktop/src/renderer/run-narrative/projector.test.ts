import { describe, expect, it } from "vitest";
import {
  createInitialRunNarrativeState,
  projectAguiEventToNarrative,
} from "./projector";
import type { AguiEvent } from "../runtime/agui";

function textMessageStart(runId = "run-1", messageId = "msg-1"): AguiEvent {
  return {
    type: "TEXT_MESSAGE_START",
    rawType: "TEXT_MESSAGE_START",
    runId,
    messageId,
  };
}

function textMessageContent(
  delta: string,
  runId = "run-1",
  messageId = "msg-1",
): AguiEvent {
  return {
    type: "TEXT_MESSAGE_CONTENT",
    rawType: "TEXT_MESSAGE_CONTENT",
    runId,
    messageId,
    delta,
  };
}

function textMessageEnd(runId = "run-1", messageId = "msg-1"): AguiEvent {
  return {
    type: "TEXT_MESSAGE_END",
    rawType: "TEXT_MESSAGE_END",
    runId,
    messageId,
  };
}

function runError(code: string, message?: string): AguiEvent {
  return {
    type: "RUN_ERROR",
    rawType: "RUN_ERROR",
    runId: "run-1",
    code,
    message,
  };
}

function runStatusChanged(
  status: "running" | "cancelling" | "cancelled" | "finished" | "error",
  message?: string,
): AguiEvent {
  return {
    type: "RUN_STATUS_CHANGED",
    rawType: "RUN_STATUS_CHANGED",
    runId: "run-1",
    status,
    message,
  };
}

describe("projectAguiEventToNarrative", () => {
  it("projects only Core-derived Write/Edit preparation while keeping raw deltas private", () => {
    let normal = projectAguiEventToNarrative(createInitialRunNarrativeState(), {
      type: "TOOL_CALL_ARGS_DELTA",
      runId: "run-1",
      toolCallId: "write-1",
      toolCallName: "Write",
      delta: '{"file_path":"/private/report.md","content":"private fragment',
      toolPreparationKind: "write",
      toolPreparationPath: "/private/report.md",
      toolPreparationAddedLines: 1,
    });
    normal = projectAguiEventToNarrative(normal, {
      type: "TOOL_CALL_ARGS_DELTA",
      runId: "run-1",
      toolCallId: "write-1",
      toolCallName: "Write",
      delta: "\\nmore private content",
      toolPreparationKind: "write",
      toolPreparationPath: "report.md",
      toolPreparationAddedLines: 2,
    });
    const media = projectAguiEventToNarrative(
      createInitialRunNarrativeState(),
      {
      type: "TOOL_CALL_ARGS_DELTA",
      runId: "run-1",
      toolCallId: "image-1",
      toolCallName: "image_generate",
      delta: '{"prompt":"partial',
      toolPreparationKind: "write",
      toolPreparationAddedLines: 99,
      },
    );

    expect(normal.activeItem).toEqual({
      kind: "tool",
      id: "tool:run-1:write-1",
      toolCallId: "write-1",
      title: "写入 report.md",
      detail: "+2 行",
      status: "active",
    });
    expect(normal.committedItems).toEqual([]);
    expect(JSON.stringify(normal)).not.toContain("private fragment");
    expect(JSON.stringify(normal)).not.toContain("more private content");
    expect(JSON.stringify(normal)).not.toContain("/private/");
    expect(media.activeItem).toBeUndefined();
    expect(media.committedItems).toEqual([]);
    expect(JSON.stringify(media)).not.toContain("partial");
  });

  it("uses only complete tool arguments and keeps END non-terminal", () => {
    let state = createInitialRunNarrativeState();
    const base = {
      runId: "run-1",
      toolCallId: "write-1",
      toolCallName: "Write",
    };
    state = projectAguiEventToNarrative(state, {
      type: "TOOL_CALL_START",
      ...base,
    });
    state = projectAguiEventToNarrative(state, {
      type: "TOOL_CALL_ARGS",
      ...base,
      delta: JSON.stringify({
        file_path: "/tmp/report.md",
        content: "private document body",
      }),
    });
    state = projectAguiEventToNarrative(state, {
      type: "TOOL_CALL_END",
      ...base,
    });

    expect(state.activeItem).toEqual({
      kind: "tool",
      id: "tool:run-1:write-1",
      toolCallId: "write-1",
      title: "Write",
      detail: undefined,
      status: "active",
    });
    expect(JSON.stringify(state)).not.toContain("private document body");
  });

  it("uses a basename-only semantic row for completed read-only arguments", () => {
    let state = projectAguiEventToNarrative(createInitialRunNarrativeState(), {
      type: "TOOL_CALL_START",
      runId: "run-1",
      toolCallId: "read-1",
      toolCallName: "Read",
    });
    state = projectAguiEventToNarrative(state, {
      type: "TOOL_CALL_ARGS",
      runId: "run-1",
      toolCallId: "read-1",
      toolCallName: "Read",
      delta: JSON.stringify({
        file_path: "/tmp/reports/quarterly-report.pdf",
      }),
    });

    expect(state.activeItem).toMatchObject({
      kind: "exploring",
      actions: [
        {
          title: "读取 quarterly-report.pdf",
          status: "active",
        },
      ],
    });
    expect(JSON.stringify(state)).not.toContain("/tmp/reports/");
  });

  it("marks an unresolved active tool as cancelled when the run stops", () => {
    let state = projectAguiEventToNarrative(createInitialRunNarrativeState(), {
      type: "TOOL_CALL_START",
      runId: "run-1",
      toolCallId: "write-1",
      toolCallName: "Write",
    });
    state = projectAguiEventToNarrative(state, runStatusChanged("cancelled"));

    expect(state.committedItems).toContainEqual({
      kind: "tool",
      id: "tool:run-1:write-1",
      toolCallId: "write-1",
      title: "Write",
      detail: undefined,
      status: "cancelled",
    });
  });

  it("shows a transient model status after a tool-result continuation", () => {
    let state = createInitialRunNarrativeState();
    state = projectAguiEventToNarrative(state, {
      type: "TOOL_CALL_START",
      runId: "run-1",
      toolCallId: "read-1",
      toolCallName: "Read",
    });
    state = projectAguiEventToNarrative(state, {
      type: "TOOL_CALL_RESULT",
      runId: "run-1",
      toolCallId: "read-1",
      toolCallName: "Read",
      toolStatus: "succeeded",
      content: "document parts ready",
    });
    state = projectAguiEventToNarrative(state, {
      type: "AWAKEN_TURN_STARTED",
      runId: "run-1",
      turnId: "turn-2",
      turnCount: 2,
      transitionReason: "tool_result_continuation",
    });

    expect(state.activeItem).toEqual({
      kind: "planning",
      id: "planning:run-1:turn-2",
      text: "正在分析工具结果",
      status: "active",
      phase: "continuation",
    });

    state = projectAguiEventToNarrative(state, {
      type: "REASONING_START",
      runId: "run-1",
      messageId: "reasoning-2",
    });
    expect(state.activeItem).toMatchObject({
      kind: "thinking",
      id: "thinking:run-1:reasoning-2",
      status: "active",
    });
  });

  it("uses continuation-specific labels and ignores an initial model turn", () => {
    const transitions = [
      ["reactive_compact_retry", "上下文压缩中"],
      ["max_output_continuation", "正在继续生成"],
      ["xml_fallback_continuation", "正在继续处理工具结果"],
    ] as const;

    for (const [transitionReason, text] of transitions) {
      const state = projectAguiEventToNarrative(
        createInitialRunNarrativeState(),
        {
        type: "AWAKEN_TURN_STARTED",
        runId: "run-1",
        turnId: `turn-${transitionReason}`,
        transitionReason,
        },
      );
      expect(state.activeItem).toMatchObject({
        kind: "planning",
        text,
        phase:
          transitionReason === "reactive_compact_retry"
            ? "compaction"
            : "continuation",
      });
    }

    const initial = projectAguiEventToNarrative(
      createInitialRunNarrativeState(),
      {
      type: "AWAKEN_TURN_STARTED",
      runId: "run-1",
      turnId: "turn-1",
      transitionReason: "initial",
      },
    );
    expect(initial.activeItem).toBeUndefined();
  });

  it("keeps one live compaction status across compact completion and retry", () => {
    let state = projectAguiEventToNarrative(
      createInitialRunNarrativeState(),
      {
        type: "AWAKEN_COMPACT_STARTED",
        runId: "run-1",
        seq: 1,
        contextCompactMode: "reactive",
      },
    );

    expect(state.activeItem).toEqual({
      kind: "planning",
      id: "planning:run-1:context-compaction",
      text: "上下文压缩中",
      status: "active",
      phase: "compaction",
    });

    state = projectAguiEventToNarrative(state, {
      type: "AWAKEN_COMPACT_FINISHED",
      runId: "run-1",
      seq: 2,
      contextCompactMode: "reactive",
      contextCompactBoundaryId: "compact-1",
      contextCompactPreEstimateTokens: 120000,
      contextCompactPostEstimateTokens: 48000,
    });
    state = projectAguiEventToNarrative(state, {
      type: "AWAKEN_TURN_STARTED",
      runId: "run-1",
      turnId: "turn-2",
      turnCount: 2,
      transitionReason: "reactive_compact_retry",
    });

    expect(state.activeItem).toMatchObject({
      kind: "planning",
      id: "planning:run-1:context-compaction",
      text: "上下文压缩中",
      phase: "compaction",
    });
    expect(
      state.committedItems.filter((item) => item.kind === "planning"),
    ).toHaveLength(0);
    expect(
      state.committedItems.filter((item) => item.kind === "compactBoundary"),
    ).toHaveLength(1);
  });

  it("lets the next answer, tool, or terminal event replace the transient model status", () => {
    const waitingState = () =>
      projectAguiEventToNarrative(createInitialRunNarrativeState(), {
        type: "AWAKEN_TURN_STARTED",
        runId: "run-1",
        turnId: "turn-2",
        transitionReason: "tool_result_continuation",
      });

    const answering = projectAguiEventToNarrative(
      waitingState(),
      textMessageStart(),
    );
    expect(answering.activeItem).toMatchObject({
      kind: "answer",
      status: "active",
    });

    const usingTool = projectAguiEventToNarrative(waitingState(), {
      type: "TOOL_CALL_START",
      runId: "run-1",
      toolCallId: "tool-2",
      toolCallName: "Glob",
    });
    expect(usingTool.activeItem).toMatchObject({
      kind: "exploring",
      status: "active",
    });

    const finished = projectAguiEventToNarrative(waitingState(), {
      type: "RUN_FINISHED",
      runId: "run-1",
    });
    expect(finished.activeItem).toBeUndefined();
  });

  it("keeps all projected text frames that share one semantic record sequence", () => {
    let state = createInitialRunNarrativeState();
    state = projectAguiEventToNarrative(state, {
      ...textMessageStart(),
      seq: 7,
    });
    state = projectAguiEventToNarrative(state, {
      ...textMessageContent("历史回答正文"),
      seq: 7,
    });
    state = projectAguiEventToNarrative(state, {
      ...textMessageEnd(),
      seq: 7,
    });

    expect(state.committedItems).toContainEqual({
      kind: "answer",
      id: "answer:run-1:msg-1",
      markdown: "历史回答正文",
      status: "done",
    });
  });

  it("reconciles a durable assistant snapshot with already streamed deltas", () => {
    let state = createInitialRunNarrativeState();
    state = projectAguiEventToNarrative(state, textMessageStart());
    state = projectAguiEventToNarrative(
      state,
      textMessageContent("streamed answer"),
    );
    state = projectAguiEventToNarrative(state, {
      ...textMessageStart(),
      seq: 32,
      semanticSeq: 48,
      recordId: "rec_assistant_1",
    });
    state = projectAguiEventToNarrative(state, {
      ...textMessageContent("streamed answer"),
      seq: 33,
      semanticSeq: 49,
      recordId: "rec_assistant_1",
    });
    state = projectAguiEventToNarrative(state, {
      ...textMessageEnd(),
      seq: 34,
      semanticSeq: 50,
      recordId: "rec_assistant_1",
    });

    expect(state.committedItems).toContainEqual({
      kind: "answer",
      id: "answer:run-1:msg-1",
      markdown: "streamed answer",
      status: "done",
    });
    expect(
      [
      ...state.committedItems,
      ...(state.activeItem ? [state.activeItem] : []),
      ].map((item) => item.id),
    ).toHaveLength(
      new Set(
        [
      ...state.committedItems,
      ...(state.activeItem ? [state.activeItem] : []),
        ].map((item) => item.id),
      ).size,
    );
  });

  it("reconciles a durable reasoning snapshot with already streamed deltas", () => {
    let state = createInitialRunNarrativeState();
    state = projectAguiEventToNarrative(state, {
      type: "REASONING_MESSAGE_START",
      rawType: "REASONING_MESSAGE_START",
      runId: "run-1",
      messageId: "reasoning-1",
    });
    state = projectAguiEventToNarrative(state, {
      type: "REASONING_MESSAGE_CONTENT",
      rawType: "REASONING_MESSAGE_CONTENT",
      runId: "run-1",
      messageId: "reasoning-1",
      delta: "先检查技能目录",
    });
    state = projectAguiEventToNarrative(state, {
      type: "REASONING_MESSAGE_START",
      rawType: "REASONING_MESSAGE_START",
      runId: "run-1",
      messageId: "reasoning-1",
      seq: 40,
      semanticSeq: 50,
      recordId: "rec-reasoning-1",
    });
    state = projectAguiEventToNarrative(state, {
      type: "REASONING_MESSAGE_CONTENT",
      rawType: "REASONING_MESSAGE_CONTENT",
      runId: "run-1",
      messageId: "reasoning-1",
      delta: "先检查技能目录",
      seq: 41,
      semanticSeq: 51,
      recordId: "rec-reasoning-1",
    });
    state = projectAguiEventToNarrative(state, {
      type: "REASONING_MESSAGE_END",
      rawType: "REASONING_MESSAGE_END",
      runId: "run-1",
      messageId: "reasoning-1",
      seq: 42,
      semanticSeq: 52,
      recordId: "rec-reasoning-1",
    });

    expect(state.committedItems).toContainEqual({
      kind: "thinking",
      id: "thinking:run-1:reasoning-1",
      text: "先检查技能目录",
      status: "done",
    });
    const visibleIds = [
      ...state.committedItems,
      ...(state.activeItem ? [state.activeItem] : []),
    ].map((item) => item.id);
    expect(visibleIds).toHaveLength(new Set(visibleIds).size);
  });

  it("keeps reasoning before the answer when its durable snapshot arrives after streamed text", () => {
    let state = createInitialRunNarrativeState();
    state = projectAguiEventToNarrative(state, {
      type: "REASONING_MESSAGE_START",
      rawType: "REASONING_MESSAGE_START",
      runId: "run-1",
      messageId: "reasoning-1",
    });
    state = projectAguiEventToNarrative(state, {
      type: "REASONING_MESSAGE_CONTENT",
      rawType: "REASONING_MESSAGE_CONTENT",
      runId: "run-1",
      messageId: "reasoning-1",
      delta: "先思考",
    });
    state = projectAguiEventToNarrative(state, textMessageStart());
    state = projectAguiEventToNarrative(state, textMessageContent("最终回答"));

    for (const event of [
      {
        type: "REASONING_MESSAGE_START",
        rawType: "REASONING_MESSAGE_START",
        runId: "run-1",
        messageId: "reasoning-1",
        seq: 50,
        recordId: "rec-reasoning-1",
      },
      {
        type: "REASONING_MESSAGE_CONTENT",
        rawType: "REASONING_MESSAGE_CONTENT",
        runId: "run-1",
        messageId: "reasoning-1",
        delta: "先思考",
        seq: 51,
        recordId: "rec-reasoning-1",
      },
      {
        type: "REASONING_MESSAGE_END",
        rawType: "REASONING_MESSAGE_END",
        runId: "run-1",
        messageId: "reasoning-1",
        seq: 52,
        recordId: "rec-reasoning-1",
      },
      { ...textMessageStart(), seq: 60, recordId: "rec-answer-1" },
      { ...textMessageContent("最终回答"), seq: 61, recordId: "rec-answer-1" },
      { ...textMessageEnd(), seq: 62, recordId: "rec-answer-1" },
    ] satisfies AguiEvent[]) {
      state = projectAguiEventToNarrative(state, event);
    }

    expect(state.committedItems.map((item) => item.id)).toEqual([
      "thinking:run-1:reasoning-1",
      "answer:run-1:msg-1",
    ]);
    expect(state.committedItems[0]).toMatchObject({ text: "先思考" });
    expect(state.committedItems[1]).toMatchObject({ markdown: "最终回答" });
  });

  it("reconciles terminal durable snapshots without reactivating rendered thought or answer rows", () => {
    let state = createInitialRunNarrativeState();
    state = projectAguiEventToNarrative(state, {
      type: "REASONING_MESSAGE_START",
      rawType: "REASONING_MESSAGE_START",
      runId: "run-1",
      messageId: "reasoning-1",
    });
    state = projectAguiEventToNarrative(state, {
      type: "REASONING_MESSAGE_CONTENT",
      rawType: "REASONING_MESSAGE_CONTENT",
      runId: "run-1",
      messageId: "reasoning-1",
      delta: "流式思考",
    });
    state = projectAguiEventToNarrative(state, textMessageStart());
    state = projectAguiEventToNarrative(state, textMessageContent("流式回答"));
    state = projectAguiEventToNarrative(state, textMessageEnd());

    expect(state.activeItem).toBeUndefined();
    expect(state.committedItems.map((item) => item.id)).toEqual([
      "thinking:run-1:reasoning-1",
      "answer:run-1:msg-1",
    ]);

    const durableReasoningStart = {
      type: "REASONING_MESSAGE_START",
      rawType: "REASONING_MESSAGE_START",
      runId: "run-1",
      messageId: "reasoning-1",
      seq: 33,
      recordId: "rec-reasoning-1",
    } satisfies AguiEvent;
    const beforeDurableReasoningStart = state;
    state = projectAguiEventToNarrative(state, durableReasoningStart);
    expect(state.committedItems).toBe(
      beforeDurableReasoningStart.committedItems,
    );
    expect(state.activeItem).toBeUndefined();
    expect(state.committedItems.map((item) => item.id)).toEqual([
      "thinking:run-1:reasoning-1",
      "answer:run-1:msg-1",
    ]);

    state = projectAguiEventToNarrative(state, {
      ...durableReasoningStart,
      type: "REASONING_MESSAGE_CONTENT",
      rawType: "REASONING_MESSAGE_CONTENT",
      seq: 34,
      delta: "耐久思考快照",
    });
    state = projectAguiEventToNarrative(state, {
      ...durableReasoningStart,
      type: "REASONING_MESSAGE_END",
      rawType: "REASONING_MESSAGE_END",
      seq: 35,
    });
    expect(state.activeItem).toBeUndefined();
    expect(state.committedItems[0]).toMatchObject({
      text: "耐久思考快照",
      status: "done",
    });

    const durableAnswerStart = {
      ...textMessageStart(),
      seq: 36,
      recordId: "rec-answer-1",
    } satisfies AguiEvent;
    const beforeDurableAnswerStart = state;
    state = projectAguiEventToNarrative(state, durableAnswerStart);
    expect(state.committedItems).toBe(beforeDurableAnswerStart.committedItems);
    expect(state.activeItem).toBeUndefined();
    state = projectAguiEventToNarrative(state, {
      ...textMessageContent("耐久回答快照"),
      seq: 37,
      recordId: "rec-answer-1",
    });
    state = projectAguiEventToNarrative(state, {
      ...textMessageEnd(),
      seq: 38,
      recordId: "rec-answer-1",
    });

    expect(state.activeItem).toBeUndefined();
    expect(state.committedItems).toEqual([
      {
        kind: "thinking",
        id: "thinking:run-1:reasoning-1",
        text: "耐久思考快照",
        status: "done",
      },
      {
        kind: "answer",
        id: "answer:run-1:msg-1",
        markdown: "耐久回答快照",
        status: "done",
      },
    ]);

    state = projectAguiEventToNarrative(state, {
      type: "RUN_FINISHED",
      rawType: "RUN_FINISHED",
      runId: "run-1",
      seq: 39,
    });
    expect(state.activeItem).toBeUndefined();
    expect(state.committedItems.map((item) => item.id)).toEqual([
      "thinking:run-1:reasoning-1",
      "answer:run-1:msg-1",
    ]);
    expect(state.committedItems[1]).toMatchObject({
      kind: "answer",
      markdown: "耐久回答快照",
      status: "done",
      isFinal: true,
    });
  });

  it("adds a hint item when a run is cancelled by the user", () => {
    let state = createInitialRunNarrativeState();
    state = projectAguiEventToNarrative(state, textMessageStart());
    state = projectAguiEventToNarrative(state, textMessageContent("hello"));

    state = projectAguiEventToNarrative(state, runError("RUN_CANCELLED"));

    const answer = state.committedItems.find((item) => item.kind === "answer");
    expect(answer).toBeDefined();
    expect(answer?.kind === "answer" && answer.status).toBe("done");

    const hint = state.committedItems.find((item) => item.kind === "hint");
    expect(hint).toBeDefined();
    expect(hint?.kind === "hint" && hint.text).toBe("你停止了生成本次回答");
  });

  it("adds a hint item from message when run status changes to cancelled", () => {
    let state = createInitialRunNarrativeState();
    state = projectAguiEventToNarrative(state, textMessageStart());
    state = projectAguiEventToNarrative(state, textMessageContent("hello"));

    state = projectAguiEventToNarrative(
      state,
      runStatusChanged("cancelled", "你停止生成了本次回答"),
    );

    const answer = state.committedItems.find((item) => item.kind === "answer");
    expect(answer).toBeDefined();
    expect(answer?.kind === "answer" && answer.status).toBe("done");

    const hint = state.committedItems.find((item) => item.kind === "hint");
    expect(hint).toBeDefined();
    expect(hint?.kind === "hint" && hint.text).toBe("你停止生成了本次回答");
  });

  it("uses a default hint text when cancelled status has no message", () => {
    let state = createInitialRunNarrativeState();
    state = projectAguiEventToNarrative(state, textMessageStart());
    state = projectAguiEventToNarrative(state, textMessageContent("hello"));

    state = projectAguiEventToNarrative(state, runStatusChanged("cancelled"));

    const answer = state.committedItems.find((item) => item.kind === "answer");
    expect(answer).toBeDefined();

    const hint = state.committedItems.find((item) => item.kind === "hint");
    expect(hint?.kind === "hint" && hint.text).toBe("你停止生成了本次回答");
  });

  it("keeps answer content when cancelled mid-stream", () => {
    let state = createInitialRunNarrativeState();
    state = projectAguiEventToNarrative(state, textMessageStart());
    state = projectAguiEventToNarrative(state, textMessageContent("partial"));

    state = projectAguiEventToNarrative(state, runError("RUN_CANCELLED"));

    const answer = state.committedItems.find((item) => item.kind === "answer");
    expect(answer?.kind === "answer" && answer.markdown).toBe("partial");

    const hint = state.committedItems.find((item) => item.kind === "hint");
    expect(hint?.kind === "hint" && hint.text).toBe("你停止了生成本次回答");
  });

  it("adds a hint item after an already-committed answer when cancelled late", () => {
    let state = createInitialRunNarrativeState();
    state = projectAguiEventToNarrative(state, textMessageStart());
    state = projectAguiEventToNarrative(state, textMessageContent("hello"));
    state = projectAguiEventToNarrative(state, textMessageEnd());

    state = projectAguiEventToNarrative(state, runError("RUN_CANCELLED"));

    const answer = state.committedItems.find((item) => item.kind === "answer");
    expect(answer).toBeDefined();
    expect(answer?.kind === "answer" && answer.status).toBe("done");

    const hint = state.committedItems.find((item) => item.kind === "hint");
    expect(hint).toBeDefined();
    expect(hint?.kind === "hint" && hint.text).toBe("你停止了生成本次回答");
  });

  it("deduplicates cancel hint items when both RUN_STATUS_CHANGED and RUN_ERROR arrive", () => {
    let state = createInitialRunNarrativeState();
    state = projectAguiEventToNarrative(state, textMessageStart());
    state = projectAguiEventToNarrative(state, textMessageContent("hello"));

    state = projectAguiEventToNarrative(state, runStatusChanged("cancelled"));
    state = projectAguiEventToNarrative(state, runError("RUN_CANCELLED"));

    const hints = state.committedItems.filter((item) => item.kind === "hint");
    expect(hints).toHaveLength(1);
    expect(hints[0]?.kind === "hint" && hints[0].text).toBe(
      "你停止了生成本次回答",
    );
  });

  it("maps toolStatus to ToolDisplayState without content heuristics", () => {
    let state = createInitialRunNarrativeState();
    state = projectAguiEventToNarrative(state, {
      type: "TOOL_CALL_START",
      toolCallId: "shell-1",
      toolCallName: "shell",
    });
    state = projectAguiEventToNarrative(state, {
      type: "TOOL_CALL_RESULT",
      toolCallId: "shell-1",
      toolCallName: "shell",
      content: "",
      toolStatus: "succeeded",
    });

    const tool = state.committedItems.find((item) => item.kind === "tool");
    expect(tool).toMatchObject({
      kind: "tool",
      status: "done",
      toolStatus: "succeeded",
    });

    state = createInitialRunNarrativeState();
    state = projectAguiEventToNarrative(state, {
      type: "TOOL_CALL_RESULT",
      toolCallId: "shell-2",
      toolCallName: "shell",
      content: "工具执行失败: quoted",
      toolStatus: "failed",
      processExitCode: 7,
    });
    const failed = state.committedItems.find((item) => item.kind === "tool");
    expect(failed).toMatchObject({
      kind: "tool",
      status: "error",
      toolStatus: "failed",
    });
  });

  it("projects an invoked Skill as one replay-stable trace row", () => {
    let state = createInitialRunNarrativeState();
    state = projectAguiEventToNarrative(state, {
      type: "AWAKEN_SKILL_DISCOVERED",
      runId: "run-skill",
      skillId: "documents",
    });
    expect(state.committedItems).toHaveLength(0);

    state = projectAguiEventToNarrative(state, {
      type: "AWAKEN_SKILL_INVOKED",
      runId: "run-skill",
      skillId: "documents",
      skillVersion: "1.2.0",
      skillSource: "project",
    });
    expect(state.committedItems).toContainEqual({
      kind: "skill",
      id: "skill:run-skill:documents",
      title: "documents",
      detail: "project · v1.2.0",
      output: undefined,
      status: "active",
    });

    state = projectAguiEventToNarrative(state, {
      type: "AWAKEN_SKILL_EXECUTION_CONTEXT",
      runId: "run-skill",
      skillId: "documents",
      skillVersion: "1.2.0",
      skillSource: "project",
    });
    state = projectAguiEventToNarrative(state, {
      type: "AWAKEN_SKILL_CONTENT_LOADED",
      runId: "run-skill",
      skillId: "documents",
      skillVersion: "1.2.0",
      skillSource: "project",
      skillResource: "references/layout.md",
    });

    const skills = state.committedItems.filter((item) => item.kind === "skill");
    expect(skills).toHaveLength(1);
    expect(skills[0]).toMatchObject({
      status: "done",
      output: "已加载资源：references/layout.md",
    });
  });

  it("keeps Skill load failures visible as an expandable error trace", () => {
    let state = createInitialRunNarrativeState();
    state = projectAguiEventToNarrative(state, {
      type: "AWAKEN_SKILL_FAILED_TO_LOAD",
      runId: "run-skill-failed",
      skillId: "broken-skill",
      skillDiagnostics: ["SKILL.md is missing"],
    });

    expect(state.committedItems).toContainEqual({
      kind: "skill",
      id: "skill:run-skill-failed:broken-skill",
      title: "broken-skill",
      detail: undefined,
      output: "SKILL.md is missing",
      status: "error",
    });
  });

  it("projects Hook start and completion into one replay-stable safe trace", () => {
    let state = createInitialRunNarrativeState();
    state = projectAguiEventToNarrative(state, {
      type: "AWAKEN_HOOK_STARTED", runId: "run-hook", hookInvocationId: "hookinv-1",
      hookId: "project:lint", hookEvent: "PreToolUse", hookSourceKind: "project",
    });
    state = projectAguiEventToNarrative(state, {
      type: "AWAKEN_HOOK_COMPLETED", runId: "run-hook", hookInvocationId: "hookinv-1",
      hookId: "project:lint", hookEvent: "PreToolUse", hookSourceKind: "project",
      hookAction: "block_tool", hookReasonCode: "policy", hookReason: "unsafe target",
      hookOutputPreview: "bounded diagnostic", status: "blocked", durationMs: 9,
    });
    const hooks = state.committedItems.filter((item) => item.kind === "hook");
    expect(hooks).toHaveLength(1);
    expect(hooks[0]).toMatchObject({
      id: "hook:run-hook:hookinv-1", title: "Hook · project:lint",
      detail: "PreToolUse · project · 9 ms", output: "policy\nunsafe target\nbounded diagnostic",
      status: "blocked",
    });
  });

  it("keeps BeforeFinish blocks non-final and exposes the retry transition", () => {
	let state = createInitialRunNarrativeState();
	state = projectAguiEventToNarrative(state, { type: "TEXT_MESSAGE_START", runId: "run-finish", messageId: "candidate-1" });
	state = projectAguiEventToNarrative(state, { type: "TEXT_MESSAGE_CONTENT", runId: "run-finish", messageId: "candidate-1", delta: "draft" });
	state = projectAguiEventToNarrative(state, { type: "TEXT_MESSAGE_END", runId: "run-finish", messageId: "candidate-1" });
	state = projectAguiEventToNarrative(state, {
		type: "AWAKEN_HOOK_COMPLETED", runId: "run-finish", hookInvocationId: "finish-hook-1",
		hookId: "user:review", hookEvent: "BeforeFinish", hookAction: "block_finish", status: "blocked",
	});
	state = projectAguiEventToNarrative(state, {
		type: "AWAKEN_TURN_STARTED", runId: "run-finish", turnId: "turn-2", transitionReason: "before_finish_hook_retry",
	});
	expect(state.committedItems.find((item) => item.kind === "answer")).toMatchObject({ markdown: "draft", status: "done" });
	expect(state.committedItems.find((item) => item.kind === "answer")).not.toHaveProperty("isFinal", true);
	expect(state.committedItems.find((item) => item.kind === "hook")).toMatchObject({ status: "blocked" });
	expect(state.activeItem).toMatchObject({ kind: "planning", text: "正在根据 Hook 反馈完善回答" });
  });

  it("does not project recovery supervision as extra tool calls", () => {
    let state = createInitialRunNarrativeState();
    state = projectAguiEventToNarrative(state, {
      type: "TOOL_CALL_START",
      toolCallId: "call-1",
      toolCallName: "apply_patch",
    });
    state = projectAguiEventToNarrative(state, {
      type: "TOOL_CALL_RESULT",
      toolCallId: "call-1",
      toolCallName: "apply_patch",
      toolStatus: "failed",
      content: "patch context mismatch",
    });
    state = projectAguiEventToNarrative(state, {
      type: "AWAKEN_TOOL_RECOVERY_REQUIRED",
      toolCallId: "call-1",
      toolCallName: "apply_patch",
      errorCode: "patch_context_mismatch",
      message: "recovery_required",
      recoveryOptions: ["read_file", "use_text_edit"],
    });
    state = projectAguiEventToNarrative(state, {
      type: "AWAKEN_TOOL_RETRY_BLOCKED",
      toolCallId: "call-2",
      toolCallName: "apply_patch",
      errorCode: "tool_retry_blocked",
      message: "blocked repeated tool call",
      recoveryOptions: ["change_strategy"],
    });

    const notices = state.committedItems.filter((item) => item.kind === "tool");
    expect(notices).toHaveLength(1);
    expect(notices[0]).toMatchObject({
      kind: "tool",
      toolCallId: "call-1",
      title: "apply_patch",
      status: "error",
    });
  });

  it("keeps recovery-only events out of the narrative", () => {
    let state = createInitialRunNarrativeState();
    state = projectAguiEventToNarrative(state, {
      type: "AWAKEN_TOOL_RECOVERY_REQUIRED",
      toolCallId: "call-3",
      toolCallName: "read_artifact",
      errorCode: "read_artifact_specialized_tool_required",
      message:
        "strategy_change_required: 6 tool failures accumulated (turn=6 run=6); do not repeat the failing approach",
      recoveryOptions: [
        "use_specialized_artifact_tool",
        "change_tool_or_library",
      ],
    });

    expect(state.committedItems).toEqual([]);
  });

  it("projects bounded Memory and Dream receipts without topic bodies", () => {
    let state = createInitialRunNarrativeState();
    state = projectAguiEventToNarrative(state, {
      type: "AWAKEN_MEMORY_RECALLED",
      recordId: "memory-recall-1",
      memories: [
        { memoryId: "mem_1", scope: "user", memoryType: "user", revision: 2 },
      ],
    });
    state = projectAguiEventToNarrative(state, {
      type: "AWAKEN_MEMORY_DREAM_COMPLETED",
      recordId: "dream_1",
      diagnostic: {
        reports: { user: { updated: 1, superseded: 2, conflicts: 0 } },
      },
    });

    const receipts = state.committedItems.filter(
      (item) => item.kind === "memoryReceipt",
    );
    expect(receipts).toHaveLength(2);
    expect(receipts[0]).toMatchObject({
      title: "已召回 1 条记忆",
      detail: "用户 · user · r2",
      status: "done",
    });
    expect(receipts[1]).toMatchObject({
      title: "记忆整理完成",
      detail: "更新 1 条 · 归档 2 条 · 冲突 0 条",
      status: "done",
    });
    expect(JSON.stringify(receipts)).not.toContain("topic body");
  });

  it("repairs legacy assistant snapshots with a leading think block", () => {
    let state = createInitialRunNarrativeState();
    state = projectAguiEventToNarrative(state, {
      type: "TEXT_MESSAGE_START",
      runId: "run-minimax",
      messageId: "message-minimax",
    });
    state = projectAguiEventToNarrative(state, {
      type: "TEXT_MESSAGE_CONTENT",
      runId: "run-minimax",
      messageId: "message-minimax",
      recordId: "assistant-message-old",
      delta: "<think>private host reasoning</think>\n\n最终回答",
    });

    expect(state.committedItems).toContainEqual({
      kind: "thinking",
      id: "thinking:run-minimax:message-minimax",
      text: "private host reasoning",
      status: "done",
    });
    expect(state.committedItems).toContainEqual({
      kind: "answer",
      id: "answer:run-minimax:message-minimax",
      markdown: "最终回答",
      status: "done",
    });
    expect(state.activeItem).toBeUndefined();
  });

  it("replaces a generic Memory mutation row with its typed receipt", () => {
    let state = createInitialRunNarrativeState();
    state = projectAguiEventToNarrative(state, {
      type: "TOOL_CALL_START",
      runId: "run-memory",
      toolCallId: "call-memory",
      toolCallName: "memory",
    });
    state = projectAguiEventToNarrative(state, {
      type: "TOOL_CALL_RESULT",
      runId: "run-memory",
      toolCallId: "call-memory",
      toolCallName: "memory",
      toolStatus: "failed",
      content: "memory revision conflict",
    });
    state = projectAguiEventToNarrative(state, {
      type: "AWAKEN_MEMORY_REJECTED",
      runId: "run-memory",
      toolCallId: "call-memory",
      recordId: "memory-rejected-1",
      errorCode: "memory_revision_conflict",
      memories: [
        {
          memoryId: "mem_1",
          scope: "user",
          memoryType: "feedback",
          revision: 1,
        },
      ],
    });

    expect(state.committedItems.filter((item) => item.kind === "tool")).toEqual(
      [],
    );
    expect(
      state.committedItems.filter((item) => item.kind === "memoryReceipt"),
    ).toHaveLength(1);
  });

  it("deduplicates Dream receipts by task identity instead of terminal status", () => {
    let state = createInitialRunNarrativeState();
    for (const recordId of ["dream_1", "dream_2", "dream_2"]) {
      state = projectAguiEventToNarrative(state, {
        type: "AWAKEN_MEMORY_DREAM_FAILED",
        recordId,
        errorCode: "DREAM_PLAN_FAILED",
      });
    }

    const receipts = state.committedItems.filter(
      (item) => item.kind === "memoryReceipt",
    );
    expect(receipts).toHaveLength(2);
    expect(receipts.map((item) => item.id)).toEqual([
      "dream:dream_1",
      "dream:dream_2",
    ]);
  });

  it("projects ACP session and turn lifecycle into committed items", () => {
    let state = createInitialRunNarrativeState();
    state = projectAguiEventToNarrative(state, {
      type: "AWAKEN_ACP_SESSION_CHANGED",
      runId: "run-1",
      toolCallId: "call-acp-1",
      status: "started",
      acpRunnerId: "claude-code",
      acpSessionId: "sess-1",
      message: "Session started",
    });
    state = projectAguiEventToNarrative(state, {
      type: "AWAKEN_ACP_TURN_CHANGED",
      runId: "run-1",
      toolCallId: "call-acp-1",
      status: "started",
      acpRunnerId: "claude-code",
      acpSessionId: "sess-1",
      message: "Turn started",
    });
    state = projectAguiEventToNarrative(state, {
      type: "AWAKEN_ACP_TURN_CHANGED",
      runId: "run-1",
      toolCallId: "call-acp-1",
      status: "finished",
      acpRunnerId: "claude-code",
      acpSessionId: "sess-1",
      message: "Turn finished",
    });
    state = projectAguiEventToNarrative(state, {
      type: "AWAKEN_ACP_SESSION_CHANGED",
      runId: "run-1",
      toolCallId: "call-acp-1",
      status: "closed",
      acpRunnerId: "claude-code",
      acpSessionId: "sess-1",
      message: "Session closed",
    });

    const sessions = state.committedItems.filter(
      (item) => item.kind === "acpSession",
    );
    expect(sessions).toHaveLength(1);
    expect(sessions[0]).toMatchObject({
      id: "acp-session:run-1:claude-code:sess-1",
      runnerId: "claude-code",
      sessionId: "sess-1",
      status: "closed",
      message: "Session closed",
    });
  });

  it("projects ACP tool snapshots into committed items", () => {
    let state = createInitialRunNarrativeState();
    state = projectAguiEventToNarrative(state, {
      type: "AWAKEN_ACP_TOOL_CHANGED",
      runId: "run-1",
      toolCallId: "call-acp-1",
      acpRunnerId: "claude-code",
      acpSessionId: "sess-1",
      acpToolCallId: "tool-1",
      toolCallName: "Read",
      message: "Reading file",
    });
    state = projectAguiEventToNarrative(state, {
      type: "AWAKEN_ACP_TOOL_CHANGED",
      runId: "run-1",
      toolCallId: "call-acp-1",
      acpRunnerId: "claude-code",
      acpSessionId: "sess-1",
      acpToolCallId: "tool-1",
      toolCallName: "Read",
      content: "Read",
      message: "Reading file updated",
    });

    const tools = state.committedItems.filter(
      (item) => item.kind === "acpTool",
    );
    expect(tools).toHaveLength(1);
    expect(tools[0]).toMatchObject({
      id: "acp-tool:run-1:claude-code:sess-1:tool-1",
      title: "Read",
      detail: "Reading file updated",
      status: "active",
    });
  });

  it("ignores ACP permission request and resolution events", () => {
    let state = createInitialRunNarrativeState();
    state = projectAguiEventToNarrative(state, {
      type: "AWAKEN_ACP_PERMISSION_REQUESTED",
      runId: "run-1",
      interactionId: "perm-1",
      acpRunnerId: "claude-code",
      acpPermissionOptions: [{ optionId: "allow", name: "Allow" }],
    });
    state = projectAguiEventToNarrative(state, {
      type: "AWAKEN_ACP_PERMISSION_RESOLVED",
      runId: "run-1",
      interactionId: "perm-1",
      status: "allow",
    });

    expect(state.committedItems).toHaveLength(0);
  });

  it("projects AWAKEN_SCHEDULED_NOTICE as a scheduled notice card", () => {
    let state = createInitialRunNarrativeState();
    state = projectAguiEventToNarrative(state, {
      type: "AWAKEN_SCHEDULED_NOTICE",
      runId: "run-notice-1",
      threadId: "thread-1",
      recordId: "record-1",
      seq: 5,
      taskId: "sched-1",
      executionId: "exec-1",
      taskName: "休息提醒",
      noticeContent: "起来活动一下，喝口水～",
      timestamp: 1700000000000,
    });

    expect(state.committedItems).toHaveLength(1);
    expect(state.committedItems[0]).toMatchObject({
      kind: "scheduledNotice",
      taskId: "sched-1",
      executionId: "exec-1",
      taskName: "休息提醒",
      noticeContent: "起来活动一下，喝口水～",
    });
  });

  it("upserts native subagent lifecycle and activity without replacing active thinking", () => {
    let state = createInitialRunNarrativeState();
    state = projectAguiEventToNarrative(state, {
      type: "REASONING_MESSAGE_START",
      runId: "run-1",
      messageId: "reasoning-1",
    });
    state = projectAguiEventToNarrative(state, {
      type: "AWAKEN_SUBAGENT_CHANGED",
      runId: "run-1",
      subagentTaskId: "task-1",
      subagentDescription: "检查认证边界",
      subagentAgentType: "review",
      subagentStatus: "running",
    });
    state = projectAguiEventToNarrative(state, {
      type: "AWAKEN_SUBAGENT_ACTIVITY",
      runId: "run-1",
      subagentTaskId: "task-1",
      toolCallName: "Read",
      subagentActivity: "读取 auth/service.go",
      subagentToolUseCount: 1,
      subagentTotalTokens: 450,
      subagentStartedAt: "2026-08-02T07:00:00Z",
    });
    state = projectAguiEventToNarrative(state, {
      type: "AWAKEN_SUBAGENT_CHANGED",
      runId: "run-1",
      subagentTaskId: "task-2",
      subagentDescription: "检查测试入口",
      subagentAgentType: "explore",
      subagentStatus: "running",
    });
    state = projectAguiEventToNarrative(state, {
      type: "AWAKEN_SUBAGENT_CHANGED",
      runId: "run-1",
      subagentTaskId: "task-1",
      subagentDescription: "检查认证边界",
      subagentAgentType: "review",
      subagentStatus: "completed",
      subagentResultPreview: "未发现越权路径",
      subagentToolUseCount: 2,
      subagentInputTokens: 600,
      subagentOutputTokens: 200,
      subagentTotalTokens: 800,
      subagentFinishedAt: "2026-08-02T07:00:05Z",
    });

    expect(state.activeItem).toMatchObject({ kind: "thinking" });
    expect(state.committedItems).toContainEqual({
      kind: "subagentGroup",
      id: "subagents:run-1",
      tasks: [
        {
          taskId: "task-1",
          description: "检查认证边界",
          agentType: "review",
          workspaceMode: undefined,
          status: "completed",
          activity: undefined,
          toolUseCount: 2,
          estimatedTokens: undefined,
          inputTokens: 600,
          outputTokens: 200,
          totalTokens: 800,
          createdAt: undefined,
          startedAt: "2026-08-02T07:00:00Z",
          finishedAt: "2026-08-02T07:00:05Z",
          resultPreview: "未发现越权路径",
          errorCode: undefined,
          errorMessage: undefined,
        },
        {
          taskId: "task-2",
          description: "检查测试入口",
          agentType: "explore",
          workspaceMode: undefined,
          status: "running",
          activity: undefined,
          toolUseCount: 0,
          estimatedTokens: undefined,
          inputTokens: undefined,
          outputTokens: undefined,
          totalTokens: undefined,
          createdAt: undefined,
          startedAt: undefined,
          finishedAt: undefined,
          resultPreview: undefined,
          errorCode: undefined,
          errorMessage: undefined,
        },
      ],
    });
  });

  it("projects plan/general workspace modes onto the narrative group", () => {
    let state = createInitialRunNarrativeState();
    state = projectAguiEventToNarrative(state, {
      type: "AWAKEN_SUBAGENT_CHANGED",
      runId: "run-1",
      subagentTaskId: "task-plan",
      subagentDescription: "规划改动",
      subagentAgentType: "plan",
      subagentWorkspaceMode: "shared_readonly",
      subagentStatus: "running",
    });
    state = projectAguiEventToNarrative(state, {
      type: "AWAKEN_SUBAGENT_CHANGED",
      runId: "run-1",
      subagentTaskId: "task-write",
      subagentDescription: "实现改动",
      subagentAgentType: "general",
      subagentWorkspaceMode: "git_worktree",
      subagentStatus: "queued",
    });
    const group = state.committedItems.find((item) => item.kind === "subagentGroup");
    expect(group).toMatchObject({
      kind: "subagentGroup",
      tasks: [
        { taskId: "task-plan", agentType: "plan", workspaceMode: "shared_readonly" },
        { taskId: "task-write", agentType: "general", workspaceMode: "git_worktree" },
      ],
    });
  });

  it("keeps one media generation item from provider progress through the ready result", () => {
    let state = createInitialRunNarrativeState();
    const base = {
      runId: "run-media",
      toolCallId: "call-media",
      toolCallName: "video_generate",
    };
    state = projectAguiEventToNarrative(state, {
      type: "TOOL_CALL_START",
      ...base,
    });
    expect(state.activeItem).toMatchObject({
      kind: "mediaGeneration",
      id: "media:run-media:call-media",
      status: "preparing",
      mediaKind: "video",
    });

    state = projectAguiEventToNarrative(state, {
      type: "AWAKEN_COMMAND_OUTPUT",
      ...base,
      commandStream: "media_status",
      delta: '{"stage":"queued","message":"视频任务正在排队"}\n',
    });
    expect(state.activeItem).toMatchObject({
      kind: "mediaGeneration",
      status: "queued",
      detail: "视频任务正在排队",
    });

    state = projectAguiEventToNarrative(state, {
      type: "TOOL_CALL_RESULT",
      ...base,
      toolStatus: "succeeded",
      content: JSON.stringify({
        status: "succeeded",
        kind: "video",
        paths: ["output/generated-media/video.mp4"],
      }),
    });
    expect(state.activeItem).toBeUndefined();
    expect(state.committedItems).toContainEqual({
      kind: "mediaGeneration",
      id: "media:run-media:call-media",
      toolCallId: "call-media",
      runId: "run-media",
      mediaKind: "video",
      title: "视频生成",
      detail: "视频已生成",
      status: "ready",
      artifactPaths: ["output/generated-media/video.mp4"],
    });
  });

  it("preserves per-item partial group results for independent cards", () => {
    let state = createInitialRunNarrativeState();
    const base = {
      runId: "run-partial-group",
      toolCallId: "call-partial-group",
      toolCallName: "image_generate",
    };
    state = projectAguiEventToNarrative(state, {
      type: "TOOL_CALL_START",
      ...base,
    });
    state = projectAguiEventToNarrative(state, {
      type: "TOOL_CALL_ARGS",
      ...base,
      delta: JSON.stringify({ prompt: "三张", mode: "group", max_images: 3 }),
    });
    state = projectAguiEventToNarrative(state, {
      type: "TOOL_CALL_RESULT",
      ...base,
      toolStatus: "failed",
      transportStatus: "completed",
      content: JSON.stringify({
        generation_status: "partial",
        requested_count: 3,
        paths: ["output/1.png", "output/3.png"],
        items: [
          { index: 1, status: "ready", path: "output/1.png" },
          { index: 2, status: "error" },
          { index: 3, status: "ready", path: "output/3.png" },
        ],
      }),
    });
    expect(state.committedItems[0]).toMatchObject({
      kind: "mediaGeneration",
      status: "partial",
      requestedCount: 3,
      artifactPaths: ["output/1.png", "output/3.png"],
      mediaItems: [
        { index: 1, status: "ready", path: "output/1.png" },
        { index: 2, status: "error" },
        { index: 3, status: "ready", path: "output/3.png" },
      ],
    });
  });

  it("keeps a submitted video task as unknown with its resumable task id", () => {
    let state = createInitialRunNarrativeState();
    const base = {
      runId: "run-video-unknown",
      toolCallId: "call-video-unknown",
      toolCallName: "video_generate",
    };
    state = projectAguiEventToNarrative(state, {
      type: "TOOL_CALL_START",
      ...base,
    });
    state = projectAguiEventToNarrative(state, {
      type: "AWAKEN_COMMAND_OUTPUT",
      ...base,
      commandStream: "media_status",
      delta:
        '{"stage":"queued","message":"视频任务已提交","task_id":"cgt-1"}\n',
    });
    expect(state.activeItem).toMatchObject({
      taskId: "cgt-1",
      status: "queued",
    });
    state = projectAguiEventToNarrative(state, {
      type: "TOOL_CALL_RESULT",
      ...base,
      toolStatus: "failed",
      transportStatus: "unknown",
      content: JSON.stringify({
        generation_status: "unknown",
        task_id: "cgt-1",
        requested_count: 1,
        paths: [],
        items: [{ index: 1, status: "unknown" }],
      }),
    });
    expect(state.committedItems[0]).toMatchObject({
      kind: "mediaGeneration",
      status: "unknown",
      taskId: "cgt-1",
      mediaItems: [{ index: 1, status: "unknown" }],
    });
  });

  it("projects an image POST timeout as unknown instead of fifteen failed cards", () => {
    let state = createInitialRunNarrativeState();
    const base = {
      runId: "run-image-unknown",
      toolCallId: "call-image-unknown",
      toolCallName: "image_generate",
    };
    state = projectAguiEventToNarrative(state, {
      type: "TOOL_CALL_START",
      ...base,
    });
    state = projectAguiEventToNarrative(state, {
      type: "TOOL_CALL_ARGS",
      ...base,
      delta: JSON.stringify({
        prompt: "十五张",
        mode: "group",
        max_images: 15,
      }),
    });
    state = projectAguiEventToNarrative(state, {
      type: "TOOL_CALL_RESULT",
      ...base,
      toolStatus: "failed",
      transportStatus: "unknown",
      content: "图片请求结果未知",
    });
    expect(state.committedItems[0]).toMatchObject({
      kind: "mediaGeneration",
      status: "unknown",
      requestedCount: 15,
      artifactPaths: [],
    });
  });

  it("keeps the requested group-image limit and the actual ready artifact count", () => {
    let state = createInitialRunNarrativeState();
    const base = {
      runId: "run-image-group",
      toolCallId: "call-image-group",
      toolCallName: "image_generate",
    };
    state = projectAguiEventToNarrative(state, {
      type: "TOOL_CALL_START",
      ...base,
    });
    state = projectAguiEventToNarrative(state, {
      type: "TOOL_CALL_ARGS",
      ...base,
      delta: JSON.stringify({
        prompt: "一组连贯产品图",
        mode: "group",
        max_images: 12,
        size: "2048x1024",
      }),
    });
    expect(state.activeItem).toMatchObject({
      kind: "mediaGeneration",
      requestedCount: 12,
      requestedAspectRatio: 2,
      detail: "一组连贯产品图",
    });

    state = projectAguiEventToNarrative(state, {
      type: "TOOL_CALL_RESULT",
      ...base,
      toolStatus: "succeeded",
      content: JSON.stringify({
        status: "succeeded",
        kind: "image",
        paths: ["output/1.png", "output/2.png", "output/3.png"],
      }),
    });
    expect(state.committedItems[0]).toMatchObject({
      kind: "mediaGeneration",
      status: "ready",
      requestedCount: 12,
      requestedAspectRatio: 2,
      detail: "已生成 3 张图片",
      artifactPaths: ["output/1.png", "output/2.png", "output/3.png"],
    });
  });

  it("projects a requested video ratio for its independent loading card", () => {
    let state = createInitialRunNarrativeState();
    const base = {
      runId: "run-video-ratio",
      toolCallId: "call-video-ratio",
      toolCallName: "video_generate",
    };
    state = projectAguiEventToNarrative(state, {
      type: "TOOL_CALL_START",
      ...base,
    });
    state = projectAguiEventToNarrative(state, {
      type: "TOOL_CALL_ARGS",
      ...base,
      delta: JSON.stringify({ prompt: "竖版视频", ratio: "9:16" }),
    });

    expect(state.activeItem).toMatchObject({
      kind: "mediaGeneration",
      requestedAspectRatio: 9 / 16,
    });
  });

  it("recovers historical media files whose old output schema marked the tool result failed", () => {
    let state = createInitialRunNarrativeState();
    state = projectAguiEventToNarrative(state, {
      type: "TOOL_CALL_RESULT",
      runId: "run-old",
      toolCallId: "call-old",
      toolCallName: "image_generate",
      toolStatus: "failed",
      content: JSON.stringify({
        status: "succeeded",
        kind: "image",
        paths: ["output/generated-media/legacy.png"],
      }),
    });
    expect(state.committedItems[0]).toMatchObject({
      kind: "mediaGeneration",
      status: "ready",
      artifactPaths: ["output/generated-media/legacy.png"],
    });
  });

  it("settles an active media generation card when the run is cancelled", () => {
    let state = createInitialRunNarrativeState();
    state = projectAguiEventToNarrative(state, {
      type: "TOOL_CALL_START",
      runId: "run-media-cancelled",
      toolCallId: "call-media-cancelled",
      toolCallName: "video_generate",
    });

    state = projectAguiEventToNarrative(state, runError("RUN_CANCELLED"));

    expect(state.activeItem).toBeUndefined();
    expect(state.committedItems[0]).toMatchObject({
      kind: "mediaGeneration",
      status: "cancelled",
      detail: "生成已停止",
    });
  });
});

describe("run plan terminal projection (RP-D03)", () => {
  const planItemsV1 = [
    { step: "实现运行计划投影", activeForm: "正在实现运行计划投影", status: "in_progress" },
    { step: "补齐自动化验收", activeForm: "正在补齐自动化验收", status: "pending" },
  ];
  const planItemsV2 = [
    { step: "实现运行计划投影", activeForm: "正在实现运行计划投影", status: "completed" },
    { step: "补齐自动化验收", activeForm: "正在补齐自动化验收", status: "completed" },
  ];

  function planUpdated(overrides: Partial<AguiEvent> = {}): AguiEvent {
    return {
      type: "AWAKEN_PLAN_UPDATED",
      runId: "run-1",
      planId: "plan-1",
      planVersion: 1,
      planItems: planItemsV1,
      planCompletedCount: 0,
      planTotalCount: 2,
      planCurrentStep: 1,
      planAllCompleted: false,
      toolCallId: "call-plan-1",
      semanticSeq: 10,
      ...overrides,
    };
  }

  function updatePlanStart(toolCallId = "call-plan-1", seq = 8): AguiEvent {
    return {
      type: "TOOL_CALL_START",
      runId: "run-1",
      toolCallId,
      toolCallName: "update_plan",
      semanticSeq: seq,
    };
  }

  function updatePlanResult(
    toolStatus: string,
    toolCallId = "call-plan-1",
    seq = 11,
  ): AguiEvent {
    return {
      type: "TOOL_CALL_RESULT",
      runId: "run-1",
      toolCallId,
      toolCallName: "update_plan",
      toolStatus,
      content: toolStatus === "failed" ? "plan_version_conflict" : "{}",
      semanticSeq: seq,
    };
  }

  function answerSequence(state: ReturnType<typeof createInitialRunNarrativeState>) {
    let next = projectAguiEventToNarrative(state, {
      ...textMessageStart(),
      semanticSeq: 20,
    });
    next = projectAguiEventToNarrative(next, {
      ...textMessageContent("done"),
      semanticSeq: 21,
    });
    next = projectAguiEventToNarrative(next, {
      ...textMessageEnd(),
      semanticSeq: 22,
    });
    return next;
  }

  function runPlanItems(state: { committedItems: { kind: string }[] }) {
    return state.committedItems.filter((item) => item.kind === "runPlan");
  }

  it("projects the terminal snapshot before the final answer with real step states", () => {
    let state = createInitialRunNarrativeState();
    state = projectAguiEventToNarrative(state, updatePlanStart());
    state = projectAguiEventToNarrative(state, planUpdated());
    state = projectAguiEventToNarrative(state, updatePlanResult("succeeded"));
    state = answerSequence(state);
    state = projectAguiEventToNarrative(state, {
      type: "RUN_FINISHED",
      runId: "run-1",
      semanticSeq: 30,
    });

    const plans = runPlanItems(state);
    expect(plans).toHaveLength(1);
    expect(plans[0]).toMatchObject({
      kind: "runPlan",
      id: "run-plan:run-1:plan-1",
      runId: "run-1",
      planId: "plan-1",
      version: 1,
      title: "执行计划 · 0/2",
      completedCount: 0,
      totalCount: 2,
      items: planItemsV1,
    });
    const kinds = state.committedItems.map((item) => item.kind);
    expect(kinds.indexOf("runPlan")).toBeLessThan(kinds.lastIndexOf("answer"));
    expect(state.committedItems.at(-1)).toMatchObject({
      kind: "answer",
      isFinal: true,
    });
  });

  it("keeps a stable id so duplicate terminal frames and replay do not duplicate the item", () => {
    const script: AguiEvent[] = [
      updatePlanStart(),
      planUpdated(),
      updatePlanResult("succeeded"),
      { ...textMessageStart(), semanticSeq: 20 },
      { ...textMessageContent("done"), semanticSeq: 21 },
      { ...textMessageEnd(), semanticSeq: 22 },
      { type: "RUN_FINISHED", runId: "run-1", semanticSeq: 30 },
    ];
    let state = createInitialRunNarrativeState();
    for (const event of script) {
      state = projectAguiEventToNarrative(state, event);
    }
    // Replay overlap: the same frames arrive again (same seq, deduped) and a
    // second terminal frame with a fresh seq must upsert onto the stable id.
    for (const event of script) {
      state = projectAguiEventToNarrative(state, event);
    }
    state = projectAguiEventToNarrative(state, {
      type: "RUN_FINISHED",
      runId: "run-1",
      semanticSeq: 31,
    });

    expect(runPlanItems(state)).toHaveLength(1);
    expect(
      state.committedItems.filter((item) => item.id === "run-plan:run-1:plan-1"),
    ).toHaveLength(1);
  });

  it("removes the generic tool row of a successful update_plan", () => {
    let state = createInitialRunNarrativeState();
    state = projectAguiEventToNarrative(state, updatePlanStart());
    state = projectAguiEventToNarrative(state, updatePlanResult("succeeded"));
    expect(
      state.committedItems.some((item) => item.id === "tool:run-1:call-plan-1"),
    ).toBe(true);

    state = projectAguiEventToNarrative(state, planUpdated());

    expect(
      state.committedItems.some((item) => item.id === "tool:run-1:call-plan-1"),
    ).toBe(false);
    expect(state.activeItem).toBeUndefined();
    expect(state.inspectorSelection).toBeUndefined();
  });

  it("never creates a generic tool row when the receipt precedes the result frame", () => {
    let state = createInitialRunNarrativeState();
    state = projectAguiEventToNarrative(state, updatePlanStart());
    // Journal order puts plan_updated before the canonical tool_result.
    state = projectAguiEventToNarrative(state, planUpdated());
    state = projectAguiEventToNarrative(state, updatePlanResult("succeeded"));

    expect(
      state.committedItems.some(
        (item) => item.kind === "tool" && item.id === "tool:run-1:call-plan-1",
      ),
    ).toBe(false);
    expect(state.activeItem).toBeUndefined();
  });

  it("keeps the error tool row of a failed update_plan without a plan receipt", () => {
    let state = createInitialRunNarrativeState();
    state = projectAguiEventToNarrative(state, updatePlanStart());
    state = projectAguiEventToNarrative(state, updatePlanResult("failed"));
    state = projectAguiEventToNarrative(state, {
      type: "RUN_FINISHED",
      runId: "run-1",
      semanticSeq: 30,
    });

    expect(
      state.committedItems.some(
        (item) =>
          item.id === "tool:run-1:call-plan-1" &&
          item.kind === "tool" &&
          item.status === "error",
      ),
    ).toBe(true);
    expect(runPlanItems(state)).toHaveLength(0);
  });

  it("keeps unfinished steps pending/in_progress when the run is cancelled", () => {
    let state = createInitialRunNarrativeState();
    state = projectAguiEventToNarrative(
      state,
      planUpdated({
        planItems: [
          { step: "第一步", activeForm: "正在做第一步", status: "completed" },
          { step: "第二步", activeForm: "正在做第二步", status: "in_progress" },
          { step: "第三步", activeForm: "正在做第三步", status: "pending" },
        ],
        planCompletedCount: 1,
        planTotalCount: 3,
      }),
    );
    state = projectAguiEventToNarrative(
      state,
      runError("RUN_CANCELLED", "cancelled"),
    );

    const plans = runPlanItems(state);
    expect(plans).toHaveLength(1);
    expect(plans[0]).toMatchObject({
      title: "执行计划 · 1/3",
      items: [
        { step: "第一步", status: "completed" },
        { step: "第二步", status: "in_progress" },
        { step: "第三步", status: "pending" },
      ],
    });
  });

  it("keeps only the latest snapshot across multiple versions", () => {
    let state = createInitialRunNarrativeState();
    state = projectAguiEventToNarrative(state, planUpdated());
    state = projectAguiEventToNarrative(
      state,
      planUpdated({
        planVersion: 2,
        planItems: planItemsV2,
        planCompletedCount: 2,
        planAllCompleted: true,
        toolCallId: "call-plan-2",
        semanticSeq: 12,
        planExplanation: "提前完成",
      }),
    );
    // A stale lower-version frame must not roll the snapshot back.
    state = projectAguiEventToNarrative(
      state,
      planUpdated({ semanticSeq: 13 }),
    );
    state = projectAguiEventToNarrative(state, {
      type: "RUN_FINISHED",
      runId: "run-1",
      semanticSeq: 30,
    });

    const plans = runPlanItems(state);
    expect(plans).toHaveLength(1);
    expect(plans[0]).toMatchObject({
      version: 2,
      title: "执行计划 · 2/2",
      explanation: "提前完成",
      items: planItemsV2,
    });
  });

  it("discards in-progress thinking on provider reconnect reset and restores planning", () => {
    let state = projectAguiEventToNarrative(createInitialRunNarrativeState(), {
      type: "REASONING_START",
      runId: "run-1",
      messageId: "think-live",
    });
    state = projectAguiEventToNarrative(state, {
      type: "REASONING_MESSAGE_CONTENT",
      runId: "run-1",
      messageId: "think-live",
      delta: "partial thought that never finished",
    });
    expect(state.activeItem).toMatchObject({
      kind: "thinking",
      text: "partial thought that never finished",
    });

    state = projectAguiEventToNarrative(state, {
      type: "AWAKEN_PROVIDER_RECONNECT",
      runId: "run-1",
      providerReconnect: {
        phase: "reconnecting",
        attempt: 1,
        maxAttempts: 5,
        detail: "stream eof",
        resetInProgress: true,
      },
    });
    expect(state.activeItem).toMatchObject({
      kind: "planning",
      id: "planning:run-1",
      text: "Planning next moves",
      status: "active",
    });
    expect(
      state.committedItems.some((item) => item.kind === "thinking"),
    ).toBe(false);
    expect(state.committedItems.at(-1)).toMatchObject({
      kind: "providerReconnect",
      phase: "reconnecting",
      attempt: 1,
      maxAttempts: 5,
      detail: "stream eof",
    });
  });

  it("keeps thinking when provider reconnect does not reset in-progress output", () => {
    let state = projectAguiEventToNarrative(createInitialRunNarrativeState(), {
      type: "REASONING_START",
      runId: "run-1",
      messageId: "think-keep",
    });
    state = projectAguiEventToNarrative(state, {
      type: "REASONING_MESSAGE_CONTENT",
      runId: "run-1",
      messageId: "think-keep",
      delta: "still thinking",
    });
    state = projectAguiEventToNarrative(state, {
      type: "AWAKEN_PROVIDER_RECONNECT",
      runId: "run-1",
      providerReconnect: {
        phase: "waiting_for_network",
        attempt: 1,
        maxAttempts: 0,
        detail: "tls handshake eof",
      },
    });
    expect(state.activeItem).toMatchObject({
      kind: "thinking",
      text: "still thinking",
    });
    expect(state.committedItems.at(-1)).toMatchObject({
      kind: "providerReconnect",
      phase: "waiting_for_network",
      detail: "tls handshake eof",
    });
  });

  it("appends stacked provider reconnect rows after already committed work", () => {
    let state = projectAguiEventToNarrative(createInitialRunNarrativeState(), {
      type: "TOOL_CALL_START",
      runId: "run-1",
      toolCallId: "tool-1",
      toolCallName: "Bash",
    });
    state = projectAguiEventToNarrative(state, {
      type: "TOOL_CALL_RESULT",
      runId: "run-1",
      toolCallId: "tool-1",
      toolCallName: "Bash",
      toolStatus: "succeeded",
    });
    state = projectAguiEventToNarrative(state, {
      type: "AWAKEN_TURN_STARTED",
      runId: "run-1",
      turnId: "turn-2",
      transitionReason: "tool_result_continuation",
    });
    state = projectAguiEventToNarrative(state, {
      type: "AWAKEN_PROVIDER_RECONNECT",
      runId: "run-1",
      providerReconnect: {
        phase: "reconnecting",
        attempt: 5,
        maxAttempts: 5,
        detail: "tls handshake eof",
      },
    });
    state = projectAguiEventToNarrative(state, {
      type: "AWAKEN_PROVIDER_RECONNECT",
      runId: "run-1",
      providerReconnect: {
        phase: "waiting_for_network",
        attempt: 2,
        maxAttempts: 0,
        detail: "error sending request",
      },
    });

    const reconnects = state.committedItems.filter(
      (item) => item.kind === "providerReconnect",
    );
    expect(reconnects).toEqual([
      expect.objectContaining({
        kind: "providerReconnect",
        phase: "reconnecting",
        attempt: 5,
        maxAttempts: 5,
        detail: "tls handshake eof",
      }),
      expect.objectContaining({
        kind: "providerReconnect",
        phase: "waiting_for_network",
        attempt: 2,
        maxAttempts: 0,
        detail: "error sending request",
      }),
    ]);
    expect(
      state.committedItems.findIndex((item) => item.kind === "tool"),
    ).toBeLessThan(
      state.committedItems.findIndex((item) => item.kind === "providerReconnect"),
    );
    expect(state.activeItem).toMatchObject({
      kind: "planning",
      phase: "continuation",
    });
  });

  it("clears live provider reconnect rows on recovered and run terminal", () => {
    let state = projectAguiEventToNarrative(createInitialRunNarrativeState(), {
      type: "AWAKEN_PROVIDER_RECONNECT",
      runId: "run-1",
      providerReconnect: {
        phase: "waiting_for_network",
        attempt: 1,
        maxAttempts: 0,
        detail: "tls handshake eof",
      },
    });
    expect(
      state.committedItems.some((item) => item.kind === "providerReconnect"),
    ).toBe(true);

    const recovered = projectAguiEventToNarrative(state, {
      type: "AWAKEN_PROVIDER_RECONNECT",
      runId: "run-1",
      providerReconnect: { phase: "recovered" },
    });
    expect(
      recovered.committedItems.some((item) => item.kind === "providerReconnect"),
    ).toBe(false);

    const finished = projectAguiEventToNarrative(state, {
      type: "RUN_FINISHED",
      runId: "run-1",
    });
    expect(
      finished.committedItems.some((item) => item.kind === "providerReconnect"),
    ).toBe(false);
  });
});
