import type { ButtonHTMLAttributes, ReactNode } from "react";

type IconButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & {
  label: string;
  variant?: "square" | "circle";
  size?: "sm" | "md";
  children: ReactNode;
};

export function IconButton({
  label,
  variant = "square",
  size = "md",
  children,
  disabled,
  ...props
}: IconButtonProps) {
  const dimension = size === "sm" ? 32 : 34;
  const borderRadius = variant === "circle" ? "999px" : "var(--aw-radius-sm)";

  return (
    <button
      type="button"
      aria-label={label}
      title={label}
      disabled={disabled}
      {...props}
      style={{
        display: "inline-grid",
        placeItems: "center",
        width: dimension,
        height: dimension,
        border: "1px solid transparent",
        borderRadius,
        background: "transparent",
        color: "var(--aw-text-secondary)",
        cursor: disabled ? "not-allowed" : "pointer",
        opacity: disabled ? 0.45 : 1,
        transition: "background-color 160ms ease, color 160ms ease, border-color 160ms ease",
        ...props.style,
      }}
      onMouseEnter={(e) => {
        if (!disabled) {
          e.currentTarget.style.background = "var(--aw-bg-fill-main)";
        }
        props.onMouseEnter?.(e);
      }}
      onMouseLeave={(e) => {
        e.currentTarget.style.background = "transparent";
        props.onMouseLeave?.(e);
      }}
      onMouseDown={(e) => {
        if (!disabled) {
          e.currentTarget.style.background = "var(--aw-bg-fill-dark)";
        }
        props.onMouseDown?.(e);
      }}
      onMouseUp={(e) => {
        e.currentTarget.style.background = "var(--aw-bg-fill-main)";
        props.onMouseUp?.(e);
      }}
    >
      {children}
    </button>
  );
}
