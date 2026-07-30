import {
  createEmptyAwakenScheduledTaskDraft,
  type AwakenScheduledTaskDraft,
  type AwakenScheduledTaskDraftField,
  type AwakenScheduledTaskProfileOption,
  type AwakenScheduledTaskProjectOption,
} from "./AwakenScheduledTaskEditorDialog";
import type { ComposerBarToken } from "../AwakenComposerBar";

export const AwakenScheduledTaskEditorDemoProjects: AwakenScheduledTaskProjectOption[] = [
  { projectId: "project_awaken", name: "Awaken" },
  { projectId: "project_personal", name: "个人" },
  { projectId: "project_empty", name: "临时笔记（零文件夹项目）" },
];

export const AwakenScheduledTaskEditorDemoProfiles: AwakenScheduledTaskProfileOption[] = [
  { profileId: "profile_kimi", name: "Kimi for Coding" },
  { profileId: "profile_local", name: "本地模型" },
];

export const AwakenScheduledTaskEditorDemoTimezones: string[] = [
  "Asia/Shanghai",
  "Asia/Tokyo",
  "Europe/Berlin",
  "America/New_York",
  "UTC",
];

export type AwakenScheduledTaskEditorDialogDemoState = {
  id: string;
  caption: string;
  mode: "create" | "edit";
  value: AwakenScheduledTaskDraft;
  errorMessage?: string;
  fieldErrors?: Partial<Record<AwakenScheduledTaskDraftField, string>>;
  composerTokens?: ComposerBarToken[];
};

const createCronDraft: AwakenScheduledTaskDraft = {
  ...createEmptyAwakenScheduledTaskDraft("Asia/Shanghai"),
  name: "休息提醒",
  projectId: "project_personal",
  cronPreset: "custom",
  cron: "*/25 9-17 * * 1-5",
  cronTime: "09:00",
  cronWeekdays: [1, 2, 3, 4, 5],
  content: "持续工作25分钟啦，起来活动一下，喝口水，顺便看看远处放松眼睛～",
};

const createWeeklyDraft: AwakenScheduledTaskDraft = {
  ...createEmptyAwakenScheduledTaskDraft("Asia/Shanghai"),
  name: "周会前工作总结",
  projectId: "project_awaken",
  cronPreset: "weekly",
  cron: "30 8 * * 5",
  cronTime: "08:30",
  cronWeekdays: [5],
  actionKind: "agent",
  content: "请基于最近一周的 memory，生成本周工作总结。",
};

const editCalendarDraft: AwakenScheduledTaskDraft = {
  ...createEmptyAwakenScheduledTaskDraft("Asia/Shanghai"),
  name: "两个月周会提醒",
  projectId: "project_awaken",
  scheduleKind: "calendar",
  runAtLocal: "2026-07-31T08:45",
  repeat: true,
  repeatEveryDays: 7,
  repeatEndKind: "until",
  repeatUntilLocal: "2026-09-25T08:45",
  actionKind: "reminder",
  content: "15分钟后周会开始。",
  templateOrigin: { templateId: "repeat_until_text_weekly_meeting", source: "builtin" },
};

const invalidDraft: AwakenScheduledTaskDraft = {
  ...createCronDraft,
  name: "",
};

export const AwakenScheduledTaskEditorDialogStates: AwakenScheduledTaskEditorDialogDemoState[] = [
  {
    id: "create-cron",
    caption: "创建 · 循环任务（自定义 Cron）",
    mode: "create",
    value: createCronDraft,
  },
  {
    id: "create-weekly",
    caption: "创建 · 循环任务（每周五 08:30）",
    mode: "create",
    value: createWeeklyDraft,
    composerTokens: [
      {
        id: "skill:weekly-summary:project",
        type: "skill",
        label: "weekly-summary",
        title: "/Awaken/.awaken/skills/weekly-summary/SKILL.md",
      },
    ],
  },
  {
    id: "edit-calendar",
    caption: "编辑 · 日程任务（每 7 天，截止时间，含模板来源）",
    mode: "edit",
    value: editCalendarDraft,
  },
  {
    id: "create-invalid",
    caption: "创建 · 校验错误就地展示（保存失败不丢表单）",
    mode: "create",
    value: invalidDraft,
    errorMessage: "保存失败：请修正以下字段后重试。",
    fieldErrors: { name: "名称不能为空" },
  },
];
