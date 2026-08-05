import "./AwakenSelect.css";
import { useId, useRef, useState } from "react";
import { Check, ChevronDown } from "lucide-react";

export type AwakenSelectOption = {
  value: string;
  label: string;
  disabled?: boolean;
};

export type AwakenSelectProps = {
  value: string;
  options: readonly AwakenSelectOption[];
  placeholder?: string;
  ariaLabel?: string;
  labelledBy?: string;
  disabled?: boolean;
  className?: string;
  onValueChange: (value: string) => void;
};

export function AwakenSelect({
  value,
  options,
  placeholder = "请选择",
  ariaLabel,
  labelledBy,
  disabled = false,
  className,
  onValueChange,
}: AwakenSelectProps) {
  const [open, setOpen] = useState(false);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const menuId = useId();
  const selectedOption = options.find((option) => option.value === value);

  function focusOption(edge: "first" | "last" | "selected"): void {
    window.setTimeout(() => {
      const menu = document.getElementById(menuId);
      const enabledOptions = Array.from(
        menu?.querySelectorAll<HTMLButtonElement>(
          '.aw-select__option:not(:disabled)',
        ) ?? [],
      );
      const target =
        edge === "selected"
          ? enabledOptions.find(
              (option) => option.getAttribute("data-value") === value,
            ) ?? enabledOptions[0]
          : edge === "last"
            ? enabledOptions.at(-1)
            : enabledOptions[0];
      target?.focus();
    }, 0);
  }

  function choose(nextValue: string): void {
    onValueChange(nextValue);
    setOpen(false);
    window.setTimeout(() => triggerRef.current?.focus(), 0);
  }

  return (
    <div
      className={`aw-select${className ? ` ${className}` : ""}`}
      onBlur={(event) => {
        if (!event.currentTarget.contains(event.relatedTarget)) setOpen(false);
      }}
    >
      <button
        ref={triggerRef}
        type="button"
        className="aw-select__trigger"
        aria-label={labelledBy ? undefined : ariaLabel}
        aria-labelledby={labelledBy}
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-controls={open ? menuId : undefined}
        disabled={disabled}
        onClick={() => setOpen((current) => !current)}
        onKeyDown={(event) => {
          if (event.key === "Escape") {
            setOpen(false);
            return;
          }
          if (event.key === "ArrowDown" || event.key === "ArrowUp") {
            event.preventDefault();
            if (!open) setOpen(true);
            focusOption(event.key === "ArrowUp" ? "last" : "selected");
          }
        }}
      >
        <span data-placeholder={selectedOption ? undefined : "true"}>
          {selectedOption?.label ?? placeholder}
        </span>
        <ChevronDown
          className="aw-select__chevron"
          size={16}
          strokeWidth={1.9}
          aria-hidden="true"
        />
      </button>

      {open ? (
        <div
          id={menuId}
          className="aw-select__menu"
          role="listbox"
          aria-label={labelledBy ? undefined : ariaLabel}
          aria-labelledby={labelledBy}
          onKeyDown={(event) => {
            if (event.key === "Escape") {
              event.preventDefault();
              setOpen(false);
              triggerRef.current?.focus();
              return;
            }
            if (!(event.target instanceof HTMLButtonElement)) return;
            if (!["ArrowDown", "ArrowUp", "Home", "End"].includes(event.key)) {
              return;
            }
            const enabledOptions = Array.from(
              event.currentTarget.querySelectorAll<HTMLButtonElement>(
                ".aw-select__option:not(:disabled)",
              ),
            );
            const currentIndex = enabledOptions.indexOf(event.target);
            const nextIndex =
              event.key === "Home"
                ? 0
                : event.key === "End"
                  ? enabledOptions.length - 1
                  : Math.min(
                      enabledOptions.length - 1,
                      Math.max(
                        0,
                        currentIndex + (event.key === "ArrowDown" ? 1 : -1),
                      ),
                    );
            event.preventDefault();
            enabledOptions[nextIndex]?.focus();
          }}
        >
          {options.map((option) => (
            <button
              type="button"
              className="aw-select__option"
              role="option"
              aria-selected={value === option.value}
              data-value={option.value}
              disabled={option.disabled}
              key={option.value}
              onClick={() => choose(option.value)}
            >
              <span>{option.label}</span>
              {value === option.value ? (
                <Check size={15} strokeWidth={2} aria-hidden="true" />
              ) : null}
            </button>
          ))}
          {options.length === 0 ? (
            <p className="aw-select__empty">当前没有可选项</p>
          ) : null}
        </div>
      ) : null}
    </div>
  );
}
