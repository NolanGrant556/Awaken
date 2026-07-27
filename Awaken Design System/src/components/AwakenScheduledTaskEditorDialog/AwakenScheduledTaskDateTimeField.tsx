import { useEffect, useMemo, useState } from "react";
import { CalendarDays, ChevronDown, ChevronLeft, ChevronRight, Clock } from "lucide-react";
import { AwakenPopover } from "../AwakenPopover";

type LocalDateTime = {
  year: number;
  month: number;
  day: number;
  hour: number;
  minute: number;
};

export type AwakenScheduledTaskDateTimeFieldProps = {
  value: string;
  timezone: string;
  ariaLabel: string;
  invalid?: boolean;
  disabled?: boolean;
  onChange: (value: string) => void;
};

const WEEKDAYS = ["日", "一", "二", "三", "四", "五", "六"];

function pad2(value: number): string {
  return value.toString().padStart(2, "0");
}

function parseLocalDateTime(value: string): LocalDateTime | undefined {
  const match = value.match(/^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})$/);
  if (!match) return undefined;
  const parsed = {
    year: Number.parseInt(match[1], 10),
    month: Number.parseInt(match[2], 10),
    day: Number.parseInt(match[3], 10),
    hour: Number.parseInt(match[4], 10),
    minute: Number.parseInt(match[5], 10),
  };
  if (
    parsed.month < 1 || parsed.month > 12 || parsed.day < 1 || parsed.day > 31 ||
    parsed.hour < 0 || parsed.hour > 23 || parsed.minute < 0 || parsed.minute > 59
  ) {
    return undefined;
  }
  return parsed;
}

function nowInTimezone(timezone: string): LocalDateTime {
  try {
    const parts = new Intl.DateTimeFormat("en-US", {
      timeZone: timezone,
      year: "numeric",
      month: "2-digit",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
      hourCycle: "h23",
    }).formatToParts(new Date());
    const take = (type: Intl.DateTimeFormatPartTypes) =>
      Number.parseInt(parts.find((part) => part.type === type)?.value ?? "0", 10);
    return {
      year: take("year"),
      month: take("month"),
      day: take("day"),
      hour: take("hour"),
      minute: take("minute"),
    };
  } catch {
    const now = new Date();
    return {
      year: now.getFullYear(),
      month: now.getMonth() + 1,
      day: now.getDate(),
      hour: now.getHours(),
      minute: now.getMinutes(),
    };
  }
}

function serializeLocalDateTime(value: LocalDateTime): string {
  return `${value.year}-${pad2(value.month)}-${pad2(value.day)}T${pad2(value.hour)}:${pad2(value.minute)}`;
}

function clamp(value: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, value));
}

function displayValue(value: string): string {
  const parsed = parseLocalDateTime(value);
  if (!parsed) return "选择日期和时间";
  return `${parsed.year}年${parsed.month}月${parsed.day}日  ${pad2(parsed.hour)}:${pad2(parsed.minute)}`;
}

export function AwakenScheduledTaskDateTimeField({
  value,
  timezone,
  ariaLabel,
  invalid,
  disabled,
  onChange,
}: AwakenScheduledTaskDateTimeFieldProps) {
  const selected = parseLocalDateTime(value);
  const fallback = selected ?? nowInTimezone(timezone);
  const [open, setOpen] = useState(false);
  const [visibleMonth, setVisibleMonth] = useState(() => ({
    year: fallback.year,
    month: fallback.month,
  }));

  useEffect(() => {
    if (!open) return;
    const current = parseLocalDateTime(value) ?? nowInTimezone(timezone);
    setVisibleMonth({ year: current.year, month: current.month });
  }, [open, timezone, value]);

  const days = useMemo(() => {
    const firstWeekday = new Date(Date.UTC(visibleMonth.year, visibleMonth.month - 1, 1)).getUTCDay();
    return Array.from({ length: 42 }, (_, index) => {
      const date = new Date(Date.UTC(visibleMonth.year, visibleMonth.month - 1, index - firstWeekday + 1));
      return {
        year: date.getUTCFullYear(),
        month: date.getUTCMonth() + 1,
        day: date.getUTCDate(),
        currentMonth: date.getUTCMonth() + 1 === visibleMonth.month,
      };
    });
  }, [visibleMonth]);

  const updateTimePart = (patch: Partial<Pick<LocalDateTime, "hour" | "minute">>) => {
    const current = parseLocalDateTime(value) ?? nowInTimezone(timezone);
    onChange(serializeLocalDateTime({ ...current, ...patch }));
  };

  const moveMonth = (offset: number) => {
    const date = new Date(Date.UTC(visibleMonth.year, visibleMonth.month - 1 + offset, 1));
    setVisibleMonth({ year: date.getUTCFullYear(), month: date.getUTCMonth() + 1 });
  };

  const moveCalendarFocus = (button: HTMLButtonElement, dayOffset: number) => {
    const key = button.dataset.calendarDate;
    if (!key) return;
    const date = new Date(`${key}T00:00:00Z`);
    date.setUTCDate(date.getUTCDate() + dayOffset);
    const nextKey = `${date.getUTCFullYear()}-${pad2(date.getUTCMonth() + 1)}-${pad2(date.getUTCDate())}`;
    setVisibleMonth({ year: date.getUTCFullYear(), month: date.getUTCMonth() + 1 });
    window.setTimeout(() => {
      document.querySelector<HTMLButtonElement>(`[data-calendar-date="${nextKey}"]`)?.focus();
    }, 0);
  };

  return (
    <AwakenPopover
      open={open}
      onOpenChange={(nextOpen) => {
        if (!disabled) setOpen(nextOpen);
      }}
      placement="bottom-start"
      offset={6}
      trigger={
        <button
          type="button"
          className="aw-scheduled-task-editor__picker-trigger"
          aria-label={ariaLabel}
          aria-expanded={open}
          aria-invalid={invalid || undefined}
          disabled={disabled}
        >
          <CalendarDays size={16} strokeWidth={1.8} aria-hidden="true" />
          <span data-placeholder={!selected ? "true" : "false"}>{displayValue(value)}</span>
          <ChevronDown size={15} strokeWidth={1.9} aria-hidden="true" />
        </button>
      }
      content={
        <div
          className="aw-scheduled-task-editor__datetime-popover"
          aria-label={`${ariaLabel}选择器`}
          onKeyDown={(event) => {
            if (event.key === "Escape") event.stopPropagation();
          }}
        >
          <div className="aw-scheduled-task-editor__calendar-head">
            <strong>{visibleMonth.year}年 {visibleMonth.month}月</strong>
            <span>
              <button type="button" aria-label="上个月" onClick={() => moveMonth(-1)}>
                <ChevronLeft size={16} strokeWidth={1.9} />
              </button>
              <button type="button" aria-label="下个月" onClick={() => moveMonth(1)}>
                <ChevronRight size={16} strokeWidth={1.9} />
              </button>
            </span>
          </div>
          <div
            className="aw-scheduled-task-editor__calendar-grid"
            role="grid"
            onKeyDown={(event) => {
              if (!(event.target instanceof HTMLButtonElement)) return;
              const offsets: Record<string, number> = {
                ArrowLeft: -1,
                ArrowRight: 1,
                ArrowUp: -7,
                ArrowDown: 7,
              };
              const dayOffset = offsets[event.key];
              if (dayOffset === undefined) return;
              event.preventDefault();
              moveCalendarFocus(event.target, dayOffset);
            }}
          >
            {WEEKDAYS.map((weekday) => (
              <span key={weekday} className="aw-scheduled-task-editor__calendar-weekday">{weekday}</span>
            ))}
            {days.map((day) => {
              const isSelected = selected?.year === day.year && selected.month === day.month && selected.day === day.day;
              return (
                <button
                  type="button"
                  key={`${day.year}-${day.month}-${day.day}`}
                  data-calendar-date={`${day.year}-${pad2(day.month)}-${pad2(day.day)}`}
                  data-outside={!day.currentMonth ? "true" : "false"}
                  data-selected={isSelected ? "true" : "false"}
                  aria-label={`${day.year}年${day.month}月${day.day}日`}
                  aria-pressed={isSelected}
                  onClick={() => {
                    const time = selected ?? nowInTimezone(timezone);
                    onChange(serializeLocalDateTime({ ...time, year: day.year, month: day.month, day: day.day }));
                    setVisibleMonth({ year: day.year, month: day.month });
                  }}
                >
                  {day.day}
                </button>
              );
            })}
          </div>
          <div className="aw-scheduled-task-editor__time-row">
            <span><Clock size={15} strokeWidth={1.8} />时间</span>
            <label>
              <span className="aw-visually-hidden">小时</span>
              <input
                type="number"
                min={0}
                max={23}
                value={pad2(selected?.hour ?? fallback.hour)}
                onChange={(event) => updateTimePart({ hour: clamp(Number.parseInt(event.target.value || "0", 10), 0, 23) })}
              />
            </label>
            <b>:</b>
            <label>
              <span className="aw-visually-hidden">分钟</span>
              <input
                type="number"
                min={0}
                max={59}
                value={pad2(selected?.minute ?? fallback.minute)}
                onChange={(event) => updateTimePart({ minute: clamp(Number.parseInt(event.target.value || "0", 10), 0, 59) })}
              />
            </label>
          </div>
          <div className="aw-scheduled-task-editor__picker-actions">
            <button type="button" onClick={() => onChange("")}>清除</button>
            <span>
              <button
                type="button"
                onClick={() => {
                  const now = nowInTimezone(timezone);
                  const time = selected ?? now;
                  onChange(serializeLocalDateTime({ ...time, year: now.year, month: now.month, day: now.day }));
                  setVisibleMonth({ year: now.year, month: now.month });
                }}
              >
                今天
              </button>
              <button type="button" data-primary="true" onClick={() => setOpen(false)}>完成</button>
            </span>
          </div>
        </div>
      }
    />
  );
}
