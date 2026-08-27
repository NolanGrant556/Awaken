import fs from "node:fs/promises";
import path from "node:path";
import type {
  MessageBoxOptions,
  MessageBoxReturnValue,
  OpenDialogOptions,
  OpenDialogReturnValue,
} from "electron";
import type { WorkspacePickResult } from "../shared/runtime.js";

type PluginSourceDialog = {
  showMessageBox(options: MessageBoxOptions): Promise<MessageBoxReturnValue>;
  showOpenDialog(options: OpenDialogOptions): Promise<OpenDialogReturnValue>;
};

type PluginSourceStats = {
  isDirectory(): boolean;
  isFile(): boolean;
};

type PluginSourcePickerOptions = {
  defaultPath?: string;
  platform?: NodeJS.Platform;
  statPath?: (candidate: string) => Promise<PluginSourceStats>;
};

export type PluginSourceSelectionKind = "mixed" | "directory" | "zip";

export function pluginSourceOpenDialogOptions(
  kind: PluginSourceSelectionKind,
  defaultPath?: string,
): OpenDialogOptions {
  const properties: OpenDialogOptions["properties"] =
    kind === "mixed"
      ? ["openFile", "openDirectory"]
      : kind === "directory"
        ? ["openDirectory"]
        : ["openFile"];
  return {
    title: "选择插件来源",
    buttonLabel: "选择",
    properties,
    ...(defaultPath ? { defaultPath } : {}),
    ...(kind === "directory"
      ? {}
      : {
          filters: [{ name: "插件 ZIP 包", extensions: ["zip"] }],
        }),
  };
}

async function pickPluginSourceKind(
  sourceDialog: PluginSourceDialog,
  platform: NodeJS.Platform,
): Promise<PluginSourceSelectionKind | null> {
  if (platform === "darwin") {
    return "mixed";
  }

  const result = await sourceDialog.showMessageBox({
    type: "question",
    title: "选择插件来源",
    message: "请选择本地插件来源类型",
    detail: "可以选择包含 plugin.json 的目录，或 .zip 插件包。",
    buttons: ["选择目录", "选择 ZIP", "取消"],
    defaultId: 0,
    cancelId: 2,
    noLink: true,
  });
  if (result.response === 0) return "directory";
  if (result.response === 1) return "zip";
  return null;
}

export async function validatePluginSourcePath(
  selected: string,
  statPath: (candidate: string) => Promise<PluginSourceStats> = fs.stat,
): Promise<WorkspacePickResult> {
  try {
    const stats = await statPath(selected);
    if (stats.isDirectory()) {
      return { canceled: false, path: selected };
    }
    if (stats.isFile() && path.extname(selected).toLowerCase() === ".zip") {
      return { canceled: false, path: selected };
    }
    return { canceled: true, error: "请选择插件目录或 .zip 文件" };
  } catch {
    return { canceled: true, error: "无法访问所选插件来源" };
  }
}

export async function pickPluginSource(
  sourceDialog: PluginSourceDialog,
  options: PluginSourcePickerOptions = {},
): Promise<WorkspacePickResult> {
  const kind = await pickPluginSourceKind(
    sourceDialog,
    options.platform ?? process.platform,
  );
  if (!kind) {
    return { canceled: true };
  }

  const result = await sourceDialog.showOpenDialog(
    pluginSourceOpenDialogOptions(kind, options.defaultPath),
  );
  if (result.canceled || result.filePaths.length === 0) {
    return { canceled: true };
  }
  return validatePluginSourcePath(result.filePaths[0], options.statPath);
}
