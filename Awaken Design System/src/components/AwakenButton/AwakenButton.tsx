import "./AwakenButton.css";
import type { ButtonHTMLAttributes, ReactNode } from "react";

export type AwakenButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: "default" | "primary" | "ghost";
  size?: "sm" | "md" | "lg";
  icon?: ReactNode;
  loading?: boolean;
  children: ReactNode;
};

export function AwakenButton({
  variant = "default",
  size = "md",
  icon,
  loading,
  children,
  disabled,
  ...props
}: AwakenButtonProps) {
  const height = size === "sm" ? 24 : size === "md" ? 28 : 32;
  const padding = size === "sm" ? "0 8px" : size === "md" ? "0 12px" : "0 16px";
  const fontSize = size === "sm" ? "var(--aw-font-size-sm)" : "var(--aw-font-size-base)";

  const isPrimary = variant === "primary";
  const isGhost = variant === "ghost";

  const baseStyle: React.CSSProperties = {
    display: "inline-flex",
    alignItems: "center",
    justifyContent: "center",
    gap: "6px",
    height,
    padding,
    borderRadius: "var(--aw-radius-sm)",
    fontSize,
    fontWeight: "var(--aw-font-weight-semibold)",
    lineHeight: "var(--aw-line-height-base)",
    whiteSpace: "nowrap",
    cursor: disabled || loading ? "not-allowed" : "pointer",
    opacity: disabled || loading ? 0.45 : 1,
    transition: "background-color var(--aw-transition-fast), color var(--aw-transition-fast), border-color var(--aw-transition-fast), opacity var(--aw-transition-fast)",
  };

  const variantStyle: React.CSSProperties = isPrimary
    ? {
        border: "1px solid var(--aw-button-black)",
        background: "var(--aw-button-black)",
        color: "var(--aw-text-onblack)",
      }
    : isGhost
      ? {
          border: "1px solid transparent",
          background: "transparent",
          color: "var(--aw-text-secondary)",
        }
      : {
        border: "1px solid var(--aw-border-main)",
        background: "var(--aw-bg-fill-light)",
        color: "var(--aw-text-primary)",
        };

  return (
    <button
      type="button"
      disabled={disabled || loading}
      {...props}
      style={{ ...baseStyle, ...variantStyle, ...props.style }}
      onMouseEnter={(e) => {
        if (!disabled && !loading) {
          if (isPrimary) {
            e.currentTarget.style.background = "var(--aw-button-black-hover)";
          } else {
            e.currentTarget.style.background = isGhost ? "var(--aw-bg-fill-light)" : "var(--aw-bg-fill-main)";
          }
        }
        props.onMouseEnter?.(e);
      }}
      onMouseLeave={(e) => {
        e.currentTarget.style.background = variantStyle.background as string;
        props.onMouseLeave?.(e);
      }}
    >
      {loading ? (
        <span
          style={{
            width: 14,
            height: 14,
            border: "2px solid currentColor",
            borderTopColor: "transparent",
            borderRadius: "999px",
            animation: "spin 900ms linear infinite",
          }}
        />
      ) : (
        icon
      )}
      {children}
    </button>
  );
}
