import "./AwakenScheduledTaskEditorDialog.css";
import { useId, type ChangeEvent, type ReactNode } from "react";
import { Check, Clock } from "lucide-react";
import { AwakenButton } from "../AwakenButton";
import { AwakenDialog } from "../AwakenDialog";
import { AwakenSwitch } from "../AwakenSwitch";

export type AwakenScheduledTaskScheduleKind = "cron" | "calendar";
export type AwakenScheduledTaskActionKind = "reminder" | "agent";
export type AwakenScheduledTaskCronPreset = "hourly" | "daily" | "weekly" | "custom";
export type AwakenScheduledTaskRepeatEndKind = "never" | "until" | "count";

// 模板来源只记录“由哪个内置模板预填”，由 Renderer 在保存时转译为 Core 的 template_origin；
// Design System 不构造 task_id/version/expected_version 等 Core 概念。
export type AwakenScheduledTaskTemplateOrigin = {
  templateId: string;
  source: "builtin";
};

// 编辑器 draft（camelCase）。日程时间使用 datetime-local 本地字符串（"YYYY-MM-DDTHH:mm"），
// 一律按 draft.timezone（IANA）解释；换算为带 offset 的 RFC3339 是 Renderer 保存时的职责。
export type AwakenScheduledTaskDraft = {
  name: string;
  enabled: boolean;
  projectId: string;
  scheduleKind: AwakenScheduledTaskScheduleKind;
  cronPreset: AwakenScheduledTaskCronPreset;
  cron: string;
  cronTime: string; // "HH:mm"，用于 daily/weekly 预设
  cronWeekdays: number[]; // 0=周日..6=周六，用于 weekly 预设
  runAtLocal: string;
  repeat: boolean;
  repeatEveryDays: number;
  repeatEndKind: AwakenScheduledTaskRepeatEndKind;
  repeatUntilLocal: string;
  repeatCount: number;
  timezone: string;
  actionKind: AwakenScheduledTaskActionKind;
  content: string;
  profileId: string;
  timeoutSeconds: number;
  misfireGraceSeconds: number;
  templateOrigin?: AwakenScheduledTaskTemplateOrigin | null;
};

export type AwakenScheduledTaskProjectOption = {
  projectId: string;
  name: string;
};

export type AwakenScheduledTaskProfileOption = {
  profileId: string;
  name: string;
};

// fieldErrors 的可用键；值为就地展示的校验提示，由调用方（最终由 Core 校验结果）给出。
export type AwakenScheduledTaskDraftField =
  | "name"
  | "projectId"
  | "cron"
  | "cronTime"
  | "cronWeekdays"
  | "runAt"
  | "repeatEveryDays"
  | "repeatUntil"
  | "repeatCount"
  | "timezone"
  | "content"
  | "timeoutSeconds"
  | "misfireGraceSeconds";

export type AwakenScheduledTaskEditorDialogProps = {
  open: boolean;
  mode: "create" | "edit";
  value: AwakenScheduledTaskDraft;
  projects: AwakenScheduledTaskProjectOption[];
  profiles: AwakenScheduledTaskProfileOption[];
  timezones: string[];
  busy?: boolean;
  errorMessage?: string | null;
  fieldErrors?: Partial<Record<AwakenScheduledTaskDraftField, string>>;
  onOpenChange: (open: boolean) => void;
  onValueChange: (draft: AwakenScheduledTaskDraft) => void;
  onSubmit: (draft: AwakenScheduledTaskDraft) => void;
};

// 预设只覆盖契约 §4.2 的每小时/每天/每周；表达式本身只是预填值，用户可以切到自定义再改。
export const AWAKEN_SCHEDULED_TASK_CRON_PRESET_EXPRESSIONS: Record<
  Exclude<AwakenScheduledTaskCronPreset, "custom">,
  string
> = {
  hourly: "0 * * * *",
  daily: "0 9 * * *",
  weekly: "0 9 * * 1",
};

export function createEmptyAwakenScheduledTaskDraft(timezone: string): AwakenScheduledTaskDraft {
  return {
    name: "",
    enabled: true,
    projectId: "",
    scheduleKind: "cron",
    cronPreset: "daily",
    cron: AWAKEN_SCHEDULED_TASK_CRON_PRESET_EXPRESSIONS.daily,
    cronTime: "09:00",
    cronWeekdays: [1],
    runAtLocal: "",
    repeat: false,
    repeatEveryDays: 1,
    repeatEndKind: "never",
    repeatUntilLocal: "",
    repeatCount: 1,
    timezone,
    actionKind: "reminder",
    content: "",
    profileId: "",
    timeoutSeconds: 600,
    misfireGraceSeconds: 600,
    templateOrigin: null,
  };
}

const CRON_WEEKDAY_LABELS = ["周日", "周一", "周二", "周三", "周四", "周五", "周六"];

function parseTimeHHMM(value: string): { hour: number; minute: number } | undefined {
  const match = value.match(/^(\d{1,2}):(\d{2})$/);
  if (!match) return undefined;
  const hour = Number.parseInt(match[1], 10);
  const minute = Number.parseInt(match[2], 10);
  if (hour < 0 || hour > 23 || minute < 0 || minute > 59) return undefined;
  return { hour, minute };
}

function pad2(value: number): string {
  return value.toString().padStart(2, "0");
}

/** 由 daily/weekly/hourly 预设和时间/星期生成 Cron 表达式。custom _preset 不调用。 */
export function buildCronExpressionFromPreset(
  preset: Exclude<AwakenScheduledTaskCronPreset, "custom">,
  cronTime: string,
  cronWeekdays: number[],
): string {
  const time = parseTimeHHMM(cronTime) ?? { hour: 9, minute: 0 };
  if (preset === "hourly") {
    return `${time.minute} * * * *`;
  }
  if (preset === "daily") {
    return `${time.minute} ${time.hour} * * *`;
  }
  const uniqueDays = [...new Set(cronWeekdays.filter((d) => d >= 0 && d <= 6))].sort((a, b) => a - b);
  const dow = uniqueDays.length > 0 ? uniqueDays.join(",") : "1";
  return `${time.minute} ${time.hour} * * ${dow}`;
}

function defaultWeekdaysForPreset(preset: AwakenScheduledTaskCronPreset): number[] {
  if (preset === "weekly") return [1];
  return [];
}

function rebuildCronExpression(
  preset: AwakenScheduledTaskCronPreset,
  cronTime: string,
  cronWeekdays: number[],
  currentCron: string,
): string {
  if (preset === "custom") return currentCron;
  return buildCronExpressionFromPreset(preset, cronTime, cronWeekdays);
}

function EditorField({
  label,
  error,
  children,
}: {
  label: string;
  error?: string;
  children: ReactNode;
}) {
  return (
    <div className="aw-scheduled-task-editor__field">
      <span className="aw-scheduled-task-editor__field-label">{label}</span>
      {children}
      {error ? (
        <span className="aw-scheduled-task-editor__field-error" role="alert">{error}</span>
      ) : null}
    </div>
  );
}

export function AwakenScheduledTaskEditorDialog({
  open,
  mode,
  value,
  projects,
  profiles,
  timezones,
  busy = false,
  errorMessage,
  fieldErrors,
  onOpenChange,
  onValueChange,
  onSubmit,
}: AwakenScheduledTaskEditorDialogProps) {
  const radioGroupName = useId();
  const update = (patch: Partial<AwakenScheduledTaskDraft>) => onValueChange({ ...value, ...patch });
  const updateNumber = (key: "repeatEveryDays" | "repeatCount" | "timeoutSeconds" | "misfireGraceSeconds") =>
    (event: ChangeEvent<HTMLInputElement>) => {
      const parsed = Number.parseInt(event.target.value, 10);
      update({ [key]: Number.isFinite(parsed) ? parsed : 0 });
    };
  // 编辑历史任务时 draft.timezone 可能不在当前候选列表里，保留原值避免静默改时区。
  const timezoneOptions = value.timezone && !timezones.includes(value.timezone)
    ? [value.timezone, ...timezones]
    : timezones;

  return (
    <AwakenDialog
      open={open}
      className="aw-scheduled-task-editor-dialog"
      title={mode === "create" ? "创建定时任务" : "编辑定时任务"}
      onOpenChange={onOpenChange}
      footer={
        <>
          <AwakenButton variant="default" size="md" disabled={busy} onClick={() => onOpenChange(false)}>取消</AwakenButton>
          <AwakenButton variant="primary" size="md" loading={busy} onClick={() => onSubmit(value)}>
            <Check size={15} strokeWidth={1.9} />保存任务
          </AwakenButton>
        </>
      }
    >
      <div className="aw-scheduled-task-editor__body">
        {errorMessage ? (
          <div className="aw-scheduled-task-editor__error" role="alert">{errorMessage}</div>
        ) : null}

        <section className="aw-scheduled-task-editor__section" aria-label="基本信息">
          <h3 className="aw-scheduled-task-editor__section-title">基本信息</h3>
          <EditorField label="名称" error={fieldErrors?.name}>
            <input
              type="text"
              className="aw-scheduled-task-editor__control"
              aria-label="任务名称"
              aria-invalid={fieldErrors?.name ? true : undefined}
              value={value.name}
              disabled={busy}
              onChange={(event) => update({ name: event.target.value })}
            />
          </EditorField>
          <EditorField label="启用">
            <AwakenSwitch
              checked={value.enabled}
              label="定时任务"
              busy={busy}
              onCheckedChange={(enabled) => update({ enabled })}
            />
          </EditorField>
        </section>

        <section className="aw-scheduled-task-editor__section" aria-label="运行位置">
          <h3 className="aw-scheduled-task-editor__section-title">运行位置</h3>
          <EditorField label="项目" error={fieldErrors?.projectId}>
            {projects.length > 0 ? (
              <div className="aw-scheduled-task-editor__radio-list" role="radiogroup" aria-label="运行项目">
                {projects.map((project) => (
                  <label className="aw-scheduled-task-editor__radio" key={project.projectId}>
                    <input
                      type="radio"
                      name={`${radioGroupName}-project`}
                      checked={value.projectId === project.projectId}
                      disabled={busy}
                      onChange={() => update({ projectId: project.projectId })}
                    />
                    <span>{project.name}</span>
                  </label>
                ))}
              </div>
            ) : (
              <span className="aw-scheduled-task-editor__hint">暂无可用项目，请先在项目中创建。</span>
            )}
          </EditorField>
        </section>

        <section className="aw-scheduled-task-editor__section" aria-label="调度">
          <h3 className="aw-scheduled-task-editor__section-title">调度</h3>
          <div className="aw-scheduled-task-editor__tabs" role="tablist" aria-label="调度方式">
            <button
              type="button"
              role="tab"
              aria-selected={value.scheduleKind === "cron"}
              data-active={value.scheduleKind === "cron" ? "true" : "false"}
              disabled={busy}
              onClick={() => update({ scheduleKind: "cron" })}
            >
              循环任务
            </button>
            <button
              type="button"
              role="tab"
              aria-selected={value.scheduleKind === "calendar"}
              data-active={value.scheduleKind === "calendar" ? "true" : "false"}
              disabled={busy}
              onClick={() => update({ scheduleKind: "calendar" })}
            >
              日程任务
            </button>
          </div>
          {value.scheduleKind === "cron" ? (
            <>
              <EditorField label="循环计划">
                <select
                  className="aw-scheduled-task-editor__control"
                  aria-label="循环计划"
                  value={value.cronPreset}
                  disabled={busy}
                  onChange={(event) => {
                    const preset = event.target.value as AwakenScheduledTaskCronPreset;
                    if (preset === "custom") {
                      update({ cronPreset: preset });
                      return;
                    }
                    const nextTime = value.cronTime || "09:00";
                    const nextWeekdays = preset === "weekly" && value.cronWeekdays.length > 0
                      ? value.cronWeekdays
                      : defaultWeekdaysForPreset(preset);
                    update({
                      cronPreset: preset,
                      cronTime: nextTime,
                      cronWeekdays: nextWeekdays,
                      cron: rebuildCronExpression(preset, nextTime, nextWeekdays, value.cron),
                    });
                  }}
                >
                  <option value="hourly">每小时</option>
                  <option value="daily">每天</option>
                  <option value="weekly">每周</option>
                  <option value="custom">自定义</option>
                </select>
              </EditorField>
              {value.cronPreset !== "custom" ? (
                <>
                  {value.cronPreset === "daily" || value.cronPreset === "weekly" ? (
                    <EditorField label="执行时间" error={fieldErrors?.cronTime ?? fieldErrors?.cron}>
                      <span className="aw-scheduled-task-editor__time-wrapper">
                        <input
                          type="time"
                          className="aw-scheduled-task-editor__control aw-scheduled-task-editor__control--time"
                          aria-label="执行时间"
                          aria-invalid={fieldErrors?.cronTime || fieldErrors?.cron ? true : undefined}
                          value={value.cronTime}
                          disabled={busy}
                          onChange={(event) => {
                            const cronTime = event.target.value;
                            update({
                              cronTime,
                              cron: rebuildCronExpression(
                                value.cronPreset,
                                cronTime,
                                value.cronWeekdays,
                                value.cron,
                              ),
                            });
                          }}
                        />
                        <span className="aw-scheduled-task-editor__time-icon" aria-hidden="true">
                          <Clock size={16} strokeWidth={1.9} />
                        </span>
                      </span>
                    </EditorField>
                  ) : null}
                  {value.cronPreset === "weekly" ? (
                    <EditorField label="星期" error={fieldErrors?.cronWeekdays ?? fieldErrors?.cron}>
                      <div
                        className="aw-scheduled-task-editor__weekdays"
                        role="group"
                        aria-label="星期"
                      >
                        {CRON_WEEKDAY_LABELS.map((label, index) => (
                          <label key={index} className="aw-scheduled-task-editor__weekday">
                            <input
                              type="checkbox"
                              aria-label={label}
                              checked={value.cronWeekdays.includes(index)}
                              disabled={busy}
                              onChange={(event) => {
                                const next = event.target.checked
                                  ? [...value.cronWeekdays, index]
                                  : value.cronWeekdays.filter((d) => d !== index);
                                update({
                                  cronWeekdays: next,
                                  cron: rebuildCronExpression(
                                    value.cronPreset,
                                    value.cronTime,
                                    next,
                                    value.cron,
                                  ),
                                });
                              }}
                            />
                            <span>{label}</span>
                          </label>
                        ))}
                      </div>
                    </EditorField>
                  ) : null}
                </>
              ) : (
                <EditorField label="Cron 表达式（分 时 日 月 周）" error={fieldErrors?.cron}>
                  <input
                    type="text"
                    className="aw-scheduled-task-editor__control aw-scheduled-task-editor__control--mono"
                    aria-label="自定义 Cron 表达式"
                    aria-invalid={fieldErrors?.cron ? true : undefined}
                    placeholder="*/25 9-17 * * 1-5"
                    spellCheck={false}
                    value={value.cron}
                    disabled={busy}
                    onChange={(event) => update({ cron: event.target.value })}
                  />
                </EditorField>
              )}
            </>
          ) : (
            <>
              <EditorField label="开始时间" error={fieldErrors?.runAt}>
                <input
                  type="datetime-local"
                  className="aw-scheduled-task-editor__control"
                  aria-label="开始时间"
                  aria-invalid={fieldErrors?.runAt ? true : undefined}
                  value={value.runAtLocal}
                  disabled={busy}
                  onChange={(event) => update({ runAtLocal: event.target.value })}
                />
              </EditorField>
              <EditorField label="重复">
                <AwakenSwitch
                  checked={value.repeat}
                  label="重复"
                  busy={busy}
                  onCheckedChange={(repeat) => update({ repeat })}
                />
              </EditorField>
              {value.repeat ? (
                <>
                  <EditorField label="每 N 天" error={fieldErrors?.repeatEveryDays}>
                    <input
                      type="number"
                      className="aw-scheduled-task-editor__control"
                      aria-label="每 N 天重复"
                      aria-invalid={fieldErrors?.repeatEveryDays ? true : undefined}
                      min={1}
                      step={1}
                      value={value.repeatEveryDays}
                      disabled={busy}
                      onChange={updateNumber("repeatEveryDays")}
                    />
                  </EditorField>
                  <EditorField label="结束方式">
                    <div className="aw-scheduled-task-editor__radio-list" role="radiogroup" aria-label="结束方式">
                      <label className="aw-scheduled-task-editor__radio">
                        <input
                          type="radio"
                          name={`${radioGroupName}-repeat-end`}
                          checked={value.repeatEndKind === "never"}
                          disabled={busy}
                          onChange={() => update({ repeatEndKind: "never" })}
                        />
                        <span>永不结束</span>
                      </label>
                      <label className="aw-scheduled-task-editor__radio">
                        <input
                          type="radio"
                          name={`${radioGroupName}-repeat-end`}
                          checked={value.repeatEndKind === "until"}
                          disabled={busy}
                          onChange={() => update({ repeatEndKind: "until" })}
                        />
                        <span>截止时间</span>
                      </label>
                      <label className="aw-scheduled-task-editor__radio">
                        <input
                          type="radio"
                          name={`${radioGroupName}-repeat-end`}
                          checked={value.repeatEndKind === "count"}
                          disabled={busy}
                          onChange={() => update({ repeatEndKind: "count" })}
                        />
                        <span>限定次数</span>
                      </label>
                    </div>
                  </EditorField>
                  {value.repeatEndKind === "until" ? (
                    <EditorField label="截止时间" error={fieldErrors?.repeatUntil}>
                      <input
                        type="datetime-local"
                        className="aw-scheduled-task-editor__control"
                        aria-label="重复截止时间"
                        aria-invalid={fieldErrors?.repeatUntil ? true : undefined}
                        value={value.repeatUntilLocal}
                        disabled={busy}
                        onChange={(event) => update({ repeatUntilLocal: event.target.value })}
                      />
                    </EditorField>
                  ) : null}
                  {value.repeatEndKind === "count" ? (
                    <EditorField label="重复次数" error={fieldErrors?.repeatCount}>
                      <input
                        type="number"
                        className="aw-scheduled-task-editor__control"
                        aria-label="重复次数"
                        aria-invalid={fieldErrors?.repeatCount ? true : undefined}
                        min={1}
                        max={10000}
                        step={1}
                        value={value.repeatCount}
                        disabled={busy}
                        onChange={updateNumber("repeatCount")}
                      />
                    </EditorField>
                  ) : null}
                </>
              ) : null}
            </>
          )}
          <EditorField label="时区" error={fieldErrors?.timezone}>
            <select
              className="aw-scheduled-task-editor__control"
              aria-label="时区"
              aria-invalid={fieldErrors?.timezone ? true : undefined}
              value={value.timezone}
              disabled={busy}
              onChange={(event) => update({ timezone: event.target.value })}
            >
              {timezoneOptions.map((timezone) => (
                <option key={timezone} value={timezone}>{timezone}</option>
              ))}
            </select>
          </EditorField>
        </section>

        <section className="aw-scheduled-task-editor__section" aria-label="任务内容">
          <h3 className="aw-scheduled-task-editor__section-title">任务内容</h3>
          <div className="aw-scheduled-task-editor__tabs" role="tablist" aria-label="任务类型">
            <button
              type="button"
              role="tab"
              aria-selected={value.actionKind === "reminder"}
              data-active={value.actionKind === "reminder" ? "true" : "false"}
              disabled={busy}
              onClick={() => update({ actionKind: "reminder" })}
            >
              提醒
            </button>
            <button
              type="button"
              role="tab"
              aria-selected={value.actionKind === "agent"}
              data-active={value.actionKind === "agent" ? "true" : "false"}
              disabled={busy}
              onClick={() => update({ actionKind: "agent" })}
            >
              Agent
            </button>
          </div>
          <EditorField
            label={value.actionKind === "reminder" ? "提醒正文" : "Agent prompt"}
            error={fieldErrors?.content}
          >
            <textarea
              className="aw-scheduled-task-editor__control aw-scheduled-task-editor__textarea"
              aria-label={value.actionKind === "reminder" ? "提醒正文" : "Agent prompt"}
              aria-invalid={fieldErrors?.content ? true : undefined}
              rows={6}
              value={value.content}
              disabled={busy}
              onChange={(event) => update({ content: event.target.value })}
            />
          </EditorField>
        </section>

        <section className="aw-scheduled-task-editor__section" aria-label="模型">
          <h3 className="aw-scheduled-task-editor__section-title">模型</h3>
          <EditorField label="Profile">
            <select
              className="aw-scheduled-task-editor__control"
              aria-label="模型 profile"
              value={value.profileId}
              disabled={busy}
              onChange={(event) => update({ profileId: event.target.value })}
            >
              <option value="">每次运行时使用当前默认 profile</option>
              {profiles.map((profile) => (
                <option key={profile.profileId} value={profile.profileId}>{profile.name}</option>
              ))}
            </select>
          </EditorField>
        </section>

        <section className="aw-scheduled-task-editor__section" aria-label="高级选项">
          <h3 className="aw-scheduled-task-editor__section-title">高级选项</h3>
          <EditorField label="超时秒数" error={fieldErrors?.timeoutSeconds}>
            <input
              type="number"
              className="aw-scheduled-task-editor__control"
              aria-label="超时秒数"
              aria-invalid={fieldErrors?.timeoutSeconds ? true : undefined}
              min={30}
              max={3600}
              step={1}
              value={value.timeoutSeconds}
              disabled={busy}
              onChange={updateNumber("timeoutSeconds")}
            />
          </EditorField>
          <EditorField label="misfire 宽限秒数" error={fieldErrors?.misfireGraceSeconds}>
            <input
              type="number"
              className="aw-scheduled-task-editor__control"
              aria-label="misfire 宽限秒数"
              aria-invalid={fieldErrors?.misfireGraceSeconds ? true : undefined}
              min={0}
              max={86400}
              step={1}
              value={value.misfireGraceSeconds}
              disabled={busy}
              onChange={updateNumber("misfireGraceSeconds")}
            />
          </EditorField>
        </section>
      </div>
    </AwakenDialog>
  );
}
