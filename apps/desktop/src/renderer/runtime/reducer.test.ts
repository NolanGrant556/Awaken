import { describe, expect, it } from "vitest";
import type { AguiEvent } from "../../shared/agui";
import type { RuntimeRun, RuntimeThread, SupervisorStatusInfo } from "../../shared/runtime";
import {
  createInitialRuntimeState,
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

function countRunCancelledSteps(state: ReturnType<typeof createInitialRuntimeState>): number {
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

describe("runtime reducer cancel handling", () => {
  it("run/cancelRequested sets phase and status to cancelling", () => {
    const state = dispatch([{ type: "run/cancelRequested" }]);
    expect(state.phase).toBe("cancelling");
    expect(state.status).toBe("cancelling");
  });

  it("run/cancelSync with cancelled status keeps cancelled out of failed state", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "cancel me" },
      { type: "run/created", run: makeRun({ runId: "run-1", threadId: "thread-1" }) },
      { type: "run/threadAdded", threadId: "thread-1", label: "cancel me", runId: "run-1" },
      { type: "run/cancelRequested" },
      { type: "run/cancelSync", run: makeRun({ runId: "run-1", threadId: "thread-1", status: "cancelled" }) },
    ]);

    expect(state.phase).toBe("cancelled");
    expect(state.status).toBe("cancelled");
    expect(state.timelineStatus).toBe("done");
    expect(state.errorMessage).toBeUndefined();
    expect(state.threads[0]).toMatchObject({
      runStatus: "cancelled",
      activeRunId: undefined,
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
      runEvent({ type: "RUN_ERROR", code: "RUN_CANCELLED", message: "Run was cancelled by user" }),
    ]);
    expect(state.phase).toBe("cancelled");
    expect(countRunCancelledSteps(state)).toBe(1);
    expect(state.errorMessage).toBeUndefined();
  });

  it("RUN_ERROR with code RUN_CANCELLED alone sets phase to cancelled", () => {
    const state = dispatch([
      runEvent({ type: "RUN_ERROR", code: "RUN_CANCELLED", message: "Run was cancelled by user" }),
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
    expect(state.errorMessage).toBe("API key 缺失，请在 Settings > Models 配置模型 API key");
    expect(state.errorMessage).not.toContain("sk-");
    expect(state.errorMessage).not.toContain("LLM_API_KEY=");
    const errorStep = state.steps.find((step) => step.id === "run-error");
    expect(errorStep?.description).toBe("API key 缺失，请在 Settings > Models 配置模型 API key");
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
});

describe("runtime reducer replay idempotency", () => {
  it("run/recovered resets state so full replay does not duplicate text deltas", () => {
    // Simulate a partial state with some accumulated text, then a recovery
    // (which resets to initial), then a full replay of text deltas.
    const state = dispatch([
      runEvent({ type: "TEXT_MESSAGE_CONTENT", messageId: "msg-1", delta: "partial" }),
      { type: "run/recovered", run: makeRun({ status: "running" }) },
      runEvent({ type: "TEXT_MESSAGE_CONTENT", messageId: "msg-1", delta: "hello" }),
      runEvent({ type: "TEXT_MESSAGE_CONTENT", messageId: "msg-1", delta: " world" }),
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

  it("run/recovered keeps mass message review out of generic approvals", () => {
    const state = dispatch([
      runEvent({
        type: "AWAKEN_MASS_MESSAGE_REVIEW_REQUESTED",
        reviewId: "appr-1",
        approvalTitle: "Approve write",
      }),
      { type: "run/recovered", run: makeRun({ status: "running" }) },
      runEvent({
        type: "AWAKEN_MASS_MESSAGE_REVIEW_REQUESTED",
        reviewId: "appr-1",
        approvalTitle: "Approve write",
      }),
    ]);
    expect(state.approvals).toHaveLength(0);
    expect(state.narrative.activeItem).toMatchObject({
      kind: "tool",
      id: "mass-message-review:run-1:appr-1",
      status: "active",
    });
  });

  it("run/recovered with terminal status sets correct phase", () => {
    const finishedState = dispatch([
      { type: "run/recovered", run: makeRun({ status: "finished" }) },
    ]);
    expect(finishedState.phase).toBe("done");

    const errorState = dispatch([
      { type: "run/recovered", run: makeRun({ status: "error", error: "boom" }) },
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

  it("run/recovered restores prompt from thread label before full replay", () => {
    const state = dispatch([
      {
        type: "run/recovered",
        run: makeRun({ runId: "run-1", threadId: "thread-1", status: "running" }),
        prompt: "Restore this run",
      },
      runEvent({ type: "RUN_STARTED", seq: 1, runId: "run-1", threadId: "thread-1" }),
      runEvent({ type: "RUN_STARTED", seq: 1, runId: "run-1", threadId: "thread-1" }),
    ]);

    expect(state.prompt).toBe("Restore this run");
    expect(state.narrative.committedItems.filter((item) => item.kind === "prompt")).toEqual([
      { kind: "prompt", id: "prompt:thread-1:1", text: "Restore this run" },
    ]);
  });

  it("run/recovered rebuilds pending mass message review as a dedicated notice", () => {
    const state = dispatch([
      {
        type: "run/recovered",
        run: makeRun({ runId: "run-1", threadId: "thread-1", status: "running" }),
        prompt: "Approve recovered patch",
      },
      runEvent({ type: "RUN_STARTED", seq: 1, runId: "run-1", threadId: "thread-1" }),
      runEvent({
        type: "AWAKEN_MASS_MESSAGE_REVIEW_REQUESTED",
        seq: 2,
        reviewId: "approval-1",
        approvalTitle: "Approve patch",
      }),
    ]);

    expect(state.phase).toBe("running");
    expect(state.status).toBe("running");
    expect(state.approvals).toHaveLength(0);
    expect(state.narrative.activeItem).toEqual({
      kind: "tool",
      id: "mass-message-review:run-1:approval-1",
      title: "Mass message review required",
      detail: "",
      status: "active",
    });
  });

  it("run/recovered rebuilds terminal answer as done during replay", () => {
    const state = dispatch([
      {
        type: "run/recovered",
        run: makeRun({ runId: "run-1", threadId: "thread-1", status: "finished" }),
        prompt: "Recover final answer",
      },
      runEvent({ type: "RUN_STARTED", seq: 1, runId: "run-1", threadId: "thread-1" }),
      runEvent({ type: "TEXT_MESSAGE_CONTENT", seq: 2, messageId: "msg-1", delta: "done" }),
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
        run: makeRun({ runId: "run-1", threadId: "thread-1", status: "running" }),
        prompt: "Recover mixed run",
      },
      runEvent({ type: "RUN_STARTED", seq: 1, runId: "run-1", threadId: "thread-1" }),
      runEvent({ type: "REASONING_MESSAGE_CONTENT", seq: 2, messageId: "reason-1", delta: "checking" }),
      runEvent({ type: "TOOL_CALL_START", seq: 3, toolCallId: "read-1", toolCallName: "read_file" }),
      runEvent({ type: "TOOL_CALL_RESULT", seq: 4, toolCallId: "read-1", toolCallName: "read_file", content: "README" }),
      runEvent({
        type: "AWAKEN_ARTIFACT_CREATED",
        seq: 5,
        artifactId: "artifact-1",
        artifactTitle: "note.md",
        artifactKind: "file",
        artifactPath: "note.md",
      }),
      runEvent({ type: "TEXT_MESSAGE_CONTENT", seq: 6, messageId: "msg-1", delta: "hello" }),
      { type: "run/reconnecting" },
      runEvent({ type: "TEXT_MESSAGE_CONTENT", seq: 6, messageId: "msg-1", delta: "hello" }),
      runEvent({ type: "TEXT_MESSAGE_CONTENT", seq: 7, messageId: "msg-1", delta: " world" }),
    ]);

    expect(state.narrative.committedItems.filter((item) => item.kind === "prompt")).toHaveLength(1);
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
    const state = dispatch([{ type: "run/requested", prompt: "Build the workbench" }]);

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

  it("projects reasoning and text deltas into thinking and answer items", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "Say hello" },
      runEvent({ type: "RUN_STARTED", seq: 1 }),
      runEvent({ type: "REASONING_MESSAGE_CONTENT", seq: 2, messageId: "r1", delta: "checking" }),
      runEvent({ type: "TEXT_MESSAGE_CONTENT", seq: 3, messageId: "m1", delta: "hello" }),
      runEvent({ type: "TEXT_MESSAGE_CONTENT", seq: 4, messageId: "m1", delta: " world" }),
      runEvent({ type: "TEXT_MESSAGE_END", seq: 5, messageId: "m1" }),
    ]);

    const thinking = state.narrative.committedItems.find((item) => item.kind === "thinking");
    const answer = state.narrative.committedItems.find((item) => item.kind === "answer");

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
      { type: "run/created", run: makeRun({ runId: "run-1", threadId: "thread-1" }) },
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
        toolCallName: "read_file",
      }),
      runEvent({
        type: "TOOL_CALL_RESULT",
        seq: 4,
        runId: "run-1",
        toolCallId: "read-1",
        toolCallName: "read_file",
        content: "package main",
      }),
      runEvent({
        type: "TEXT_MESSAGE_CONTENT",
        seq: 5,
        runId: "run-1",
        messageId: "m1",
        delta: "最终总结。",
      }),
      runEvent({ type: "RUN_FINISHED", seq: 6, runId: "run-1", result: "done" }),
    ]);

    const answers = state.narrative.committedItems.filter((item) => item.kind === "answer");

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

  it("projects tool and mass message review events while deferring artifact summaries", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "Patch a file" },
      runEvent({ type: "RUN_STARTED", seq: 1 }),
      runEvent({ type: "TOOL_CALL_START", seq: 2, toolCallId: "tool-1", toolCallName: "apply_patch" }),
      runEvent({ type: "TOOL_CALL_RESULT", seq: 3, toolCallId: "tool-1", toolCallName: "apply_patch", content: "pending approval" }),
      runEvent({ type: "AWAKEN_MASS_MESSAGE_REVIEW_REQUESTED", seq: 4, reviewId: "approval-1", approvalTitle: "Approve patch" }),
      runEvent({ type: "AWAKEN_ARTIFACT_CREATED", seq: 5, artifactId: "artifact-1", artifactTitle: "note.md", artifactKind: "added", artifactPath: "note.md" }),
    ]);

    const tool = state.narrative.committedItems.find((item) => item.id === "tool:tool-1");

    expect(tool).toMatchObject({
      kind: "tool",
      title: "apply_patch",
      detail: "pending approval",
      status: "done",
    });
    expect(state.narrative.activeItem).toMatchObject({
      kind: "tool",
      id: "mass-message-review:approval-1",
      title: "Mass message review required",
      status: "active",
    });
    expect(state.artifacts).toContainEqual(expect.objectContaining({ id: "artifact-1" }));
    expect(state.narrative.committedItems.some((item) => item.kind === "fileChange")).toBe(false);
    expect(state.narrative.inspectorSelection).toEqual({
      kind: "tool",
      id: "mass-message-review:approval-1",
    });
  });

  it("projects consecutive read-only tools into one exploring item", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "Summarize project" },
      runEvent({ type: "RUN_STARTED", seq: 1 }),
      runEvent({ type: "TOOL_CALL_START", seq: 2, toolCallId: "read-1", toolCallName: "read_file" }),
      runEvent({ type: "TOOL_CALL_ARGS", seq: 3, toolCallId: "read-1", toolCallName: "read_file", delta: "{\"path\":\"README.md\",\"offset\":1,\"limit\":80}" }),
      runEvent({ type: "TOOL_CALL_RESULT", seq: 4, toolCallId: "read-1", toolCallName: "read_file", content: "# Awaken\nlong content that should stay out of center narrative" }),
      runEvent({ type: "TOOL_CALL_START", seq: 5, toolCallId: "list-1", toolCallName: "list_files" }),
      runEvent({ type: "TOOL_CALL_ARGS", seq: 6, toolCallId: "list-1", toolCallName: "list_files", delta: "{\"pattern\":\"**/*.md\"}" }),
      runEvent({ type: "TOOL_CALL_RESULT", seq: 7, toolCallId: "list-1", toolCallName: "list_files", content: "README.md\ndocs/intro.md" }),
      runEvent({ type: "TOOL_CALL_START", seq: 8, toolCallId: "grep-1", toolCallName: "grep_search" }),
      runEvent({ type: "TOOL_CALL_ARGS", seq: 9, toolCallId: "grep-1", toolCallName: "grep_search", delta: "{\"query\":\"Run Workbench\",\"include\":\"**/*.md\"}" }),
      runEvent({ type: "TOOL_CALL_RESULT", seq: 10, toolCallId: "grep-1", toolCallName: "grep_search", content: "docs/desktop-run-workbench-vertical-slices.md:1:Run Workbench" }),
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
    expect(state.narrative.activeItem.actions.map((action) => action.id)).toEqual([
      "explore:read-1",
      "explore:list-1",
      "explore:grep-1",
    ]);
    expect(state.narrative.activeItem.actions[0]).toMatchObject({
      title: "Read README.md L1-L80",
      status: "done",
    });
    expect(state.narrative.activeItem.actions[1]).toMatchObject({
      title: "List **/*.md",
      status: "done",
    });
    expect(state.narrative.activeItem.actions[2]).toMatchObject({
      title: "Search \"Run Workbench\" in **/*.md",
      status: "done",
    });
    expect(JSON.stringify(state.narrative.activeItem.actions)).not.toContain("long content that should stay out");
  });

  it("keeps later read-only tools after intervening thinking instead of backfilling older exploring", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "Inspect ordering" },
      runEvent({ type: "RUN_STARTED", seq: 1 }),
      runEvent({ type: "TOOL_CALL_START", seq: 2, toolCallId: "read-1", toolCallName: "read_file" }),
      runEvent({ type: "TOOL_CALL_ARGS", seq: 3, toolCallId: "read-1", toolCallName: "read_file", delta: "{\"path\":\"README.md\"}" }),
      runEvent({ type: "TOOL_CALL_RESULT", seq: 4, toolCallId: "read-1", toolCallName: "read_file", content: "# Awaken" }),
      runEvent({ type: "REASONING_MESSAGE_START", seq: 5, messageId: "reason-1" }),
      runEvent({ type: "REASONING_MESSAGE_CONTENT", seq: 6, messageId: "reason-1", delta: "Now inspect source." }),
      runEvent({ type: "REASONING_MESSAGE_END", seq: 7, messageId: "reason-1" }),
      runEvent({ type: "TOOL_CALL_START", seq: 8, toolCallId: "list-1", toolCallName: "list_files" }),
      runEvent({ type: "TOOL_CALL_ARGS", seq: 9, toolCallId: "list-1", toolCallName: "list_files", delta: "{\"pattern\":\"src/**/*.go\"}" }),
      runEvent({ type: "TOOL_CALL_RESULT", seq: 10, toolCallId: "list-1", toolCallName: "list_files", content: "src/main.go" }),
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
      runEvent({ type: "TOOL_CALL_START", seq: 1, toolCallId: "read-1", toolCallName: "read_file" }),
      runEvent({ type: "TOOL_CALL_ARGS", seq: 2, toolCallId: "read-1", toolCallName: "read_file", delta: "{\"path\":\"README.md\"}" }),
      runEvent({ type: "TOOL_CALL_RESULT", seq: 3, toolCallId: "read-1", toolCallName: "read_file", content: "# Awaken" }),
      runEvent({ type: "TOOL_CALL_START", seq: 1, toolCallId: "read-1", toolCallName: "read_file" }),
      runEvent({ type: "TOOL_CALL_ARGS", seq: 2, toolCallId: "read-1", toolCallName: "read_file", delta: "{\"path\":\"README.md\"}" }),
      runEvent({ type: "TOOL_CALL_RESULT", seq: 3, toolCallId: "read-1", toolCallName: "read_file", content: "# Awaken" }),
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
      runEvent({ type: "TOOL_CALL_ARGS", seq: 1, toolCallId: "read-1", toolCallName: "read_file", delta: "{\"path\":\"README.md\"}" }),
      runEvent({ type: "TOOL_CALL_RESULT", seq: 2, toolCallId: "read-1", toolCallName: "read_file", content: "# Awaken\nfull result" }),
    ]);

    expect(state.toolResults).toContainEqual({
      id: "read-1",
      title: "read_file",
      args: "{\"path\":\"README.md\"}",
      content: "# Awaken\nfull result",
      status: "done",
    });
  });

  it("accumulates args deltas, replaces them with full args fallback, and projects progress", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "Inspect streaming tool args" },
      runEvent({ type: "TOOL_CALL_ARGS_DELTA", seq: 1, toolCallId: "read-1", toolCallName: "read_file", delta: "{\"path\"" }),
      runEvent({ type: "TOOL_CALL_ARGS_DELTA", seq: 2, toolCallId: "read-1", toolCallName: "read_file", delta: ":\"README.md\"}" }),
      runEvent({ type: "TOOL_PROGRESS", seq: 3, toolCallId: "read-1", toolCallName: "read_file", progress: "Executing tool" }),
      runEvent({ type: "TOOL_CALL_ARGS", seq: 4, toolCallId: "read-1", toolCallName: "read_file", delta: "{\"path\":\"README.md\"}" }),
    ]);

    expect(state.toolResults).toContainEqual({
      id: "read-1",
      title: "read_file",
      args: "{\"path\":\"README.md\"}",
      content: "",
      status: "active",
    });
    const step = state.steps.find((item) => item.id === "tool:read-1");
    expect(step).toMatchObject({
      title: "read_file",
      description: "{\"path\":\"README.md\"}",
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
      title: "Read README.md",
      detail: "{\"path\":\"README.md\"}",
      status: "active",
    });
  });

  it("clears active tool projection when a run is cancelled during tool argument preparation", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "Cancel during tool prep" },
      runEvent({ type: "TOOL_CALL_ARGS_DELTA", seq: 1, toolCallId: "tool-1", toolCallName: "run_command", delta: "{\"command\"" }),
      runEvent({ type: "TOOL_PROGRESS", seq: 2, toolCallId: "tool-1", toolCallName: "run_command", progress: "Executing tool" }),
      runEvent({ type: "RUN_STATUS_CHANGED", seq: 3, status: "cancelled" }),
    ]);

    expect(state.phase).toBe("cancelled");
    expect(state.narrative.activeItem).toBeUndefined();
    expect(state.narrative.committedItems).toContainEqual({
      kind: "tool",
      id: "tool:tool-1",
      title: "run_command",
      detail: "Executing tool",
      status: "done",
    });
  });

  it("keeps non-read-only tool active between tool call end and result progress", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "Run command" },
      runEvent({ type: "TOOL_CALL_START", seq: 1, toolCallId: "tool-1", toolCallName: "run_command" }),
      runEvent({ type: "TOOL_CALL_ARGS", seq: 2, toolCallId: "tool-1", toolCallName: "run_command", delta: "{\"command\":\"pwd\"}" }),
      runEvent({ type: "TOOL_CALL_END", seq: 3, toolCallId: "tool-1", toolCallName: "run_command" }),
      runEvent({ type: "TOOL_PROGRESS", seq: 4, toolCallId: "tool-1", toolCallName: "run_command", progress: "Executing tool" }),
    ]);

    expect(state.narrative.committedItems.find((item) => item.id === "tool:tool-1")).toBeUndefined();
    expect(state.narrative.activeItem).toEqual({
      kind: "tool",
      id: "tool:tool-1",
      title: "run_command",
      detail: "Executing tool",
      status: "active",
    });
  });

  it("stores command output deltas for the terminal inspector", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "Run shell" },
      runEvent({ type: "AWAKEN_COMMAND_OUTPUT", seq: 1, toolCallId: "shell-1", toolCallName: "shell", commandStream: "stdout", delta: "hel" }),
      runEvent({ type: "AWAKEN_COMMAND_OUTPUT", seq: 2, toolCallId: "shell-1", toolCallName: "shell", commandStream: "stdout", delta: "lo", truncated: true }),
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
    expect(state.steps.find((step) => step.id === "tool:shell-1")).toMatchObject({
      title: "shell",
      description: "lo",
      status: "active",
      tag: "stdout",
    });
  });

  it("stores patch previews and turn diff for the changes inspector", () => {
    const patch = "*** Begin Patch\n*** Add File: note.md\n+hello\n*** End Patch";
    const diff = "diff --git a/note.md b/note.md\nnew file mode 100644";
    const state = dispatch([
      { type: "run/requested", prompt: "Patch file" },
      runEvent({
        type: "AWAKEN_PATCH_UPDATED",
        seq: 1,
        toolCallId: "patch-1",
        toolCallName: "apply_patch",
        patch,
        patchSummary: "新增 1 个文件",
        affectedPaths: ["note.md"],
      }),
      runEvent({ type: "AWAKEN_TURN_DIFF", seq: 2, diff, truncated: true }),
    ]);

    expect(state.patchPreviews).toEqual([
      {
        id: "patch-1",
        title: "apply_patch",
        patch,
        summary: "新增 1 个文件",
        affectedPaths: ["note.md"],
        truncated: undefined,
      },
    ]);
    expect(state.turnDiff).toEqual({ diff, truncated: true });
    expect(state.steps.find((step) => step.id === "patch:patch-1")).toMatchObject({
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
        toolCallName: "read_file",
        toolDecisionAction: "allow",
        toolDecisionSource: "static_permission",
        toolDecisionReason: "read-only tool",
      }),
      runEvent({
        type: "AWAKEN_TOOL_PERMISSION_DECIDED",
        seq: 2,
        toolCallId: "patch-1",
        toolCallName: "apply_patch",
        toolDecisionAction: "deny",
        toolDecisionSource: "headless_default",
        toolDecisionReason: "writes require allowlist",
        affectedPaths: ["note.md"],
        patchSummary: "modify note.md",
      }),
    ]);

    expect(state.approvals).toHaveLength(0);
    expect(state.inspectorSummary.lastToolDecision).toMatchObject({
      toolCallId: "patch-1",
      toolName: "apply_patch",
      action: "deny",
      source: "headless_default",
      affectedPaths: ["note.md"],
      patchSummary: "modify note.md",
    });
    expect(state.toolResults.find((tool) => tool.id === "read-1")).toMatchObject({
      title: "read_file",
      status: "done",
    });
    expect(state.toolResults.find((tool) => tool.id === "patch-1")).toMatchObject({
      title: "apply_patch",
      status: "error",
      content: expect.stringContaining("writes require allowlist"),
    });
    expect(state.steps.find((step) => step.id === "tool-decision:patch-1")).toMatchObject({
      title: "apply_patch deny",
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
        message: "compact finished: auto boundary=compact_1 estimated_tokens=4800",
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
      eventType: "AWAKEN_COMPACT_FINISHED",
      estimatedTokens: 5200,
      actualTokens: 4310,
      effectiveWindow: 12000,
      estimatedDeltaTokens: -4800,
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
    });
    expect(state.inspectorSummary.terminalReason).toBe("completed");
    expect(state.inspectorSummary.terminalMessage).toBe("done");
    expect(state.steps.find((step) => step.id === "context-window")).toMatchObject({
      title: "Context window warning",
      status: "done",
    });
    expect(state.steps.find((step) => step.id === "compact")).toMatchObject({
      title: "Compact finished",
      status: "done",
      tag: "compact_1",
    });
    expect(state.narrative.committedItems.find((item) => item.kind === "compactBoundary")).toMatchObject({
      kind: "compactBoundary",
      status: "finished",
      title: "已自动压缩上下文",
      detail: expect.stringContaining("9.6k -> 4.8k"),
      boundaryId: "compact_1",
      preEstimate: 9600,
      postEstimate: 4800,
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
        message: "compact finished: reactive boundary=compact_legacy estimated_tokens=4800",
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
      eventType: "AWAKEN_COMPACT_FINISHED",
      estimatedTokens: 4800,
      effectiveWindow: 12000,
      estimatedDeltaTokens: -4800,
    });
    expect(state.narrative.committedItems.find((item) => item.kind === "compactBoundary")).toMatchObject({
      kind: "compactBoundary",
      status: "finished",
      title: "已压缩上下文",
      detail: expect.stringContaining("compact_legacy"),
      boundaryId: "compact_legacy",
      postEstimate: 4800,
    });
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
      runEvent({ type: "TOOL_CALL_ARGS", seq: 1, toolCallId: "read-1", toolCallName: "read_file", delta: "{\"path\":\"missing.md\"}" }),
      runEvent({
        type: "TOOL_CALL_RESULT",
        seq: 2,
        toolCallId: "read-1",
        toolCallName: "read_file",
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
        delta: "{\"path\":\"README.md\"}",
      }),
    ]);

    expect(state.narrative.inspectorSelection).toEqual({
      kind: "tool",
      id: "tool:read-1",
    });
  });

  it("points inspector selection to mass message review when review is requested", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "Patch a file" },
      runEvent({
        type: "AWAKEN_MASS_MESSAGE_REVIEW_REQUESTED",
        reviewId: "approval-1",
        approvalTitle: "Approve patch",
      }),
    ]);

    expect(state.narrative.activeItem).toMatchObject({
      kind: "tool",
      id: "mass-message-review:approval-1",
      status: "active",
    });
    expect(state.narrative.inspectorSelection).toEqual({
      kind: "tool",
      id: "mass-message-review:approval-1",
    });
  });

  it("commits approved mass message review narrative without generic approvals", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "Patch a file" },
      runEvent({
        type: "AWAKEN_MASS_MESSAGE_REVIEW_REQUESTED",
        seq: 1,
        reviewId: "approval-1",
        massMessageChannel: "email",
        massMessageRecipientScope: "selected contacts",
        massMessageRecipientCount: 12,
        massMessageBodyPreview: "Hello",
      }),
      runEvent({
        type: "AWAKEN_MASS_MESSAGE_REVIEW_RESOLVED",
        seq: 2,
        reviewId: "approval-1",
        approvalTitle: "Approve patch",
        reviewDecision: "approved",
      }),
    ]);

    expect(state.approvals).toHaveLength(0);
    expect(state.narrative.activeItem).toBeUndefined();
    expect(state.narrative.committedItems).toContainEqual(
      expect.objectContaining({
        kind: "tool",
        id: "mass-message-review:approval-1",
        title: "Mass message approved",
        status: "done",
      }),
    );
  });

  it("commits rejected mass message review narrative without generic approvals", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "Patch a file" },
      runEvent({
        type: "AWAKEN_MASS_MESSAGE_REVIEW_REQUESTED",
        seq: 1,
        reviewId: "approval-1",
        approvalTitle: "Approve patch",
      }),
      runEvent({
        type: "AWAKEN_MASS_MESSAGE_REVIEW_RESOLVED",
        seq: 2,
        reviewId: "approval-1",
        approvalTitle: "Approve patch",
        reviewDecision: "rejected",
      }),
    ]);

    expect(state.approvals).toHaveLength(0);
    expect(state.narrative.activeItem).toBeUndefined();
    expect(state.narrative.committedItems).toContainEqual(
      expect.objectContaining({
        kind: "tool",
        id: "mass-message-review:approval-1",
        title: "Mass message rejected",
        status: "error",
      }),
    );
  });

  it("does not create generic approval controls when a pending mass message review is cancelled", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "Patch a file" },
      runEvent({
        type: "AWAKEN_MASS_MESSAGE_REVIEW_REQUESTED",
        seq: 1,
        reviewId: "approval-1",
        approvalTitle: "Approve patch",
      }),
      runEvent({
        type: "RUN_STATUS_CHANGED",
        seq: 2,
        status: "cancelled",
      }),
    ]);

    expect(state.approvals).toHaveLength(0);
    expect(state.narrative.activeItem).toBeUndefined();
    expect(
      state.narrative.committedItems.some((item) => item.kind === "approval"),
    ).toBe(false);
  });

  it("keeps pending mass message review notice during transport stream interruption", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "Patch a file" },
      runEvent({
        type: "AWAKEN_MASS_MESSAGE_REVIEW_REQUESTED",
        seq: 1,
        reviewId: "approval-1",
        approvalTitle: "Approve patch",
      }),
      { type: "run/streamError", message: "stream disconnected" },
    ]);

    expect(state.phase).toBe("reconnecting");
    expect(state.timelineStatus).toBe("running");
    expect(state.approvals).toHaveLength(0);
    expect(state.narrative.activeItem).toMatchObject({
      kind: "tool",
      id: "mass-message-review:approval-1",
      status: "active",
    });
    expect(state.narrative.committedItems.some((item) => item.id === "answer:error:stream")).toBe(false);
  });

  it("renders final stream failure as terminal error instead of reconnecting", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "Patch a file" },
      runEvent({
        type: "AWAKEN_MASS_MESSAGE_REVIEW_REQUESTED",
        seq: 1,
        reviewId: "approval-1",
        approvalTitle: "Approve patch",
      }),
      { type: "run/streamFailed", message: "failed after 5 reconnect attempts" },
    ]);

    expect(state.phase).toBe("error");
    expect(state.status).toBe("error");
    expect(state.timelineStatus).toBe("error");
    expect(state.errorMessage).toBe("failed after 5 reconnect attempts");
    expect(state.approvals).toHaveLength(0);
    expect(state.steps.find((step) => step.id === "stream-failed")).toMatchObject({
      title: "Stream failed",
      status: "error",
    });
    expect(state.narrative.committedItems.some((item) => item.id === "answer:error:current:stream")).toBe(true);
  });

  it("projects artifact path, title, and status into file change narrative and artifacts", () => {
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
      runEvent({ type: "TEXT_MESSAGE_CONTENT", seq: 2, messageId: "m1", delta: "done" }),
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
    });
    const answerIndex = state.narrative.committedItems.findIndex((item) => item.kind === "answer");
    const fileChangeIndex = state.narrative.committedItems.findIndex((item) => item.kind === "fileChange");
    expect(answerIndex).toBeGreaterThanOrEqual(0);
    expect(fileChangeIndex).toBeGreaterThan(answerIndex);
    expect(state.narrative.committedItems[fileChangeIndex]).toMatchObject({
      kind: "fileChange",
      id: "fileChange:current:summary",
      status: "done",
      files: [
        {
          id: "artifact-1",
          title: "example.ts",
          path: "src/example.ts",
          description: "Updated file",
          status: "done",
        },
      ],
    });
  });

  it("does not duplicate narrative items or deltas when sequenced events replay", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "Replay safe" },
      runEvent({ type: "TEXT_MESSAGE_CONTENT", seq: 1, messageId: "m1", delta: "he" }),
      runEvent({ type: "TEXT_MESSAGE_CONTENT", seq: 2, messageId: "m1", delta: "llo" }),
      runEvent({ type: "TEXT_MESSAGE_CONTENT", seq: 1, messageId: "m1", delta: "he" }),
      runEvent({ type: "TEXT_MESSAGE_CONTENT", seq: 2, messageId: "m1", delta: "llo" }),
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
      runEvent({ type: "TEXT_MESSAGE_CONTENT", seq: 1, messageId: "m1", delta: "done" }),
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
      { type: "run/created", run: makeRun({ runId: "run-1", threadId: "thread-1" }) },
      runEvent({
        type: "TEXT_MESSAGE_CONTENT",
        runId: "run-1",
        messageId: "m1",
        delta: "done",
      }),
      runEventAt({ type: "RUN_FINISHED", runId: "run-1", result: "done" }, 66000),
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

  it("annotates an already committed text message when finished status arrives", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "Finish", startedAtMs: 2000 },
      { type: "run/created", run: makeRun({ runId: "run-1", threadId: "thread-1" }) },
      runEvent({
        type: "TEXT_MESSAGE_CONTENT",
        runId: "run-1",
        messageId: "m1",
        delta: "done",
      }),
      runEvent({ type: "TEXT_MESSAGE_END", runId: "run-1", messageId: "m1" }),
      runEventAt({ type: "RUN_STATUS_CHANGED", runId: "run-1", status: "finished" }, 47000),
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
      { type: "run/created", run: makeRun({ runId: "run-1", threadId: "thread-1" }) },
      runEvent({
        type: "TEXT_MESSAGE_CONTENT",
        runId: "run-1",
        messageId: "m1",
        delta: "streamed done",
      }),
      runEventAt({ type: "RUN_STATUS_CHANGED", runId: "run-1", status: "finished" }, 47000),
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
      runEvent({ type: "TEXT_MESSAGE_CONTENT", messageId: "m1", delta: "partial" }),
      runEventAt({ type: "RUN_STATUS_CHANGED", status: "cancelled" }, 3000),
    ]);
    const errored = dispatch([
      { type: "run/requested", prompt: "Error", startedAtMs: 1000 },
      runEventAt({ type: "RUN_ERROR", code: "INTERNAL_ERROR", message: "boom" }, 3000),
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
      runEvent({ type: "TEXT_MESSAGE_CONTENT", seq: 1, messageId: "m1", delta: "partial" }),
      { type: "run/reconnecting" },
      { type: "run/streamError", message: "stream disconnected" },
      runEvent({ type: "TEXT_MESSAGE_CONTENT", seq: 2, messageId: "m1", delta: " recovered" }),
    ]);

    expect(state.phase).toBe("reconnecting");
    expect(state.errorMessage).toBeUndefined();
    expect(state.narrative.activeItem).toMatchObject({
      kind: "answer",
      id: "answer:m1",
      markdown: "partial recovered",
      status: "active",
    });
    expect(state.narrative.committedItems.some((item) => item.id === "answer:error:stream")).toBe(false);
  });
});

describe("runtime reducer thread snapshots", () => {
  it("tracks sidebar run status across run lifecycle", () => {
    let state = createInitialRuntimeState();
    state = reduceRuntimeState(state, { type: "run/requested", prompt: "update file" });
    state = reduceRuntimeState(state, { type: "run/created", run: makeRun({ runId: "run-1", threadId: "thread-1" }) });
    state = reduceRuntimeState(state, { type: "run/threadAdded", threadId: "thread-1", label: "update file", runId: "run-1" });

    expect(state.threads[0]).toMatchObject({
      threadId: "thread-1",
      runStatus: "running",
      activeRunId: "run-1",
      approvalCount: 0,
      artifactCount: 0,
    });

    state = reduceRuntimeState(state, runEvent({ type: "RUN_STATUS_CHANGED", status: "running" }));
    state = reduceRuntimeState(state, runEvent({
      type: "AWAKEN_MASS_MESSAGE_REVIEW_REQUESTED",
      reviewId: "approval-1",
      approvalTitle: "Approve patch",
    }));

    expect(state.threads[0]).toMatchObject({
      runStatus: "running",
      approvalCount: 0,
    });

    state = reduceRuntimeState(state, runEvent({
      type: "AWAKEN_MASS_MESSAGE_REVIEW_RESOLVED",
      reviewId: "approval-1",
      reviewDecision: "approved",
    }));
    state = reduceRuntimeState(state, runEvent({
      type: "AWAKEN_ARTIFACT_CREATED",
      artifactId: "artifact-1",
      artifactTitle: "example.ts",
      artifactKind: "file",
      artifactPath: "src/example.ts",
    }));

    expect(state.threads[0]).toMatchObject({
      approvalCount: 0,
      artifactCount: 1,
    });

    state = reduceRuntimeState(state, runEvent({ type: "RUN_FINISHED", result: "done" }));

    expect(state.threads[0]).toMatchObject({
      runStatus: "finished",
      activeRunId: undefined,
      artifactCount: 1,
    });
  });

  it("restores the selected thread view when switching conversations", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "first prompt" },
      { type: "run/created", run: makeRun({ runId: "run-1", threadId: "thread-1" }) },
      { type: "run/threadAdded", threadId: "thread-1", label: "first prompt", runId: "run-1" },
      runEvent({
        type: "TEXT_MESSAGE_CONTENT",
        threadId: "thread-1",
        runId: "run-1",
        messageId: "msg-1",
        delta: "first answer",
      }),
      { type: "run/reset" },
      { type: "run/requested", prompt: "second prompt" },
      { type: "run/created", run: makeRun({ runId: "run-2", threadId: "thread-2" }) },
      { type: "run/threadAdded", threadId: "thread-2", label: "second prompt", runId: "run-2" },
      runEvent({ type: "TEXT_MESSAGE_CONTENT", threadId: "thread-2", runId: "run-2", messageId: "msg-2", delta: "second answer" }),
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
      text: "first prompt",
    });
    expect(
      narrativeItems.some((item) => item.kind === "prompt" && item.text === "second prompt"),
    ).toBe(false);
    expect(state.threads.map((thread) => thread.threadId)).toEqual(["thread-1", "thread-2"]);
  });

  it("restores mass message review and artifact narrative when switching threads", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "first prompt" },
      { type: "run/created", run: makeRun({ runId: "run-1", threadId: "thread-1" }) },
      { type: "run/threadAdded", threadId: "thread-1", label: "first prompt", runId: "run-1" },
      runEvent({
        type: "AWAKEN_MASS_MESSAGE_REVIEW_REQUESTED",
        reviewId: "approval-1",
        approvalTitle: "Approve patch",
      }),
      runEvent({
        type: "AWAKEN_ARTIFACT_CREATED",
        artifactId: "artifact-1",
        artifactTitle: "example.ts",
        artifactKind: "file",
        artifactPath: "src/example.ts",
      }),
      { type: "run/reset" },
      { type: "run/requested", prompt: "second prompt" },
      { type: "run/created", run: makeRun({ runId: "run-2", threadId: "thread-2" }) },
      { type: "run/threadAdded", threadId: "thread-2", label: "second prompt", runId: "run-2" },
      runEvent({ type: "TEXT_MESSAGE_CONTENT", threadId: "thread-2", runId: "run-2", messageId: "msg-2", delta: "second answer" }),
      { type: "run/threadSwitch", threadId: "thread-1" },
    ]);

    const narrativeItems = [
      ...state.narrative.committedItems,
      ...(state.narrative.activeItem ? [state.narrative.activeItem] : []),
    ];

    expect(state.activeThreadId).toBe("thread-1");
    expect(state.approvals).toHaveLength(0);
    expect(state.artifacts).toHaveLength(1);
    expect(
      narrativeItems.some((item) => item.id === "mass-message-review:run-1:approval-1"),
    ).toBe(true);
    expect(narrativeItems.some((item) => item.kind === "fileChange")).toBe(false);
  });

  it("starts a follow-up run in the same active thread after completion", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "first prompt" },
      { type: "run/created", run: makeRun({ runId: "run-1", threadId: "thread-1" }) },
      { type: "run/threadAdded", threadId: "thread-1", label: "first prompt", runId: "run-1" },
      runEvent({
        type: "TEXT_MESSAGE_CONTENT",
        threadId: "thread-1",
        runId: "run-1",
        messageId: "msg-1",
        delta: "first answer",
      }),
      runEvent({ type: "RUN_FINISHED", result: "first done" }),
      { type: "run/requested", prompt: "follow up prompt" },
      { type: "run/created", run: makeRun({ runId: "run-2", threadId: "thread-1" }) },
      { type: "run/threadAdded", threadId: "thread-1", label: "follow up prompt", runId: "run-2" },
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
    const prompts = state.narrative.committedItems.filter((item) => item.kind === "prompt");
    expect(prompts).toEqual([
      { kind: "prompt", id: "prompt:draft:1", text: "first prompt" },
      { kind: "prompt", id: "prompt:thread-1:2", text: "follow up prompt" },
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
      { type: "run/created", run: makeRun({ runId: "run-1", threadId: "thread-1" }) },
      { type: "run/threadAdded", threadId: "thread-1", label: "first prompt", runId: "run-1" },
      runEvent({ type: "TEXT_MESSAGE_CONTENT", delta: "first answer" }),
      runEvent({ type: "RUN_FINISHED", result: "first done" }),
      { type: "run/requested", prompt: "follow up prompt" },
      { type: "run/created", run: makeRun({ runId: "run-2", threadId: "thread-1" }) },
      { type: "run/threadAdded", threadId: "thread-1", label: "follow up prompt", runId: "run-2" },
      runEvent({ type: "TEXT_MESSAGE_CONTENT", delta: "second answer" }),
      runEvent({ type: "RUN_FINISHED", result: "second done" }),
    ]);

    const answers = state.narrative.committedItems.filter((item) => item.kind === "answer");
    expect(answers).toEqual([
      { kind: "answer", id: "answer:run-1", markdown: "first answer", status: "done", isFinal: true },
      { kind: "answer", id: "answer:run-2", markdown: "second answer", status: "done", isFinal: true },
    ]);
  });

  it("loads runtime thread list without losing saved thread views", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "first prompt" },
      { type: "run/created", run: makeRun({ runId: "run-1", threadId: "thread-1" }) },
      { type: "run/threadAdded", threadId: "thread-1", label: "first prompt", runId: "run-1" },
      runEvent({ type: "TEXT_MESSAGE_CONTENT", threadId: "thread-1", runId: "run-1", messageId: "msg-1", delta: "first answer" }),
      {
        type: "run/threadsLoaded",
        threads: [
          {
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
      { type: "run/created", run: makeRun({ runId: "run-1", threadId: "thread-1" }) },
      { type: "run/threadAdded", threadId: "thread-1", label: "first prompt", runId: "run-1" },
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
