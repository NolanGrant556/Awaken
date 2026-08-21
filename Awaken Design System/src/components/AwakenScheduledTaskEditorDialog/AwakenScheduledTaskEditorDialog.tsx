import "./AwakenScheduledTaskEditorDialog.css";
import {
  useId,
  type ChangeEvent,
  type KeyboardEvent as ReactKeyboardEvent,
  type ReactNode,
} from "react";
import { CalendarClock, Check, ChevronDown, Clock } from "../../icons";
import { AwakenButton } from "../AwakenButton";
import {
  AwakenComposerBar,
  type ComposerBarSlashPalette,
  type ComposerBarStructuredInputValue,
  type ComposerBarToken,
} from "../AwakenComposerBar";
import type { ComposerInlineInputElement } from "../AwakenComposerBar/AwakenComposerInlineInput";
import { AwakenDialog } from "../AwakenDialog";
import { AwakenSelect } from "../AwakenSelect";
import { AwakenSwitch } from "../AwakenSwitch";
import { AwakenScheduledTaskDateTimeField } from "./AwakenScheduledTaskDateTimeField";
import { AwakenScheduledTaskProjectPicker } from "./AwakenScheduledTaskProjectPicker";
import { AwakenScheduledTaskTimezonePicker } from "./AwakenScheduledTaskTimezonePicker";

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
  systemTimezone?: string;
  busy?: boolean;
  errorMessage?: string | null;
  fieldErrors?: Partial<Record<AwakenScheduledTaskDraftField, string>>;
  composerTokens?: ComposerBarToken[];
  slashPalette?: ComposerBarSlashPalette;
  onOpenChange: (open: boolean) => void;
  onValueChange: (draft: AwakenScheduledTaskDraft) => void;
  onComposerStructuredInputChange?: (value: ComposerBarStructuredInputValue) => void;
  onComposerInputKeyDown?: (
    event: ReactKeyboardEvent<HTMLTextAreaElement | ComposerInlineInputElement>,
  ) => void;
  onRemoveComposerToken?: (tokenId: string) => void;
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

const AWAKEN_SCHEDULED_TASK_CRON_PRESET_OPTIONS = [
  { value: "hourly", label: "每小时" },
  { value: "daily", label: "每天" },
  { value: "weekly", label: "每周" },
  { value: "custom", label: "自定义" },
] as const;

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
  systemTimezone,
  busy = false,
  errorMessage,
  fieldErrors,
  composerTokens = [],
  slashPalette,
  onOpenChange,
  onValueChange,
  onComposerStructuredInputChange,
  onComposerInputKeyDown,
  onRemoveComposerToken,
  onSubmit,
}: AwakenScheduledTaskEditorDialogProps) {
  const radioGroupName = useId();
  const profileOptions = [
    { value: "", label: "每次运行时使用当前默认 profile" },
    ...profiles.map((profile) => ({ value: profile.profileId, label: profile.name })),
  ];
  const update = (patch: Partial<AwakenScheduledTaskDraft>) => onValueChange({ ...value, ...patch });
  const updateNumber = (key: "repeatEveryDays" | "repeatCount" | "timeoutSeconds" | "misfireGraceSeconds") =>
    (event: ChangeEvent<HTMLInputElement>) => {
      const parsed = Number.parseInt(event.target.value, 10);
      update({ [key]: Number.isFinite(parsed) ? parsed : 0 });
    };
  return (
    <AwakenDialog
      open={open}
      className="aw-scheduled-task-editor-dialog"
      title={
        <span className="aw-scheduled-task-editor__dialog-title">
          <span className="aw-scheduled-task-editor__dialog-icon" aria-hidden="true">
            <CalendarClock size={18} strokeWidth={1.8} />
          </span>
          <span>
            <strong>{mode === "create" ? "创建定时任务" : "编辑定时任务"}</strong>
            <small>设定运行位置、执行计划与任务内容</small>
          </span>
        </span>
      }
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

        <section className="aw-scheduled-task-editor__section aw-scheduled-task-editor__section--intro" aria-label="基本信息">
          <div className="aw-scheduled-task-editor__section-heading">
            <span>01</span>
            <div>
              <h3 className="aw-scheduled-task-editor__section-title">基本信息</h3>
              <p>为任务命名，并选择是否立即启用。</p>
            </div>
          </div>
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
          <div className="aw-scheduled-task-editor__switch-row">
            <span>
              <strong>保存后启用</strong>
              <small>关闭后会保存为暂停状态</small>
            </span>
            <AwakenSwitch
              checked={value.enabled}
              label="保存后启用"
              busy={busy}
              onCheckedChange={(enabled) => update({ enabled })}
            />
          </div>
        </section>

        <section className="aw-scheduled-task-editor__section" aria-label="运行位置">
          <div className="aw-scheduled-task-editor__section-heading">
            <span>02</span>
            <div>
              <h3 className="aw-scheduled-task-editor__section-title">运行位置</h3>
              <p>任务只会继承所选项目的默认工作目录。</p>
            </div>
          </div>
          <EditorField label="项目" error={fieldErrors?.projectId}>
            {projects.length > 0 ? (
              <AwakenScheduledTaskProjectPicker
                value={value.projectId}
                projects={projects}
                invalid={Boolean(fieldErrors?.projectId)}
                disabled={busy}
                onChange={(projectId) => update({ projectId })}
              />
            ) : (
              <span className="aw-scheduled-task-editor__hint">暂无可用项目，请先在项目中创建。</span>
            )}
          </EditorField>
        </section>

        <section className="aw-scheduled-task-editor__section" aria-label="调度">
          <div className="aw-scheduled-task-editor__section-heading">
            <span>03</span>
            <div>
              <h3 className="aw-scheduled-task-editor__section-title">执行计划</h3>
              <p>选择循环规则，或指定一次日程时间。</p>
            </div>
          </div>
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
                <AwakenSelect
                  value={value.cronPreset}
                  options={AWAKEN_SCHEDULED_TASK_CRON_PRESET_OPTIONS}
                  ariaLabel="循环计划"
                  preferredPlacement="top"
                  disabled={busy}
                  onValueChange={(nextValue) => {
                    const preset = nextValue as AwakenScheduledTaskCronPreset;
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
                />
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
                <AwakenScheduledTaskDateTimeField
                  ariaLabel="开始时间"
                  value={value.runAtLocal}
                  timezone={value.timezone}
                  invalid={Boolean(fieldErrors?.runAt)}
                  disabled={busy}
                  onChange={(runAtLocal) => update({ runAtLocal })}
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
                      <AwakenScheduledTaskDateTimeField
                        ariaLabel="重复截止时间"
                        value={value.repeatUntilLocal}
                        timezone={value.timezone}
                        invalid={Boolean(fieldErrors?.repeatUntil)}
                        disabled={busy}
                        onChange={(repeatUntilLocal) => update({ repeatUntilLocal })}
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
            <AwakenScheduledTaskTimezonePicker
              value={value.timezone}
              timezones={timezones}
              systemTimezone={systemTimezone}
              referenceLocal={value.scheduleKind === "calendar" ? value.runAtLocal : undefined}
              invalid={Boolean(fieldErrors?.timezone)}
              disabled={busy}
              onChange={(timezone) => update({ timezone })}
            />
          </EditorField>
        </section>

        <section className="aw-scheduled-task-editor__section" aria-label="任务内容">
          <div className="aw-scheduled-task-editor__section-heading">
            <span>04</span>
            <div>
              <h3 className="aw-scheduled-task-editor__section-title">任务内容</h3>
              <p>发送一条提醒，或让 Agent 在项目中完成工作。</p>
            </div>
          </div>
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
            {value.actionKind === "agent" ? (
              <div
                className="aw-scheduled-task-editor__agent-composer"
                aria-invalid={fieldErrors?.content ? true : undefined}
              >
                <AwakenComposerBar
                  variant="embedded"
                  value={value.content}
                  tokens={composerTokens}
                  slashPalette={slashPalette}
                  inputAriaLabel="Agent prompt"
                  placeholder="输入任务，键入 / 选择 Skill"
                  disabled={busy}
                  submitOnEnter={false}
                  onChange={(content) => update({ content })}
                  onStructuredInputChange={onComposerStructuredInputChange}
                  onInputKeyDown={onComposerInputKeyDown}
                  onRemoveToken={onRemoveComposerToken}
                />
              </div>
            ) : (
              <textarea
                className="aw-scheduled-task-editor__control aw-scheduled-task-editor__textarea"
                aria-label="提醒正文"
                aria-invalid={fieldErrors?.content ? true : undefined}
                rows={6}
                value={value.content}
                disabled={busy}
                onChange={(event) => update({ content: event.target.value })}
              />
            )}
          </EditorField>
        </section>

        <details className="aw-scheduled-task-editor__advanced">
          <summary>
            <span>
              <strong>模型与高级选项</strong>
              <small>默认会使用运行时的当前模型配置</small>
            </span>
            <ChevronDown size={16} strokeWidth={1.9} aria-hidden="true" />
          </summary>
          <div className="aw-scheduled-task-editor__advanced-body">
            <section aria-label="模型">
              <EditorField label="模型 Profile">
                <AwakenSelect
                  className="aw-scheduled-task-editor__profile-select"
                  value={value.profileId}
                  options={profileOptions}
                  ariaLabel="模型 profile"
                  disabled={busy}
                  onValueChange={(profileId) => update({ profileId })}
                />
              </EditorField>
            </section>
            <section aria-label="高级选项" className="aw-scheduled-task-editor__advanced-grid">
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
              <EditorField label="错过执行宽限秒数" error={fieldErrors?.misfireGraceSeconds}>
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
        </details>
      </div>
    </AwakenDialog>
  );
}
