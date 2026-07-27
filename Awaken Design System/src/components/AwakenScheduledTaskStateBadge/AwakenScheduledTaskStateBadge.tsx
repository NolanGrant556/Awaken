import "./AwakenScheduledTaskStateBadge.css";

// 任务 materialized state 与执行状态，取自《定时任务设计契约》§6.2；
// 组件只负责把状态映射到既有语义 tone 族，文案一律由调用方传入。
export type AwakenScheduledTaskState =
  | "enabled"
  | "paused"
  | "running"
  | "completed"
  | "invalid";

export type AwakenScheduledTaskExecutionStatus =
  | "queued"
  | "running"
  | "success"
  | "error"
  | "skipped"
  | "cancelled"
  | "interrupted";

export type AwakenScheduledTaskStateBadgeTone =
  | "success"
  | "warning"
  | "error"
  | "running"
  | "neutral";

export type AwakenScheduledTaskStateBadgeProps = {
  state: AwakenScheduledTaskState | AwakenScheduledTaskExecutionStatus;
  label: string;
};

const toneByState: Record<string, AwakenScheduledTaskStateBadgeTone> = {
  enabled: "success",
  paused: "neutral",
  running: "running",
  completed: "neutral",
  invalid: "error",
  queued: "neutral",
  success: "success",
  error: "error",
  skipped: "warning",
  cancelled: "neutral",
  interrupted: "warning",
};

export function scheduledTaskStateBadgeTone(
  state: AwakenScheduledTaskStateBadgeProps["state"],
): AwakenScheduledTaskStateBadgeTone {
  return toneByState[state] ?? "neutral";
}

export function AwakenScheduledTaskStateBadge({
  state,
  label,
}: AwakenScheduledTaskStateBadgeProps) {
  return (
    <span
      className="aw-scheduled-task-state-badge"
      data-state={state}
      data-tone={scheduledTaskStateBadgeTone(state)}
    >
      {label}
    </span>
  );
}
