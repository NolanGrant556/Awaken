// 内置定时任务模板目录的唯一权威实现（《定时任务设计契约》§5）。
// 本模块是纯函数：选择模板只产生编辑器 draft，绝不发任何请求；
// Core payload（task_id/version/run_at offset 换算等）由保存链路负责，不在这里出现。
import type { AwakenScheduledTaskDraft } from "@awaken/design-system";

export type ScheduledTaskTemplateCategoryId = "cron" | "calendar";

export type ScheduledTaskTemplate = {
  templateId: string;
  category: ScheduledTaskTemplateCategoryId;
  name: string;
  description: string;
  frequencyText: string;
  actionKind: "reminder" | "agent";
  content: string;
  cron?: string;
};

// 名称与正文逐字取自契约 §5.1/§5.2；description/frequencyText 是模板卡的展示文案。
export const SCHEDULED_TASK_TEMPLATES: readonly ScheduledTaskTemplate[] = [
  {
    templateId: "daily_tech_news_brief",
    category: "cron",
    name: "工作日科技新闻早报",
    description: "每个工作日早上整理值得关注的科技新闻，输出 5–8 条要点与趋势判断。",
    frequencyText: "工作日 09:30",
    actionKind: "agent",
    content: "整理今天最值得关注的科技新闻，输出 5–8 条；每条含标题、核心进展、关注原因，最后给出趋势判断",
    cron: "30 9 * * 1-5",
  },
  {
    templateId: "weekend_relaxation_reminder",
    category: "cron",
    name: "周末电影推荐",
    description: "周六日早上推荐 5 部近期热度高、口碑好的电影。",
    frequencyText: "周六/周日 10:00",
    actionKind: "agent",
    content: "推荐 5 部近期热度高、口碑好的电影，给出类型、看点、适合人群和可得的上映/平台信息",
    cron: "0 10 * * 6,0",
  },
  {
    templateId: "pomodoro_break_reminder",
    category: "cron",
    name: "休息提醒",
    description: "工作时段每 25 分钟提醒起身活动、喝水、放松眼睛。",
    frequencyText: "工作日 09:00–17:00 每 25 分钟",
    actionKind: "reminder",
    content: "持续工作25分钟啦，起来活动一下，喝口水，顺便看看远处放松眼睛～",
    cron: "*/25 9-17 * * 1-5",
  },
  {
    templateId: "pet_care_reminder",
    category: "cron",
    name: "宠物驱虫/疫苗提醒",
    description: "每月 15 日晚上提醒给毛孩子安排驱虫/疫苗检查。",
    frequencyText: "每月 15 日 20:00",
    actionKind: "reminder",
    content: "小提醒：今天记得给毛孩子安排驱虫/疫苗检查喔～",
    cron: "0 20 15 * *",
  },
  {
    templateId: "once_text_birthday_reminder",
    category: "calendar",
    name: "生日提醒",
    description: "所选时区下一个 1 月 1 日早上提醒送上生日祝福，不重复。",
    frequencyText: "下一个 1 月 1 日 09:00，不重复",
    actionKind: "reminder",
    content: "xx在1月1日过生日，别忘了送上祝福～",
  },
  {
    templateId: "once_agent_business_trip_prep",
    category: "calendar",
    name: "出差前天气与行程准备",
    description: "出差前一晚查询目的地天气，给出穿衣、携带物品和行程准备建议。",
    frequencyText: "明日 20:00，不重复",
    actionKind: "agent",
    content: "我明天要出差，请查询目的地天气，并给出穿衣、携带物品和行程准备建议。",
  },
  {
    templateId: "repeat_count_text_medicine_reminder",
    category: "calendar",
    name: "吃药提醒（14次）",
    description: "明天开始每天早上提醒吃药，连续 14 次。",
    frequencyText: "明日 09:00 起，每天 1 次，共 14 次",
    actionKind: "reminder",
    content: "注意身体，别忘记吃药哦。",
  },
  {
    templateId: "repeat_count_agent_diet_plan",
    category: "calendar",
    name: "14天饮食计划",
    description: "明天开始每天早上生成当天的增肌饮食建议，连续 14 次。",
    frequencyText: "明日 08:00 起，每天 1 次，共 14 次",
    actionKind: "agent",
    content: "我最近在增肌，请为我生成今天的饮食建议。",
  },
  {
    templateId: "repeat_until_text_weekly_meeting",
    category: "calendar",
    name: "两个月周会提醒",
    description: "每周五早上周会前 15 分钟提醒，持续 8 周。",
    frequencyText: "每周五 08:45，共 8 周",
    actionKind: "reminder",
    content: "15分钟后周会开始。",
  },
  {
    templateId: "repeat_until_agent_weekly_summary",
    category: "calendar",
    name: "周会前工作总结",
    description: "每周五早上基于最近一周的 memory 生成工作总结，持续 8 周。",
    frequencyText: "每周五 08:30，共 8 周",
    actionKind: "agent",
    content: "请基于最近一周的 memory，生成本周工作总结，供周会前快速回顾。",
  },
];

export function getScheduledTaskTemplate(templateId: string): ScheduledTaskTemplate | undefined {
  return SCHEDULED_TASK_TEMPLATES.find((template) => template.templateId === templateId);
}

// 投影为 AwakenScheduledTaskTemplateDialog 的分类 props；契约规定恰好“循环任务”“日程任务”两类。
export function buildScheduledTaskTemplateCategories(): Array<{
  id: ScheduledTaskTemplateCategoryId;
  label: string;
  templates: Array<Pick<ScheduledTaskTemplate, "templateId" | "name" | "description" | "frequencyText">>;
}> {
  const toCard = (template: ScheduledTaskTemplate) => ({
    templateId: template.templateId,
    name: template.name,
    description: template.description,
    frequencyText: template.frequencyText,
  });
  return [
    { id: "cron", label: "循环任务", templates: SCHEDULED_TASK_TEMPLATES.filter((t) => t.category === "cron").map(toCard) },
    { id: "calendar", label: "日程任务", templates: SCHEDULED_TASK_TEMPLATES.filter((t) => t.category === "calendar").map(toCard) },
  ];
}

// === 时区感知的动态默认时间 ===
// “明日 / 下一个周五 / 下一个 1 月 1 日”必须按所选 IANA 时区的本地历法日期计算，
// 既不是 UTC 也不是机器本地时区；结果写入 draft 的 datetime-local 本地字符串，
// 由 draft.timezone 一并表达（与 AwakenScheduledTaskEditorDialog 的 draft 对齐）。

type ZonedWallClock = {
  year: number;
  month: number;
  day: number;
  hour: number;
  minute: number;
  weekday: number; // 0=周日 … 6=周六
};

const WEEKDAY_INDEX: Record<string, number> = {
  Sun: 0,
  Mon: 1,
  Tue: 2,
  Wed: 3,
  Thu: 4,
  Fri: 5,
  Sat: 6,
};

function zonedWallClock(now: Date, timeZone: string): ZonedWallClock {
  const parts = new Intl.DateTimeFormat("en-US", {
    timeZone,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hourCycle: "h23",
    weekday: "short",
  }).formatToParts(now);
  const take = (type: Intl.DateTimeFormatPartTypes) =>
    parts.find((part) => part.type === type)?.value ?? "";
  return {
    year: Number(take("year")),
    month: Number(take("month")),
    day: Number(take("day")),
    hour: Number(take("hour")),
    minute: Number(take("minute")),
    weekday: WEEKDAY_INDEX[take("weekday")] ?? 0,
  };
}

type LocalDate = { year: number; month: number; day: number };

// 纯历法加减：把 y/m/d 当作 UTC 午夜做推移，避免 DST 影响“过几天”的日期计算。
function addDays(date: LocalDate, delta: number): LocalDate {
  const shifted = new Date(Date.UTC(date.year, date.month - 1, date.day + delta));
  return {
    year: shifted.getUTCFullYear(),
    month: shifted.getUTCMonth() + 1,
    day: shifted.getUTCDate(),
  };
}

function pad2(value: number): string {
  return String(value).padStart(2, "0");
}

function formatLocal(date: LocalDate, hour: number, minute: number): string {
  return `${date.year}-${pad2(date.month)}-${pad2(date.day)}T${pad2(hour)}:${pad2(minute)}`;
}

// 所选时区下下一个未来 1 月 1 日 09:00：当天早于 09:00 才算“未来”，否则推到下一年。
function nextJanuaryFirst(clock: ZonedWallClock): string {
  const thisYearIsFuture = clock.month === 1 && clock.day === 1 && clock.hour * 60 + clock.minute < 9 * 60;
  const year = thisYearIsFuture ? clock.year : clock.year + 1;
  return formatLocal({ year, month: 1, day: 1 }, 9, 0);
}

// 下一个未来周五的指定本地时刻：今天就是周五但已过该时刻时推到下一周。
function nextFriday(clock: ZonedWallClock, hour: number, minute: number): string {
  const today: LocalDate = { year: clock.year, month: clock.month, day: clock.day };
  let delta = (5 - clock.weekday + 7) % 7;
  if (delta === 0 && clock.hour * 60 + clock.minute >= hour * 60 + minute) {
    delta = 7;
  }
  return formatLocal(addDays(today, delta), hour, minute);
}

function tomorrow(clock: ZonedWallClock, hour: number, minute: number): string {
  return formatLocal(addDays({ year: clock.year, month: clock.month, day: clock.day }, 1), hour, minute);
}

// 截止 = 开始后 8 周（56 个历法日），保持同一本地时刻，保证 until 严格晚于 runAt。
function eightWeeksAfter(runAtLocal: string): string {
  const [datePart, timePart] = runAtLocal.split("T");
  const [year, month, day] = datePart.split("-").map(Number);
  const [hour, minute] = timePart.split(":").map(Number);
  return formatLocal(addDays({ year, month, day }, 56), hour, minute);
}

export function buildTemplateDraft(
  templateId: string,
  timezone: string,
  now: Date,
): AwakenScheduledTaskDraft {
  const template = getScheduledTaskTemplate(templateId);
  if (!template) {
    throw new Error(`未知的定时任务模板：${templateId}`);
  }

  const clock = zonedWallClock(now, timezone);
  const draft: AwakenScheduledTaskDraft = {
    name: template.name,
    enabled: true,
    // 项目不猜测，必须由用户在保存前显式选择（契约 §5）。
    projectId: "",
    scheduleKind: template.category,
    cronPreset: "custom",
    cron: "",
    cronTime: "09:00",
    cronWeekdays: [],
    runAtLocal: "",
    repeat: false,
    repeatEveryDays: 1,
    repeatEndKind: "never",
    repeatUntilLocal: "",
    repeatCount: 1,
    timezone,
    actionKind: template.actionKind,
    content: template.content,
    // 空 profileId 表示每次触发时解析当前默认 ready profile。
    profileId: "",
    timeoutSeconds: 600,
    misfireGraceSeconds: 600,
    templateOrigin: { templateId: template.templateId, source: "builtin" },
  };

  if (template.category === "cron") {
    draft.cron = template.cron ?? "";
    switch (template.templateId) {
      case "daily_tech_news_brief":
        draft.cronPreset = "weekly";
        draft.cronTime = "09:30";
        draft.cronWeekdays = [1, 2, 3, 4, 5];
        break;
      case "weekend_relaxation_reminder":
        draft.cronPreset = "weekly";
        draft.cronTime = "10:00";
        draft.cronWeekdays = [0, 6];
        break;
      default:
        draft.cronPreset = "custom";
        draft.cronTime = "09:00";
        draft.cronWeekdays = [];
        break;
    }
    return draft;
  }

  switch (templateId) {
    case "once_text_birthday_reminder":
      draft.runAtLocal = nextJanuaryFirst(clock);
      break;
    case "once_agent_business_trip_prep":
      draft.runAtLocal = tomorrow(clock, 20, 0);
      break;
    case "repeat_count_text_medicine_reminder":
      draft.runAtLocal = tomorrow(clock, 9, 0);
      draft.repeat = true;
      draft.repeatEveryDays = 1;
      draft.repeatEndKind = "count";
      draft.repeatCount = 14;
      break;
    case "repeat_count_agent_diet_plan":
      draft.runAtLocal = tomorrow(clock, 8, 0);
      draft.repeat = true;
      draft.repeatEveryDays = 1;
      draft.repeatEndKind = "count";
      draft.repeatCount = 14;
      break;
    case "repeat_until_text_weekly_meeting":
      draft.runAtLocal = nextFriday(clock, 8, 45);
      draft.repeat = true;
      draft.repeatEveryDays = 7;
      draft.repeatEndKind = "until";
      draft.repeatUntilLocal = eightWeeksAfter(draft.runAtLocal);
      break;
    case "repeat_until_agent_weekly_summary":
      draft.runAtLocal = nextFriday(clock, 8, 30);
      draft.repeat = true;
      draft.repeatEveryDays = 7;
      draft.repeatEndKind = "until";
      draft.repeatUntilLocal = eightWeeksAfter(draft.runAtLocal);
      break;
    default:
      throw new Error(`日程模板缺少动态时间规则：${templateId}`);
  }
  return draft;
}
