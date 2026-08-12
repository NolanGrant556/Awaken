import { execFile } from "node:child_process";
import { accessSync, constants, existsSync } from "node:fs";
import { dirname, join } from "node:path";
import { app, shell } from "electron";
import type { ComputerRuntimeSetupResult } from "../shared/runtime.js";

export type ComputerPermission = "screen_recording" | "accessibility";

const target = `${process.platform}-${process.arch}`;

function findRepoRoot(): string | null {
  const appPath = app.getAppPath();
  // Development: appPath is apps/desktop
  const devRoot = dirname(dirname(appPath));
  if (existsSync(join(devRoot, "scripts", "prepare-node-runtime.mjs"))) {
    return devRoot;
  }
  // Packaged app does not ship scripts; runtime should already be bundled.
  return null;
}

function nodeBinaryPath(root: string): string {
  const relativeBinaryDir = target.startsWith("win32-") ? [] : ["bin"];
  return join(
    root,
    "apps",
    "desktop",
    "resources",
    "bin",
    target,
    "node",
    ...relativeBinaryDir,
    target.startsWith("win32-") ? "node.exe" : "node",
  );
}

function cuaHostScriptPath(root: string): string {
  return join(root, "apps", "desktop", "resources", "cua-driver", "host.mjs");
}

function cuaHostNodeModulesPath(root: string): string {
  return join(
    root,
    "apps",
    "desktop",
    "resources",
    "cua-driver",
    "node_modules",
    ".package-lock.json",
  );
}

function canExecute(path: string): boolean {
  try {
    accessSync(path, constants.X_OK);
    return true;
  } catch {
    return false;
  }
}

function runScript(
  scriptPath: string,
  args: string[] = [],
  options: { cwd?: string; nodePath?: string } = {},
): Promise<void> {
  return new Promise((resolve, reject) => {
    if (!existsSync(scriptPath)) {
      reject(new Error(`setup script not found: ${scriptPath}`));
      return;
    }

    // Prefer the bundled Node runtime so we do not depend on system Node.
    // Fall back to process.env.NODE (if set) or the system `node` binary.
    const nodePath = options.nodePath ?? process.env.NODE ?? "node";

    execFile(
      nodePath,
      [scriptPath, ...args],
      {
        cwd: options.cwd,
        env: { ...process.env, AWAKEN_BINARY_TARGETS: target },
      },
      (error) => {
        if (error) {
          reject(error);
        } else {
          resolve();
        }
      },
    );
  });
}

export async function prepareComputerRuntime(): Promise<ComputerRuntimeSetupResult> {
  const root = findRepoRoot();
  if (!root) {
    // Packaged mode: scripts are not shipped; rely on bundled resources.
    const packagedRoot = dirname(dirname(app.getAppPath()));
    const nodePath = nodeBinaryPath(packagedRoot);
    const hostScriptPath = cuaHostScriptPath(packagedRoot);
    const hostModulesPath = cuaHostNodeModulesPath(packagedRoot);
    const nodeReady = existsSync(nodePath) && canExecute(nodePath);
    const cuaHostReady =
      existsSync(hostScriptPath) && existsSync(hostModulesPath);
    if (!nodeReady || !cuaHostReady) {
      return {
        node_ready: nodeReady,
        cua_host_ready: cuaHostReady,
        message:
          "打包应用缺少 bundled Node runtime 或 CUA Host，请重新安装完整安装包。",
      };
    }
    return { node_ready: nodeReady, cua_host_ready: cuaHostReady };
  }

  const nodePath = nodeBinaryPath(root);
  const hostScriptPath = cuaHostScriptPath(root);
  const hostModulesPath = cuaHostNodeModulesPath(root);

  let nodeReady = existsSync(nodePath) && canExecute(nodePath);
  let cuaHostReady = existsSync(hostScriptPath) && existsSync(hostModulesPath);

  if (nodeReady && cuaHostReady) {
    return { node_ready: nodeReady, cua_host_ready: cuaHostReady };
  }

  // Prepare Node runtime first using whatever Node is available on this machine.
  try {
    if (!nodeReady) {
      await runScript(join(root, "scripts", "prepare-node-runtime.mjs"), [
        `--targets=${target}`,
      ]);
      nodeReady = existsSync(nodePath) && canExecute(nodePath);
    }
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    return {
      node_ready: nodeReady,
      cua_host_ready: cuaHostReady,
      message: `Node runtime 准备失败: ${message}`,
    };
  }

  // Now use the freshly prepared Node runtime to install CUA driver deps.
  try {
    if (!cuaHostReady) {
      await runScript(join(root, "scripts", "prepare-cua-driver.mjs"), [], {
        cwd: root,
        nodePath,
      });
      cuaHostReady = existsSync(hostScriptPath) && existsSync(hostModulesPath);
    }
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    return {
      node_ready: nodeReady,
      cua_host_ready: cuaHostReady,
      message: `CUA Host 准备失败: ${message}`,
    };
  }

  return { node_ready: nodeReady, cua_host_ready: cuaHostReady };
}

export async function openComputerPermissionSettings(
  permission: ComputerPermission,
): Promise<void> {
  if (process.platform !== "darwin") {
    return;
  }
  const pane =
    permission === "screen_recording"
      ? "Privacy_ScreenCapture"
      : "Privacy_Accessibility";
  await shell.openExternal(
    `x-apple.systempreferences:com.apple.preference.security?${pane}`,
  );
}
