import { mockDesktopReply } from "./bridge-test-helper";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { AwakenBridge, SupervisorStatusInfo } from "../../shared/runtime";
import {
  createRuntimeClient,
  desktopOperationIssueFromError,
} from "./client";

beforeEach(() => {
  vi.useFakeTimers();
});

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

function createElectronBridge(
  status: SupervisorStatusInfo | Promise<SupervisorStatusInfo>,
): { bridge: AwakenBridge; emitStatus: (info: SupervisorStatusInfo) => void } {
  const statusCallbacks: Array<(info: SupervisorStatusInfo) => void> = [];
  const bridge = {
    platform: "macos",
    getRun: mockDesktopReply(),
    getThreadTranscript: mockDesktopReply(),
    listThreads: mockDesktopReply(async () => []),
    renameThread: mockDesktopReply(),
    deleteThread: mockDesktopReply(),
    deleteThreads: mockDesktopReply(),
    startRun: vi.fn(),
    cancelRun: mockDesktopReply(),
    answerUserQuestion: mockDesktopReply(),
    cancelUserQuestion: mockDesktopReply(),
    subscribeRun: mockDesktopReply(async () => "subscription-1"),
    unsubscribeRun: vi.fn(),
    getRuntimeInfo: vi.fn(),
    pickDirectory: vi.fn(async () => ({ canceled: true as const })),
    pickPluginSource: mockDesktopReply(async () => ({ canceled: true as const })),
    pickProjectFolders: vi.fn(async () => ({ canceled: true as const })),
    validateWorkspace: vi.fn(async () => ({ valid: true as const })),
    deleteMcpServer: vi.fn(async () => undefined),
    getSupervisorStatus: vi.fn(() => Promise.resolve(status)),
    onSupervisorStatusChange: vi.fn((callback: (info: SupervisorStatusInfo) => void) => {
      statusCallbacks.push(callback);
      return () => {};
    }),
    notifyRendererReady: vi.fn(),
  } as unknown as AwakenBridge;

  return {
    bridge,
    emitStatus: (info: SupervisorStatusInfo) => {
      for (const callback of statusCallbacks) {
        callback(info);
      }
    },
  };
}

function installBridge(bridge: AwakenBridge): void {
  vi.stubGlobal("window", {
    location: { origin: "http://localhost" },
    awaken: bridge,
  });
}

describe("electron runtime client", () => {
  it("disposes a reconnect that resolves after unsubscribe and ignores its late frames", async () => {
    const { bridge } = createElectronBridge({ status: "running" });
    let finish!: (id: string) => void;
    const handlers: Array<Parameters<AwakenBridge["subscribeRun"]>[1]> = [];
    bridge.subscribeRun = mockDesktopReply((_runId, callbacks) => {
      handlers.push(callbacks);
      return handlers.length === 1 ? Promise.resolve("first") : new Promise<string>((resolve) => { finish = resolve; });
    });
    installBridge(bridge);
    const onEvent = vi.fn();
    const stop = await createRuntimeClient().subscribeRun("run-1", { onEvent });
    handlers[0].onClose?.();
    await vi.advanceTimersByTimeAsync(1000);
    stop();
    finish("late"); await vi.advanceTimersByTimeAsync(0);
    handlers[1].onEvent({ type: "RUN_STARTED", runId: "run-1" });
    expect(bridge.unsubscribeRun).toHaveBeenCalledWith("late");
    expect(onEvent).not.toHaveBeenCalled();
  });
  it("forwards question cancellation through the dedicated bridge", async () => {
    const { bridge } = createElectronBridge({ status: "running" });
    installBridge(bridge);
    const client = createRuntimeClient();

    await client.cancelUserQuestion("run-1", "question-1");

    expect(bridge.cancelUserQuestion).toHaveBeenCalledWith("run-1", "question-1");
    expect(bridge.cancelRun).not.toHaveBeenCalled();
  });

  it("recovers the structured credential issue attached by the preload bridge", () => {
    const error = Object.assign(new Error("模型密钥无法读取"), {
      code: "credential_unreadable",
      retryable: true,
      actions: ["retry", "reenter", "unknown"],
    });

    expect(desktopOperationIssueFromError(error)).toEqual({
      code: "credential_unreadable",
      message: "模型密钥无法读取",
      retryable: true,
      actions: ["retry", "reenter"],
    });
  });

  it("keeps recovery actions when a plain IPC result crosses into the renderer", async () => {
    const { bridge } = createElectronBridge({ status: "running" });
    bridge.startRun = vi.fn(async () => ({
      ok: false as const,
      issue: {
        code: "credential_unreadable" as const,
        message: "模型密钥无法读取",
        retryable: true,
        actions: ["retry" as const, "reenter" as const],
      },
    }));
    installBridge(bridge);
    const client = createRuntimeClient();
    let caught: unknown;

    try {
      await client.startRun({} as never);
    } catch (error) {
      caught = error;
    }

    expect(desktopOperationIssueFromError(caught)).toEqual({
      code: "credential_unreadable",
      message: "模型密钥无法读取",
      retryable: true,
      actions: ["retry", "reenter"],
    });
  });

  it("requires the Electron bridge", () => {
    vi.stubGlobal("window", { location: { origin: "http://localhost" } });

    expect(() => createRuntimeClient()).toThrow(
      "Awaken Desktop must run through the Electron window.awaken bridge",
    );
  });

  it("does not expose a renderer-visible runtime URL", () => {
    const { bridge } = createElectronBridge({ status: "running" });
    installBridge(bridge);

    const client = createRuntimeClient();
    expect(client.source).toBe("electron");
    expect("runtimeBaseUrl" in client).toBe(false);
  });

  it("returns supervisor status through the bridge", async () => {
    const { bridge } = createElectronBridge({ status: "running" });
    installBridge(bridge);

    const client = createRuntimeClient();
    await expect(client.getSupervisorStatus()).resolves.toEqual({ status: "running" });
  });

  it("forwards plugin source browsing through the dedicated bridge", async () => {
    const { bridge } = createElectronBridge({ status: "running" });
    bridge.pickPluginSource = mockDesktopReply(async () => ({
      canceled: false as const,
      path: "/Users/test/Desktop/acme.zip",
    }));
    installBridge(bridge);

    const client = createRuntimeClient();
    await expect(client.pickPluginSource("/Users/test/Desktop")).resolves.toEqual({
      canceled: false,
      path: "/Users/test/Desktop/acme.zip",
    });
    expect(bridge.pickPluginSource).toHaveBeenCalledWith("/Users/test/Desktop");
  });

  it("forwards project-entry reveal through the dedicated bridge", async () => {
    const { bridge } = createElectronBridge({ status: "running" });
    bridge.revealProjectEntry = vi.fn(async () => ({ ok: true as const }));
    installBridge(bridge);
    const request = {
      projectId: "project-1",
      folderPath: "/workspace/app",
      relativePath: "README.md",
      kind: "file" as const,
    };

    const client = createRuntimeClient();
    await expect(client.revealProjectEntry(request)).resolves.toEqual({ ok: true });
    expect(bridge.revealProjectEntry).toHaveBeenCalledWith(request);
  });

  it("forwards project rename through the dedicated bridge", async () => {
    const { bridge } = createElectronBridge({ status: "running" });
    bridge.renameProject = vi.fn(async () => ({
      ok: true as const,
      project: {
        projectId: "project-1",
        workspaceRoot: "",
        folders: [],
        displayName: "季度复盘",
        state: "active" as const,
        createdAt: "2026-08-27T00:00:00Z",
        updatedAt: "2026-08-27T00:01:00Z",
        version: 2,
      },
    }));
    installBridge(bridge);

    const client = createRuntimeClient();
    await expect(client.renameProject("project-1", "季度复盘")).resolves.toMatchObject({
      ok: true,
      project: { projectId: "project-1", displayName: "季度复盘" },
    });
    expect(bridge.renameProject).toHaveBeenCalledWith("project-1", "季度复盘");
  });

  it("forwards batch thread deletion through the typed bridge", async () => {
    const { bridge } = createElectronBridge({ status: "running" });
    bridge.deleteThreads = mockDesktopReply(async () => ({
      ok: true as const,
      deletedThreadIds: ["thread-1"],
      failures: [{ threadId: "thread-2", code: "THREAD_BUSY", message: "busy" }],
    }));
    installBridge(bridge);

    const client = createRuntimeClient();
    await expect(client.deleteThreads(["thread-1", "thread-2"])).resolves.toEqual({
      ok: true,
      deletedThreadIds: ["thread-1"],
      failures: [{ threadId: "thread-2", code: "THREAD_BUSY", message: "busy" }],
    });
    expect(bridge.deleteThreads).toHaveBeenCalledWith(["thread-1", "thread-2"]);
  });

  it("forwards sanitized supervisor status events", () => {
    const { bridge, emitStatus } = createElectronBridge({ status: "starting" });
    installBridge(bridge);

    const client = createRuntimeClient();
    const onStatus = vi.fn();
    client.onSupervisorStatusChange(onStatus);
    emitStatus({ status: "crashed" });

    expect(onStatus).toHaveBeenCalledWith({ status: "crashed" });
  });

  it("reconnects with stream cursor instead of durable semantic seq", async () => {
    const { bridge } = createElectronBridge({ status: "running" });
    const subscriptions: Array<{
      handlers: Parameters<AwakenBridge["subscribeRun"]>[1];
      afterSeq?: number;
    }> = [];
    bridge.subscribeRun = mockDesktopReply(async (_runId, handlers, afterSeq) => {
      subscriptions.push({ handlers, afterSeq });
      return `subscription-${subscriptions.length}`;
    });
    installBridge(bridge);

    const client = createRuntimeClient();
    const onReconnect = vi.fn();
    const unsubscribe = await client.subscribeRun("run-1", {
      onEvent: vi.fn(),
      onReconnect,
    });

    subscriptions[0].handlers.onEvent({
      type: "TEXT_MESSAGE_CONTENT",
      seq: 999,
      semanticSeq: 999,
      streamCursor: 32,
      delta: "partial",
    });
    subscriptions[0].handlers.onError?.("connection dropped");

    expect(onReconnect).toHaveBeenCalledTimes(1);
    expect(bridge.unsubscribeRun).toHaveBeenCalledWith("subscription-1");

    await vi.advanceTimersByTimeAsync(1000);

    expect(subscriptions).toHaveLength(2);
    expect(subscriptions[1].afterSeq).toBe(32);

    unsubscribe();
  });

  it("keeps reconnecting while the run is not terminal", async () => {
    const { bridge } = createElectronBridge({ status: "running" });
    const subscriptions: Array<{
      handlers: Parameters<AwakenBridge["subscribeRun"]>[1];
      afterSeq?: number;
    }> = [];
    bridge.subscribeRun = mockDesktopReply(async (_runId, handlers, afterSeq) => {
      subscriptions.push({ handlers, afterSeq });
      return `subscription-${subscriptions.length}`;
    });
    installBridge(bridge);

    const client = createRuntimeClient();
    const onError = vi.fn();
    const onReconnect = vi.fn();
    await client.subscribeRun("run-1", {
      onEvent: vi.fn(),
      onError,
      onReconnect,
    });

    for (let i = 0; i < 8; i += 1) {
      subscriptions[subscriptions.length - 1].handlers.onError?.("connection dropped");
      await vi.advanceTimersByTimeAsync(10000);
    }

    expect(onError).not.toHaveBeenCalled();
    expect(onReconnect).toHaveBeenCalledTimes(8);
    expect(subscriptions).toHaveLength(9);
  });

  it("terminal close does not reconnect", async () => {
    const { bridge } = createElectronBridge({ status: "running" });
    const subscriptions: Array<{
      handlers: Parameters<AwakenBridge["subscribeRun"]>[1];
      afterSeq?: number;
    }> = [];
    bridge.subscribeRun = mockDesktopReply(async (_runId, handlers, afterSeq) => {
      subscriptions.push({ handlers, afterSeq });
      return `subscription-${subscriptions.length}`;
    });
    installBridge(bridge);

    const client = createRuntimeClient();
    const onClose = vi.fn();
    const onReconnect = vi.fn();
    await client.subscribeRun("run-1", {
      onEvent: vi.fn(),
      onClose,
      onReconnect,
    });

    subscriptions[0].handlers.onEvent({ type: "RUN_FINISHED", seq: 16 });
    subscriptions[0].handlers.onClose?.();

    expect(onClose).toHaveBeenCalledTimes(1);
    expect(onReconnect).not.toHaveBeenCalled();
  });

  it("does not reconnect when the run is not found on the server", async () => {
    const { bridge } = createElectronBridge({ status: "running" });
    const subscriptions: Array<{
      handlers: Parameters<AwakenBridge["subscribeRun"]>[1];
      afterSeq?: number;
    }> = [];
    bridge.subscribeRun = mockDesktopReply(async (_runId, handlers, afterSeq) => {
      subscriptions.push({ handlers, afterSeq });
      return `subscription-${subscriptions.length}`;
    });
    installBridge(bridge);

    const client = createRuntimeClient();
    const onError = vi.fn();
    const onReconnect = vi.fn();
    await client.subscribeRun("run-1", {
      onEvent: vi.fn(),
      onError,
      onReconnect,
    });

    subscriptions[0].handlers.onError?.("404 page not found");

    expect(onReconnect).not.toHaveBeenCalled();
    expect(onError).toHaveBeenCalledWith(
      expect.stringContaining("Run session was not found"),
    );

    // Advancing timers should not trigger any further subscribe attempts.
    await vi.advanceTimersByTimeAsync(10000);
    expect(subscriptions).toHaveLength(1);
  });
});

describe("structured subscription errors", () => {
  it("stops retrying a missing run even when the message is localized", async () => {
    const { bridge } = createElectronBridge({ status: "running" });
    bridge.subscribeRun = vi.fn(async () => ({
      ok: false as const,
      issue: { code: "RUN_NOT_FOUND", status: 404, message: "运行记录已不存在" },
    }));
    installBridge(bridge);
    const onError = vi.fn();
    const onReconnect = vi.fn();
    await createRuntimeClient().subscribeRun("missing-run", { onEvent: vi.fn(), onError, onReconnect });
    await vi.advanceTimersByTimeAsync(30000);
    expect(bridge.subscribeRun).toHaveBeenCalledTimes(1);
    expect(onReconnect).not.toHaveBeenCalled();
    expect(onError).toHaveBeenCalledWith(expect.stringContaining("运行记录已不存在"));
  });
});

describe("electron runtime client thread archive", () => {
  it("forwards the thread list view through the typed bridge", async () => {
    const { bridge } = createElectronBridge({ status: "running" });
    bridge.listThreads = mockDesktopReply(async () => []);
    installBridge(bridge);

    const client = createRuntimeClient();
    await client.listThreads("archived");
    expect(bridge.listThreads).toHaveBeenCalledWith("archived");

    await client.listThreads();
    expect(bridge.listThreads).toHaveBeenLastCalledWith(undefined);
  });

  it("forwards archive mutations and preserves structured error codes", async () => {
    const { bridge } = createElectronBridge({ status: "running" });
    bridge.setThreadArchived = vi
      .fn()
      .mockResolvedValueOnce({ ok: true, value: { ok: true, thread: { threadId: "thread-1", archived: true } } })
      .mockResolvedValueOnce({
        ok: true,
        value: {
          ok: false,
          code: "THREAD_BUSY",
          message: "请先停止正在运行的任务",
        },
      });
    installBridge(bridge);

    const client = createRuntimeClient();
    await expect(client.setThreadArchived("thread-1", true)).resolves.toEqual({
      ok: true,
      thread: { threadId: "thread-1", archived: true },
    });
    expect(bridge.setThreadArchived).toHaveBeenCalledWith("thread-1", true);

    await expect(client.setThreadArchived("thread-1", false)).resolves.toEqual({
      ok: false,
      code: "THREAD_BUSY",
      message: "请先停止正在运行的任务",
    });
  });
});
