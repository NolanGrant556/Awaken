import "./AwakenSettingsSwitch.css";
import { AwakenSwitch } from "../AwakenSwitch";

export type AwakenSettingsSwitchProps = {
  label: string;
  ariaLabel?: string;
  description?: string;
  checked: boolean;
  disabled?: boolean;
  busy?: boolean;
  variant?: "control" | "inline" | "card";
  onCheckedChange?: (checked: boolean) => void;
};

export function AwakenSettingsSwitch({
  label,
  ariaLabel,
  description,
  checked,
  disabled = false,
  busy = false,
  variant = "inline",
  onCheckedChange,
}: AwakenSettingsSwitchProps) {
  // busy 必须阻止重复操作，但它是瞬时态：外层行不跟着变灰或换光标，避免保存时闪烁。
  const permanentlyUnavailable = disabled || !onCheckedChange;
  const unavailable = permanentlyUnavailable || busy;
  const control = <AwakenSwitch checked={checked} label={ariaLabel ?? label} busy={busy} disabled={unavailable} size="compact" onCheckedChange={onCheckedChange} />;

  if (variant === "control") return control;

  return (
    <div
      className={[
        "aw-settings-switch",
        `aw-settings-switch--${variant}`,
        permanentlyUnavailable ? "aw-settings-switch--disabled" : "",
      ].filter(Boolean).join(" ")}
    >
      <span className="aw-settings-switch__copy">
        <strong>{label}</strong>
        {description ? <span>{description}</span> : null}
      </span>
      {control}
    </div>
  );
}
