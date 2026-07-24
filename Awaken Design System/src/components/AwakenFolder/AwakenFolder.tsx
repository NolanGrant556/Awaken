import "./AwakenFolder.css";
import type { ButtonHTMLAttributes } from "react";

export type AwakenFolderProps = Omit<
  ButtonHTMLAttributes<HTMLButtonElement>,
  "children" | "title"
> & {
  label: string;
  meta?: string;
  size?: "default" | "compact" | "dense";
  open?: boolean;
  onOpen?: () => void;
};

export function AwakenFolder({
  label,
  meta,
  size = "default",
  open = false,
  onOpen,
  disabled,
  className,
  onClick,
  "aria-label": ariaLabel,
  "aria-expanded": ariaExpanded,
  ...props
}: AwakenFolderProps) {
  return (
    <button
      type="button"
      aria-label={ariaLabel ?? `打开${label}`}
      aria-expanded={ariaExpanded ?? open}
      data-open={open ? "true" : "false"}
      data-size={size}
      disabled={disabled}
      {...props}
      title={label}
      className={["aw-folder", className ?? ""].filter(Boolean).join(" ")}
      onClick={(event) => {
        onClick?.(event);
        if (!event.defaultPrevented && !disabled) {
          onOpen?.();
        }
      }}
    >
      <span className="aw-folder__stage">
        <span className="aw-folder__layer aw-folder__layer--back" aria-hidden="true" />
        <span className="aw-folder__layer aw-folder__layer--paper-rear" aria-hidden="true" />
        <span className="aw-folder__layer aw-folder__layer--paper-front" aria-hidden="true">
          <span className="aw-folder__sheet-lines" />
        </span>
        <span className="aw-folder__face">
          <span className="aw-folder__copy">
            <span className="aw-folder__name">{label}</span>
            {meta ? <span className="aw-folder__count">{meta}</span> : null}
          </span>
        </span>
      </span>
    </button>
  );
}
