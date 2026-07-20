import "./AwakenSettingsProviderCard.css";
import { useState, type ReactNode } from "react";
import { ChevronDown } from "lucide-react";

export type AwakenSettingsProviderCardProps = {
  name: string;
  iconLabel?: string;
  expanded?: boolean;
  defaultExpanded?: boolean;
  disabled?: boolean;
  children?: ReactNode;
  onExpandedChange?: (expanded: boolean) => void;
};

export function AwakenSettingsProviderCard({
  name,
  iconLabel,
  expanded,
  defaultExpanded = false,
  disabled = false,
  children,
  onExpandedChange,
}: AwakenSettingsProviderCardProps) {
  const [internalExpanded, setInternalExpanded] = useState(defaultExpanded);
  const isControlled = expanded !== undefined;
  const isExpanded = isControlled ? expanded : internalExpanded;

  function toggle(): void {
    if (disabled) return;
    const nextExpanded = !isExpanded;
    if (!isControlled) setInternalExpanded(nextExpanded);
    onExpandedChange?.(nextExpanded);
  }

  return (
    <div className="model-provider-card">
      <button
        type="button"
        className="model-provider-toggle"
        aria-expanded={isExpanded}
        disabled={disabled}
        onClick={toggle}
      >
        <span className="model-provider-icon">{iconLabel ?? name.slice(0, 1)}</span>
        <span className="model-provider-title"><strong>{name}</strong></span>
        <ChevronDown className="model-provider-chevron" size={15} strokeWidth={1.9} aria-hidden="true" />
      </button>
      {isExpanded ? <div className="model-provider-form">{children}</div> : null}
    </div>
  );
}
