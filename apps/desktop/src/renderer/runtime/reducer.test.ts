import { describe, expect, it } from "vitest";
import type { AguiEvent } from "../../shared/agui";
import type {
  RuntimeRun,
  RuntimeThread,
  SupervisorStatusInfo,
} from "../../shared/runtime";
import {
  createInitialRuntimeState,
  latestThreadRunPlan,
  reduceRuntimeState,
  type RuntimeAction,
} from "./reducer";

function dispatch(actions: RuntimeAction[]) {
  let state = createInitialRuntimeState();
  for (const action of actions) {
    state = reduceRuntimeState(state, action);
  }
  return state;
}

function runEvent(event: AguiEvent): RuntimeAction {
  return { type: "run/event", event };
}

function runEventAt(event: AguiEvent, receivedAtMs: number): RuntimeAction {
  return { type: "run/event", event, receivedAtMs };
}

function countRunCancelledSteps(
  state: ReturnType<typeof createInitialRuntimeState>,
): number {
  return state.steps.filter((step) => step.id === "run-cancelled").length;
}

function makeRun(overrides: Partial<RuntimeRun> = {}): RuntimeRun {
  return {
    runId: "run-1",
    threadId: "thread-1",
    status: "running",
    cancelRequested: false,
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

describe("runtime reducer user question resolution", () => {
  it("removes a cancelled question while keeping the run active", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "需要一个选择" },
      runEvent({
        type: "AWAKEN_USER_QUESTION_REQUESTED",
        runId: "run-1",
        threadId: "thread-1",
        toolCallId: "tool-ask-1",
        toolCallName: "ask_user",
        questionId: "question-1",
        content:
          '{"questions":[{"question":"Choose a theme","options":[{"label":"Dark"},{"label":"Light"}]}]}',
      }),
      runEvent({
        type: "AWAKEN_USER_QUESTION_RESOLVED",
        runId: "run-1",
        threadId: "thread-1",
        toolCallId: "tool-ask-1",
        toolCallName: "ask_user",
        questionId: "question-1",
        questionResolution: "cancelled",
      }),
    ]);

    expect(state.userQuestions).toEqual([]);
    expect(state.phase).toBe("starting");
    expect(state.status).not.toBe("cancelled");
    expect(
      state.steps.find((step) => step.id === "user-question:question-1"),
    ).toMatchObject({
      title: "Question cancelled",
      description: "The user chose not to answer; the run continues.",
      status: "done",
    });
  });
});

describe("runtime reducer thread rewind replay", () => {
  it("clears the sidebar run head when replay restores an empty lineage", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "first" },
      {
        type: "run/created",
        run: makeRun({
          runId: "run-1",
          threadId: "thread-1",
          status: "finished",
        }),
      },
      {
        type: "run/threadAdded",
        threadId: "thread-1",
        label: "first",
        runId: "run-1",
      },
      {
        type: "run/replayBatch",
        actions: [
          {
            type: "run/threadReplayStarted",
            threadId: "thread-1",
          },
        ],
      },
    ]);

    expect(state.runId).toBeUndefined();
    expect(state.threads[0]?.threadId).toBe("thread-1");
    expect(state.threads[0]?.runId).toBeUndefined();
    expect(state.threads[0]?.activeRunId).toBeUndefined();
  });

  it("keeps only the canonical prefix when a replacement run is requested after rewind replay", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "first" },
      {
        type: "run/created",
        run: makeRun({
          runId: "run-1",
          threadId: "thread-1",
          status: "finished",
        }),
      },
      {
        type: "run/threadAdded",
        threadId: "thread-1",
        label: "first",
        runId: "run-1",
      },
      { type: "run/requested", prompt: "second" },
      {
        type: "run/created",
        run: makeRun({
          runId: "run-2",
          threadId: "thread-1",
          status: "finished",
        }),
      },
      {
        type: "run/replayBatch",
        actions: [
          {
            type: "run/threadReplayStarted",
            threadId: "thread-1",
            runId: "run-1",
          },
          { type: "run/requested", prompt: "first", runId: "run-1" },
          {
            type: "run/created",
            run: makeRun({
              runId: "run-1",
              threadId: "thread-1",
              status: "finished",
            }),
          },
        ],
      },
      { type: "run/requested", prompt: "replacement" },
    ]);

    const prompts = state.narrative.committedItems.filter(
      (item) => item.kind === "prompt",
    );
    expect(prompts.map((item) => item.text)).toEqual(["first", "replacement"]);
  });

  it("starts a replacement from an empty lineage with a single prompt", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "first" },
      {
        type: "run/created",
        run: makeRun({
          runId: "run-1",
          threadId: "thread-1",
          status: "finished",
        }),
      },
      {
        type: "run/threadAdded",
        threadId: "thread-1",
        label: "first",
        runId: "run-1",
      },
      {
        type: "run/replayBatch",
        actions: [
          {
            type: "run/threadReplayStarted",
            threadId: "thread-1",
          },
        ],
      },
      { type: "run/requested", prompt: "edited" },
    ]);

    const prompts = state.narrative.committedItems.filter(
      (item) => item.kind === "prompt",
    );
    expect(prompts.map((item) => item.text)).toEqual(["edited"]);
  });

  it("replaces the first prompt in one transition without an empty intermediate narrative", () => {
    const beforeReplace = dispatch([
      { type: "run/requested", prompt: "first" },
      {
        type: "run/created",
        run: makeRun({
          runId: "run-1",
          threadId: "thread-1",
          status: "finished",
        }),
      },
      {
        type: "run/threadAdded",
        threadId: "thread-1",
        label: "first",
        runId: "run-1",
      },
    ]);
    expect(
      beforeReplace.narrative.committedItems
        .filter((item) => item.kind === "prompt")
        .map((item) => item.text),
    ).toEqual(["first"]);

    const replaced = reduceRuntimeState(beforeReplace, {
      type: "run/promptReplaced",
      targetRunId: "run-1",
      prompt: "edited first message",
    });

    expect(replaced.activeThreadId).toBe("thread-1");
    expect(replaced.phase).toBe("starting");
    expect(replaced.runId).toBeUndefined();
    expect(
      replaced.narrative.committedItems
        .filter((item) => item.kind === "prompt")
        .map((item) => item.text),
    ).toEqual(["edited first message"]);
    expect(replaced.narrative.activeItem).toMatchObject({
      kind: "planning",
      status: "active",
    });
  });

  it("keeps the prefix and drops the edited suffix in one prompt replacement", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "first" },
      {
        type: "run/created",
        run: makeRun({
          runId: "run-1",
          threadId: "thread-1",
          status: "finished",
        }),
      },
      {
        type: "run/threadAdded",
        threadId: "thread-1",
        label: "first",
        runId: "run-1",
      },
      { type: "run/requested", prompt: "second" },
      {
        type: "run/created",
        run: makeRun({
          runId: "run-2",
          threadId: "thread-1",
          status: "finished",
        }),
      },
      { type: "run/requested", prompt: "third" },
      {
        type: "run/created",
        run: makeRun({
          runId: "run-3",
          threadId: "thread-1",
          status: "finished",
        }),
      },
      {
        type: "run/promptReplaced",
        targetRunId: "run-2",
        prompt: "edited second",
      },
    ]);

    expect(
      state.narrative.committedItems
        .filter((item) => item.kind === "prompt")
        .map((item) => item.text),
    ).toEqual(["first", "edited second"]);
    expect(state.phase).toBe("starting");
    expect(state.activeThreadId).toBe("thread-1");
  });
});

describe("runtime reducer prompt timestamps", () => {
  it("stores startedAtMs on the requested prompt", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "hello", startedAtMs: 1_725_000_000_000 },
    ]);
    expect(state.narrative.committedItems[0]).toMatchObject({
      kind: "prompt",
      text: "hello",
      createdAtMs: 1_725_000_000_000,
    });
  });
});

describe("runtime reducer cancel handling", () => {
  it("restores transcript actions in one reducer transition and marks only replayed narrative items", () => {
    const replayed = reduceRuntimeState(createInitialRuntimeState(), {
      type: "run/replayBatch",
      actions: [
        {
          type: "run/threadReplayStarted",
          threadId: "thread-1",
          runId: "run-1",
        },
        { type: "run/requested", prompt: "读取历史文件" },
        runEvent({
          type: "REASONING_MESSAGE_CONTENT",
          runId: "run-1",
          threadId: "thread-1",
          messageId: "reasoning-history",
          delta: "先分析项目文件",
        }),
        runEvent({
          type: "TOOL_CALL_START",
          runId: "run-1",
          threadId: "thread-1",
          toolCallId: "read-history",
          toolCallName: "Read",
        }),
        runEvent({
          type: "TOOL_CALL_RESULT",
          runId: "run-1",
          threadId: "thread-1",
          toolCallId: "read-history",
          toolCallName: "Read",
          toolStatus: "succeeded",
          content: "历史内容",
        }),
      ],
    });

    const replayedNarrativeItems = [
      ...replayed.narrative.committedItems,
      ...(replayed.narrative.activeItem ? [replayed.narrative.activeItem] : []),
    ];
    const historicalTool = replayedNarrativeItems.find(
      (item) => item.kind === "tool" || item.kind === "exploring",
    );
    expect(historicalTool).toBeDefined();
    expect(replayed.narrative.replayedItemIds).toContain(historicalTool?.id);
    const historicalThought = replayedNarrativeItems.find(
      (item) => item.kind === "thinking",
    );
    expect(historicalThought).toBeDefined();
    expect(replayed.narrative.replayedItemIds).toContain(historicalThought?.id);
    if (historicalTool?.kind === "exploring") {
      expect(replayed.narrative.replayedItemIds).toContain(
        historicalTool.actions[0]?.id,
      );
    }
    expect(
      replayed.threadViews["thread-1"]?.narrative.replayedItemIds,
    ).toContain(historicalTool?.id);

    const withLiveTool = reduceRuntimeState(
      replayed,
      runEvent({
        type: "TOOL_CALL_START",
        runId: "run-2",
        threadId: "thread-1",
        toolCallId: "read-live",
        toolCallName: "Read",
      }),
    );
    expect(withLiveTool.narrative.activeItem?.kind).toBe("exploring");
    if (withLiveTool.narrative.activeItem?.kind !== "exploring") {
      throw new Error("live exploring item missing");
    }
    const liveAction = withLiveTool.narrative.activeItem.actions.find(
      (candidate) => candidate.toolCallId === "read-live",
    );
    expect(liveAction).toBeDefined();
    expect(withLiveTool.narrative.replayedItemIds).not.toContain(
      liveAction?.id,
    );
  });

  it("projects run creation failures into the current conversation turn", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "start a task" },
      { type: "run/createFailed", message: "Failed to create run" },
    ]);

    expect(state.phase).toBe("error");
    expect(state.errorMessage).toBe("Failed to create run");
    expect(
      state.narrative.committedItems.find(
        (item) => item.kind === "answer" && item.status === "error",
      ),
    ).toMatchObject({
      kind: "answer",
      markdown: "Failed to create run",
      status: "error",
    });
    expect(state.narrative.activeItem).toBeUndefined();
  });

  it("run/cancelRequested sets phase and status to cancelling", () => {
    const state = dispatch([{ type: "run/cancelRequested" }]);
    expect(state.phase).toBe("cancelling");
    expect(state.status).toBe("cancelling");
  });

  it("run/cancelSync with cancelled status keeps cancelled out of failed state", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "cancel me" },
      {
        type: "run/created",
        run: makeRun({ runId: "run-1", threadId: "thread-1" }),
      },
      {
        type: "run/threadAdded",
        threadId: "thread-1",
        label: "cancel me",
        runId: "run-1",
      },
      runEvent({
        type: "AWAKEN_ARTIFACT_CREATED",
        artifactId: "cancelled-report",
        artifactTitle: "已生成报告.pdf",
        artifactKind: "pdf",
        artifactPath: "output/已生成报告.pdf",
        artifactStatus: "ready",
        artifactRole: "deliverable",
        artifactContractVersion: 2,
        artifactProducer: "artifact_publish",
      }),
      { type: "run/cancelRequested" },
      {
        type: "run/cancelSync",
        run: makeRun({
          runId: "run-1",
          threadId: "thread-1",
          status: "cancelled",
        }),
      },
    ]);

    expect(state.phase).toBe("cancelled");
    expect(state.status).toBe("cancelled");
    expect(state.timelineStatus).toBe("done");
    expect(state.errorMessage).toBeUndefined();
    expect(state.threads[0]).toMatchObject({
      runStatus: "cancelled",
      activeRunId: undefined,
    });
    expect(
      state.narrative.committedItems.find(
        (item) => item.kind === "deliverables",
      ),
    ).toMatchObject({
      terminalStatus: "cancelled",
      files: [
        {
          artifactId: "cancelled-report",
          extension: "pdf",
          fileType: "pdf",
        },
      ],
    });
  });

  it("RUN_STATUS_CHANGED cancelled sets phase to cancelled and adds one run-cancelled step", () => {
    const state = dispatch([
      runEvent({ type: "RUN_STATUS_CHANGED", status: "cancelled" }),
    ]);
    expect(state.phase).toBe("cancelled");
    expect(countRunCancelledSteps(state)).toBe(1);
  });

  it("RUN_STATUS_CHANGED(cancelled) followed by RUN_ERROR(RUN_CANCELLED) keeps a single run-cancelled step", () => {
    const state = dispatch([
      runEvent({ type: "RUN_STATUS_CHANGED", status: "cancelled" }),
      runEvent({
        type: "RUN_ERROR",
        code: "RUN_CANCELLED",
        message: "Run was cancelled by user",
      }),
    ]);
    expect(state.phase).toBe("cancelled");
    expect(countRunCancelledSteps(state)).toBe(1);
    expect(state.errorMessage).toBeUndefined();
  });

  it("RUN_ERROR with code RUN_CANCELLED alone sets phase to cancelled", () => {
    const state = dispatch([
      runEvent({
        type: "RUN_ERROR",
        code: "RUN_CANCELLED",
        message: "Run was cancelled by user",
      }),
    ]);
    expect(state.phase).toBe("cancelled");
    expect(countRunCancelledSteps(state)).toBe(1);
  });

  it("RUN_ERROR without RUN_CANCELLED code sets phase to error and does not add run-cancelled step", () => {
    const state = dispatch([
      runEvent({ type: "RUN_ERROR", code: "INTERNAL_ERROR", message: "boom" }),
    ]);
    expect(state.phase).toBe("error");
    expect(state.errorMessage).toBe("boom");
    expect(countRunCancelledSteps(state)).toBe(0);
  });

  it("RUN_ERROR with PROVIDER_AUTH_MISSING maps to readable API key message without leaking secrets", () => {
    const state = dispatch([
      runEvent({
        type: "RUN_ERROR",
        code: "PROVIDER_AUTH_MISSING",
        message: "missing API key for provider",
      }),
    ]);
    expect(state.phase).toBe("error");
    expect(state.errorMessage).toBe(
      "API key 缺失，请在 Settings > Models 配置模型 API key",
    );
    expect(state.errorMessage).not.toContain("sk-");
    expect(state.errorMessage).not.toContain("LLM_API_KEY=");
    const errorStep = state.steps.find((step) => step.id === "run-error");
    expect(errorStep?.description).toBe(
      "API key 缺失，请在 Settings > Models 配置模型 API key",
    );
    expect(errorStep?.tag).toBe("PROVIDER_AUTH_MISSING");
  });

  it("RUN_ERROR with PROVIDER_REQUEST_FAILED maps to readable provider failure message without leaking secrets", () => {
    const state = dispatch([
      runEvent({
        type: "RUN_ERROR",
        code: "PROVIDER_REQUEST_FAILED",
        message: "upstream returned 503",
      }),
    ]);
    expect(state.phase).toBe("error");
    expect(state.errorMessage).toContain("Provider 请求失败：");
    expect(state.errorMessage).toContain("upstream returned 503");
    expect(state.errorMessage).not.toContain("sk-");
    const errorStep = state.steps.find((step) => step.id === "run-error");
    expect(errorStep?.description).toContain("Provider 请求失败：");
    expect(errorStep?.tag).toBe("PROVIDER_REQUEST_FAILED");
  });

  it("RUN_ERROR hides internal compaction diagnostics from UI state", () => {
    const state = dispatch([
      runEvent({
        type: "RUN_ERROR",
        message:
          "LLM stream call failed: COMPACT_TARGET_UNACHIEVABLE: projected=13745 target=3200 mandatory_floor=13065",
      }),
    ]);
    expect(state.phase).toBe("error");
    expect(state.errorMessage).toBe("本次任务未能继续，请重试。");
    expect(state.errorMessage).not.toContain("COMPACT_TARGET_UNACHIEVABLE");
    expect(state.steps.find((step) => step.id === "run-error")?.description).toBe(
      "本次任务未能继续，请重试。",
    );
  });
});

describe("runtime reducer native subagent isolation", () => {
  it("ignores orphan tool progress instead of projecting it into the root agent", () => {
    const state = dispatch([
      runEvent({
        type: "AWAKEN_SUBAGENT_CHANGED",
        runId: "run-1",
        subagentTaskId: "task-1",
        subagentAgentId: "agent-1",
        subagentDescription: "排查会话隔离",
        subagentAgentType: "explore",
        subagentStatus: "running",
      }),
      runEvent({
        type: "TOOL_PROGRESS",
        runId: "run-1",
        toolCallId: "child-bash-1",
        toolCallName: "Bash",
        progress: "正在运行子任务命令",
      }),
    ]);

    expect(state.toolResults).toHaveLength(0);
    expect(state.steps.some((step) => step.id.includes("child-bash-1"))).toBe(
      false,
    );
    expect(
      state.narrative.committedItems.some(
        (item) => item.kind === "tool" || item.kind === "exploring",
      ),
    ).toBe(false);
    expect(state.narrative.activeItem).toBeUndefined();
  });

  it("routes child reasoning, tools, and answers into independent task narrative", () => {
    const state = dispatch([
      runEvent({
        type: "AWAKEN_SUBAGENT_CHANGED",
        runId: "run-1",
        subagentTaskId: "task-1",
        subagentAgentId: "agent-1",
        subagentDescription: "排查会话隔离",
        subagentAgentType: "explore",
        subagentStatus: "running",
      }),
      runEvent({
        type: "REASONING_MESSAGE_CONTENT",
        runId: "run-1",
        messageId: "reasoning-1",
        subagentTaskId: "task-1",
        subagentAgentId: "agent-1",
        delta: "先检查事件身份",
      }),
      runEvent({
        type: "TOOL_CALL_START",
        runId: "run-1",
        toolCallId: "Read_0",
        toolCallName: "Read",
        subagentTaskId: "task-1",
        subagentAgentId: "agent-1",
      }),
      runEvent({
        type: "TOOL_CALL_RESULT",
        runId: "run-1",
        toolCallId: "Read_0",
        toolCallName: "Read",
        toolStatus: "succeeded",
        content: "读取完成",
        subagentTaskId: "task-1",
        subagentAgentId: "agent-1",
      }),
      runEvent({
        type: "TEXT_MESSAGE_CONTENT",
        runId: "run-1",
        messageId: "answer-1",
        subagentTaskId: "task-1",
        subagentAgentId: "agent-1",
        delta: "已定位隔离问题",
      }),
    ]);

    expect(state.reasoningText).toBe("");
    expect(state.assistantText).toBe("");
    expect(state.toolResults).toHaveLength(0);
    expect(state.subagentOrder).toEqual(["task-1"]);
    const task = state.subagentsById["task-1"];
    expect(task.agentId).toBe("agent-1");
    const childItems = [
      ...task.narrative.committedItems,
      ...(task.narrative.activeItem ? [task.narrative.activeItem] : []),
    ];
    expect(childItems.some((item) => item.kind === "thinking")).toBe(true);
    expect(
      childItems.some(
        (item) => item.kind === "tool" || item.kind === "exploring",
      ),
    ).toBe(true);
    expect(childItems.some((item) => item.kind === "answer")).toBe(true);
  });

  it("keeps terminal task truth stable and clears stale activity", () => {
    const state = dispatch([
      runEvent({
        type: "AWAKEN_SUBAGENT_ACTIVITY",
        subagentTaskId: "task-1",
        subagentStatus: "running",
        subagentActivity: "正在使用 Read",
      }),
      runEvent({
        type: "AWAKEN_SUBAGENT_CHANGED",
        subagentTaskId: "task-1",
        subagentStatus: "completed",
        subagentResultPreview: "完成",
      }),
      runEvent({
        type: "AWAKEN_SUBAGENT_ACTIVITY",
        subagentTaskId: "task-1",
        subagentStatus: "running",
        subagentActivity: "迟到的事件",
      }),
    ]);

    expect(state.subagentsById["task-1"]).toMatchObject({
      status: "completed",
      resultPreview: "完成",
    });
    expect(state.subagentsById["task-1"].activity).toBeUndefined();
  });

  it("does not allow a late running event to reverse cancelling", () => {
    const state = dispatch([
      runEvent({
        type: "AWAKEN_SUBAGENT_CHANGED",
        subagentTaskId: "task-1",
        subagentStatus: "running",
      }),
      runEvent({
        type: "AWAKEN_SUBAGENT_CHANGED",
        subagentTaskId: "task-1",
        subagentStatus: "cancelling",
      }),
      runEvent({
        type: "AWAKEN_SUBAGENT_ACTIVITY",
        subagentTaskId: "task-1",
        subagentStatus: "running",
        subagentActivity: "迟到的 running",
      }),
    ]);

    expect(state.subagentsById["task-1"].status).toBe("cancelling");
  });

  it("keeps plan/general isolation fields on the independent task view", () => {
    const state = dispatch([
      runEvent({
        type: "AWAKEN_SUBAGENT_CHANGED",
        runId: "run-1",
        subagentTaskId: "task-write",
        subagentAgentId: "agent-write",
        subagentDescription: "实现改动",
        subagentAgentType: "general",
        subagentWorkspaceMode: "shared",
        subagentStatus: "running",
      }),
    ]);
    expect(state.subagentsById["task-write"]).toMatchObject({
      agentType: "general",
      workspaceMode: "shared",
      description: "实现改动",
    });
  });
});

describe("runtime reducer canonical auth wire", () => {
  it("projects authUrl from the Go AG-UI contract", () => {
    const state = dispatch([
      runEvent({
        type: "AWAKEN_AUTH_REQUIRED",
        toolCallId: "call-auth",
        authProvider: "github",
        authScope: "repo",
        authUrl: "https://github.com/login/device",
      }),
    ]);
    expect(
      state.steps.find((step) => step.id.startsWith("auth:")),
    ).toMatchObject({
      title: "Authorization required",
      description: "github | repo | https://github.com/login/device",
    });
  });
});

describe("runtime reducer replay idempotency", () => {
  it("run/recovered resets state so full replay does not duplicate text deltas", () => {
    // Simulate a partial state with some accumulated text, then a recovery
    // (which resets to initial), then a full replay of text deltas.
    const state = dispatch([
      runEvent({
        type: "TEXT_MESSAGE_CONTENT",
        messageId: "msg-1",
        delta: "partial",
      }),
      { type: "run/recovered", run: makeRun({ status: "running" }) },
      runEvent({
        type: "TEXT_MESSAGE_CONTENT",
        messageId: "msg-1",
        delta: "hello",
      }),
      runEvent({
        type: "TEXT_MESSAGE_CONTENT",
        messageId: "msg-1",
        delta: " world",
      }),
    ]);
    expect(state.assistantText).toBe("hello world");
  });

  it("run/recovered resets state so replayed artifacts do not duplicate", () => {
    const state = dispatch([
      runEvent({
        type: "AWAKEN_ARTIFACT_CREATED",
        artifactId: "art-1",
        artifactTitle: "README",
        artifactKind: "file",
      }),
      { type: "run/recovered", run: makeRun({ status: "running" }) },
      runEvent({
        type: "AWAKEN_ARTIFACT_CREATED",
        artifactId: "art-1",
        artifactTitle: "README",
        artifactKind: "file",
      }),
      runEvent({
        type: "AWAKEN_ARTIFACT_UPDATED",
        artifactId: "art-1",
        artifactTitle: "README",
        artifactKind: "file",
        artifactStatus: "ready",
      }),
    ]);
    expect(state.artifacts.length).toBe(1);
    expect(state.artifacts[0].id).toBe("art-1");
  });

  it("run/recovered with terminal status sets correct phase", () => {
    const finishedState = dispatch([
      { type: "run/recovered", run: makeRun({ status: "finished" }) },
    ]);
    expect(finishedState.phase).toBe("done");

    const errorState = dispatch([
      {
        type: "run/recovered",
        run: makeRun({ status: "error", error: "boom" }),
      },
    ]);
    expect(errorState.phase).toBe("error");
    expect(errorState.errorMessage).toBe("boom");
  });

  it("run/recovered sets timelineTitle to recovery label", () => {
    const state = dispatch([
      { type: "run/recovered", run: makeRun({ status: "running" }) },
    ]);
    expect(state.timelineTitle).toBe("恢复运行");
  });

  it("run/recovered does not affect new run prompt title", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "Build a feature" },
    ]);
    expect(state.timelineTitle).toContain("Build a feature");
  });

  it("run/recovered keeps inline token position through durable leadingText", () => {
    const leadingText = "现在我需要你帮我分析现有的财务数据风险，然后使用 ";
    const state = dispatch([
      {
        type: "run/recovered",
        run: makeRun({
          runId: "run-1",
          threadId: "thread-1",
          status: "running",
          composerTokens: [
            {
              id: "skill:pptx:user",
              type: "skill",
              label: "pptx",
              leadingText,
              payload: { skillId: "pptx", sourceOrigin: "user" },
            },
          ],
        }),
        prompt:
          "现在我需要你帮我分析现有的财务数据风险，然后使用 输出一份风险报告给我",
      },
    ]);

    const promptItem = state.narrative.committedItems.find(
      (item) => item.kind === "prompt",
    );
    expect(promptItem?.kind).toBe("prompt");
    if (promptItem?.kind !== "prompt") {
      throw new Error("prompt item missing");
    }
    expect(promptItem.tokens?.[0]?.leadingText).toBe(leadingText);
  });

  it("run/recovered restores prompt from thread label before full replay", () => {
    const state = dispatch([
      {
        type: "run/recovered",
        run: makeRun({
          runId: "run-1",
          threadId: "thread-1",
          status: "running",
        }),
        prompt: "Restore this run",
      },
      runEvent({
        type: "RUN_STARTED",
        seq: 1,
        runId: "run-1",
        threadId: "thread-1",
      }),
      runEvent({
        type: "RUN_STARTED",
        seq: 1,
        runId: "run-1",
        threadId: "thread-1",
      }),
    ]);

    expect(state.prompt).toBe("Restore this run");
    expect(
      state.narrative.committedItems.filter((item) => item.kind === "prompt"),
    ).toEqual([
      { kind: "prompt", id: "prompt:thread-1:1", text: "Restore this run", createdAtMs: Date.parse("2026-01-01T00:00:00Z") },
    ]);
  });

  it("run/recovered rebuilds terminal answer as done during replay", () => {
    const state = dispatch([
      {
        type: "run/recovered",
        run: makeRun({
          runId: "run-1",
          threadId: "thread-1",
          status: "finished",
        }),
        prompt: "Recover final answer",
      },
      runEvent({
        type: "RUN_STARTED",
        seq: 1,
        runId: "run-1",
        threadId: "thread-1",
      }),
      runEvent({
        type: "TEXT_MESSAGE_CONTENT",
        seq: 2,
        messageId: "msg-1",
        delta: "done",
      }),
      runEvent({ type: "RUN_FINISHED", seq: 3, result: "done" }),
    ]);

    expect(state.phase).toBe("done");
    expect(state.narrative.activeItem).toBeUndefined();
    expect(state.narrative.committedItems).toContainEqual({
      kind: "answer",
      id: "answer:run-1:msg-1",
      markdown: "done",
      status: "done",
      isFinal: true,
    });
  });

  it("desktop recovery smoke replays overlap and continues live tail", () => {
    const state = dispatch([
      {
        type: "run/recovered",
        run: makeRun({
          runId: "run-1",
          threadId: "thread-1",
          status: "running",
        }),
        prompt: "Recover mixed run",
      },
      runEvent({
        type: "RUN_STARTED",
        seq: 1,
        runId: "run-1",
        threadId: "thread-1",
      }),
      runEvent({
        type: "REASONING_MESSAGE_CONTENT",
        seq: 2,
        messageId: "reason-1",
        delta: "checking",
      }),
      runEvent({
        type: "TOOL_CALL_START",
        seq: 3,
        toolCallId: "read-1",
        toolCallName: "read_file",
      }),
      runEvent({
        type: "TOOL_CALL_RESULT",
        seq: 4,
        toolCallId: "read-1",
        toolCallName: "read_file",
        content: "README",
      }),
      runEvent({
        type: "AWAKEN_ARTIFACT_CREATED",
        seq: 5,
        artifactId: "artifact-1",
        artifactTitle: "note.md",
        artifactKind: "file",
        artifactPath: "note.md",
      }),
      runEvent({
        type: "TEXT_MESSAGE_CONTENT",
        seq: 6,
        messageId: "msg-1",
        delta: "hello",
      }),
      { type: "run/reconnecting" },
      runEvent({
        type: "TEXT_MESSAGE_CONTENT",
        seq: 6,
        messageId: "msg-1",
        delta: "hello",
      }),
      runEvent({
        type: "TEXT_MESSAGE_CONTENT",
        seq: 7,
        messageId: "msg-1",
        delta: " world",
      }),
    ]);

    expect(
      state.narrative.committedItems.filter((item) => item.kind === "prompt"),
    ).toHaveLength(1);
    expect(state.reasoningText).toBe("checking");
    expect(state.artifacts).toHaveLength(1);
    expect(state.narrative.activeItem).toMatchObject({
      kind: "answer",
      id: "answer:run-1:msg-1",
      markdown: "hello world",
      status: "active",
    });
  });
});

describe("runtime reducer run narrative projection", () => {
  it("run/requested creates prompt committed item and planning active item", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "Build the workbench" },
    ]);

    expect(state.narrative.committedItems).toEqual([
      { kind: "prompt", id: "prompt:draft:1", text: "Build the workbench" },
    ]);
    expect(state.narrative.activeItem).toEqual({
      kind: "planning",
      id: "planning:draft:1",
      text: "Planning next moves",
      status: "active",
    });
  });

  it("keeps selected composer skill tokens on prompt narrative items", () => {
    const state = dispatch([
      {
        type: "run/requested",
        prompt: "你好，你看到这个 skill 不?",
        promptTokens: [
          {
            id: "skill:deep-research:user",
            type: "skill",
            label: "deep-research",
            leadingText: "你好，你看到这个 skill 不?",
            payload: {
              skillId: "deep-research",
              sourceOrigin: "user",
              skillFilePath: "/home/.awaken/skills/deep-research/SKILL.md",
            },
          },
        ],
      },
    ]);

    expect(state.narrative.committedItems[0]).toMatchObject({
      kind: "prompt",
      text: "你好，你看到这个 skill 不?",
      tokens: [
        {
          id: "skill:deep-research:user",
          type: "skill",
          label: "deep-research",
          leadingText: "你好，你看到这个 skill 不?",
          payload: {
            skillId: "deep-research",
            sourceOrigin: "user",
            skillFilePath: "/home/.awaken/skills/deep-research/SKILL.md",
          },
        },
      ],
    });
  });

  it("creates a durable prompt item for a comment-only run", () => {
    const comment = {
      id: "comment-1",
      projectId: "project-1",
      folderPath: "/workspace",
      relativePath: "src/main.ts",
      displayName: "main.ts",
      contentDigest: "a".repeat(64),
      startLine: 7,
      endLine: 8,
      selectedText: "const value = 1;",
      comment: "请提取为常量",
      createdAt: "2026-08-31T00:00:00.000Z",
      validationState: "ready" as const,
    };
    const state = dispatch([{
      type: "run/requested",
      prompt: "",
      promptFileComments: [comment],
    }]);

    expect(state.narrative.committedItems[0]).toMatchObject({
      kind: "prompt",
      text: "",
      fileComments: [comment],
    });
  });

  it("projects reasoning and text deltas into thinking and answer items", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "Say hello" },
      runEvent({ type: "RUN_STARTED", seq: 1 }),
      runEvent({
        type: "REASONING_MESSAGE_CONTENT",
        seq: 2,
        messageId: "r1",
        delta: "checking",
      }),
      runEvent({
        type: "TEXT_MESSAGE_CONTENT",
        seq: 3,
        messageId: "m1",
        delta: "hello",
      }),
      runEvent({
        type: "TEXT_MESSAGE_CONTENT",
        seq: 4,
        messageId: "m1",
        delta: " world",
      }),
      runEvent({ type: "TEXT_MESSAGE_END", seq: 5, messageId: "m1" }),
    ]);

    const thinking = state.narrative.committedItems.find(
      (item) => item.kind === "thinking",
    );
    const answer = state.narrative.committedItems.find(
      (item) => item.kind === "answer",
    );

    expect(thinking).toMatchObject({
      kind: "thinking",
      id: "thinking:r1",
      text: "checking",
      status: "done",
    });
    expect(answer).toMatchObject({
      kind: "answer",
      id: "answer:m1",
      markdown: "hello world",
      status: "done",
    });
    expect(state.narrative.activeItem).toBeUndefined();
  });

  it("keeps assistant text segments when a reused message id is interrupted by tools", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "Investigate" },
      {
        type: "run/created",
        run: makeRun({ runId: "run-1", threadId: "thread-1" }),
      },
      runEvent({ type: "RUN_STARTED", seq: 1, runId: "run-1" }),
      runEvent({
        type: "TEXT_MESSAGE_CONTENT",
        seq: 2,
        runId: "run-1",
        messageId: "m1",
        delta: "我先看文件。",
      }),
      runEvent({
        type: "TOOL_CALL_START",
        seq: 3,
        runId: "run-1",
        toolCallId: "read-1",
        toolCallName: "Read",
      }),
      runEvent({
        type: "TOOL_CALL_RESULT",
        seq: 4,
        runId: "run-1",
        toolCallId: "read-1",
        toolCallName: "Read",
        content: "package main",
      }),
      runEvent({
        type: "TEXT_MESSAGE_CONTENT",
        seq: 5,
        runId: "run-1",
        messageId: "m1",
        delta: "最终总结。",
      }),
      runEvent({
        type: "RUN_FINISHED",
        seq: 6,
        runId: "run-1",
        result: "done",
      }),
    ]);

    const answers = state.narrative.committedItems.filter(
      (item) => item.kind === "answer",
    );

    expect(answers).toEqual([
      {
        kind: "answer",
        id: "answer:run-1:m1",
        markdown: "我先看文件。",
        status: "done",
      },
      {
        kind: "answer",
        id: "answer:run-1:m1:segment:2",
        markdown: "最终总结。",
        status: "done",
        isFinal: true,
      },
    ]);
    expect(state.narrative.committedItems.map((item) => item.kind)).toEqual([
      "prompt",
      "planning",
      "answer",
      "exploring",
      "answer",
    ]);
  });

  it("projects tool events while deferring artifact summaries", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "Patch a file" },
      runEvent({ type: "RUN_STARTED", seq: 1 }),
      runEvent({
        type: "TOOL_CALL_START",
        seq: 2,
        toolCallId: "tool-1",
        toolCallName: "apply_patch",
      }),
      runEvent({
        type: "TOOL_CALL_RESULT",
        seq: 3,
        toolCallId: "tool-1",
        toolCallName: "apply_patch",
        content: "pending approval",
      }),
      runEvent({
        type: "AWAKEN_ARTIFACT_CREATED",
        seq: 5,
        artifactId: "artifact-1",
        artifactTitle: "note.md",
        artifactKind: "added",
        artifactPath: "note.md",
      }),
    ]);

    const tool = state.narrative.committedItems.find(
      (item) => item.id === "tool:tool-1",
    );

    expect(tool).toMatchObject({
      kind: "tool",
      title: "apply_patch",
      detail: "pending approval",
      status: "done",
    });
    expect(state.artifacts).toContainEqual(
      expect.objectContaining({ id: "artifact-1" }),
    );
    expect(
      state.narrative.committedItems.some((item) => item.kind === "fileChange"),
    ).toBe(false);
    expect(state.narrative.inspectorSelection).toEqual({
      kind: "tool",
      id: "tool:tool-1",
    });
  });

  it("projects consecutive read-only tools into one exploring item", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "Summarize project" },
      runEvent({ type: "RUN_STARTED", seq: 1 }),
      runEvent({
        type: "TOOL_CALL_START",
        seq: 2,
        toolCallId: "read-1",
        toolCallName: "Read",
      }),
      runEvent({
        type: "TOOL_CALL_ARGS",
        seq: 3,
        toolCallId: "read-1",
        toolCallName: "Read",
        delta: '{"file_path":"README.md","offset":1,"limit":80}',
      }),
      runEvent({
        type: "TOOL_CALL_RESULT",
        seq: 4,
        toolCallId: "read-1",
        toolCallName: "Read",
        content:
          "# Awaken\nlong content that should stay out of center narrative",
      }),
      runEvent({
        type: "TOOL_CALL_START",
        seq: 5,
        toolCallId: "list-1",
        toolCallName: "Glob",
      }),
      runEvent({
        type: "TOOL_CALL_ARGS",
        seq: 6,
        toolCallId: "list-1",
        toolCallName: "Glob",
        delta: '{"pattern":"**/*.md"}',
      }),
      runEvent({
        type: "TOOL_CALL_RESULT",
        seq: 7,
        toolCallId: "list-1",
        toolCallName: "Glob",
        content: "README.md\ndocs/intro.md",
      }),
      runEvent({
        type: "TOOL_CALL_START",
        seq: 8,
        toolCallId: "grep-1",
        toolCallName: "Grep",
      }),
      runEvent({
        type: "TOOL_CALL_ARGS",
        seq: 9,
        toolCallId: "grep-1",
        toolCallName: "Grep",
        delta: '{"pattern":"Run Workbench","glob":"**/*.md"}',
      }),
      runEvent({
        type: "TOOL_CALL_RESULT",
        seq: 10,
        toolCallId: "grep-1",
        toolCallName: "Grep",
        content:
          "docs/desktop-run-workbench-vertical-slices.md:1:Run Workbench",
      }),
    ]);

    expect(state.narrative.activeItem).toMatchObject({
      kind: "exploring",
      id: "exploring:current:read-1",
      status: "active",
    });
    if (state.narrative.activeItem?.kind !== "exploring") {
      throw new Error("expected exploring item");
    }
    expect(state.narrative.activeItem.actions).toHaveLength(3);
    expect(
      state.narrative.activeItem.actions.map((action) => action.id),
    ).toEqual(["explore:read-1", "explore:list-1", "explore:grep-1"]);
    expect(state.narrative.activeItem.actions[0]).toMatchObject({
      title: "读取 README.md L1-L80",
      status: "done",
    });
    expect(state.narrative.activeItem.actions[1]).toMatchObject({
      title: "搜索 Markdown 文件",
      visualKind: "search",
      status: "done",
    });
    expect(state.narrative.activeItem.actions[2]).toMatchObject({
      title: '搜索 "Run Workbench" · Markdown 文件',
      visualKind: "search",
      status: "done",
    });
    expect(JSON.stringify(state.narrative.activeItem.actions)).not.toContain(
      "long content that should stay out",
    );
  });

  it("keeps later read-only tools after intervening thinking instead of backfilling older exploring", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "Inspect ordering" },
      runEvent({ type: "RUN_STARTED", seq: 1 }),
      runEvent({
        type: "TOOL_CALL_START",
        seq: 2,
        toolCallId: "read-1",
        toolCallName: "Read",
      }),
      runEvent({
        type: "TOOL_CALL_ARGS",
        seq: 3,
        toolCallId: "read-1",
        toolCallName: "Read",
        delta: '{"file_path":"README.md"}',
      }),
      runEvent({
        type: "TOOL_CALL_RESULT",
        seq: 4,
        toolCallId: "read-1",
        toolCallName: "Read",
        content: "# Awaken",
      }),
      runEvent({
        type: "REASONING_MESSAGE_START",
        seq: 5,
        messageId: "reason-1",
      }),
      runEvent({
        type: "REASONING_MESSAGE_CONTENT",
        seq: 6,
        messageId: "reason-1",
        delta: "Now inspect source.",
      }),
      runEvent({
        type: "REASONING_MESSAGE_END",
        seq: 7,
        messageId: "reason-1",
      }),
      runEvent({
        type: "TOOL_CALL_START",
        seq: 8,
        toolCallId: "list-1",
        toolCallName: "Glob",
      }),
      runEvent({
        type: "TOOL_CALL_ARGS",
        seq: 9,
        toolCallId: "list-1",
        toolCallName: "Glob",
        delta: '{"pattern":"src/**/*.go"}',
      }),
      runEvent({
        type: "TOOL_CALL_RESULT",
        seq: 10,
        toolCallId: "list-1",
        toolCallName: "Glob",
        content: "src/main.go",
      }),
      runEvent({ type: "RUN_FINISHED", seq: 11 }),
    ]);

    const visibleItems = state.narrative.committedItems.filter(
      (item) => item.kind !== "prompt" && item.kind !== "planning",
    );

    expect(visibleItems.map((item) => item.id)).toEqual([
      "exploring:current:read-1",
      "thinking:reason-1",
      "exploring:current:list-1",
    ]);
    expect(visibleItems[0]).toMatchObject({
      kind: "exploring",
      actions: [expect.objectContaining({ id: "explore:read-1" })],
    });
    expect(visibleItems[2]).toMatchObject({
      kind: "exploring",
      actions: [expect.objectContaining({ id: "explore:list-1" })],
    });
  });

  it("does not duplicate read-only exploring actions when sequenced events replay", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "Replay exploring" },
      runEvent({
        type: "TOOL_CALL_START",
        seq: 1,
        toolCallId: "read-1",
        toolCallName: "Read",
      }),
      runEvent({
        type: "TOOL_CALL_ARGS",
        seq: 2,
        toolCallId: "read-1",
        toolCallName: "Read",
        delta: '{"file_path":"README.md"}',
      }),
      runEvent({
        type: "TOOL_CALL_RESULT",
        seq: 3,
        toolCallId: "read-1",
        toolCallName: "Read",
        content: "# Awaken",
      }),
      runEvent({
        type: "TOOL_CALL_START",
        seq: 1,
        toolCallId: "read-1",
        toolCallName: "Read",
      }),
      runEvent({
        type: "TOOL_CALL_ARGS",
        seq: 2,
        toolCallId: "read-1",
        toolCallName: "Read",
        delta: '{"file_path":"README.md"}',
      }),
      runEvent({
        type: "TOOL_CALL_RESULT",
        seq: 3,
        toolCallId: "read-1",
        toolCallName: "Read",
        content: "# Awaken",
      }),
    ]);

    expect(state.narrative.activeItem).toMatchObject({
      kind: "exploring",
      id: "exploring:current:read-1",
    });
    if (state.narrative.activeItem?.kind !== "exploring") {
      throw new Error("expected exploring item");
    }
    expect(state.narrative.activeItem.actions).toHaveLength(1);
  });

  it("stores raw tool args and result for inspector records", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "Inspect tool" },
      runEvent({
        type: "TOOL_CALL_ARGS",
        seq: 1,
        toolCallId: "read-1",
        toolCallName: "Read",
        delta: '{"file_path":"README.md"}',
      }),
      runEvent({
        type: "TOOL_CALL_RESULT",
        seq: 2,
        toolCallId: "read-1",
        toolCallName: "Read",
        content: "# Awaken\nfull result",
      }),
    ]);

    expect(state.toolResults).toContainEqual({
      id: "read-1",
      title: "Read",
      args: '{"file_path":"README.md"}',
      content: "# Awaken\nfull result",
      status: "done",
    });
  });

  it("preserves neutral diagnostic reasons in tool history and narrative", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "Search for a missing token" },
      runEvent({
        type: "TOOL_CALL_RESULT",
        seq: 1,
        toolCallId: "rg-1",
        toolCallName: "Bash",
        content: '{"exit_code":1}',
        toolStatus: "succeeded",
        semanticStatus: "neutral",
        processStatus: "exited",
        processExitCode: 1,
        diagnostic: { reason: "no_match" },
      }),
    ]);

    expect(state.toolResults).toContainEqual({
      id: "rg-1",
      title: "Bash",
      content: '{"exit_code":1}',
      args: undefined,
      status: "done",
      toolStatus: "succeeded",
      diagnostic: { reason: "no_match" },
    });
    expect(
      state.steps.find((step) => step.id === "tool:rg-1")?.description,
    ).toContain("diagnostic: no_match");
    expect(state.narrative.committedItems).toContainEqual(
      expect.objectContaining({
        kind: "tool",
        id: "tool:rg-1",
        detail: expect.stringContaining("diagnostic: no_match"),
        status: "done",
      }),
    );
  });

  it("retains provider, snapshot, dependency and side-effect metadata", () => {
    const state = dispatch([
      runEvent({
        type: "TOOL_CALL_RESULT",
        seq: 1,
        toolCallId: "render-1",
        toolCallName: "presentation_render",
        content: "rendered",
        toolStatus: "succeeded",
        environmentSnapshotId: "env-1",
        toolAvailabilitySnapshotId: "tools-1",
        providerId: "builtin.presentation_render",
        providerSource: "builtin",
        capability: "presentation.render",
        capabilities: ["presentation.render", "artifact.preview"],
        readiness: "ready",
        dependency: { name: "libreoffice", kind: "executable" },
        dependencies: [{ name: "libreoffice", kind: "executable" }],
        sideEffectState: "applied",
      }),
    ]);

    expect(state.toolResults[0]).toMatchObject({
      id: "render-1",
      environmentSnapshotId: "env-1",
      toolAvailabilitySnapshotId: "tools-1",
      providerId: "builtin.presentation_render",
      providerSource: "builtin",
      capability: "presentation.render",
      capabilities: ["presentation.render", "artifact.preview"],
      readiness: "ready",
      dependency: { name: "libreoffice", kind: "executable" },
      dependencies: [{ name: "libreoffice", kind: "executable" }],
      sideEffectState: "applied",
    });
  });

  it("ignores args deltas, stores the complete args snapshot, and projects progress", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "Inspect streaming tool args" },
      runEvent({
        type: "TOOL_CALL_ARGS_DELTA",
        seq: 1,
        toolCallId: "read-1",
        toolCallName: "Read",
        delta: '{"file_path"',
      }),
      runEvent({
        type: "TOOL_CALL_ARGS_DELTA",
        seq: 2,
        toolCallId: "read-1",
        toolCallName: "Read",
        delta: ':"README.md"}',
      }),
      runEvent({
        type: "TOOL_PROGRESS",
        seq: 3,
        toolCallId: "read-1",
        toolCallName: "Read",
        progress: "Executing tool",
      }),
      runEvent({
        type: "TOOL_CALL_ARGS",
        seq: 4,
        toolCallId: "read-1",
        toolCallName: "Read",
        delta: '{"file_path":"README.md"}',
      }),
    ]);

    expect(state.toolResults).toContainEqual({
      id: "read-1",
      title: "Read",
      args: '{"file_path":"README.md"}',
      content: "",
      status: "active",
    });
    const step = state.steps.find((item) => item.id === "tool:read-1");
    expect(step).toMatchObject({
      title: "Read",
      description: "Tool arguments ready",
      status: "active",
    });
    expect(state.narrative.activeItem).toMatchObject({
      kind: "exploring",
      id: "exploring:current:read-1",
      status: "active",
    });
    if (state.narrative.activeItem?.kind !== "exploring") {
      throw new Error("expected exploring item");
    }
    expect(state.narrative.activeItem.actions[0]).toMatchObject({
      id: "explore:read-1",
      toolCallId: "read-1",
      title: "读取 README.md",
      detail: undefined,
      status: "active",
    });
  });

  it("creates only a sanitized live Write row from partial tool arguments", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "Wait for a complete tool call" },
      runEvent({
        type: "TOOL_CALL_ARGS_DELTA",
        seq: 1,
        toolCallId: "write-1",
        toolCallName: "Write",
        delta: '{"content":"private fragment',
        toolPreparationKind: "write",
        toolPreparationPath: "/private/report.md",
        toolPreparationAddedLines: 7,
      }),
    ]);

    expect(state.toolResults).toEqual([]);
    expect(state.steps).toEqual([]);
    expect(state.narrative.activeItem).toEqual({
      kind: "tool",
      id: "tool:write-1",
      toolCallId: "write-1",
      title: "写入 report.md",
      detail: "+7 行",
      status: "active",
    });
    expect(JSON.stringify(state)).not.toContain("private fragment");
    expect(JSON.stringify(state)).not.toContain("/private/");
  });

  it("does not rerender for hidden or unchanged argument delta snapshots", () => {
    const requested = reduceRuntimeState(createInitialRuntimeState(), {
      type: "run/requested",
      prompt: "Stream one file",
    });
    const hidden = reduceRuntimeState(
      requested,
      runEvent({
        type: "TOOL_CALL_ARGS_DELTA",
        toolCallId: "bash-1",
        toolCallName: "Bash",
        delta: '{"command":"private fragment',
      }),
    );
    expect(hidden).toBe(requested);

    const first = reduceRuntimeState(
      hidden,
      runEvent({
        type: "TOOL_CALL_ARGS_DELTA",
        toolCallId: "write-1",
        toolCallName: "Write",
        delta: '{"content":"private fragment',
        toolPreparationKind: "write",
        toolPreparationPath: "report.md",
        toolPreparationAddedLines: 1,
      }),
    );
    const unchanged = reduceRuntimeState(
      first,
      runEvent({
        type: "TOOL_CALL_ARGS_DELTA",
        toolCallId: "write-1",
        toolCallName: "Write",
        delta: " more private fragment",
        toolPreparationKind: "write",
        toolPreparationPath: "report.md",
        toolPreparationAddedLines: 1,
      }),
    );
    expect(unchanged).toBe(first);
  });

  it("shows a host-generated redacted install activity without assistant narration", () => {
    const message =
      "正在安装依赖: pip install requests --index-url sk-[redacted]";
    const state = dispatch([
      { type: "run/requested", prompt: "Prepare dependency" },
      runEvent({
        type: "TOOL_CALL_START",
        seq: 1,
        toolCallId: "shell-install-1",
        toolCallName: "shell",
      }),
      runEvent({
        type: "TOOL_PROGRESS",
        seq: 2,
        toolCallId: "shell-install-1",
        toolCallName: "shell",
        toolProgressStage: "started",
        toolProgressMessage: message,
        progress: message,
      }),
    ]);

    expect(state.narrative.activeItem).toEqual({
      kind: "tool",
      id: "tool:shell-install-1",
      toolCallId: "shell-install-1",
      title: "shell",
      detail: message,
      status: "active",
    });
    expect(JSON.stringify(state)).not.toContain(
      "sk-abcdefghijklmnopqrstuvwxyz",
    );
  });

  it("clears active tool projection when a run is cancelled during tool argument preparation", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "Cancel during tool prep" },
      runEvent({
        type: "TOOL_CALL_START",
        seq: 1,
        toolCallId: "tool-1",
        toolCallName: "run_command",
      }),
      runEvent({
        type: "TOOL_CALL_ARGS_DELTA",
        seq: 2,
        toolCallId: "tool-1",
        toolCallName: "run_command",
        delta: '{"command"',
      }),
      runEvent({
        type: "TOOL_PROGRESS",
        seq: 3,
        toolCallId: "tool-1",
        toolCallName: "run_command",
        progress: "Executing tool",
      }),
      runEvent({ type: "RUN_STATUS_CHANGED", seq: 4, status: "cancelled" }),
    ]);

    expect(state.phase).toBe("cancelled");
    expect(state.narrative.activeItem).toBeUndefined();
    expect(state.narrative.committedItems).toContainEqual({
      kind: "tool",
      id: "tool:tool-1",
      toolCallId: "tool-1",
      title: "run_command",
      detail: "Executing tool",
      status: "cancelled",
    });
  });

  it("keeps non-read-only tool active between tool call end and result progress", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "Run command" },
      runEvent({
        type: "TOOL_CALL_START",
        seq: 1,
        toolCallId: "tool-1",
        toolCallName: "run_command",
      }),
      runEvent({
        type: "TOOL_CALL_ARGS",
        seq: 2,
        toolCallId: "tool-1",
        toolCallName: "run_command",
        delta: '{"command":"pwd"}',
      }),
      runEvent({
        type: "TOOL_CALL_END",
        seq: 3,
        toolCallId: "tool-1",
        toolCallName: "run_command",
      }),
      runEvent({
        type: "TOOL_PROGRESS",
        seq: 4,
        toolCallId: "tool-1",
        toolCallName: "run_command",
        progress: "Executing tool",
      }),
    ]);

    expect(
      state.narrative.committedItems.find((item) => item.id === "tool:tool-1"),
    ).toBeUndefined();
    expect(state.narrative.activeItem).toEqual({
      kind: "tool",
      id: "tool:tool-1",
      toolCallId: "tool-1",
      title: "run_command",
      detail: "Executing tool",
      status: "active",
    });
  });

  it("stores command output deltas for the terminal inspector", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "Run shell" },
      runEvent({
        type: "TOOL_CALL_START",
        seq: 1,
        toolCallId: "shell-1",
        toolCallName: "shell",
      }),
      runEvent({
        type: "AWAKEN_COMMAND_OUTPUT",
        seq: 2,
        toolCallId: "shell-1",
        toolCallName: "shell",
        commandStream: "stdout",
        delta: "hel",
      }),
      runEvent({
        type: "AWAKEN_COMMAND_OUTPUT",
        seq: 3,
        toolCallId: "shell-1",
        toolCallName: "shell",
        commandStream: "stdout",
        delta: "lo",
        truncated: true,
      }),
    ]);

    expect(state.toolResults).toContainEqual({
      id: "shell-1",
      title: "shell",
      args: undefined,
      content: "",
      commandOutput: "hello",
      commandOutputTruncated: true,
      status: "active",
    });
    expect(
      state.steps.find((step) => step.id === "tool:shell-1"),
    ).toMatchObject({
      title: "shell",
      description: "lo",
      status: "active",
      tag: "stdout",
    });
  });

  it("stores patch previews and turn diff for the changes inspector", () => {
    const patch =
      "*** Begin Patch\n*** Add File: note.md\n+hello\n*** End Patch";
    const diff = "diff --git a/note.md b/note.md\nnew file mode 100644";
    const state = dispatch([
      { type: "run/requested", prompt: "Patch file" },
      runEvent({
        type: "AWAKEN_PATCH_UPDATED",
        seq: 1,
        toolCallId: "patch-1",
        toolCallName: "Edit",
        patch,
        patchSummary: "新增 1 个文件",
        affectedPaths: ["note.md"],
      }),
      runEvent({ type: "AWAKEN_TURN_DIFF", seq: 2, diff, truncated: true }),
    ]);

    expect(state.patchPreviews).toEqual([
      {
        id: "patch-1",
        title: "Edit",
        patch,
        summary: "新增 1 个文件",
        affectedPaths: ["note.md"],
        truncated: undefined,
      },
    ]);
    expect(state.turnDiff).toEqual({ diff, truncated: true });
    expect(
      state.steps.find((step) => step.id === "patch:patch-1"),
    ).toMatchObject({
      title: "Patch preview updated",
      status: "active",
      tag: "patch",
    });
    expect(state.steps.find((step) => step.id === "turn-diff")).toMatchObject({
      title: "Turn diff captured",
      status: "done",
      tag: "truncated",
    });
  });

  it("projects AUTO tool decisions without pending approval controls", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "Inspect auto decisions" },
      runEvent({
        type: "AWAKEN_TOOL_PERMISSION_DECIDED",
        seq: 1,
        toolCallId: "read-1",
        toolCallName: "Read",
        toolDecisionAction: "allow",
        toolDecisionSource: "static_permission",
        toolDecisionReason: "read-only tool",
      }),
      runEvent({
        type: "AWAKEN_TOOL_PERMISSION_DECIDED",
        seq: 2,
        toolCallId: "patch-1",
        toolCallName: "Edit",
        toolDecisionAction: "deny",
        toolDecisionSource: "headless_default",
        toolDecisionReason: "writes require allowlist",
        affectedPaths: ["note.md"],
        patchSummary: "modify note.md",
      }),
    ]);

    expect(state.inspectorSummary.lastToolDecision).toMatchObject({
      toolCallId: "patch-1",
      toolName: "Edit",
      action: "deny",
      source: "headless_default",
      affectedPaths: ["note.md"],
      patchSummary: "modify note.md",
    });
    expect(
      state.toolResults.find((tool) => tool.id === "read-1"),
    ).toMatchObject({
      title: "Read",
      status: "done",
    });
    expect(
      state.toolResults.find((tool) => tool.id === "patch-1"),
    ).toMatchObject({
      title: "Edit",
      status: "error",
      content: expect.stringContaining("writes require allowlist"),
    });
    expect(
      state.steps.find((step) => step.id === "tool-decision:patch-1"),
    ).toMatchObject({
      title: "Edit deny",
      status: "error",
      tag: "headless_default",
    });
  });

  it("projects turn, context, compact, and terminal state for the inspector", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "Compact context" },
      runEvent({
        type: "AWAKEN_TURN_STARTED",
        seq: 1,
        turnId: "turn-1",
        turnCount: 1,
        transitionReason: "user_input",
      }),
      runEvent({
        type: "AWAKEN_CONTEXT_WARNING",
        seq: 2,
        message: "context warning",
        contextEffectiveWindow: 12000,
        contextEstimatedTokens: 9600,
        contextWarningThreshold: 9000,
      }),
      runEvent({
        type: "AWAKEN_COMPACT_FINISHED",
        seq: 3,
        message:
          "compact finished: auto boundary=compact_1 estimated_tokens=4800",
        contextCompactMode: "auto",
        contextCompactBoundaryId: "compact_1",
        contextCompactPreEstimateTokens: 9600,
        contextCompactPostEstimateTokens: 4800,
        contextEffectiveWindow: 12000,
        contextEstimatedTokens: 4800,
        contextEstimatedDeltaTokens: -4800,
        compactToolResultsPrunedCount: 2,
        compactPrunedSafeGroupCount: 1,
        compactPrunedMessageCount: 3,
        compactSummaryMaxTokens: 8192,
        compactSummaryRetryCount: 1,
        compactSummaryRawChars: 321,
        compactSummaryParsedSections: 13,
        compactSummaryMeaningfulSections: 0,
        compactSummaryQualityIssue:
          "semantic compact summary contained no meaningful section content despite non-empty source history",
        compactSummarySourceHadUser: true,
        compactSummaryLatestUserPinned: true,
        compactFallbackMarker: "[COMPACT_FALLBACK:llm_summary_failed]",
        compactMandatoryFloor: 3100,
        compactLocalProjected: 4400,
        compactConservativeProjected: 4800,
        compactContributionTokens: { system_prompt: 1200, conversation: 3200 },
        compactNativeAttempted: true,
        compactNativeUsed: false,
        compactNativeReason: "native_projection_fallback",
        compactFailureReason: "provider_timeout",
      }),
      runEvent({
        type: "AWAKEN_TURN_FINISHED",
        seq: 4,
        turnId: "turn-1",
        turnCount: 1,
        terminalReason: "final_response",
      }),
      runEvent({
        type: "RUN_FINISHED",
        seq: 5,
        result: "done",
        terminalReason: "completed",
        contextModelWindow: 12000,
        contextEstimatedTokens: 5200,
        contextActualTokens: 4310,
      }),
    ]);

    expect(state.inspectorSummary.currentTurn).toMatchObject({
      turnId: "turn-1",
      turnCount: 1,
      status: "done",
      terminalReason: "final_response",
    });
    expect(state.inspectorSummary.contextWindow).toMatchObject({
      eventType: "AWAKEN_CONTEXT_WARNING",
      estimatedTokens: 9600,
      effectiveWindow: 12000,
    });
    expect(state.inspectorSummary.compact).toMatchObject({
      status: "finished",
      mode: "auto",
      boundaryId: "compact_1",
      preCompactEstimatedTokens: 9600,
      postCompactEstimatedTokens: 4800,
      fallbackState: "structured",
      toolResultsPrunedCount: 2,
      prunedSafeGroupCount: 1,
      prunedMessageCount: 3,
      summaryMaxTokens: 8192,
      summaryRetryCount: 1,
      summaryRawChars: 321,
      summaryParsedSections: 13,
      summaryMeaningfulSections: 0,
      summaryQualityIssue:
        "semantic compact summary contained no meaningful section content despite non-empty source history",
      summarySourceHadUser: true,
      summaryLatestUserPinned: true,
      fallbackMarker: "[COMPACT_FALLBACK:llm_summary_failed]",
      mandatoryFloorTokens: 3100,
      localProjectedTokens: 4400,
      conservativeProjectedTokens: 4800,
      contributionTokens: { system_prompt: 1200, conversation: 3200 },
      nativeAttempted: true,
      nativeUsed: false,
      nativeReason: "native_projection_fallback",
      failureReason: "provider_timeout",
    });
    expect(state.inspectorSummary.terminalReason).toBe("completed");
    expect(state.inspectorSummary.terminalMessage).toBe("done");
    expect(
      state.steps.find((step) => step.id === "context-window"),
    ).toMatchObject({
      title: "Context window warning",
      status: "done",
    });
    expect(state.steps.find((step) => step.id === "compact")).toMatchObject({
      title: "Compact finished",
      status: "done",
      tag: "compact_1",
    });
    expect(
      state.narrative.committedItems.find(
        (item) => item.kind === "compactBoundary",
      ),
    ).toMatchObject({
      kind: "compactBoundary",
      status: "finished",
      title: "已自动压缩上下文",
      detail: expect.stringContaining("9.6k -> 4.8k"),
      boundaryId: "compact_1",
      preEstimate: 9600,
      postEstimate: 4800,
    });
  });

  it("keeps the current context estimate separate from provider usage", () => {
    const state = dispatch([
      runEvent({
        type: "AWAKEN_CONTEXT_SNAPSHOT",
        seq: 1,
        contextUsageSnapshot: {
          schemaVersion: 4,
          snapshotId: "ctx-1",
          estimatorId: "awaken_leaf_v4",
          basis: "sent_request",
          requestId: "req-1",
          modelWindowTokens: 256000,
          outputReserveTokens: 16000,
          effectiveInputWindowTokens: 240000,
          estimatedInputTokens: 92000,
          warningThresholdTokens: 204000,
          autoCompactThresholdTokens: 216000,
          compactTargetTokens: 72000,
          generatedAt: "2026-07-19T00:00:00Z",
          contributions: [
            { kind: "system_prompt", estimatedTokens: 2000, sourceCount: 1 },
            {
              kind: "conversation",
              estimatedTokens: 90000,
              sourceCount: 20,
            },
          ],
        },
      }),
      runEvent({
        type: "AWAKEN_MODEL_USAGE",
        seq: 2,
        modelUsage: {
          schemaVersion: 1,
          requestId: "req-1",
          purpose: "agent_turn",
          inputTokens: 88400,
          outputTokens: 2100,
          totalTokens: 90500,
          usageComplete: true,
          latencyMs: 30,
          status: "succeeded",
          completedAt: "2026-07-19T00:00:01Z",
        },
      }),
      runEvent({
        type: "AWAKEN_MODEL_USAGE",
        seq: 3,
        modelUsage: {
          schemaVersion: 1,
          requestId: "req-title",
          purpose: "thread_title",
          inputTokens: 200,
          outputTokens: 12,
          totalTokens: 212,
          usageComplete: true,
          latencyMs: 10,
          status: "succeeded",
          completedAt: "2026-07-19T00:00:02Z",
        },
      }),
    ]);

    expect(state.inspectorSummary.contextWindow).toMatchObject({
      estimatedTokens: 92000,
      actualTokens: undefined,
      modelWindow: 256000,
      outputReserve: 16000,
      effectiveWindow: 240000,
      snapshot: {
        snapshotId: "ctx-1",
        contributions: expect.arrayContaining([
          expect.objectContaining({
            kind: "conversation",
            estimatedTokens: 90000,
          }),
        ]),
      },
      latestModelUsage: {
        requestId: "req-1",
        inputTokens: 88400,
        totalTokens: 90500,
      },
      modelUsages: [
        expect.objectContaining({ requestId: "req-1", purpose: "agent_turn" }),
        expect.objectContaining({
          requestId: "req-title",
          purpose: "thread_title",
        }),
      ],
    });
  });

  it("keeps compact observability working from legacy fallback fields", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "Compact fallback" },
      runEvent({
        type: "AWAKEN_CONTEXT_WARNING",
        seq: 1,
        message: "context warning",
        contextEffectiveWindow: 12000,
        contextEstimatedTokens: 9600,
        contextWarningThreshold: 9000,
      }),
      runEvent({
        type: "AWAKEN_COMPACT_FINISHED",
        seq: 2,
        message:
          "compact finished: reactive boundary=compact_legacy estimated_tokens=4800",
        contextEffectiveWindow: 12000,
        contextEstimatedTokens: 4800,
        contextEstimatedDeltaTokens: -4800,
      }),
    ]);

    expect(state.inspectorSummary.compact).toMatchObject({
      status: "finished",
      mode: "reactive",
      boundaryId: "compact_legacy",
      preCompactEstimatedTokens: 9600,
      postCompactEstimatedTokens: 4800,
      fallbackState: "legacy",
    });
    expect(state.inspectorSummary.contextWindow).toMatchObject({
      eventType: "AWAKEN_CONTEXT_WARNING",
      estimatedTokens: 9600,
      effectiveWindow: 12000,
    });
    expect(
      state.narrative.committedItems.find(
        (item) => item.kind === "compactBoundary",
      ),
    ).toMatchObject({
      kind: "compactBoundary",
      status: "finished",
      title: "已压缩上下文",
      detail: expect.stringContaining("compact_legacy"),
      boundaryId: "compact_legacy",
      postEstimate: 4800,
    });
  });

  it("keeps the last complete context snapshot visible while compaction starts or fails", () => {
    const state = dispatch([
      runEvent({
        type: "AWAKEN_CONTEXT_SNAPSHOT",
        seq: 1,
        contextUsageSnapshot: {
          schemaVersion: 4,
          snapshotId: "ctx-before-compact",
          estimatorId: "awaken_leaf_v4",
          basis: "sent_request",
          requestId: "req-before-compact",
          modelWindowTokens: 256000,
          outputReserveTokens: 16000,
          effectiveInputWindowTokens: 240000,
          estimatedInputTokens: 92000,
          warningThresholdTokens: 204000,
          autoCompactThresholdTokens: 216000,
          compactTargetTokens: 72000,
          generatedAt: "2026-08-31T00:00:00Z",
          contributions: [
            {
              kind: "conversation",
              estimatedTokens: 92000,
              sourceCount: 20,
            },
          ],
        },
        modelUsage: {
          schemaVersion: 1,
          requestId: "req-before-compact",
          purpose: "agent_turn",
          inputTokens: 88400,
          outputTokens: 2100,
          totalTokens: 90500,
          usageComplete: true,
          latencyMs: 30,
          status: "succeeded",
          completedAt: "2026-08-31T00:00:01Z",
        },
      }),
      runEvent({
        type: "AWAKEN_COMPACT_STARTED",
        seq: 2,
        contextEstimatedTokens: 18100,
        contextCompactMode: "auto",
        contextCompactPreEstimateTokens: 92000,
        message: "compact started: auto",
      }),
      runEvent({
        type: "AWAKEN_COMPACT_FAILED",
        seq: 3,
        contextEstimatedTokens: 24000,
        contextCompactMode: "auto",
        compactFailureReason: "still_above_auto_threshold",
        message: "compact failed: auto",
      }),
      runEvent({
        type: "AWAKEN_CONTEXT_SNAPSHOT",
        seq: 4,
        contextUsageSnapshot: {
          schemaVersion: 4,
          snapshotId: "ctx-after-failed-compact",
          estimatorId: "awaken_leaf_v4",
          basis: "current_projection",
          modelWindowTokens: 256000,
          outputReserveTokens: 16000,
          effectiveInputWindowTokens: 240000,
          estimatedInputTokens: 24000,
          warningThresholdTokens: 204000,
          autoCompactThresholdTokens: 216000,
          compactTargetTokens: 24000,
          generatedAt: "2026-08-31T00:00:04Z",
          contributions: [
            {
              kind: "conversation",
              estimatedTokens: 24000,
              sourceCount: 5,
            },
          ],
        },
      }),
      runEvent({
        type: "RUN_FINISHED",
        seq: 5,
        contextEstimatedTokens: 23000,
        terminalReason: "completed",
      }),
    ]);

    expect(state.inspectorSummary.contextWindow).toMatchObject({
      eventType: "AWAKEN_CONTEXT_SNAPSHOT",
      estimatedTokens: 92000,
      snapshot: {
        snapshotId: "ctx-before-compact",
      },
      latestModelUsage: {
        requestId: "req-before-compact",
        inputTokens: 88400,
      },
    });
    expect(state.inspectorSummary.compact).toMatchObject({
      status: "failed",
      mode: "auto",
      preCompactEstimatedTokens: 92000,
      failureReason: "still_above_auto_threshold",
    });
  });

  it("waits for a complete post-compact snapshot before replacing visible context usage", () => {
    const beforePostSnapshot = dispatch([
      runEvent({
        type: "AWAKEN_CONTEXT_SNAPSHOT",
        seq: 1,
        contextUsageSnapshot: {
          schemaVersion: 4,
          snapshotId: "ctx-before-compact-finished",
          estimatorId: "awaken_leaf_v4",
          basis: "sent_request",
          requestId: "req-before-compact-finished",
          modelWindowTokens: 256000,
          outputReserveTokens: 16000,
          effectiveInputWindowTokens: 240000,
          estimatedInputTokens: 92000,
          warningThresholdTokens: 204000,
          autoCompactThresholdTokens: 216000,
          compactTargetTokens: 24000,
          generatedAt: "2026-08-31T00:00:00Z",
          contributions: [
            {
              kind: "conversation",
              estimatedTokens: 92000,
              sourceCount: 20,
            },
          ],
        },
      }),
      runEvent({
        type: "AWAKEN_COMPACT_STARTED",
        seq: 2,
        contextEstimatedTokens: 92000,
        contextCompactMode: "auto",
        contextCompactPreEstimateTokens: 92000,
      }),
      runEvent({
        type: "AWAKEN_COMPACT_FINISHED",
        seq: 3,
        contextEstimatedTokens: 18000,
        contextCompactMode: "auto",
        contextCompactPreEstimateTokens: 92000,
        contextCompactPostEstimateTokens: 18000,
        contextCompactBoundaryId: "compact-atomic-refresh",
      }),
      runEvent({
        type: "RUN_FINISHED",
        seq: 4,
        contextEstimatedTokens: 18000,
        contextActualTokens: 17600,
        terminalReason: "completed",
      }),
    ]);

    expect(beforePostSnapshot.inspectorSummary.contextWindow).toMatchObject({
      eventType: "AWAKEN_CONTEXT_SNAPSHOT",
      estimatedTokens: 92000,
      snapshot: { snapshotId: "ctx-before-compact-finished" },
    });
    expect(beforePostSnapshot.inspectorSummary.compact).toMatchObject({
      status: "finished",
      boundaryId: "compact-atomic-refresh",
      postCompactEstimatedTokens: 18000,
    });

    const afterPostSnapshot = dispatch([
      runEvent({
        type: "AWAKEN_CONTEXT_SNAPSHOT",
        seq: 1,
        contextUsageSnapshot: {
          schemaVersion: 4,
          snapshotId: "ctx-before-compact-finished",
          estimatorId: "awaken_leaf_v4",
          basis: "sent_request",
          requestId: "req-before-compact-finished",
          modelWindowTokens: 256000,
          outputReserveTokens: 16000,
          effectiveInputWindowTokens: 240000,
          estimatedInputTokens: 92000,
          warningThresholdTokens: 204000,
          autoCompactThresholdTokens: 216000,
          compactTargetTokens: 24000,
          generatedAt: "2026-08-31T00:00:00Z",
          contributions: [
            {
              kind: "conversation",
              estimatedTokens: 92000,
              sourceCount: 20,
            },
          ],
        },
      }),
      runEvent({
        type: "AWAKEN_COMPACT_FINISHED",
        seq: 2,
        contextEstimatedTokens: 18000,
        contextCompactMode: "auto",
        contextCompactPostEstimateTokens: 18000,
        contextCompactBoundaryId: "compact-atomic-refresh",
      }),
      runEvent({
        type: "AWAKEN_CONTEXT_SNAPSHOT",
        seq: 3,
        contextUsageSnapshot: {
          schemaVersion: 4,
          snapshotId: "ctx-after-compact-finished",
          estimatorId: "awaken_leaf_v4",
          basis: "current_projection",
          modelWindowTokens: 256000,
          outputReserveTokens: 16000,
          effectiveInputWindowTokens: 240000,
          estimatedInputTokens: 18000,
          warningThresholdTokens: 204000,
          autoCompactThresholdTokens: 216000,
          compactTargetTokens: 24000,
          generatedAt: "2026-08-31T00:00:03Z",
          contributions: [
            {
              kind: "conversation",
              estimatedTokens: 18000,
              sourceCount: 4,
            },
          ],
        },
      }),
    ]);

    expect(afterPostSnapshot.inspectorSummary.contextWindow).toMatchObject({
      eventType: "AWAKEN_CONTEXT_SNAPSHOT",
      estimatedTokens: 18000,
      snapshot: { snapshotId: "ctx-after-compact-finished" },
    });
  });

  it("reconstructs failed compact freezing from durable compact model usage", () => {
    const state = dispatch([
      runEvent({ type: "RUN_STARTED", seq: 1, runId: "run-before-failure" }),
      runEvent({
        type: "AWAKEN_CONTEXT_SNAPSHOT",
        seq: 2,
        contextUsageSnapshot: {
          schemaVersion: 4,
          snapshotId: "ctx-before-replayed-failure",
          estimatorId: "awaken_leaf_v4",
          basis: "sent_request",
          modelWindowTokens: 267307,
          outputReserveTokens: 0,
          effectiveInputWindowTokens: 267307,
          estimatedInputTokens: 147700,
          warningThresholdTokens: 227210,
          autoCompactThresholdTokens: 240576,
          compactTargetTokens: 26730,
          generatedAt: "2026-09-01T00:00:00Z",
          contributions: [
            {
              kind: "conversation",
              estimatedTokens: 130000,
              sourceCount: 40,
            },
          ],
        },
      }),
      runEvent({
        type: "AWAKEN_MODEL_USAGE",
        seq: 3,
        modelUsage: {
          schemaVersion: 1,
          requestId: "req-agent-before-failure",
          purpose: "agent_turn",
          inputTokens: 193000,
          outputTokens: 300,
          totalTokens: 193300,
          usageComplete: true,
          latencyMs: 10,
          status: "succeeded",
          completedAt: "2026-09-01T00:00:01Z",
        },
      }),
      runEvent({
        type: "AWAKEN_MODEL_USAGE",
        seq: 4,
        modelUsage: {
          schemaVersion: 1,
          requestId: "req-compact-summary",
          purpose: "compact_summary",
          inputTokens: 17404,
          outputTokens: 1000,
          totalTokens: 18404,
          usageComplete: true,
          latencyMs: 10,
          status: "succeeded",
          completedAt: "2026-09-01T00:00:02Z",
        },
      }),
      runEvent({
        type: "AWAKEN_CONTEXT_SNAPSHOT",
        seq: 5,
        contextUsageSnapshot: {
          schemaVersion: 4,
          snapshotId: "ctx-after-replayed-failure",
          estimatorId: "awaken_leaf_v4",
          basis: "sent_request",
          modelWindowTokens: 267307,
          outputReserveTokens: 0,
          effectiveInputWindowTokens: 267307,
          estimatedInputTokens: 205802,
          warningThresholdTokens: 227210,
          autoCompactThresholdTokens: 240576,
          compactTargetTokens: 26730,
          generatedAt: "2026-09-01T00:00:03Z",
          contributions: [
            {
              kind: "conversation",
              estimatedTokens: 188000,
              sourceCount: 48,
            },
          ],
        },
      }),
      runEvent({
        type: "RUN_ERROR",
        seq: 6,
        code: "PROVIDER_REQUEST_FAILED",
        message: "compact continuation failed",
      }),
    ]);

    expect(state.inspectorSummary.contextWindow).toMatchObject({
      snapshot: { snapshotId: "ctx-before-replayed-failure" },
      estimatedTokens: 147700,
      latestModelUsage: {
        requestId: "req-agent-before-failure",
        inputTokens: 193000,
      },
    });
    expect(
      state.inspectorSummary.awaitingContextRefreshAfterCompact,
    ).toBe(true);
  });

  it("accepts a hard-target post-compact snapshot during journal replay", () => {
    const state = dispatch([
      runEvent({ type: "RUN_STARTED", seq: 1, runId: "run-replay-success" }),
      runEvent({
        type: "AWAKEN_CONTEXT_SNAPSHOT",
        seq: 2,
        contextUsageSnapshot: {
          schemaVersion: 4,
          snapshotId: "ctx-before-replay-success",
          estimatorId: "awaken_leaf_v4",
          basis: "sent_request",
          modelWindowTokens: 267307,
          outputReserveTokens: 0,
          effectiveInputWindowTokens: 267307,
          estimatedInputTokens: 240000,
          warningThresholdTokens: 227210,
          autoCompactThresholdTokens: 240576,
          compactTargetTokens: 26730,
          generatedAt: "2026-09-01T00:00:00Z",
        },
      }),
      runEvent({
        type: "AWAKEN_MODEL_USAGE",
        seq: 3,
        modelUsage: {
          schemaVersion: 1,
          requestId: "req-replay-compact",
          purpose: "compact_summary",
          inputTokens: 17000,
          outputTokens: 1000,
          totalTokens: 18000,
          usageComplete: true,
          latencyMs: 10,
          status: "succeeded",
          completedAt: "2026-09-01T00:00:01Z",
        },
      }),
      runEvent({
        type: "AWAKEN_CONTEXT_SNAPSHOT",
        seq: 4,
        contextUsageSnapshot: {
          schemaVersion: 4,
          snapshotId: "ctx-after-replay-success",
          estimatorId: "awaken_leaf_v4",
          basis: "sent_request",
          modelWindowTokens: 267307,
          outputReserveTokens: 0,
          effectiveInputWindowTokens: 267307,
          estimatedInputTokens: 18872,
          warningThresholdTokens: 227210,
          autoCompactThresholdTokens: 240576,
          compactTargetTokens: 26730,
          generatedAt: "2026-09-01T00:00:02Z",
        },
      }),
    ]);

    expect(state.inspectorSummary.contextWindow).toMatchObject({
      snapshot: { snapshotId: "ctx-after-replay-success" },
      estimatedTokens: 18872,
    });
    expect(
      state.inspectorSummary.awaitingContextRefreshAfterCompact,
    ).toBe(false);
  });

  it("keeps unknown runtime events visible without failing projection", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "Future event" },
      runEvent({
        type: "AWAKEN_UNKNOWN_EVENT",
        seq: 1,
        rawType: "FutureEvent",
        message: "future payload",
        rawEvent: { type: "FutureEvent", futureField: "value" },
      }),
    ]);

    expect(state.phase).toBe("starting");
    expect(state.inspectorSummary.unknownEvents).toEqual([
      { rawType: "FutureEvent", message: "future payload" },
    ]);
    expect(state.steps.find((step) => step.id === "unknown:1")).toMatchObject({
      title: "FutureEvent",
      description: "future payload",
      status: "done",
      tag: "unknown",
    });
  });

  it("marks failed read-only tool actions and inspector records as error from toolStatus", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "Inspect failure" },
      runEvent({
        type: "TOOL_CALL_ARGS",
        seq: 1,
        toolCallId: "read-1",
        toolCallName: "Read",
        delta: '{"file_path":"missing.md"}',
      }),
      runEvent({
        type: "TOOL_CALL_RESULT",
        seq: 2,
        toolCallId: "read-1",
        toolCallName: "Read",
        content: "工具执行失败: missing.md",
        toolStatus: "failed",
      }),
    ]);

    expect(state.narrative.activeItem).toMatchObject({
      kind: "exploring",
      id: "exploring:current:read-1",
    });
    if (state.narrative.activeItem?.kind !== "exploring") {
      throw new Error("expected exploring item");
    }
    expect(state.narrative.activeItem.actions[0]).toMatchObject({
      id: "explore:read-1",
      status: "error",
      toolStatus: "failed",
    });
    expect(state.toolResults[0]).toMatchObject({
      id: "read-1",
      status: "error",
      toolStatus: "failed",
    });
  });

  it("does not infer tool error from Chinese content when toolStatus is succeeded", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "Inspect success" },
      runEvent({
        type: "TOOL_CALL_RESULT",
        seq: 1,
        toolCallId: "shell-1",
        toolCallName: "shell",
        content: "工具执行失败: quoted in stdout",
        toolStatus: "succeeded",
      }),
    ]);

    expect(state.toolResults[0]).toMatchObject({
      id: "shell-1",
      status: "done",
      toolStatus: "succeeded",
    });
  });

  it("treats empty successful tool content as done when toolStatus is succeeded", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "Empty ok" },
      runEvent({
        type: "TOOL_CALL_RESULT",
        seq: 1,
        toolCallId: "tool-1",
        toolCallName: "run_command",
        content: "",
        toolStatus: "succeeded",
      }),
    ]);

    expect(state.toolResults[0]).toMatchObject({
      id: "tool-1",
      status: "done",
      toolStatus: "succeeded",
    });
  });

  it("records tool recovery required and retry blocked inspector steps", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "Recover" },
      runEvent({
        type: "AWAKEN_TOOL_RECOVERY_REQUIRED",
        seq: 1,
        toolCallId: "call-1",
        toolCallName: "apply_patch",
        errorCode: "patch_context_mismatch",
        message: "recovery_required",
        recoveryOptions: ["read_file", "use_text_edit"],
      }),
      runEvent({
        type: "AWAKEN_TOOL_RETRY_BLOCKED",
        seq: 2,
        toolCallId: "call-2",
        toolCallName: "apply_patch",
        errorCode: "tool_retry_blocked",
        message: "blocked",
        recoveryOptions: ["change_strategy"],
      }),
    ]);

    const recovery = state.steps.find((step) => step.tag === "recovery");
    const blocked = state.steps.find((step) => step.tag === "retry-blocked");
    expect(recovery).toMatchObject({
      title: "Tool recovery required",
      status: "error",
    });
    expect(blocked).toMatchObject({
      title: "Tool retry blocked",
      status: "error",
    });
  });

  it("projects durable Skill lifecycle events into stable activity steps", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "Build a deck" },
      runEvent({
        type: "AWAKEN_SKILL_INVOKED",
        seq: 1,
        skillId: "presentations",
        skillVersion: "2",
        skillStatus: "ok",
      }),
      runEvent({
        type: "AWAKEN_SKILL_EXECUTION_CONTEXT",
        seq: 2,
        skillId: "presentations",
        skillEffectiveTools: ["presentation_create", "shell"],
      }),
      runEvent({
        type: "AWAKEN_SKILL_CONTENT_LOADED",
        seq: 3,
        skillId: "presentations",
        skillResource: "references/layout.md",
      }),
    ]);

    expect(state.steps).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          id: "skill:presentations",
          title: "Skill execution context ready",
          status: "done",
          tag: "skill",
        }),
        expect.objectContaining({
          id: "skill:presentations:resource:references/layout.md",
          title: "Skill content loaded",
          status: "done",
          tag: "skill",
        }),
      ]),
    );
  });

  it("keeps failed Skill loading as an error activity", () => {
    const state = dispatch([
      runEvent({
        type: "AWAKEN_SKILL_FAILED_TO_LOAD",
        seq: 1,
        skillId: "broken-skill",
        skillDiagnostics: ["parse_failed: frontmatter is invalid"],
      }),
    ]);

    expect(state.steps).toContainEqual(
      expect.objectContaining({
        id: "skill:broken-skill",
        title: "Skill failed to load",
        description: expect.stringContaining("frontmatter is invalid"),
        status: "error",
      }),
    );
  });

  it("points inspector selection to tool details when a tool result arrives", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "Inspect tool" },
      runEvent({
        type: "TOOL_CALL_RESULT",
        seq: 1,
        toolCallId: "tool-1",
        toolCallName: "run_command",
        content: "command output",
      }),
    ]);

    expect(state.narrative.inspectorSelection).toEqual({
      kind: "tool",
      id: "tool:tool-1",
    });
  });

  it("points inspector selection to read-only tool details during exploring", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "Inspect read" },
      runEvent({
        type: "TOOL_CALL_ARGS",
        seq: 1,
        toolCallId: "read-1",
        toolCallName: "read_file",
        delta: '{"path":"README.md"}',
      }),
    ]);

    expect(state.narrative.inspectorSelection).toEqual({
      kind: "tool",
      id: "tool:read-1",
    });
  });

  it("renders final stream failure as terminal error instead of reconnecting", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "Patch a file" },
      runEvent({
        type: "AWAKEN_ARTIFACT_CREATED",
        seq: 2,
        artifactId: "stream-report",
        artifactTitle: "中断前报告.pdf",
        artifactKind: "pdf",
        artifactPath: "output/中断前报告.pdf",
        artifactStatus: "ready",
        artifactRole: "deliverable",
        artifactContractVersion: 2,
        artifactProducer: "artifact_publish",
      }),
      {
        type: "run/streamFailed",
        message: "failed after 5 reconnect attempts",
      },
    ]);

    expect(state.phase).toBe("error");
    expect(state.status).toBe("error");
    expect(state.timelineStatus).toBe("error");
    expect(state.errorMessage).toBe("failed after 5 reconnect attempts");
    expect(
      state.steps.find((step) => step.id === "stream-failed"),
    ).toMatchObject({
      title: "Stream failed",
      status: "error",
    });
    expect(
      state.narrative.committedItems.some(
        (item) => item.id === "answer:error:current:stream",
      ),
    ).toBe(true);
    expect(
      state.narrative.committedItems.find(
        (item) => item.kind === "deliverables",
      ),
    ).toMatchObject({
      terminalStatus: "error",
      files: [{ artifactId: "stream-report" }],
    });
  });

  it("keeps support artifacts for inspection without rendering a final file list", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "Patch a file" },
      runEvent({
        type: "AWAKEN_ARTIFACT_CREATED",
        seq: 1,
        artifactId: "artifact-1",
        artifactTitle: "example.ts",
        artifactKind: "file",
        artifactPath: "src/example.ts",
        artifactDescription: "Updated file",
        artifactStatus: "ready",
      }),
      runEvent({
        type: "TEXT_MESSAGE_CONTENT",
        seq: 2,
        messageId: "m1",
        delta: "done",
      }),
      runEvent({ type: "RUN_FINISHED", seq: 3, result: "done" }),
    ]);

    expect(state.artifacts).toContainEqual({
      id: "artifact-1",
      title: "example.ts",
      description: "Updated file",
      kind: "file",
      path: "src/example.ts",
      content: undefined,
      status: "ready",
      role: "support",
      contractVersion: 1,
      producerTool: undefined,
    });
    expect(
      state.narrative.committedItems.some(
        (item) => item.kind === "fileChange" || item.kind === "deliverables",
      ),
    ).toBe(false);
  });

  it("does not publish deliverables with missing or unknown status", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "Create reports" },
      runEvent({
        type: "AWAKEN_ARTIFACT_CREATED",
        seq: 1,
        artifactId: "missing-status",
        artifactTitle: "missing.pdf",
        artifactKind: "pdf",
        artifactPath: "output/missing.pdf",
        artifactRole: "deliverable",
        artifactContractVersion: 2,
        artifactProducer: "artifact_publish",
      }),
      runEvent({
        type: "AWAKEN_ARTIFACT_CREATED",
        seq: 2,
        artifactId: "unknown-status",
        artifactTitle: "unknown.pdf",
        artifactKind: "pdf",
        artifactPath: "output/unknown.pdf",
        artifactStatus: "complete" as never,
        artifactRole: "deliverable",
        artifactContractVersion: 2,
        artifactProducer: "artifact_publish",
      }),
      runEvent({ type: "RUN_FINISHED", seq: 3, result: "done" }),
    ]);

    expect(state.artifacts.map((artifact) => artifact.status)).toEqual([
      undefined,
      undefined,
    ]);
    expect(
      state.narrative.committedItems.some(
        (item) => item.kind === "deliverables",
      ),
    ).toBe(false);
  });

  it("renders ready deliverables after the final answer and hides previews", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "Create a report" },
      runEvent({
        type: "AWAKEN_ARTIFACT_CREATED",
        seq: 1,
        artifactId: "report",
        artifactTitle: "风险报告.docx",
        artifactKind: "document",
        artifactPath: "output/风险报告.docx",
        artifactStatus: "ready",
        artifactRole: "deliverable",
        artifactContractVersion: 2,
        artifactProducer: "document_create",
      }),
      runEvent({
        type: "AWAKEN_ARTIFACT_CREATED",
        seq: 2,
        artifactId: "preview",
        artifactTitle: "page-01.png",
        artifactKind: "image",
        artifactPath: "output/preview/page-01.png",
        artifactStatus: "ready",
        artifactRole: "preview",
        artifactContractVersion: 2,
        artifactProducer: "artifact_render",
      }),
      runEvent({
        type: "TEXT_MESSAGE_CONTENT",
        seq: 3,
        messageId: "m1",
        delta: "done",
      }),
      runEvent({ type: "RUN_FINISHED", seq: 4, result: "done" }),
    ]);

    const answerIndex = state.narrative.committedItems.findIndex(
      (item) => item.kind === "answer",
    );
    const deliverableIndex = state.narrative.committedItems.findIndex(
      (item) => item.kind === "deliverables",
    );
    expect(deliverableIndex).toBeGreaterThan(answerIndex);
    expect(state.narrative.committedItems[deliverableIndex]).toMatchObject({
      kind: "deliverables",
      id: "deliverables:current:summary",
      terminalStatus: "finished",
      files: [
        {
          artifactId: "report",
          title: "风险报告.docx",
          extension: "docx",
          fileType: "word",
          legacy: false,
        },
      ],
    });
  });

  it("projects ready image and video artifacts into final deliverables", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "Create media" },
      runEvent({
        type: "AWAKEN_ARTIFACT_CREATED",
        seq: 1,
        artifactId: "image-1",
        artifactTitle: "主视觉.png",
        artifactKind: "image",
        artifactPath: "output/generated-media/image.png",
        artifactStatus: "ready",
        artifactRole: "deliverable",
        artifactContractVersion: 2,
        artifactProducer: "image_generate",
      }),
      runEvent({
        type: "AWAKEN_ARTIFACT_CREATED",
        seq: 2,
        artifactId: "video-1",
        artifactTitle: "成片.mp4",
        artifactKind: "video",
        artifactPath: "output/generated-media/video.mp4",
        artifactStatus: "ready",
        artifactRole: "deliverable",
        artifactContractVersion: 2,
        artifactProducer: "video_generate",
      }),
      runEvent({ type: "RUN_FINISHED", seq: 3, result: "done" }),
    ]);

    expect(
      state.narrative.committedItems.find(
        (item) => item.kind === "deliverables",
      ),
    ).toMatchObject({
      files: [
        { artifactId: "image-1", fileType: "image", extension: "png" },
        { artifactId: "video-1", fileType: "video", extension: "mp4" },
      ],
    });
  });

  it("places a retried group image back at its original visible position", () => {
    const imageEvent = (
      seq: number,
      artifactId: string,
      artifactGroupIndex: number,
    ): RuntimeAction =>
      runEvent({
        type: "AWAKEN_ARTIFACT_CREATED",
        seq,
        artifactId,
        artifactTitle: `${artifactId}.png`,
        artifactKind: "image",
        artifactPath: `output/generated-media/${artifactId}.png`,
        artifactStatus: "ready",
        artifactRole: "deliverable",
        artifactContractVersion: 2,
        artifactProducer: "image_generate",
        artifactGroupId: "group-5",
        artifactGroupIndex,
      });

    const state = dispatch([
      { type: "run/requested", prompt: "Generate five images" },
      imageEvent(1, "image-1", 1),
      imageEvent(2, "image-2", 2),
      imageEvent(3, "image-4", 4),
      imageEvent(4, "image-5", 5),
      imageEvent(5, "image-3-retry", 3),
      runEvent({ type: "RUN_FINISHED", seq: 6, result: "done" }),
    ]);

    const deliverables = state.narrative.committedItems.find(
      (item) => item.kind === "deliverables",
    );
    expect(deliverables).toMatchObject({
      files: [
        { artifactId: "image-1" },
        { artifactId: "image-2" },
        { artifactId: "image-3-retry" },
        { artifactId: "image-4" },
        { artifactId: "image-5" },
      ],
    });
  });

  it("renders every created or modified document and excludes deleted documents", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "Create and update reports" },
      runEvent({
        type: "AWAKEN_ARTIFACT_CREATED",
        seq: 1,
        artifactId: "apply_patch:added:new.md",
        artifactTitle: "new.md",
        artifactKind: "added",
        artifactPath: "output/new.md",
        artifactStatus: "ready",
        artifactRole: "deliverable",
        artifactContractVersion: 2,
        artifactProducer: "apply_patch",
      }),
      runEvent({
        type: "AWAKEN_ARTIFACT_UPDATED",
        seq: 2,
        artifactId: "apply_patch:modified:existing.md",
        artifactTitle: "existing.md",
        artifactKind: "modified",
        artifactPath: "output/existing.md",
        artifactStatus: "ready",
        artifactRole: "deliverable",
        artifactContractVersion: 2,
        artifactProducer: "apply_patch",
      }),
      runEvent({
        type: "AWAKEN_ARTIFACT_UPDATED",
        seq: 3,
        artifactId: "apply_patch:added:removed.md",
        artifactTitle: "removed.md",
        artifactKind: "added",
        artifactPath: "output/removed.md",
        artifactStatus: "ready",
        artifactRole: "deliverable",
        artifactContractVersion: 2,
        artifactProducer: "apply_patch",
      }),
      runEvent({
        type: "AWAKEN_ARTIFACT_UPDATED",
        seq: 4,
        artifactId: "apply_patch:deleted:removed.md",
        artifactTitle: "removed.md",
        artifactKind: "deleted",
        artifactPath: "output/removed.md",
        artifactStatus: "ready",
        artifactRole: "support",
        artifactContractVersion: 2,
        artifactProducer: "apply_patch",
      }),
      runEvent({
        type: "AWAKEN_ARTIFACT_CREATED",
        seq: 5,
        artifactId: "office_export_pdf:report.pdf",
        artifactTitle: "report.pdf",
        artifactKind: "pdf",
        artifactPath: "output/report.pdf",
        artifactStatus: "ready",
        artifactRole: "deliverable",
        artifactContractVersion: 2,
        artifactProducer: "office_export_pdf",
      }),
      runEvent({ type: "RUN_FINISHED", seq: 6, result: "done" }),
    ]);

    expect(
      state.narrative.committedItems.find(
        (item) => item.kind === "deliverables",
      ),
    ).toMatchObject({
      files: [
        { artifactId: "apply_patch:added:new.md" },
        { artifactId: "apply_patch:modified:existing.md" },
        { artifactId: "office_export_pdf:report.pdf" },
      ],
    });
  });

  it.each([
    {
      name: "failed",
      terminal: runEvent({
        type: "RUN_ERROR",
        seq: 2,
        code: "TOOL_FAILED",
        message: "failed",
      }),
      expected: "error" as const,
    },
    {
      name: "cancelled",
      terminal: runEvent({
        type: "RUN_ERROR",
        seq: 2,
        code: "RUN_CANCELLED",
        message: "cancelled",
      }),
      expected: "cancelled" as const,
    },
  ])(
    "keeps ready deliverables when a run is $name",
    ({ terminal, expected }) => {
    const state = dispatch([
      { type: "run/requested", prompt: "Create a report" },
      runEvent({
        type: "AWAKEN_ARTIFACT_CREATED",
        seq: 1,
        artifactId: "report",
        artifactTitle: "报告.pdf",
        artifactKind: "pdf",
        artifactPath: "output/报告.pdf",
        artifactStatus: "ready",
        artifactRole: "deliverable",
        artifactContractVersion: 2,
        artifactProducer: "artifact_export",
      }),
      terminal,
    ]);
    expect(
        state.narrative.committedItems.find(
          (item) => item.kind === "deliverables",
        ),
      ).toMatchObject({
        terminalStatus: expected,
        files: [{ artifactId: "report" }],
  });
    },
  );

  it("does not duplicate narrative items or deltas when sequenced events replay", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "Replay safe" },
      runEvent({
        type: "TEXT_MESSAGE_CONTENT",
        seq: 1,
        messageId: "m1",
        delta: "he",
      }),
      runEvent({
        type: "TEXT_MESSAGE_CONTENT",
        seq: 2,
        messageId: "m1",
        delta: "llo",
      }),
      runEvent({
        type: "TEXT_MESSAGE_CONTENT",
        seq: 1,
        messageId: "m1",
        delta: "he",
      }),
      runEvent({
        type: "TEXT_MESSAGE_CONTENT",
        seq: 2,
        messageId: "m1",
        delta: "llo",
      }),
    ]);

    const ids = [
      ...state.narrative.committedItems.map((item) => item.id),
      ...(state.narrative.activeItem ? [state.narrative.activeItem.id] : []),
    ];
    expect(new Set(ids).size).toBe(ids.length);
    expect(state.narrative.activeItem).toMatchObject({
      kind: "answer",
      id: "answer:m1",
      markdown: "hello",
      status: "active",
    });
  });

  it("does not double assistantText when a durable snapshot follows streaming", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "Snapshot" },
      runEvent({
        type: "TEXT_MESSAGE_CONTENT",
        seq: 16,
        messageId: "m1",
        delta: "hello",
      }),
      runEvent({
        type: "TEXT_MESSAGE_CONTENT",
        seq: 32,
        semanticSeq: 64,
        recordId: "rec_assistant_1",
        messageId: "m1",
        delta: "hello",
      }),
    ]);

    expect(state.assistantText).toBe("hello");
    expect(state.narrative.committedItems).toContainEqual({
      kind: "answer",
      id: "answer:m1",
      markdown: "hello",
      status: "done",
    });
    expect(state.narrative.activeItem).toBeUndefined();
  });

  it("preserves repeated unsequenced text deltas", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "Repeat token" },
      runEvent({ type: "TEXT_MESSAGE_CONTENT", messageId: "m1", delta: "ha" }),
      runEvent({ type: "TEXT_MESSAGE_CONTENT", messageId: "m1", delta: "ha" }),
    ]);

    expect(state.narrative.activeItem).toMatchObject({
      kind: "answer",
      id: "answer:m1",
      markdown: "haha",
      status: "active",
    });
  });

  it("commits active narrative item when run finishes", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "Finish" },
      runEvent({
        type: "TEXT_MESSAGE_CONTENT",
        seq: 1,
        messageId: "m1",
        delta: "done",
      }),
      runEvent({ type: "RUN_FINISHED", seq: 2, result: "done" }),
    ]);

    expect(state.narrative.activeItem).toBeUndefined();
    expect(state.narrative.committedItems).toContainEqual({
      kind: "answer",
      id: "answer:m1",
      markdown: "done",
      status: "done",
      isFinal: true,
    });
  });

  it("annotates the successful final answer with elapsed runtime", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "Finish", startedAtMs: 1000 },
      {
        type: "run/created",
        run: makeRun({ runId: "run-1", threadId: "thread-1" }),
      },
      runEvent({
        type: "TEXT_MESSAGE_CONTENT",
        runId: "run-1",
        messageId: "m1",
        delta: "done",
      }),
      runEventAt(
        { type: "RUN_FINISHED", runId: "run-1", result: "done" },
        66000,
      ),
    ]);

    expect(state.narrative.committedItems).toContainEqual({
      kind: "answer",
      id: "answer:run-1:m1",
      markdown: "done",
      status: "done",
      isFinal: true,
      elapsedMs: 65000,
    });
  });

  it("uses durable event timestamps instead of client request and delivery time", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "Finish", startedAtMs: 1000 },
      {
        type: "run/created",
        run: makeRun({ runId: "run-1", threadId: "thread-1" }),
      },
      runEventAt(
        {
          type: "RUN_STARTED",
          runId: "run-1",
          threadId: "thread-1",
          timestamp: 5000,
        },
        9000,
      ),
      runEvent({
        type: "TEXT_MESSAGE_CONTENT",
        runId: "run-1",
        messageId: "m1",
        delta: "done",
      }),
      runEventAt(
        {
          type: "RUN_FINISHED",
          runId: "run-1",
          result: "done",
          timestamp: 48000,
        },
        53000,
      ),
    ]);

    expect(state.narrative.committedItems).toContainEqual({
      kind: "answer",
      id: "answer:run-1:m1",
      markdown: "done",
      status: "done",
      isFinal: true,
      elapsedMs: 43000,
    });
  });

  it("annotates an already committed text message when finished status arrives", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "Finish", startedAtMs: 2000 },
      {
        type: "run/created",
        run: makeRun({ runId: "run-1", threadId: "thread-1" }),
      },
      runEvent({
        type: "TEXT_MESSAGE_CONTENT",
        runId: "run-1",
        messageId: "m1",
        delta: "done",
      }),
      runEvent({ type: "TEXT_MESSAGE_END", runId: "run-1", messageId: "m1" }),
      runEventAt(
        { type: "RUN_STATUS_CHANGED", runId: "run-1", status: "finished" },
        47000,
      ),
    ]);

    expect(state.narrative.committedItems).toContainEqual({
      kind: "answer",
      id: "answer:run-1:m1",
      markdown: "done",
      status: "done",
      isFinal: true,
      elapsedMs: 45000,
    });
  });

  it("commits a streaming answer and annotates elapsed runtime when finished status arrives", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "Finish", startedAtMs: 2000 },
      {
        type: "run/created",
        run: makeRun({ runId: "run-1", threadId: "thread-1" }),
      },
      runEvent({
        type: "TEXT_MESSAGE_CONTENT",
        runId: "run-1",
        messageId: "m1",
        delta: "streamed done",
      }),
      runEventAt(
        { type: "RUN_STATUS_CHANGED", runId: "run-1", status: "finished" },
        47000,
      ),
    ]);

    expect(state.narrative.committedItems).toContainEqual({
      kind: "answer",
      id: "answer:run-1:m1",
      markdown: "streamed done",
      status: "done",
      isFinal: true,
      elapsedMs: 45000,
    });
  });

  it("does not annotate cancelled or error answers with elapsed runtime", () => {
    const cancelled = dispatch([
      { type: "run/requested", prompt: "Cancel", startedAtMs: 1000 },
      runEvent({
        type: "TEXT_MESSAGE_CONTENT",
        messageId: "m1",
        delta: "partial",
      }),
      runEventAt({ type: "RUN_STATUS_CHANGED", status: "cancelled" }, 3000),
    ]);
    const errored = dispatch([
      { type: "run/requested", prompt: "Error", startedAtMs: 1000 },
      runEventAt(
        { type: "RUN_ERROR", code: "INTERNAL_ERROR", message: "boom" },
        3000,
      ),
    ]);

    const cancelledAnswer = cancelled.narrative.committedItems.find(
      (item) => item.kind === "answer",
    );
    const errorAnswer = errored.narrative.committedItems.find(
      (item) => item.kind === "answer",
    );

    expect(cancelledAnswer).not.toHaveProperty("elapsedMs");
    expect(errorAnswer).not.toHaveProperty("elapsedMs");
  });

  it("projects provider errors into sanitized error answer items", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "Needs provider" },
      runEvent({
        type: "RUN_ERROR",
        seq: 1,
        code: "PROVIDER_AUTH_MISSING",
        message: "missing API key sk-test",
      }),
    ]);

    expect(state.narrative.activeItem).toBeUndefined();
    expect(state.narrative.committedItems).toContainEqual({
      kind: "answer",
      id: "answer:error:1",
      markdown: "API key 缺失，请在 Settings > Models 配置模型 API key",
      status: "error",
      isFinal: true,
    });
    expect(
      state.narrative.committedItems.some(
        (item) => item.kind === "answer" && item.markdown.includes("sk-test"),
      ),
    ).toBe(false);
  });

  it("keeps active answer available for live tail after stream interruption", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "Stream" },
      runEvent({
        type: "TEXT_MESSAGE_CONTENT",
        seq: 1,
        messageId: "m1",
        delta: "partial",
      }),
      { type: "run/reconnecting" },
      { type: "run/streamError", message: "stream disconnected" },
      runEvent({
        type: "TEXT_MESSAGE_CONTENT",
        seq: 2,
        messageId: "m1",
        delta: " recovered",
      }),
    ]);

    expect(state.phase).toBe("reconnecting");
    expect(state.errorMessage).toBeUndefined();
    expect(state.narrative.activeItem).toMatchObject({
      kind: "answer",
      id: "answer:m1",
      markdown: "partial recovered",
      status: "active",
    });
    expect(
      state.narrative.committedItems.some(
        (item) => item.id === "answer:error:stream",
      ),
    ).toBe(false);
  });
});

describe("runtime reducer thread snapshots", () => {
  it("tracks sidebar run status across run lifecycle", () => {
    let state = createInitialRuntimeState();
    state = reduceRuntimeState(state, {
      type: "run/requested",
      prompt: "update file",
    });
    state = reduceRuntimeState(state, {
      type: "run/created",
      run: makeRun({ runId: "run-1", threadId: "thread-1" }),
    });
    state = reduceRuntimeState(state, {
      type: "run/threadAdded",
      threadId: "thread-1",
      label: "update file",
      runId: "run-1",
    });

    expect(state.threads[0]).toMatchObject({
      threadId: "thread-1",
      runStatus: "running",
      activeRunId: "run-1",
      artifactCount: 0,
    });

    state = reduceRuntimeState(
      state,
      runEvent({ type: "RUN_STATUS_CHANGED", status: "running" }),
    );
    state = reduceRuntimeState(
      state,
      runEvent({
      type: "AWAKEN_ARTIFACT_CREATED",
      artifactId: "artifact-1",
      artifactTitle: "example.ts",
      artifactKind: "file",
      artifactPath: "src/example.ts",
      }),
    );

    expect(state.threads[0]).toMatchObject({
      artifactCount: 1,
    });

    state = reduceRuntimeState(
      state,
      runEvent({ type: "RUN_FINISHED", result: "done" }),
    );

    expect(state.threads[0]).toMatchObject({
      runStatus: "finished",
      activeRunId: undefined,
      artifactCount: 1,
    });
  });

  it("restores the selected thread view when switching conversations", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "first prompt" },
      {
        type: "run/created",
        run: makeRun({ runId: "run-1", threadId: "thread-1" }),
      },
      {
        type: "run/threadAdded",
        threadId: "thread-1",
        label: "first prompt",
        runId: "run-1",
      },
      runEvent({
        type: "TEXT_MESSAGE_CONTENT",
        threadId: "thread-1",
        runId: "run-1",
        messageId: "msg-1",
        delta: "first answer",
      }),
      { type: "run/reset" },
      { type: "run/requested", prompt: "second prompt" },
      {
        type: "run/created",
        run: makeRun({ runId: "run-2", threadId: "thread-2" }),
      },
      {
        type: "run/threadAdded",
        threadId: "thread-2",
        label: "second prompt",
        runId: "run-2",
      },
      runEvent({
        type: "TEXT_MESSAGE_CONTENT",
        threadId: "thread-2",
        runId: "run-2",
        messageId: "msg-2",
        delta: "second answer",
      }),
      { type: "run/threadSwitch", threadId: "thread-1" },
    ]);

    expect(state.activeThreadId).toBe("thread-1");
    expect(state.runId).toBe("run-1");
    expect(state.assistantText).toBe("first answer");
    const narrativeItems = [
      ...state.narrative.committedItems,
      ...(state.narrative.activeItem ? [state.narrative.activeItem] : []),
    ];
    expect(narrativeItems.some((item) => item.kind === "answer")).toBe(true);
    expect(narrativeItems).toContainEqual({
      kind: "prompt",
      id: "prompt:draft:1",
      runId: "run-1",
      text: "first prompt",
    });
    expect(
      narrativeItems.some(
        (item) => item.kind === "prompt" && item.text === "second prompt",
      ),
    ).toBe(false);
    expect(state.threads.map((thread) => thread.threadId)).toEqual([
      "thread-1",
      "thread-2",
    ]);
  });

  it("restores artifact state when switching threads", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "first prompt" },
      {
        type: "run/created",
        run: makeRun({ runId: "run-1", threadId: "thread-1" }),
      },
      {
        type: "run/threadAdded",
        threadId: "thread-1",
        label: "first prompt",
        runId: "run-1",
      },
      runEvent({
        type: "AWAKEN_ARTIFACT_CREATED",
        artifactId: "artifact-1",
        artifactTitle: "example.ts",
        artifactKind: "file",
        artifactPath: "src/example.ts",
      }),
      { type: "run/reset" },
      { type: "run/requested", prompt: "second prompt" },
      {
        type: "run/created",
        run: makeRun({ runId: "run-2", threadId: "thread-2" }),
      },
      {
        type: "run/threadAdded",
        threadId: "thread-2",
        label: "second prompt",
        runId: "run-2",
      },
      runEvent({
        type: "TEXT_MESSAGE_CONTENT",
        threadId: "thread-2",
        runId: "run-2",
        messageId: "msg-2",
        delta: "second answer",
      }),
      { type: "run/threadSwitch", threadId: "thread-1" },
    ]);

    const narrativeItems = [
      ...state.narrative.committedItems,
      ...(state.narrative.activeItem ? [state.narrative.activeItem] : []),
    ];

    expect(state.activeThreadId).toBe("thread-1");
    expect(state.artifacts).toHaveLength(1);
    expect(narrativeItems.some((item) => item.kind === "fileChange")).toBe(
      false,
    );
  });

  it("starts a follow-up run in the same active thread after completion", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "first prompt" },
      {
        type: "run/created",
        run: makeRun({ runId: "run-1", threadId: "thread-1" }),
      },
      {
        type: "run/threadAdded",
        threadId: "thread-1",
        label: "first prompt",
        runId: "run-1",
      },
      runEvent({
        type: "TEXT_MESSAGE_CONTENT",
        threadId: "thread-1",
        runId: "run-1",
        messageId: "msg-1",
        delta: "first answer",
      }),
      runEvent({ type: "RUN_FINISHED", result: "first done" }),
      { type: "run/requested", prompt: "follow up prompt" },
      {
        type: "run/created",
        run: makeRun({ runId: "run-2", threadId: "thread-1" }),
      },
      {
        type: "run/threadAdded",
        threadId: "thread-1",
        label: "follow up prompt",
        runId: "run-2",
      },
    ]);

    expect(state.activeThreadId).toBe("thread-1");
    expect(state.threadId).toBe("thread-1");
    expect(state.runId).toBe("run-2");
    expect(state.threads).toHaveLength(1);
    expect(state.threads[0]).toMatchObject({
      threadId: "thread-1",
      runId: "run-2",
      runStatus: "running",
      activeRunId: "run-2",
    });
    const prompts = state.narrative.committedItems.filter(
      (item) => item.kind === "prompt",
    );
    expect(prompts).toEqual([
      { kind: "prompt", id: "prompt:draft:1", runId: "run-1", text: "first prompt" },
      { kind: "prompt", id: "prompt:thread-1:2", runId: "run-2", text: "follow up prompt" },
    ]);
    expect(new Set(prompts.map((item) => item.id)).size).toBe(2);
    expect(state.narrative.committedItems).toContainEqual({
      kind: "answer",
      id: "answer:run-1:msg-1",
      markdown: "first answer",
      status: "done",
      isFinal: true,
    });
    expect(state.narrative.activeItem).toEqual({
      kind: "planning",
      id: "planning:thread-1:2",
      text: "Planning next moves",
      status: "active",
    });
  });

  it("uses run-scoped fallback ids for consecutive answers without message ids", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "first prompt" },
      {
        type: "run/created",
        run: makeRun({ runId: "run-1", threadId: "thread-1" }),
      },
      {
        type: "run/threadAdded",
        threadId: "thread-1",
        label: "first prompt",
        runId: "run-1",
      },
      runEvent({ type: "TEXT_MESSAGE_CONTENT", delta: "first answer" }),
      runEvent({ type: "RUN_FINISHED", result: "first done" }),
      { type: "run/requested", prompt: "follow up prompt" },
      {
        type: "run/created",
        run: makeRun({ runId: "run-2", threadId: "thread-1" }),
      },
      {
        type: "run/threadAdded",
        threadId: "thread-1",
        label: "follow up prompt",
        runId: "run-2",
      },
      runEvent({ type: "TEXT_MESSAGE_CONTENT", delta: "second answer" }),
      runEvent({ type: "RUN_FINISHED", result: "second done" }),
    ]);

    const answers = state.narrative.committedItems.filter(
      (item) => item.kind === "answer",
    );
    expect(answers).toEqual([
      {
        kind: "answer",
        id: "answer:run-1",
        markdown: "first answer",
        status: "done",
        isFinal: true,
      },
      {
        kind: "answer",
        id: "answer:run-2",
        markdown: "second answer",
        status: "done",
        isFinal: true,
      },
    ]);
  });

  it("loads runtime thread list without losing saved thread views", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "first prompt" },
      {
        type: "run/created",
        run: makeRun({ runId: "run-1", threadId: "thread-1" }),
      },
      {
        type: "run/threadAdded",
        threadId: "thread-1",
        label: "first prompt",
        runId: "run-1",
      },
      runEvent({
        type: "TEXT_MESSAGE_CONTENT",
        threadId: "thread-1",
        runId: "run-1",
        messageId: "msg-1",
        delta: "first answer",
      }),
      {
        type: "run/threadsLoaded",
        threads: [
          {
            archived: false,
            threadId: "thread-1",
            label: "first prompt",
            runId: "run-1",
            workspace: { workspaceRoot: "/tmp", writableRoots: ["/tmp"] },
            createdAt: "2026-01-01T00:00:00Z",
            updatedAt: "2026-01-01T00:00:00Z",
          },
        ],
      },
      { type: "run/threadSwitch", threadId: "thread-1" },
    ]);

    expect(state.assistantText).toBe("first answer");
    expect(state.threads).toHaveLength(1);
  });

  it("clears stale active thread when runtime no longer lists it", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "first prompt" },
      {
        type: "run/created",
        run: makeRun({ runId: "run-1", threadId: "thread-1" }),
      },
      {
        type: "run/threadAdded",
        threadId: "thread-1",
        label: "first prompt",
        runId: "run-1",
      },
      { type: "run/threadsLoaded", threads: [] },
    ]);

    expect(state.activeThreadId).toBeUndefined();
    expect(state.threadId).toBeUndefined();
    expect(state.runId).toBeUndefined();
  });
});

describe("runtime reducer supervisor state", () => {
  it("initial state has null supervisorStatus", () => {
    const state = createInitialRuntimeState();
    expect(state.supervisorStatus).toBeNull();
  });

  it("supervisor/status-changed updates only sanitized status", () => {
    const info: SupervisorStatusInfo = {
      status: "crashed",
    };
    const state = dispatch([{ type: "supervisor/status-changed", info }]);
    expect(state.supervisorStatus).toBe("crashed");
  });

  it("supervisor state is preserved across run/reset", () => {
    const state = dispatch([
      {
        type: "supervisor/status-changed",
        info: { status: "running" },
      },
      { type: "run/requested", prompt: "test" },
      { type: "run/reset" },
    ]);
    expect(state.supervisorStatus).toBe("running");
  });

  it("supervisor state is preserved across run/recovered", () => {
    const state = dispatch([
      {
        type: "supervisor/status-changed",
        info: { status: "running" },
      },
      { type: "run/recovered", run: makeRun({ status: "running" }) },
    ]);
    expect(state.supervisorStatus).toBe("running");
  });
});

describe("runtime reducer thread title lifecycle", () => {
  function makeThread(overrides: Partial<RuntimeThread> = {}): RuntimeThread {
    return {
      threadId: "thread-1",
      archived: false,
      label: "fallback",
      workspace: { workspaceRoot: "/tmp", writableRoots: ["/tmp"] },
      createdAt: "2026-01-01T00:00:00Z",
      updatedAt: "2026-01-01T00:00:00Z",
      ...overrides,
    };
  }

  it("run/threadAdded does not overwrite the label of an existing thread", () => {
    const state = dispatch([
      {
        type: "run/threadAdded",
        threadId: "thread-1",
        label: "first user message",
        runId: "run-1",
      },
      {
        type: "run/threadAdded",
        threadId: "thread-1",
        label: "follow up message",
        runId: "run-2",
      },
    ]);

    const thread = state.threads.find((t) => t.threadId === "thread-1");
    expect(thread).toBeDefined();
    expect(thread?.label).toBe("first user message");
    expect(thread?.runId).toBe("run-2");
  });

  it("RUN_STARTED does not overwrite the thread label with the current prompt", () => {
    const state = dispatch([
      {
        type: "run/threadAdded",
        threadId: "thread-1",
        label: "first user message",
        runId: "run-1",
      },
      { type: "run/requested", prompt: "follow up prompt" },
      runEvent({
        type: "RUN_STARTED",
        runId: "run-2",
        threadId: "thread-1",
      }),
    ]);

    const thread = state.threads.find((t) => t.threadId === "thread-1");
    expect(thread?.label).toBe("first user message");
    expect(thread?.runId).toBe("run-2");
  });

  it("run/threadsLoaded refreshes an LLM-generated title", () => {
    const state = dispatch([
      {
        type: "run/threadAdded",
        threadId: "thread-1",
        label: "first user message",
        runId: "run-1",
      },
      {
        type: "run/threadsLoaded",
        threads: [
          makeThread({
            threadId: "thread-1",
            label: "LLM title",
            titleSource: "llm",
            titleStatus: "ready",
            titleUpdatedAt: "2026-01-01T00:00:01Z",
          }),
        ],
      },
    ]);

    const thread = state.threads.find((t) => t.threadId === "thread-1");
    expect(thread).toMatchObject({
      label: "LLM title",
      titleSource: "llm",
      titleStatus: "ready",
      titleUpdatedAt: "2026-01-01T00:00:01Z",
    });
  });

  it("run/threadRenamed immediately updates the active thread and the thread list", () => {
    const state = dispatch([
      {
        type: "run/threadAdded",
        threadId: "thread-1",
        label: "old title",
        runId: "run-1",
      },
      {
        type: "run/threadAdded",
        threadId: "thread-2",
        label: "other thread",
        runId: "run-3",
      },
      { type: "run/threadSwitch", threadId: "thread-1" },
      {
        type: "run/threadRenamed",
        thread: makeThread({
          threadId: "thread-1",
          label: "renamed title",
          titleSource: "manual",
          titleStatus: "ready",
          runId: "run-1",
        }),
      },
    ]);

    expect(state.activeThreadId).toBe("thread-1");

    const activeThread = state.threads.find((t) => t.threadId === "thread-1");
    expect(activeThread).toMatchObject({
      label: "renamed title",
      titleSource: "manual",
      titleStatus: "ready",
      runStatus: "running",
      runId: "run-1",
    });

    const otherThread = state.threads.find((t) => t.threadId === "thread-2");
    expect(otherThread?.label).toBe("other thread");
  });
});

describe("runtime reducer ACP permission gate", () => {
  it("creates an acp-permission external interaction with options", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "delegate" },
      { type: "run/created", run: makeRun() },
      runEvent({
        type: "AWAKEN_ACP_PERMISSION_REQUESTED",
        runId: "run-1",
        interactionId: "perm-1",
        toolCallId: "call-acp-1",
        acpRunnerId: "claude-code",
        message: "Claude Code wants to edit files",
        acpPermissionOptions: [
          { optionId: "allow", name: "Allow" },
          { optionId: "deny", name: "Deny" },
        ],
      }),
    ]);

    expect(state.externalInteractions).toHaveLength(1);
    expect(state.externalInteractions[0]).toMatchObject({
      id: "perm-1",
      kind: "acp-permission",
      title: "Claude Code wants to edit files",
      provider: "claude-code",
      recoveries: ["acp_option"],
      options: [
        { optionId: "allow", name: "Allow" },
        { optionId: "deny", name: "Deny" },
      ],
    });
  });

  it("removes the acp-permission interaction on resolution", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "delegate" },
      { type: "run/created", run: makeRun() },
      runEvent({
        type: "AWAKEN_ACP_PERMISSION_REQUESTED",
        runId: "run-1",
        interactionId: "perm-1",
        acpRunnerId: "claude-code",
        acpPermissionOptions: [{ optionId: "allow", name: "Allow" }],
      }),
      runEvent({
        type: "AWAKEN_ACP_PERMISSION_RESOLVED",
        runId: "run-1",
        interactionId: "perm-1",
        status: "allow",
      }),
    ]);

    expect(state.externalInteractions).toHaveLength(0);
  });

  it("does not create ACP permission interactions after the run is finished", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "delegate" },
      { type: "run/created", run: makeRun() },
      { type: "run/cancelRequested" },
      { type: "run/cancelSync", run: makeRun({ status: "cancelled" }) },
      runEvent({
        type: "AWAKEN_ACP_PERMISSION_REQUESTED",
        runId: "run-1",
        interactionId: "perm-1",
        acpRunnerId: "claude-code",
        acpPermissionOptions: [{ optionId: "allow", name: "Allow" }],
      }),
    ]);

    expect(state.externalInteractions).toHaveLength(0);
  });
});

describe("runtime reducer MCP elicitation gate", () => {
  it("preserves the validated flat form schema", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "call plugin MCP" },
      { type: "run/created", run: makeRun() },
      runEvent({
        type: "AWAKEN_EXTERNAL_INTERACTION_REQUESTED",
        runId: "run-1",
        interactionId: "mcp-elicit-1",
        interactionKind: "mcp-elicitation",
        interactionTitle: "MCP 请求确认",
        interactionSafeMessage: "Choose a destination",
        interactionProvider: "plugin__demo__server",
        interactionRecoveries: ["submit_form", "cancel"],
        interactionFormSchema: {
          type: "object",
          properties: { targetPath: { type: "string", default: ".env.local" } },
          required: ["targetPath"],
        },
      }),
    ]);

    expect(state.externalInteractions[0]).toMatchObject({
      id: "mcp-elicit-1",
      kind: "mcp-elicitation",
      provider: "plugin__demo__server",
      formSchema: {
        properties: { targetPath: { default: ".env.local" } },
      },
    });
  });
});

describe("runtime reducer run plan snapshots", () => {
  const planItemsV1 = [
    { step: "调查现状", activeForm: "正在调查现状", status: "completed" },
    {
      step: "实现运行计划协议",
      activeForm: "正在实现运行计划协议",
      status: "in_progress",
    },
    {
      step: "补齐自动化验收",
      activeForm: "正在补齐自动化验收",
      status: "pending",
    },
  ];

  function planEvent(overrides: Partial<AguiEvent> = {}): AguiEvent {
    return {
      type: "AWAKEN_PLAN_UPDATED",
      threadId: "thread-1",
      runId: "run-1",
      turnId: "turn-1",
      toolCallId: "call-plan-1",
      planId: "plan-1",
      planVersion: 1,
      planItems: planItemsV1,
      planCompletedCount: 1,
      planTotalCount: 3,
      planCurrentStep: 2,
      planAllCompleted: false,
      timestamp: 1_700_000_000_000,
      ...overrides,
    };
  }

  function stateWithActiveRun() {
    return dispatch([
      { type: "run/requested", prompt: "实现运行计划" },
      { type: "run/created", run: makeRun() },
    ]);
  }

  it("upserts the full snapshot keyed by runId without a turn-body narrative item", () => {
    const before = stateWithActiveRun();
    const after = reduceRuntimeState(before, runEvent(planEvent()));

    const plan = after.runPlansById["run-1"];
    expect(plan).toMatchObject({
      runId: "run-1",
      planId: "plan-1",
      version: 1,
      completedCount: 1,
      totalCount: 3,
      currentStep: 2,
      allCompleted: false,
      toolCallId: "call-plan-1",
      updatedAtMs: 1_700_000_000_000,
    });
    expect(plan?.items).toEqual(planItemsV1);
    // Plan frames update only the task-plan store; neither live nor terminal
    // snapshots enter the narrative body or step timeline.
    expect(after.narrative.committedItems).toEqual(
      before.narrative.committedItems,
    );
    expect(after.steps).toBe(before.steps);
  });

  it("keeps a trimmed explanation and omits an empty one", () => {
    const withExplanation = reduceRuntimeState(
      stateWithActiveRun(),
      runEvent(planEvent({ planExplanation: "  调查后增加一个必要步骤  " })),
    );
    expect(withExplanation.runPlansById["run-1"]?.explanation).toBe(
      "调查后增加一个必要步骤",
    );

    const withoutExplanation = reduceRuntimeState(
      stateWithActiveRun(),
      runEvent(planEvent({ planExplanation: "   " })),
    );
    expect(
      withoutExplanation.runPlansById["run-1"]?.explanation,
    ).toBeUndefined();
  });

  it("treats a duplicate same-version frame as idempotent", () => {
    const first = reduceRuntimeState(
      stateWithActiveRun(),
      runEvent(planEvent()),
    );
    const second = reduceRuntimeState(first, runEvent(planEvent()));

    expect(second).toBe(first);
    expect(second.runPlansById["run-1"]?.version).toBe(1);
  });

  it("ignores a lower version and replaces in place on a higher version", () => {
    const v2Items = [
      planItemsV1[0],
      { ...planItemsV1[1], status: "completed" },
      { ...planItemsV1[2], status: "in_progress" },
    ];
    let state = reduceRuntimeState(stateWithActiveRun(), runEvent(planEvent()));
    state = reduceRuntimeState(
      state,
      runEvent(
        planEvent({
          planVersion: 2,
          planItems: v2Items,
          planCompletedCount: 2,
          planCurrentStep: 3,
          toolCallId: "call-plan-2",
        }),
      ),
    );
    expect(state.runPlansById["run-1"]).toMatchObject({
      version: 2,
      completedCount: 2,
      currentStep: 3,
      toolCallId: "call-plan-2",
    });
    expect(state.runPlansById["run-1"]?.items[1]?.status).toBe("completed");

    // A stale late frame must not move the snapshot backwards.
    const stale = reduceRuntimeState(
      state,
      runEvent(planEvent({ planVersion: 1 })),
    );
    expect(stale).toBe(state);
    expect(stale.runPlansById["run-1"]?.version).toBe(2);
  });

  it("replaces the run entry when a different planId arrives for the same run", () => {
    let state = reduceRuntimeState(stateWithActiveRun(), runEvent(planEvent()));
    state = reduceRuntimeState(
      state,
      runEvent(
        planEvent({ planId: "plan-2", planVersion: 1, planTotalCount: 1 }),
      ),
    );
    expect(state.runPlansById["run-1"]).toMatchObject({
      planId: "plan-2",
      version: 1,
      totalCount: 1,
    });
  });

  it("falls back to the active run when the frame has no runId", () => {
    const state = reduceRuntimeState(
      stateWithActiveRun(),
      runEvent(planEvent({ runId: undefined })),
    );
    expect(state.runPlansById["run-1"]?.planId).toBe("plan-1");
  });

  it("drops frames that carry neither a planId nor a resolvable run", () => {
    const before = stateWithActiveRun();
    const noPlanId = reduceRuntimeState(
      before,
      runEvent(planEvent({ planId: "  " })),
    );
    expect(noPlanId.runPlansById).toEqual(before.runPlansById);
    expect(noPlanId.narrative.committedItems).toEqual(
      before.narrative.committedItems,
    );

    const idle = createInitialRuntimeState();
    // No frame runId and no active run: there is no run scope to store under.
    const noRun = reduceRuntimeState(
      idle,
      runEvent(planEvent({ runId: undefined })),
    );
    expect(noRun.runPlansById).toEqual({});
  });

  it("keeps the latest task plan when a continuation run is requested", () => {
    let state = reduceRuntimeState(stateWithActiveRun(), runEvent(planEvent()));
    expect(state.runPlansById["run-1"]).toBeDefined();

    state = reduceRuntimeState(state, {
      type: "run/requested",
      prompt: "下一个任务",
    });
    expect(state.runPlansById["run-1"]).toMatchObject({
      planId: "plan-1",
      completedCount: 1,
    });
  });

  it("selects the newest durable plan across interrupted and continued runs", () => {
    const interrupted = {
      ...planEvent(),
      runId: "run-1",
      planCompletedCount: 2,
      timestamp: 1_700_000_000_000,
    };
    const completed = {
      ...planEvent(),
      runId: "run-2",
      planId: "plan-2",
      planCompletedCount: 10,
      planTotalCount: 10,
      planAllCompleted: true,
      timestamp: 1_700_000_010_000,
    };
    const state = dispatch([
      { type: "run/requested", prompt: "开始任务" },
      { type: "run/created", run: makeRun({ runId: "run-1" }) },
      runEvent(interrupted),
      { type: "run/requested", prompt: "继续" },
      { type: "run/created", run: makeRun({ runId: "run-2" }) },
      runEvent(completed),
    ]);

    expect(latestThreadRunPlan(state.runPlansById)).toMatchObject({
      planId: "plan-2",
      completedCount: 10,
      totalCount: 10,
      allCompleted: true,
    });
  });

  it("upserts through the replay batch path with the same version rules", () => {
    const state = reduceRuntimeState(createInitialRuntimeState(), {
      type: "run/replayBatch",
      actions: [
        {
          type: "run/threadReplayStarted",
          threadId: "thread-1",
          runId: "run-1",
        },
        runEvent(planEvent({ planVersion: 2, planCompletedCount: 2 })),
        // Stale overlap from a reconnect must not regress the replayed snapshot.
        runEvent(planEvent({ planVersion: 1 })),
      ],
    });

    expect(state.runPlansById["run-1"]).toMatchObject({
      version: 2,
      completedCount: 2,
    });
    expect(state.threadViews["thread-1"]?.runPlansById["run-1"]?.version).toBe(
      2,
    );
  });

  it("keeps plans isolated per thread and restores them on switch back", () => {
    let state = dispatch([
      runEvent({ type: "RUN_STARTED", threadId: "thread-1", runId: "run-1" }),
      runEvent(planEvent()),
      { type: "run/threadSwitch", threadId: "thread-2" },
      runEvent(
        planEvent({
          threadId: "thread-2",
          runId: "run-2",
          planId: "plan-2",
          planVersion: 3,
        }),
      ),
    ]);

    expect(state.runPlansById["run-2"]?.planId).toBe("plan-2");
    expect(state.runPlansById["run-1"]).toBeUndefined();
    expect(state.threadViews["thread-1"]?.runPlansById["run-1"]).toMatchObject({
      planId: "plan-1",
      version: 1,
    });

    state = reduceRuntimeState(state, {
      type: "run/threadSwitch",
      threadId: "thread-1",
    });
    expect(state.runPlansById["run-1"]?.planId).toBe("plan-1");
    expect(state.runPlansById["run-2"]).toBeUndefined();
  });

	it("projects instruction snapshots from start responses and replay events", () => {
		const summary = {
			snapshotId: "instructions-1",
			capturedAt: "2026-08-20T10:00:00Z",
			partial: true,
      sources: [
        {
				scope: "project" as const,
				displayPath: ".awaken/AGENTS.md",
				scopeRoot: "/project",
				originalBytes: 10,
				appliedBytes: 10,
				truncated: false,
				status: "applied" as const,
        },
      ],
			diagnostics: [],
		};
		let state = reduceRuntimeState(createInitialRuntimeState(), {
			type: "run/created",
			run: {
				runId: "run-1",
				threadId: "thread-1",
				status: "running",
				cancelRequested: false,
				createdAt: "2026-08-20T10:00:00Z",
				updatedAt: "2026-08-20T10:00:00Z",
				instructionSnapshot: summary,
			},
		});
		expect(state.inspectorSummary.instructionSnapshot).toEqual(summary);

    state = reduceRuntimeState(
      state,
      runEvent({
			type: "AWAKEN_INSTRUCTION_SNAPSHOT",
        instructionSnapshot: {
          ...summary,
          snapshotId: "instructions-replayed",
          partial: false,
        },
      }),
    );
    expect(state.inspectorSummary.instructionSnapshot?.snapshotId).toBe(
      "instructions-replayed",
    );
	});

  it("keeps the run running and records live provider reconnect attempts", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "继续当前任务" },
      runEvent({
        type: "RUN_STARTED",
        runId: "run-1",
        threadId: "thread-1",
      }),
      runEvent({
        type: "REASONING_MESSAGE_CONTENT",
        runId: "run-1",
        messageId: "think-1",
        delta: "partial",
      }),
      runEvent({
        type: "AWAKEN_PROVIDER_RECONNECT",
        runId: "run-1",
        providerReconnect: {
          phase: "waiting_for_network",
          attempt: 1,
          maxAttempts: 0,
          detail: "tls handshake eof",
        },
      }),
    ]);

    expect(state.phase).toBe("running");
    expect(state.providerReconnectAttempts).toEqual([
      {
        phase: "waiting_for_network",
        attempt: 1,
        maxAttempts: 0,
        detail: "tls handshake eof",
        resetInProgress: undefined,
      },
    ]);
    expect(state.steps.some((step) => step.tag === "unknown")).toBe(false);
    expect(state.reasoningText).toBe("partial");
  });

  it("clears ephemeral model text when reconnect resets in-progress output", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "继续当前任务" },
      runEvent({
        type: "TEXT_MESSAGE_CONTENT",
        runId: "run-1",
        messageId: "ans-1",
        delta: "half answer",
      }),
      runEvent({
        type: "AWAKEN_PROVIDER_RECONNECT",
        runId: "run-1",
        providerReconnect: {
          phase: "reconnecting",
          attempt: 1,
          maxAttempts: 5,
          detail: "stream eof",
          resetInProgress: true,
        },
      }),
    ]);

    expect(state.assistantText).toBe("");
    expect(state.narrative.activeItem).toMatchObject({
      kind: "planning",
      status: "active",
    });
    expect(state.providerReconnectAttempts).toHaveLength(1);
  });

  it("clears provider reconnect rows after recovery or run terminal", () => {
    const recovered = dispatch([
      { type: "run/requested", prompt: "继续当前任务" },
      runEvent({
        type: "AWAKEN_PROVIDER_RECONNECT",
        runId: "run-1",
        providerReconnect: {
          phase: "waiting_for_network",
          attempt: 1,
          maxAttempts: 0,
          detail: "tls handshake eof",
        },
      }),
      runEvent({
        type: "AWAKEN_PROVIDER_RECONNECT",
        runId: "run-1",
        providerReconnect: { phase: "recovered" },
      }),
    ]);
    expect(recovered.providerReconnectAttempts).toEqual([]);

    const failed = dispatch([
      { type: "run/requested", prompt: "继续当前任务" },
      runEvent({
        type: "AWAKEN_PROVIDER_RECONNECT",
        runId: "run-1",
        providerReconnect: {
          phase: "reconnecting",
          attempt: 5,
          maxAttempts: 5,
          detail: "stream eof",
        },
      }),
      runEvent({
        type: "RUN_ERROR",
        code: "PROVIDER_REQUEST_FAILED",
        message: "provider request failed",
      }),
    ]);
    expect(failed.providerReconnectAttempts).toEqual([]);
    expect(failed.phase).toBe("error");
  });

  it("upserts the same reconnect phase instead of recording every backoff tick", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "继续当前任务" },
      {
        type: "run/created",
        run: makeRun({ status: "running" }),
      },
      runEvent({
        type: "AWAKEN_PROVIDER_RECONNECT",
        runId: "run-1",
        seq: 11,
        providerReconnect: {
          phase: "waiting_for_network",
          attempt: 1,
          maxAttempts: 0,
          detail: "tls handshake eof",
        },
      }),
      runEvent({
        type: "AWAKEN_PROVIDER_RECONNECT",
        runId: "run-1",
        seq: 12,
        providerReconnect: {
          phase: "waiting_for_network",
          attempt: 4,
          maxAttempts: 0,
          detail: "error sending request",
        },
      }),
    ]);

    expect(state.providerReconnectAttempts).toEqual([
      {
        phase: "waiting_for_network",
        attempt: 4,
        maxAttempts: 0,
        detail: "error sending request",
        resetInProgress: undefined,
      },
    ]);
    expect(
      state.narrative.committedItems.filter(
        (item) => item.kind === "providerReconnect",
      ),
    ).toHaveLength(1);
  });

  it("keeps in-session live reconnect rows after transcript replay of a running run", () => {
    const live = dispatch([
      { type: "run/requested", prompt: "继续当前任务" },
      {
        type: "run/created",
        run: makeRun({ status: "running" }),
      },
      runEvent({
        type: "TOOL_CALL_START",
        runId: "run-1",
        toolCallId: "tool-1",
        toolCallName: "Bash",
      }),
      runEvent({
        type: "TOOL_CALL_RESULT",
        runId: "run-1",
        toolCallId: "tool-1",
        toolCallName: "Bash",
        toolStatus: "succeeded",
      }),
      runEvent({
        type: "AWAKEN_PROVIDER_RECONNECT",
        runId: "run-1",
        seq: 80,
        providerReconnect: {
          phase: "waiting_for_network",
          attempt: 2,
          maxAttempts: 0,
          detail: "tls handshake eof",
        },
      }),
    ]);

    const replayed = reduceRuntimeState(live, {
      type: "run/replayBatch",
      actions: [
        {
          type: "run/threadReplayStarted",
          threadId: "thread-1",
          runId: "run-1",
        },
        { type: "run/requested", prompt: "继续当前任务" },
        {
          type: "run/created",
          run: makeRun({ status: "running" }),
        },
        runEvent({
          type: "TOOL_CALL_START",
          runId: "run-1",
          toolCallId: "tool-1",
          toolCallName: "Bash",
        }),
        runEvent({
          type: "TOOL_CALL_RESULT",
          runId: "run-1",
          toolCallId: "tool-1",
          toolCallName: "Bash",
          toolStatus: "succeeded",
        }),
      ],
    });

    const reconnects = replayed.narrative.committedItems.filter(
      (item) => item.kind === "providerReconnect",
    );
    expect(reconnects).toEqual([
      expect.objectContaining({
        phase: "waiting_for_network",
        attempt: 2,
        detail: "tls handshake eof",
      }),
    ]);
    expect(replayed.providerReconnectAttempts).toEqual([
      expect.objectContaining({
        phase: "waiting_for_network",
        attempt: 2,
      }),
    ]);
    expect(replayed.narrative.replayedItemIds).not.toContain(reconnects[0]?.id);
  });

  it("does not restore a reconnect row when newer replayed output proves recovery", () => {
    const live = dispatch([
      { type: "run/requested", prompt: "继续当前任务" },
      {
        type: "run/created",
        run: makeRun({ status: "running" }),
      },
      runEvent({
        type: "TOOL_CALL_RESULT",
        runId: "run-1",
        toolCallId: "tool-1",
        toolCallName: "Bash",
        toolStatus: "succeeded",
        timestamp: 100,
      }),
      runEvent({
        type: "AWAKEN_PROVIDER_RECONNECT",
        runId: "run-1",
        timestamp: 200,
        providerReconnect: {
          phase: "waiting_for_network",
          attempt: 2,
          maxAttempts: 0,
        },
      }),
    ]);

    const replayed = reduceRuntimeState(live, {
      type: "run/replayBatch",
      actions: [
        {
          type: "run/threadReplayStarted",
          threadId: "thread-1",
          runId: "run-1",
        },
        { type: "run/requested", prompt: "继续当前任务" },
        {
          type: "run/created",
          run: makeRun({ status: "running" }),
        },
        runEvent({
          type: "TOOL_CALL_RESULT",
          runId: "run-1",
          toolCallId: "tool-1",
          toolCallName: "Bash",
          toolStatus: "succeeded",
          timestamp: 100,
        }),
        runEvent({
          type: "TEXT_MESSAGE_CONTENT",
          runId: "run-1",
          messageId: "resumed",
          delta: "provider recovered while this thread was away",
          timestamp: 300,
        }),
      ],
    });

    expect(
      replayed.narrative.committedItems.some(
        (item) => item.kind === "providerReconnect",
      ),
    ).toBe(false);
    expect(replayed.providerReconnectAttempts).toEqual([]);
  });

  it("does not restore live reconnect rows when transcript replay ends the run", () => {
    const live = dispatch([
      { type: "run/requested", prompt: "继续当前任务" },
      {
        type: "run/created",
        run: makeRun({ status: "running" }),
      },
      runEvent({
        type: "AWAKEN_PROVIDER_RECONNECT",
        runId: "run-1",
        seq: 81,
        providerReconnect: {
          phase: "reconnecting",
          attempt: 2,
          maxAttempts: 5,
          detail: "stream eof",
        },
      }),
    ]);

    const replayed = reduceRuntimeState(live, {
      type: "run/replayBatch",
      actions: [
        {
          type: "run/threadReplayStarted",
          threadId: "thread-1",
          runId: "run-1",
        },
        { type: "run/requested", prompt: "继续当前任务" },
        {
          type: "run/created",
          run: makeRun({ status: "finished" }),
        },
        runEvent({ type: "RUN_FINISHED", runId: "run-1" }),
      ],
    });

    expect(
      replayed.narrative.committedItems.some(
        (item) => item.kind === "providerReconnect",
      ),
    ).toBe(false);
    expect(replayed.providerReconnectAttempts).toEqual([]);
  });
});

describe("runtime reducer computer pointer hints", () => {
  it("does not project overlay cursor hints into Workbench state", () => {
    const before = dispatch([{ type: "run/requested", prompt: "打开计算器" }]);
    const after = reduceRuntimeState(before, runEvent({
      type: "AWAKEN_COMPUTER_POINTER_HINT",
      runId: "run-1",
      computerPointerKind: "click",
      computerPointerSpace: "primary_display_dip",
      computerPointerX: 100,
      computerPointerY: 40,
    }));

    expect(after.steps).toEqual(before.steps);
    expect(after.narrative.committedItems).toEqual(before.narrative.committedItems);
    expect(after.phase).toBe(before.phase);
  });
});
