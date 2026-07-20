import "./AwakenConnectorToggle.css";
import type { ButtonHTMLAttributes } from "react";

export type AwakenConnectorToggleProps = Omit<
  ButtonHTMLAttributes<HTMLButtonElement>,
  "children" | "onChange"
> & {
  checked: boolean;
  label: string;
  busy?: boolean;
  onCheckedChange?: (checked: boolean) => void;
};

export function AwakenConnectorToggle({
  checked,
  label,
  busy = false,
  disabled,
  className,
  onCheckedChange,
  onClick,
  ...props
}: AwakenConnectorToggleProps) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={`${checked ? "停用" : "启用"} ${label}`}
      aria-busy={busy || undefined}
      data-checked={checked ? "true" : "false"}
      data-busy={busy ? "true" : "false"}
      disabled={disabled || busy}
      {...props}
      className={["aw-connector-toggle", className ?? ""].filter(Boolean).join(" ")}
      onClick={(event) => {
        onClick?.(event);
        if (!event.defaultPrevented && !disabled && !busy) {
          onCheckedChange?.(!checked);
        }
      }}
    >
      <span className="aw-connector-toggle__thumb" aria-hidden="true" />
    </button>
  );
}
