import "./AwakenSelect.css";
import { AwakenIcon } from "../AwakenIcon";
import { useId, useState } from "react";
import {
  useAwakenAnchoredMenuLayout,
  type AwakenAnchoredMenuPlacement,
} from "../../hooks/useAwakenAnchoredMenuLayout";

const MENU_GAP = 6;
const MENU_MAX_HEIGHT = 180;
const MENU_OPTION_HEIGHT = 32;
const MENU_OPTION_GAP = 2;
const MENU_VERTICAL_PADDING = 10;

function estimateMenuHeight(optionCount: number): number {
  if (optionCount === 0) return 58;
  return Math.min(
    MENU_MAX_HEIGHT,
    MENU_VERTICAL_PADDING +
      optionCount * MENU_OPTION_HEIGHT +
      Math.max(0, optionCount - 1) * MENU_OPTION_GAP,
  );
}

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
  preferredPlacement?: AwakenAnchoredMenuPlacement;
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
  preferredPlacement = "bottom",
  onValueChange,
}: AwakenSelectProps) {
  const [open, setOpen] = useState(false);
  const {
    anchorRef: triggerRef,
    menuRef,
    placement: menuPlacement,
    maxHeight: menuMaxHeight,
    updateLayout: updateMenuLayout,
  } = useAwakenAnchoredMenuLayout<HTMLButtonElement, HTMLDivElement>({
    open,
    estimatedHeight: estimateMenuHeight(options.length),
    maxHeight: MENU_MAX_HEIGHT,
    preferredPlacement,
    gap: MENU_GAP,
  });
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
      data-menu-placement={menuPlacement}
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
        onClick={() => {
          if (!open) updateMenuLayout();
          setOpen((current) => !current);
        }}
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
        <AwakenIcon concept="dropdown" size={16} className="aw-select__chevron" />
      </button>

      {open ? (
        <div
          ref={menuRef}
          id={menuId}
          className="aw-select__menu"
          style={{ maxHeight: menuMaxHeight }}
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
                <AwakenIcon concept="step-done" size={16} />
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
