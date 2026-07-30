import { beforeEach, describe, expect, it, vi } from "vitest";
import type { AguiEvent } from "../shared/agui.js";
import { COMPUTER_OVERLAY_CHANNELS } from "../shared/computer-overlay.js";

const electronMocks = vi.hoisted(() => {
  const handlers = new Map<string, (...args: any[]) => any>();
  const windows: any[] = [];
  let nextWebContentsId = 100;
  const BrowserWindow = vi.fn(function BrowserWindowMock() {
    const listeners = new Map<string, () => void>();
    const webContentsListeners = new Map<string, () => void>();
    const window = {
      destroyed: false,
      visible: false,
      minimized: false,
      webContents: {
        id: nextWebContentsId++,
        send: vi.fn(),
        setWindowOpenHandler: vi.fn(),
        on: vi.fn((name: string, listener: () => void) => {
          webContentsListeners.set(name, listener);
        }),
      },
      isDestroyed: vi.fn(() => window.destroyed),
      loadFile: vi.fn(async () => {
        webContentsListeners.get("did-finish-load")?.();
      }),
      loadURL: vi.fn(async () => {
        webContentsListeners.get("did-finish-load")?.();
      }),
      showInactive: vi.fn(() => {
        window.visible = true;
      }),
      hide: vi.fn(() => {
        window.visible = false;
      }),
      isVisible: vi.fn(() => window.visible),
      isMinimized: vi.fn(() => window.minimized),
      restore: vi.fn(() => {
        window.minimized = false;
      }),
      destroy: vi.fn(() => {
        window.destroyed = true;
        listeners.get("closed")?.();
      }),
      setBounds: vi.fn(),
      setVisibleOnAllWorkspaces: vi.fn(),
      setAlwaysOnTop: vi.fn(),
      moveTop: vi.fn(),
      on: vi.fn((name: string, listener: () => void) => {
        listeners.set(name, listener);
      }),
    };
    windows.push(window);
    return window;
  });
  return { BrowserWindow, handlers, windows };
});

vi.mock("electron", () => ({
  app: { isPackaged: false },
  BrowserWindow: electronMocks.BrowserWindow,
  ipcMain: {
    handle: vi.fn((channel: string, handler: (...args: any[]) => any) => {
      electronMocks.handlers.set(channel, handler);
    }),
    removeHandler: vi.fn((channel: string) => {
      electronMocks.handlers.delete(channel);
    }),
  },
  screen: {
    on: vi.fn(),
    removeListener: vi.fn(),
    getCursorScreenPoint: vi.fn(() => ({ x: 500, y: 400 })),
    getDisplayNearestPoint: vi.fn(() => ({
      workArea: { x: 0, y: 24, width: 1440, height: 876 },
    })),
  },
}));

import {
  ComputerOverlayManager,
  isComputerToolName,
  resolveComputerOverlayPaths,
} from "./computer-overlay-manager.js";

function event(input: Partial<AguiEvent> & Pick<AguiEvent, "type">): AguiEvent {
  return input as AguiEvent;
}

describe("ComputerOverlayManager", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    electronMocks.handlers.clear();
    electronMocks.windows.splice(0);
  });

  it("stays visible from the first Computer call until the run ends", async () => {
    const manager = new ComputerOverlayManager({ stopComputerUse: vi.fn(async () => undefined) });
    manager.initialize();

    manager.handleRunEvent(
      event({
        type: "TOOL_CALL_START",
        runId: "run-1",
        toolCallId: "read-1",
        toolCallName: "read",
      }),
      "run-1",
    );
    expect(electronMocks.BrowserWindow).not.toHaveBeenCalled();

    manager.handleRunEvent(
      event({
        type: "TOOL_CALL_START",
        runId: "run-1",
        toolCallId: "computer-1",
        toolCallName: "computer_observe",
      }),
      "run-1",
    );
    await vi.waitFor(() =>
      expect(electronMocks.windows[0]?.showInactive).toHaveBeenCalledTimes(1),
    );
    const overlay = electronMocks.windows[0];
    expect(overlay.setAlwaysOnTop).toHaveBeenCalledWith(true, "screen-saver");

    manager.handleRunEvent(
      event({
        type: "TOOL_CALL_END",
        runId: "run-1",
        toolCallId: "computer-1",
        toolCallName: "computer_observe",
      }),
      "run-1",
    );
    expect(overlay.hide).not.toHaveBeenCalled();

    manager.handleRunEvent(
      event({
        type: "TOOL_CALL_RESULT",
        runId: "run-1",
        toolCallId: "computer-1",
        toolCallName: "computer_observe",
      }),
      "run-1",
    );
    expect(overlay.hide).not.toHaveBeenCalled();

    manager.handleRunEvent(
      event({
        type: "TOOL_CALL_START",
        runId: "run-1",
        toolCallId: "computer-2",
        toolCallName: "computer_act",
      }),
      "run-1",
    );
    expect(overlay.hide).not.toHaveBeenCalled();
    expect(electronMocks.BrowserWindow).toHaveBeenCalledTimes(1);

    manager.handleRunEvent(event({ type: "RUN_FINISHED", runId: "run-1" }), "run-1");
    expect(overlay.hide).toHaveBeenCalledTimes(1);
  });

  it("clears active calls when the run or Computer session ends", async () => {
    const manager = new ComputerOverlayManager({ stopComputerUse: vi.fn(async () => undefined) });
    manager.initialize();
    manager.handleRunEvent(
      event({
        type: "TOOL_CALL_START",
        toolCallId: "computer-1",
        toolCallName: "computer_act",
      }),
      "run-1",
    );
    await vi.waitFor(() =>
      expect(electronMocks.windows[0]?.showInactive).toHaveBeenCalledTimes(1),
    );
    const overlay = electronMocks.windows[0];

    manager.handleRunEvent(
      event({
        type: "AWAKEN_COMPUTER_SESSION_CHANGED",
        computerSessionState: "closed",
      }),
      "run-1",
    );
    expect(overlay.hide).toHaveBeenCalledTimes(1);

    manager.handleRunEvent(
      event({
        type: "TOOL_CALL_START",
        toolCallId: "computer-2",
        toolCallName: "computer_verify",
      }),
      "run-2",
    );
    await vi.waitFor(() => expect(overlay.showInactive).toHaveBeenCalledTimes(2));
    manager.handleRunEvent(event({ type: "RUN_ERROR", runId: "run-2" }), "run-2");
    expect(overlay.hide).toHaveBeenCalledTimes(2);
  });

  it("stops only Computer Use from the overlay IPC and hides after success", async () => {
    const stopComputerUse = vi.fn(async () => undefined);
    const manager = new ComputerOverlayManager({ stopComputerUse });
    manager.initialize();
    manager.handleRunEvent(
      event({
        type: "TOOL_CALL_START",
        toolCallId: "computer-1",
        toolCallName: "computer_act",
      }),
      "run-stop",
    );
    await vi.waitFor(() =>
      expect(electronMocks.windows[0]?.showInactive).toHaveBeenCalledTimes(1),
    );
    const overlay = electronMocks.windows[0];
    const stopHandler = electronMocks.handlers.get(COMPUTER_OVERLAY_CHANNELS.stop);
    expect(stopHandler).toBeTypeOf("function");

    const result = await stopHandler?.({ sender: overlay.webContents });

    expect(result).toEqual({ ok: true });
    expect(stopComputerUse).toHaveBeenCalledWith("run-stop");
    expect(overlay.hide).toHaveBeenCalled();
  });

  it("rejects stop requests from any WebContents other than the overlay", async () => {
    const stopComputerUse = vi.fn(async () => undefined);
    const manager = new ComputerOverlayManager({ stopComputerUse });
    manager.initialize();
    manager.handleRunEvent(
      event({
        type: "TOOL_CALL_START",
        toolCallId: "computer-1",
        toolCallName: "computer_act",
      }),
      "run-stop",
    );
    await vi.waitFor(() =>
      expect(electronMocks.windows[0]?.showInactive).toHaveBeenCalledTimes(1),
    );
    const stopHandler = electronMocks.handlers.get(COMPUTER_OVERLAY_CHANNELS.stop);

    const result = await stopHandler?.({ sender: { id: 9999 } });

    expect(result).toEqual({
      ok: false,
      message: "Computer Use 当前没有正在执行的调用。",
    });
    expect(stopComputerUse).not.toHaveBeenCalled();
    await manager.dispose();
  });
});

describe("computer overlay helpers", () => {
  it("recognizes only the three model-visible Computer tools", () => {
    expect(isComputerToolName("computer_observe")).toBe(true);
    expect(isComputerToolName("computer_act")).toBe(true);
    expect(isComputerToolName("computer_verify")).toBe(true);
    expect(isComputerToolName("bash")).toBe(false);
  });

  it("resolves preload and renderer paths from dist/main", () => {
    expect(resolveComputerOverlayPaths("/app/dist/main")).toEqual({
      preloadPath: "/app/dist/preload/computer-overlay.js",
      rendererPath: "/app/dist/renderer/computer-overlay.html",
    });
  });
});
