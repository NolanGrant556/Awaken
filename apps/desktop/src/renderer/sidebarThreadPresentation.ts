import { type ProjectMenuItem } from "@awaken/design-system";
import { type ThreadItem } from "./runtime/reducer";

export function shouldShowSidebarThreadSpinner(thread: ThreadItem): boolean {
  if (thread.runStatus === "running" || thread.runStatus === "cancelling") {
    return true;
  }
  if (thread.runStatus) {
    return false;
  }
  return Boolean(thread.activeRunId);
}

export function sidebarThreadArchiveMenuState(thread: ThreadItem): {
  disabled: boolean;
  disabledReason?: string;
} {
  if (shouldShowSidebarThreadSpinner(thread)) {
    return { disabled: true, disabledReason: "请先停止正在运行的任务" };
  }
  if (thread.source === "scheduled_task") {
    return { disabled: true, disabledReason: "定时任务会话由定时任务管理" };
  }
  return { disabled: false };
}

export function sidebarThreadMenuItems(
  thread: ThreadItem,
  actions: {
    onRename: () => void;
    onArchive: () => void;
    onDelete: () => void;
  },
): ProjectMenuItem[] {
  const archiveState = sidebarThreadArchiveMenuState(thread);
  return [
    {
      id: "rename-thread",
      label: "重命名",
      onSelect: actions.onRename,
    },
    {
      id: "archive-thread",
      label: "归档",
      disabled: archiveState.disabled,
      disabledReason: archiveState.disabledReason,
      onSelect: actions.onArchive,
    },
    {
      id: "delete-thread",
      label: "删除会话",
      tone: "danger",
      onSelect: actions.onDelete,
    },
  ];
}
