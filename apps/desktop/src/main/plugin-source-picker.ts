import fs from "node:fs/promises";
import path from "node:path";
import type {
  OpenDialogOptions,
  OpenDialogReturnValue,
} from "electron";
import type { WorkspacePickResult } from "../shared/runtime.js";

type PluginSourceDialog = {
  showOpenDialog(options: OpenDialogOptions): Promise<OpenDialogReturnValue>;
};

type PluginSourceStats = {
  isFile(): boolean;
};

type PluginSourcePickerOptions = {
  defaultPath?: string;
  statPath?: (candidate: string) => Promise<PluginSourceStats>;
};

export function pluginSourceOpenDialogOptions(
  defaultPath?: string,
): OpenDialogOptions {
  return {
    title: "选择插件 ZIP 包",
    buttonLabel: "选择 ZIP",
    properties: ["openFile"],
    ...(defaultPath ? { defaultPath } : {}),
    filters: [{ name: "插件 ZIP 包", extensions: ["zip"] }],
  };
}

export async function validatePluginSourcePath(
  selected: string,
  statPath: (candidate: string) => Promise<PluginSourceStats> = fs.stat,
): Promise<WorkspacePickResult> {
  try {
    const stats = await statPath(selected);
    if (stats.isFile() && path.extname(selected).toLowerCase() === ".zip") {
      return { canceled: false, path: selected };
    }
    return { canceled: true, error: "请选择 .zip 插件包" };
  } catch {
    return { canceled: true, error: "无法访问所选插件来源" };
  }
}

export async function resolveDroppedPluginSourcePath(
  droppedPath?: string,
  statPath: (candidate: string) => Promise<PluginSourceStats> = fs.stat,
): Promise<WorkspacePickResult> {
  if (!droppedPath?.trim()) {
    return { canceled: true, error: "无法访问拖入的插件 ZIP" };
  }
  return validatePluginSourcePath(droppedPath, statPath);
}

export async function pickPluginSource(
  sourceDialog: PluginSourceDialog,
  options: PluginSourcePickerOptions = {},
): Promise<WorkspacePickResult> {
  const result = await sourceDialog.showOpenDialog(
    pluginSourceOpenDialogOptions(options.defaultPath),
  );
  if (result.canceled || result.filePaths.length === 0) {
    return { canceled: true };
  }
  return validatePluginSourcePath(result.filePaths[0], options.statPath);
}
