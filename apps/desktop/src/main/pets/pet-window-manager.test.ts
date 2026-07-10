import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";
import fs from "node:fs/promises";
import path from "node:path";
import os from "node:os";
import { PetStore } from "./pet-store.js";
import { BrowserWindow } from "electron";
import {
  applyPetWindowVisibilityPolicy,
  PetWindowManager,
  resolvePetWindowPaths,
} from "./pet-window-manager.js";

vi.mock("electron", () => ({
  app: {
    isPackaged: false,
    getPath: (name: string) => {
      if (name === "home") return "/home/user";
      return `/tmp/${name}`;
    },
  },
  BrowserWindow: vi.fn().mockImplementation(function BrowserWindowMock() {
    return {
      webContents: {
        id: 1,
        send: vi.fn(),
        on: vi.fn(),
        setWindowOpenHandler: vi.fn(),
      },
      on: vi.fn(),
      setPosition: vi.fn(),
      setBounds: vi.fn(),
      setShape: vi.fn(),
      getBounds: vi.fn(() => ({ x: 100, y: 100, width: 384, height: 400 })),
      setIgnoreMouseEvents: vi.fn(),
      setVisibleOnAllWorkspaces: vi.fn(),
      setAlwaysOnTop: vi.fn(),
      show: vi.fn(),
      showInactive: vi.fn(),
      hide: vi.fn(),
      close: vi.fn(),
      moveTop: vi.fn(),
      isDestroyed: vi.fn(() => false),
      isVisible: vi.fn(() => true),
      isMinimized: vi.fn(() => false),
      restore: vi.fn(),
      loadURL: vi.fn().mockResolvedValue(undefined),
      loadFile: vi.fn().mockResolvedValue(undefined),
    };
  }),
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

async function createSpritesheet(dir: string, name = "spritesheet.webp"): Promise<string> {
  const file = path.join(dir, name);
  // A minimal valid WebP image (1x1 pixel, lossy).
  const webp = Buffer.from(
    "524946461a0000005745425056503820 0d0000009d012a0000fa00ba017d01",
    "hex",
  );
  await fs.writeFile(file, webp);
  return file;
}

describe("applyPetWindowVisibilityPolicy", () => {
  it("keeps the macOS app process type intact while preserving fullscreen visibility", () => {
    const window = {
      setVisibleOnAllWorkspaces: vi.fn(),
      setAlwaysOnTop: vi.fn(),
      moveTop: vi.fn(),
    };

    applyPetWindowVisibilityPolicy(window, "darwin", true);

    expect(window.setVisibleOnAllWorkspaces).toHaveBeenCalledWith(true, {
      visibleOnFullScreen: true,
      skipTransformProcessType: true,
    });
    expect(window.setAlwaysOnTop).toHaveBeenCalledWith(true, "screen-saver");
    expect(window.moveTop).toHaveBeenCalledOnce();
  });

  it("preserves the existing Windows visibility policy", () => {
    const window = {
      setVisibleOnAllWorkspaces: vi.fn(),
      setAlwaysOnTop: vi.fn(),
      moveTop: vi.fn(),
    };

    applyPetWindowVisibilityPolicy(window, "win32");

    expect(window.setVisibleOnAllWorkspaces).not.toHaveBeenCalled();
    expect(window.setAlwaysOnTop).toHaveBeenCalledWith(true, "screen-saver");
    expect(window.moveTop).not.toHaveBeenCalled();
  });
});

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

  it("creates a screen-recording resilient overlay when a pet is selected", async () => {
    const spritesheet = await createSpritesheet(tempDir);
    await store.importOrUpdatePet({
      source: "codex-custom",
      sourceId: "my-pet",
      displayName: "My Pet",
      description: "A pet.",
      spritesheetSourcePath: spritesheet,
      sourcePath: "/fake/codex/pets/my-pet",
    });
    await store.selectPet("my-pet");

    const manager = new PetWindowManager({
      petStore: store,
      focusMainWindow: vi.fn(),
      activateThread: vi.fn(),
      getRuntimeBaseUrl: () => null,
    });
    await manager.initialize();
    await manager.show();

    expect(manager.isOpen()).toBe(true);
    const ctorCalls = vi.mocked(BrowserWindow).mock.calls;
    const lastCall = ctorCalls[ctorCalls.length - 1]?.[0];
    expect(lastCall).toMatchObject({
      paintWhenInitiallyHidden: true,
      webPreferences: expect.objectContaining({
        backgroundThrottling: false,
      }),
    });

    const win = vi.mocked(BrowserWindow).mock.results[ctorCalls.length - 1]?.value;
    expect(win.showInactive).toHaveBeenCalled();
    expect(win.setAlwaysOnTop).toHaveBeenCalledWith(true, "screen-saver");
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
