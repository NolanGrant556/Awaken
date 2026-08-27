import { describe, expect, it, vi } from "vitest";
import {
  pickPluginSource,
  pluginSourceOpenDialogOptions,
  validatePluginSourcePath,
} from "./plugin-source-picker.js";

function stats(kind: "directory" | "file") {
  return {
    isDirectory: () => kind === "directory",
    isFile: () => kind === "file",
  };
}

describe("plugin source picker", () => {
  it("lets the macOS dialog select either a directory or a ZIP file", async () => {
    const showOpenDialog = vi.fn(async () => ({
      canceled: false,
      filePaths: ["/Users/test/Desktop/acme.zip"],
    }));
    const showMessageBox = vi.fn();

    await expect(
      pickPluginSource(
        { showOpenDialog, showMessageBox },
        {
          platform: "darwin",
          defaultPath: "/Users/test/Desktop",
          statPath: async () => stats("file"),
        },
      ),
    ).resolves.toEqual({
      canceled: false,
      path: "/Users/test/Desktop/acme.zip",
    });
    expect(showMessageBox).not.toHaveBeenCalled();
    expect(showOpenDialog).toHaveBeenCalledWith({
      title: "选择插件来源",
      buttonLabel: "选择",
      properties: ["openFile", "openDirectory"],
      defaultPath: "/Users/test/Desktop",
      filters: [{ name: "插件 ZIP 包", extensions: ["zip"] }],
    });
  });

  it("uses an explicit source-kind choice on platforms without mixed native pickers", async () => {
    const showOpenDialog = vi.fn(async () => ({
      canceled: false,
      filePaths: ["C:\\plugins\\acme.zip"],
    }));
    const showMessageBox = vi.fn(async () => ({
      response: 1,
      checkboxChecked: false,
    }));

    await expect(
      pickPluginSource(
        { showOpenDialog, showMessageBox },
        {
          platform: "win32",
          statPath: async () => stats("file"),
        },
      ),
    ).resolves.toEqual({
      canceled: false,
      path: "C:\\plugins\\acme.zip",
    });
    expect(showOpenDialog).toHaveBeenCalledWith(
      pluginSourceOpenDialogOptions("zip"),
    );
  });

  it("accepts directories and ZIP files but rejects other files", async () => {
    await expect(
      validatePluginSourcePath("/tmp/acme", async () => stats("directory")),
    ).resolves.toEqual({ canceled: false, path: "/tmp/acme" });
    await expect(
      validatePluginSourcePath("/tmp/acme.ZIP", async () => stats("file")),
    ).resolves.toEqual({ canceled: false, path: "/tmp/acme.ZIP" });
    await expect(
      validatePluginSourcePath("/tmp/acme.txt", async () => stats("file")),
    ).resolves.toEqual({
      canceled: true,
      error: "请选择插件目录或 .zip 文件",
    });
    await expect(
      validatePluginSourcePath("/tmp/missing.zip", async () => {
        throw new Error("missing");
      }),
    ).resolves.toEqual({
      canceled: true,
      error: "无法访问所选插件来源",
    });
  });

  it("returns a canceled result when either chooser is canceled", async () => {
    const showOpenDialog = vi.fn(async () => ({ canceled: true, filePaths: [] }));
    const showMessageBox = vi.fn(async () => ({
      response: 2,
      checkboxChecked: false,
    }));

    await expect(
      pickPluginSource({ showOpenDialog, showMessageBox }, { platform: "linux" }),
    ).resolves.toEqual({ canceled: true });
    expect(showOpenDialog).not.toHaveBeenCalled();

    await expect(
      pickPluginSource(
        { showOpenDialog, showMessageBox: vi.fn() },
        { platform: "darwin" },
      ),
    ).resolves.toEqual({ canceled: true });
  });
});
