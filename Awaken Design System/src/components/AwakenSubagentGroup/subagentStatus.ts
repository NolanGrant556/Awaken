export type AwakenSubagentExecutionStatus =
  | "queued"
  | "running"
  | "completed"
  | "failed"
  | "cancelling"
  | "cancelled"
  | "interrupted";

export const awakenSubagentStatusLabels: Record<
  AwakenSubagentExecutionStatus,
  string
> = {
  queued: "排队中",
  running: "运行中",
  completed: "已完成",
  failed: "失败",
  cancelling: "正在停止",
  cancelled: "已停止",
  interrupted: "已中断",
};
