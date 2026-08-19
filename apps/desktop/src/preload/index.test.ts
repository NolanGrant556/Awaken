import { beforeAll, describe, expect, it, vi } from "vitest";

// The preload bundle runs against a mocked electron module so the exposed
// bridge can be inspected directly.

type Listener = (...args: unknown[]) => void;

const invokeMock = vi.fn();
const sendMock = vi.fn();
const listeners = new Map<string, Listener>();
let exposedBridge: Record<string, unknown> | undefined;

vi.mock("electron", () => ({
  contextBridge: {
    exposeInMainWorld: (_name: string, bridge: Record<string, unknown>) => {
      exposedBridge = bridge;
    },
  },
  ipcRenderer: {
    invoke: (...args: unknown[]) => invokeMock(...args),
    send: (...args: unknown[]) => sendMock(...args),
    on: (channel: string, handler: Listener) => {
      listeners.set(channel, handler);
    },
    removeListener: (channel: string) => {
      listeners.delete(channel);
    },
  },
}));

beforeAll(async () => {
  await import("./index.js");
});

describe("preload scheduled task bridge", () => {
  it("exposes only narrow scheduled task methods", () => {
    expect(exposedBridge).toBeDefined();
    const bridge = exposedBridge as Record<string, unknown>;
    for (const key of [
      "listScheduledTasks",
      "getScheduledTask",
      "createScheduledTask",
      "patchScheduledTask",
      "deleteScheduledTask",
      "pauseScheduledTask",
      "resumeScheduledTask",
      "runScheduledTaskNow",
      "cancelScheduledTaskExecution",
      "listScheduledTaskExecutions",
      "getSystemTimezone",
      "subscribeScheduledTaskEvents",
      "onScheduledTaskThreadActivation",
    ]) {
      expect(typeof bridge[key], key).toBe("function");
    }
    // No raw Core URL, no arbitrary fetch/request escape hatch.
    for (const [key, value] of Object.entries(bridge)) {
      if (typeof value === "string") {
        expect(value, key).not.toMatch(/^https?:\/\//);
      }
      expect(key).not.toMatch(/fetch|requestRaw|coreUrl|baseUrl/i);
    }
  });

  it("forwards CRUD calls to the typed IPC channels", async () => {
    const bridge = exposedBridge as Record<string, (...args: unknown[]) => unknown>;
    invokeMock.mockResolvedValue([]);

    await bridge.listScheduledTasks();
    expect(invokeMock).toHaveBeenLastCalledWith("awaken:list-scheduled-tasks");

    await bridge.getScheduledTask("sched_1");
    expect(invokeMock).toHaveBeenLastCalledWith("awaken:get-scheduled-task", "sched_1");

    const createInput = { name: "x" };
    await bridge.createScheduledTask(createInput);
    expect(invokeMock).toHaveBeenLastCalledWith("awaken:create-scheduled-task", createInput);

    const patchInput = { expectedVersion: 2, name: "y" };
    await bridge.patchScheduledTask("sched_1", patchInput);
    expect(invokeMock).toHaveBeenLastCalledWith("awaken:patch-scheduled-task", "sched_1", patchInput);

    await bridge.deleteScheduledTask("sched_1", 2, "key-1");
    expect(invokeMock).toHaveBeenLastCalledWith("awaken:delete-scheduled-task", {
      taskId: "sched_1",
      expectedVersion: 2,
      idempotencyKey: "key-1",
    });

    await bridge.pauseScheduledTask("sched_1", 3);
    expect(invokeMock).toHaveBeenLastCalledWith("awaken:pause-scheduled-task", {
      taskId: "sched_1",
      expectedVersion: 3,
    });

    await bridge.resumeScheduledTask("sched_1", 4);
    expect(invokeMock).toHaveBeenLastCalledWith("awaken:resume-scheduled-task", {
      taskId: "sched_1",
      expectedVersion: 4,
    });

    await bridge.runScheduledTaskNow("sched_1", "run-key");
    expect(invokeMock).toHaveBeenLastCalledWith("awaken:run-scheduled-task-now", {
      taskId: "sched_1",
      idempotencyKey: "run-key",
    });

    await bridge.cancelScheduledTaskExecution("sched_1");
    expect(invokeMock).toHaveBeenLastCalledWith("awaken:cancel-scheduled-task-execution", "sched_1");

    await bridge.listScheduledTaskExecutions("sched_1");
    expect(invokeMock).toHaveBeenLastCalledWith("awaken:list-scheduled-task-executions", "sched_1");

    await bridge.getSystemTimezone();
    expect(invokeMock).toHaveBeenLastCalledWith("awaken:get-system-timezone");
  });

  it("dispatches scheduled task events to subscribed handlers until unsubscribe", () => {
    const bridge = exposedBridge as Record<string, (handler: (event: unknown) => void) => () => void>;
    const received: unknown[] = [];
    const unsubscribe = bridge.subscribeScheduledTaskEvents((event) => received.push(event));

    const dispatch = listeners.get("awaken:scheduled-task-event");
    expect(dispatch).toBeDefined();
    dispatch!({}, { type: "notice", taskId: "sched_1", content: "正文" });
    expect(received).toEqual([{ type: "notice", taskId: "sched_1", content: "正文" }]);

    unsubscribe();
    dispatch!({}, { type: "task_changed", taskId: "sched_1" });
    expect(received).toHaveLength(1);
  });

  it("activates threads only for non-empty thread ids", () => {
    const bridge = exposedBridge as Record<string, (handler: (threadId: string) => void) => () => void>;
    const activated: string[] = [];
    const unsubscribe = bridge.onScheduledTaskThreadActivation((threadId) => activated.push(threadId));

    const dispatch = listeners.get("awaken:scheduled-task-thread-activation");
    expect(dispatch).toBeDefined();
    dispatch!({}, { threadId: "thread-9" });
    dispatch!({}, { threadId: "" });
    dispatch!({}, {});
    expect(activated).toEqual(["thread-9"]);

    unsubscribe();
    dispatch!({}, { threadId: "thread-10" });
    expect(activated).toEqual(["thread-9"]);
  });
});

describe("preload desktop update bridge", () => {
  it("exposes only narrow update methods with no payload escape hatch", () => {
    const bridge = exposedBridge as Record<string, unknown>;
    for (const key of [
      "getDesktopUpdateState",
      "checkDesktopUpdate",
      "downloadDesktopUpdate",
      "restartAndInstallDesktopUpdate",
      "onDesktopUpdateStateChanged",
    ]) {
      expect(typeof bridge[key], key).toBe("function");
    }
  });

  it("forwards commands to the typed channels without any payload", async () => {
    const bridge = exposedBridge as Record<string, (...args: unknown[]) => unknown>;
    invokeMock.mockResolvedValue({ operationId: "op", phase: "idle", currentVersion: "0.1.0" });

    await bridge.getDesktopUpdateState();
    expect(invokeMock).toHaveBeenLastCalledWith("awaken:desktop-update:get-state");

    await bridge.checkDesktopUpdate();
    expect(invokeMock).toHaveBeenLastCalledWith("awaken:desktop-update:check");

    await bridge.downloadDesktopUpdate();
    expect(invokeMock).toHaveBeenLastCalledWith("awaken:desktop-update:download");

    await bridge.restartAndInstallDesktopUpdate();
    expect(invokeMock).toHaveBeenLastCalledWith("awaken:desktop-update:restart-and-install");
  });

  it("dispatches state snapshots until unsubscribe and drops malformed payloads", () => {
    const bridge = exposedBridge as Record<string, (handler: (snapshot: unknown) => void) => () => void>;
    const received: unknown[] = [];
    const unsubscribe = bridge.onDesktopUpdateStateChanged((snapshot) => received.push(snapshot));

    const dispatch = listeners.get("awaken:desktop-update:state-changed");
    expect(dispatch).toBeDefined();
    const snapshot = { operationId: "op-1", phase: "available", currentVersion: "0.1.0" };
    dispatch!({}, snapshot);
    dispatch!({}, { noPhase: true });
    expect(received).toEqual([snapshot]);

    unsubscribe();
    dispatch!({}, snapshot);
    expect(received).toHaveLength(1);
  });
});

describe("preload project directory watch bridge", () => {
  it("forwards scoped watch lifecycle calls and removes change listeners", async () => {
    const bridge = exposedBridge as Record<string, (...args: unknown[]) => unknown>;
    const request = { projectId: "project-1", folderPath: "/workspace/app" };
    invokeMock.mockResolvedValue({ ok: true, subscriptionId: "watch-1" });

    await bridge.watchProjectDirectory(request);
    expect(invokeMock).toHaveBeenLastCalledWith("awaken:watch-project-directory", request);
    await bridge.unwatchProjectDirectory("watch-1");
    expect(invokeMock).toHaveBeenLastCalledWith("awaken:unwatch-project-directory", "watch-1");

    const received: unknown[] = [];
    const unsubscribe = bridge.onProjectDirectoryChanged(
      (event: unknown) => received.push(event),
    ) as () => void;
    const dispatch = listeners.get("awaken:project-directory-changed");
    expect(dispatch).toBeDefined();
    dispatch!({}, { subscriptionId: "watch-1" });
    expect(received).toEqual([{ subscriptionId: "watch-1" }]);

    unsubscribe();
    expect(listeners.has("awaken:project-directory-changed")).toBe(false);
  });
});
