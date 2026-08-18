// Desktop 更新状态的 Renderer 投影：只把 Main snapshot 映射为侧栏按钮/确认框文案。
// App 不维护第二套 phase reducer；状态 authority 在 Electron Main。
// 设计权威：docs/01_桌面工作台/桌面生命周期与发布/应用版本与自动更新设计契约.md §3、§9.4。

import type { AwakenSidebarUpdateAction } from "@awaken/design-system";
import {
  isDesktopUpdateActionablePhase,
  type DesktopUpdateErrorCode,
  type DesktopUpdateSnapshot,
} from "../shared/desktop-update";

export type DesktopUpdateUiCallbacks = {
  onDownload: () => void;
  onRestartRequested: () => void;
  onRetry: (retryFrom: "check" | "download") => void;
};

/** 侧栏只在可操作状态显示更新入口；disabled/idle/checking/current/installing 不显示。 */
export function buildSidebarUpdateAction(
  snapshot: DesktopUpdateSnapshot | null,
  callbacks: DesktopUpdateUiCallbacks,
): AwakenSidebarUpdateAction | undefined {
  if (!snapshot || !isDesktopUpdateActionablePhase(snapshot.phase)) {
    return undefined;
  }
  switch (snapshot.phase) {
    case "available":
      return {
        state: "available",
        label: snapshot.availableVersion
          ? `更新到 v${snapshot.availableVersion}`
          : "发现新版本",
        onClick: callbacks.onDownload,
      };
    case "downloading": {
      const percent =
        typeof snapshot.percent === "number" ? Math.floor(snapshot.percent) : undefined;
      return {
        state: "downloading",
        label: percent !== undefined ? `正在下载 ${percent}%` : "正在下载…",
        progress: percent,
        onClick: () => undefined,
      };
    }
    case "downloaded":
      return {
        state: "downloaded",
        label: "重启完成更新",
        onClick: callbacks.onRestartRequested,
      };
    case "error":
      return {
        state: "error",
        label: "更新失败，点击重试",
        onClick: () => callbacks.onRetry(snapshot.retryFrom ?? "check"),
      };
    default:
      return undefined;
  }
}

/**
 * 重启确认框中的阻断说明。active_runs / shutdown_failed 时"立即重启"不可用，
 * 文案不暴露本机路径、Token 或签名细节。
 */
export function desktopUpdateInstallBlockedMessage(
  errorCode: DesktopUpdateErrorCode | undefined,
): string | null {
  switch (errorCode) {
    case "active_runs":
      return "当前仍有正在运行或正在取消的任务。请等待任务结束，或先使用现有取消入口停止任务后再重启更新。";
    case "shutdown_failed":
      return "停止运行环境失败，更新未安装。请稍后重试；当前任务与数据均未受影响。";
    case "install_launch_failed":
      return "启动更新安装器失败，应用尚未退出。请重试；如果问题持续，请重新下载更新。";
    default:
      return null;
  }
}
