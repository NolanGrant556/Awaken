// 安装版升级 smoke 的 Main 侧驱动。
// 只在启动参数要求、且包内 app-update.yml 已经是 127.0.0.1 generic feed 时运行。
// 不读取环境变量里的 URL，也不接受 Renderer 传入的地址。

import type { DesktopUpdateSnapshot } from "../shared/desktop-update.js";

export const UPDATE_SMOKE_FLAG = "--awaken-update-smoke";
export const UPDATE_SMOKE_INSTALL_FLAG = "--awaken-update-smoke-install";

const ACTIVE_RUN_RETRY_MS = 1_000;

export type UpdateSmokeResult = "ignored" | "ok" | "failed";

export type UpdateSmokeController = {
  check: () => Promise<DesktopUpdateSnapshot>;
  download: () => Promise<DesktopUpdateSnapshot>;
  restartAndInstall: () => Promise<DesktopUpdateSnapshot>;
};

export function isUpdateSmokeRequested(argv: readonly string[]): boolean {
  return argv.includes(UPDATE_SMOKE_FLAG);
}

/** 只接受 http://127.0.0.1[:port][/path]，拒绝用户名、密码和其他主机。 */
export function isLoopbackHttpUrl(value: string): boolean {
  let url: URL;
  try {
    url = new URL(value);
  } catch {
    return false;
  }
  if (url.protocol !== "http:") return false;
  if (url.hostname !== "127.0.0.1") return false;
  if (url.username || url.password) return false;
  if (url.port) {
    const port = Number(url.port);
    if (!Number.isInteger(port) || port < 1 || port > 65535) return false;
  }
  return true;
}

export function isLoopbackGenericFeed(text: string): boolean {
  const provider = text.match(/^provider:\s*["']?generic["']?\s*$/m);
  const url = text.match(/^url:\s*["']?([^"'\s]+)["']?\s*$/m);
  return Boolean(provider && url && isLoopbackHttpUrl(url[1]));
}

export async function runPackagedUpdateSmoke(options: {
  feedText: string;
  install: boolean;
  controller: UpdateSmokeController;
  log: (line: string) => void;
  now?: () => number;
  sleep?: (ms: number) => Promise<void>;
  deadlineMs?: number;
}): Promise<UpdateSmokeResult> {
  if (!isLoopbackGenericFeed(options.feedText)) {
    options.log("desktop_update_smoke_ignored");
    return "ignored";
  }
  const checked = await options.controller.check();
  if (checked.phase !== "available" || !checked.availableVersion) {
    options.log(
      `desktop_update_smoke_failed check ${checked.phase} ${checked.errorCode ?? ""}`.trim(),
    );
    return "failed";
  }
  options.log(`desktop_update_smoke_available ${checked.availableVersion}`);
  const downloaded = await options.controller.download();
  if (downloaded.phase !== "downloaded" || !downloaded.availableVersion) {
    options.log(
      `desktop_update_smoke_failed download ${downloaded.phase} ${downloaded.errorCode ?? ""}`.trim(),
    );
    return "failed";
  }
  // 下载完成先记录“稍后”，再决定是否重启安装。
  options.log(`desktop_update_smoke_deferred ${downloaded.availableVersion}`);
  if (!options.install) return "ok";

  const now = options.now ?? (() => Date.now());
  const sleep = options.sleep ?? ((ms) => new Promise((resolve) => setTimeout(resolve, ms)));
  const deadline = now() + (options.deadlineMs ?? 180_000);
  let installing = await options.controller.restartAndInstall();
  while (installing.phase === "downloaded" && installing.errorCode === "active_runs") {
    if (now() >= deadline) {
      options.log("desktop_update_smoke_failed install active_runs");
      return "failed";
    }
    await sleep(ACTIVE_RUN_RETRY_MS);
    installing = await options.controller.restartAndInstall();
  }
  if (installing.phase !== "installing") {
    options.log(
      `desktop_update_smoke_failed install ${installing.phase} ${installing.errorCode ?? ""}`.trim(),
    );
    return "failed";
  }
  options.log(`desktop_update_smoke_installing ${downloaded.availableVersion}`);
  return "ok";
}
