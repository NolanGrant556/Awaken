import {
  app,
  BrowserWindow,
  ipcMain,
  screen,
  type BrowserWindowConstructorOptions,
  type IpcMainInvokeEvent,
} from "electron";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import type { AguiEvent } from "../shared/agui.js";
import {
  COMPUTER_CURSOR_CHANNELS,
  COMPUTER_OVERLAY_CHANNELS,
  type ComputerCursorHint,
  type ComputerOverlayState,
  type ComputerOverlayStopResult,
} from "../shared/computer-overlay.js";

const __dirname = dirname(fileURLToPath(import.meta.url));
const overlayWidth = 360;
const overlayHeight = 58;
const overlayTopMargin = 18;
const overlayRecoveryIntervalMs = 3_000;
const cursorSize = 32;
const cursorHotspotX = 6;
const cursorHotspotY = 4;
const computerToolNames = new Set([
  "computer_observe",
  "computer_act",
  "computer_verify",
]);

export type ComputerOverlayManagerDependencies = {
  stopComputerUse: (runId: string) => Promise<void>;
};

export type OverlayScreenPoint = {
  x: number;
  y: number;
  toX?: number;
  toY?: number;
};

export function resolveComputerOverlayPaths(baseDir: string): {
  preloadPath: string;
  rendererPath: string;
  cursorPreloadPath: string;
  cursorRendererPath: string;
} {
  return {
    preloadPath: join(baseDir, "../preload/computer-overlay.js"),
    rendererPath: join(baseDir, "../renderer/computer-overlay.html"),
    cursorPreloadPath: join(baseDir, "../preload/computer-cursor.js"),
    cursorRendererPath: join(baseDir, "../renderer/computer-cursor.html"),
  };
}

export function isComputerToolName(name: string | undefined): boolean {
  return computerToolNames.has((name ?? "").trim().toLowerCase());
}

export function computerPointerScreenPoint(
  event: AguiEvent,
  primaryBounds: { x: number; y: number },
): OverlayScreenPoint | null {
  if (
    typeof event.computerPointerX !== "number" ||
    typeof event.computerPointerY !== "number" ||
    !Number.isFinite(event.computerPointerX) ||
    !Number.isFinite(event.computerPointerY)
  ) {
    return null;
  }
  const space = event.computerPointerSpace || "primary_display_dip";
  const origin =
    space === "screen_dip"
      ? { x: 0, y: 0 }
      : space === "primary_display_dip"
        ? primaryBounds
        : null;
  if (!origin) return null;
  const point: OverlayScreenPoint = {
    x: Math.round(origin.x + event.computerPointerX),
    y: Math.round(origin.y + event.computerPointerY),
  };
  if (
    typeof event.computerPointerToX === "number" &&
    typeof event.computerPointerToY === "number" &&
    Number.isFinite(event.computerPointerToX) &&
    Number.isFinite(event.computerPointerToY)
  ) {
    point.toX = Math.round(origin.x + event.computerPointerToX);
    point.toY = Math.round(origin.y + event.computerPointerToY);
  }
  return point;
}

export class ComputerOverlayManager {
  private window: BrowserWindow | null = null;
  private cursorWindow: BrowserWindow | null = null;
  private readonly stopComputerUse: (runId: string) => Promise<void>;
  private readonly computerRuns = new Map<string, true>();
  private currentState: ComputerOverlayState | null = null;
  private handlersRegistered = false;
  private recoveryTimer: ReturnType<typeof setInterval> | null = null;
  private cursorSuppressed = false;
  private lastCursorPoint: { x: number; y: number } | null = null;
  private readonly handleDisplayChange = (): void => {
    if (!this.currentState || !this.window || this.window.isDestroyed()) return;
    this.placeAtScreenTop(this.window);
    this.restoreVisibility();
  };

  constructor(dependencies: ComputerOverlayManagerDependencies) {
    this.stopComputerUse = dependencies.stopComputerUse;
  }

  initialize(): void {
    if (this.handlersRegistered) return;
    this.handlersRegistered = true;
    screen.on("display-added", this.handleDisplayChange);
    screen.on("display-removed", this.handleDisplayChange);
    screen.on("display-metrics-changed", this.handleDisplayChange);
    ipcMain.handle(COMPUTER_OVERLAY_CHANNELS.getState, (event) => {
      if (!this.isOverlaySender(event)) return null;
      return this.currentState;
    });
    ipcMain.handle(COMPUTER_OVERLAY_CHANNELS.stop, async (event) => {
      if (!this.isOverlaySender(event) || !this.currentState) {
        return { ok: false, message: "Computer Use 当前没有正在执行的调用。" };
      }
      return this.stopCurrentComputerUse();
    });
  }

  handleRunEvent(event: AguiEvent, fallbackRunId: string): void {
    const runId = (event.runId ?? fallbackRunId).trim();
    if (!runId) return;

    if (event.type === "TOOL_CALL_START" && isComputerToolName(event.toolCallName)) {
      // Once a run enters Computer Use, keep the system indicator visible
      // across observe/act/verify calls and the model reasoning gaps between
      // them. Only a session/run terminal event can prove the phase is over.
      this.computerRuns.delete(runId);
      this.computerRuns.set(runId, true);
      if (this.currentState?.runId === runId) {
        this.restoreVisibility();
      } else {
        void this.showForRun(runId);
      }
      return;
    }

    if (event.type === "TOOL_CALL_END" || event.type === "TOOL_CALL_RESULT") {
      // Neither event proves this was the final Computer call in the run.
      // Keep the indicator through the next model turn so observe -> act ->
      // verify remains one uninterrupted, user-controllable phase.
      return;
    }

    if (event.type === "AWAKEN_COMPUTER_POINTER_HINT") {
      if (this.currentState?.runId === runId) {
        void this.applyPointerHint(event);
      }
      return;
    }

    if (event.type === "AWAKEN_COMPUTER_HANDOFF_STARTED") {
      if (this.currentState?.runId === runId) {
        this.cursorSuppressed = true;
        this.hideCursor();
      }
      return;
    }

    if (event.type === "AWAKEN_COMPUTER_HANDOFF_ENDED") {
      if (this.currentState?.runId === runId) {
        this.cursorSuppressed = false;
        void this.showCursor(this.lastCursorPoint ?? this.defaultCursorPoint());
      }
      return;
    }

    const sessionClosed =
      event.type === "AWAKEN_COMPUTER_SESSION_CHANGED" &&
      event.computerSessionState === "closed";
    const runTerminal =
      event.type === "RUN_FINISHED" ||
      event.type === "RUN_ERROR" ||
      (event.type === "RUN_STATUS_CHANGED" &&
        ["finished", "error", "cancelled"].includes(event.status ?? ""));
    if (sessionClosed || runTerminal) {
      this.clearRun(runId);
    }
  }

  handleRunStreamClosed(runId: string): void {
    this.clearRun(runId);
  }

  async dispose(): Promise<void> {
    this.stopRecoveryWatchdog();
    this.computerRuns.clear();
    this.currentState = null;
    this.cursorSuppressed = false;
    this.lastCursorPoint = null;
    if (this.handlersRegistered) {
      ipcMain.removeHandler(COMPUTER_OVERLAY_CHANNELS.getState);
      ipcMain.removeHandler(COMPUTER_OVERLAY_CHANNELS.stop);
      this.handlersRegistered = false;
    }
    screen.removeListener("display-added", this.handleDisplayChange);
    screen.removeListener("display-removed", this.handleDisplayChange);
    screen.removeListener("display-metrics-changed", this.handleDisplayChange);
    if (this.window && !this.window.isDestroyed()) {
      this.window.destroy();
    }
    this.window = null;
    if (this.cursorWindow && !this.cursorWindow.isDestroyed()) {
      this.cursorWindow.destroy();
    }
    this.cursorWindow = null;
  }

  private async showForRun(runId: string): Promise<void> {
    this.currentState = { runId, stopping: false };
    this.cursorSuppressed = false;
    const overlay = await this.ensureWindow();
    if (!this.currentState || this.currentState.runId !== runId) return;
    this.placeAtScreenTop(overlay);
    overlay.webContents.send(COMPUTER_OVERLAY_CHANNELS.stateChanged, this.currentState);
    overlay.showInactive();
    this.applyVisibilityPolicy(overlay, true);
    await this.showCursor(this.lastCursorPoint ?? this.defaultCursorPoint());
    this.startRecoveryWatchdog();
  }

  private syncVisibility(): void {
    const activeRunId = Array.from(this.computerRuns.keys()).at(-1);
    if (activeRunId) {
      if (this.currentState?.runId !== activeRunId) {
        void this.showForRun(activeRunId);
      }
      return;
    }
    this.hide();
  }

  private clearRun(runId: string): void {
    this.computerRuns.delete(runId);
    if (this.currentState?.runId === runId) {
      this.syncVisibility();
    }
  }

  private hide(): void {
    this.stopRecoveryWatchdog();
    this.currentState = null;
    this.cursorSuppressed = false;
    this.lastCursorPoint = null;
    if (this.window && !this.window.isDestroyed()) {
      this.window.webContents.send(COMPUTER_OVERLAY_CHANNELS.stateChanged, null);
      this.window.hide();
    }
    this.hideCursor();
  }

  private async stopCurrentComputerUse(): Promise<ComputerOverlayStopResult> {
    const state = this.currentState;
    if (!state || state.stopping) {
      return { ok: false, message: "Computer Use 正在停止。" };
    }
    this.currentState = { ...state, stopping: true };
    this.window?.webContents.send(COMPUTER_OVERLAY_CHANNELS.stateChanged, this.currentState);
    try {
      await this.stopComputerUse(state.runId);
      this.clearRun(state.runId);
      return { ok: true };
    } catch (error) {
      this.currentState = { ...state, stopping: false };
      this.window?.webContents.send(COMPUTER_OVERLAY_CHANNELS.stateChanged, this.currentState);
      return {
        ok: false,
        message: error instanceof Error ? error.message : "停止 Computer Use 失败。",
      };
    }
  }

  private async ensureWindow(): Promise<BrowserWindow> {
    if (this.window && !this.window.isDestroyed()) return this.window;

    const display = screen.getDisplayNearestPoint(screen.getCursorScreenPoint());
    const options: BrowserWindowConstructorOptions = {
      width: overlayWidth,
      height: overlayHeight,
      x: Math.round(display.workArea.x + (display.workArea.width - overlayWidth) / 2),
      y: display.workArea.y + overlayTopMargin,
      frame: false,
      transparent: true,
      backgroundColor: "#00000000",
      alwaysOnTop: true,
      skipTaskbar: true,
      resizable: false,
      movable: false,
      minimizable: false,
      maximizable: false,
      fullscreenable: false,
      hasShadow: false,
      show: false,
      focusable: true,
      acceptFirstMouse: true,
      paintWhenInitiallyHidden: true,
      webPreferences: {
        preload: resolveComputerOverlayPaths(__dirname).preloadPath,
        contextIsolation: true,
        nodeIntegration: false,
        sandbox: false,
        backgroundThrottling: false,
      },
    };
    const overlay = new BrowserWindow(options);
    this.window = overlay;
    this.applyVisibilityPolicy(overlay);
    overlay.on("closed", () => {
      if (this.window === overlay) this.window = null;
    });
    overlay.webContents.setWindowOpenHandler(() => ({ action: "deny" }));
    overlay.webContents.on("did-finish-load", () => {
      if (this.window !== overlay || overlay.isDestroyed()) return;
      overlay.webContents.send(COMPUTER_OVERLAY_CHANNELS.stateChanged, this.currentState);
    });
    const { rendererPath } = resolveComputerOverlayPaths(__dirname);
    if (app.isPackaged) {
      await overlay.loadFile(rendererPath);
    } else {
      await overlay.loadURL("http://127.0.0.1:5174/computer-overlay.html");
    }
    return overlay;
  }

  private async ensureCursorWindow(): Promise<BrowserWindow> {
    if (this.cursorWindow && !this.cursorWindow.isDestroyed()) return this.cursorWindow;

    const point = this.lastCursorPoint ?? this.defaultCursorPoint();
    const overlay = new BrowserWindow({
      width: cursorSize,
      height: cursorSize,
      x: point.x - cursorHotspotX,
      y: point.y - cursorHotspotY,
      frame: false,
      transparent: true,
      backgroundColor: "#00000000",
      alwaysOnTop: true,
      skipTaskbar: true,
      resizable: false,
      movable: false,
      minimizable: false,
      maximizable: false,
      fullscreenable: false,
      focusable: false,
      hasShadow: false,
      show: false,
      acceptFirstMouse: false,
      paintWhenInitiallyHidden: true,
      roundedCorners: false,
      webPreferences: {
        preload: resolveComputerOverlayPaths(__dirname).cursorPreloadPath,
        contextIsolation: true,
        nodeIntegration: false,
        sandbox: false,
        backgroundThrottling: false,
      },
    });
    this.cursorWindow = overlay;
    overlay.setIgnoreMouseEvents(true, { forward: true });
    this.applyVisibilityPolicy(overlay);
    overlay.on("closed", () => {
      if (this.cursorWindow === overlay) this.cursorWindow = null;
    });
    overlay.webContents.setWindowOpenHandler(() => ({ action: "deny" }));
    const { cursorRendererPath } = resolveComputerOverlayPaths(__dirname);
    if (app.isPackaged) {
      await overlay.loadFile(cursorRendererPath);
    } else {
      await overlay.loadURL("http://127.0.0.1:5174/computer-cursor.html");
    }
    return overlay;
  }

  private async applyPointerHint(event: AguiEvent): Promise<void> {
    const primary = screen.getPrimaryDisplay();
    const point = computerPointerScreenPoint(event, primary.bounds);
    const pulse = event.computerPointerKind === "click" || event.computerPointerKind === "double_click";
    if (point?.toX != null && point.toY != null) {
      await this.showCursor({ x: point.x, y: point.y }, { kind: event.computerPointerKind ?? "", pulse: false });
      await this.showCursor({ x: point.toX, y: point.toY }, { kind: event.computerPointerKind ?? "", pulse });
      return;
    }
    if (point) {
      await this.showCursor(point, { kind: event.computerPointerKind ?? "", pulse });
    }
  }

  private async showCursor(
    point: { x: number; y: number },
    hint?: ComputerCursorHint,
  ): Promise<void> {
    this.lastCursorPoint = point;
    if (this.cursorSuppressed || !this.currentState) return;
    const overlay = await this.ensureCursorWindow();
    if (this.cursorSuppressed || !this.currentState) return;
    overlay.setBounds(
      {
        x: point.x - cursorHotspotX,
        y: point.y - cursorHotspotY,
        width: cursorSize,
        height: cursorSize,
      },
      false,
    );
    overlay.showInactive();
    this.applyVisibilityPolicy(overlay, true);
    overlay.setIgnoreMouseEvents(true, { forward: true });
    if (hint) {
      overlay.webContents.send(COMPUTER_CURSOR_CHANNELS.hintChanged, hint);
    }
  }

  private hideCursor(): void {
    if (this.cursorWindow && !this.cursorWindow.isDestroyed()) {
      this.cursorWindow.hide();
    }
  }

  private defaultCursorPoint(): { x: number; y: number } {
    const display = screen.getPrimaryDisplay();
    return {
      x: Math.round(display.bounds.x + display.bounds.width / 2),
      y: Math.round(display.bounds.y + display.bounds.height / 2),
    };
  }

  private startRecoveryWatchdog(): void {
    this.stopRecoveryWatchdog();
    this.recoveryTimer = setInterval(() => {
      this.restoreVisibility();
    }, overlayRecoveryIntervalMs);
  }

  private stopRecoveryWatchdog(): void {
    if (!this.recoveryTimer) return;
    clearInterval(this.recoveryTimer);
    this.recoveryTimer = null;
  }

  private restoreVisibility(): void {
    const overlay = this.window;
    if (!this.currentState || !overlay || overlay.isDestroyed()) return;
    if (overlay.isMinimized()) overlay.restore();
    if (!overlay.isVisible()) overlay.showInactive();
    this.applyVisibilityPolicy(overlay, true);
    if (!this.cursorSuppressed && this.cursorWindow && !this.cursorWindow.isDestroyed()) {
      if (!this.cursorWindow.isVisible()) this.cursorWindow.showInactive();
      this.applyVisibilityPolicy(this.cursorWindow, true);
      this.cursorWindow.setIgnoreMouseEvents(true, { forward: true });
    }
  }

  private placeAtScreenTop(overlay: BrowserWindow): void {
    const display = screen.getDisplayNearestPoint(screen.getCursorScreenPoint());
    overlay.setBounds(
      {
        x: Math.round(display.workArea.x + (display.workArea.width - overlayWidth) / 2),
        y: display.workArea.y + overlayTopMargin,
        width: overlayWidth,
        height: overlayHeight,
      },
      false,
    );
  }

  private applyVisibilityPolicy(overlay: BrowserWindow, moveToTop = false): void {
    if (process.platform === "darwin") {
      overlay.setVisibleOnAllWorkspaces(true, {
        visibleOnFullScreen: true,
        skipTransformProcessType: true,
      });
    }
    overlay.setAlwaysOnTop(true, "screen-saver");
    if (moveToTop) overlay.moveTop();
  }

  private isOverlaySender(event: IpcMainInvokeEvent): boolean {
    return Boolean(
      this.window &&
        !this.window.isDestroyed() &&
        event.sender.id === this.window.webContents.id,
    );
  }
}
