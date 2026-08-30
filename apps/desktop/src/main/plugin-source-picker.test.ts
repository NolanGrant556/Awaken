import { describe, expect, it, vi } from "vitest";
import {
  pickPluginSource,
  resolveDroppedPluginSourcePath,
  validatePluginSourcePath,
} from "./plugin-source-picker.js";

function stats(kind: "file" | "other") {
  return {
    isFile: () => kind === "file",
  };
}

describe("plugin source picker", () => {
  it("opens a ZIP-only file picker on every platform", async () => {
    const showOpenDialog = vi.fn(async () => ({
      canceled: false,
      filePaths: ["/Users/test/Desktop/acme.zip"],
    }));

    await expect(
      pickPluginSource(
        { showOpenDialog },
        {
          defaultPath: "/Users/test/Desktop",
          statPath: async () => stats("file"),
        },
      ),
    ).resolves.toEqual({
      canceled: false,
      path: "/Users/test/Desktop/acme.zip",
    });
    expect(showOpenDialog).toHaveBeenCalledWith({
      title: "选择插件 ZIP 包",
      buttonLabel: "选择 ZIP",
      properties: ["openFile"],
      defaultPath: "/Users/test/Desktop",
      filters: [{ name: "插件 ZIP 包", extensions: ["zip"] }],
    });
  });

  it("accepts ZIP files but rejects directories and other files", async () => {
    await expect(
      validatePluginSourcePath("/tmp/acme", async () => stats("other")),
    ).resolves.toEqual({ canceled: true, error: "请选择 .zip 插件包" });
    await expect(
      validatePluginSourcePath("/tmp/acme.ZIP", async () => stats("file")),
    ).resolves.toEqual({ canceled: false, path: "/tmp/acme.ZIP" });
    await expect(
      validatePluginSourcePath("/tmp/acme.txt", async () => stats("file")),
    ).resolves.toEqual({
      canceled: true,
      error: "请选择 .zip 插件包",
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

  it("returns a canceled result when the file chooser is canceled", async () => {
    const showOpenDialog = vi.fn(async () => ({ canceled: true, filePaths: [] }));

    await expect(
      pickPluginSource({ showOpenDialog }),
    ).resolves.toEqual({ canceled: true });
  });

  it("applies the same ZIP-only validation to dropped file paths", async () => {
    await expect(resolveDroppedPluginSourcePath()).resolves.toEqual({
      canceled: true,
      error: "无法访问拖入的插件 ZIP",
    });
    await expect(
      resolveDroppedPluginSourcePath("/tmp/dropped.txt", async () => stats("file")),
    ).resolves.toEqual({ canceled: true, error: "请选择 .zip 插件包" });
    await expect(
      resolveDroppedPluginSourcePath("/tmp/dropped.zip", async () => stats("file")),
    ).resolves.toEqual({ canceled: false, path: "/tmp/dropped.zip" });
  });
});
