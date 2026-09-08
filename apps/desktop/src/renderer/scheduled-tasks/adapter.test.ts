import { describe, expect, it } from "vitest";
import { createEmptyAwakenScheduledTaskDraft } from "@awaken/design-system";
import type {
  ScheduledTask,
  ScheduledTaskExecution,
} from "../../shared/runtime";
import {
  cronPresetForExpression,
  describeCronExpression,
  draftToCreateInput,
  draftToPatchInput,
  formatExecutionItem,
  formatLastRunText,
  formatNextRunText,
  formatScheduleText,
  isTaskAlreadyRunningError,
  isVersionConflictError,
  mapScheduledTaskSubmitError,
  rfc3339ToWallClockLocal,
  taskToDraft,
  wallClockLocalToRfc3339,
} from "./adapter";

function calendarTask(overrides: Partial<ScheduledTask> = {}): ScheduledTask {
  return {
    taskId: "task-1",
    version: 3,
    name: "吃药提醒",
    enabled: true,
    projectId: "project-1",
    schedule: {
      kind: "calendar",
      runAt: "2026-07-28T09:00:00+08:00",
      timezone: "Asia/Shanghai",
      repeatEveryDays: 1,
      repeatEnd: { kind: "count", count: 14 },
    },
    action: { kind: "reminder", content: "别忘记吃药" },
    execution: { profileId: "", timeoutSeconds: 600, misfireGraceSeconds: 600 },
    templateOrigin: { templateId: "repeat_count_text_medicine_reminder", source: "builtin" },
    state: "enabled",
    createdAt: "2026-07-27T00:00:00Z",
    updatedAt: "2026-07-27T00:00:00Z",
    ...overrides,
  };
}

describe("wallClockLocalToRfc3339", () => {
  it("converts Asia/Shanghai wall clock to +08:00 RFC3339", () => {
    expect(wallClockLocalToRfc3339("2026-07-28T09:30", "Asia/Shanghai")).toBe(
      "2026-07-28T09:30:00+08:00",
    );
  });

  it("uses EDT (-04:00) for America/New_York in summer", () => {
    expect(wallClockLocalToRfc3339("2026-07-15T09:00", "America/New_York")).toBe(
      "2026-07-15T09:00:00-04:00",
    );
  });

  it("uses EST (-05:00) for America/New_York in winter", () => {
    expect(wallClockLocalToRfc3339("2026-01-15T09:00", "America/New_York")).toBe(
      "2026-01-15T09:00:00-05:00",
    );
  });

  it("keeps unparseable input untouched for Core validation", () => {
    expect(wallClockLocalToRfc3339("", "Asia/Shanghai")).toBe("");
  });
});

describe("rfc3339ToWallClockLocal", () => {
  it("projects an instant into the target timezone wall clock", () => {
    expect(rfc3339ToWallClockLocal("2026-07-28T01:30:00Z", "Asia/Shanghai")).toBe(
      "2026-07-28T09:30",
    );
    expect(rfc3339ToWallClockLocal("2026-01-15T14:00:00Z", "America/New_York")).toBe(
      "2026-01-15T09:00",
    );
  });

  it("round-trips with wallClockLocalToRfc3339 across DST", () => {
    for (const [local, tz] of [
      ["2026-07-15T09:00", "America/New_York"],
      ["2026-01-15T09:00", "America/New_York"],
      ["2026-07-28T09:30", "Asia/Shanghai"],
    ] as const) {
      expect(rfc3339ToWallClockLocal(wallClockLocalToRfc3339(local, tz), tz)).toBe(local);
    }
  });
});

describe("cronPresetForExpression", () => {
  it("maps preset expressions back to presets", () => {
    expect(cronPresetForExpression("0 * * * *")).toBe("hourly");
    expect(cronPresetForExpression("0 9 * * *")).toBe("daily");
    expect(cronPresetForExpression("0 9 * * 1")).toBe("weekly");
  });

  it("recognizes daily/weekly expressions with arbitrary time", () => {
    expect(cronPresetForExpression("30 9 * * 1-5")).toBe("weekly");
    expect(cronPresetForExpression("0 14 * * *")).toBe("daily");
    expect(cronPresetForExpression("15 10 * * 3,5")).toBe("weekly");
  });
});

describe("draftToCreateInput / draftToPatchInput", () => {
  it("converts a cron draft, trimming the name", () => {
    const draft = createEmptyAwakenScheduledTaskDraft("Asia/Shanghai");
    draft.name = "  每日提醒  ";
    draft.projectId = "project-1";
    draft.content = "喝水";
    expect(draftToCreateInput(draft)).toEqual({
      name: "每日提醒",
      enabled: true,
      projectId: "project-1",
      schedule: { kind: "cron", cron: "0 9 * * *", timezone: "Asia/Shanghai" },
      action: { kind: "reminder", content: "喝水" },
      execution: { profileId: "", timeoutSeconds: 600, misfireGraceSeconds: 600 },
    });
  });

  it("converts a one-shot calendar draft without repeat fields", () => {
    const draft = createEmptyAwakenScheduledTaskDraft("Asia/Shanghai");
    draft.scheduleKind = "calendar";
    draft.runAtLocal = "2026-07-28T09:00";
    const input = draftToCreateInput(draft);
    expect(input.schedule).toEqual({
      kind: "calendar",
      runAt: "2026-07-28T09:00:00+08:00",
      timezone: "Asia/Shanghai",
    });
  });

  it("converts repeat-until and repeat-count calendar drafts", () => {
    const until = createEmptyAwakenScheduledTaskDraft("Asia/Shanghai");
    until.scheduleKind = "calendar";
    until.runAtLocal = "2026-07-31T08:45";
    until.repeat = true;
    until.repeatEveryDays = 7;
    until.repeatEndKind = "until";
    until.repeatUntilLocal = "2026-09-25T08:45";
    expect(draftToCreateInput(until).schedule).toEqual({
      kind: "calendar",
      runAt: "2026-07-31T08:45:00+08:00",
      timezone: "Asia/Shanghai",
      repeatEveryDays: 7,
      repeatEnd: { kind: "until", until: "2026-09-25T08:45:00+08:00" },
    });

    const count = createEmptyAwakenScheduledTaskDraft("Asia/Shanghai");
    count.scheduleKind = "calendar";
    count.runAtLocal = "2026-07-28T09:00";
    count.repeat = true;
    count.repeatEveryDays = 1;
    count.repeatEndKind = "count";
    count.repeatCount = 14;
    expect(draftToCreateInput(count).schedule).toEqual({
      kind: "calendar",
      runAt: "2026-07-28T09:00:00+08:00",
      timezone: "Asia/Shanghai",
      repeatEveryDays: 1,
      repeatEnd: { kind: "count", count: 14 },
    });
  });

  it("omits repeatEnd for never-ending repeats and passes templateOrigin through", () => {
    const draft = createEmptyAwakenScheduledTaskDraft("Asia/Shanghai");
    draft.scheduleKind = "calendar";
    draft.runAtLocal = "2026-07-28T09:00";
    draft.repeat = true;
    draft.repeatEveryDays = 1;
    draft.repeatEndKind = "never";
    draft.templateOrigin = { templateId: "repeat_count_text_medicine_reminder", source: "builtin" };
    const input = draftToCreateInput(draft);
    expect(input.schedule).toEqual({
      kind: "calendar",
      runAt: "2026-07-28T09:00:00+08:00",
      timezone: "Asia/Shanghai",
      repeatEveryDays: 1,
    });
    expect(input.templateOrigin).toEqual({
      templateId: "repeat_count_text_medicine_reminder",
      source: "builtin",
    });
  });

  it("adds expectedVersion for patches", () => {
    const draft = createEmptyAwakenScheduledTaskDraft("UTC");
    expect(draftToPatchInput(draft, 7).expectedVersion).toBe(7);
  });

  it("builds cron from weekly preset time and weekdays", () => {
    const draft = createEmptyAwakenScheduledTaskDraft("Asia/Shanghai");
    draft.name = "周报";
    draft.projectId = "project-1";
    draft.content = "写周报";
    draft.cronPreset = "weekly";
    draft.cronTime = "17:30";
    draft.cronWeekdays = [1, 3, 5];
    draft.cron = "unused-should-be-rebuilt";
    expect(draftToCreateInput(draft).schedule).toEqual({
      kind: "cron",
      cron: "30 17 * * 1,3,5",
      timezone: "Asia/Shanghai",
    });
  });

  it("builds cron from daily preset time", () => {
    const draft = createEmptyAwakenScheduledTaskDraft("Asia/Shanghai");
    draft.cronPreset = "daily";
    draft.cronTime = "08:00";
    draft.cronWeekdays = [];
    expect(draftToCreateInput(draft).schedule).toEqual({
      kind: "cron",
      cron: "0 8 * * *",
      timezone: "Asia/Shanghai",
    });
  });
});

describe("taskToDraft", () => {
  it("round-trips a calendar repeat-count task", () => {
    const draft = taskToDraft(calendarTask());
    expect(draft).toMatchObject({
      name: "吃药提醒",
      enabled: true,
      projectId: "project-1",
      scheduleKind: "calendar",
      runAtLocal: "2026-07-28T09:00",
      repeat: true,
      repeatEveryDays: 1,
      repeatEndKind: "count",
      repeatCount: 14,
      timezone: "Asia/Shanghai",
      actionKind: "reminder",
      content: "别忘记吃药",
      templateOrigin: { templateId: "repeat_count_text_medicine_reminder", source: "builtin" },
    });
    expect(draftToCreateInput(draft)).toEqual({
      name: "吃药提醒",
      enabled: true,
      projectId: "project-1",
      schedule: calendarTask().schedule,
      action: { kind: "reminder", content: "别忘记吃药" },
      execution: { profileId: "", timeoutSeconds: 600, misfireGraceSeconds: 600 },
      templateOrigin: { templateId: "repeat_count_text_medicine_reminder", source: "builtin" },
    });
  });

  it("recognizes preset cron expressions and custom ones", () => {
    const daily = taskToDraft(
      calendarTask({ schedule: { kind: "cron", cron: "0 9 * * *", timezone: "UTC" } }),
    );
    expect(daily.cronPreset).toBe("daily");
    expect(daily.cronTime).toBe("09:00");

    const weekly = taskToDraft(
      calendarTask({ schedule: { kind: "cron", cron: "30 17 * * 1,3,5", timezone: "UTC" } }),
    );
    expect(weekly.cronPreset).toBe("weekly");
    expect(weekly.cronTime).toBe("17:30");
    expect(weekly.cronWeekdays).toEqual([1, 3, 5]);

    const custom = taskToDraft(
      calendarTask({ schedule: { kind: "cron", cron: "*/25 9-17 * * 1-5", timezone: "UTC" } }),
    );
    expect(custom.cronPreset).toBe("custom");
    expect(custom.cron).toBe("*/25 9-17 * * 1-5");
  });
});

describe("describeCronExpression / formatScheduleText", () => {
  it("describes common expressions", () => {
    expect(describeCronExpression("30 9 * * 1-5")).toBe("工作日 09:30");
    expect(describeCronExpression("0 10 * * 6,0")).toBe("周六/周日 10:00");
    expect(describeCronExpression("0 9 * * *")).toBe("每天 09:00");
    expect(describeCronExpression("0 9 * * 1")).toBe("每周一 09:00");
    expect(describeCronExpression("0 * * * *")).toBe("每小时第 0 分钟");
    expect(describeCronExpression("0 20 15 * *")).toBe("每月 15 日 20:00");
    expect(describeCronExpression("*/25 9-17 * * 1-5")).toBe("工作日 09:00–17:00 每 25 分钟");
    expect(describeCronExpression("not-a-cron")).toBe("Cron not-a-cron");
  });

  it("formats cron schedule text with timezone", () => {
    const task = calendarTask({
      schedule: { kind: "cron", cron: "30 9 * * 1-5", timezone: "Asia/Shanghai" },
    });
    expect(formatScheduleText(task)).toBe("工作日 09:30 · 中国标准时间（东8小时）");
  });

  it("formats calendar schedule text with repeat rules", () => {
    expect(formatScheduleText(calendarTask())).toBe(
      "2026-07-28 09:00 · 每 1 天 · 限 14 次 · 中国标准时间（东8小时）",
    );
    const once = calendarTask({
      schedule: {
        kind: "calendar",
        runAt: "2027-01-01T09:00:00+08:00",
        timezone: "Asia/Shanghai",
      },
    });
    expect(formatScheduleText(once)).toBe("2027-01-01 09:00 · 不重复 · 中国标准时间（东8小时）");
    const until = calendarTask({
      schedule: {
        kind: "calendar",
        runAt: "2026-07-31T08:45:00+08:00",
        timezone: "Asia/Shanghai",
        repeatEveryDays: 7,
        repeatEnd: { kind: "until", until: "2026-09-25T08:45:00+08:00" },
      },
    });
    expect(formatScheduleText(until)).toBe(
      "2026-07-31 08:45 · 每 7 天 · 截至 2026-09-25 08:45 · 中国标准时间（东8小时）",
    );
  });
});

describe("run text formatting", () => {
  it("formats next run and last run", () => {
    const task = calendarTask({
      nextRunAt: "2026-07-28T01:00:00Z",
      lastExecution: {
        executionId: "exec-1",
        status: "success",
        startedAt: "2026-07-27T01:00:00Z",
        finishedAt: "2026-07-27T01:00:42Z",
      },
    });
    expect(formatNextRunText(task, "UTC")).toBe("下次 2026-07-28 01:00");
    expect(formatLastRunText(task, "UTC")).toBe("最近 2026-07-27 01:00 成功");
    expect(formatNextRunText(calendarTask({ nextRunAt: null }), "UTC")).toBeNull();
    expect(formatLastRunText(calendarTask({ lastExecution: null }), "UTC")).toBeNull();
  });

  it("formats execution history items with trigger labels", () => {
    const execution: ScheduledTaskExecution = {
      executionId: "exec-1",
      taskId: "task-1",
      taskVersion: 3,
      trigger: "misfire",
      scheduledFor: "2026-07-27T01:00:00Z",
      startedAt: "2026-07-27T02:00:00Z",
      finishedAt: "2026-07-27T02:00:10Z",
      status: "error",
      threadId: "thread-1",
      errorMessage: "provider timeout",
    };
    expect(formatExecutionItem(execution, "UTC")).toEqual({
      executionId: "exec-1",
      triggerLabel: "补跑",
      status: "error",
      statusLabel: "失败",
      scheduledForText: "2026-07-27 01:00",
      startedAtText: "2026-07-27 02:00",
      finishedAtText: "2026-07-27 02:00",
      errorText: "provider timeout",
      threadId: "thread-1",
    });
  });
});

describe("submit error mapping", () => {
  const ipc = (message: string) =>
    new Error(`Error invoking remote method 'awaken:create-scheduled-task': RuntimeDomainError: ${message}`);

  it("maps validation messages to editor field errors", () => {
    expect(mapScheduledTaskSubmitError(ipc("VALIDATION_FAILED: invalid schedule.cron: is not a valid five-field cron expression"))).toEqual({
      errorMessage: null,
      fieldErrors: { cron: "is not a valid five-field cron expression" },
    });
    expect(mapScheduledTaskSubmitError(ipc("VALIDATION_FAILED: invalid name: must be 1-120 characters"))).toEqual({
      errorMessage: null,
      fieldErrors: { name: "must be 1-120 characters" },
    });
    expect(
      mapScheduledTaskSubmitError(ipc("VALIDATION_FAILED: invalid schedule.repeat_end.until: must be later than run_at")),
    ).toEqual({
      errorMessage: null,
      fieldErrors: { repeatUntil: "must be later than run_at" },
    });
  });

  it("maps version conflicts to a stable message", () => {
    const mapped = mapScheduledTaskSubmitError(ipc("VERSION_CONFLICT: task was modified; reload and retry"));
    expect(mapped.fieldErrors).toEqual({});
    expect(mapped.errorMessage).toBe("任务已被其他地方修改，请刷新后重试。");
    expect(isVersionConflictError(ipc("VERSION_CONFLICT: task was modified; reload and retry"))).toBe(true);
  });

  it("detects already-running conflicts", () => {
    expect(isTaskAlreadyRunningError(ipc("TASK_ALREADY_RUNNING: task already has an active execution"))).toBe(true);
    expect(isTaskAlreadyRunningError(new Error("something else"))).toBe(false);
  });

  it("falls back to the full message when the field is unknown", () => {
    expect(
      mapScheduledTaskSubmitError(ipc("VALIDATION_FAILED: invalid execution.profile_id: profile is not ready")),
    ).toEqual({
      errorMessage: "invalid execution.profile_id: profile is not ready",
      fieldErrors: {},
    });
  });
});
