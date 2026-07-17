import "./AwakenIconButton.css";
import type { ButtonHTMLAttributes, ReactNode } from "react";
import { AwakenTooltip } from "../AwakenTooltip";

export type AwakenIconButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & {
  label: string;
  variant?: "square" | "circle";
  size?: "sm" | "md";
  showTooltip?: boolean;
  children: ReactNode;
};

export function AwakenIconButton({
  label,
  variant = "square",
  size = "md",
  showTooltip = true,
  children,
  disabled,
  title,
  ...props
}: AwakenIconButtonProps) {
  const dimension = size === "sm" ? 28 : 32;
  const borderRadius = variant === "circle" ? "999px" : "var(--aw-radius-sm)";

  const button = (
    <button
      type="button"
      aria-label={label}
      title={showTooltip ? (title ?? label) : undefined}
      disabled={disabled}
      {...props}
      style={{
        display: "inline-grid",
        placeItems: "center",
        width: dimension,
        height: dimension,
        padding: 0,
        border: "1px solid transparent",
        borderRadius,
        background: "transparent",
        color: "var(--aw-text-secondary)",
        cursor: disabled ? "not-allowed" : "pointer",
        opacity: disabled ? 0.45 : 1,
        transition: "background-color var(--aw-transition-fast), color var(--aw-transition-fast), border-color var(--aw-transition-fast)",
        ...props.style,
      }}
      onMouseEnter={(e) => {
        if (!disabled) {
          e.currentTarget.style.background = "var(--aw-bg-fill-light)";
        }
        props.onMouseEnter?.(e);
      }}
      onMouseLeave={(e) => {
        e.currentTarget.style.background = "transparent";
        props.onMouseLeave?.(e);
      }}
      onMouseDown={(e) => {
        if (!disabled) {
          e.currentTarget.style.background = "var(--aw-bg-fill-main)";
        }
        props.onMouseDown?.(e);
      }}
      onMouseUp={(e) => {
        e.currentTarget.style.background = "var(--aw-bg-fill-light)";
        props.onMouseUp?.(e);
      }}
    >
      {children}
    </button>
  );

  return <AwakenTooltip label={label} disabled={!showTooltip}>{button}</AwakenTooltip>;
}
