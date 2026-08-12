import { beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  accessSync: vi.fn(),
  existsSync: vi.fn(),
  getAppPath: vi.fn(),
  openExternal: vi.fn(),
}));

vi.mock("node:fs", () => ({
  accessSync: mocks.accessSync,
  constants: { X_OK: 1 },
  existsSync: mocks.existsSync,
}));

vi.mock("electron", () => ({
  app: { getAppPath: mocks.getAppPath },
  shell: { openExternal: mocks.openExternal },
}));

import {
  openComputerPermissionSettings,
  prepareComputerRuntime,
} from "./computer-setup.js";

describe("prepareComputerRuntime", () => {
  beforeEach(() => {
    mocks.accessSync.mockReset();
    mocks.existsSync.mockReset();
    mocks.getAppPath.mockReset();
    mocks.openExternal.mockReset();
    mocks.openExternal.mockResolvedValue(undefined);
    mocks.getAppPath.mockReturnValue(
      "/Applications/Awaken.app/Contents/Resources/app.asar",
    );
    mocks.existsSync.mockImplementation(
      (path: string) => !path.endsWith("scripts/prepare-node-runtime.mjs"),
    );
  });

  it("returns the shared snake_case IPC contract when bundled resources are ready", async () => {
    await expect(prepareComputerRuntime()).resolves.toEqual({
      node_ready: true,
      cua_host_ready: true,
    });
  });

  it("uses the same IPC contract when packaged resources are missing", async () => {
    mocks.existsSync.mockImplementation((path: string) => {
      if (path.endsWith("scripts/prepare-node-runtime.mjs")) return false;
      return !path.includes("cua-driver");
    });

    await expect(prepareComputerRuntime()).resolves.toEqual({
      node_ready: true,
      cua_host_ready: false,
      message:
        "打包应用缺少 bundled Node runtime 或 CUA Host，请重新安装完整安装包。",
    });
  });

  it.runIf(process.platform === "darwin")(
    "opens the exact macOS privacy panes for both Computer permissions",
    async () => {
      await openComputerPermissionSettings("screen_recording");
      await openComputerPermissionSettings("accessibility");

      expect(mocks.openExternal).toHaveBeenNthCalledWith(
        1,
        "x-apple.systempreferences:com.apple.preference.security?Privacy_ScreenCapture",
      );
      expect(mocks.openExternal).toHaveBeenNthCalledWith(
        2,
        "x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility",
      );
    },
  );
});
