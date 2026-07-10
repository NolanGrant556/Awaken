import {
  app,
  BrowserWindow,
  ipcMain,
  protocol,
  screen as electronScreen,
  shell,
  type Rectangle,
} from "electron";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import fs from "node:fs/promises";
import {
  type PetActivitySnapshot,
  type PetManifest,
  type PetNotificationActionRequest,
  type PetOverlayBootstrap,
  type PetOverlayPlacement,
  type ScreenPoint,
  petAssetUrl,
} from "../../shared/pets.js";
import type { PetStore } from "./pet-store.js";
import { PET_ASSET_PROTOCOL, parsePetAssetUrl, PET_IPC_CHANNELS } from "./pet-protocol.js";
import {
  clampPetOverlayBoundsToWorkArea,
  fixedOverlaySize,
  minOverlaySize,
  normalizePetWindowBoundsForPlacement,
  resolveBottomAnchoredDragTop,
  resolvePetOverlayWindowSize,
  resolveSavedPetWindowBounds,
  resizePetWindowBoundsFromBottomRight,
  stabilizePetWindowBounds,
  type OverlaySize,
} from "./pet-window-geometry.js";

async function readErrorMessage(response: Response): Promise<string> {
  const fallback = `Runtime request failed (${response.status})`;
  const body = (await response.text()).trim();
  if (!body) return fallback;
  try {
    const parsed = JSON.parse(body);
    const code = parsed.code ?? parsed.error;
    if (code && parsed.message) {
      return `${code}: ${parsed.message}`;
    }
  } catch {
    // fall through
  }
  return body;
}

const __dirname = dirname(fileURLToPath(import.meta.url));

type PetDragOrigin = {
  rendererX: number;
  rendererY: number;
  cursorX: number;
  cursorY: number;
  windowX: number;
  windowY: number;
  windowHeight: number;
  lastLogAtMs: number;
};

function isFinitePoint(point: unknown): point is ScreenPoint {
  if (!point || typeof point !== "object") return false;
  const candidate = point as Partial<ScreenPoint>;
  return (
    typeof candidate.x === "number" &&
    Number.isFinite(candidate.x) &&
    typeof candidate.y === "number" &&
    Number.isFinite(candidate.y)
  );
}

export function resolvePetWindowPaths(baseDir: string): {
  preloadPath: string;
  rendererPath: string;
} {
  return {
    preloadPath: join(baseDir, "../../preload/pet-overlay.js"),
    rendererPath: join(baseDir, "../../renderer/pet-overlay.html"),
  };
}

export type PetWindowManagerDependencies = {
  petStore: PetStore;
  focusMainWindow: () => void;
  activateThread: (threadId: string) => void;
  getRuntimeBaseUrl: () => string | null;
};

export class PetWindowManager {
  private window: BrowserWindow | null = null;
  private petStore: PetStore;
  private focusMainWindow: () => void;
  private activateThread: (threadId: string) => void;
  private getRuntimeBaseUrl: () => string | null;
  private latestSnapshot: PetActivitySnapshot = { sessions: [], emittedAtMs: 0 };
  private latestWindowSize: OverlaySize = fixedOverlaySize;
  private latestInteractiveRegion: Rectangle = { x: 0, y: 0, ...fixedOverlaySize };
  private dragOrigin: PetDragOrigin | null = null;
  private registeredHandlers = false;
  private recoveryTimer: ReturnType<typeof setInterval> | null = null;
  private readonly handleDisplayChange = (): void => {
    if (!this.isOpen()) return;
    const bounds = this.window!.getBounds();
    const display = electronScreen.getDisplayMatching(bounds);
    this.window!.setBounds(
      this.clampBoundsToDisplay(bounds, display.bounds),
      false,
    );
    this.savePlacement();
  };

  constructor(deps: PetWindowManagerDependencies) {
    this.petStore = deps.petStore;
    this.focusMainWindow = deps.focusMainWindow;
    this.activateThread = deps.activateThread;
    this.getRuntimeBaseUrl = deps.getRuntimeBaseUrl;
  }

  async initialize(): Promise<void> {
    this.registerProtocol();
    this.registerIpcHandlers();
    electronScreen.on("display-added", this.handleDisplayChange);
    electronScreen.on("display-removed", this.handleDisplayChange);
    electronScreen.on("display-metrics-changed", this.handleDisplayChange);
  }

  isOpen(): boolean {
    return this.window !== null && !this.window.isDestroyed();
  }

  async show(petId?: string): Promise<void> {
    const targetPetId = petId ?? this.petStore.getSettings().selectedPetId ?? undefined;
    if (!targetPetId) return;

    if (this.isOpen()) {
      await this.setPet(targetPetId);
      this.window?.showInactive();
      this.restoreAlwaysOnTop();
      this.startRecoveryWatchdog();
      return;
    }

    await this.createWindow(targetPetId);
    this.startRecoveryWatchdog();
  }

  hide(): void {
    this.stopRecoveryWatchdog();
    if (this.isOpen()) {
      this.savePlacement();
      this.window?.hide();
    }
  }

  async setPet(petId: string): Promise<void> {
    if (!this.isOpen()) return;
    const manifest = this.petStore.getAllManifests().find((m) => m.id === petId);
    if (!manifest) return;

    this.window?.webContents.send(
      PET_IPC_CHANNELS.petOverlayBootstrap,
      this.buildBootstrap(manifest),
    );
  }

  updateActivity(snapshot: PetActivitySnapshot): void {
    this.latestSnapshot = snapshot;
    if (this.isOpen()) {
      this.window?.webContents.send(PET_IPC_CHANNELS.petActivityChanged, snapshot);
    }
  }

  notifyMainWindowFocused(): void {
    if (this.isOpen()) {
      this.window?.webContents.send(PET_IPC_CHANNELS.petMainWindowFocused);
    }
  }

  async dispose(): Promise<void> {
    this.stopRecoveryWatchdog();
    if (this.isOpen()) {
      await this.petStore.setPlacement(this.buildPlacement());
      this.window?.close();
      this.window = null;
    }
    this.unregisterIpcHandlers();
    electronScreen.removeListener("display-added", this.handleDisplayChange);
    electronScreen.removeListener("display-removed", this.handleDisplayChange);
    electronScreen.removeListener("display-metrics-changed", this.handleDisplayChange);
  }

  private async createWindow(petId: string): Promise<void> {
    const manifest = this.petStore.getAllManifests().find((m) => m.id === petId);
    if (!manifest) return;

    const placement = this.resolvePlacement();
    this.latestWindowSize = { width: placement.width, height: placement.height };
    this.latestInteractiveRegion = { x: 0, y: 0, ...this.latestWindowSize };

    const { preloadPath, rendererPath } = resolvePetWindowPaths(__dirname);

    this.window = new BrowserWindow({
      ...placement,
      minWidth: minOverlaySize.width,
      minHeight: minOverlaySize.height,
      maxWidth: fixedOverlaySize.width,
      maxHeight: fixedOverlaySize.height,
      frame: false,
      transparent: true,
      backgroundColor: "#00000000",
      alwaysOnTop: true,
      skipTaskbar: true,
      resizable: false,
      hasShadow: false,
      show: false,
      focusable: true,
      paintWhenInitiallyHidden: true,
      webPreferences: {
        preload: preloadPath,
        contextIsolation: true,
        nodeIntegration: false,
        sandbox: false,
        backgroundThrottling: false,
      },
    });

    if (process.platform === "darwin") {
      this.window.setVisibleOnAllWorkspaces(true, { visibleOnFullScreen: true });
      this.window.setAlwaysOnTop(true, "screen-saver");
    } else {
      this.window.setAlwaysOnTop(true, "screen-saver");
    }

    this.window.setIgnoreMouseEvents(true, { forward: true });

    this.window.on("closed", () => {
      this.window = null;
    });

    this.window.webContents.setWindowOpenHandler(({ url }) => {
      void shell.openExternal(url);
      return { action: "deny" };
    });

    this.window.webContents.on("did-finish-load", () => {
      if (!this.window || this.window.isDestroyed()) return;
      this.window.webContents.send(
        PET_IPC_CHANNELS.petOverlayBootstrap,
        this.buildBootstrap(manifest),
      );
      this.window.webContents.send(PET_IPC_CHANNELS.petActivityChanged, this.latestSnapshot);
    });

    const loadUrl = app.isPackaged
      ? this.window.loadFile(rendererPath)
      : this.window.loadURL("http://127.0.0.1:5174/pet-overlay.html");

    await loadUrl;

    this.window.showInactive();
  }

  private restoreAlwaysOnTop(): void {
    if (!this.isOpen()) return;
    if (process.platform === "darwin") {
      this.window!.setVisibleOnAllWorkspaces(true, { visibleOnFullScreen: true });
    }
    this.window!.setAlwaysOnTop(true, "screen-saver");
    this.window!.moveTop();
  }

  private startRecoveryWatchdog(): void {
    this.stopRecoveryWatchdog();
    this.recoveryTimer = setInterval(() => {
      void this.recoverPetWindowIfNeeded();
    }, 3000);
  }

  private stopRecoveryWatchdog(): void {
    if (this.recoveryTimer) {
      clearInterval(this.recoveryTimer);
      this.recoveryTimer = null;
    }
  }

  private async recoverPetWindowIfNeeded(): Promise<void> {
    const settings = this.petStore.getSettings();
    const shouldBeVisible = settings.overlayOpen && settings.selectedPetId;
    if (!shouldBeVisible) {
      return;
    }

    if (!this.isOpen()) {
      if (settings.selectedPetId) {
        await this.show(settings.selectedPetId);
      }
      return;
    }

    const win = this.window!;
    if (win.isMinimized()) {
      win.restore();
    }
    if (!win.isVisible()) {
      win.showInactive();
    }
    this.restoreAlwaysOnTop();
  }

  private registerProtocol(): void {
    if (protocol.isProtocolHandled(PET_ASSET_PROTOCOL.slice(0, -1))) {
      return;
    }
    protocol.handle(PET_ASSET_PROTOCOL.slice(0, -1), async (request) => {
      const parsed = parsePetAssetUrl(request.url);
      if (!parsed || parsed.assetName !== "spritesheet") {
        return new Response("Not found", { status: 404 });
      }
      const assetPath = this.petStore.getPetAssetPath(parsed.petId);
      if (!assetPath) {
        console.warn(
          `[awaken-desktop] pet asset is not registered: ${parsed.petId}`,
        );
        return new Response("Not found", { status: 404 });
      }
      try {
        const data = await fs.readFile(assetPath);
        const ext = assetPath.endsWith(".png") ? "png" : "webp";
        return new Response(data, {
          headers: {
            "Content-Type": ext === "png" ? "image/png" : "image/webp",
            "Content-Security-Policy": "default-src 'none'; img-src 'self';",
          },
        });
      } catch (error) {
        console.warn(
          `[awaken-desktop] failed to read pet asset: ${
            error instanceof Error ? error.message : "unknown error"
          }`,
        );
        return new Response("Not found", { status: 404 });
      }
    });
  }

  private registerIpcHandlers(): void {
    if (this.registeredHandlers) return;
    this.registeredHandlers = true;

    ipcMain.handle(PET_IPC_CHANNELS.petOverlayBootstrap, async () => {
      const settings = this.petStore.getSettings();
      const manifest = settings.selectedPetId
        ? this.petStore.getAllManifests().find((m) => m.id === settings.selectedPetId)
        : undefined;
      const bootstrap: PetOverlayBootstrap = {
        petId: manifest?.id ?? null,
        displayName: manifest?.displayName ?? null,
        spriteVersionNumber: manifest?.spriteVersionNumber ?? 1,
        spritesheetUrl: manifest
          ? petAssetUrl(manifest.id, manifest.sourceFingerprint)
          : null,
        mascotWidthPx: settings.mascotWidthPx,
        isOpen: this.isOpen(),
        welcomedPetIds: settings.welcomedPetIds,
        dismissedNotificationKeys: settings.dismissedNotificationKeys,
      };
      return bootstrap;
    });

    ipcMain.on(PET_IPC_CHANNELS.petPointerInteractive, (_event, interactive: boolean) => {
      if (!this.isOpen() || this.supportsWindowShape()) return;
      this.window?.setIgnoreMouseEvents(!interactive, interactive ? undefined : { forward: true });
    });

    ipcMain.on(PET_IPC_CHANNELS.petDragStart, (_event, point: unknown) => {
      if (!this.isOpen()) return;
      const rawBounds = this.window!.getBounds();
      const stableBounds = stabilizePetWindowBounds(rawBounds, this.latestWindowSize);
      if (
        rawBounds.x !== stableBounds.x ||
        rawBounds.y !== stableBounds.y ||
        rawBounds.width !== stableBounds.width ||
        rawBounds.height !== stableBounds.height
      ) {
        this.window!.setBounds(stableBounds, false);
      }
      const cursor = electronScreen.getCursorScreenPoint();
      const bounds = this.window!.getBounds();
      const rendererPoint = isFinitePoint(point) ? point : cursor;
      this.dragOrigin = {
        rendererX: rendererPoint.x,
        rendererY: rendererPoint.y,
        cursorX: cursor.x,
        cursorY: cursor.y,
        windowX: bounds.x,
        windowY: bounds.y,
        windowHeight: bounds.height,
        lastLogAtMs: 0,
      };
      this.debugPet("drag:start", {
        rendererPoint,
        cursor,
        bounds,
        display: this.describeDisplayForBounds(bounds),
      });
    });

    ipcMain.on(PET_IPC_CHANNELS.petDragMove, (_event, point: unknown) => {
      if (!this.isOpen() || !this.dragOrigin) return;
      const rendererPoint = isFinitePoint(point) ? point : null;
      const cursor = electronScreen.getCursorScreenPoint();
      const dx = rendererPoint
        ? rendererPoint.x - this.dragOrigin.rendererX
        : cursor.x - this.dragOrigin.cursorX;
      const dy = rendererPoint
        ? rendererPoint.y - this.dragOrigin.rendererY
        : cursor.y - this.dragOrigin.cursorY;
      const nextX = Math.round(this.dragOrigin.windowX + dx);
      const boundsBefore = this.window!.getBounds();
      const nextY = resolveBottomAnchoredDragTop(
        this.dragOrigin.windowY,
        this.dragOrigin.windowHeight,
        dy,
        boundsBefore.height,
      );
      const unclampedBounds = { x: nextX, y: nextY, ...this.latestWindowSize };
      const targetDisplay = electronScreen.getDisplayMatching(unclampedBounds);
      const targetBounds = this.clampBoundsToDisplay(
        unclampedBounds,
        targetDisplay.bounds,
      );
      this.window!.setBounds(targetBounds, false);
      let boundsAfter = this.window!.getBounds();
      const correctedY = resolveBottomAnchoredDragTop(
        this.dragOrigin.windowY,
        this.dragOrigin.windowHeight,
        dy,
        boundsAfter.height,
      );
      const correctedBounds = this.clampBoundsToDisplay(
        { x: targetBounds.x, y: correctedY, ...this.latestWindowSize },
        targetDisplay.bounds,
      );
      if (
        Math.abs(boundsAfter.y - correctedBounds.y) > 0 ||
        boundsAfter.width !== this.latestWindowSize.width ||
        boundsAfter.height !== this.latestWindowSize.height
      ) {
        this.window!.setBounds(correctedBounds, false);
        boundsAfter = this.window!.getBounds();
      }
      const now = Date.now();
      if (now - this.dragOrigin.lastLogAtMs > 120) {
        this.dragOrigin.lastLogAtMs = now;
        this.debugPet("drag:move", {
          rendererPoint,
          cursor,
          rendererDelta: rendererPoint
            ? {
                x: rendererPoint.x - this.dragOrigin.rendererX,
                y: rendererPoint.y - this.dragOrigin.rendererY,
              }
            : null,
          cursorDelta: {
            x: cursor.x - this.dragOrigin.cursorX,
            y: cursor.y - this.dragOrigin.cursorY,
          },
          nextPosition: { x: nextX, y: nextY },
          targetBounds,
          correctedPosition: { x: correctedBounds.x, y: correctedBounds.y },
          boundsBefore,
          boundsAfter,
          display: this.describeDisplayForBounds(boundsBefore),
        });
      }
    });

    ipcMain.on(PET_IPC_CHANNELS.petDragEnd, (_event, point: unknown) => {
      if (this.isOpen()) {
        this.debugPet("drag:end", {
          rendererPoint: isFinitePoint(point) ? point : null,
          cursor: electronScreen.getCursorScreenPoint(),
          bounds: this.window!.getBounds(),
        });
      }
      this.dragOrigin = null;
      this.savePlacement();
    });

    ipcMain.on(PET_IPC_CHANNELS.petResizeMascot, (_event, width: number) => {
      if (!this.isOpen()) return;
      this.window!.webContents.send(PET_IPC_CHANNELS.petResizeMascot, width);
    });

    ipcMain.on(PET_IPC_CHANNELS.petFinishResize, (_event, width: number) => {
      void this.finishResize(width).catch((error: unknown) => {
        console.error("[awaken-desktop] failed to persist pet size", error);
      });
    });

    ipcMain.on(
      PET_IPC_CHANNELS.petReportBounds,
      (_event, bounds: { width: number; height: number }) => {
        this.updateInteractiveRegion(bounds.width, bounds.height);
      },
    );

    ipcMain.handle(
      PET_IPC_CHANNELS.petActivateNotification,
      async (_event, notificationId: string) => {
        const session = this.latestSnapshot.sessions.find(
          (candidate) => candidate.key === notificationId,
        );
        if (session?.threadId) {
          this.activateThread(session.threadId);
        } else {
          this.focusMainWindow();
        }
      },
    );

    ipcMain.handle(
      PET_IPC_CHANNELS.petPerformNotificationAction,
      async (_event, request: PetNotificationActionRequest) => {
        if (request.type === "stop-run") {
          await this.cancelRun(request.runId);
          return;
        }
        // Other action types (approvals, answers, follow-ups) require the main
        // window's UI context; fall back to focusing it.
        this.focusMainWindow();
      },
    );

    ipcMain.handle(
      PET_IPC_CHANNELS.petDismissNotification,
      async (_event, input: { notificationId: string; turnKey: string | null }) => {
        const key = input.turnKey
          ? `${input.notificationId}:${input.turnKey}`
          : input.notificationId;
        await this.petStore.recordDismissedNotificationKeys([key]);
      },
    );

    ipcMain.handle(PET_IPC_CHANNELS.petRecordPetWelcomed, async (_event, petId: string) => {
      await this.petStore.recordWelcomedPet(petId);
    });

    ipcMain.on(PET_IPC_CHANNELS.petOpenMainWindow, () => {
      this.focusMainWindow();
    });

    ipcMain.on(PET_IPC_CHANNELS.petCloseOverlay, () => {
      void this.closeOverlay().catch((error: unknown) => {
        console.error("[awaken-desktop] failed to close pet overlay", error);
      });
    });
  }

  private unregisterIpcHandlers(): void {
    // IPC handlers are global; we avoid removing them because other windows may still rely on them.
    // In a future refactor we could track registration count.
  }

  private resolvePlacement(): Rectangle {
    const settings = this.petStore.getSettings();
    const displays = electronScreen.getAllDisplays();
    const primary = electronScreen.getPrimaryDisplay();

    const saved = Object.values(settings.placementByDisplay).sort(
      (a, b) => Date.parse(b.updatedAt) - Date.parse(a.updatedAt),
    )[0];
    if (saved) {
      const matchingDisplay = displays.find(
        (d) =>
          d.id.toString() === saved.displayId ||
          this.workAreaNearlyEqual(d.workArea, saved.workArea),
      );
      if (matchingDisplay) {
        const compactBounds = resolveSavedPetWindowBounds(saved.windowBounds);
        return this.clampBoundsToDisplay(compactBounds, matchingDisplay.bounds);
      }
    }

    const displayBounds = primary.bounds;
    const initialSize = this.resolveInitialWindowSize();
    return this.clampBoundsToDisplay(
      {
        x: displayBounds.x + displayBounds.width - initialSize.width - 24,
        y: displayBounds.y + displayBounds.height - initialSize.height - 24,
        ...initialSize,
      },
      displayBounds,
    );
  }

  private resolveInitialWindowSize(): OverlaySize {
    const mascotWidth = this.petStore.getSettings().mascotWidthPx;
    const mascotHeight = Math.round((mascotWidth / 192) * 208);
    return resolvePetOverlayWindowSize(mascotWidth + 18, mascotHeight + 22);
  }

  private clampBoundsToDisplay(bounds: Rectangle, displayBounds: Rectangle): Rectangle {
    return clampPetOverlayBoundsToWorkArea(
      bounds,
      displayBounds,
      this.latestInteractiveRegion,
    );
  }

  private workAreaNearlyEqual(a: Rectangle, b: Rectangle): boolean {
    return (
      Math.abs(a.x - b.x) < 10 &&
      Math.abs(a.y - b.y) < 10 &&
      Math.abs(a.width - b.width) < 50 &&
      Math.abs(a.height - b.height) < 50
    );
  }

  private updateInteractiveRegion(width: number, height: number): void {
    if (!this.isOpen()) return;
    const windowSize = resolvePetOverlayWindowSize(width, height);
    const currentBounds = this.window!.getBounds();
    const nextBounds = resizePetWindowBoundsFromBottomRight(currentBounds, windowSize);
    const display = electronScreen.getDisplayMatching(nextBounds);
    const clampedBounds = clampPetOverlayBoundsToWorkArea(
      nextBounds,
      display.bounds,
      { x: 0, y: 0, ...windowSize },
    );
    if (
      currentBounds.x !== clampedBounds.x ||
      currentBounds.y !== clampedBounds.y ||
      currentBounds.width !== clampedBounds.width ||
      currentBounds.height !== clampedBounds.height
    ) {
      this.window!.setBounds(clampedBounds, false);
    }
    this.latestWindowSize = { width: clampedBounds.width, height: clampedBounds.height };
    this.latestInteractiveRegion = { x: 0, y: 0, ...this.latestWindowSize };
    this.debugPet("shape:update", {
      reportedBounds: { width, height },
      windowSize,
      shape: this.latestInteractiveRegion,
      windowBoundsBefore: currentBounds,
      windowBoundsAfter: this.window!.getBounds(),
      supportsShape: this.supportsWindowShape(),
    });
    if (this.supportsWindowShape()) {
      this.window!.setShape([this.latestInteractiveRegion]);
      this.window!.setIgnoreMouseEvents(false);
    }
  }

  private supportsWindowShape(): boolean {
    return process.platform === "win32" || process.platform === "linux";
  }

  private buildPlacement(): PetOverlayPlacement {
    if (!this.isOpen()) {
      const settings = this.petStore.getSettings();
      return (
        Object.values(settings.placementByDisplay).sort(
          (a, b) => Date.parse(b.updatedAt) - Date.parse(a.updatedAt),
        )[0] ?? {
          displayId: electronScreen.getPrimaryDisplay().id.toString(),
          workArea: electronScreen.getPrimaryDisplay().workArea,
          windowBounds: { x: 0, y: 0, ...minOverlaySize },
          mascotWidthPx: settings.mascotWidthPx,
          updatedAt: new Date().toISOString(),
        }
      );
    }
    const display = electronScreen.getDisplayMatching(this.window!.getBounds());
    return {
      displayId: display.id.toString(),
      workArea: display.workArea,
      windowBounds: normalizePetWindowBoundsForPlacement(this.window!.getBounds()),
      mascotWidthPx: this.petStore.getSettings().mascotWidthPx,
      updatedAt: new Date().toISOString(),
    };
  }

  private async cancelRun(runId: string): Promise<void> {
    const baseUrl = this.getRuntimeBaseUrl();
    if (!baseUrl) {
      throw new Error("Runtime unavailable");
    }
    const url = new URL(
      `/v1/runs/${encodeURIComponent(runId)}/cancel`,
      baseUrl.endsWith("/") ? baseUrl : `${baseUrl}/`,
    ).toString();
    const response = await fetch(url, { method: "POST" });
    if (!response.ok) {
      throw new Error(await readErrorMessage(response));
    }
  }

  private savePlacement(): void {
    const placement = this.buildPlacement();
    this.debugPet("placement:save", {
      savedBounds: placement.windowBounds,
      actualBounds: this.isOpen() ? this.window!.getBounds() : null,
    });
    void this.petStore.setPlacement(placement).catch((error: unknown) => {
      console.error("[awaken-desktop] failed to save pet placement", error);
    });
  }

  private debugPet(event: string, details: Record<string, unknown>): void {
    if (app.isPackaged && process.env.AWAKEN_PET_DEBUG !== "1") return;
    console.debug(`[awaken-pet-debug] ${event}`, JSON.stringify(details));
  }

  private describeDisplayForBounds(bounds: Rectangle): Record<string, unknown> {
    const display = electronScreen.getDisplayMatching(bounds);
    return {
      id: display.id,
      scaleFactor: display.scaleFactor,
      workArea: display.workArea,
    };
  }

  private buildBootstrap(manifest: PetManifest): PetOverlayBootstrap {
    const settings = this.petStore.getSettings();
    return {
      petId: manifest.id,
      displayName: manifest.displayName,
      spriteVersionNumber: manifest.spriteVersionNumber,
      spritesheetUrl: petAssetUrl(manifest.id, manifest.sourceFingerprint),
      mascotWidthPx: settings.mascotWidthPx,
      isOpen: true,
      welcomedPetIds: settings.welcomedPetIds,
      dismissedNotificationKeys: settings.dismissedNotificationKeys,
    };
  }

  private async closeOverlay(): Promise<void> {
    await this.petStore.setOverlayOpen(false);
    this.hide();
  }

  private async finishResize(width: number): Promise<void> {
    await this.petStore.setMascotWidthPx(width);
    await this.petStore.setPlacement(this.buildPlacement());
  }
}
