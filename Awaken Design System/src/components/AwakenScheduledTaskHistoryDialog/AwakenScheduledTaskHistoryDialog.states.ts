import type { AwakenScheduledTaskExecutionItem } from "./AwakenScheduledTaskHistoryDialog";

export const AwakenScheduledTaskHistoryDemoItems: AwakenScheduledTaskExecutionItem[] = [
  {
    executionId: "exec_003",
    triggerLabel: "计划触发",
    status: "success",
    statusLabel: "成功",
    scheduledForText: "2026-07-27 09:30",
    startedAtText: "2026-07-27 09:30:01",
    finishedAtText: "2026-07-27 09:31:20",
    threadId: "thread_sched_tech_news",
  },
  {
    executionId: "exec_002",
    triggerLabel: "手动执行",
    status: "error",
    statusLabel: "失败",
    scheduledForText: "2026-07-26 15:00",
    startedAtText: "2026-07-26 15:00:01",
    finishedAtText: "2026-07-26 15:00:12",
    errorText: "MODEL_PROFILE_UNAVAILABLE：显式 profile 已被禁用",
    threadId: "thread_sched_tech_news",
  },
  {
    executionId: "exec_001",
    triggerLabel: "补跑（misfire）",
    status: "skipped",
    statusLabel: "已跳过",
    scheduledForText: "2026-07-26 09:30",
    startedAtText: "—",
    finishedAtText: "—",
    errorText: "MISFIRE_GRACE_EXCEEDED：超过宽限期",
    threadId: null,
  },
  {
    executionId: "exec_000",
    triggerLabel: "计划触发",
    status: "cancelled",
    statusLabel: "已取消",
    scheduledForText: "2026-07-25 09:30",
    startedAtText: "2026-07-25 09:30:01",
    finishedAtText: "2026-07-25 09:30:20",
    threadId: "thread_sched_tech_news",
  },
];

export type AwakenScheduledTaskHistoryDialogDemoState = {
  id: string;
  caption: string;
  title: string;
  items: AwakenScheduledTaskExecutionItem[];
};

export const AwakenScheduledTaskHistoryDialogStates: AwakenScheduledTaskHistoryDialogDemoState[] = [
  {
    id: "populated",
    caption: "执行记录（含错误条目与可打开线程）",
    title: "执行记录 · 工作日科技新闻早报",
    items: AwakenScheduledTaskHistoryDemoItems,
  },
  {
    id: "empty",
    caption: "空态",
    title: "执行记录 · 休息提醒",
    items: [],
  },
];
