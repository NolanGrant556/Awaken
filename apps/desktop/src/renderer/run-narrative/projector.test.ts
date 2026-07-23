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

  it("surfaces strategy_change_required directives with a dedicated title", () => {
    let state = createInitialRunNarrativeState();
    state = projectAguiEventToNarrative(state, {
      type: "AWAKEN_TOOL_RECOVERY_REQUIRED",
      toolCallId: "call-3",
      toolCallName: "read_artifact",
      errorCode: "read_artifact_specialized_tool_required",
      message:
        "strategy_change_required: 6 tool failures accumulated (turn=6 run=6); do not repeat the failing approach",
      recoveryOptions: ["use_specialized_artifact_tool", "change_tool_or_library"],
    });

    const notice = state.committedItems.find(
      (item) => item.kind === "tool" && item.title === "Strategy change required",
    );
    expect(notice).toBeDefined();
    if (notice?.kind !== "tool") {
      throw new Error("strategy change notice missing");
    }
    expect(notice.detail).toContain("use_specialized_artifact_tool");
  });

  it("projects bounded Memory and Dream receipts without topic bodies", () => {
    let state = createInitialRunNarrativeState();
    state = projectAguiEventToNarrative(state, {
      type: "AWAKEN_MEMORY_RECALLED",
      recordId: "memory-recall-1",
      memories: [{ memoryId: "mem_1", scope: "user", memoryType: "user", revision: 2 }],
    });
    state = projectAguiEventToNarrative(state, {
      type: "AWAKEN_MEMORY_DREAM_COMPLETED",
      recordId: "dream_1",
      diagnostic: { reports: { user: { updated: 1, superseded: 2, conflicts: 0 } } },
    });

    const receipts = state.committedItems.filter((item) => item.kind === "memoryReceipt");
    expect(receipts).toHaveLength(2);
    expect(receipts[0]).toMatchObject({ title: "已召回 1 条记忆", detail: "用户 · user · r2", status: "done" });
    expect(receipts[1]).toMatchObject({ title: "记忆整理完成", detail: "更新 1 条 · 归档 2 条 · 冲突 0 条", status: "done" });
    expect(JSON.stringify(receipts)).not.toContain("topic body");
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

    const sessions = state.committedItems.filter((item) => item.kind === "acpSession");
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

    const tools = state.committedItems.filter((item) => item.kind === "acpTool");
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
});
