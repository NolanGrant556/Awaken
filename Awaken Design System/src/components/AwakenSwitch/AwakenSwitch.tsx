import "./AwakenSwitch.css";
import type { ButtonHTMLAttributes } from "react";

export type AwakenSwitchProps = Omit<
  ButtonHTMLAttributes<HTMLButtonElement>,
  "children" | "onChange"
> & {
  checked: boolean;
  label: string;
  busy?: boolean;
  size?: "default" | "compact";
  onCheckedChange?: (checked: boolean) => void;
};

export function AwakenSwitch({
  checked,
  label,
  busy = false,
  size = "default",
  disabled,
  className,
  onCheckedChange,
  onClick,
  ...props
}: AwakenSwitchProps) {
  const unavailable = disabled || busy;

  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={`${checked ? "关闭" : "开启"}${label}`}
      aria-busy={busy || undefined}
      data-checked={checked ? "true" : "false"}
      data-busy={busy ? "true" : "false"}
      data-size={size}
      disabled={unavailable}
      {...props}
      className={["aw-switch", className ?? ""].filter(Boolean).join(" ")}
      onClick={(event) => {
        onClick?.(event);
        if (!event.defaultPrevented && !unavailable) {
          onCheckedChange?.(!checked);
        }
      }}
    >
      <span className="aw-switch__thumb" aria-hidden="true" />
    </button>
  );
}
