import { describe, expect, it } from "vitest";
import type { AguiEvent } from "../../shared/agui";
import type { RuntimeLogEntry, RuntimeRun, SupervisorStatusInfo } from "../../shared/runtime";
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
    expect(state.errorMessage).toBe("API key 缺失，请在 .env 中配置 LLM_API_KEY");
    expect(state.errorMessage).not.toContain("sk-");
    expect(state.errorMessage).not.toContain("LLM_API_KEY=");
    const errorStep = state.steps.find((step) => step.id === "run-error");
    expect(errorStep?.description).toBe("API key 缺失，请在 .env 中配置 LLM_API_KEY");
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

  it("run/recovered resets state so replayed approvals do not duplicate", () => {
    const state = dispatch([
      runEvent({
        type: "AWAKEN_APPROVAL_REQUESTED",
        approvalId: "appr-1",
        approvalTitle: "Approve write",
      }),
      { type: "run/recovered", run: makeRun({ status: "running" }) },
      runEvent({
        type: "AWAKEN_APPROVAL_REQUESTED",
        approvalId: "appr-1",
        approvalTitle: "Approve write",
      }),
    ]);
    expect(state.approvals.length).toBe(1);
    expect(state.approvals[0].id).toBe("appr-1");
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
});

describe("runtime reducer thread snapshots", () => {
  it("restores the selected thread view when switching conversations", () => {
    const state = dispatch([
      { type: "run/requested", prompt: "first prompt" },
      { type: "run/created", run: makeRun({ runId: "run-1", threadId: "thread-1" }) },
      { type: "run/threadAdded", threadId: "thread-1", label: "first prompt", runId: "run-1" },
      runEvent({ type: "TEXT_MESSAGE_CONTENT", threadId: "thread-1", runId: "run-1", messageId: "msg-1", delta: "first answer" }),
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
    expect(state.threads.map((thread) => thread.threadId)).toEqual(["thread-1", "thread-2"]);
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
  it("initial state has null supervisorStatus and empty runtimeLogs", () => {
    const state = createInitialRuntimeState();
    expect(state.supervisorStatus).toBeNull();
    expect(state.supervisorRuntimeBaseUrl).toBeUndefined();
    expect(state.runtimeLogs).toEqual([]);
    expect(state.supervisorErrorMessage).toBeUndefined();
    expect(state.supervisorExitCode).toBeUndefined();
  });

  it("supervisor/status-changed updates status, errorMessage, and exitCode", () => {
    const info: SupervisorStatusInfo = {
      status: "crashed",
      runtimeBaseUrl: "http://127.0.0.1:9001",
      errorMessage: "runtime exited",
      exitCode: 1,
    };
    const state = dispatch([{ type: "supervisor/status-changed", info }]);
    expect(state.supervisorStatus).toBe("crashed");
    expect(state.supervisorRuntimeBaseUrl).toBe("http://127.0.0.1:9001");
    expect(state.supervisorErrorMessage).toBe("runtime exited");
    expect(state.supervisorExitCode).toBe(1);
  });

  it("supervisor/status-changed clears runtimeBaseUrl, errorMessage, and exitCode when not provided", () => {
    const state = dispatch([
      {
        type: "supervisor/status-changed",
        info: {
          status: "crashed",
          runtimeBaseUrl: "http://127.0.0.1:9001",
          errorMessage: "boom",
          exitCode: 1,
        },
      },
      { type: "supervisor/status-changed", info: { status: "running" } },
    ]);
    expect(state.supervisorStatus).toBe("running");
    expect(state.supervisorRuntimeBaseUrl).toBeUndefined();
    expect(state.supervisorErrorMessage).toBeUndefined();
    expect(state.supervisorExitCode).toBeUndefined();
  });

  it("supervisor/log appends entries to runtimeLogs", () => {
    const entry1: RuntimeLogEntry = { id: 0, timestamp: 1000, stream: "stdout", line: "line 1" };
    const entry2: RuntimeLogEntry = { id: 1, timestamp: 2000, stream: "stderr", line: "line 2" };
    const state = dispatch([
      { type: "supervisor/log", entry: entry1 },
      { type: "supervisor/log", entry: entry2 },
    ]);
    expect(state.runtimeLogs).toEqual([entry1, entry2]);
  });

  it("supervisor/log caps at 1000 entries by shifting oldest", () => {
    const actions: RuntimeAction[] = [];
    for (let i = 0; i < 1005; i++) {
      actions.push({
        type: "supervisor/log",
        entry: { id: i, timestamp: i, stream: "stdout", line: `line ${i}` },
      });
    }
    const state = dispatch(actions);
    expect(state.runtimeLogs).toHaveLength(1000);
    expect(state.runtimeLogs[0].line).toBe("line 5");
    expect(state.runtimeLogs[999].line).toBe("line 1004");
  });

  it("supervisor/logs-replace replaces entire runtimeLogs", () => {
    const logs: RuntimeLogEntry[] = [
      { id: 0, timestamp: 1000, stream: "stdout", line: "replaced 1" },
      { id: 1, timestamp: 2000, stream: "stderr", line: "replaced 2" },
    ];
    const state = dispatch([
      { type: "supervisor/log", entry: { id: 0, timestamp: 0, stream: "stdout", line: "old" } },
      { type: "supervisor/logs-replace", logs },
    ]);
    expect(state.runtimeLogs).toEqual(logs);
  });

  it("supervisor/reset clears supervisor status and logs", () => {
    const state = dispatch([
      { type: "supervisor/status-changed", info: { status: "crashed", errorMessage: "boom", exitCode: 1 } },
      { type: "supervisor/log", entry: { id: 0, timestamp: 0, stream: "stdout", line: "log" } },
      { type: "supervisor/reset" },
    ]);
    expect(state.supervisorStatus).toBeNull();
    expect(state.supervisorRuntimeBaseUrl).toBeUndefined();
    expect(state.supervisorErrorMessage).toBeUndefined();
    expect(state.supervisorExitCode).toBeUndefined();
    expect(state.runtimeLogs).toEqual([]);
  });

  it("supervisor state is preserved across run/reset", () => {
    const state = dispatch([
      {
        type: "supervisor/status-changed",
        info: { status: "running", runtimeBaseUrl: "http://127.0.0.1:9001" },
      },
      { type: "supervisor/log", entry: { id: 0, timestamp: 0, stream: "stdout", line: "log" } },
      { type: "run/requested", prompt: "test" },
      { type: "run/reset" },
    ]);
    expect(state.supervisorStatus).toBe("running");
    expect(state.supervisorRuntimeBaseUrl).toBe("http://127.0.0.1:9001");
    expect(state.runtimeLogs).toHaveLength(1);
    expect(state.runtimeLogs[0].line).toBe("log");
  });

  it("supervisor state is preserved across run/recovered", () => {
    const state = dispatch([
      {
        type: "supervisor/status-changed",
        info: { status: "running", runtimeBaseUrl: "http://127.0.0.1:9001" },
      },
      { type: "supervisor/log", entry: { id: 0, timestamp: 0, stream: "stdout", line: "log" } },
      { type: "run/recovered", run: makeRun({ status: "running" }) },
    ]);
    expect(state.supervisorStatus).toBe("running");
    expect(state.supervisorRuntimeBaseUrl).toBe("http://127.0.0.1:9001");
    expect(state.runtimeLogs).toHaveLength(1);
  });
});
