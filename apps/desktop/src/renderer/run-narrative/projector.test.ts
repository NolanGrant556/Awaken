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
          status: "completed",
          activity: "读取 auth/service.go",
          resultPreview: "未发现越权路径",
          errorCode: undefined,
        },
        {
          taskId: "task-2",
          description: "检查测试入口",
          agentType: "explore",
          status: "running",
          activity: undefined,
          resultPreview: undefined,
          errorCode: undefined,
        },
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
    state = projectAguiEventToNarrative(state, { type: "TOOL_CALL_START", ...base });
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
    state = projectAguiEventToNarrative(state, { type: "TOOL_CALL_START", ...base });
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
    state = projectAguiEventToNarrative(state, { type: "TOOL_CALL_START", ...base });
    state = projectAguiEventToNarrative(state, {
      type: "AWAKEN_COMMAND_OUTPUT",
      ...base,
      commandStream: "media_status",
      delta: '{"stage":"queued","message":"视频任务已提交","task_id":"cgt-1"}\n',
    });
    expect(state.activeItem).toMatchObject({ taskId: "cgt-1", status: "queued" });
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
    state = projectAguiEventToNarrative(state, { type: "TOOL_CALL_START", ...base });
    state = projectAguiEventToNarrative(state, {
      type: "TOOL_CALL_ARGS",
      ...base,
      delta: JSON.stringify({ prompt: "十五张", mode: "group", max_images: 15 }),
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
    state = projectAguiEventToNarrative(state, { type: "TOOL_CALL_START", ...base });
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
    state = projectAguiEventToNarrative(state, { type: "TOOL_CALL_START", ...base });
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
