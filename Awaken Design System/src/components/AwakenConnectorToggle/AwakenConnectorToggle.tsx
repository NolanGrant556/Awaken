import { AwakenSwitch, type AwakenSwitchProps } from "../AwakenSwitch";

export type AwakenConnectorToggleProps = AwakenSwitchProps;

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
  return <AwakenSwitch checked={checked} label={label} busy={busy} disabled={disabled} className={className} onCheckedChange={onCheckedChange} onClick={onClick} {...props} />;
}
