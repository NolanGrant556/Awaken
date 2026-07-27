// 定时任务 Renderer 适配层：编辑器 draft ↔ Core wire payload 的唯一转换点。
// Design System 只懂 draft 与展示字符串，Core 只懂 RFC3339/cron/字段校验；
// 时区 offset 换算、cron 人类可读文案、HTTP 错误 → fieldErrors 的映射都收敛在这里。
import {
  AWAKEN_SCHEDULED_TASK_CRON_PRESET_EXPRESSIONS,
  buildCronExpressionFromPreset,
  createEmptyAwakenScheduledTaskDraft,
  type AwakenScheduledTaskCronPreset,
  type AwakenScheduledTaskDraft,
  type AwakenScheduledTaskDraftField,
  type AwakenScheduledTaskExecutionItem,
} from "@awaken/design-system";
import type {
  ScheduledTask,
  ScheduledTaskActionKind,
  ScheduledTaskCreateInput,
  ScheduledTaskExecution,
  ScheduledTaskExecutionStatus,
  ScheduledTaskExecutionTrigger,
  ScheduledTaskPatchInput,
  ScheduledTaskSchedule,
  ScheduledTaskState,
} from "../../shared/runtime";

// === 时区换算：datetime-local 本地字符串 ↔ 带 offset 的 RFC3339 ===
// 只用 Intl API：先假设本地时刻是 UTC，求该时区在附近时刻的 offset，
// 再迭代一次消除 DST 边界误差（与 templates.ts 的 zonedWallClock 同一思路）。

function pad2(value: number): string {
  return String(value).padStart(2, "0");
}

const WALL_CLOCK_FORMAT: Intl.DateTimeFormatOptions = {
  year: "numeric",
  month: "2-digit",
  day: "2-digit",
  hour: "2-digit",
  minute: "2-digit",
  second: "2-digit",
  hourCycle: "h23",
};

function wallClockParts(
  instant: Date,
  timeZone: string,
): { year: number; month: number; day: number; hour: number; minute: number; second: number } {
  const parts = new Intl.DateTimeFormat("en-US", {
    ...WALL_CLOCK_FORMAT,
    timeZone,
  }).formatToParts(instant);
  const take = (type: Intl.DateTimeFormatPartTypes) =>
    Number(parts.find((part) => part.type === type)?.value ?? "0");
  return {
    year: take("year"),
    month: take("month"),
    day: take("day"),
    hour: take("hour"),
    minute: take("minute"),
    second: take("second"),
  };
}

/** 时区在某一瞬间相对 UTC 的偏移（分钟，东正西负）。 */
export function timezoneOffsetMinutesAt(instantMs: number, timeZone: string): number {
  const wall = wallClockParts(new Date(instantMs), timeZone);
  const wallAsUtc = Date.UTC(wall.year, wall.month - 1, wall.day, wall.hour, wall.minute, wall.second);
  return Math.round((wallAsUtc - instantMs) / 60000);
}

/**
 * 把 "YYYY-MM-DDTHH:mm"（datetime-local）按 IANA 时区解释为真实时刻，
 * 输出带 offset 的 RFC3339（如 2026-07-28T09:30:00+08:00）。
 * 输入无法解析时原样返回，交给 Core 校验并通过 fieldErrors 回显。
 */
export function wallClockLocalToRfc3339(local: string, timeZone: string): string {
  const match = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})(?::(\d{2}))?/.exec(local.trim());
  if (!match) {
    return local;
  }
  const [, year, month, day, hour, minute, second] = match;
  const naiveUtc = Date.UTC(
    Number(year),
    Number(month) - 1,
    Number(day),
    Number(hour),
    Number(minute),
    Number(second ?? "0"),
  );
  let offsetMinutes = timezoneOffsetMinutesAt(naiveUtc, timeZone);
  let instantMs = naiveUtc - offsetMinutes * 60000;
  // 第二次迭代：DST 切换边界附近第一次求出的 offset 可能差一小时。
  offsetMinutes = timezoneOffsetMinutesAt(instantMs, timeZone);
  instantMs = naiveUtc - offsetMinutes * 60000;

  const sign = offsetMinutes < 0 ? "-" : "+";
  const abs = Math.abs(offsetMinutes);
  const offsetText = `${sign}${pad2(Math.floor(abs / 60))}:${pad2(abs % 60)}`;
  // RFC3339 呈现该时区的本地墙钟 + offset，而不是 UTC 时刻 + offset。
  const wall = wallClockParts(new Date(instantMs), timeZone);
  return `${wall.year}-${pad2(wall.month)}-${pad2(wall.day)}T${pad2(wall.hour)}:${pad2(wall.minute)}:${pad2(wall.second)}${offsetText}`;
}

/** RFC3339 → 指定时区的 datetime-local 本地字符串，用于编辑回显。 */
export function rfc3339ToWallClockLocal(rfc3339: string, timeZone: string): string {
  const instantMs = Date.parse(rfc3339);
  if (!Number.isFinite(instantMs)) {
    return "";
  }
  const wall = wallClockParts(new Date(instantMs), timeZone);
  return `${wall.year}-${pad2(wall.month)}-${pad2(wall.day)}T${pad2(wall.hour)}:${pad2(wall.minute)}`;
}

// === draft ↔ wire ===

/** cron 表达式反推编辑器 preset；不在 daily/weekly/hourly 模式内则为 custom。 */
export function cronPresetForExpression(expression: string): AwakenScheduledTaskCronPreset {
  const normalized = expression.trim();
  for (const [preset, presetExpression] of Object.entries(
    AWAKEN_SCHEDULED_TASK_CRON_PRESET_EXPRESSIONS,
  )) {
    if (presetExpression === normalized) {
      return preset as Exclude<AwakenScheduledTaskCronPreset, "custom">;
    }
  }
  if (/^\d{1,2} \d{1,2} \* \* \*$/.test(normalized)) return "daily";
  if (/^\d{1,2} \d{1,2} \* \* [\d,\-]+$/.test(normalized)) return "weekly";
  if (/^\d{1,2} \* \* \* \*$/.test(normalized)) return "hourly";
  return "custom";
}

function parseCronTime(expression: string): string {
  const fields = expression.trim().split(/\s+/);
  const minute = fields[0] ?? "0";
  const hour = fields[1] ?? "9";
  return `${hour.padStart(2, "0")}:${minute.padStart(2, "0")}`;
}

function parseCronWeekdays(expression: string): number[] {
  const fields = expression.trim().split(/\s+/);
  const dowField = fields[4] ?? "1";
  const days = new Set<number>();
  for (const part of dowField.split(",")) {
    if (/^\d+$/.test(part)) {
      const d = Number(part) % 7;
      days.add(d);
    } else if (/^(\d+)-(\d+)$/.test(part)) {
      const start = Number(RegExp.$1);
      const end = Number(RegExp.$2);
      for (let d = start; d <= end; d += 1) {
        days.add(d % 7);
      }
    }
  }
  return [...days].sort((a, b) => a - b);
}

function scheduleFromDraft(draft: AwakenScheduledTaskDraft): ScheduledTaskSchedule {
  if (draft.scheduleKind === "cron") {
    const cron =
      draft.cronPreset === "custom"
        ? draft.cron.trim()
        : buildCronExpressionFromPreset(draft.cronPreset, draft.cronTime, draft.cronWeekdays);
    return { kind: "cron", cron, timezone: draft.timezone };
  }
  const schedule: ScheduledTaskSchedule = {
    kind: "calendar",
    runAt: wallClockLocalToRfc3339(draft.runAtLocal, draft.timezone),
    timezone: draft.timezone,
  };
  if (draft.repeat) {
    schedule.repeatEveryDays = draft.repeatEveryDays;
    if (draft.repeatEndKind === "until") {
      schedule.repeatEnd = {
        kind: "until",
        until: wallClockLocalToRfc3339(draft.repeatUntilLocal, draft.timezone),
      };
    } else if (draft.repeatEndKind === "count") {
      schedule.repeatEnd = { kind: "count", count: draft.repeatCount };
    }
    // "never" 用省略 repeatEnd 表达（Core 约定：nil repeat_end = 永不结束）。
  }
  return schedule;
}

export function draftToCreateInput(draft: AwakenScheduledTaskDraft): ScheduledTaskCreateInput {
  return {
    name: draft.name.trim(),
    enabled: draft.enabled,
    projectId: draft.projectId,
    schedule: scheduleFromDraft(draft),
    action: { kind: draft.actionKind, content: draft.content },
    execution: {
      profileId: draft.profileId,
      timeoutSeconds: draft.timeoutSeconds,
      misfireGraceSeconds: draft.misfireGraceSeconds,
    },
    ...(draft.templateOrigin
      ? {
          templateOrigin: {
            templateId: draft.templateOrigin.templateId,
            source: draft.templateOrigin.source,
          },
        }
      : {}),
  };
}

export function draftToPatchInput(
  draft: AwakenScheduledTaskDraft,
  expectedVersion: number,
): ScheduledTaskPatchInput {
  return { expectedVersion, ...draftToCreateInput(draft) };
}

/** 编辑回显：Core 任务 → 编辑器 draft。 */
export function taskToDraft(task: ScheduledTask): AwakenScheduledTaskDraft {
  const draft = createEmptyAwakenScheduledTaskDraft(task.schedule.timezone);
  draft.name = task.name;
  draft.enabled = task.enabled;
  draft.projectId = task.projectId;
  draft.actionKind = task.action.kind;
  draft.content = task.action.content;
  draft.profileId = task.execution.profileId;
  draft.timeoutSeconds = task.execution.timeoutSeconds;
  draft.misfireGraceSeconds = task.execution.misfireGraceSeconds;
  // 模板目录只有 builtin 一类来源；其他来源按内置记录，仅作展示回溯。
  draft.templateOrigin = task.templateOrigin
    ? { templateId: task.templateOrigin.templateId, source: "builtin" }
    : null;

  if (task.schedule.kind === "cron") {
    draft.scheduleKind = "cron";
    draft.cron = task.schedule.cron;
    draft.cronPreset = cronPresetForExpression(task.schedule.cron);
    draft.cronTime = parseCronTime(task.schedule.cron);
    draft.cronWeekdays = parseCronWeekdays(task.schedule.cron);
    return draft;
  }

  draft.scheduleKind = "calendar";
  draft.runAtLocal = rfc3339ToWallClockLocal(task.schedule.runAt, task.schedule.timezone);
  if (task.schedule.repeatEveryDays !== undefined) {
    draft.repeat = true;
    draft.repeatEveryDays = task.schedule.repeatEveryDays;
    const repeatEnd = task.schedule.repeatEnd;
    draft.repeatEndKind = repeatEnd?.kind ?? "never";
    if (repeatEnd?.kind === "until" && repeatEnd.until) {
      draft.repeatUntilLocal = rfc3339ToWallClockLocal(repeatEnd.until, task.schedule.timezone);
    }
    if (repeatEnd?.kind === "count" && repeatEnd.count !== undefined) {
      draft.repeatCount = repeatEnd.count;
    }
  }
  return draft;
}

// === 展示文案 ===

export const SCHEDULED_TASK_STATE_LABELS: Record<ScheduledTaskState, string> = {
  enabled: "启用",
  paused: "已暂停",
  running: "运行中",
  completed: "已完成",
  invalid: "配置无效",
};

export const SCHEDULED_TASK_KIND_LABELS: Record<ScheduledTaskActionKind, string> = {
  reminder: "提醒",
  agent: "Agent",
};

export const SCHEDULED_TASK_EXECUTION_STATUS_LABELS: Record<ScheduledTaskExecutionStatus, string> = {
  queued: "排队中",
  running: "运行中",
  success: "成功",
  error: "失败",
  skipped: "已跳过",
  cancelled: "已取消",
  interrupted: "已中断",
};

export const SCHEDULED_TASK_TRIGGER_LABELS: Record<ScheduledTaskExecutionTrigger, string> = {
  scheduled: "定时",
  manual: "手动",
  misfire: "补跑",
};

const CRON_DOW_LABELS: Record<string, string> = {
  "0": "周日",
  "1": "周一",
  "2": "周二",
  "3": "周三",
  "4": "周四",
  "5": "周五",
  "6": "周六",
  "7": "周日",
};

function cronDowText(field: string): string {
  if (field === "1-5") return "工作日";
  if (field === "6,0" || field === "0,6") return "周六/周日";
  const single = CRON_DOW_LABELS[field];
  if (single) return `每${single}`;
  return "";
}

/** 五字段 cron 的尽力而为中文描述；无法识别时回退为原始表达式。 */
export function describeCronExpression(expression: string): string {
  const fields = expression.trim().split(/\s+/);
  if (fields.length !== 5) {
    return `Cron ${expression.trim()}`;
  }
  const [minute, hour, dayOfMonth, , dayOfWeek] = fields;

  if (minute === "*" && hour === "*" && dayOfMonth === "*" && dayOfWeek === "*") {
    return "每分钟";
  }
  if (minute.startsWith("*/") && dayOfMonth === "*") {
    const every = minute.slice(2);
    const dowText = cronDowText(dayOfWeek) || "每天";
    const hourRange = /^\d+-\d+$/.test(hour)
      ? ` ${hour.split("-").map((bound) => `${bound.padStart(2, "0")}:00`).join("–")}`
      : "";
    return `${dowText}${hourRange} 每 ${every} 分钟`;
  }

  if (/^\d+$/.test(minute) && hour === "*" && dayOfMonth === "*" && dayOfWeek === "*") {
    return `每小时第 ${Number(minute)} 分钟`;
  }

  const timeText = /^\d+$/.test(minute) && /^\d+$/.test(hour)
    ? `${hour.padStart(2, "0")}:${minute.padStart(2, "0")}`
    : "";
  if (!timeText) {
    return `Cron ${expression.trim()}`;
  }
  if (dayOfMonth !== "*") {
    return `每月 ${Number(dayOfMonth)} 日 ${timeText}`;
  }
  const dowText = cronDowText(dayOfWeek);
  if (dowText) {
    return `${dowText} ${timeText}`;
  }
  return `每天 ${timeText}`;
}

function formatDateTimeInZone(rfc3339: string, timeZone?: string): string {
  const instantMs = Date.parse(rfc3339);
  if (!Number.isFinite(instantMs)) {
    return rfc3339;
  }
  const parts = new Intl.DateTimeFormat("en-US", {
    ...(timeZone ? { timeZone } : {}),
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hourCycle: "h23",
  }).formatToParts(new Date(instantMs));
  const take = (type: Intl.DateTimeFormatPartTypes) =>
    parts.find((part) => part.type === type)?.value ?? "";
  return `${take("year")}-${take("month")}-${take("day")} ${take("hour")}:${take("minute")}`;
}

/** 列表行的计划文案：人类可读的频率 + 时区。 */
export function formatScheduleText(task: ScheduledTask): string {
  const schedule = task.schedule;
  if (schedule.kind === "cron") {
    return `${describeCronExpression(schedule.cron)} · ${schedule.timezone}`;
  }
  const parts = [formatDateTimeInZone(schedule.runAt, schedule.timezone)];
  if (schedule.repeatEveryDays !== undefined) {
    parts.push(`每 ${schedule.repeatEveryDays} 天`);
    const repeatEnd = schedule.repeatEnd;
    if (repeatEnd?.kind === "count" && repeatEnd.count !== undefined) {
      parts.push(`限 ${repeatEnd.count} 次`);
    } else if (repeatEnd?.kind === "until" && repeatEnd.until) {
      parts.push(`截至 ${formatDateTimeInZone(repeatEnd.until, schedule.timezone)}`);
    }
  } else {
    parts.push("不重复");
  }
  parts.push(schedule.timezone);
  return parts.join(" · ");
}

export function formatNextRunText(task: ScheduledTask, timeZone?: string): string | null {
  if (!task.nextRunAt) {
    return null;
  }
  return `下次 ${formatDateTimeInZone(task.nextRunAt, timeZone)}`;
}

export function formatLastRunText(task: ScheduledTask, timeZone?: string): string | null {
  const last = task.lastExecution;
  if (!last) {
    return null;
  }
  const at = last.finishedAt ?? last.startedAt;
  const statusLabel =
    SCHEDULED_TASK_EXECUTION_STATUS_LABELS[last.status as ScheduledTaskExecutionStatus] ??
    last.status;
  if (!at) {
    return `最近 ${statusLabel}`;
  }
  return `最近 ${formatDateTimeInZone(at, timeZone)} ${statusLabel}`;
}

/** 执行记录窗条目（≤50 条倒序由 Core 保证）。 */
export function formatExecutionItem(
  execution: ScheduledTaskExecution,
  timeZone?: string,
): AwakenScheduledTaskExecutionItem {
  const formatOrDash = (value?: string) => (value ? formatDateTimeInZone(value, timeZone) : "—");
  return {
    executionId: execution.executionId,
    triggerLabel: SCHEDULED_TASK_TRIGGER_LABELS[execution.trigger] ?? execution.trigger,
    status: execution.status,
    statusLabel:
      SCHEDULED_TASK_EXECUTION_STATUS_LABELS[execution.status] ?? execution.status,
    scheduledForText: formatOrDash(execution.scheduledFor),
    startedAtText: formatOrDash(execution.startedAt),
    finishedAtText: formatOrDash(execution.finishedAt),
    errorText: execution.errorMessage ?? null,
    threadId: execution.threadId ?? null,
  };
}

// === 提交错误映射 ===
// Main 把 Core 的 domain error code 以 "<CODE>: <message>" 形式嵌入 IPC 错误消息
// （Electron 只保留 message）；没有 code 时按 Core 稳定文案做子串匹配兜底。

export type ScheduledTaskSubmitErrorMapping = {
  errorMessage: string | null;
  fieldErrors: Partial<Record<AwakenScheduledTaskDraftField, string>>;
};

const WIRE_FIELD_TO_DRAFT_FIELD: Record<string, AwakenScheduledTaskDraftField> = {
  name: "name",
  project_id: "projectId",
  "schedule.cron": "cron",
  "schedule.run_at": "runAt",
  "schedule.repeat_every_days": "repeatEveryDays",
  "schedule.repeat_end": "repeatUntil",
  "schedule.repeat_end.until": "repeatUntil",
  "schedule.repeat_end.count": "repeatCount",
  "schedule.timezone": "timezone",
  "action.content": "content",
  "execution.timeout_seconds": "timeoutSeconds",
  "execution.misfire_grace_seconds": "misfireGraceSeconds",
};

const IPC_ERROR_PREFIX = /^Error invoking remote method '[^']*':\s*/;

/** 去掉 Electron IPC 与 error name 前缀，留下干净的领域错误文案。 */
export function scheduledTaskErrorMessage(error: unknown): string {
  let message = error instanceof Error ? error.message : String(error);
  message = message.replace(IPC_ERROR_PREFIX, "");
  message = message.replace(/^RuntimeDomainError:\s*/, "");
  return message;
}

export function isVersionConflictError(error: unknown): boolean {
  const message = scheduledTaskErrorMessage(error);
  return message.includes("VERSION_CONFLICT") || message.includes("task was modified");
}

export function isTaskAlreadyRunningError(error: unknown): boolean {
  const message = scheduledTaskErrorMessage(error);
  return (
    message.includes("TASK_ALREADY_RUNNING") ||
    message.includes("task already has an active execution")
  );
}

export const VERSION_CONFLICT_MESSAGE = "任务已被其他地方修改，请刷新后重试。";
export const TASK_ALREADY_RUNNING_MESSAGE = "该任务已有正在进行的执行，请等待结束后再试。";

/** 创建/编辑提交失败 → 编辑器 fieldErrors（尽力而为）或整体 errorMessage。 */
export function mapScheduledTaskSubmitError(error: unknown): ScheduledTaskSubmitErrorMapping {
  if (isVersionConflictError(error)) {
    return { errorMessage: VERSION_CONFLICT_MESSAGE, fieldErrors: {} };
  }
  const message = scheduledTaskErrorMessage(error);
  const match = /invalid ([a-z_.]+):\s*(.+?)\s*$/.exec(message);
  if (match) {
    const field = WIRE_FIELD_TO_DRAFT_FIELD[match[1]];
    if (field) {
      return { errorMessage: null, fieldErrors: { [field]: match[2] } };
    }
  }
  return { errorMessage: message || "保存失败，请稍后重试。", fieldErrors: {} };
}
