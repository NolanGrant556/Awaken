import "./AwakenPill.css";
import type { ButtonHTMLAttributes, ReactNode } from "react";

export type AwakenPillProps = ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: "default" | "bordered";
  size?: "sm" | "md";
  icon?: ReactNode;
  active?: boolean;
  children: ReactNode;
};

export function AwakenPill({
  variant = "default",
  size = "md",
  icon,
  active,
  children,
  disabled,
  ...props
}: AwakenPillProps) {
  const height = size === "sm" ? 28 : 32;
  const isBordered = variant === "bordered";

  const baseStyle: React.CSSProperties = {
    display: "inline-flex",
    alignItems: "center",
    justifyContent: "center",
    gap: "6px",
    height,
    maxWidth: "100%",
    padding: "0 10px",
    border: isBordered ? "1px solid var(--aw-border-main)" : "1px solid transparent",
    borderRadius: "var(--aw-radius-pill)",
    background: active ? "var(--aw-bg-fill-main)" : "transparent",
    color: active ? "var(--aw-text-primary)" : "var(--aw-text-secondary)",
    fontSize: "var(--aw-font-size-sm)",
    lineHeight: "var(--aw-line-height-sm)",
    whiteSpace: "nowrap",
    cursor: disabled ? "not-allowed" : "pointer",
    opacity: disabled ? 0.45 : 1,
    transition: "background-color var(--aw-transition-fast), color var(--aw-transition-fast), border-color var(--aw-transition-fast)",
  };

  return (
    <button
      type="button"
      disabled={disabled}
      aria-pressed={active}
      {...props}
      style={{ ...baseStyle, ...props.style }}
      onMouseEnter={(e) => {
        if (!disabled) {
          e.currentTarget.style.background = active ? "var(--aw-bg-fill-main)" : "var(--aw-bg-fill-light)";
          e.currentTarget.style.color = "var(--aw-text-primary)";
        }
        props.onMouseEnter?.(e);
      }}
      onMouseLeave={(e) => {
        e.currentTarget.style.background = active ? "var(--aw-bg-fill-main)" : "transparent";
        e.currentTarget.style.color = active ? "var(--aw-text-primary)" : "var(--aw-text-secondary)";
        props.onMouseLeave?.(e);
      }}
    >
      {icon}
      <span className="aw-truncate">{children}</span>
    </button>
  );
}
