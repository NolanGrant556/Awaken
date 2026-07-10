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
    state = projectAguiEventToNarrative(state, textMessageContent("streamed answer"));
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

  it("surfaces tool recovery required and retry blocked notices", () => {
    let state = createInitialRunNarrativeState();
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
    expect(notices.some((item) => item.kind === "tool" && item.title === "Tool recovery required")).toBe(true);
    expect(notices.some((item) => item.kind === "tool" && item.title === "Tool retry blocked")).toBe(true);
  });
});
