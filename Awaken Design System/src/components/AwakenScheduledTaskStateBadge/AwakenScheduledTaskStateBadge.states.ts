import type {
  AwakenScheduledTaskExecutionStatus,
  AwakenScheduledTaskState,
} from "./AwakenScheduledTaskStateBadge";

export type AwakenScheduledTaskStateBadgeState = {
  id: string;
  state: AwakenScheduledTaskState | AwakenScheduledTaskExecutionStatus;
  label: string;
};

export const AwakenScheduledTaskStateBadgeStates: AwakenScheduledTaskStateBadgeState[] = [
  { id: "task-enabled", state: "enabled", label: "已启用" },
  { id: "task-paused", state: "paused", label: "已暂停" },
  { id: "task-running", state: "running", label: "运行中" },
  { id: "task-completed", state: "completed", label: "已完成" },
  { id: "task-invalid", state: "invalid", label: "配置无效" },
  { id: "execution-queued", state: "queued", label: "排队中" },
  { id: "execution-running", state: "running", label: "执行中" },
  { id: "execution-success", state: "success", label: "成功" },
  { id: "execution-error", state: "error", label: "失败" },
  { id: "execution-skipped", state: "skipped", label: "已跳过" },
  { id: "execution-cancelled", state: "cancelled", label: "已取消" },
  { id: "execution-interrupted", state: "interrupted", label: "已中断" },
];
