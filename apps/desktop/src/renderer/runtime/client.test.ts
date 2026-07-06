import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { AwakenBridge, SupervisorStatusInfo } from "../../shared/runtime";
import { createRuntimeClient } from "./client";

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
    getRun: vi.fn(),
    getThreadTranscript: vi.fn(),
    listThreads: vi.fn(async () => []),
    renameThread: vi.fn(),
    deleteThread: vi.fn(),
    startRun: vi.fn(),
    cancelRun: vi.fn(),
    resolveApproval: vi.fn(),
    subscribeRun: vi.fn(async () => "subscription-1"),
    unsubscribeRun: vi.fn(),
    getRuntimeInfo: vi.fn(),
    pickWorkspace: vi.fn(async () => ({ canceled: true as const })),
    validateWorkspace: vi.fn(async () => ({ valid: true as const })),
    persistWorkspaceRegistry: vi.fn(async () => undefined),
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

  it("forwards sanitized supervisor status events", () => {
    const { bridge, emitStatus } = createElectronBridge({ status: "starting" });
    installBridge(bridge);

    const client = createRuntimeClient();
    const onStatus = vi.fn();
    client.onSupervisorStatusChange(onStatus);
    emitStatus({ status: "crashed" });

    expect(onStatus).toHaveBeenCalledWith({ status: "crashed" });
  });

  it("reconnects bridge subscriptions with after_seq", async () => {
    const { bridge } = createElectronBridge({ status: "running" });
    const subscriptions: Array<{
      handlers: Parameters<AwakenBridge["subscribeRun"]>[1];
      afterSeq?: number;
    }> = [];
    bridge.subscribeRun = vi.fn(async (_runId, handlers, afterSeq) => {
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

    subscriptions[0].handlers.onEvent({ type: "TEXT_MESSAGE_CONTENT", seq: 32, delta: "partial" });
    subscriptions[0].handlers.onError?.("connection dropped");

    expect(onReconnect).toHaveBeenCalledTimes(1);
    expect(bridge.unsubscribeRun).toHaveBeenCalledWith("subscription-1");

    await vi.advanceTimersByTimeAsync(1000);

    expect(subscriptions).toHaveLength(2);
    expect(subscriptions[1].afterSeq).toBe(32);

    unsubscribe();
  });

  it("max retries delivers a readable bridge reconnect error", async () => {
    const { bridge } = createElectronBridge({ status: "running" });
    const subscriptions: Array<{
      handlers: Parameters<AwakenBridge["subscribeRun"]>[1];
      afterSeq?: number;
    }> = [];
    bridge.subscribeRun = vi.fn(async (_runId, handlers, afterSeq) => {
      subscriptions.push({ handlers, afterSeq });
      return `subscription-${subscriptions.length}`;
    });
    installBridge(bridge);

    const client = createRuntimeClient();
    const onError = vi.fn();
    await client.subscribeRun("run-1", {
      onEvent: vi.fn(),
      onError,
      onReconnect: vi.fn(),
    });

    for (let i = 0; i < 6; i += 1) {
      subscriptions[subscriptions.length - 1].handlers.onError?.("connection dropped");
      await vi.advanceTimersByTimeAsync(10000);
    }

    expect(onError).toHaveBeenCalledWith(
      expect.stringContaining("failed after 5 reconnect attempts"),
    );
  });

  it("terminal close does not reconnect", async () => {
    const { bridge } = createElectronBridge({ status: "running" });
    const subscriptions: Array<{
      handlers: Parameters<AwakenBridge["subscribeRun"]>[1];
      afterSeq?: number;
    }> = [];
    bridge.subscribeRun = vi.fn(async (_runId, handlers, afterSeq) => {
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
});
