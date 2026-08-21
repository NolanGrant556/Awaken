import { useMemo, useRef, useState } from "react";
import { AwakenIcon } from "../AwakenIcon";
import { AwakenPopover } from "../AwakenPopover";
import { presentAwakenTimezone } from "./timezone";

export type AwakenScheduledTaskTimezonePickerProps = {
  value: string;
  timezones: string[];
  systemTimezone?: string;
  referenceLocal?: string;
  invalid?: boolean;
  disabled?: boolean;
  onChange: (timezone: string) => void;
};

export function AwakenScheduledTaskTimezonePicker({
  value,
  timezones,
  systemTimezone,
  referenceLocal,
  invalid,
  disabled,
  onChange,
}: AwakenScheduledTaskTimezonePickerProps) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const searchRef = useRef<HTMLInputElement>(null);
  const referenceDate = useMemo(() => {
    const parsed = referenceLocal ? new Date(`${referenceLocal}:00Z`) : new Date();
    return Number.isNaN(parsed.getTime()) ? new Date() : parsed;
  }, [open, referenceLocal]);
  const options = useMemo(() => {
    const unique = [...new Set([value, systemTimezone, ...timezones].filter(Boolean) as string[])];
    return unique.map((timezone) => presentAwakenTimezone(timezone, referenceDate));
  }, [referenceDate, systemTimezone, timezones, value]);
  const selected = presentAwakenTimezone(value || systemTimezone || "UTC", referenceDate);
  const normalizedQuery = query.trim().toLocaleLowerCase("zh-CN");
  const filtered = normalizedQuery
    ? options.filter((option) => option.searchText.includes(normalizedQuery))
    : options;
  const groups = [...new Set(filtered.map((option) => option.group))];

  return (
    <AwakenPopover
      open={open}
      onOpenChange={(nextOpen) => {
        if (disabled) return;
        setOpen(nextOpen);
        if (nextOpen) {
          setQuery("");
          window.setTimeout(() => searchRef.current?.focus(), 0);
        }
      }}
      placement="bottom-start"
      offset={6}
      trigger={
        <button
          type="button"
          className="aw-scheduled-task-editor__picker-trigger aw-scheduled-task-editor__timezone-trigger"
          aria-label="时区"
          aria-expanded={open}
          aria-invalid={invalid || undefined}
          disabled={disabled}
        >
          <AwakenIcon concept="browser" size={16} />
          <span>
            <strong>{selected.name}</strong>
            <small>{selected.offsetLabel}</small>
          </span>
          <AwakenIcon concept="dropdown" size={16} />
        </button>
      }
      content={
        <div
          className="aw-scheduled-task-editor__timezone-popover"
          onKeyDown={(event) => {
            if (event.key === "Escape") event.stopPropagation();
          }}
        >
          <label className="aw-scheduled-task-editor__timezone-search">
            <AwakenIcon concept="search" size={16} />
            <input
              ref={searchRef}
              type="search"
              aria-label="搜索时区"
              placeholder="搜索城市或时区"
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === "ArrowDown") {
                  event.preventDefault();
                  document.querySelector<HTMLButtonElement>(
                    ".aw-scheduled-task-editor__timezone-list button",
                  )?.focus();
                }
              }}
            />
          </label>
          <div
            className="aw-scheduled-task-editor__timezone-list"
            role="listbox"
            aria-label="时区列表"
            onKeyDown={(event) => {
              if (!(event.target instanceof HTMLButtonElement)) return;
              if (!["ArrowDown", "ArrowUp", "Home", "End"].includes(event.key)) return;
              const buttons = Array.from(
                event.currentTarget.querySelectorAll<HTMLButtonElement>("button"),
              );
              const currentIndex = buttons.indexOf(event.target);
              const nextIndex = event.key === "Home"
                ? 0
                : event.key === "End"
                  ? buttons.length - 1
                  : Math.min(
                      buttons.length - 1,
                      Math.max(0, currentIndex + (event.key === "ArrowDown" ? 1 : -1)),
                    );
              event.preventDefault();
              buttons[nextIndex]?.focus();
            }}
          >
            {systemTimezone && !normalizedQuery ? (
              <div className="aw-scheduled-task-editor__timezone-system">
                <span>本机时区</span>
                <button
                  type="button"
                  role="option"
                  aria-selected={value === systemTimezone}
                  onClick={() => {
                    onChange(systemTimezone);
                    setOpen(false);
                  }}
                >
                  <span>
                    <strong>{presentAwakenTimezone(systemTimezone, referenceDate).name}</strong>
                    <small>{presentAwakenTimezone(systemTimezone, referenceDate).offsetLabel}</small>
                  </span>
                  {value === systemTimezone ? <AwakenIcon concept="step-done" size={16} /> : null}
                </button>
              </div>
            ) : null}
            {groups.map((group) => (
              <section key={group} aria-label={group}>
                <h4>{group}</h4>
                {filtered.filter((option) => option.group === group).map((option) => (
                  <button
                    type="button"
                    role="option"
                    aria-selected={value === option.id}
                    key={option.id}
                    onClick={() => {
                      onChange(option.id);
                      setOpen(false);
                    }}
                  >
                    <span>
                      <strong>{option.name}</strong>
                      <small>{option.offsetLabel}</small>
                    </span>
                    {value === option.id ? <AwakenIcon concept="step-done" size={16} /> : null}
                  </button>
                ))}
              </section>
            ))}
            {filtered.length === 0 ? (
              <p className="aw-scheduled-task-editor__timezone-empty">没有匹配的时区</p>
            ) : null}
          </div>
        </div>
      }
    />
  );
}
