import { mkdtempSync, readFileSync, writeFileSync, existsSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { describe, expect, it, vi } from "vitest";
import type {
  DesktopRuntimeActivity,
  DesktopUpdateSnapshot,
} from "../shared/desktop-update.js";
import {
  DesktopUpdateManager,
  type DesktopUpdateManagerDeps,
  type DesktopUpdaterLike,
} from "./desktop-update-manager.js";

type Listener = Parameters<DesktopUpdaterLike["on"]>[1];

class FakeUpdater implements DesktopUpdaterLike {
  autoDownload = true;
  autoInstallOnAppQuit = true;
  allowPrerelease = true;
  allowDowngrade = true;
  readonly listeners = new Map<string, Set<Listener>>();
  readonly checkForUpdates = vi.fn(async (): Promise<unknown> => undefined);
  readonly downloadUpdate = vi.fn(async (): Promise<unknown> => undefined);
  readonly quitAndInstall = vi.fn();

  on(event: string, listener: Listener): void {
    const set = this.listeners.get(event) ?? new Set<Listener>();
    set.add(listener);
    this.listeners.set(event, set);
  }

  removeListener(event: string, listener: Listener): void {
    this.listeners.get(event)?.delete(listener);
  }

  emit(event: string, ...args: unknown[]): void {
    for (const listener of this.listeners.get(event) ?? []) {
      listener(...(args as never[]));
    }
  }

  listenerCount(event: string): number {
    return this.listeners.get(event)?.size ?? 0;
  }
}

const IDLE_ACTIVITY: DesktopRuntimeActivity = {
  active_run_count: 0,
  cancelling_run_count: 0,
  observed_at: "2026-08-18T00:00:00.000Z",
};

function createManager(overrides: Partial<DesktopUpdateManagerDeps> = {}) {
  const updater = new FakeUpdater();
  let now = 1_000_000;
  const receiptPath = join(mkdtempSync(join(tmpdir(), "awaken-update-test-")), "receipt.json");
  const logs: Array<{ event: string; fields: Record<string, unknown> }> = [];
  const deps: DesktopUpdateManagerDeps = {
    updater,
    enabled: true,
    currentVersion: "0.1.0",
    platform: "darwin",
    arch: "arm64",
    receiptPath,
    fetchActivity: vi.fn(async () => IDLE_ACTIVITY),
    requestShutdown: vi.fn(async () => ({ ok: true }) as const),
    now: () => now,
    logger: (event, fields) => logs.push({ event, fields }),
    ...overrides,
  };
  const manager = new DesktopUpdateManager(deps);
  return {
    manager,
    updater,
    deps,
    receiptPath,
    logs,
    advance: (ms: number) => {
      now += ms;
    },
  };
}

function availableCheck(updater: FakeUpdater, version = "0.2.0"): void {
  updater.checkForUpdates.mockImplementationOnce(async () => {
    updater.emit("update-available", { version, releaseName: `Awaken ${version}` });
  });
}

describe("DesktopUpdateManager 初始化与门禁", () => {
  it("dev/ad-hoc 身份禁用：所有命令直接返回 disabled 快照", async () => {
    const { manager, updater } = createManager({ enabled: false });
    manager.initialize();
    expect(manager.getSnapshot().phase).toBe("disabled");
    expect(await manager.check("startup")).toMatchObject({ phase: "disabled" });
    expect(await manager.download()).toMatchObject({ phase: "disabled" });
    expect(await manager.restartAndInstall()).toMatchObject({ phase: "disabled" });
    expect(updater.checkForUpdates).not.toHaveBeenCalled();
  });

  it("不支持的平台/架构禁用更新器", () => {
    const { manager } = createManager({ platform: "win32", arch: "arm64" });
    manager.initialize();
    expect(manager.getSnapshot().phase).toBe("disabled");
  });

  it("启用时显式设置 updater 安全标志并绑定事件", () => {
    const { manager, updater } = createManager();
    manager.initialize();
    expect(updater.autoDownload).toBe(false);
    expect(updater.autoInstallOnAppQuit).toBe(false);
    expect(updater.allowPrerelease).toBe(false);
    expect(updater.allowDowngrade).toBe(false);
    for (const event of [
      "checking-for-update",
      "update-available",
      "update-not-available",
      "download-progress",
      "update-downloaded",
      "error",
      "before-quit-for-update",
    ]) {
      expect(updater.listenerCount(event)).toBe(1);
    }
    expect(manager.getSnapshot().phase).toBe("idle");
  });

  it("dispose 成对移除全部 updater listener", () => {
    const { manager, updater } = createManager();
    manager.initialize();
    manager.dispose();
    for (const event of ["update-available", "download-progress", "error"]) {
      expect(updater.listenerCount(event)).toBe(0);
    }
  });
});

describe("DesktopUpdateManager 检查", () => {
  it("发现更高 stable 版本进入 available，且不自动下载", async () => {
    const { manager, updater } = createManager();
    manager.initialize();
    availableCheck(updater);
    const snapshot = await manager.check("startup");
    expect(snapshot.phase).toBe("available");
    expect(snapshot.availableVersion).toBe("0.2.0");
    expect(snapshot.releaseName).toBe("Awaken 0.2.0");
    expect(snapshot.target).toBe("darwin-arm64");
    expect(updater.downloadUpdate).not.toHaveBeenCalled();
  });

  it("无新版本进入 current；拒绝降级与同版本覆盖", async () => {
    const { manager, updater } = createManager();
    manager.initialize();
    updater.checkForUpdates.mockImplementationOnce(async () => {
      updater.emit("update-not-available", { version: "0.1.0" });
    });
    expect((await manager.check("startup")).phase).toBe("current");

    availableCheck(updater, "0.1.0");
    expect((await manager.check("retry")).phase).toBe("current");

    availableCheck(updater, "0.0.9");
    expect((await manager.check("retry")).phase).toBe("current");

    updater.checkForUpdates.mockImplementationOnce(async () => {
      updater.emit("update-available", { version: "0.2.0-beta.1" });
    });
    expect((await manager.check("retry")).phase).toBe("current");
  });

  it("进行中的 operation 去重：重复命令返回当前快照，不发起第二份请求", async () => {
    const { manager, updater } = createManager();
    manager.initialize();
    let release!: () => void;
    updater.checkForUpdates.mockImplementationOnce(
      () => new Promise<void>((resolve) => (release = resolve)),
    );
    const first = manager.check("startup");
    const second = await manager.check("retry");
    expect(second.phase).toBe("checking");
    expect(updater.checkForUpdates).toHaveBeenCalledTimes(1);
    updater.emit("update-not-available", { version: "0.1.0" });
    release();
    expect((await first).phase).toBe("current");
  });

  it("resume/activate 30 分钟内节流，interval 6 小时内节流", async () => {
    const { manager, updater, advance } = createManager();
    manager.initialize();
    updater.checkForUpdates.mockImplementation(async () => {
      updater.emit("update-not-available", { version: "0.1.0" });
    });
    await manager.check("startup");
    expect(updater.checkForUpdates).toHaveBeenCalledTimes(1);

    advance(10 * 60 * 1000);
    await manager.check("resume");
    expect(updater.checkForUpdates).toHaveBeenCalledTimes(1);

    advance(31 * 60 * 1000);
    await manager.check("activate");
    expect(updater.checkForUpdates).toHaveBeenCalledTimes(2);

    advance(60 * 60 * 1000);
    await manager.check("interval");
    expect(updater.checkForUpdates).toHaveBeenCalledTimes(2);

    advance(6 * 60 * 60 * 1000 + 1);
    await manager.check("interval");
    expect(updater.checkForUpdates).toHaveBeenCalledTimes(3);
  });

  it("后台检查失败静默恢复原状态，用户主动重试才暴露错误", async () => {
    const { manager, updater } = createManager();
    manager.initialize();
    updater.checkForUpdates.mockRejectedValueOnce(
      new Error("net::ERR_INTERNET_DISCONNECTED"),
    );
    const silent = await manager.check("startup");
    expect(silent.phase).toBe("idle");
    expect(silent.errorCode).toBeUndefined();

    updater.checkForUpdates.mockRejectedValueOnce(
      new Error("net::ERR_INTERNET_DISCONNECTED"),
    );
    const snapshot = await manager.check("retry");
    expect(snapshot.phase).toBe("error");
    expect(snapshot.errorCode).toBe("network_unavailable");
    expect(snapshot.retryFrom).toBe("check");
  });

  it("每次进入 checking 生成新 operationId，迟到事件不得覆盖新状态", async () => {
    const { manager, updater } = createManager();
    manager.initialize();
    availableCheck(updater, "0.2.0");
    const first = await manager.check("startup");
    updater.checkForUpdates.mockImplementationOnce(async () => {
      updater.emit("update-not-available", { version: "0.1.0" });
    });
    const second = await manager.check("retry");
    expect(second.operationId).not.toBe(first.operationId);
    // 第一次操作的迟到事件不改变第二次操作产生的状态。
    updater.emit("update-available", { version: "9.9.9" });
    expect(manager.getSnapshot().phase).toBe("current");
  });
});

describe("DesktopUpdateManager 下载", () => {
  async function reachAvailable(ctx: ReturnType<typeof createManager>): Promise<void> {
    availableCheck(ctx.updater);
    await ctx.manager.check("startup");
  }

  it("非 available/error 状态拒绝下载", async () => {
    const ctx = createManager();
    ctx.manager.initialize();
    const snapshot = await ctx.manager.download();
    expect(snapshot.phase).toBe("idle");
    expect(ctx.updater.downloadUpdate).not.toHaveBeenCalled();
  });

  it("显式下载：进度节流、单调、限制 0–99，完成后 100 并写 receipt", async () => {
    const ctx = createManager();
    const { manager, updater, receiptPath, advance } = ctx;
    manager.initialize();
    await reachAvailable(ctx);
    const seen: DesktopUpdateSnapshot[] = [];
    manager.subscribe((snapshot) => seen.push(snapshot));

    updater.downloadUpdate.mockImplementationOnce(async () => {
      updater.emit("download-progress", { total: 100, transferred: 10, percent: 10 });
      advance(100); // 200ms 节流窗口内，应被忽略
      updater.emit("download-progress", { total: 100, transferred: 20, percent: 20 });
      advance(250);
      updater.emit("download-progress", { total: 100, transferred: 15, percent: 15 });
      advance(250);
      updater.emit("download-progress", { total: 100, transferred: 100, percent: 100 });
      updater.emit("update-downloaded", { version: "0.2.0" });
    });
    const snapshot = await manager.download();
    expect(snapshot.phase).toBe("downloaded");
    expect(snapshot.percent).toBe(100);

    const progressPercents = seen
      .filter((s) => s.phase === "downloading" && s.percent !== undefined)
      .map((s) => s.percent);
    // 200ms 内的第二帧被节流；15% 不回退；下载中不出现 100。
    expect(progressPercents).toEqual([10, 15, 99]);

    const receipt = JSON.parse(readFileSync(receiptPath, "utf8"));
    expect(receipt).toMatchObject({
      schemaVersion: 1,
      version: "0.2.0",
      target: "darwin-arm64",
      releaseTag: "v0.2.0",
    });
    expect(typeof receipt.downloadedAt).toBe("string");
  });

  it("未知 totalBytes 时不伪造百分比", async () => {
    const ctx = createManager();
    const { manager, updater, advance } = ctx;
    manager.initialize();
    await reachAvailable(ctx);
    const seen: DesktopUpdateSnapshot[] = [];
    manager.subscribe((snapshot) => seen.push(snapshot));
    updater.downloadUpdate.mockImplementationOnce(async () => {
      updater.emit("download-progress", { transferred: 512 });
      advance(250);
      updater.emit("download-progress", { transferred: 1024 });
      updater.emit("update-downloaded", { version: "0.2.0" });
    });
    await manager.download();
    const frames = seen.filter(
      (s) => s.phase === "downloading" && (s.transferredBytes ?? 0) > 0,
    );
    expect(frames.length).toBeGreaterThan(0);
    for (const frame of frames) {
      expect(frame.percent).toBeUndefined();
      expect(frame.transferredBytes).toBeGreaterThan(0);
    }
  });

  it("下载失败进入 error(retryFrom=download)，可按原阶段重试", async () => {
    const ctx = createManager();
    const { manager, updater } = ctx;
    manager.initialize();
    await reachAvailable(ctx);
    updater.downloadUpdate.mockRejectedValueOnce(new Error("sha512 checksum mismatch"));
    const failed = await manager.download();
    expect(failed.phase).toBe("error");
    expect(failed.errorCode).toBe("checksum_mismatch");
    expect(failed.retryFrom).toBe("download");
    expect(failed.availableVersion).toBe("0.2.0");

    updater.downloadUpdate.mockImplementationOnce(async () => {
      updater.emit("update-downloaded", { version: "0.2.0" });
    });
    const retried = await manager.download();
    expect(retried.phase).toBe("downloaded");
    expect(updater.downloadUpdate).toHaveBeenCalledTimes(2);
  });

  it("下载 promise 未产生已验证完成事件时 fail closed", async () => {
    const ctx = createManager();
    ctx.manager.initialize();
    await reachAvailable(ctx);
    const snapshot = await ctx.manager.download();
    expect(snapshot).toMatchObject({
      phase: "error",
      errorCode: "download_failed",
      retryFrom: "download",
    });
  });

  it("拒绝与已选择版本不一致的 update-downloaded 事件", async () => {
    const ctx = createManager();
    ctx.manager.initialize();
    await reachAvailable(ctx);
    ctx.updater.downloadUpdate.mockImplementationOnce(async () => {
      ctx.updater.emit("update-downloaded", { version: "0.3.0" });
    });
    const snapshot = await ctx.manager.download();
    expect(snapshot).toMatchObject({
      phase: "error",
      errorCode: "metadata_invalid",
      retryFrom: "download",
      availableVersion: "0.2.0",
    });
    expect(existsSync(ctx.receiptPath)).toBe(false);
  });

  it("已下载待重启时忽略定时、唤醒和手工检查，不覆盖缓存状态", async () => {
    const ctx = createManager();
    ctx.manager.initialize();
    await reachAvailable(ctx);
    ctx.updater.downloadUpdate.mockImplementationOnce(async () => {
      ctx.updater.emit("update-downloaded", { version: "0.2.0" });
    });
    await ctx.manager.download();
    const checksBefore = ctx.updater.checkForUpdates.mock.calls.length;
    for (const reason of ["interval", "resume", "activate", "retry"] as const) {
      expect((await ctx.manager.check(reason)).phase).toBe("downloaded");
    }
    expect(ctx.updater.checkForUpdates).toHaveBeenCalledTimes(checksBefore);
  });
});

describe("DesktopUpdateManager 重启安装", () => {
  async function reachDownloaded(ctx: ReturnType<typeof createManager>): Promise<void> {
    availableCheck(ctx.updater);
    await ctx.manager.check("startup");
    ctx.updater.downloadUpdate.mockImplementationOnce(async () => {
      ctx.updater.emit("update-downloaded", { version: "0.2.0" });
    });
    await ctx.manager.download();
  }

  it("非 downloaded 状态拒绝安装", async () => {
    const ctx = createManager();
    ctx.manager.initialize();
    const snapshot = await ctx.manager.restartAndInstall();
    expect(snapshot.phase).toBe("idle");
    expect(ctx.updater.quitAndInstall).not.toHaveBeenCalled();
  });

  it("成功路径：现查 activity → shutdown → quitAndInstall(false, true)", async () => {
    const ctx = createManager();
    const { manager, updater, deps } = ctx;
    manager.initialize();
    await reachDownloaded(ctx);
    const snapshot = await manager.restartAndInstall();
    expect(snapshot.phase).toBe("installing");
    expect(deps.fetchActivity).toHaveBeenCalledTimes(1);
    expect(deps.requestShutdown).toHaveBeenCalledWith("update-install");
    expect(updater.quitAndInstall).toHaveBeenCalledTimes(1);
    expect(updater.quitAndInstall).toHaveBeenCalledWith(false, true);
  });

  it("存在 active run 时 fail closed：保持 downloaded，不调用安装器", async () => {
    const ctx = createManager({
      fetchActivity: vi.fn(async () => ({
        active_run_count: 1,
        cancelling_run_count: 0,
        observed_at: "2026-08-18T00:00:00.000Z",
      })),
    });
    const { manager, updater, deps } = ctx;
    manager.initialize();
    await reachDownloaded(ctx);
    const snapshot = await manager.restartAndInstall();
    expect(snapshot.phase).toBe("downloaded");
    expect(snapshot.errorCode).toBe("active_runs");
    expect(deps.requestShutdown).not.toHaveBeenCalled();
    expect(updater.quitAndInstall).not.toHaveBeenCalled();
  });

  it("activity 快照不可达同样 fail closed 为 active_runs", async () => {
    const ctx = createManager({ fetchActivity: vi.fn(async () => null) });
    const { manager, updater } = ctx;
    manager.initialize();
    await reachDownloaded(ctx);
    const snapshot = await manager.restartAndInstall();
    expect(snapshot.phase).toBe("downloaded");
    expect(snapshot.errorCode).toBe("active_runs");
    expect(updater.quitAndInstall).not.toHaveBeenCalled();
  });

  it("畸形 activity 快照同样 fail closed", async () => {
    const ctx = createManager({
      fetchActivity: vi.fn(async () => ({
        active_run_count: -1,
        cancelling_run_count: 0,
        observed_at: "invalid",
      })),
    });
    ctx.manager.initialize();
    await reachDownloaded(ctx);
    expect((await ctx.manager.restartAndInstall()).errorCode).toBe("active_runs");
    expect(ctx.updater.quitAndInstall).not.toHaveBeenCalled();
  });

  it("cancelling run 也阻止安装", async () => {
    const ctx = createManager({
      fetchActivity: vi.fn(async () => ({
        active_run_count: 1,
        cancelling_run_count: 1,
        observed_at: "2026-08-18T00:00:00.000Z",
      })),
    });
    const { manager, updater } = ctx;
    manager.initialize();
    await reachDownloaded(ctx);
    expect((await manager.restartAndInstall()).errorCode).toBe("active_runs");
    expect(updater.quitAndInstall).not.toHaveBeenCalled();
  });

  it("清理失败恢复 downloaded 并标记 shutdown_failed，不强制退出", async () => {
    const ctx = createManager({
      requestShutdown: vi.fn(async () => ({ ok: false, reason: "timeout" }) as const),
    });
    const { manager, updater } = ctx;
    manager.initialize();
    await reachDownloaded(ctx);
    const snapshot = await manager.restartAndInstall();
    expect(snapshot.phase).toBe("downloaded");
    expect(snapshot.errorCode).toBe("shutdown_failed");
    expect(updater.quitAndInstall).not.toHaveBeenCalled();
  });
});

describe("DesktopUpdateManager receipt 启动对账", () => {
  it("过期或同版本 receipt 在初始化时被删除", () => {
    const ctx = createManager();
    writeFileSync(
      ctx.receiptPath,
      JSON.stringify({
        schemaVersion: 1,
        version: "0.1.0",
        target: "darwin-arm64",
        releaseTag: "v0.1.0",
        downloadedAt: "2026-08-17T00:00:00.000Z",
      }),
    );
    ctx.manager.initialize();
    expect(existsSync(ctx.receiptPath)).toBe(false);
  });

  it("target 不一致的 receipt 被删除", () => {
    const ctx = createManager();
    writeFileSync(
      ctx.receiptPath,
      JSON.stringify({
        schemaVersion: 1,
        version: "0.2.0",
        target: "win32-x64",
        releaseTag: "v0.2.0",
        downloadedAt: "2026-08-17T00:00:00.000Z",
      }),
    );
    ctx.manager.initialize();
    expect(existsSync(ctx.receiptPath)).toBe(false);
  });

  it("更高版本 receipt 仅作为线索保留，不恢复 downloaded 状态", () => {
    const ctx = createManager();
    writeFileSync(
      ctx.receiptPath,
      JSON.stringify({
        schemaVersion: 1,
        version: "0.2.0",
        target: "darwin-arm64",
        releaseTag: "v0.2.0",
        downloadedAt: "2026-08-17T00:00:00.000Z",
      }),
    );
    ctx.manager.initialize();
    expect(existsSync(ctx.receiptPath)).toBe(true);
    expect(ctx.manager.getSnapshot().phase).toBe("idle");
    expect(ctx.logs.some((entry) => entry.event === "desktop_update_receipt_noticed")).toBe(true);
  });
});
