import "./AwakenSettingsSwitch.css";

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
  const unavailable = disabled || busy || !onCheckedChange;
  const control = (
    <span className="configured-model-switch">
      <input
        type="checkbox"
        checked={checked}
        disabled={unavailable}
        aria-label={ariaLabel ?? label}
        aria-busy={busy || undefined}
        onChange={(event) => onCheckedChange?.(event.target.checked)}
      />
      <span />
    </span>
  );

  if (variant === "control") return control;

  return (
    <label
      className={[
        "aw-settings-switch",
        `aw-settings-switch--${variant}`,
        unavailable ? "aw-settings-switch--disabled" : "",
      ].filter(Boolean).join(" ")}
    >
      <span className="aw-settings-switch__copy">
        <strong>{label}</strong>
        {description ? <span>{description}</span> : null}
      </span>
      {control}
    </label>
  );
}
