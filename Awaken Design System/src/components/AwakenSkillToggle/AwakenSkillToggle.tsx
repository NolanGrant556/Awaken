import { AwakenSwitch, type AwakenSwitchProps } from "../AwakenSwitch";

export type AwakenSkillToggleProps = AwakenSwitchProps;

export function AwakenSkillToggle({
  checked,
  label,
  busy = false,
  disabled,
  className,
  onCheckedChange,
  onClick,
  ...props
}: AwakenSkillToggleProps) {
  return <AwakenSwitch checked={checked} label={label} busy={busy} disabled={disabled} className={className} onCheckedChange={onCheckedChange} onClick={onClick} {...props} />;
}
