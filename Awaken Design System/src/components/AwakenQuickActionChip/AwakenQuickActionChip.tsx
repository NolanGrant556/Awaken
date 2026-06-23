import "./AwakenQuickActionChip.css";
import type { ReactNode } from "react";

export type AwakenQuickActionChipProps = {
  icon?: ReactNode;
  label: string;
  prompt: string;
  onClick?: (prompt: string) => void;
  disabled?: boolean;
};

export function AwakenQuickActionChip({
  icon,
  label,
  prompt,
  onClick,
  disabled,
}: AwakenQuickActionChipProps) {
  return (
    <button
      type="button"
      onClick={() => onClick?.(prompt)}
      disabled={disabled}
      style={{
        display: "inline-flex",
        alignItems: "center",
        gap: "6px",
        height: 28,
        padding: "0 12px",
        border: "1px solid var(--aw-border-main)",
        borderRadius: "var(--aw-radius-pill)",
        background: "transparent",
        color: "var(--aw-text-secondary)",
        fontSize: "var(--aw-font-size-sm)",
        lineHeight: "var(--aw-line-height-sm)",
        whiteSpace: "nowrap",
        cursor: disabled ? "not-allowed" : "pointer",
        opacity: disabled ? 0.45 : 1,
        transition: "background-color var(--aw-transition-fast), color var(--aw-transition-fast), border-color var(--aw-transition-fast)",
      }}
      onMouseEnter={(e) => {
        if (!disabled) {
          e.currentTarget.style.background = "var(--aw-bg-fill-light)";
          e.currentTarget.style.color = "var(--aw-text-primary)";
          e.currentTarget.style.borderColor = "var(--aw-border-main)";
        }
      }}
      onMouseLeave={(e) => {
        e.currentTarget.style.background = "transparent";
        e.currentTarget.style.color = "var(--aw-text-secondary)";
        e.currentTarget.style.borderColor = "var(--aw-border-main)";
      }}
      onMouseDown={(e) => {
        if (!disabled) {
          e.currentTarget.style.background = "var(--aw-bg-fill-main)";
        }
      }}
      onMouseUp={(e) => {
        e.currentTarget.style.background = "var(--aw-bg-fill-light)";
      }}
    >
      {icon}
      <span className="aw-truncate">{label}</span>
    </button>
  );
}
