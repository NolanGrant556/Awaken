import type { ComponentTone } from "@awaken/design-system";
import type { ThreadItem } from "./runtime/reducer";

export type ThreadStatusPresentation = {
  label: string;
  tone: ComponentTone;
  active: boolean;
};

function isRunningLikeStatus(status?: ThreadItem["runStatus"]): boolean {
  return status === "queued" || status === "running" || status === "awaiting_approval" || status === "cancelling";
}

export function describeThreadStatus(thread: ThreadItem): ThreadStatusPresentation {
  if ((thread.approvalCount ?? 0) > 0 || thread.runStatus === "awaiting_approval") {
    return { label: "待输入", tone: "permission", active: true };
  }

  switch (thread.runStatus) {
    case "queued":
      return { label: "queued", tone: "local", active: true };
    case "cancelling":
      return { label: "取消中", tone: "warning", active: true };
    case "cancelled":
      return { label: "已取消", tone: "neutral", active: false };
    case "error":
      return { label: "出错", tone: "error", active: false };
    case "finished":
      return { label: "已完成", tone: "success", active: false };
    case "running":
      if ((thread.artifactCount ?? 0) > 0) {
        return { label: "待复核", tone: "warning", active: true };
      }
      return { label: "运行中", tone: "local", active: true };
    default:
      if (thread.activeRunId || isRunningLikeStatus(thread.runStatus)) {
        return { label: "运行中", tone: "local", active: true };
      }
      if ((thread.artifactCount ?? 0) > 0) {
        return { label: "待复核", tone: "warning", active: false };
      }
      return { label: "空闲", tone: "neutral", active: false };
  }
}

export function threadDisplayTitle(thread: ThreadItem): string {
  return (
    thread.metadata?.displayTitle ||
    thread.metadata?.aiTitle ||
    thread.metadata?.customTitle ||
    thread.label ||
    "Untitled thread"
  );
}

function threadUpdatedAtMs(thread: ThreadItem): number {
  if (!thread.updatedAt) {
    return 0;
  }
  const parsed = Date.parse(thread.updatedAt);
  return Number.isFinite(parsed) ? parsed : 0;
}

export function splitTaskCenterThreads(threads: ThreadItem[]): {
  running: ThreadItem[];
  recent: ThreadItem[];
} {
  const sorted = [...threads].sort((a, b) => {
    const activeDelta = Number(describeThreadStatus(b).active) - Number(describeThreadStatus(a).active);
    if (activeDelta !== 0) {
      return activeDelta;
    }
    return threadUpdatedAtMs(b) - threadUpdatedAtMs(a);
  });

  const running: ThreadItem[] = [];
  const recent: ThreadItem[] = [];
  for (const thread of sorted) {
    if (describeThreadStatus(thread).active) {
      running.push(thread);
      continue;
    }
    recent.push(thread);
  }
  return { running, recent };
}

export function formatThreadUpdatedAt(value?: string, now = Date.now()): string {
  if (!value) {
    return "unknown";
  }
  const timestamp = Date.parse(value);
  if (!Number.isFinite(timestamp)) {
    return "unknown";
  }

  const diffMs = Math.max(0, now - timestamp);
  const minute = 60_000;
  const hour = 60 * minute;
  if (diffMs < minute) {
    return "just now";
  }
  if (diffMs < hour) {
    return `${Math.floor(diffMs / minute)}m ago`;
  }
  if (diffMs < 24 * hour) {
    return `${Math.floor(diffMs / hour)}h ago`;
  }

  const date = new Date(timestamp);
  const formatter = new Intl.DateTimeFormat(undefined, {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
  return formatter.format(date);
}
