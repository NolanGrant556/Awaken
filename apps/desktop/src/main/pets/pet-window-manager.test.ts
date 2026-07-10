import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";
import fs from "node:fs/promises";
import path from "node:path";
import os from "node:os";
import { PetStore } from "./pet-store.js";
import {
  PET_OVERLAY_MAX_HEIGHT,
  PET_OVERLAY_MAX_WIDTH,
  PetWindowManager,
  clampPetOverlayBoundsToWorkArea,
  normalizePetWindowBoundsForPlacement,
  resolveBottomAnchoredDragTop,
  resolvePetOverlayShape,
  resolveSavedPetWindowBounds,
  resolvePetWindowPaths,
  stabilizePetWindowBounds,
  sanitizePetOverlayContentSize,
} from "./pet-window-manager.js";

vi.mock("electron", () => ({
  app: {
    isPackaged: false,
    getPath: (name: string) => {
      if (name === "home") return "/home/user";
      return `/tmp/${name}`;
    },
  },
  BrowserWindow: vi.fn().mockImplementation(() => ({
    webContents: {
      id: 1,
      send: vi.fn(),
      on: vi.fn(),
      setWindowOpenHandler: vi.fn(),
    },
    setPosition: vi.fn(),
    setBounds: vi.fn(),
    setShape: vi.fn(),
    getBounds: vi.fn(() => ({ x: 100, y: 100, width: 384, height: 400 })),
    setIgnoreMouseEvents: vi.fn(),
    setVisibleOnAllWorkspaces: vi.fn(),
    setAlwaysOnTop: vi.fn(),
    show: vi.fn(),
    hide: vi.fn(),
    close: vi.fn(),
    isDestroyed: vi.fn(() => false),
    isVisible: vi.fn(() => true),
    loadURL: vi.fn().mockResolvedValue(undefined),
    loadFile: vi.fn().mockResolvedValue(undefined),
  })),
  ipcMain: {
    handle: vi.fn(),
    on: vi.fn(),
  },
  protocol: {
    isProtocolHandled: vi.fn(() => false),
    handle: vi.fn(),
  },
  screen: {
    on: vi.fn(),
    removeListener: vi.fn(),
    getAllDisplays: vi.fn(() => [
      {
        id: 1,
        bounds: { x: 0, y: 0, width: 1920, height: 1080 },
        workArea: { x: 0, y: 0, width: 1920, height: 1080 },
      },
    ]),
    getPrimaryDisplay: vi.fn(() => ({
      id: 1,
      bounds: { x: 0, y: 0, width: 1920, height: 1080 },
      workArea: { x: 0, y: 0, width: 1920, height: 1080 },
    })),
    getDisplayMatching: vi.fn(() => ({
      id: 1,
      scaleFactor: 1,
      bounds: { x: 0, y: 0, width: 1920, height: 1080 },
      workArea: { x: 0, y: 0, width: 1920, height: 1080 },
    })),
  },
  shell: {
    openExternal: vi.fn(),
  },
}));

async function createTempDir(): Promise<string> {
  return fs.mkdtemp(path.join(os.tmpdir(), "awaken-pet-window-"));
}

describe("PetWindowManager", () => {
  let tempDir: string;
  let configDir: string;
  let dataDir: string;
  let store: PetStore;

  beforeEach(async () => {
    tempDir = await createTempDir();
    configDir = path.join(tempDir, "config");
    dataDir = path.join(tempDir, "userData");
    store = new PetStore(configDir, dataDir);
    await store.initialize();
  });

  afterEach(async () => {
    await fs.rm(tempDir, { recursive: true, force: true });
  });

  it("initializes without creating a window", async () => {
    const manager = new PetWindowManager({
      petStore: store,
      focusMainWindow: vi.fn(),
      activateThread: vi.fn(),
      getRuntimeBaseUrl: () => null,
    });
    await manager.initialize();
    expect(manager.isOpen()).toBe(false);
  });

  it("does not show when no pet is selected", async () => {
    const manager = new PetWindowManager({
      petStore: store,
      focusMainWindow: vi.fn(),
      activateThread: vi.fn(),
      getRuntimeBaseUrl: () => null,
    });
    await manager.initialize();
    await manager.show();
    expect(manager.isOpen()).toBe(false);
  });
});

describe("resolvePetWindowPaths", () => {
  it("resolves preload and renderer from the nested main/pets output directory", () => {
    const baseDir = path.join("app", "dist", "main", "pets");
    const paths = resolvePetWindowPaths(baseDir);
    expect(paths.preloadPath).toBe(
      path.join("app", "dist", "preload", "pet-overlay.js"),
    );
    expect(paths.rendererPath).toBe(
      path.join("app", "dist", "renderer", "pet-overlay.html"),
    );
  });
});

describe("sanitizePetOverlayContentSize", () => {
  it("caps runaway renderer measurements to the pet overlay viewport", () => {
    expect(sanitizePetOverlayContentSize(3_660, 3_548)).toEqual({
      width: PET_OVERLAY_MAX_WIDTH,
      height: PET_OVERLAY_MAX_HEIGHT,
    });
  });

  it("uses a bounded fallback for invalid measurements", () => {
    expect(
      sanitizePetOverlayContentSize(Number.NaN, Number.POSITIVE_INFINITY, {
        width: 220,
        height: 260,
      }),
    ).toEqual({ width: 220, height: 260 });
  });
});

describe("resolvePetOverlayShape", () => {
  it("keeps a slightly padded interactive region at the bottom-left of the fixed window", () => {
    expect(resolvePetOverlayShape(120, 127)).toEqual({
      x: 0,
      y: PET_OVERLAY_MAX_HEIGHT - 129,
      width: 122,
      height: 129,
    });
  });

  it("does not grow beyond the fixed overlay window", () => {
    expect(resolvePetOverlayShape(PET_OVERLAY_MAX_WIDTH, PET_OVERLAY_MAX_HEIGHT)).toEqual({
      x: 0,
      y: 0,
      width: PET_OVERLAY_MAX_WIDTH,
      height: PET_OVERLAY_MAX_HEIGHT,
    });
  });
});

describe("pet overlay placement bounds", () => {
  it("stores a stable logical size even when Electron reports DPI-inflated bounds", () => {
    expect(
      normalizePetWindowBoundsForPlacement({
        x: 1307,
        y: 181,
        width: 477,
        height: 493,
      }),
    ).toEqual({
      x: 1307,
      y: 181,
      width: PET_OVERLAY_MAX_WIDTH,
      height: PET_OVERLAY_MAX_HEIGHT,
    });
  });

  it("does not bottom-anchor saved DPI-inflated bounds on restore", () => {
    expect(
      resolveSavedPetWindowBounds({
        x: 1307,
        y: 181,
        width: 477,
        height: 493,
      }),
    ).toEqual({
      x: 1307,
      y: 181,
      width: PET_OVERLAY_MAX_WIDTH,
      height: PET_OVERLAY_MAX_HEIGHT,
    });
  });

  it("bottom-anchors legacy runaway saved bounds on restore", () => {
    expect(
      resolveSavedPetWindowBounds({
        x: 1307,
        y: 181,
        width: 3816,
        height: 3706,
      }),
    ).toEqual({
      x: 1307,
      y: 181 + 3706 - PET_OVERLAY_MAX_HEIGHT,
      width: PET_OVERLAY_MAX_WIDTH,
      height: PET_OVERLAY_MAX_HEIGHT,
    });
  });
});

describe("resolveBottomAnchoredDragTop", () => {
  it("keeps the bottom edge stable when the observed window height grows", () => {
    expect(resolveBottomAnchoredDragTop(181, 403, 0, 452)).toBe(132);
  });

  it("applies pointer y movement relative to the stable bottom edge", () => {
    expect(resolveBottomAnchoredDragTop(181, 403, 20, 452)).toBe(152);
  });
});

describe("stabilizePetWindowBounds", () => {
  it("shrinks a drifted overlay window while preserving the bottom edge", () => {
    expect(
      stabilizePetWindowBounds({
        x: 1612,
        y: 507,
        width: 488,
        height: 525,
      }),
    ).toEqual({
      x: 1612,
      y: 632,
      width: PET_OVERLAY_MAX_WIDTH,
      height: PET_OVERLAY_MAX_HEIGHT,
    });
  });
});

describe("clampPetOverlayBoundsToWorkArea", () => {
  it("keeps the pet overlay inside the display work area", () => {
    expect(
      clampPetOverlayBoundsToWorkArea(
        { x: 1662, y: 759, width: PET_OVERLAY_MAX_WIDTH, height: PET_OVERLAY_MAX_HEIGHT },
        { x: 0, y: 0, width: 2048, height: 1104 },
      ),
    ).toEqual({
      x: 1662,
      y: 704,
      width: PET_OVERLAY_MAX_WIDTH,
      height: PET_OVERLAY_MAX_HEIGHT,
    });
  });

  it("allows transparent window margins outside the display when the visible region stays inside", () => {
    expect(
      clampPetOverlayBoundsToWorkArea(
        { x: 2000, y: -400, width: PET_OVERLAY_MAX_WIDTH, height: PET_OVERLAY_MAX_HEIGHT },
        { x: 0, y: 0, width: 2048, height: 1104 },
        { x: 0, y: 270, width: 120, height: 130 },
      ),
    ).toEqual({
      x: 1928,
      y: -270,
      width: PET_OVERLAY_MAX_WIDTH,
      height: PET_OVERLAY_MAX_HEIGHT,
    });
  });
});
