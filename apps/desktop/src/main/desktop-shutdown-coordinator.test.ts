import { describe, expect, it, vi } from "vitest";
import { DesktopShutdownCoordinator } from "./desktop-shutdown-coordinator.js";

describe("DesktopShutdownCoordinator", () => {
  it("normal 模式按顺序执行同步与异步 owner 清理", async () => {
    const order: string[] = [];
    const coordinator = new DesktopShutdownCoordinator({
      stopSyncOwners: () => order.push("sync"),
      stopAsyncOwners: async () => {
        order.push("async");
      },
    });
    const result = await coordinator.stop("normal");
    expect(result).toEqual({ ok: true });
    expect(order).toEqual(["sync", "async"]);
    expect(coordinator.updateInstallReady).toBe(false);
    expect(coordinator.settled).toBe(true);
  });

  it("update-install 完成后 updateInstallReady 为 true", async () => {
    const finalize = vi.fn();
    const coordinator = new DesktopShutdownCoordinator({
      stopSyncOwners: () => undefined,
      stopAsyncOwners: async () => undefined,
      finalizeOwners: finalize,
    });
    expect(await coordinator.stop("update-install")).toEqual({ ok: true });
    expect(coordinator.updateInstallReady).toBe(true);
    expect(finalize).toHaveBeenCalledTimes(1);
  });

  it("重入共享同一 promise，不并发关闭两次", async () => {
    const sync = vi.fn();
    let release!: () => void;
    const coordinator = new DesktopShutdownCoordinator({
      stopSyncOwners: sync,
      stopAsyncOwners: () => new Promise<void>((resolve) => (release = resolve)),
    });
    const first = coordinator.stop("normal");
    const second = coordinator.stop("update-install");
    expect(sync).toHaveBeenCalledTimes(1);
    release();
    expect(await first).toEqual({ ok: true });
    expect(await second).toEqual({ ok: true });
    // 先发起者决定 mode。
    expect(coordinator.updateInstallReady).toBe(false);
  });

  it("异步清理抛错时返回 ok:false 且不标记完成，可重试", async () => {
    let attempts = 0;
    const finalize = vi.fn();
    const coordinator = new DesktopShutdownCoordinator({
      stopSyncOwners: () => undefined,
      stopAsyncOwners: async () => {
        attempts += 1;
        if (attempts === 1) throw new Error("supervisor stop failed");
      },
      finalizeOwners: finalize,
    });
    const failed = await coordinator.stop("update-install");
    expect(failed).toEqual({ ok: false, reason: "supervisor stop failed" });
    expect(coordinator.updateInstallReady).toBe(false);
    expect(finalize).not.toHaveBeenCalled();
    expect(await coordinator.stop("update-install")).toEqual({ ok: true });
    expect(finalize).toHaveBeenCalledTimes(1);
  });

  it("清理超时返回失败，不强制完成", async () => {
    const coordinator = new DesktopShutdownCoordinator({
      stopSyncOwners: () => undefined,
      stopAsyncOwners: () => new Promise<void>(() => undefined),
      timeoutMs: 20,
      logger: () => undefined,
    });
    const result = await coordinator.stop("update-install");
    expect(result.ok).toBe(false);
    if (!result.ok) {
      expect(result.reason).toContain("timed out");
    }
    expect(coordinator.updateInstallReady).toBe(false);
  });

  it("同步 owner 抛错同样归类为失败", async () => {
    const coordinator = new DesktopShutdownCoordinator({
      stopSyncOwners: () => {
        throw new Error("sync boom");
      },
      stopAsyncOwners: async () => undefined,
      logger: () => undefined,
    });
    const result = await coordinator.stop("normal");
    expect(result).toEqual({ ok: false, reason: "sync boom" });
  });
});
