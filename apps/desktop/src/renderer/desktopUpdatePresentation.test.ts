import { describe, expect, it, vi } from "vitest";
import type { DesktopUpdateSnapshot } from "../shared/desktop-update";
import {
  buildSidebarUpdateAction,
  desktopUpdateInstallBlockedMessage,
  type DesktopUpdateUiCallbacks,
} from "./desktopUpdatePresentation";

function snapshot(patch: Partial<DesktopUpdateSnapshot>): DesktopUpdateSnapshot {
  return {
    operationId: "op-1",
    phase: "idle",
    currentVersion: "0.1.0",
    ...patch,
  };
}

function callbacks(): DesktopUpdateUiCallbacks & {
  downloaded: ReturnType<typeof vi.fn>;
  restart: ReturnType<typeof vi.fn>;
  retried: ReturnType<typeof vi.fn>;
} {
  const downloaded = vi.fn();
  const restart = vi.fn();
  const retried = vi.fn();
  return {
    onDownload: downloaded,
    onRestartRequested: restart,
    onRetry: retried,
    downloaded,
    restart,
    retried,
  };
}

describe("buildSidebarUpdateAction", () => {
  it("disabled/idle/checking/current/installing 不显示入口", () => {
    const cbs = callbacks();
    for (const phase of ["disabled", "idle", "checking", "current", "installing"] as const) {
      expect(buildSidebarUpdateAction(snapshot({ phase }), cbs)).toBeUndefined();
    }
    expect(buildSidebarUpdateAction(null, cbs)).toBeUndefined();
  });

  it("available 显示目标版本并触发下载", () => {
    const cbs = callbacks();
    const action = buildSidebarUpdateAction(
      snapshot({ phase: "available", availableVersion: "0.2.0" }),
      cbs,
    );
    expect(action).toMatchObject({ state: "available", label: "更新到 v0.2.0" });
    action?.onClick();
    expect(cbs.downloaded).toHaveBeenCalledTimes(1);
    expect(cbs.restart).not.toHaveBeenCalled();
  });

  it("downloading 显示百分比进度；未知总大小时不伪造百分比", () => {
    const cbs = callbacks();
    const withPercent = buildSidebarUpdateAction(
      snapshot({ phase: "downloading", percent: 37.6 }),
      cbs,
    );
    expect(withPercent).toMatchObject({
      state: "downloading",
      label: "正在下载 37%",
      progress: 37,
    });
    const unknown = buildSidebarUpdateAction(snapshot({ phase: "downloading" }), cbs);
    expect(unknown).toMatchObject({ state: "downloading", label: "正在下载…" });
    expect(unknown?.progress).toBeUndefined();
  });

  it("downloaded 提供重启入口", () => {
    const cbs = callbacks();
    const action = buildSidebarUpdateAction(snapshot({ phase: "downloaded" }), cbs);
    expect(action).toMatchObject({ state: "downloaded", label: "重启完成更新" });
    action?.onClick();
    expect(cbs.restart).toHaveBeenCalledTimes(1);
  });

  it("error 按 retryFrom 路由重试阶段", () => {
    const cbs = callbacks();
    const fromCheck = buildSidebarUpdateAction(
      snapshot({ phase: "error", errorCode: "network_unavailable", retryFrom: "check" }),
      cbs,
    );
    expect(fromCheck).toMatchObject({ state: "error", label: "更新失败，点击重试" });
    fromCheck?.onClick();
    expect(cbs.retried).toHaveBeenCalledWith("check");

    const fromDownload = buildSidebarUpdateAction(
      snapshot({ phase: "error", errorCode: "download_failed", retryFrom: "download" }),
      cbs,
    );
    fromDownload?.onClick();
    expect(cbs.retried).toHaveBeenCalledWith("download");
  });
});

describe("desktopUpdateInstallBlockedMessage", () => {
  it("active_runs 说明等待或取消任务", () => {
    expect(desktopUpdateInstallBlockedMessage("active_runs")).toContain("正在运行");
  });

  it("shutdown_failed 说明可重试且数据未受影响", () => {
    expect(desktopUpdateInstallBlockedMessage("shutdown_failed")).toContain("重试");
  });

  it("其他错误码不阻断确认框", () => {
    expect(desktopUpdateInstallBlockedMessage(undefined)).toBeNull();
    expect(desktopUpdateInstallBlockedMessage("network_unavailable")).toBeNull();
  });

  it("安装器启动失败提供可重试说明", () => {
    expect(desktopUpdateInstallBlockedMessage("install_launch_failed")).toContain("重试");
  });
});
