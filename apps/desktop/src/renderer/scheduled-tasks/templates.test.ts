import { describe, expect, it } from "vitest";
import {
  buildScheduledTaskTemplateCategories,
  buildTemplateDraft,
  SCHEDULED_TASK_TEMPLATES,
} from "./templates";

// === 测试辅助：把 “YYYY-MM-DDTHH:mm” 按 IANA 时区还原成绝对时间 ===

function wallClockMs(instantMs: number, timeZone: string): number {
  const parts = new Intl.DateTimeFormat("en-CA", {
    timeZone,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hourCycle: "h23",
  }).formatToParts(new Date(instantMs));
  const take = (type: string) => Number(parts.find((part) => part.type === type)?.value ?? "0");
  return Date.UTC(take("year"), take("month") - 1, take("day"), take("hour"), take("minute"), take("second"));
}

function zonedLocalToDate(local: string, timeZone: string): Date {
  const [datePart, timePart] = local.split("T");
  const [year, month, day] = datePart.split("-").map(Number);
  const [hour, minute] = timePart.split(":").map(Number);
  const target = Date.UTC(year, month - 1, day, hour, minute, 0);
  let instant = target;
  for (let attempt = 0; attempt < 4; attempt += 1) {
    instant = target - (wallClockMs(instant, timeZone) - instant);
  }
  if (wallClockMs(instant, timeZone) !== target) {
    throw new Error(`无法在 ${timeZone} 解析本地时间 ${local}`);
  }
  return new Date(instant);
}

function localDate(local: string): { year: number; month: number; day: number } {
  const [year, month, day] = local.split("T")[0].split("-").map(Number);
  return { year, month, day };
}

// 历法日期的星期几与时区无关，直接按 UTC 午夜取。
function weekdayOf(local: string): number {
  const { year, month, day } = localDate(local);
  return new Date(Date.UTC(year, month - 1, day)).getUTCDay();
}

function calendarDayDiff(fromLocal: string, toLocal: string): number {
  const from = localDate(fromLocal);
  const to = localDate(toLocal);
  return Math.round(
    (Date.UTC(to.year, to.month - 1, to.day) - Date.UTC(from.year, from.month - 1, from.day)) / 86_400_000,
  );
}

// 与实现无关的“该时区当前本地日期”，用于独立推算“明日”。
function zonedToday(now: Date, timeZone: string): { year: number; month: number; day: number } {
  const parts = new Intl.DateTimeFormat("en-CA", {
    timeZone,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).formatToParts(now);
  const take = (type: string) => Number(parts.find((part) => part.type === type)?.value ?? "0");
  return { year: take("year"), month: take("month"), day: take("day") };
}

function expectedTomorrow(now: Date, timeZone: string): string {
  const today = zonedToday(now, timeZone);
  const shifted = new Date(Date.UTC(today.year, today.month - 1, today.day + 1));
  return `${shifted.getUTCFullYear()}-${String(shifted.getUTCMonth() + 1).padStart(2, "0")}-${String(shifted.getUTCDate()).padStart(2, "0")}`;
}

const TIMEZONES = ["Asia/Shanghai", "America/New_York", "Europe/Berlin"] as const;
const FIXED_NOW = new Date("2026-07-26T12:00:00Z"); // 周日

describe("SCHEDULED_TASK_TEMPLATES 目录", () => {
  it("恰好 10 项、ID 唯一、分类 4/6", () => {
    expect(SCHEDULED_TASK_TEMPLATES).toHaveLength(10);
    expect(new Set(SCHEDULED_TASK_TEMPLATES.map((t) => t.templateId)).size).toBe(10);
    expect(SCHEDULED_TASK_TEMPLATES.filter((t) => t.category === "cron")).toHaveLength(4);
    expect(SCHEDULED_TASK_TEMPLATES.filter((t) => t.category === "calendar")).toHaveLength(6);
  });

  it("每个模板字段完整", () => {
    for (const template of SCHEDULED_TASK_TEMPLATES) {
      expect(template.name.trim().length).toBeGreaterThan(0);
      expect(template.description.trim().length).toBeGreaterThan(0);
      expect(template.frequencyText.trim().length).toBeGreaterThan(0);
      expect(template.content.trim().length).toBeGreaterThan(0);
      expect(["reminder", "agent"]).toContain(template.actionKind);
      if (template.category === "cron") {
        expect(template.cron?.trim().length).toBeGreaterThan(0);
      } else {
        expect(template.cron).toBeUndefined();
      }
    }
  });

  // 契约：[内置模板目录](docs/12_定时任务/定时执行与提醒/需求文档.md#内置模板目录)
  it("模板名称逐字符合契约 “内置模板目录”", () => {
    expect(SCHEDULED_TASK_TEMPLATES.map((t) => [t.templateId, t.name])).toEqual([
      ["daily_tech_news_brief", "工作日科技新闻早报"],
      ["weekend_relaxation_reminder", "周末电影推荐"],
      ["pomodoro_break_reminder", "休息提醒"],
      ["pet_care_reminder", "宠物驱虫/疫苗提醒"],
      ["once_text_birthday_reminder", "生日提醒"],
      ["once_agent_business_trip_prep", "出差前天气与行程准备"],
      ["repeat_count_text_medicine_reminder", "吃药提醒（14次）"],
      ["repeat_count_agent_diet_plan", "14天饮食计划"],
      ["repeat_until_text_weekly_meeting", "两个月周会提醒"],
      ["repeat_until_agent_weekly_summary", "周会前工作总结"],
    ]);
  });

  // 契约：[循环任务](docs/12_定时任务/定时执行与提醒/需求文档.md#循环任务)
  it("cron 表达式与契约 “循环任务” 一致", () => {
    const cronById = Object.fromEntries(
      SCHEDULED_TASK_TEMPLATES.filter((t) => t.category === "cron").map((t) => [t.templateId, t.cron]),
    );
    expect(cronById).toEqual({
      daily_tech_news_brief: "30 9 * * 1-5",
      weekend_relaxation_reminder: "0 10 * * 6,0",
      pomodoro_break_reminder: "*/25 9-17 * * 1-5",
      pet_care_reminder: "0 20 15 * *",
    });
  });

  // 契约：[循环任务](docs/12_定时任务/定时执行与提醒/需求文档.md#循环任务)、[日程任务](docs/12_定时任务/定时执行与提醒/需求文档.md#日程任务)
  it("模板正文逐字符合契约 “循环任务”/“日程任务”", () => {
    const contentById = Object.fromEntries(SCHEDULED_TASK_TEMPLATES.map((t) => [t.templateId, t.content]));
    expect(contentById).toEqual({
      daily_tech_news_brief: "整理今天最值得关注的科技新闻，输出 5–8 条；每条含标题、核心进展、关注原因，最后给出趋势判断",
      weekend_relaxation_reminder: "推荐 5 部近期热度高、口碑好的电影，给出类型、看点、适合人群和可得的上映/平台信息",
      pomodoro_break_reminder: "持续工作25分钟啦，起来活动一下，喝口水，顺便看看远处放松眼睛～",
      pet_care_reminder: "小提醒：今天记得给毛孩子安排驱虫/疫苗检查喔～",
      once_text_birthday_reminder: "xx在1月1日过生日，别忘了送上祝福～",
      once_agent_business_trip_prep: "我明天要出差，请查询目的地天气，并给出穿衣、携带物品和行程准备建议。",
      repeat_count_text_medicine_reminder: "注意身体，别忘记吃药哦。",
      repeat_count_agent_diet_plan: "我最近在增肌，请为我生成今天的饮食建议。",
      repeat_until_text_weekly_meeting: "15分钟后周会开始。",
      repeat_until_agent_weekly_summary: "请基于最近一周的 memory，生成本周工作总结，供周会前快速回顾。",
    });
  });

  it("模板类型与契约一致", () => {
    const kindById = Object.fromEntries(SCHEDULED_TASK_TEMPLATES.map((t) => [t.templateId, t.actionKind]));
    expect(kindById).toEqual({
      daily_tech_news_brief: "agent",
      weekend_relaxation_reminder: "agent",
      pomodoro_break_reminder: "reminder",
      pet_care_reminder: "reminder",
      once_text_birthday_reminder: "reminder",
      once_agent_business_trip_prep: "agent",
      repeat_count_text_medicine_reminder: "reminder",
      repeat_count_agent_diet_plan: "agent",
      repeat_until_text_weekly_meeting: "reminder",
      repeat_until_agent_weekly_summary: "agent",
    });
  });

  it("分类投影恰好是循环任务/日程任务两类", () => {
    const categories = buildScheduledTaskTemplateCategories();
    expect(categories.map((c) => c.label)).toEqual(["循环任务", "日程任务"]);
    expect(categories[0].templates).toHaveLength(4);
    expect(categories[1].templates).toHaveLength(6);
    for (const category of categories) {
      for (const card of category.templates) {
        expect(card.name.trim().length).toBeGreaterThan(0);
        expect(card.description.trim().length).toBeGreaterThan(0);
        expect(card.frequencyText.trim().length).toBeGreaterThan(0);
      }
    }
  });
});

describe("buildTemplateDraft 通用默认", () => {
  it("所有模板共享契约默认值", () => {
    for (const timezone of TIMEZONES) {
      for (const template of SCHEDULED_TASK_TEMPLATES) {
        const draft = buildTemplateDraft(template.templateId, timezone, FIXED_NOW);
        expect(draft.name).toBe(template.name);
        expect(draft.enabled).toBe(true);
        expect(draft.projectId).toBe("");
        expect(draft.profileId).toBe("");
        expect(draft.timeoutSeconds).toBe(600);
        expect(draft.misfireGraceSeconds).toBe(600);
        expect(draft.timezone).toBe(timezone);
        expect(draft.actionKind).toBe(template.actionKind);
        expect(draft.content).toBe(template.content);
        expect(draft.templateOrigin).toEqual({ templateId: template.templateId, source: "builtin" });
        expect(draft.scheduleKind).toBe(template.category);
      }
    }
  });

  it("未知模板 ID 抛错", () => {
    expect(() => buildTemplateDraft("no_such_template", "Asia/Shanghai", FIXED_NOW)).toThrow();
  });

  it("cron 模板预填表达式且不产生日程字段", () => {
    const daily = buildTemplateDraft("daily_tech_news_brief", "Asia/Shanghai", FIXED_NOW);
    expect(daily.scheduleKind).toBe("cron");
    expect(daily.cronPreset).toBe("weekly");
    expect(daily.cron).toBe("30 9 * * 1-5");
    expect(daily.cronTime).toBe("09:30");
    expect(daily.cronWeekdays).toEqual([1, 2, 3, 4, 5]);
    expect(daily.runAtLocal).toBe("");
    expect(daily.repeat).toBe(false);

    const weekend = buildTemplateDraft("weekend_relaxation_reminder", "Asia/Shanghai", FIXED_NOW);
    expect(weekend.cronPreset).toBe("weekly");
    expect(weekend.cron).toBe("0 10 * * 6,0");
    expect(weekend.cronTime).toBe("10:00");
    expect(weekend.cronWeekdays).toEqual([0, 6]);

    const custom = buildTemplateDraft("pomodoro_break_reminder", "Asia/Shanghai", FIXED_NOW);
    expect(custom.cronPreset).toBe("custom");
    expect(custom.cron).toBe("*/25 9-17 * * 1-5");
  });
});

describe("buildTemplateDraft 日程动态时间", () => {
  it.each(TIMEZONES)("生日提醒：%s 下一个未来 1 月 1 日 09:00，不重复", (timezone) => {
    const draft = buildTemplateDraft("once_text_birthday_reminder", timezone, FIXED_NOW);
    expect(draft.runAtLocal.endsWith("T09:00")).toBe(true);
    expect(draft.runAtLocal.slice(5, 10)).toBe("01-01");
    expect(zonedLocalToDate(draft.runAtLocal, timezone).getTime()).toBeGreaterThan(FIXED_NOW.getTime());
    expect(draft.repeat).toBe(false);
  });

  it("生日提醒：1 月 1 日 09:00 前用当年，之后用下一年", () => {
    // 上海 2026-01-01 04:00（UTC 2025-12-31 20:00）→ 当天 09:00 仍在未来
    expect(buildTemplateDraft("once_text_birthday_reminder", "Asia/Shanghai", new Date("2025-12-31T20:00:00Z")).runAtLocal)
      .toBe("2026-01-01T09:00");
    // 上海 2026-01-01 10:00（UTC 2026-01-01 02:00）→ 推到 2027 年
    expect(buildTemplateDraft("once_text_birthday_reminder", "Asia/Shanghai", new Date("2026-01-01T02:00:00Z")).runAtLocal)
      .toBe("2027-01-01T09:00");
  });

  it.each(TIMEZONES)("出差准备：%s 明日 20:00，不重复", (timezone) => {
    const draft = buildTemplateDraft("once_agent_business_trip_prep", timezone, FIXED_NOW);
    expect(draft.runAtLocal).toBe(`${expectedTomorrow(FIXED_NOW, timezone)}T20:00`);
    expect(zonedLocalToDate(draft.runAtLocal, timezone).getTime()).toBeGreaterThan(FIXED_NOW.getTime());
    expect(draft.repeat).toBe(false);
  });

  it.each(TIMEZONES)("吃药/饮食：%s 明日起每 1 天，限定 14 次", (timezone) => {
    const tomorrowDate = expectedTomorrow(FIXED_NOW, timezone);
    const medicine = buildTemplateDraft("repeat_count_text_medicine_reminder", timezone, FIXED_NOW);
    expect(medicine.runAtLocal).toBe(`${tomorrowDate}T09:00`);
    expect(medicine.repeat).toBe(true);
    expect(medicine.repeatEveryDays).toBe(1);
    expect(medicine.repeatEndKind).toBe("count");
    expect(medicine.repeatCount).toBe(14);
    expect(zonedLocalToDate(medicine.runAtLocal, timezone).getTime()).toBeGreaterThan(FIXED_NOW.getTime());

    const diet = buildTemplateDraft("repeat_count_agent_diet_plan", timezone, FIXED_NOW);
    expect(diet.runAtLocal).toBe(`${tomorrowDate}T08:00`);
    expect(diet.repeatEveryDays).toBe(1);
    expect(diet.repeatEndKind).toBe("count");
    expect(diet.repeatCount).toBe(14);
    expect(zonedLocalToDate(diet.runAtLocal, timezone).getTime()).toBeGreaterThan(FIXED_NOW.getTime());
  });

  it.each(TIMEZONES)("周会提醒/周总结：%s 下一个未来周五，每 7 天，截止开始后 8 周", (timezone) => {
    const meeting = buildTemplateDraft("repeat_until_text_weekly_meeting", timezone, FIXED_NOW);
    expect(weekdayOf(meeting.runAtLocal)).toBe(5);
    expect(meeting.runAtLocal.endsWith("T08:45")).toBe(true);
    expect(meeting.repeatEveryDays).toBe(7);
    expect(meeting.repeatEndKind).toBe("until");
    expect(calendarDayDiff(meeting.runAtLocal, meeting.repeatUntilLocal)).toBe(56);
    expect(zonedLocalToDate(meeting.runAtLocal, timezone).getTime()).toBeGreaterThan(FIXED_NOW.getTime());
    expect(zonedLocalToDate(meeting.repeatUntilLocal, timezone).getTime())
      .toBeGreaterThan(zonedLocalToDate(meeting.runAtLocal, timezone).getTime());

    const summary = buildTemplateDraft("repeat_until_agent_weekly_summary", timezone, FIXED_NOW);
    expect(weekdayOf(summary.runAtLocal)).toBe(5);
    expect(summary.runAtLocal.endsWith("T08:30")).toBe(true);
    expect(summary.repeatEveryDays).toBe(7);
    expect(summary.repeatEndKind).toBe("until");
    expect(calendarDayDiff(summary.runAtLocal, summary.repeatUntilLocal)).toBe(56);
    expect(zonedLocalToDate(summary.runAtLocal, timezone).getTime()).toBeGreaterThan(FIXED_NOW.getTime());
    expect(zonedLocalToDate(summary.repeatUntilLocal, timezone).getTime())
      .toBeGreaterThan(zonedLocalToDate(summary.runAtLocal, timezone).getTime());
  });

  it("周五当天：早于目标时刻用当天，已过则推到下一周", () => {
    // 上海周五 2026-07-31 06:00（UTC 2026-07-30 22:00）→ 当天 08:45 仍在未来
    expect(buildTemplateDraft("repeat_until_text_weekly_meeting", "Asia/Shanghai", new Date("2026-07-30T22:00:00Z")).runAtLocal)
      .toBe("2026-07-31T08:45");
    // 上海周五 2026-07-31 09:00（UTC 2026-07-31 01:00）→ 已过 08:45，推到 8 月 7 日
    expect(buildTemplateDraft("repeat_until_text_weekly_meeting", "Asia/Shanghai", new Date("2026-07-31T01:00:00Z")).runAtLocal)
      .toBe("2026-08-07T08:45");
    // 纽约周四晚上（UTC 2026-07-31 02:00 = EDT 周四 22:00）→ 次日周五
    expect(buildTemplateDraft("repeat_until_agent_weekly_summary", "America/New_York", new Date("2026-07-31T02:00:00Z")).runAtLocal)
      .toBe("2026-07-31T08:30");
  });
});
